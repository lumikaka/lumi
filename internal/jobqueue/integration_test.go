package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/files"
	"lumi/internal/llm"
	"lumi/internal/modelsettings"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
	"lumi/internal/story"

	"github.com/riverqueue/river/rivertype"
)

type fakeProviderClient struct {
	mu          sync.Mutex
	attempts    map[string]int
	lastRequest llm.Request
	started     chan string
	release     chan struct{}
}

func newFakeProviderClient() *fakeProviderClient {
	return &fakeProviderClient{attempts: make(map[string]int), started: make(chan string, 10), release: make(chan struct{})}
}

func (client *fakeProviderClient) Check(context.Context, string, string, string) error { return nil }

func (client *fakeProviderClient) Generate(ctx context.Context, request llm.Request, onDelta func(string) error) (llm.Response, error) {
	mode := "success"
	for _, candidate := range []string{"cancel", "retry", "restart", "stale"} {
		if strings.Contains(request.Prompt, "["+candidate+"]") {
			mode = candidate
		}
	}
	client.mu.Lock()
	client.lastRequest = request
	client.attempts[mode]++
	attempt := client.attempts[mode]
	client.mu.Unlock()
	select {
	case client.started <- mode:
	default:
	}
	if mode == "cancel" {
		if onDelta != nil {
			if err := onDelta("不应落盘的片段"); err != nil {
				return llm.Response{}, err
			}
		}
		<-ctx.Done()
		return llm.Response{}, ctx.Err()
	}
	if mode == "restart" && attempt == 1 {
		<-ctx.Done()
		return llm.Response{}, ctx.Err()
	}
	if mode == "stale" {
		select {
		case <-client.release:
		case <-ctx.Done():
			return llm.Response{}, ctx.Err()
		}
	}
	if mode == "retry" && attempt == 1 {
		return llm.Response{}, &llm.Error{Code: llm.CodeAuthentication, SafeMessage: "测试鉴权失败。"}
	}
	var content string
	switch {
	case strings.Contains(request.Prompt, `"sections"`) && strings.Contains(request.Prompt, `"section_no"`):
		chapterCode := firstChapterCode(request.Prompt)
		encoded, _ := json.Marshal(map[string]any{"chapter_code": chapterCode, "title": "漫画标题", "sections": []map[string]any{{"section_no": 1, "title": "相遇", "storyboard": "## Section 核心剧情目标\n相遇。\n\n## 关键视觉瞬间\n**瞬间 1：凝视**"}}})
		content = string(encoded)
	case strings.Contains(request.Prompt, `"chapter_plans": []`):
		encoded, _ := json.Marshal(map[string]any{"story_md": "# STORY.md\n\n从现有章节反推的故事。", "chapter_plans": []any{}})
		content = string(encoded)
	case strings.Contains(request.Prompt, "target_chapter_codes_json") || strings.Contains(request.Prompt, "Server-assigned target chapter codes") || strings.Contains(request.Prompt, "服务端确定的目标编号"):
		codes := targetChapterCodes(request.Prompt)
		plans := make([]map[string]string, 0, len(codes))
		for _, code := range codes {
			plans = append(plans, map[string]string{"chapter_code": code, "title": "计划 " + code, "outline": "一个清晰剧情单元"})
		}
		encoded, _ := json.Marshal(map[string]any{"chapter_plans": plans})
		content = string(encoded)
	case strings.Contains(request.Prompt, `"story_md"`) && strings.Contains(request.Prompt, `"chapter_plans"`):
		encoded, _ := json.Marshal(map[string]any{"story_md": "# STORY.md\n\n生成的项目故事。", "chapter_plans": []map[string]string{{"chapter_code": "vol01.ch01", "title": "开端", "outline": "主角踏出第一步"}}})
		content = string(encoded)
	default:
		chapterCode := responseChapterCode(request.Prompt)
		encoded, _ := json.Marshal(map[string]string{"chapter_code": chapterCode, "title": "生成标题", "content": "生成后的完整正文", "content_format": "txt"})
		content = string(encoded)
	}
	encoded := []byte(content)
	chunks := []string{string(encoded[:len(encoded)/2]), string(encoded[len(encoded)/2:])}
	for _, chunk := range chunks {
		if onDelta != nil {
			if err := onDelta(chunk); err != nil {
				return llm.Response{}, err
			}
		}
	}
	return llm.Response{Content: strings.Join(chunks, ""), Usage: llm.Usage{InputTokens: 11, OutputTokens: 7}, FinishReason: "stop"}, nil
}

func firstChapterCode(value string) string {
	if match := regexp.MustCompile(`vol[0-9]+\.ch[0-9]+`).FindString(value); match != "" {
		return match
	}
	return "vol01.ch01"
}

func responseChapterCode(value string) string {
	codes := regexp.MustCompile(`vol[0-9]+\.ch[0-9]+`).FindAllString(value, -1)
	if (strings.Contains(value, "下一章 chapter_code") || strings.Contains(value, "Next chapter chapter_code")) && len(codes) > 0 {
		return codes[len(codes)-1]
	}
	return firstChapterCode(value)
}

func uniqueChapterCodes(value string) []string {
	seen := map[string]bool{}
	var result []string
	for _, code := range regexp.MustCompile(`vol[0-9]+\.ch[0-9]+`).FindAllString(value, -1) {
		if !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}
	return result
}

func targetChapterCodes(value string) []string {
	for _, marker := range []string{"Server-assigned target chapter codes:", "服务端确定的目标编号："} {
		if index := strings.Index(value, marker); index >= 0 {
			return uniqueChapterCodes(value[index+len(marker):])
		}
	}
	return uniqueChapterCodes(value)
}

func (client *fakeProviderClient) requestSnapshot() llm.Request {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.lastRequest
}

type queueHarness struct {
	app       *appstore.Store
	projects  *project.Manager
	queue     *Manager
	provider  provider.Provider
	project   project.Summary
	stories   *story.Service
	fakeModel *fakeProviderClient
}

