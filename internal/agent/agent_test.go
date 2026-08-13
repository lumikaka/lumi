package agent

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/files"
	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/llmlog"
	"lumi/internal/modelsettings"
	"lumi/internal/picturebook"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
	"lumi/internal/story"
)

type agentQueueFake struct {
	mu       sync.Mutex
	nextID   int64
	jobs     []JobSpec
	cancels  []string
	tasks    map[string]DomainTask
	retries  []string
	requests []DomainTaskRequest
}

func (queue *agentQueueFake) EnqueueAgentTx(_ context.Context, _ string, _ *sql.Tx, spec JobSpec) (int64, error) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	queue.nextID++
	queue.jobs = append(queue.jobs, spec)
	return queue.nextID, nil
}

func (queue *agentQueueFake) CancelAgentJob(context.Context, string, int64) error { return nil }

func (queue *agentQueueFake) CancelAgentWork(_, workUUID string) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	queue.cancels = append(queue.cancels, workUUID)
}

func (queue *agentQueueFake) StartDomainTask(_ context.Context, _ string, request DomainTaskRequest) (DomainTask, error) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	queue.requests = append(queue.requests, request)
	uuid, err := newUUIDv7()
	if err != nil {
		return DomainTask{}, err
	}
	task := DomainTask{UUID: uuid, Kind: request.Kind, ResourceUUID: request.ResourceUUID, Status: "queued"}
	if queue.tasks == nil {
		queue.tasks = make(map[string]DomainTask)
	}
	queue.tasks[uuid] = task
	return task, nil
}

func (queue *agentQueueFake) GetDomainTask(_ context.Context, _ string, _ string, taskUUID string) (DomainTask, error) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	task, ok := queue.tasks[taskUUID]
	if !ok {
		return DomainTask{}, errors.New("unexpected domain task")
	}
	return task, nil
}

func (queue *agentQueueFake) CancelDomainTask(context.Context, string, string, string) error {
	return nil
}

func (queue *agentQueueFake) RetryDomainTask(_ context.Context, _ string, _ string, taskUUID string) (DomainTask, error) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	task, ok := queue.tasks[taskUUID]
	if !ok {
		return DomainTask{}, errors.New("unexpected domain task retry")
	}
	task.Status = "queued"
	queue.tasks[taskUUID] = task
	queue.retries = append(queue.retries, taskUUID)
	return task, nil
}

type toolModelFake struct {
	mu        sync.Mutex
	responses []llm.ChatResponse
	requests  []llm.ChatRequest
	calls     int
	onCall    func(int)
	respond   func(int, llm.ChatRequest) (llm.ChatResponse, error)
}

func (model *toolModelFake) Complete(_ context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	model.mu.Lock()
	model.calls++
	call := model.calls
	model.requests = append(model.requests, request)
	hook := model.onCall
	respond := model.respond
	if respond != nil {
		model.mu.Unlock()
		if hook != nil {
			hook(call)
		}
		return respond(call, request)
	}
	if len(model.responses) == 0 {
		model.mu.Unlock()
		if hook != nil {
			hook(call)
		}
		return finalResponse("完成"), nil
	}
	response := model.responses[0]
	model.responses = model.responses[1:]
	model.mu.Unlock()
	if hook != nil {
		hook(call)
	}
	return response, nil
}

type imageClientFake struct {
	mu        sync.Mutex
	requests  []imagegen.Request
	deadlines []time.Time
	response  imagegen.Response
	err       error
	generate  func(context.Context, imagegen.Request) (imagegen.Response, error)
}

func (client *imageClientFake) Generate(ctx context.Context, request imagegen.Request) (imagegen.Response, error) {
	client.mu.Lock()
	copyRequest := request
	copyRequest.Images = append([]imagegen.ImageInput(nil), request.Images...)
	client.requests = append(client.requests, copyRequest)
	deadline, _ := ctx.Deadline()
	client.deadlines = append(client.deadlines, deadline)
	response, err, generate := client.response, client.err, client.generate
	client.mu.Unlock()
	if generate != nil {
		return generate(ctx, request)
	}
	if err != nil {
		return imagegen.Response{}, err
	}
	if ctx.Err() != nil {
		return imagegen.Response{}, ctx.Err()
	}
	return response, nil
}

type agentHarness struct {
	app       *appstore.Store
	projects  *project.Manager
	provider  provider.Provider
	providers *provider.Service
	project   project.Summary
	store     *project.Store
	queue     *agentQueueFake
	model     *toolModelFake
	service   *Service
}

func newAgentHarness(t *testing.T, responses ...llm.ChatResponse) *agentHarness {
	t.Helper()
	return newAgentHarnessWithPictureBook(t, &project.PictureBookInput{Format: project.PictureBookVertical}, responses...)
}

func newAgentHarnessWithPictureBook(t *testing.T, pictureBook *project.PictureBookInput, responses ...llm.ChatResponse) *agentHarness {
	t.Helper()
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	configured, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/agent-model", DefaultImageModel: "test/image-model", APIKey: "must-not-leak"})
	if err != nil {
		t.Fatal(err)
	}
	projects := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen)
	created, err := projects.CreateWithInput(ctx, project.CreateInput{
		Name:        "Agent Test",
		PictureBook: pictureBook,
	}, project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	queue := &agentQueueFake{}
	model := &toolModelFake{responses: responses}
	service := NewService(projects, providers, model, queue, nil)
	var projectStore *project.Store
	if err := projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		projectStore = store
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	harness := &agentHarness{app: app, projects: projects, provider: configured, providers: providers, project: created, store: projectStore, queue: queue, model: model, service: service}
	t.Cleanup(func() {
		providers.Close()
		_ = projects.Close()
		_ = app.Close()
	})
	return harness
}

func TestYoloPageMomentPlanCoversVerticalStripAndOtherFormats(t *testing.T) {
	strip, stripMax := yoloPageMomentPlan(project.PictureBookProfile{Format: project.PictureBookVertical})
	if strip != "Section 1: 1; Section 2: 3; Section 3: 2; Section 4: 1; Section 5: 2; Section 6: 3" || stripMax != "3" {
		t.Fatalf("vertical strip plan=%q max=%s", strip, stripMax)
	}
	four := project.ComicLayoutFourPanel
	fourPlan, fourMax := yoloPageMomentPlan(project.PictureBookProfile{Format: project.PictureBookComicStory, ComicLayout: &four})
	if fourPlan != "[4,4,4,4,4,4]" || fourMax != "4" {
		t.Fatalf("four-panel plan=%q max=%s", fourPlan, fourMax)
	}
	page := project.ComicLayoutPageComic
	pagePlan, pageMax := yoloPageMomentPlan(project.PictureBookProfile{Format: project.PictureBookComicStory, ComicLayout: &page})
	if pagePlan != "[4,5,3,6,4,5]" || pageMax != "6" {
		t.Fatalf("page-comic plan=%q max=%s", pagePlan, pageMax)
	}
	classic, classicMax := yoloPageMomentPlan(project.PictureBookProfile{Format: project.PictureBookClassic})
	if classic != "[1,1,1,1,1,1]" || classicMax != "1" {
		t.Fatalf("classic plan=%q max=%s", classic, classicMax)
	}
}

func TestYoloRejectsUnsupportedPictureBookRatioBeforeCreatingWorkflow(t *testing.T) {
	harness := newAgentHarnessWithPictureBook(t, &project.PictureBookInput{Format: project.PictureBookClassic})
	_, err := harness.service.CreateYoloWorkflow(context.Background(), harness.project.UUID, CreateYoloInput{
		Title: "Unsupported ratio", StoryPrompt: "A landscape page.", IdempotencyKey: "unsupported-ratio-yolo",
	})
	var domainErr *Error
	if !errors.As(err, &domainErr) || domainErr.Code != picturebook.CodeAspectRatioUnsupported {
		t.Fatalf("error=%v", err)
	}
	var workflows, threads int64
	if err := harness.store.DB().Table("workflows").Count(&workflows).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chat_threads").Count(&threads).Error; err != nil {
		t.Fatal(err)
	}
	if workflows != 0 || threads != 0 {
		t.Fatalf("preflight persisted workflows=%d threads=%d", workflows, threads)
	}
}

func TestExistingThreadUsesNewActiveProviderOnlyForNewTurns(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread := harness.createThread(t)
	if _, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "first"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := harness.providers.Settings().Update(ctx, map[string]any{
		sitesettings.BailianWorkspaceKey: "workspace-1",
		sitesettings.BailianRegionKey:    "cn-beijing",
		sitesettings.BailianAPIKeyKey:    "bailian-secret",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := harness.providers.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bailian := items[1]
	if _, err := harness.providers.MarkVerified(ctx, bailian.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.providers.Activate(ctx, provider.TypeAliyunBailian); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "second"}); err != nil {
		t.Fatal(err)
	}
	var runs []runRecord
	if err := harness.store.DB().Order("id ASC").Find(&runs).Error; err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].ProviderUUID != harness.provider.UUID || runs[1].ProviderUUID != bailian.UUID || runs[1].Model != provider.BailianTextModel {
		t.Fatalf("run snapshots=%+v", runs)
	}
}

func TestChatThreadAndRunsFreezeProjectScenarioModelSource(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	resolver := modelsettings.NewResolver(harness.providers)
	settings, err := resolver.Patch(ctx, harness.store, modelsettings.PatchInput{ExpectedRevision: 0, Changes: map[string]*modelsettings.Selection{
		modelsettings.ChatArea: {ProviderUUID: harness.provider.UUID, Model: harness.provider.DefaultModel},
	}})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "场景模型"})
	if err != nil {
		t.Fatal(err)
	}
	if thread.ProviderUUID != harness.provider.UUID || thread.Model != harness.provider.DefaultModel || thread.ModelSource != modelsettings.SourceScenarioOverride {
		t.Fatalf("thread model snapshot=%+v", thread)
	}
	if _, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "first"}); err != nil {
		t.Fatal(err)
	}
	var firstRun runRecord
	if err := harness.store.DB().Order("id DESC").First(&firstRun).Error; err != nil {
		t.Fatal(err)
	}
	if firstRun.ProviderUUID != harness.provider.UUID || firstRun.Model != harness.provider.DefaultModel || firstRun.ModelSource != modelsettings.SourceScenarioOverride {
		t.Fatalf("run model snapshot=%+v", firstRun)
	}
	if _, err := resolver.Patch(ctx, harness.store, modelsettings.PatchInput{ExpectedRevision: settings.Revision, Changes: map[string]*modelsettings.Selection{modelsettings.ChatArea: nil}}); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "second"}); err != nil {
		t.Fatal(err)
	}
	var secondRun runRecord
	if err := harness.store.DB().Order("id DESC").First(&secondRun).Error; err != nil {
		t.Fatal(err)
	}
	if secondRun.ModelSource != modelsettings.SourceGlobalDefault || secondRun.ID == firstRun.ID {
		t.Fatalf("new run did not resolve current inherited setting: %+v", secondRun)
	}
}

