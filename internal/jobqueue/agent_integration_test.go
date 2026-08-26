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

type inlineWorkflowAgentModel struct {
	projectUUID, chapterUUID string
	storyStarted             chan struct{}
	releaseStory             chan struct{}
	storyErr                 error
}

func newInlineWorkflowAgentModel() *inlineWorkflowAgentModel {
	return &inlineWorkflowAgentModel{storyStarted: make(chan struct{}, 1), releaseStory: make(chan struct{})}
}

func (*inlineWorkflowAgentModel) Check(context.Context, string, string, string) error { return nil }

func (model *inlineWorkflowAgentModel) Generate(ctx context.Context, request llm.Request, onDelta func(string) error) (llm.Response, error) {
	select {
	case model.storyStarted <- struct{}{}:
	default:
	}
	select {
	case <-model.releaseStory:
	case <-ctx.Done():
		return llm.Response{}, ctx.Err()
	}
	if model.storyErr != nil {
		return llm.Response{}, model.storyErr
	}
	content, _ := json.Marshal(map[string]string{
		"chapter_code": "vol01.ch01", "title": "月光邮差", "content": "小狐狸完成了月光信件的旅程。", "content_format": "txt",
	})
	if onDelta != nil {
		_ = onDelta(string(content))
	}
	return llm.Response{Content: string(content), FinishReason: "stop"}, nil
}

func (model *inlineWorkflowAgentModel) Complete(_ context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	for _, message := range request.Messages {
		if strings.Contains(message.Content, `"workflow_uuid"`) && strings.Contains(message.Content, `"status":"completed"`) {
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "章节 Workflow 已完成，我已读取终态结果并继续回复。"}, FinishReason: "stop"}, nil
		}
		if strings.Contains(message.Content, `"workflow_uuid"`) && strings.Contains(message.Content, `"success":false`) {
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "章节 Workflow 未完成，我已读取结构化终态并向用户说明。"}, FinishReason: "stop"}, nil
		}
	}
	last := ""
	if len(request.Messages) > 0 {
		last = request.Messages[len(request.Messages)-1].Content
	}
	if strings.Contains(last, "发起章节生成") {
		arguments, _ := json.Marshal(map[string]any{
			"url":             "/api/v1/projects/" + model.projectUUID + "/chapters/" + model.chapterUUID + "/generations",
			"method":          "POST",
			"request_body":    map[string]any{"prompt_key": "story_chapter", "prompt": "写出这一章的完整正文。"},
			"response_filter": ".data | {uuid,status}",
		})
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "create-chapter-workflow", Name: "request_api", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "旁路 Chat Run 已完成。"}, FinishReason: "stop"}, nil
}

type inlineWorkflowTestEnv struct {
	ctx      context.Context
	projects *project.Manager
	agents   *agent.Service
	project  project.Summary
	provider provider.Provider
	chapter  story.Chapter
}

func setupInlineWorkflowTestEnv(t *testing.T, model *inlineWorkflowAgentModel) inlineWorkflowTestEnv {
	t.Helper()
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	configured, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/inline-model", APIKey: "inline-secret"})
	if err != nil {
		t.Fatal(err)
	}
	queue := NewManager(providers, model, nil)
	projects := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen)
	agents := agent.NewService(projects, providers, model, queue, nil)
	queue.WithAgentService(agents)
	projects.WithRuntime(queue).WithOpenHook(queue.StartProject).WithOpenHook(agents.ReconcileOnOpen)
	created, err := projects.Create(ctx, "Inline Workflow", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = projects.Close(); _ = app.Close() })
	model.projectUUID = created.UUID
	var chapter story.Chapter
	if err := projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		var createErr error
		chapter, createErr = story.NewService(store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "第一章"})
		return createErr
	}); err != nil {
		t.Fatal(err)
	}
	model.chapterUUID = chapter.UUID
	return inlineWorkflowTestEnv{ctx: ctx, projects: projects, agents: agents, project: created, provider: configured, chapter: chapter}
}