func newQueueHarness(t *testing.T) *queueHarness {
	t.Helper()
	return newQueueHarnessWithPictureBook(t, &project.PictureBookInput{Format: project.PictureBookVertical})
}

func newQueueHarnessWithPictureBook(t *testing.T, pictureBook *project.PictureBookInput) *queueHarness {
	t.Helper()
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providerService := provider.NewService(app, provider.NewMemorySecretStore())
	configured, err := providerService.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/story-model", DefaultImageModel: "openai/gpt-image-1.5", APIKey: "project-must-never-contain-this-api-key"})
	if err != nil {
		t.Fatal(err)
	}
	fakeModel := newFakeProviderClient()
	queue := NewManager(providerService, fakeModel, nil)
	projects := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen).WithRuntime(queue).WithOpenHook(queue.StartProject)
	created, err := projects.CreateWithInput(ctx, project.CreateInput{
		Name:        "AI Story",
		PictureBook: pictureBook,
	}, project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var stories *story.Service
	if err := projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		stories = story.NewService(store)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	harness := &queueHarness{app: app, projects: projects, queue: queue, provider: configured, project: created, stories: stories, fakeModel: fakeModel}
	t.Cleanup(func() {
		_ = projects.Close()
		_ = app.Close()
	})
	return harness
}

func (harness *queueHarness) runtime(t *testing.T) *projectRuntime {
	t.Helper()
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func TestManagerKeepsProjectRuntimesAndRunningWorkIndependent(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch77")
	task := harness.createTask(t, chapter, "[stale] keep running while another project opens", "multi-project-runtime")
	waitFakeStarted(t, harness.fakeModel, "stale")
	firstRuntime := harness.runtime(t)

	second, err := harness.projects.Create(context.Background(), "Second runtime", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	secondRuntime, err := harness.queue.runtimeFor(second.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if secondRuntime == firstRuntime || secondRuntime.projectUUID != second.UUID {
		t.Fatalf("second runtime = %+v, first = %+v", secondRuntime, firstRuntime)
	}
	if current, err := harness.queue.runtimeFor(harness.project.UUID); err != nil || current != firstRuntime {
		t.Fatalf("opening second project replaced first runtime: runtime=%p error=%v", current, err)
	}
	if busy, err := harness.queue.HasActiveWork(context.Background(), harness.project.UUID); err != nil || !busy {
		t.Fatalf("first project active work = %t, error = %v", busy, err)
	}

	close(harness.fakeModel.release)
	if completed := waitTaskStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted); completed.Status != StatusCompleted {
		t.Fatalf("first project task = %+v", completed)
	}
	if _, err := harness.projects.CloseProject(context.Background(), second.UUID); err != nil {
		t.Fatal(err)
	}
	if current, err := harness.queue.runtimeFor(harness.project.UUID); err != nil || current != firstRuntime {
		t.Fatalf("closing second project affected first runtime: runtime=%p error=%v", current, err)
	}
}

func TestPictureBookMomentCountPlans(t *testing.T) {
	fourPanel := project.ComicLayoutFourPanel
	pageComic := project.ComicLayoutPageComic
	tests := []struct {
		name    string
		profile project.PictureBookProfile
		want    []int
		max     int
	}{
		{"classic", project.PictureBookProfile{Format: project.PictureBookClassic}, []int{1, 1, 1, 1, 1, 1}, 1},
		{"wordless", project.PictureBookProfile{Format: project.PictureBookWordless}, []int{1, 1, 1, 1, 1, 1}, 1},
		{"interactive", project.PictureBookProfile{Format: project.PictureBookInteractive}, []int{1, 1, 1, 1, 1, 1}, 1},
		{"four panel", project.PictureBookProfile{Format: project.PictureBookComicStory, ComicLayout: &fourPanel}, []int{4, 4, 4, 4, 4, 4}, 4},
		{"page comic", project.PictureBookProfile{Format: project.PictureBookComicStory, ComicLayout: &pageComic}, []int{4, 5, 3, 6, 4, 5}, 6},
		{"vertical strip", project.PictureBookProfile{Format: project.PictureBookVertical}, []int{2, 3, 1, 2, 3, 1}, 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pictureBookMomentCountPlan(test.profile, 6); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("plan=%v, want %v", got, test.want)
			}
			if got := maxPictureBookMoments(test.profile); got != test.max {
				t.Fatalf("max=%d, want %d", got, test.max)
			}
		})
	}
}