func (harness *agentHarness) createThread(t *testing.T) Thread {
	t.Helper()
	thread, err := harness.service.CreateThread(context.Background(), harness.project.UUID, CreateThreadInput{Title: "创作助手", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	return thread
}

func TestPremiseThreadScopesAndSceneToolsStayBoundToSubject(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	projectThread := harness.createThread(t)
	generationThread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "生成单项", Scope: ThreadScopePremise, Scene: ScenePremiseAsset, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	productionService := production.NewService(harness.store, nil)
	upload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "reference.png", Reader: bytes.NewReader(agentTestPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := productionService.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetCharacter, Title: "月光邮差"})
	if err != nil {
		t.Fatal(err)
	}
	referenceThread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "引用设定", Scope: ThreadScopePremise, Scene: SceneAssetReference, SubjectUUID: asset.UUID, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	projectThreads, err := harness.service.ListThreads(ctx, harness.project.UUID, ThreadScopeProject)
	if err != nil || len(projectThreads) != 1 || projectThreads[0].UUID != projectThread.UUID {
		t.Fatalf("project threads=%+v err=%v", projectThreads, err)
	}
	premiseThreads, err := harness.service.ListThreads(ctx, harness.project.UUID, ThreadScopePremise)
	if err != nil || len(premiseThreads) != 2 || premiseThreads[0].Scope != ThreadScopePremise {
		t.Fatalf("premise threads=%+v err=%v", premiseThreads, err)
	}
	if referenceThread.SubjectUUID != asset.UUID || referenceThread.Scene != SceneAssetReference {
		t.Fatalf("reference thread=%+v", referenceThread)
	}

	var projectRow, generationRow, referenceRow threadRecord
	if err := harness.store.DB().Where("uuid = ?", projectThread.UUID).First(&projectRow).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Where("uuid = ?", generationThread.UUID).First(&generationRow).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Where("uuid = ?", referenceThread.UUID).First(&referenceRow).Error; err != nil {
		t.Fatal(err)
	}
	generationTools := toolDefinitionNames(llmToolDefinitions(generationRow))
	referenceTools := toolDefinitionNames(llmToolDefinitions(referenceRow))
	if !generationTools["image_gen"] || !generationTools["create_premise_asset"] || generationTools["generate_premise_asset"] || generationTools["generate_premise_asset_variant"] || generationTools["update_story_profile"] {
		t.Fatalf("generation scene tools=%v", generationTools)
	}
	if len(referenceTools) != 3 || !referenceTools[currentProjectAPIToolName] || !referenceTools["image_gen"] || !referenceTools["request_user_input"] || referenceTools["get_premise_asset"] || referenceTools["create_premise_asset"] || referenceTools["update_premise_asset"] {
		t.Fatalf("reference scene tools=%v", referenceTools)
	}
	if generationTools[currentProjectAPIToolName] || toolDefinitionNames(llmToolDefinitions(projectRow))[currentProjectAPIToolName] {
		t.Fatal("request_current_project_api leaked outside asset_reference")
	}
	otherUUID, _ := newUUIDv7()
	if _, err := parseCurrentProjectAPIRequest(toolContext{ProjectUUID: harness.project.UUID, Thread: referenceRow}, map[string]any{"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + otherUUID}); err == nil {
		t.Fatal("asset reference scene accepted a different target UUID")
	}
	if _, err := parseCurrentProjectAPIRequest(toolContext{ProjectUUID: harness.project.UUID, Thread: referenceRow}, map[string]any{"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + asset.UUID}); err != nil {
		t.Fatal("asset reference scene rejected its bound subject UUID")
	}
	premise, err := productionService.GetPremise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prompts, err := loadContextPrompts(ctx, harness.store, referenceRow)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{harness.project.UUID, asset.UUID, asset.Title, asset.CurrentVariant.Asset.UUID, premise.DefaultStyle, currentProjectAPIToolName, "POST", "PATCH", "DELETE"} {
		if !strings.Contains(prompts.Scene, expected) {
			t.Fatalf("asset reference prompt is missing %q: %s", expected, prompts.Scene)
		}
	}
	if strings.Contains(prompts.Scene, "{{") || strings.Contains(prompts.Scene, "get_premise_asset") || strings.Contains(prompts.Scene, "update_premise_asset") {
		t.Fatalf("asset reference prompt retained placeholders or typed tools: %s", prompts.Scene)
	}
}

func TestStoryboardReferenceThreadStaysBoundToOneComicSection(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	storyService := story.NewService(harness.store)
	chapter, err := storyService.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "月光来信", Content: "开篇", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	productionService := production.NewService(harness.store, nil)
	section, err := productionService.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "窗边来信", StoryboardMD: "## 原始分镜\n月光落在信封上。"})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "分镜引用 · 窗边来信", Scope: ThreadScopeProject, Scene: SceneStoryboardReference, SubjectUUID: section.UUID, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	if thread.Scope != ThreadScopeProject || thread.Scene != SceneStoryboardReference || thread.SubjectUUID != section.UUID {
		t.Fatalf("storyboard thread=%+v", thread)
	}

	var row threadRecord
	if err := harness.store.DB().Where("uuid = ?", thread.UUID).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	tools := toolDefinitionNames(llmToolDefinitions(row))
	if !tools["get_comic_section"] || !tools["update_comic_storyboard"] || !tools["request_user_input"] || tools["start_generation"] || tools["update_chapter_story"] {
		t.Fatalf("storyboard scene tools=%v", tools)
	}
	otherUUID, _ := newUUIDv7()
	if toolTargetAllowedForThread("update_comic_storyboard", map[string]any{"section_uuid": otherUUID}, row) {
		t.Fatal("storyboard reference scene accepted a different section UUID")
	}
	if !toolTargetAllowedForThread("update_comic_storyboard", map[string]any{"section_uuid": section.UUID}, row) {
		t.Fatal("storyboard reference scene rejected its bound section UUID")
	}
	prompts, err := loadContextPrompts(ctx, harness.store, row)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompts.Scene, chapter.UUID) || !strings.Contains(prompts.Scene, section.UUID) || strings.Contains(prompts.Scene, "{{") {
		t.Fatalf("storyboard scene prompt=%q", prompts.Scene)
	}
	missingUUID, _ := newUUIDv7()
	if _, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "越界引用", Scope: ThreadScopeProject, Scene: SceneStoryboardReference, SubjectUUID: missingUUID, ProviderUUID: harness.provider.UUID}); errorCode(err) != CodeNotFound {
		t.Fatalf("missing storyboard section error=%v", err)
	}
}

func toolDefinitionNames(definitions []llm.ToolDefinition) map[string]bool {
	result := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		result[definition.Name] = true
	}
	return result
}

func TestPremiseSceneGenerationToolsStartGoProductionTasks(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	generationThread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "生成单项", Scope: ThreadScopePremise, Scene: ScenePremiseAsset, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	generationTurn, err := harness.service.CreateTurn(ctx, harness.project.UUID, generationThread.UUID, CreateTurnInput{InputText: "生成月亮邮局"})
	if err != nil {
		t.Fatal(err)
	}
	generationContext, err := harness.service.loadToolContext(ctx, harness.store, generationThread.UUID, generationTurn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	createArgs := map[string]any{"asset_type": "scene", "title": "月亮邮局", "summary": "夜间邮局", "tags": []any{"night"}, "prompt": "夜蓝木屋与暖黄窗光"}
	encodedCreate, _ := json.Marshal(createArgs)
	result, err := harness.service.executeTool(ctx, harness.store, generationContext, toolExecutionRecord{ToolName: "generate_premise_asset", ArgumentsJSON: string(encodedCreate), IdempotencyKey: "single-scene-task"})
	if err != nil || !json.Valid(result) {
		t.Fatalf("create tool result=%s err=%v", result, err)
	}

	productionService := production.NewService(harness.store, nil)
	upload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "reference.png", Reader: bytes.NewReader(agentTestPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := productionService.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetCharacter, Title: "月光邮差"})
	if err != nil {
		t.Fatal(err)
	}
	referenceThread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "引用设定", Scope: ThreadScopePremise, Scene: SceneAssetReference, SubjectUUID: asset.UUID, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	referenceTurn, err := harness.service.CreateTurn(ctx, harness.project.UUID, referenceThread.UUID, CreateTurnInput{InputText: "改成冬季制服"})
	if err != nil {
		t.Fatal(err)
	}
	referenceContext, err := harness.service.loadToolContext(ctx, harness.store, referenceThread.UUID, referenceTurn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	variantArgs := map[string]any{"prompt": "保持身份特征，改成冬季邮差制服"}
	encodedVariant, _ := json.Marshal(variantArgs)
	result, err = harness.service.executeTool(ctx, harness.store, referenceContext, toolExecutionRecord{ToolName: "generate_premise_asset_variant", ArgumentsJSON: string(encodedVariant), IdempotencyKey: "variant-scene-task"})
	if err != nil || !json.Valid(result) {
		t.Fatalf("variant tool result=%s err=%v", result, err)
	}

	harness.queue.mu.Lock()
	requests := append([]DomainTaskRequest(nil), harness.queue.requests...)
	harness.queue.mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("domain task requests=%+v", requests)
	}
	if requests[0].Kind != KindPremiseAssetGeneration || requests[0].ResourceUUID != generationThread.UUID || requests[0].AssetOperation != "create" || requests[0].AssetTitle != "月亮邮局" {
		t.Fatalf("create request=%+v", requests[0])
	}
	if requests[1].Kind != KindPremiseAssetGeneration || requests[1].ResourceUUID != asset.UUID || requests[1].AssetOperation != "variant" || requests[1].AssetTitle != "" {
		t.Fatalf("variant request=%+v", requests[1])
	}
}

