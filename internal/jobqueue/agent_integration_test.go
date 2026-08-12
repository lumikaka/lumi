package jobqueue

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/llm"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/story"
)

type riverAgentModel struct {
	mu       sync.Mutex
	requests []llm.ChatRequest
	started  chan struct{}
	release  chan struct{}
	calls    int
}

func newRiverAgentModel() *riverAgentModel {
	return &riverAgentModel{started: make(chan struct{}, 1), release: make(chan struct{})}
}

func (*riverAgentModel) Check(context.Context, string, string, string) error { return nil }

func (*riverAgentModel) Generate(_ context.Context, request llm.Request, onDelta func(string) error) (llm.Response, error) {
	content := "第一章：小狐狸在月光下收到一封信，穿过森林，把星光送给害怕黑夜的朋友。"
	if strings.Contains(request.Prompt, "漫画设定项选择器") || strings.Contains(request.Prompt, "comic setting-asset selector") {
		sectionID := ""
		if match := regexp.MustCompile(`(?m)## Section\s+([0-9a-f-]{36})`).FindStringSubmatch(request.Prompt); len(match) == 2 {
			sectionID = match[1]
		}
		encoded, _ := json.Marshal(map[string]any{"sectionId": sectionID, "titles": []string{"月光小狐狸"}, "reason": "主角直接出现"})
		content = string(encoded)
	}
	if strings.Contains(request.SystemPrompt, "Return one valid JSON object only") {
		chapterCode := "vol01.ch01"
		if match := regexp.MustCompile(`"chapter_code":\s*"(vol[0-9]+\.ch[0-9]+)"`).FindStringSubmatch(request.Prompt); len(match) == 2 {
			chapterCode = match[1]
		}
		encoded, _ := json.Marshal(map[string]string{"chapter_code": chapterCode, "title": "月光邮差", "content": content, "content_format": "txt"})
		content = string(encoded)
	}
	if len(request.Images) > 0 {
		content = `{"plan":{"layout":"white_background_objects"},"assets":[{"filename":"moon-fox.png","title":"月光小狐狸","summary":"温暖的信使","tags":["角色","主角"],"crop_box":{"x":0,"y":0,"width":1,"height":1},"confidence":0.99}],"quality_checks":["主体完整"]}`
	}
	if onDelta != nil {
		_ = onDelta(content)
	}
	return llm.Response{Content: content, FinishReason: "stop"}, nil
}

func (model *riverAgentModel) Complete(ctx context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	model.mu.Lock()
	model.calls++
	call := model.calls
	model.requests = append(model.requests, request)
	model.mu.Unlock()
	if call == 1 && len(request.Tools) > 0 {
		model.started <- struct{}{}
		select {
		case <-model.release:
		case <-ctx.Done():
			return llm.ChatResponse{}, ctx.Err()
		}
	}
	content := "River Agent 完成"
	user := ""
	if len(request.Messages) > 0 {
		user = request.Messages[len(request.Messages)-1].Content
	}
	switch {
	case strings.Contains(user, "根据用户故事想法生成 STORY.md") || strings.Contains(user, "Generate STORY.md and chapter plans"):
		content = `{"story_md":"# STORY.md\n\n月光小狐狸替月亮送信，帮助害怕黑夜的朋友。","chapter_plans":[{"chapter_code":"vol01.ch01","title":"月光邮差","outline":"小狐狸收到月亮信件并踏上旅程。"}]}`
	case strings.Contains(user, "根据已有章节正文反推漫画 STORY.md") || strings.Contains(user, "Infer the comic STORY.md"):
		content = `{"story_md":"# STORY.md\n\n## 故事梗概\n月光小狐狸替月亮送信，帮助害怕黑夜的朋友。","chapter_plans":[]}`
	case strings.Contains(user, "漫画分集脚本") || strings.Contains(user, "comic episode script"):
		content = `{"chapter_code":"vol01.ch01","title":"月光邮差","sections":[{"section_no":1,"title":"月下启程","storyboard":"## Section 核心剧情目标\n\n小狐狸收到月亮的信。\n\n## 关键视觉瞬间\n\n**瞬间 1：收信**（全屏 / 竖向 / 顶部）\n* **镜头与调度**：月光照入森林。"}]}`
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: content}, FinishReason: "stop"}, nil
}