func (harness *queueHarness) createChapter(t *testing.T, code string) story.Chapter {
	t.Helper()
	chapter, err := harness.stories.CreateChapter(context.Background(), story.CreateChapterInput{ChapterCode: code, Title: "Opening", Content: "原有正文", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	return chapter
}

func (harness *queueHarness) createTask(t *testing.T, chapter story.Chapter, prompt, key string) Task {
	t.Helper()
	task, err := harness.queue.CreateChapterGeneration(context.Background(), harness.project.UUID, CreateGenerationInput{ChapterUUID: chapter.UUID, ProviderUUID: harness.provider.UUID, Prompt: prompt, IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func waitTaskStatus(t *testing.T, manager *Manager, projectUUID, taskUUID, wanted string) Task {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var last Task
	var lastErr error
	for time.Now().Before(deadline) {
		last, lastErr = manager.GetTask(context.Background(), projectUUID, taskUUID)
		if lastErr == nil && last.Status == wanted {
			return last
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("task %s did not reach %s; last=%+v error=%v", taskUUID, wanted, last, lastErr)
	return Task{}
}

func waitMaintenanceStatus(t *testing.T, manager *Manager, projectUUID, taskUUID, wanted string) MaintenanceTask {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var last MaintenanceTask
	var lastErr error
	for time.Now().Before(deadline) {
		last, lastErr = manager.GetMaintenanceTask(context.Background(), projectUUID, taskUUID)
		if lastErr == nil && last.Status == wanted {
			return last
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("maintenance task %s did not reach %s; last=%+v error=%v", taskUUID, wanted, last, lastErr)
	return MaintenanceTask{}
}

func waitFakeStarted(t *testing.T, fake *fakeProviderClient, mode string) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case actual := <-fake.started:
			if actual == mode {
				return
			}
		case <-deadline.C:
			t.Fatalf("fake provider did not start %s", mode)
		}
	}
}

func TestStoryGenerationPersistsAuditedIdempotentResult(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch01")
	created := harness.createTask(t, chapter, "改写这一章", "success-one")
	completed := waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCompleted)
	if completed.Progress != 100 || completed.Attempt != 1 || completed.ResourceUUID != chapter.UUID {
		t.Fatalf("completed task = %+v", completed)
	}
	encoded, err := json.Marshal(completed)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"river_job_id", `"id"`, "project-must-never-contain-this-api-key"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("task JSON contains %q: %s", forbidden, encoded)
		}
	}
	updated, err := harness.stories.GetChapter(context.Background(), chapter.UUID)
	if err != nil || updated.Revision != chapter.Revision+1 || updated.CurrentStory == nil || updated.CurrentStory.Content != "生成后的完整正文" {
		t.Fatalf("generated chapter = %+v, error = %v", updated, err)
	}
	versions, err := harness.stories.ListChapterStories(context.Background(), chapter.UUID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions = %+v, error = %v", versions, err)
	}
	idempotent := harness.createTask(t, chapter, "这个不同 prompt 不应创建第二个 job", "success-one")
	if idempotent.UUID != created.UUID {
		t.Fatalf("idempotent task UUID = %s, want %s", idempotent.UUID, created.UUID)
	}
	events, pagination, err := harness.queue.ListTaskEvents(context.Background(), harness.project.UUID, created.UUID, 0, 0, 100)
	if err != nil || pagination.HasMore || len(events) < 4 || events[0].EventType != "task_queued" {
		t.Fatalf("events = %+v, pagination = %+v, error = %v", events, pagination, err)
	}
	firstPage, firstCursor, err := harness.queue.ListTaskEvents(context.Background(), harness.project.UUID, created.UUID, 0, 0, 2)
	if err != nil || len(firstPage) != 2 || firstCursor.NextCursor == nil || *firstCursor.NextCursor != "2" || firstCursor.PrevCursor != nil || !firstCursor.HasMore {
		t.Fatalf("first event page = %+v, cursor = %+v, error = %v", firstPage, firstCursor, err)
	}
	forward, forwardCursor, err := harness.queue.ListTaskEvents(context.Background(), harness.project.UUID, created.UUID, 0, 2, 2)
	if err != nil || len(forward) == 0 || forwardCursor.PrevCursor == nil {
		t.Fatalf("forward event page = %+v, cursor = %+v, error = %v", forward, forwardCursor, err)
	}
	backward, backwardCursor, err := harness.queue.ListTaskEvents(context.Background(), harness.project.UUID, created.UUID, forward[0].Sequence, 0, 2)
	if err != nil || len(backward) != 2 || backward[0].Sequence != 1 || backward[1].Sequence != 2 || backwardCursor.NextCursor == nil {
		t.Fatalf("backward event page = %+v, cursor = %+v, error = %v", backward, backwardCursor, err)
	}
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	var counts struct{ Agents, Logs, Results int }
	if err := runtime.store.DB().Raw(`SELECT (SELECT COUNT(*) FROM agent_runs) AS agents, (SELECT COUNT(*) FROM llm_logs) AS logs, (SELECT COUNT(*) FROM story_generation_results) AS results`).Scan(&counts).Error; err != nil {
		t.Fatal(err)
	}
	if counts.Agents != 1 || counts.Logs != 1 || counts.Results != 1 {
		t.Fatalf("audit counts = %+v", counts)
	}
	agents := agent.NewService(harness.projects, harness.queue.providers, nil, harness.queue, nil)
	threads, err := agents.ListThreads(context.Background(), harness.project.UUID)
	if err != nil || len(threads) != 1 || threads[0].Status != agent.ThreadCompleted {
		t.Fatalf("chapter ChatArea thread=%+v err=%v", threads, err)
	}
	workflows, err := agents.ListWorkflows(context.Background(), harness.project.UUID)
	if err != nil || len(workflows) != 1 {
		t.Fatalf("chapter workflows=%+v err=%v", workflows, err)
	}
	workflow := workflows[0]
	if workflow.Kind != agent.WorkflowStoryChapter || workflow.ThreadUUID != threads[0].UUID || workflow.Status != agent.WorkflowCompleted || len(workflow.Steps) != 1 {
		t.Fatalf("chapter workflow=%+v", workflow)
	}
	step := workflow.Steps[0]
	if step.StepKey != agent.WorkflowStepStoryChapter || step.TaskUUID != created.UUID || step.ResourceUUID != chapter.UUID || step.Status != agent.WorkflowCompleted || step.Progress != 100 {
		t.Fatalf("chapter workflow step=%+v", step)
	}
	var publicSnapshot map[string]any
	if err := json.Unmarshal(workflow.InputSnapshot, &publicSnapshot); err != nil {
		t.Fatal(err)
	}
	if len(publicSnapshot) != 6 || publicSnapshot["project_uuid"] != harness.project.UUID || publicSnapshot["task_uuid"] != created.UUID || publicSnapshot["chapter_uuid"] != chapter.UUID || publicSnapshot["chapter_code"] != chapter.ChapterCode || publicSnapshot["prompt_key"] != "story_chapter" || publicSnapshot["model_source"] == "" {
		t.Fatalf("chapter public workflow snapshot=%+v", publicSnapshot)
	}
	for _, forbidden := range []string{"prompt", "system_prompt", "path", "id"} {
		if _, exists := publicSnapshot[forbidden]; exists {
			t.Fatalf("chapter workflow snapshot exposed %s: %+v", forbidden, publicSnapshot)
		}
	}
	runs, err := agents.ListWorkflowRuns(context.Background(), harness.project.UUID, workflow.UUID, "", 20)
	if err != nil || len(runs.Items) != 1 || runs.Items[0].Progress != 100 || runs.Items[0].TaskUUID != created.UUID {
		t.Fatalf("chapter workflow runs=%+v err=%v", runs, err)
	}
	var requestPayload, responsePayload string
	if err := runtime.store.DB().Raw(`SELECT request_payload,response FROM llm_logs LIMIT 1`).Row().Scan(&requestPayload, &responsePayload); err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(requestPayload)) || !json.Valid([]byte(responsePayload)) || !strings.Contains(requestPayload, `"system_prompt"`) || !strings.Contains(responsePayload, `"content"`) || strings.Contains(requestPayload+responsePayload, "project-must-never-contain-this-api-key") {
		t.Fatalf("unsafe or incomplete story LLM snapshots: request=%s response=%s", requestPayload, responsePayload)
	}
	if err := runtime.store.DB().Table("task_events").Where("uuid = ?", events[0].UUID).Update("event_type", "mutated").Error; err == nil {
		t.Fatal("append-only task event accepted an update")
	}
	if err := runtime.store.DB().Table("agent_events").Where("sequence = 1").Update("event_type", "mutated").Error; err == nil {
		t.Fatal("append-only agent event accepted an update")
	}
	var snapshot string
	if err := runtime.store.DB().Raw("SELECT input_snapshot FROM task_runs WHERE uuid = ?", created.UUID).Scan(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot, "project-must-never-contain-this-api-key") {
		t.Fatal("project task snapshot contains API key")
	}
}

func TestStoryProfileWorkflowFreezesChineseCatalogRequestAndAppliesResult(t *testing.T) {
	harness := newQueueHarness(t)
	task, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, KindStoryProfileGeneration, "", CreateStoryWorkflowInput{ProviderUUID: harness.provider.UUID, Prompt: "一名修表匠发现时间会倒流", ChapterCount: 1, IdempotencyKey: "profile-zh"})
	if err != nil {
		t.Fatal(err)
	}
	waitTaskStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	request := harness.fakeModel.requestSnapshot()
	expectedSystem, _ := promptcatalog.Lookup(promptcatalog.GroupStory, "json_system", promptcatalog.LanguageChinese)
	if !strings.HasSuffix(request.SystemPrompt, expectedSystem.DefaultValue) || !strings.Contains(request.SystemPrompt, "所有新生成的可读故事内容") || !strings.Contains(request.Prompt, "一名修表匠发现时间会倒流") || strings.Contains(request.Prompt, "{{") {
		t.Fatalf("unexpected frozen Chinese profile request: system=%q prompt=%q", request.SystemPrompt, request.Prompt)
	}
	profile, err := harness.stories.GetStoryProfile(context.Background())
	if err != nil || !strings.Contains(profile.StoryMD, "生成的项目故事") {
		t.Fatalf("profile=%+v err=%v", profile, err)
	}
	chapters, err := harness.stories.ListChapters(context.Background(), "active")
	if err != nil || len(chapters) != 1 || chapters[0].ChapterCode != "vol01.ch01" {
		t.Fatalf("chapters=%+v err=%v", chapters, err)
	}
	var scenario string
	if err := harness.runtime(t).store.DB().Raw(`SELECT logs.scenario FROM llm_logs logs JOIN task_runs task ON task.id=logs.task_run_id WHERE task.uuid=?`, task.UUID).Scan(&scenario).Error; err != nil || scenario != KindStoryProfileGeneration {
		t.Fatalf("workflow LLM log scenario=%q err=%v", scenario, err)
	}
}

func TestStoryProfileFromChaptersFreezesEnglishRequest(t *testing.T) {
	harness := newQueueHarness(t)
	harness.createChapter(t, "vol01.ch01")
	detail, err := harness.stories.GetProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	language := project.GenerationLanguageEnglish
	if _, err := harness.stories.UpdateProject(context.Background(), story.UpdateProjectInput{Name: detail.Name, Description: detail.Description, GenerationLanguage: &language, ExpectedRevision: detail.Revision}); err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, KindStoryProfileFromChapters, "", CreateStoryWorkflowInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "profile-from-en"})
	if err != nil {
		t.Fatal(err)
	}
	waitTaskStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	request := harness.fakeModel.requestSnapshot()
	if !strings.Contains(request.Prompt, "Infer the comic STORY.md") || !strings.Contains(request.SystemPrompt, "Write all newly generated human-readable story content") || !strings.Contains(request.Prompt, "原有正文") || strings.Contains(request.Prompt, "{{") {
		t.Fatalf("unexpected frozen English reconstruction request: %q", request.Prompt)
	}
}