func TestChatImageAttachmentsPersistAndFollowInteractionRules(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "带图生成", Scope: ThreadScopePremise, Scene: ScenePremiseAsset, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	firstUpload := createChatReferenceUpload(t, harness.store, "first.png", agentTestColorPNG(t, color.RGBA{R: 220, A: 255}))
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "以这张图为参考", UploadUUIDs: []string{firstUpload.UUID}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := harness.service.ListItems(ctx, harness.project.UUID, thread.UUID, "", "", 20)
	if err != nil || len(page.Items) != 1 || len(page.Items[0].ImageReferences) != 1 || page.Items[0].ImageReferences[0].UploadUUID != firstUpload.UUID || !isUUIDv7(page.Items[0].ImageReferences[0].FileUUID) {
		t.Fatalf("initial image references=%+v err=%v", page.Items, err)
	}

	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.claimRun(ctx, harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	steering, err := harness.service.Steer(ctx, harness.project.UUID, thread.UUID, SteeringInput{InputText: "保留同一组参考图"})
	if err != nil || len(steering.ImageReferences) != 1 || steering.ImageReferences[0].FileUUID != page.Items[0].ImageReferences[0].FileUUID {
		t.Fatalf("inherited steering references=%+v err=%v", steering.ImageReferences, err)
	}

	secondUpload := createChatReferenceUpload(t, harness.store, "second.png", agentTestColorPNG(t, color.RGBA{G: 220, A: 255}))
	followUp, err := harness.service.CreateFollowUp(ctx, harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "下一轮使用绿色参考", UploadUUIDs: []string{secondUpload.UUID}})
	if err != nil || len(followUp.ImageReferences) != 1 || followUp.ImageReferences[0].UploadUUID != secondUpload.UUID {
		t.Fatalf("follow-up references=%+v err=%v", followUp.ImageReferences, err)
	}
	thirdUpload := createChatReferenceUpload(t, harness.store, "third.png", agentTestColorPNG(t, color.RGBA{B: 220, A: 255}))
	edited, err := harness.service.UpdateFollowUp(ctx, harness.project.UUID, thread.UUID, followUp.UUID, CreateFollowUpInput{InputText: "只修改文字", UploadUUIDs: []string{thirdUpload.UUID}})
	if err != nil || len(edited.ImageReferences) != 1 || edited.ImageReferences[0].UploadUUID != secondUpload.UUID {
		t.Fatalf("edited follow-up changed fixed references=%+v err=%v", edited.ImageReferences, err)
	}
	var referenceCount int64
	if err := harness.store.DB().Table("chat_follow_up_file_references AS refs").Joins("JOIN chat_follow_ups follow_ups ON follow_ups.id=refs.follow_up_id").Where("follow_ups.uuid=?", followUp.UUID).Count(&referenceCount).Error; err != nil || referenceCount != 1 {
		t.Fatalf("follow-up reference count=%d err=%v", referenceCount, err)
	}
	if err := harness.service.DeleteFollowUp(ctx, harness.project.UUID, thread.UUID, followUp.UUID); err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("chat_follow_up_file_references AS refs").Joins("JOIN chat_follow_ups follow_ups ON follow_ups.id=refs.follow_up_id").Where("follow_ups.uuid=?", followUp.UUID).Count(&referenceCount).Error; err != nil || referenceCount != 0 {
		t.Fatalf("deleted follow-up retained references=%d err=%v", referenceCount, err)
	}

	projectThread := harness.createThread(t)
	if _, err := harness.service.CreateTurn(ctx, harness.project.UUID, projectThread.UUID, CreateTurnInput{InputText: "普通对话", UploadUUIDs: []string{thirdUpload.UUID}}); errorCode(err) != CodeImageReferenceUnsupported {
		t.Fatalf("unsupported scene attachment error=%v", err)
	}
	storyboardThread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "分镜参考", Scope: ThreadScopeProject, Scene: SceneStoryboardReference, SubjectUUID: createComicSectionFixture(t, harness.store), ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.CreateTurn(ctx, harness.project.UUID, storyboardThread.UUID, CreateTurnInput{InputText: "分镜不可带图", UploadUUIDs: []string{thirdUpload.UUID}}); errorCode(err) != CodeImageReferenceUnsupported {
		t.Fatalf("storyboard attachment error=%v", err)
	}
	five := make([]string, 5)
	for index := range five {
		five[index], _ = newUUIDv7()
	}
	if _, err := harness.service.CreateFollowUp(ctx, harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "太多图片", UploadUUIDs: five}); errorCode(err) != CodeImageReferenceLimit {
		t.Fatalf("five attachments error=%v", err)
	}
	missingUploadUUID, _ := newUUIDv7()
	if _, err := harness.service.CreateFollowUp(ctx, harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "不存在附件", UploadUUIDs: []string{missingUploadUUID}}); errorCode(err) != CodeImageReferenceNotFound {
		t.Fatalf("missing attachment error=%v", err)
	}
	crossProjectUpload := createChatReferenceUpload(t, harness.store, "cross-project.png", agentTestPNG(t))
	crossProjectUUID, _ := newUUIDv7()
	now := time.Now().UTC()
	if err := harness.store.DB().Exec(`INSERT INTO projects(uuid,name,format_version,schema_version,created_at,updated_at) VALUES(?,?,1,16,?,?)`, crossProjectUUID, "Other project", now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Exec(`UPDATE upload_stashed SET project_id=(SELECT id FROM projects WHERE uuid=?) WHERE uuid=?`, crossProjectUUID, crossProjectUpload.UUID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.CreateFollowUp(ctx, harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "跨项目附件", UploadUUIDs: []string{crossProjectUpload.UUID}}); errorCode(err) != CodeImageReferenceProject {
		t.Fatalf("cross-project attachment error=%v", err)
	}
	fileService := files.NewService(harness.store, nil)
	expiredUpload := createChatReferenceUpload(t, harness.store, "expired.png", agentTestPNG(t))
	if err := fileService.CancelUpload(ctx, expiredUpload.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.CreateFollowUp(ctx, harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "过期附件", UploadUUIDs: []string{expiredUpload.UUID}}); errorCode(err) != CodeImageReferenceIncomplete {
		t.Fatalf("expired attachment error=%v", err)
	}
	invalidMIMEUpload := createChatReferenceUpload(t, harness.store, "invalid-mime.png", agentTestPNG(t))
	if err := harness.store.DB().Exec(`UPDATE upload_stashed SET mime_type='image/gif' WHERE uuid=?`, invalidMIMEUpload.UUID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.CreateFollowUp(ctx, harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "错误格式附件", UploadUUIDs: []string{invalidMIMEUpload.UUID}}); errorCode(err) != CodeImageReferenceInvalidMIME {
		t.Fatalf("invalid MIME attachment error=%v", err)
	}
	deletedUpload := createChatReferenceUpload(t, harness.store, "deleted.png", agentTestPNG(t))
	deletedFile, err := fileService.FinalizeUpload(ctx, deletedUpload.UUID, "project_chatbot_reference")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fileService.SoftDelete(ctx, deletedFile.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.CreateFollowUp(ctx, harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "已删除附件", UploadUUIDs: []string{deletedUpload.UUID}}); errorCode(err) != CodeImageReferenceNotFound {
		t.Fatalf("deleted attachment error=%v", err)
	}

	if _, err := harness.service.Abort(ctx, harness.project.UUID, thread.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "新 turn 不继承附件"}); err != nil {
		t.Fatal(err)
	}
	page, err = harness.service.ListItems(ctx, harness.project.UUID, thread.UUID, "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.Content == "新 turn 不继承附件" && len(item.ImageReferences) != 0 {
			t.Fatalf("ordinary new turn inherited references=%+v", item.ImageReferences)
		}
	}
}