func TestRiverAgentWorkerPersistsFIFOAcrossConcurrentEnqueue(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	configured, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/agent-model", APIKey: "river-agent-secret"})
	if err != nil {
		t.Fatal(err)
	}
	model := newRiverAgentModel()
	queue := NewManager(providers, model, nil)
	projects := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen)
	agents := agent.NewService(projects, providers, model, queue, nil)
	queue.WithAgentService(agents)
	projects.WithRuntime(queue).WithOpenHook(queue.StartProject).WithOpenHook(agents.ReconcileOnOpen)
	created, err := projects.Create(ctx, "River Agent", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = projects.Close(); _ = app.Close() })
	thread, err := agents.CreateThread(ctx, created.UUID, agent.CreateThreadInput{Title: "FIFO", ProviderUUID: configured.UUID})
	if err != nil {
		t.Fatal(err)
	}
	first, err := agents.CreateTurn(ctx, created.UUID, thread.UUID, agent.CreateTurnInput{InputText: "第一条 River 消息"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.started:
	case <-time.After(5 * time.Second):
		t.Fatal("first River Agent job did not start")
	}
	var pending int64
	if err := projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		return store.DB().Table("llm_logs").Where("source_type='project_chat' AND chat_thread_id=(SELECT id FROM chat_threads WHERE uuid=?) AND status='pending'", thread.UUID).Count(&pending).Error
	}); err != nil || pending != 1 {
		t.Fatalf("pending chat logs=%d err=%v", pending, err)
	}
	second, err := agents.CreateTurn(ctx, created.UUID, thread.UUID, agent.CreateTurnInput{InputText: "第二条 River 消息"})
	if err != nil {
		t.Fatal(err)
	}
	close(model.release)
	deadline := time.Now().Add(8 * time.Second)
	var turns []agent.Turn
	for time.Now().Before(deadline) {
		turns, err = agents.ListTurns(ctx, created.UUID, thread.UUID)
		if err == nil && len(turns) == 2 && turns[0].Status == agent.TurnCompleted && turns[1].Status == agent.TurnCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(turns) != 2 || turns[0].UUID != first.UUID || turns[1].UUID != second.UUID || turns[0].Status != agent.TurnCompleted || turns[1].Status != agent.TurnCompleted {
		t.Fatalf("River turns = %+v, error=%v", turns, err)
	}
	model.mu.Lock()
	requests := append([]llm.ChatRequest(nil), model.requests...)
	model.mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("model requests = %d", len(requests))
	}
	var completedLogs, completedSnapshots int64
	if err := projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		if err := store.DB().Table("llm_logs").Where("source_type='project_chat' AND chat_thread_id=(SELECT id FROM chat_threads WHERE uuid=?) AND status='completed'", thread.UUID).Count(&completedLogs).Error; err != nil {
			return err
		}
		return store.DB().Table("llm_logs").Where("source_type='project_chat' AND chat_thread_id=(SELECT id FROM chat_threads WHERE uuid=?) AND status='completed' AND request_payload IS NOT NULL AND response IS NOT NULL", thread.UUID).Count(&completedSnapshots).Error
	}); err != nil || completedLogs != 2 {
		t.Fatalf("completed chat logs=%d err=%v", completedLogs, err)
	}
	if completedSnapshots != completedLogs {
		t.Fatalf("completed chat snapshots=%d logs=%d", completedSnapshots, completedLogs)
	}
	for _, message := range requests[0].Messages {
		if strings.Contains(message.Content, "第二条") {
			t.Fatal("second queued turn leaked into first River model context")
		}
	}
	items, err := agents.ListItems(ctx, created.UUID, thread.UUID, "", "", 100)
	if err != nil || len(items.Items) != 4 || items.Items[0].Content != "第一条 River 消息" || items.Items[1].Content != "第二条 River 消息" {
		t.Fatalf("River items = %+v, error=%v", items.Items, err)
	}
}