func TestBatchPlanAndComicStoryboardWorkflowsApplyFrozenOutputs(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch01")
	batch, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, KindStoryChapterBatchPlan, "", CreateStoryWorkflowInput{ProviderUUID: harness.provider.UUID, Prompt: "规划两章调查剧情", ChapterCount: 2, IdempotencyKey: "batch-zh"})
	if err != nil {
		t.Fatal(err)
	}
	waitTaskStatus(t, harness.queue, harness.project.UUID, batch.UUID, StatusCompleted)
	chapters, err := harness.stories.ListChapters(context.Background(), "active")
	if err != nil || len(chapters) != 3 || chapters[1].ChapterCode != "vol01.ch02" || chapters[2].ChapterCode != "vol01.ch03" {
		t.Fatalf("planned chapters=%+v err=%v", chapters, err)
	}
	comic, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-zh"})
	if err != nil {
		t.Fatal(err)
	}
	waitTaskStatus(t, harness.queue, harness.project.UUID, comic.UUID, StatusCompleted)
	sections, err := production.NewService(harness.runtime(t).store, nil).ListSections(context.Background(), chapter.UUID)
	if err != nil || len(sections) != 1 || sections[0].CurrentStoryboard == nil || !strings.Contains(sections[0].CurrentStoryboard.ContentMD, "关键视觉瞬间") {
		t.Fatalf("sections=%+v err=%v", sections, err)
	}
	agents := agent.NewService(harness.projects, harness.queue.providers, nil, harness.queue, nil)
	threads, err := agents.ListThreads(context.Background(), harness.project.UUID)
	if err != nil || len(threads) != 2 {
		t.Fatalf("Story ChatArea threads=%+v err=%v", threads, err)
	}
	workflows, err := agents.ListWorkflows(context.Background(), harness.project.UUID)
	if err != nil || len(workflows) != 2 {
		t.Fatalf("Story workflows=%+v err=%v", workflows, err)
	}
	var workflow, batchWorkflow agent.Workflow
	for _, candidate := range workflows {
		switch candidate.Kind {
		case agent.WorkflowComicStoryboard:
			workflow = candidate
		case agent.WorkflowStoryChapterBatchPlan:
			batchWorkflow = candidate
		}
	}
	if batchWorkflow.UUID == "" || batchWorkflow.Status != agent.WorkflowCompleted || len(batchWorkflow.Steps) != 1 || batchWorkflow.Steps[0].StepKey != agent.WorkflowStepChapterBatchPlan || batchWorkflow.Steps[0].TaskUUID != batch.UUID || batchWorkflow.Steps[0].Progress != 100 {
		t.Fatalf("batch workflow projection=%+v", batchWorkflow)
	}
	if workflow.UUID == "" || workflow.Status != agent.WorkflowCompleted || len(workflow.Steps) != 1 {
		t.Fatalf("storyboard workflow projection=%+v", workflow)
	}
	var storyboardThread agent.Thread
	for _, thread := range threads {
		if thread.UUID == workflow.ThreadUUID {
			storyboardThread = thread
		}
	}
	if storyboardThread.Status != agent.ThreadCompleted {
		t.Fatalf("storyboard ChatArea thread=%+v", storyboardThread)
	}
	step := workflow.Steps[0]
	if step.StepKey != agent.WorkflowStepComicStoryboard || step.TaskUUID != comic.UUID || step.ResourceUUID != chapter.UUID || step.Status != agent.WorkflowCompleted || step.Progress != 100 || !strings.Contains(string(step.Output), sections[0].UUID) {
		t.Fatalf("storyboard workflow step=%+v", step)
	}
	runs, err := agents.ListWorkflowRuns(context.Background(), harness.project.UUID, workflow.UUID, "", 20)
	if err != nil || len(runs.Items) != 1 || runs.Items[0].Attempt != 1 {
		t.Fatalf("storyboard diagnostic runs=%+v err=%v", runs, err)
	}
	logs, err := agents.ListWorkflowLLMLogs(context.Background(), harness.project.UUID, workflow.UUID, step.UUID, 1, 20)
	if err != nil || logs.Pagination.Total != 1 || len(logs.Items) != 1 || logs.Items[0].WorkflowStepUUID != step.UUID || logs.Items[0].Scenario != KindComicStoryboardGeneration {
		t.Fatalf("storyboard diagnostic logs=%+v err=%v", logs, err)
	}
}