func TestQueuedFollowUpSteersAtomicallyOrStaysQueuedAfterWindowCloses(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "排队引导", Scope: ThreadScopePremise, Scene: ScenePremiseAsset, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	upload := createChatReferenceUpload(t, harness.store, "steer.png", agentTestPNG(t))
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "开始", UploadUUIDs: []string{upload.UUID}})
	if err != nil {
		t.Fatal(err)
	}
	followUp, err := harness.service.CreateFollowUp(ctx, harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "立即改用这个方向", UploadUUIDs: []string{upload.UUID}})
	if err != nil {
		t.Fatal(err)
	}
	fallback, err := harness.service.SteerFollowUp(ctx, harness.project.UUID, thread.UUID, followUp.UUID)
	if err != nil || fallback.DeliveryMode != "follow_up" || fallback.FollowUp == nil || fallback.FollowUp.UUID != followUp.UUID {
		t.Fatalf("fallback delivery=%+v err=%v", fallback, err)
	}
	queued, err := harness.service.ListFollowUps(ctx, harness.project.UUID, thread.UUID)
	if err != nil || len(queued) != 1 {
		t.Fatalf("fallback lost queue=%+v err=%v", queued, err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.claimRun(ctx, harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	promoted, err := harness.service.SteerFollowUp(ctx, harness.project.UUID, thread.UUID, followUp.UUID)
	if err != nil || promoted.DeliveryMode != "steering" || promoted.Item == nil || promoted.Item.Content != followUp.InputText || len(promoted.Item.ImageReferences) != 1 {
		t.Fatalf("steering delivery=%+v err=%v", promoted, err)
	}
	queued, err = harness.service.ListFollowUps(ctx, harness.project.UUID, thread.UUID)
	if err != nil || len(queued) != 0 {
		t.Fatalf("promoted queue=%+v err=%v", queued, err)
	}
	var followReferenceCount int64
	if err := harness.store.DB().Table("chat_follow_up_file_references AS refs").Joins("JOIN chat_follow_ups follow_ups ON follow_ups.id=refs.follow_up_id").Where("follow_ups.uuid=?", followUp.UUID).Count(&followReferenceCount).Error; err != nil || followReferenceCount != 0 {
		t.Fatalf("promoted references=%d err=%v", followReferenceCount, err)
	}
}

func TestThreadPaginationAndWorkflowDiagnosticsExposeOnlyPublicData(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	for _, title := range []string{"分页一", "分页二", "分页三"} {
		if _, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: title, ProviderUUID: harness.provider.UUID}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := harness.service.ListThreadsPage(ctx, harness.project.UUID, "", 1, 2)
	if err != nil || len(page.Items) != 2 || page.Pagination.Total != 3 || page.Pagination.LastPage != 2 {
		t.Fatalf("thread page=%+v err=%v", page, err)
	}
	second, err := harness.service.ListThreadsPage(ctx, harness.project.UUID, "", 2, 2)
	if err != nil || len(second.Items) != 1 || second.Items[0].UUID == page.Items[0].UUID {
		t.Fatalf("second thread page=%+v err=%v", second, err)
	}

	workflow, err := harness.service.CreateYoloWorkflow(ctx, harness.project.UUID, CreateYoloInput{Title: "诊断", StoryPrompt: "测试", ProviderUUID: harness.provider.UUID, IdempotencyKey: "diagnostics-test"})
	if err != nil {
		t.Fatal(err)
	}
	runs, err := harness.service.ListWorkflowRuns(ctx, harness.project.UUID, workflow.UUID, "", 2)
	if err != nil || len(runs.Items) != 2 || !runs.CursorPagination.HasMore {
		t.Fatalf("workflow runs=%+v err=%v", runs, err)
	}
	events, err := harness.service.ListWorkflowEvents(ctx, harness.project.UUID, workflow.UUID, "", "", 20)
	if err != nil || len(events.Items) == 0 {
		t.Fatalf("workflow events=%+v err=%v", events, err)
	}
	var workflowRow workflowRecord
	if err := harness.store.DB().Where("uuid=?", workflow.UUID).First(&workflowRow).Error; err != nil {
		t.Fatal(err)
	}
	var workflowSteps []workflowStepRecord
	if err := harness.store.DB().Where("workflow_id=?", workflowRow.ID).Order("position").Find(&workflowSteps).Error; err != nil || len(workflowSteps) < 2 {
		t.Fatalf("workflow steps=%+v err=%v", workflowSteps, err)
	}
	if _, err := llmlog.Begin(ctx, harness.store, nil, llmlog.StartInput{
		ProjectID: workflowRow.ProjectID, WorkflowID: workflowRow.ID, WorkflowStepID: workflowSteps[0].ID,
		SourceType: llmlog.SourceWorkflow, Scenario: "diagnostics_filter", RequestType: llmlog.RequestText, Attempt: 1,
		ProviderUUID: harness.provider.UUID, ProviderType: harness.provider.ProviderType, Model: harness.provider.DefaultModel,
		InputSummary: "public diagnostic fixture", RequestPayload: json.RawMessage(`{"messages":[]}`),
	}); err != nil {
		t.Fatal(err)
	}
	logs, err := harness.service.ListWorkflowLLMLogs(ctx, harness.project.UUID, workflow.UUID, "", 1, 20)
	if err != nil || logs.Pagination.Total != 1 || logs.Pagination.LastPage != 1 || len(logs.Items) != 1 || logs.Items[0].WorkflowStepUUID != workflowSteps[0].UUID {
		t.Fatalf("workflow logs=%+v err=%v", logs, err)
	}
	filteredLogs, err := harness.service.ListWorkflowLLMLogs(ctx, harness.project.UUID, workflow.UUID, workflowSteps[0].UUID, 1, 20)
	if err != nil || filteredLogs.Pagination.Total != 1 || len(filteredLogs.Items) != 1 {
		t.Fatalf("filtered workflow logs=%+v err=%v", filteredLogs, err)
	}
	emptyLogs, err := harness.service.ListWorkflowLLMLogs(ctx, harness.project.UUID, workflow.UUID, workflowSteps[1].UUID, 1, 20)
	if err != nil || emptyLogs.Pagination.Total != 0 || len(emptyLogs.Items) != 0 {
		t.Fatalf("empty filtered workflow logs=%+v err=%v", emptyLogs, err)
	}
	if _, err := harness.service.ListWorkflowLLMLogs(ctx, harness.project.UUID, workflow.UUID, "not-a-uuid", 1, 20); errorCode(err) != CodeValidation {
		t.Fatalf("invalid step filter err=%v", err)
	}
	sanitized := string(sanitizeDiagnosticJSON(`{"project_uuid":"` + harness.project.UUID + `","projectUUID":"` + harness.project.UUID + `","badUUID":"not-a-uuid","internal_id":9,"internalId":10,"root_path":"/secret","rootPath":"/secret","api_key":"secret","apiKey":"secret","access_token":"secret","nested":{"password":"bad","ok":true}}`))
	if strings.Contains(sanitized, "badUUID") || strings.Contains(sanitized, "internal_id") || strings.Contains(sanitized, "internalId") || strings.Contains(sanitized, "root_path") || strings.Contains(sanitized, "rootPath") || strings.Contains(sanitized, "api_key") || strings.Contains(sanitized, "apiKey") || strings.Contains(sanitized, "access_token") || strings.Contains(sanitized, "password") || !strings.Contains(sanitized, "project_uuid") || !strings.Contains(sanitized, "projectUUID") || !strings.Contains(sanitized, `"ok":true`) {
		t.Fatalf("sanitized payload=%s", sanitized)
	}
}

func TestChatImageGenRunsSynchronouslyWritesBackAndRecoversIdempotently(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	generatedPNG := agentTestColorPNG(t, color.RGBA{R: 40, G: 80, B: 200, A: 255})
	imageClient := &imageClientFake{response: imagegen.Response{Bytes: generatedPNG, MIMEType: "image/png", RevisedPrompt: "revised moon post office"}}
	harness.service.WithImageClient(imageClient)

	harness.model.mu.Lock()
	harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
		switch call {
		case 1:
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "image-call", Name: "image_gen", Arguments: `{"prompt":"moonlit wooden post office"}`}}}, FinishReason: "tool_calls"}, nil
		case 2:
			fileUUID := toolResultFileUUID(t, request.Messages)
			arguments, _ := json.Marshal(map[string]any{"file_uuid": fileUUID, "asset_type": "scene", "title": "月亮邮局", "summary": "夜蓝木屋与暖黄窗光", "tags": []string{"night"}})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "writeback-call", Name: "create_premise_asset", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		default:
			return finalResponse("图片已经生成并保存为“月亮邮局”设定项。"), nil
		}
	}
	harness.model.mu.Unlock()

	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "同步生成", Scope: ThreadScopePremise, Scene: ScenePremiseAsset, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	referenceUpload := createChatReferenceUpload(t, harness.store, "mood.png", agentTestColorPNG(t, color.RGBA{R: 180, G: 120, A: 255}))
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "生成月亮邮局", UploadUUIDs: []string{referenceUpload.UUID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}

	imageClient.mu.Lock()
	requests := append([]imagegen.Request(nil), imageClient.requests...)
	deadlines := append([]time.Time(nil), imageClient.deadlines...)
	imageClient.mu.Unlock()
	if len(requests) != 1 || len(deadlines) != 1 || time.Until(deadlines[0]) < 9*time.Minute || requests[0].Model != "test/image-model" || requests[0].Size != "1536x1024" || requests[0].Quality != "medium" || len(requests[0].Images) != 1 {
		t.Fatalf("image requests=%+v", requests)
	}
	assets, err := production.NewService(harness.store, nil).ListPremiseAssets(ctx, "", "active")
	if err != nil || len(assets) != 1 || assets[0].Title != "月亮邮局" || assets[0].CurrentVariant == nil {
		t.Fatalf("written premise assets=%+v err=%v", assets, err)
	}
	items, err := harness.service.ListItems(ctx, harness.project.UUID, thread.UUID, "", "", 50)
	if err != nil || items.Items[len(items.Items)-1].Content != "图片已经生成并保存为“月亮邮局”设定项。" {
		t.Fatalf("final chat items=%+v err=%v", items.Items, err)
	}
	for _, item := range items.Items {
		if item.ItemType == "tool_call" && item.Status != "completed" {
			t.Fatalf("tool call did not complete after result persistence: %+v", item)
		}
	}
	var productionRuns int64
	if err := harness.store.DB().Table("production_task_runs").Count(&productionRuns).Error; err != nil || productionRuns != 0 || len(harness.queue.requests) != 0 {
		t.Fatalf("chat image generation entered production queue: runs=%d requests=%d err=%v", productionRuns, len(harness.queue.requests), err)
	}
	var logCount int64
	var requestPayload string
	if err := harness.store.DB().Table("llm_logs").Where("source_type='project_chat' AND request_type='image' AND chat_thread_id IS NOT NULL AND chat_run_id IS NOT NULL").Count(&logCount).Error; err != nil || logCount != 1 {
		t.Fatalf("image logs=%d err=%v", logCount, err)
	}
	if err := harness.store.DB().Table("llm_logs").Select("request_payload").Where("request_type='image'").Take(&requestPayload).Error; err != nil || strings.Contains(requestPayload, "must-not-leak") || strings.Contains(requestPayload, "data:image") {
		t.Fatalf("unsafe image log payload=%q err=%v", requestPayload, err)
	}

	var imageExecution, writebackExecution toolExecutionRecord
	if err := harness.store.DB().Table("agent_tool_executions").Where("tool_name='image_gen'").Take(&imageExecution).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("agent_tool_executions").Where("tool_name='create_premise_asset'").Take(&writebackExecution).Error; err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	replayedImage, err := harness.service.executeTool(ctx, harness.store, tc, imageExecution)
	if err != nil || toolResultFileUUID(t, []llm.ChatMessage{{Role: "tool", Content: string(replayedImage)}}) != assets[0].CurrentVariant.Asset.UUID {
		t.Fatalf("replayed image result=%s err=%v", replayedImage, err)
	}
	replayedAsset, err := harness.service.executeTool(ctx, harness.store, tc, writebackExecution)
	if err != nil || !strings.Contains(string(replayedAsset), assets[0].UUID) {
		t.Fatalf("replayed writeback result=%s err=%v", replayedAsset, err)
	}
	imageClient.mu.Lock()
	callCount := len(imageClient.requests)
	imageClient.mu.Unlock()
	if callCount != 1 {
		t.Fatalf("idempotent recovery regenerated image %d times", callCount)
	}
	assets, err = production.NewService(harness.store, nil).ListPremiseAssets(ctx, "", "active")
	if err != nil || len(assets) != 1 {
		t.Fatalf("idempotent writeback created duplicates=%+v err=%v", assets, err)
	}
}