func TestRiverYoloCompletesSixDomainStepsEndToEnd(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	configured, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/yolo-model", APIKey: "yolo-secret"})
	if err != nil {
		t.Fatal(err)
	}
	model := newRiverAgentModel()
	queue := NewManager(providers, model, nil).WithImageClient(successfulImageProvider{content: productionPNG(t)})
	projects := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen)
	agents := agent.NewService(projects, providers, model, queue, nil)
	queue.WithAgentService(agents)
	projects.WithRuntime(queue).WithOpenHook(queue.StartProject).WithOpenHook(agents.ReconcileOnOpen)
	created, err := projects.CreateWithInput(ctx, project.CreateInput{Name: "Yolo E2E", PictureBook: &project.PictureBookInput{Format: project.PictureBookVertical}}, project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = projects.Close(); _ = app.Close() })
	workflow, err := agents.CreateYoloWorkflow(ctx, created.UUID, agent.CreateYoloInput{Title: "月光邮差", StoryPrompt: "一只小狐狸替月亮送出重要的信。", ProviderUUID: configured.UUID, IdempotencyKey: "river-yolo-e2e"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		workflow, err = agents.GetWorkflow(ctx, created.UUID, workflow.UUID)
		if err == nil && workflow.Status == agent.WorkflowCompleted {
			break
		}
		if err == nil && workflow.Status == agent.WorkflowFailed {
			t.Fatalf("Yolo failed: %+v", workflow)
		}
		time.Sleep(40 * time.Millisecond)
	}
	if workflow.Status != agent.WorkflowCompleted || len(workflow.Steps) != len(agent.YoloStepKeys) {
		t.Fatalf("Yolo workflow = %+v, error=%v", workflow, err)
	}
	for _, step := range workflow.Steps {
		if step.Status != "completed" {
			t.Fatalf("Yolo step not complete: %+v", step)
		}
	}
	if err := projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		chapters, err := story.NewService(store).ListChapters(ctx, "active")
		if err != nil || len(chapters) != 1 || chapters[0].CurrentStory == nil {
			t.Fatalf("Yolo chapters = %+v, error=%v", chapters, err)
		}
		service := production.NewService(store, nil)
		assets, err := service.ListPremiseAssets(ctx, "", "active")
		if err != nil || len(assets) == 0 {
			t.Fatalf("Yolo premise assets = %+v, error=%v", assets, err)
		}
		sections, err := service.ListSections(ctx, chapters[0].UUID)
		if err != nil || len(sections) < 1 || len(sections) > 6 || sections[0].CurrentImage == nil {
			t.Fatalf("Yolo comic sections = %+v, error=%v", sections, err)
		}
		var workflowLogs, workflowSnapshots int64
		if err := store.DB().Table("llm_logs AS logs").Joins("JOIN workflows ON workflows.id=logs.workflow_id").Where("workflows.uuid=? AND logs.source_type='workflow' AND logs.status='completed'", workflow.UUID).Count(&workflowLogs).Error; err != nil || workflowLogs != 3 {
			t.Fatalf("Yolo workflow logs=%d error=%v", workflowLogs, err)
		}
		if err := store.DB().Table("llm_logs AS logs").Joins("JOIN workflows ON workflows.id=logs.workflow_id").Where("workflows.uuid=? AND logs.source_type='workflow' AND logs.status='completed' AND logs.request_payload IS NOT NULL AND logs.response IS NOT NULL", workflow.UUID).Count(&workflowSnapshots).Error; err != nil || workflowSnapshots != workflowLogs {
			t.Fatalf("Yolo workflow snapshots=%d logs=%d error=%v", workflowSnapshots, workflowLogs, err)
		}
		var imageWorkflowCount int64
		if err := store.DB().Table("workflows").Where("kind=?", agent.WorkflowComicSectionImage).Count(&imageWorkflowCount).Error; err != nil || imageWorkflowCount != 0 {
			t.Fatalf("Yolo created %d duplicate comic image workflows, error=%v", imageWorkflowCount, err)
		}
		var storyboardWorkflowCount int64
		if err := store.DB().Table("workflows").Where("kind=?", agent.WorkflowComicStoryboard).Count(&storyboardWorkflowCount).Error; err != nil || storyboardWorkflowCount != 0 {
			t.Fatalf("Yolo created %d duplicate comic storyboard workflows, error=%v", storyboardWorkflowCount, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