func TestStoryGenerationFreezesProjectLanguageIntoTaskAndSystemPrompt(t *testing.T) {
	harness := newQueueHarness(t)
	ctx := context.Background()
	detail, err := harness.stories.GetProject(ctx)
	if err != nil {
		t.Fatal(err)
	}
	language := project.GenerationLanguageEnglish
	if _, err := harness.stories.UpdateProject(ctx, story.UpdateProjectInput{Name: detail.Name, Description: detail.Description, GenerationLanguage: &language, ExpectedRevision: detail.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.stories.UpdatePromptGroup(ctx, story.UpdatePromptGroupInput{
		PromptGroup:             "runtime",
		Prompts:                 map[string]string{"project_language_instruction": "FROZEN PROJECT LANGUAGE INSTRUCTION"},
		ExpectedCurrentVersions: map[string]int{"project_language_instruction": 2},
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch09")
	task := harness.createTask(t, chapter, "Write a quiet opening", "language-en")
	if _, err := harness.stories.UpdatePromptGroup(ctx, story.UpdatePromptGroupInput{
		PromptGroup:             "runtime",
		Prompts:                 map[string]string{"project_language_instruction": "NEWER PROJECT LANGUAGE INSTRUCTION"},
		ExpectedCurrentVersions: map[string]int{"project_language_instruction": 3},
	}); err != nil {
		t.Fatal(err)
	}
	completed := waitTaskStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	var snapshot storyGenerationSnapshot
	if err := json.Unmarshal(completed.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.GenerationLanguage != project.GenerationLanguageEnglish {
		t.Fatalf("snapshot language = %q", snapshot.GenerationLanguage)
	}
	request := harness.fakeModel.requestSnapshot()
	if !strings.Contains(request.SystemPrompt, "Return one valid JSON object only") {
		t.Fatalf("system prompt missing JSON contract: %q", request.SystemPrompt)
	}
	if !strings.Contains(request.SystemPrompt, "FROZEN PROJECT LANGUAGE INSTRUCTION") || strings.Contains(request.SystemPrompt, "NEWER PROJECT LANGUAGE INSTRUCTION") || !strings.Contains(request.Prompt, "Original user prompt") {
		t.Fatalf("request missing frozen English language/template context: system=%q user=%q", request.SystemPrompt, request.Prompt)
	}
}

func TestNextStoryChapterFinalRequestIsBilingualAndKeepsContextOrder(t *testing.T) {
	for _, test := range []struct {
		name, language, templateMarker, instruction string
	}{
		{name: "zh-Hans", language: project.GenerationLanguageSimplifiedChinese, templateMarker: "可选上一章 JSON", instruction: "项目语言：简体中文"},
		{name: "en", language: project.GenerationLanguageEnglish, templateMarker: "Optional previous chapter JSON", instruction: "Project language: English"},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newQueueHarness(t)
			if test.language == project.GenerationLanguageEnglish {
				detail, err := harness.stories.GetProject(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				language := test.language
				if _, err := harness.stories.UpdateProject(context.Background(), story.UpdateProjectInput{Name: detail.Name, Description: detail.Description, GenerationLanguage: &language, ExpectedRevision: detail.Revision}); err != nil {
					t.Fatal(err)
				}
			}
			harness.createChapter(t, "vol01.ch01")
			target, err := harness.stories.CreateChapter(context.Background(), story.CreateChapterInput{ChapterCode: "vol01.ch02", Title: ""})
			if err != nil {
				t.Fatal(err)
			}
			task, err := harness.queue.CreateChapterGeneration(context.Background(), harness.project.UUID, CreateGenerationInput{ChapterUUID: target.UUID, ProviderUUID: harness.provider.UUID, PromptKey: "next_story_chapter", Prompt: "让主角发现新的线索", IdempotencyKey: "next-" + test.name})
			if err != nil {
				t.Fatal(err)
			}
			waitTaskStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
			request := harness.fakeModel.requestSnapshot()
			previousAt := strings.Index(request.Prompt, test.templateMarker)
			guidanceAt := strings.Index(request.Prompt, "让主角发现新的线索")
			targetAt := strings.LastIndex(request.Prompt, "vol01.ch02")
			if !strings.Contains(request.SystemPrompt, test.instruction) || !strings.Contains(request.Prompt, "原有正文") || previousAt < 0 || guidanceAt <= previousAt || targetAt <= guidanceAt || strings.Contains(request.Prompt, "{{") {
				t.Fatalf("unexpected next-chapter request: system=%q user=%q", request.SystemPrompt, request.Prompt)
			}
		})
	}
}

func TestAssetIntegrityMaintenanceUsesRiverAndPublicTaskProjection(t *testing.T) {
	harness := newQueueHarness(t)
	created, err := harness.queue.CreateMaintenanceTask(context.Background(), harness.project.UUID, CreateMaintenanceInput{Kind: KindAssetIntegrityScan})
	if err != nil {
		t.Fatal(err)
	}
	completed := waitMaintenanceStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCompleted)
	if completed.Progress != 100 || completed.Attempt != 1 || !isUUIDv7(completed.ResourceUUID) {
		t.Fatalf("completed maintenance = %+v", completed)
	}
	encoded, err := json.Marshal(completed)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"river_job_id", `"id"`, "key_path", "root_path"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("maintenance task JSON contains %q: %s", forbidden, encoded)
		}
	}
	var scans int64
	if err := harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error {
		return store.DB().Table("integrity_scans").Where("uuid = ? AND status = ?", completed.ResourceUUID, "completed").Count(&scans).Error
	}); err != nil {
		t.Fatal(err)
	}
	if scans != 1 {
		t.Fatalf("completed scans = %d", scans)
	}
	items, err := harness.queue.ListMaintenanceTasks(context.Background(), harness.project.UUID, 10)
	if err != nil || len(items) != 1 || items[0].UUID != created.UUID {
		t.Fatalf("maintenance items = %+v, error = %v", items, err)
	}
	events, pagination, err := harness.queue.ListMaintenanceTaskEvents(context.Background(), harness.project.UUID, created.UUID, 0, 0, 20)
	if err != nil || pagination.HasMore || len(events) < 3 || events[0].EventType != "task_queued" || events[len(events)-1].EventType != "task_completed" {
		t.Fatalf("maintenance events = %+v, pagination = %+v, error = %v", events, pagination, err)
	}
	var plan files.GCPlan
	if err := harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error {
		var planErr error
		plan, planErr = files.NewService(store, nil).GCDryRun(context.Background(), 48*time.Hour)
		return planErr
	}); err != nil {
		t.Fatal(err)
	}
	gcTask, err := harness.queue.CreateMaintenanceTask(context.Background(), harness.project.UUID, CreateMaintenanceInput{Kind: KindAssetGCApply, PlanUUID: plan.UUID, GraceHours: 48})
	if err != nil {
		t.Fatal(err)
	}
	gcTask = waitMaintenanceStatus(t, harness.queue, harness.project.UUID, gcTask.UUID, StatusCompleted)
	if gcTask.MaxAttempts != 1 || !strings.Contains(string(gcTask.InputSnapshot), `"grace_hours":48`) {
		t.Fatalf("GC maintenance retry/snapshot = %+v", gcTask)
	}
}