func TestAssetReferenceImageGenOrdersCurrentAttachmentAndExplicitReferences(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	currentBytes := agentTestColorPNG(t, color.RGBA{R: 200, A: 255})
	attachmentBytes := agentTestColorPNG(t, color.RGBA{G: 200, A: 255})
	explicitBytes := agentTestColorPNG(t, color.RGBA{B: 200, A: 255})
	productionService := production.NewService(harness.store, nil)
	currentUpload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "current.png", Reader: bytes.NewReader(currentBytes)})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := productionService.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: currentUpload.UUID, AssetType: production.AssetCharacter, Title: "月光邮差"})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "引用设定", Scope: ThreadScopePremise, Scene: SceneAssetReference, SubjectUUID: asset.UUID, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	attachmentUpload := createChatReferenceUpload(t, harness.store, "attachment.png", attachmentBytes)
	explicitUpload := createChatReferenceUpload(t, harness.store, "explicit.png", explicitBytes)
	explicitFile, err := productionService.Files().FinalizeUpload(ctx, explicitUpload.UUID, "project_chatbot_reference")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "生成冬季版本", UploadUUIDs: []string{attachmentUpload.UUID}})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	imageClient := &imageClientFake{response: imagegen.Response{Bytes: agentTestPNG(t), MIMEType: "image/png", RevisedPrompt: "winter courier"}}
	harness.service.WithImageClient(imageClient)
	executionUUID, _ := newUUIDv7()
	arguments, _ := json.Marshal(map[string]any{"prompt": "winter uniform", "reference_file_uuids": []string{explicitFile.UUID}})
	result, err := harness.service.executeTool(ctx, harness.store, tc, toolExecutionRecord{UUID: executionUUID, ToolName: "image_gen", ArgumentsJSON: string(arguments)})
	if err != nil || !json.Valid(result) {
		t.Fatalf("asset reference image result=%s err=%v", result, err)
	}
	imageClient.mu.Lock()
	requests := append([]imagegen.Request(nil), imageClient.requests...)
	imageClient.mu.Unlock()
	if len(requests) != 1 || len(requests[0].Images) != 3 || !bytes.Equal(requests[0].Images[0].Data, currentBytes) || !bytes.Equal(requests[0].Images[1].Data, attachmentBytes) || !bytes.Equal(requests[0].Images[2].Data, explicitBytes) {
		t.Fatalf("reference order was not current -> attachment -> explicit: %+v", requests)
	}
}

func TestChatImageGenPropagatesTurnCancellation(t *testing.T) {
	harness := newAgentHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	imageClient := &imageClientFake{}
	imageClient.generate = func(callCtx context.Context, _ imagegen.Request) (imagegen.Response, error) {
		cancel()
		<-callCtx.Done()
		return imagegen.Response{}, callCtx.Err()
	}
	harness.service.WithImageClient(imageClient)
	thread, err := harness.service.CreateThread(context.Background(), harness.project.UUID, CreateThreadInput{Title: "取消生图", Scope: ThreadScopePremise, Scene: ScenePremiseAsset, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "生成后立即取消"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(context.Background(), harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	executionUUID, _ := newUUIDv7()
	result, err := harness.service.executeTool(ctx, harness.store, tc, toolExecutionRecord{UUID: executionUUID, ToolName: "image_gen", ArgumentsJSON: `{"prompt":"cancel this image"}`})
	if !errors.Is(err, context.Canceled) || result != nil {
		t.Fatalf("cancelled image result=%s err=%v", result, err)
	}
	var generatedFiles int64
	if err := harness.store.DB().Table("files").Where("purpose='project_chat_asset_image_generation'").Count(&generatedFiles).Error; err != nil || generatedFiles != 0 {
		t.Fatalf("cancelled image persisted files=%d err=%v", generatedFiles, err)
	}
}

func (harness *agentHarness) withStore(t *testing.T, callback func(*project.Store)) {
	t.Helper()
	err := harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error {
		callback(store)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (harness *agentHarness) execute(t *testing.T, threadUUID, turnUUID, kind string) error {
	t.Helper()
	return harness.service.ExecuteJob(context.Background(), harness.store, JobSpec{Version: 1, ProjectUUID: harness.project.UUID, JobKind: kind, ResourceUUID: turnUUID, ThreadUUID: threadUUID})
}

func finalResponse(content string) llm.ChatResponse {
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: content}, FinishReason: "stop"}
}

func TestThreadFIFOFollowUpOrderAndAbortPersist(t *testing.T) {
	harness := newAgentHarness(t, finalResponse("first"), finalResponse("second"), finalResponse("third"))
	thread := harness.createThread(t)
	first, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "第一条"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "第二条"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, second.UUID, JobChatTurn); !errors.Is(err, ErrJobNotReady) {
		t.Fatalf("second turn bypassed FIFO: %v", err)
	}
	if err := harness.execute(t, thread.UUID, first.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	harness.model.mu.Lock()
	firstRequest := harness.model.requests[0]
	harness.model.mu.Unlock()
	for _, message := range firstRequest.Messages {
		if message.Content == "第二条" {
			t.Fatal("queued second turn leaked into the first run context")
		}
	}
	if err := harness.execute(t, thread.UUID, second.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}

	trigger, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "触发跟进"})
	if err != nil {
		t.Fatal(err)
	}
	one, err := harness.service.CreateFollowUp(context.Background(), harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "跟进一"})
	if err != nil {
		t.Fatal(err)
	}
	two, _ := harness.service.CreateFollowUp(context.Background(), harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "跟进二"})
	three, _ := harness.service.CreateFollowUp(context.Background(), harness.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "跟进三"})
	moved, err := harness.service.MoveFollowUp(context.Background(), harness.project.UUID, thread.UUID, three.UUID, 1)
	if err != nil || len(moved) != 3 || moved[0].UUID != three.UUID || moved[1].UUID != one.UUID || moved[2].UUID != two.UUID {
		t.Fatalf("moved follow-ups = %+v, error = %v", moved, err)
	}
	if err := harness.service.DeleteFollowUp(context.Background(), harness.project.UUID, thread.UUID, two.UUID); err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, trigger.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	turns, err := harness.service.ListTurns(context.Background(), harness.project.UUID, thread.UUID)
	if err != nil || len(turns) != 4 || turns[3].SourceFollowUpUUID != three.UUID || turns[3].InputText != "跟进三" {
		t.Fatalf("turns after follow-up promotion = %+v, error = %v", turns, err)
	}
	remaining, err := harness.service.ListFollowUps(context.Background(), harness.project.UUID, thread.UUID)
	if err != nil || len(remaining) != 1 || remaining[0].UUID != one.UUID || remaining[0].Position != 1 {
		t.Fatalf("remaining follow-ups = %+v, error = %v", remaining, err)
	}
	aborted, err := harness.service.Abort(context.Background(), harness.project.UUID, thread.UUID)
	if err != nil || aborted.UUID != turns[3].UUID || aborted.Status != TurnCancelled {
		t.Fatalf("aborted = %+v, error = %v", aborted, err)
	}
	encoded, _ := json.Marshal(struct {
		Turns []Turn     `json:"turns"`
		Queue []FollowUp `json:"queue"`
	}{turns, remaining})
	if string(encoded) == "" || containsInternalID(encoded) {
		t.Fatalf("public response leaked internal id: %s", encoded)
	}
}

func containsInternalID(encoded []byte) bool {
	var value any
	if json.Unmarshal(encoded, &value) != nil {
		return true
	}
	var inspect func(any) bool
	inspect = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if key == "id" || inspect(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if inspect(child) {
					return true
				}
			}
		}
		return false
	}
	return inspect(value)
}

func TestToolIntentIsIdempotentAndResultIsCompact(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "读取设定"})
	if err != nil {
		t.Fatal(err)
	}
	harness.withStore(t, func(store *project.Store) {
		tc, err := harness.service.loadToolContext(context.Background(), store, thread.UUID, turn.UUID)
		if err != nil {
			t.Fatal(err)
		}
		if err := harness.service.claimRun(context.Background(), store, &tc); err != nil {
			t.Fatal(err)
		}
		first, _, completed, err := harness.service.persistToolIntent(context.Background(), store, tc, "provider-call-1", "get_story_profile", `{}`)
		if err != nil || completed {
			t.Fatalf("first intent = %+v, completed=%v, error=%v", first, completed, err)
		}
		result, err := harness.service.executeTool(context.Background(), store, tc, first)
		if err != nil || len(result) > MaxToolResult || !json.Valid(result) {
			t.Fatalf("tool result bytes=%d valid=%v error=%v", len(result), json.Valid(result), err)
		}
		if err := harness.service.persistToolResult(context.Background(), store, tc, first, result); err != nil {
			t.Fatal(err)
		}
		second, replay, completed, err := harness.service.persistToolIntent(context.Background(), store, tc, "provider-call-1", "get_story_profile", `{}`)
		if err != nil || !completed || second.ID != first.ID || string(replay) != string(result) {
			t.Fatalf("idempotent replay = %+v completed=%v result=%s error=%v", second, completed, replay, err)
		}
		var count int64
		if err := store.DB().Table("agent_tool_executions").Where("run_id=?", tc.Run.ID).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("tool execution count=%d error=%v", count, err)
		}
	})
}