func waitInlineWorkflow(t *testing.T, env inlineWorkflowTestEnv, threadUUID string) agent.Workflow {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		workflows, err := env.agents.ListWorkflows(env.ctx, env.project.UUID)
		if err == nil {
			for _, workflow := range workflows {
				if workflow.Kind == agent.WorkflowStoryChapter && workflow.ThreadUUID == threadUUID {
					return workflow
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("inline workflow was not persisted")
	return agent.Workflow{}
}

func waitTurnStatus(t *testing.T, env inlineWorkflowTestEnv, threadUUID, status string) agent.Turn {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		turns, err := env.agents.ListTurns(env.ctx, env.project.UUID, threadUUID)
		if err == nil && len(turns) > 0 && turns[len(turns)-1].Status == status {
			return turns[len(turns)-1]
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("thread %s did not reach %s", threadUUID, status)
	return agent.Turn{}
}

func TestChatToolWorkflowFailureAndCancellationResumeStructuredToolResults(t *testing.T) {
	for _, test := range []struct {
		name, terminal string
		fail           bool
	}{
		{name: "failed", terminal: agent.WorkflowFailed, fail: true},
		{name: "cancelled", terminal: agent.WorkflowCancelled},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := newInlineWorkflowAgentModel()
			if test.fail {
				model.storyErr = &llm.Error{Code: "test_generation_failed", SafeMessage: "测试生成失败。", Retryable: false}
			}
			env := setupInlineWorkflowTestEnv(t, model)
			thread, err := env.agents.CreateThread(env.ctx, env.project.UUID, agent.CreateThreadInput{Title: "终态对话", ProviderUUID: env.provider.UUID})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := env.agents.CreateTurn(env.ctx, env.project.UUID, thread.UUID, agent.CreateTurnInput{InputText: "请发起章节生成"}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-model.storyStarted:
			case <-time.After(8 * time.Second):
				t.Fatal("chapter Workflow did not start")
			}
			workflow := waitInlineWorkflow(t, env, thread.UUID)
			if test.fail {
				close(model.releaseStory)
			} else if _, err := env.agents.CancelWorkflow(env.ctx, env.project.UUID, workflow.UUID); err != nil {
				t.Fatal(err)
			}
			waitTurnStatus(t, env, thread.UUID, agent.TurnCompleted)
			workflow, err = env.agents.GetWorkflow(env.ctx, env.project.UUID, workflow.UUID)
			if err != nil || workflow.Status != test.terminal {
				t.Fatalf("workflow=%+v err=%v", workflow, err)
			}
			items, err := env.agents.ListItems(env.ctx, env.project.UUID, thread.UUID, "", "", 100)
			if err != nil {
				t.Fatal(err)
			}
			var results, replies int
			for _, item := range items.Items {
				if item.ItemType == "tool_result" {
					results++
					var payload struct {
						Success bool `json:"success"`
						Data    struct {
							Status string `json:"status"`
						} `json:"data"`
						Error struct {
							Code    string `json:"code"`
							Message string `json:"message"`
							Details string `json:"details"`
						} `json:"error"`
					}
					if err := json.Unmarshal([]byte(item.Content), &payload); err != nil || payload.Success || payload.Data.Status != test.terminal || payload.Error.Code == "" || payload.Error.Message != "异步生成未完成。" || payload.Error.Details != "" {
						t.Fatalf("terminal tool result=%s err=%v", item.Content, err)
					}
				}
				if item.ItemType == "assistant_message" && strings.Contains(item.Content, "结构化终态") {
					replies++
				}
			}
			if results != 1 || replies != 1 {
				t.Fatalf("results=%d replies=%d items=%+v", results, replies, items.Items)
			}
			if err := env.projects.WithCurrentStore(env.ctx, env.project.UUID, func(store *project.Store) error {
				var status string
				if err := store.DB().Table("workflow_awaits").Select("status").Where("workflow_id=(SELECT id FROM workflows WHERE uuid=?)", workflow.UUID).Scan(&status).Error; err != nil {
					return err
				}
				if status != "resumed" {
					t.Fatalf("await status=%s", status)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAbortWaitingChatTurnCancelsOwnedWorkflowWithoutResume(t *testing.T) {
	model := newInlineWorkflowAgentModel()
	env := setupInlineWorkflowTestEnv(t, model)
	thread, err := env.agents.CreateThread(env.ctx, env.project.UUID, agent.CreateThreadInput{Title: "取消父 Run", ProviderUUID: env.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := env.agents.CreateTurn(env.ctx, env.project.UUID, thread.UUID, agent.CreateTurnInput{InputText: "请发起章节生成"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.storyStarted:
	case <-time.After(8 * time.Second):
		t.Fatal("chapter Workflow did not start")
	}
	workflow := waitInlineWorkflow(t, env, thread.UUID)
	aborted, err := env.agents.Abort(env.ctx, env.project.UUID, thread.UUID)
	if err != nil || aborted.UUID != turn.UUID || aborted.Status != agent.TurnCancelled {
		t.Fatalf("abort=%+v err=%v", aborted, err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		workflow, err = env.agents.GetWorkflow(env.ctx, env.project.UUID, workflow.UUID)
		if err == nil && workflow.Status == agent.WorkflowCancelled {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if workflow.Status != agent.WorkflowCancelled {
		t.Fatalf("owned workflow=%+v err=%v", workflow, err)
	}
	items, err := env.agents.ListItems(env.ctx, env.project.UUID, thread.UUID, "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items.Items {
		if item.ItemType == "tool_result" || item.ItemType == "assistant_message" {
			t.Fatalf("cancelled parent was resumed by item %+v", item)
		}
	}
	if err := env.projects.WithCurrentStore(env.ctx, env.project.UUID, func(store *project.Store) error {
		var awaitStatus, runStatus string
		if err := store.DB().Table("workflow_awaits").Select("status").Where("workflow_id=(SELECT id FROM workflows WHERE uuid=?)", workflow.UUID).Scan(&awaitStatus).Error; err != nil {
			return err
		}
		if err := store.DB().Table("chat_runs").Select("status").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Scan(&runStatus).Error; err != nil {
			return err
		}
		if awaitStatus != "cancelled" || runStatus != agent.TurnCancelled {
			t.Fatalf("await=%s run=%s", awaitStatus, runStatus)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileRepairsTerminalInlineWorkflowAndResumeIsIdempotent(t *testing.T) {
	model := newInlineWorkflowAgentModel()
	env := setupInlineWorkflowTestEnv(t, model)
	thread, err := env.agents.CreateThread(env.ctx, env.project.UUID, agent.CreateThreadInput{Title: "重启恢复", ProviderUUID: env.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.agents.CreateTurn(env.ctx, env.project.UUID, thread.UUID, agent.CreateTurnInput{InputText: "请发起章节生成"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.storyStarted:
	case <-time.After(8 * time.Second):
		t.Fatal("chapter Workflow did not start")
	}
	workflow := waitInlineWorkflow(t, env, thread.UUID)
	if err := env.projects.WithCurrentStore(env.ctx, env.project.UUID, func(store *project.Store) error {
		now := time.Now().UTC()
		if err := store.DB().Exec(`UPDATE workflow_steps SET status='completed',output_json=?,completed_at=?,updated_at=? WHERE workflow_id=(SELECT id FROM workflows WHERE uuid=?)`, `{"chapter_uuid":"`+env.chapter.UUID+`"}`, now, now, workflow.UUID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`UPDATE workflows SET status='completed',completed_at=?,updated_at=? WHERE uuid=?`, now, now, workflow.UUID).Error; err != nil {
			return err
		}
		// Repeated opens cover both the missing terminal projection and a
		// duplicate attempt to deliver the same unique Resume job.
		if err := env.agents.ReconcileOnOpen(env.ctx, store); err != nil {
			return err
		}
		return env.agents.ReconcileOnOpen(env.ctx, store)
	}); err != nil {
		t.Fatal(err)
	}
	waitTurnStatus(t, env, thread.UUID, agent.TurnCompleted)
	items, err := env.agents.ListItems(env.ctx, env.project.UUID, thread.UUID, "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	var results, replies int
	for _, item := range items.Items {
		if item.ItemType == "tool_result" {
			results++
		}
		if item.ItemType == "assistant_message" && strings.Contains(item.Content, "Workflow 已完成") {
			replies++
		}
	}
	if results != 1 || replies != 1 {
		t.Fatalf("reconcile duplicated output: results=%d replies=%d items=%+v", results, replies, items.Items)
	}
	if err := env.projects.WithCurrentStore(env.ctx, env.project.UUID, func(store *project.Store) error {
		var awaits, resumed int64
		if err := store.DB().Table("workflow_awaits").Where("workflow_id=(SELECT id FROM workflows WHERE uuid=?)", workflow.UUID).Count(&awaits).Error; err != nil {
			return err
		}
		if err := store.DB().Table("workflow_awaits").Where("workflow_id=(SELECT id FROM workflows WHERE uuid=?) AND status='resumed'", workflow.UUID).Count(&resumed).Error; err != nil {
			return err
		}
		if awaits != 1 || resumed != 1 {
			t.Fatalf("awaits=%d resumed=%d", awaits, resumed)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestChatToolChapterGenerationWaitsWithoutShadowThreadAndResumes(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	configured, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/inline-model", APIKey: "inline-secret"})
	if err != nil {
		t.Fatal(err)
	}
	model := newInlineWorkflowAgentModel()
	queue := NewManager(providers, model, nil)
	projects := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen)
	agents := agent.NewService(projects, providers, model, queue, nil)
	queue.WithAgentService(agents)
	projects.WithRuntime(queue).WithOpenHook(queue.StartProject).WithOpenHook(agents.ReconcileOnOpen)
	created, err := projects.Create(ctx, "Inline Workflow", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = projects.Close(); _ = app.Close() })
	model.projectUUID = created.UUID
	var chapter story.Chapter
	if err := projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		var createErr error
		chapter, createErr = story.NewService(store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "第一章"})
		return createErr
	}); err != nil {
		t.Fatal(err)
	}
	model.chapterUUID = chapter.UUID
	thread, err := agents.CreateThread(ctx, created.UUID, agent.CreateThreadInput{Title: "当前对话", ProviderUUID: configured.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := agents.CreateTurn(ctx, created.UUID, thread.UUID, agent.CreateTurnInput{InputText: "请发起章节生成"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.storyStarted:
	case <-time.After(8 * time.Second):
		t.Fatal("chapter Workflow did not start")
	}
	deadline := time.Now().Add(5 * time.Second)
	var turns []agent.Turn
	for time.Now().Before(deadline) {
		turns, err = agents.ListTurns(ctx, created.UUID, thread.UUID)
		if err == nil && len(turns) == 1 && turns[0].Status == agent.TurnWaitingForWorkflow {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(turns) != 1 || turns[0].Status != agent.TurnWaitingForWorkflow {
		t.Fatalf("waiting turn=%+v err=%v", turns, err)
	}
	workflows, err := agents.ListWorkflows(ctx, created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	var inline agent.Workflow
	for _, candidate := range workflows {
		if candidate.Kind == agent.WorkflowStoryChapter && candidate.ThreadUUID == thread.UUID {
			inline = candidate
			break
		}
	}
	if inline.UUID == "" || inline.PresentationMode != string(agent.PresentationInline) || inline.OriginTurnUUID != turn.UUID || inline.OriginToolCallUUID == "" || inline.AwaitStatus != "waiting" {
		t.Fatalf("inline workflow=%+v", inline)
	}
	if err := projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		var threadCount, awaitCount int64
		if err := store.DB().Table("chat_threads").Count(&threadCount).Error; err != nil {
			return err
		}
		if err := store.DB().Table("workflow_awaits").Where("status='waiting'").Count(&awaitCount).Error; err != nil {
			return err
		}
		var owner struct {
			ThreadUUID, TurnUUID, RunUUID, ToolCallUUID, RunStatus, ThreadType string
		}
		if err := store.DB().Raw(`SELECT th.uuid AS thread_uuid,t.uuid AS turn_uuid,r.uuid AS run_uuid,x.tool_call_uuid,r.status AS run_status,th.thread_type
			FROM workflow_awaits a
			JOIN workflows w ON w.id=a.workflow_id
			JOIN chat_threads th ON th.id=a.chat_thread_id
			JOIN chat_turns t ON t.id=a.chat_turn_id
			JOIN chat_runs r ON r.id=a.chat_run_id
			JOIN agent_tool_executions x ON x.id=a.tool_execution_id
			WHERE w.uuid=?`, inline.UUID).Scan(&owner).Error; err != nil {
			return err
		}
		if threadCount != 1 || awaitCount != 1 || owner.ThreadUUID != thread.UUID || owner.TurnUUID != turn.UUID || owner.RunUUID != inline.OriginRunUUID || owner.ToolCallUUID != inline.OriginToolCallUUID || owner.RunStatus != agent.TurnInProgress || owner.ThreadType != agent.ThreadTypeConversation {
			t.Fatalf("threads=%d awaits=%d", threadCount, awaitCount)
		}
		if err := agents.ReconcileOnOpen(ctx, store); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	turns, err = agents.ListTurns(ctx, created.UUID, thread.UUID)
	if err != nil || len(turns) != 1 || turns[0].Status != agent.TurnWaitingForWorkflow {
		t.Fatalf("active await was replayed during reconcile: turns=%+v err=%v", turns, err)
	}

	// The story queue is deliberately blocked. A second Chat Run completing on
	// the single agent queue proves the parent worker was released, not polling.
	sideThread, err := agents.CreateThread(ctx, created.UUID, agent.CreateThreadInput{Title: "旁路对话", ProviderUUID: configured.UUID})
	if err != nil {
		t.Fatal(err)
	}
	sideTurn, err := agents.CreateTurn(ctx, created.UUID, sideThread.UUID, agent.CreateTurnInput{InputText: "旁路消息"})
	if err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		items, listErr := agents.ListTurns(ctx, created.UUID, sideThread.UUID)
		if listErr == nil && len(items) == 1 && items[0].Status == agent.TurnCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	sideTurns, err := agents.ListTurns(ctx, created.UUID, sideThread.UUID)
	if err != nil || len(sideTurns) != 1 || sideTurns[0].UUID != sideTurn.UUID || sideTurns[0].Status != agent.TurnCompleted {
		t.Fatalf("side turn=%+v err=%v", sideTurns, err)
	}

	close(model.releaseStory)
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		turns, err = agents.ListTurns(ctx, created.UUID, thread.UUID)
		if err == nil && len(turns) == 1 && turns[0].Status == agent.TurnCompleted {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(turns) != 1 || turns[0].Status != agent.TurnCompleted {
		t.Fatalf("resumed turn=%+v err=%v", turns, err)
	}
	items, err := agents.ListItems(ctx, created.UUID, thread.UUID, "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	var toolResults, assistants int
	for _, item := range items.Items {
		if item.ItemType == "tool_result" {
			toolResults++
			var result map[string]any
			if json.Unmarshal([]byte(item.Content), &result) != nil || result["success"] != true {
				t.Fatalf("tool result=%s", item.Content)
			}
		}
		if item.ItemType == "assistant_message" && strings.Contains(item.Content, "Workflow 已完成") {
			assistants++
		}
	}
	if toolResults != 1 || assistants != 1 {
		t.Fatalf("tool results=%d assistants=%d items=%+v", toolResults, assistants, items.Items)
	}
	if err := projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		var awaitStatus, runStatus, threadStatus string
		if err := store.DB().Table("workflow_awaits").Select("status").Where("workflow_id=(SELECT id FROM workflows WHERE uuid=?)", inline.UUID).Scan(&awaitStatus).Error; err != nil {
			return err
		}
		if err := store.DB().Table("chat_runs").Select("status").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Scan(&runStatus).Error; err != nil {
			return err
		}
		if err := store.DB().Table("chat_threads").Select("status").Where("uuid=?", thread.UUID).Scan(&threadStatus).Error; err != nil {
			return err
		}
		if awaitStatus != "resumed" || runStatus != agent.TurnCompleted || threadStatus != agent.ThreadIdle {
			t.Fatalf("await=%s run=%s thread=%s", awaitStatus, runStatus, threadStatus)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
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