func TestMaintenanceRetryProjectionKeepsDatabaseUniquenessActive(t *testing.T) {
	harness := newQueueHarness(t)
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	taskUUID, _ := newUUIDv7()
	now := time.Now().UTC()
	record := maintenanceRecord{UUID: taskUUID, ProjectID: runtime.projectID, Kind: KindAssetReconcile, ResourceUUID: taskUUID, InputVersion: 1, InputSnapshot: `{"version":1}`, Status: StatusRunning, Progress: 1, Attempt: 1, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now}
	if err := runtime.store.DB().Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := runtime.failMaintenance(context.Background(), record, errors.New("retryable test failure"), 1); err != nil {
		t.Fatal(err)
	}
	retrying, err := getMaintenanceRecord(context.Background(), runtime.store.DB(), runtime.projectID, taskUUID)
	if err != nil {
		t.Fatal(err)
	}
	if retrying.Status != StatusQueued || retrying.CompletedAt != nil {
		t.Fatalf("retry projection = %+v", retrying)
	}
	duplicateUUID, _ := newUUIDv7()
	duplicate := maintenanceRecord{UUID: duplicateUUID, ProjectID: runtime.projectID, Kind: KindAssetReconcile, ResourceUUID: duplicateUUID, InputVersion: 1, InputSnapshot: `{}`, Status: StatusQueued, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now}
	if err := runtime.store.DB().Create(&duplicate).Error; err == nil {
		t.Fatal("active retry allowed a duplicate maintenance kind")
	}
}