func TestToolArgumentsEnforceSchemaAndUUIDArrays(t *testing.T) {
	resourceUUID, _ := newUUIDv7()
	referenceUUID, _ := newUUIDv7()
	valid := `{"kind":"comic_image_generation","resource_uuid":"` + resourceUUID + `","prompt":"paint","premise_asset_uuids":["` + referenceUUID + `"]}`
	if _, err := validateToolArguments("start_generation", valid); err != nil {
		t.Fatalf("valid UUID array rejected: %v", err)
	}
	for name, raw := range map[string]string{
		"unknown field":  `{"unexpected":"value"}`,
		"internal id":    `{"chapter_id":1}`,
		"missing field":  `{"kind":"comic_image_generation"}`,
		"provider field": `{"kind":"comic_image_generation","resource_uuid":"` + resourceUUID + `","provider_uuid":"` + referenceUUID + `","prompt":"paint"}`,
		"bad option":     `{"input_type":"single_choice","question":"q","options":[{"label":"A","sql":"no"},{"label":"B"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			toolName := "get_story_profile"
			if name == "missing field" || name == "provider field" {
				toolName = "start_generation"
			}
			if name == "bad option" {
				toolName = "request_user_input"
			}
			if _, err := validateToolArguments(toolName, raw); err == nil {
				t.Fatalf("invalid arguments accepted: %s", raw)
			}
		})
	}
}

func TestEveryAllowlistedProjectToolReturnsScopedPublicShape(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "验证工具"})
	if err != nil {
		t.Fatal(err)
	}
	storyService := story.NewService(harness.store)
	chapter, err := storyService.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "第一章", Content: "旧正文", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	productionService := production.NewService(harness.store, nil)
	section, err := productionService.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Section 1", StoryboardMD: "旧分镜"})
	if err != nil {
		t.Fatal(err)
	}
	upload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "fixture.png", Reader: bytes.NewReader(agentTestPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := storyService.GetStoryProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var tc toolContext
	harness.withStore(t, func(store *project.Store) {
		tc, err = harness.service.loadToolContext(ctx, store, thread.UUID, turn.UUID)
		if err == nil {
			err = harness.service.claimRun(ctx, store, &tc)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	executed := make(map[string]bool)
	execute := func(name string, args map[string]any) map[string]any {
		t.Helper()
		encoded, marshalErr := json.Marshal(args)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		execution := toolExecutionRecord{ToolName: name, TargetUUID: targetUUIDForTool(name, args), ArgumentsJSON: string(encoded), IdempotencyKey: "shape-test:" + name}
		result, executeErr := harness.service.executeTool(ctx, harness.store, tc, execution)
		if executeErr != nil || len(result) > MaxToolResult || !json.Valid(result) || containsInternalID(result) {
			t.Fatalf("%s result=%s valid=%v bytes=%d error=%v", name, result, json.Valid(result), len(result), executeErr)
		}
		lower := strings.ToLower(string(result))
		for _, forbidden := range []string{"must-not-leak", `"workspace"`, `"root_path"`, `"relative_path"`, `"api_key"`} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s leaked forbidden field %s: %s", name, forbidden, result)
			}
		}
		var envelope map[string]any
		if unmarshalErr := json.Unmarshal(result, &envelope); unmarshalErr != nil || envelope["success"] != true || envelope["data"] == nil {
			t.Fatalf("%s did not return a successful compact envelope: %s", name, result)
		}
		executed[name] = true
		return envelope
	}

	execute("get_story_profile", map[string]any{})
	execute("update_story_profile", map[string]any{"story_md": "# 新故事设定", "expected_revision": profile.Revision})
	chapters := execute("list_chapters", map[string]any{})
	if items, ok := chapters["data"].(map[string]any)["items"].([]any); !ok || len(items) != 1 {
		t.Fatalf("list_chapters shape=%+v", chapters)
	}
	execute("get_chapter", map[string]any{"chapter_uuid": chapter.UUID})
	execute("update_chapter_story", map[string]any{"chapter_uuid": chapter.UUID, "content": "新正文", "content_format": "md", "expected_revision": chapter.Revision})
	execute("get_premise", map[string]any{})
	created := execute("create_premise_asset", map[string]any{"upload_uuid": upload.UUID, "asset_type": "character", "title": "小狐狸", "summary": "月光邮差", "tags": []string{"fox"}})
	assetData, ok := created["data"].(map[string]any)
	assetUUID, uuidOK := assetData["uuid"].(string)
	assetRevision, revisionOK := assetData["revision"].(float64)
	if !ok || !uuidOK || !revisionOK || !isUUIDv7(assetUUID) {
		t.Fatalf("create_premise_asset shape=%+v", created)
	}
	// Simulate a crash after the domain commit and before the Agent result
	// commit: consuming the same upload must resolve to the existing asset.
	replayed := execute("create_premise_asset", map[string]any{"upload_uuid": upload.UUID, "asset_type": "character", "title": "小狐狸", "summary": "月光邮差", "tags": []string{"fox"}})
	if replayed["data"].(map[string]any)["uuid"] != assetUUID {
		t.Fatalf("create_premise_asset replay created a second resource: %+v", replayed)
	}
	execute("list_premise_assets", map[string]any{})
	execute("get_premise_asset", map[string]any{"premise_asset_uuid": assetUUID})
	execute("update_premise_asset", map[string]any{"premise_asset_uuid": assetUUID, "expected_revision": int64(assetRevision), "summary": "可靠的月光邮差"})
	execute("get_comic_section", map[string]any{"chapter_uuid": chapter.UUID, "section_uuid": section.UUID})
	execute("update_comic_storyboard", map[string]any{"chapter_uuid": chapter.UUID, "section_uuid": section.UUID, "content_md": "新分镜", "expected_revision": section.Revision})
	execute("start_generation", map[string]any{"kind": "story_chapter_generation", "resource_uuid": chapter.UUID, "chapter_uuid": chapter.UUID, "prompt": "继续写作"})

	for _, definition := range toolDefinitions() {
		name, _ := definition["name"].(string)
		if name == "request_user_input" || name == "image_gen" || name == "generate_premise_asset" || name == "generate_premise_asset_variant" || name == currentProjectAPIToolName { // Scene tools have dedicated scope tests.
			continue
		}
		if !executed[name] {
			t.Fatalf("allowlisted tool %s lacks a response-shape assertion", name)
		}
	}
}

func agentTestPNG(t *testing.T) []byte {
	t.Helper()
	return agentTestColorPNG(t, color.RGBA{R: 120, G: 80, B: 40, A: 255})
}

func agentTestColorPNG(t *testing.T, fill color.RGBA) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			value.Set(x, y, fill)
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, value); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func createChatReferenceUpload(t *testing.T, store *project.Store, filename string, content []byte) files.Upload {
	t.Helper()
	upload, err := files.NewService(store, nil).CreateUpload(context.Background(), files.CreateUploadInput{Purpose: "project_chatbot_reference", OriginalFilename: filename, Reader: bytes.NewReader(content)})
	if err != nil {
		t.Fatal(err)
	}
	return upload
}

func createComicSectionFixture(t *testing.T, store *project.Store) string {
	t.Helper()
	storyService := story.NewService(store)
	chapter, err := storyService.CreateChapter(context.Background(), story.CreateChapterInput{ChapterCode: "vol99.ch99", Title: "附件场景", Content: "测试", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	section, err := production.NewService(store, nil).CreateSection(context.Background(), chapter.UUID, production.CreateSectionInput{Title: "附件分镜", StoryboardMD: "测试分镜"})
	if err != nil {
		t.Fatal(err)
	}
	return section.UUID
}

func toolResultFileUUID(t *testing.T, messages []llm.ChatMessage) string {
	t.Helper()
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role != "tool" {
			continue
		}
		var envelope struct {
			Success bool `json:"success"`
			Data    struct {
				FileUUID string `json:"file_uuid"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(messages[index].Content), &envelope); err == nil && envelope.Success && isUUIDv7(envelope.Data.FileUUID) {
			return envelope.Data.FileUUID
		}
	}
	t.Fatalf("tool messages do not contain a generated file_uuid: %+v", messages)
	return ""
}

func TestContextCompactionKeepsOriginalAuditItems(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "保留这条原始消息"})
	if err != nil {
		t.Fatal(err)
	}
	harness.withStore(t, func(store *project.Store) {
		tc, err := harness.service.loadToolContext(context.Background(), store, thread.UUID, turn.UUID)
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := store.DB().DB()
		if err != nil {
			t.Fatal(err)
		}
		tx, err := sqlDB.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		threadRow, err := lockThreadSQL(context.Background(), tx, tc.Thread.ProjectID, thread.UUID)
		if err != nil {
			t.Fatal(err)
		}
		for index := 0; index < 30; index++ {
			content := strings.Repeat(string(rune('a'+index%20)), 20_000)
			if _, err := appendItemTx(context.Background(), tx, &threadRow, &tc.Turn.ID, &tc.Run.ID, "assistant_message", "assistant", content, "text", "completed", "", "", "", map[string]any{}, harness.service.now().UTC()); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := tx.Exec(`UPDATE chat_threads SET next_item_sequence=? WHERE id=?`, threadRow.NextItemSequence, threadRow.ID); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		messages, contextBytes, _, err := harness.service.buildContext(context.Background(), store, tc)
		if err != nil || contextBytes > MaxContextBytes || len(messages) != 22 || !strings.Contains(messages[1].Content, "既有对话派生的摘要") {
			t.Fatalf("compacted context messages=%d bytes=%d error=%v", len(messages), contextBytes, err)
		}
		var itemCount, summaryCount int64
		_ = store.DB().Table("chat_items").Where("thread_id=?", tc.Thread.ID).Count(&itemCount).Error
		_ = store.DB().Table("agent_context_summaries").Where("thread_id=?", tc.Thread.ID).Count(&summaryCount).Error
		if itemCount != 31 || summaryCount != 1 {
			t.Fatalf("audit retention items=%d summaries=%d", itemCount, summaryCount)
		}
	})
}

func TestContextUsesProjectGenerationLanguage(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	harness.withStore(t, func(store *project.Store) {
		storyService := story.NewService(store)
		detail, err := storyService.GetProject(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		language := project.GenerationLanguageEnglish
		if _, err := storyService.UpdateProject(context.Background(), story.UpdateProjectInput{Name: detail.Name, Description: detail.Description, GenerationLanguage: &language, ExpectedRevision: detail.Revision}); err != nil {
			t.Fatal(err)
		}
	})
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "Write the next scene"})
	if err != nil {
		t.Fatal(err)
	}
	harness.withStore(t, func(store *project.Store) {
		tc, err := harness.service.loadToolContext(context.Background(), store, thread.UUID, turn.UUID)
		if err != nil {
			t.Fatal(err)
		}
		messages, _, _, err := harness.service.buildContext(context.Background(), store, tc)
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) == 0 || !strings.Contains(messages[0].Content, "Project language: English") {
			t.Fatalf("system prompt does not contain the project generation language: %+v", messages)
		}
	})
}