func TestRunningStoryGenerationCancellationPreservesCurrentStory(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch02")
	created := harness.createTask(t, chapter, "[cancel] 等待取消", "cancel-one")
	waitFakeStarted(t, harness.fakeModel, "cancel")
	cancelled, err := harness.queue.CancelTask(context.Background(), harness.project.UUID, created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != StatusCancelled {
		cancelled = waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCancelled)
	}
	current, err := harness.stories.GetChapter(context.Background(), chapter.UUID)
	if err != nil || current.Revision != chapter.Revision || current.CurrentStory == nil || current.CurrentStory.Content != "原有正文" {
		t.Fatalf("chapter after cancellation = %+v, error = %v", current, err)
	}
	versions, err := harness.stories.ListChapterStories(context.Background(), chapter.UUID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions after cancellation = %+v, error = %v", versions, err)
	}
	workflowStatus, stepStatus, threadStatus := storyTaskWorkflowState(t, harness, created.UUID)
	if workflowStatus != StatusCancelled || stepStatus != StatusCancelled || threadStatus != StatusCancelled {
		t.Fatalf("direct task cancellation workflow=%s/%s/%s", workflowStatus, stepStatus, threadStatus)
	}
}

func TestCancelledStoryGenerationCanBeExplicitlyRetried(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch08")
	created := harness.createTask(t, chapter, "[cancel] 取消后恢复", "cancel-retry-one")
	waitFakeStarted(t, harness.fakeModel, "cancel")
	if _, err := harness.queue.CancelTask(context.Background(), harness.project.UUID, created.UUID); err != nil {
		t.Fatal(err)
	}
	waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCancelled)
	retried, err := harness.queue.RetryTask(context.Background(), harness.project.UUID, created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status == StatusCancelled || retried.CancelRequestedAt != nil {
		t.Fatalf("retried task retained cancellation state: %+v", retried)
	}
	waitFakeStarted(t, harness.fakeModel, "cancel")
	if _, err := harness.queue.CancelTask(context.Background(), harness.project.UUID, created.UUID); err != nil {
		t.Fatal(err)
	}
}