func TestAgentTurnFreezesEffectivePromptsWhenQueued(t *testing.T) {
	harness := newAgentHarness(t)
	thread := harness.createThread(t)
	harness.withStore(t, func(store *project.Store) {
		storyService := story.NewService(store)
		if _, err := storyService.UpdatePromptGroup(context.Background(), story.UpdatePromptGroupInput{
			PromptGroup:             "agent",
			Prompts:                 map[string]string{"project_assistant": "FROZEN AGENT ASSISTANT"},
			ExpectedCurrentVersions: map[string]int{"project_assistant": 1},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := storyService.UpdatePromptGroup(context.Background(), story.UpdatePromptGroupInput{
			PromptGroup:             "runtime",
			Prompts:                 map[string]string{"project_language_instruction": "FROZEN AGENT LANGUAGE"},
			ExpectedCurrentVersions: map[string]int{"project_language_instruction": 1},
		}); err != nil {
			t.Fatal(err)
		}
	})
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "Keep the queued prompt snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	harness.withStore(t, func(store *project.Store) {
		storyService := story.NewService(store)
		if _, err := storyService.UpdatePromptGroup(context.Background(), story.UpdatePromptGroupInput{
			PromptGroup:             "agent",
			Prompts:                 map[string]string{"project_assistant": "NEWER AGENT ASSISTANT"},
			ExpectedCurrentVersions: map[string]int{"project_assistant": 2},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := storyService.UpdatePromptGroup(context.Background(), story.UpdatePromptGroupInput{
			PromptGroup:             "runtime",
			Prompts:                 map[string]string{"project_language_instruction": "NEWER AGENT LANGUAGE"},
			ExpectedCurrentVersions: map[string]int{"project_language_instruction": 2},
		}); err != nil {
			t.Fatal(err)
		}
		tc, err := harness.service.loadToolContext(context.Background(), store, thread.UUID, turn.UUID)
		if err != nil {
			t.Fatal(err)
		}
		messages, _, _, err := harness.service.buildContext(context.Background(), store, tc)
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) == 0 || !strings.Contains(messages[0].Content, "FROZEN AGENT ASSISTANT") || !strings.Contains(messages[0].Content, "FROZEN AGENT LANGUAGE") || strings.Contains(messages[0].Content, "NEWER AGENT") {
			t.Fatalf("agent prompt snapshot was not frozen: %+v", messages)
		}
	})
	items, err := harness.service.ListItems(context.Background(), harness.project.UUID, thread.UUID, "", "", 20)
	if err != nil || len(items.Items) == 0 || strings.Contains(string(items.Items[0].Metadata), "prompt_snapshot") {
		t.Fatalf("public item metadata leaked prompt snapshot: items=%+v error=%v", items.Items, err)
	}
}

func TestRequestUserInputPausesAndResumesSameRun(t *testing.T) {
	requestCall := llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "input-call-1", Name: "request_user_input", Arguments: `{"input_type":"single_choice","question":"选择风格","options":[{"label":"温暖"},{"label":"冒险"}]}`}}}, FinishReason: "tool_calls"}
	harness := newAgentHarness(t, requestCall, finalResponse("已采用你的选择"))
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "帮我选风格"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); !errors.Is(err, ErrWaitingInput) {
		t.Fatalf("run did not pause for input: %v", err)
	}
	requests, err := harness.service.ListUserInputRequests(context.Background(), harness.project.UUID, thread.UUID)
	if err != nil || len(requests) != 1 || requests[0].Status != "pending" || len(requests[0].Options) != 2 {
		t.Fatalf("input requests = %+v, error=%v", requests, err)
	}
	answered, err := harness.service.RespondUserInput(context.Background(), harness.project.UUID, thread.UUID, requests[0].UUID, UserInputResponse{SelectedOptionUUIDs: []string{requests[0].Options[0].UUID}})
	if err != nil || answered.Status != "resuming" || answered.RunUUID != requests[0].RunUUID {
		t.Fatalf("answered request = %+v, error=%v", answered, err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatResume); err != nil {
		t.Fatal(err)
	}
	harness.model.mu.Lock()
	requestsSent := append([]llm.ChatRequest(nil), harness.model.requests...)
	harness.model.mu.Unlock()
	if len(requestsSent) != 2 || len(requestsSent[0].Messages) < 2 || requestsSent[0].Messages[1].Content != "帮我选风格" {
		t.Fatalf("model did not receive persisted user context: %+v", requestsSent)
	}
	var toolCallID, toolResultID string
	for _, message := range requestsSent[1].Messages {
		if len(message.ToolCalls) > 0 {
			toolCallID = message.ToolCalls[0].ID
		}
		if message.Role == "tool" {
			toolResultID = message.ToolCallID
		}
	}
	if toolCallID != "input-call-1" || toolResultID != toolCallID {
		t.Fatalf("tool context IDs do not match provider call: call=%q result=%q", toolCallID, toolResultID)
	}
	turns, err := harness.service.ListTurns(context.Background(), harness.project.UUID, thread.UUID)
	if err != nil || len(turns) != 1 || turns[0].Status != TurnCompleted {
		t.Fatalf("resumed turns = %+v, error=%v", turns, err)
	}
	requests, err = harness.service.ListUserInputRequests(context.Background(), harness.project.UUID, thread.UUID)
	if err != nil || len(requests) != 1 || requests[0].Status != "resumed" || requests[0].ResumedAt == nil {
		t.Fatalf("resumed input request = %+v, error=%v", requests, err)
	}
	items, err := harness.service.ListItems(context.Background(), harness.project.UUID, thread.UUID, "", "", 100)
	if err != nil || len(items.Items) != 5 || items.Items[len(items.Items)-1].Content != "已采用你的选择" {
		t.Fatalf("resumed items = %+v, error=%v", items.Items, err)
	}
}

func TestSteeringDuringModelCallRecomputesBeforeCompletion(t *testing.T) {
	harness := newAgentHarness(t, finalResponse("这条结果应被丢弃"), finalResponse("已按 Steering 调整"))
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "先写一个版本"})
	if err != nil {
		t.Fatal(err)
	}
	harness.model.mu.Lock()
	harness.model.onCall = func(call int) {
		if call != 1 {
			return
		}
		if _, err := harness.service.Steer(context.Background(), harness.project.UUID, thread.UUID, SteeringInput{InputText: "改成更温暖的结局"}); err != nil {
			t.Errorf("steer during model call: %v", err)
		}
	}
	harness.model.mu.Unlock()
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	harness.model.mu.Lock()
	requestsSent := append([]llm.ChatRequest(nil), harness.model.requests...)
	harness.model.mu.Unlock()
	if len(requestsSent) != 2 {
		t.Fatalf("model calls = %d, want 2", len(requestsSent))
	}
	foundSteering := false
	for _, message := range requestsSent[1].Messages {
		foundSteering = foundSteering || message.Role == "user" && message.Content == "改成更温暖的结局"
	}
	if !foundSteering {
		t.Fatal("second safe-boundary context omitted Steering")
	}
	page, err := harness.service.ListItems(context.Background(), harness.project.UUID, thread.UUID, "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.Content == "这条结果应被丢弃" {
			t.Fatal("stale pre-Steering assistant response was persisted")
		}
	}
	if page.Items[len(page.Items)-1].Content != "已按 Steering 调整" {
		t.Fatalf("final item = %+v", page.Items[len(page.Items)-1])
	}
}

func TestYoloWorkflowIsIdempotentRecoverableAndCancellable(t *testing.T) {
	harness := newAgentHarness(t)
	input := CreateYoloInput{Title: "月光邮差", StoryPrompt: "一只小狐狸替月亮送信。", ProviderUUID: harness.provider.UUID, IdempotencyKey: "yolo-test-one"}
	workflow, err := harness.service.CreateYoloWorkflow(context.Background(), harness.project.UUID, input)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := harness.service.CreateYoloWorkflow(context.Background(), harness.project.UUID, input)
	if err != nil || replayed.UUID != workflow.UUID || len(workflow.Steps) != len(YoloStepKeys) {
		t.Fatalf("replayed workflow = %+v, error=%v", replayed, err)
	}
	if err := harness.execute(t, workflow.ThreadUUID, workflow.Steps[0].UUID, JobWorkflowStep); err != nil {
		t.Fatal(err)
	}
	workflow, err = harness.service.GetWorkflow(context.Background(), harness.project.UUID, workflow.UUID)
	if err != nil || workflow.Steps[0].Status != "completed" || workflow.Steps[1].Status != "queued" {
		t.Fatalf("workflow after first step = %+v, error=%v", workflow, err)
	}
	cancelled, err := harness.service.CancelWorkflow(context.Background(), harness.project.UUID, workflow.UUID)
	if err != nil || cancelled.Status != WorkflowCancelled || cancelled.Steps[1].Status != "cancelled" {
		t.Fatalf("cancelled workflow = %+v, error=%v", cancelled, err)
	}
	retried, err := harness.service.RetryWorkflow(context.Background(), harness.project.UUID, workflow.UUID)
	if err != nil || retried.Status != WorkflowQueued || retried.Steps[0].Status != "completed" || retried.Steps[1].Status != "queued" {
		t.Fatalf("retried workflow = %+v, error=%v", retried, err)
	}
	for _, step := range retried.Steps[2:] {
		if step.Status != "pending" {
			t.Fatalf("cancelled downstream step was not reset to pending: %+v", step)
		}
	}
}