func TestStoryModelOverrideIsFrozenAcrossRetry(t *testing.T) {
	harness := newQueueHarness(t)
	ctx := context.Background()
	if _, _, err := harness.queue.providers.Settings().Update(ctx, map[string]any{
		sitesettings.BailianWorkspaceKey: "workspace-1",
		sitesettings.BailianRegionKey:    "cn-beijing",
		sitesettings.BailianAPIKeyKey:    "bailian-secret",
	}); err != nil {
		t.Fatal(err)
	}
	providers, err := harness.queue.providers.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bailian, err := harness.queue.providers.MarkVerified(ctx, providers[1].UUID)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	resolver := modelsettings.NewResolver(harness.queue.providers)
	settings, err := resolver.Patch(ctx, runtime.store, modelsettings.PatchInput{ExpectedRevision: 0, Changes: map[string]*modelsettings.Selection{
		modelsettings.StoryText: {ProviderUUID: harness.provider.UUID, Model: harness.provider.DefaultModel},
	}})
	if err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch18")
	created, err := harness.queue.CreateChapterGeneration(ctx, harness.project.UUID, CreateGenerationInput{ChapterUUID: chapter.UUID, Prompt: "[retry] freeze the selected model", IdempotencyKey: "model-freeze-retry"})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusFailed)
	if failed.ProviderUUID != harness.provider.UUID || failed.Model != harness.provider.DefaultModel || failed.ModelSource != modelsettings.SourceScenarioOverride {
		t.Fatalf("initial frozen model=%+v", failed)
	}
	if _, err := resolver.Patch(ctx, runtime.store, modelsettings.PatchInput{ExpectedRevision: settings.Revision, Changes: map[string]*modelsettings.Selection{
		modelsettings.StoryText: {ProviderUUID: bailian.UUID, Model: provider.BailianTextModel},
	}}); err != nil {
		t.Fatal(err)
	}
	retried, err := harness.queue.RetryTask(ctx, harness.project.UUID, created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ProviderUUID != harness.provider.UUID || retried.Model != harness.provider.DefaultModel || retried.ModelSource != modelsettings.SourceScenarioOverride {
		t.Fatalf("retry drifted before execution: %+v", retried)
	}
	var snapshot storyGenerationSnapshot
	if err := json.Unmarshal(retried.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ProviderUUID != harness.provider.UUID || snapshot.Model != harness.provider.DefaultModel || snapshot.ModelSource != modelsettings.SourceScenarioOverride {
		t.Fatalf("retry snapshot drifted: %+v", snapshot)
	}
}

func TestCancelDoesNotContradictDurableStoryWorkflowResult(t *testing.T) {
	harness := newQueueHarness(t)
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	taskUUID, _ := newUUIDv7()
	resultUUID, _ := newUUIDv7()
	now := time.Now().UTC()
	record := taskRecord{
		UUID: taskUUID, ProjectID: runtime.projectID, Kind: KindStoryProfileGeneration,
		ResourceUUID: harness.project.UUID, InputVersion: 2, InputSnapshot: `{}`,
		Status: StatusRunning, IdempotencyKey: "durable-workflow-result", Retryable: true,
		ProviderUUID: harness.provider.UUID, Model: harness.provider.DefaultModel,
		Progress: 90, Attempt: 1, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.store.DB().Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := runtime.store.DB().Exec(`INSERT INTO story_prompt_results(uuid,task_run_id,result_kind,output_json,created_at) VALUES(?,?,?,?,?)`, resultUUID, record.ID, record.Kind, `{"story_md":"durable"}`, now).Error; err != nil {
		t.Fatal(err)
	}
	unchanged, err := harness.queue.CancelTask(context.Background(), harness.project.UUID, taskUUID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Status != StatusRunning || unchanged.CancelRequestedAt != nil {
		t.Fatalf("durable workflow result was contradicted by cancellation: %+v", unchanged)
	}
}

func TestFailedStoryGenerationCanBeExplicitlyRetriedOnce(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch03")
	created := harness.createTask(t, chapter, "[retry] 修复配置后重试", "retry-one")
	failed := waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusFailed)
	if failed.ErrorCode != llm.CodeAuthentication {
		t.Fatalf("failed task = %+v", failed)
	}
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := getTaskRecord(context.Background(), runtime.store.DB(), runtime.projectID, created.UUID)
	if err != nil || record.RiverJobID == nil {
		t.Fatalf("task record = %+v, error = %v", record, err)
	}
	waitJobState(t, runtime.client, *record.RiverJobID, rivertype.JobStateCancelled)
	customTemplate := "CUSTOM {{input_prompt}} {{story_md}} {{chapter_plan_json}} {{generated_summaries_json}} {{chapter_code}}"
	if _, err := harness.stories.CreatePromptVersion(context.Background(), story.CreatePromptInput{PromptGroup: "story", PromptKey: "story_chapter", Prompt: customTemplate, ExpectedCurrentVersion: 1}); err != nil {
		t.Fatal(err)
	}
	detail, err := harness.stories.GetProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	english := project.GenerationLanguageEnglish
	if _, err := harness.stories.UpdateProject(context.Background(), story.UpdateProjectInput{Name: detail.Name, Description: detail.Description, GenerationLanguage: &english, ExpectedRevision: detail.Revision}); err != nil {
		t.Fatal(err)
	}
	queued, err := harness.queue.RetryTask(context.Background(), harness.project.UUID, created.UUID)
	if err != nil || queued.Status != StatusQueued {
		t.Fatalf("retry task = %+v, error = %v", queued, err)
	}
	completed := waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCompleted)
	if completed.Attempt != 2 {
		t.Fatalf("completed retry = %+v", completed)
	}
	var projectionCounts struct{ Threads, Workflows int64 }
	if err := runtime.store.DB().Raw(`SELECT (SELECT COUNT(*) FROM chat_threads) AS threads,(SELECT COUNT(*) FROM workflows) AS workflows`).Scan(&projectionCounts).Error; err != nil {
		t.Fatal(err)
	}
	workflowStatus, stepStatus, threadStatus := storyTaskWorkflowState(t, harness, created.UUID)
	if projectionCounts.Threads != 1 || projectionCounts.Workflows != 1 || workflowStatus != StatusCompleted || stepStatus != StatusCompleted || threadStatus != StatusCompleted {
		t.Fatalf("direct retry chapter projection counts=%+v state=%s/%s/%s", projectionCounts, workflowStatus, stepStatus, threadStatus)
	}
	retriedRequest := harness.fakeModel.requestSnapshot()
	if !strings.Contains(retriedRequest.SystemPrompt, "项目语言：简体中文") || strings.Contains(retriedRequest.Prompt, "CUSTOM") {
		t.Fatalf("retry did not reuse frozen language/prompt snapshot: system=%q user=%q", retriedRequest.SystemPrompt, retriedRequest.Prompt)
	}
	versions, err := harness.stories.ListChapterStories(context.Background(), chapter.UUID)
	if err != nil || len(versions) != 2 || versions[0].Content != "生成后的完整正文" {
		t.Fatalf("retried versions = %+v, error = %v", versions, err)
	}
	var resultCount int64
	if err := runtime.store.DB().Table("story_generation_results").Where("task_run_id = ?", record.ID).Count(&resultCount).Error; err != nil || resultCount != 1 {
		t.Fatalf("result count = %d, error = %v", resultCount, err)
	}
}

func TestProjectCloseAndReopenRecoversDurableGeneration(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch04")
	created := harness.createTask(t, chapter, "[restart] 关闭后恢复", "restart-one")
	waitFakeStarted(t, harness.fakeModel, "restart")
	if err := harness.projects.CloseCurrent(context.Background()); err != nil {
		t.Fatal(err)
	}
	reopened, err := harness.projects.OpenRecent(context.Background(), harness.project.UUID)
	if err != nil || reopened.UUID != harness.project.UUID {
		t.Fatalf("reopened = %+v, error = %v", reopened, err)
	}
	if err := harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error {
		harness.stories = story.NewService(store)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	completed := waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCompleted)
	if completed.Attempt < 2 {
		t.Fatalf("recovered task = %+v", completed)
	}
	current, err := harness.stories.GetChapter(context.Background(), chapter.UUID)
	if err != nil || current.Revision != chapter.Revision+1 || current.CurrentStory == nil || current.CurrentStory.Content != "生成后的完整正文" {
		t.Fatalf("recovered chapter = %+v, error = %v", current, err)
	}
}

func TestGenerationSnapshotRefusesToOverwriteNewerManualRevision(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch05")
	created := harness.createTask(t, chapter, "[stale] 固化输入版本", "stale-one")
	waitFakeStarted(t, harness.fakeModel, "stale")
	manual, err := harness.stories.UpdateStory(context.Background(), chapter.UUID, story.UpdateStoryInput{Content: "生成期间完成的手动编辑", ContentFormat: "md", ExpectedRevision: chapter.Revision})
	if err != nil {
		t.Fatal(err)
	}
	close(harness.fakeModel.release)
	failed := waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusFailed)
	if failed.ErrorCode != story.CodeChapterRevisionConflict {
		t.Fatalf("stale task = %+v", failed)
	}
	current, err := harness.stories.GetChapter(context.Background(), chapter.UUID)
	if err != nil || current.Revision != manual.Revision || current.CurrentStory == nil || current.CurrentStory.Content != "生成期间完成的手动编辑" {
		t.Fatalf("chapter after stale result = %+v, error = %v", current, err)
	}
	versions, err := harness.stories.ListChapterStories(context.Background(), chapter.UUID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions after stale result = %+v, error = %v", versions, err)
	}
}

func TestClassifyUnknownPersistenceErrorsAsRetryable(t *testing.T) {
	code, _, retryable, cancelled := classifyWorkError(errors.New("disk unavailable"))
	if code != "local_persistence_error" || !retryable || cancelled {
		t.Fatalf("classification = %q retryable=%v cancelled=%v", code, retryable, cancelled)
	}
}

func TestClassifyProductionErrorsAsNonRetryableDomainFailures(t *testing.T) {
	code, message, retryable, cancelled := classifyWorkError(&production.Error{Code: production.CodeStateConflict, Message: "section state changed"})
	if code != production.CodeStateConflict || message != "section state changed" || retryable || cancelled {
		t.Fatalf("classification = %q %q retryable=%v cancelled=%v", code, message, retryable, cancelled)
	}
}