func TestYoloWorkflowFreezesEffectivePromptSet(t *testing.T) {
	harness := newAgentHarness(t)
	harness.withStore(t, func(store *project.Store) {
		storyService := story.NewService(store)
		for _, update := range []story.UpdatePromptGroupInput{
			{PromptGroup: "story", Prompts: map[string]string{"story_profile": "FROZEN YOLO STORY"}, ExpectedCurrentVersions: map[string]int{"story_profile": 1}},
			{PromptGroup: "premise_style", Prompts: map[string]string{"project_overall_style": "FROZEN YOLO STYLE"}, ExpectedCurrentVersions: map[string]int{"project_overall_style": 1}},
			{PromptGroup: "runtime", Prompts: map[string]string{"project_language_instruction": "FROZEN YOLO LANGUAGE"}, ExpectedCurrentVersions: map[string]int{"project_language_instruction": 1}},
		} {
			if _, err := storyService.UpdatePromptGroup(context.Background(), update); err != nil {
				t.Fatal(err)
			}
		}
	})
	workflow, err := harness.service.CreateYoloWorkflow(context.Background(), harness.project.UUID, CreateYoloInput{Title: "冻结提示词", StoryPrompt: "测试 Yolo 提示词快照。", ProviderUUID: harness.provider.UUID, IdempotencyKey: "yolo-prompt-freeze"})
	if err != nil {
		t.Fatal(err)
	}
	harness.withStore(t, func(store *project.Store) {
		storyService := story.NewService(store)
		for _, update := range []story.UpdatePromptGroupInput{
			{PromptGroup: "story", Prompts: map[string]string{"story_profile": "NEWER YOLO STORY"}, ExpectedCurrentVersions: map[string]int{"story_profile": 2}},
			{PromptGroup: "premise_style", Prompts: map[string]string{"project_overall_style": "NEWER YOLO STYLE"}, ExpectedCurrentVersions: map[string]int{"project_overall_style": 2}},
			{PromptGroup: "runtime", Prompts: map[string]string{"project_language_instruction": "NEWER YOLO LANGUAGE"}, ExpectedCurrentVersions: map[string]int{"project_language_instruction": 2}},
		} {
			if _, err := storyService.UpdatePromptGroup(context.Background(), update); err != nil {
				t.Fatal(err)
			}
		}
	})
	var snapshot yoloSnapshot
	if err := json.Unmarshal(workflow.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	for identity, expected := range map[string]string{
		"story/story_profile":                  "FROZEN YOLO STORY",
		"premise_style/project_overall_style":  "FROZEN YOLO STYLE",
		"runtime/project_language_instruction": "FROZEN YOLO LANGUAGE",
	} {
		if snapshot.Prompts[identity] != expected {
			t.Fatalf("Yolo prompt %s=%q, want %q", identity, snapshot.Prompts[identity], expected)
		}
	}
}

func TestYoloWorkflowFreezesTextImageAndSelectionModelSourcesAcrossRetry(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	resolver := modelsettings.NewResolver(harness.providers)
	settings, err := resolver.Patch(ctx, harness.store, modelsettings.PatchInput{ExpectedRevision: 0, Changes: map[string]*modelsettings.Selection{
		modelsettings.StoryText:               {ProviderUUID: harness.provider.UUID, Model: harness.provider.DefaultModel},
		modelsettings.ProjectImage:            {ProviderUUID: harness.provider.UUID, Model: harness.provider.DefaultImageModel},
		modelsettings.SectionPremiseSelection: {ProviderUUID: harness.provider.UUID, Model: harness.provider.DefaultModel},
	}})
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := harness.service.CreateYoloWorkflow(ctx, harness.project.UUID, CreateYoloInput{Title: "冻结模型", StoryPrompt: "验证三类模型快照。", IdempotencyKey: "yolo-model-freeze"})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot yoloSnapshot
	if err := json.Unmarshal(workflow.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if workflow.ModelSource != modelsettings.SourceScenarioOverride || snapshot.ModelSource != modelsettings.SourceScenarioOverride || snapshot.ImageModelSource != modelsettings.SourceProjectImageOverride || snapshot.SelectionModelSource != modelsettings.SourceScenarioOverride {
		t.Fatalf("Yolo model snapshot workflow=%+v snapshot=%+v", workflow, snapshot)
	}
	if _, err := resolver.Patch(ctx, harness.store, modelsettings.PatchInput{ExpectedRevision: settings.Revision, Changes: map[string]*modelsettings.Selection{
		modelsettings.StoryText: nil, modelsettings.ProjectImage: nil, modelsettings.SectionPremiseSelection: nil,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.CancelWorkflow(ctx, harness.project.UUID, workflow.UUID); err != nil {
		t.Fatal(err)
	}
	retried, err := harness.service.RetryWorkflow(ctx, harness.project.UUID, workflow.UUID)
	if err != nil {
		t.Fatal(err)
	}
	var retrySnapshot yoloSnapshot
	if err := json.Unmarshal(retried.InputSnapshot, &retrySnapshot); err != nil {
		t.Fatal(err)
	}
	if retrySnapshot.ModelSource != snapshot.ModelSource || retrySnapshot.ImageModelSource != snapshot.ImageModelSource || retrySnapshot.SelectionModelSource != snapshot.SelectionModelSource || retrySnapshot.ProviderUUID != snapshot.ProviderUUID || retrySnapshot.ImageProviderUUID != snapshot.ImageProviderUUID || retrySnapshot.SelectionProviderUUID != snapshot.SelectionProviderUUID {
		t.Fatalf("Yolo retry drifted before=%+v after=%+v", snapshot, retrySnapshot)
	}
}

func TestYoloEveryStepRetriesAndReconcilesAtSafeBoundary(t *testing.T) {
	for position, stepKey := range YoloStepKeys {
		position, stepKey := position+1, stepKey
		t.Run(stepKey, func(t *testing.T) {
			harness := newAgentHarness(t)
			workflow, err := harness.service.CreateYoloWorkflow(context.Background(), harness.project.UUID, CreateYoloInput{Title: "边界恢复", StoryPrompt: "测试每一步失败和退出。", ProviderUUID: harness.provider.UUID, IdempotencyKey: "boundary-" + stepKey})
			if err != nil {
				t.Fatal(err)
			}
			harness.withStore(t, func(store *project.Store) {
				if err := store.DB().Exec(`UPDATE workflow_steps SET status=CASE WHEN position<? THEN 'completed' WHEN position=? THEN 'failed' ELSE 'pending' END,error_code=CASE WHEN position=? THEN 'injected_failure' ELSE '' END WHERE workflow_id=(SELECT id FROM workflows WHERE uuid=?)`, position, position, position, workflow.UUID).Error; err != nil {
					t.Fatal(err)
				}
				if err := store.DB().Exec(`UPDATE workflows SET status='failed',current_step_key=?,error_code='injected_failure' WHERE uuid=?`, stepKey, workflow.UUID).Error; err != nil {
					t.Fatal(err)
				}
			})
			retried, err := harness.service.RetryWorkflow(context.Background(), harness.project.UUID, workflow.UUID)
			if err != nil || retried.Status != WorkflowQueued || retried.Steps[position-1].Status != "queued" {
				t.Fatalf("retry at %s = %+v, error=%v", stepKey, retried, err)
			}
			for index := 0; index < position-1; index++ {
				if retried.Steps[index].Status != "completed" {
					t.Fatalf("retry rewound completed step %s", retried.Steps[index].StepKey)
				}
			}
			harness.withStore(t, func(store *project.Store) {
				if err := store.DB().Exec(`UPDATE workflows SET status='running',error_code='' WHERE uuid=?`, workflow.UUID).Error; err != nil {
					t.Fatal(err)
				}
				if err := store.DB().Exec(`UPDATE workflow_steps SET status='running',error_code='' WHERE uuid=?`, retried.Steps[position-1].UUID).Error; err != nil {
					t.Fatal(err)
				}
				harness.queue.mu.Lock()
				harness.queue.jobs = nil
				harness.queue.mu.Unlock()
				if err := harness.service.ReconcileOnOpen(context.Background(), store); err != nil {
					t.Fatal(err)
				}
			})
			recovered, err := harness.service.GetWorkflow(context.Background(), harness.project.UUID, workflow.UUID)
			if err != nil || recovered.Status != WorkflowQueued || recovered.Steps[position-1].Status != "queued" || recovered.ErrorCode != CodeInterrupted {
				t.Fatalf("recovered at %s = %+v, error=%v", stepKey, recovered, err)
			}
			harness.queue.mu.Lock()
			jobs := append([]JobSpec(nil), harness.queue.jobs...)
			harness.queue.mu.Unlock()
			if len(jobs) != 1 || jobs[0].ResourceUUID != recovered.Steps[position-1].UUID || jobs[0].JobKind != JobWorkflowStep {
				t.Fatalf("reconcile jobs at %s = %+v", stepKey, jobs)
			}
		})
	}
}

func TestYoloRetryRestartsFailedProductTask(t *testing.T) {
	harness := newAgentHarness(t)
	workflow, err := harness.service.CreateYoloWorkflow(context.Background(), harness.project.UUID, CreateYoloInput{Title: "任务重试", StoryPrompt: "产品任务失败后继续。", ProviderUUID: harness.provider.UUID, IdempotencyKey: "retry-product-task"})
	if err != nil {
		t.Fatal(err)
	}
	taskUUID, _ := newUUIDv7()
	harness.queue.mu.Lock()
	harness.queue.tasks = map[string]DomainTask{taskUUID: {UUID: taskUUID, Kind: "story_chapter_generation", Status: "failed"}}
	harness.queue.mu.Unlock()
	harness.withStore(t, func(store *project.Store) {
		if err := store.DB().Exec(`UPDATE workflow_steps SET status='completed' WHERE uuid=?`, workflow.Steps[0].UUID).Error; err != nil {
			t.Fatal(err)
		}
		if err := store.DB().Exec(`UPDATE workflow_steps SET status='failed',task_uuid=?,error_code='provider_failed' WHERE uuid=?`, taskUUID, workflow.Steps[1].UUID).Error; err != nil {
			t.Fatal(err)
		}
		if err := store.DB().Exec(`UPDATE workflows SET status='failed',current_step_key='story',error_code='provider_failed' WHERE uuid=?`, workflow.UUID).Error; err != nil {
			t.Fatal(err)
		}
	})
	retried, err := harness.service.RetryWorkflow(context.Background(), harness.project.UUID, workflow.UUID)
	if err != nil || retried.Steps[1].Status != "queued" {
		t.Fatalf("workflow retry = %+v, error=%v", retried, err)
	}
	harness.queue.mu.Lock()
	retries := append([]string(nil), harness.queue.retries...)
	harness.queue.mu.Unlock()
	if len(retries) != 1 || retries[0] != taskUUID {
		t.Fatalf("domain task retries = %+v", retries)
	}
}

func TestRealtimePayloadDropsInternalAndInvalidIdentifiers(t *testing.T) {
	projectUUID, _ := newUUIDv7()
	threadUUID, _ := newUUIDv7()
	payload := publicRealtimePayload(map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "target_uuid": "", "internal_id": int64(4), "root_path": "/tmp/private", "status": "running"})
	if payload["project_uuid"] != projectUUID || payload["thread_uuid"] != threadUUID || payload["status"] != "running" {
		t.Fatalf("public fields missing: %+v", payload)
	}
	for _, forbidden := range []string{"target_uuid", "internal_id", "root_path"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("realtime payload retained %s: %+v", forbidden, payload)
		}
	}
}
