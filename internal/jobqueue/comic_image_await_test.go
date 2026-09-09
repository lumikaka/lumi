package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/imagegen"
	"lumi/internal/production"
	"lumi/internal/project"
)

type awaitingComicImageProvider struct {
	started, release chan struct{}
	client           imagegen.Client
	err              error
}

func (provider awaitingComicImageProvider) Generate(ctx context.Context, request imagegen.Request) (imagegen.Response, error) {
	close(provider.started)
	select {
	case <-provider.release:
		if provider.err != nil {
			return imagegen.Response{}, provider.err
		}
		return provider.client.Generate(ctx, request)
	case <-ctx.Done():
		return imagegen.Response{}, ctx.Err()
	}
}

func createCoverChat(t *testing.T, env inlineWorkflowTestEnv, model *inlineWorkflowAgentModel) (agent.Thread, agent.Turn) {
	t.Helper()
	if err := env.projects.WithCurrentStore(env.ctx, env.project.UUID, func(store *project.Store) error {
		service := production.NewService(store, nil)
		if _, err := service.CreateSection(env.ctx, env.chapter.UUID, production.CreateSectionInput{Title: "月下启程", StoryboardMD: "小狐狸出发送信。"}); err != nil {
			return err
		}
		section, err := service.CreateSection(env.ctx, env.chapter.UUID, production.CreateSectionInput{
			Title: "月光邮差封面", StoryboardMD: "小狐狸在月光下送信，封面标题为月光邮差。", PageRole: production.PageRoleFrontCover,
		})
		model.sectionUUIDs = []string{section.UUID}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	thread, err := env.agents.CreateThread(env.ctx, env.project.UUID, agent.CreateThreadInput{Title: "生成封面图", ProviderUUID: env.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := env.agents.CreateTurn(env.ctx, env.project.UUID, thread.UUID, agent.CreateTurnInput{InputText: "生成封面图"})
	if err != nil {
		t.Fatal(err)
	}
	return thread, turn
}

func TestChatToolComicImageWaitsInlineAndResumesOnce(t *testing.T) {
	for _, outcome := range []string{"completed", "failed", "cancelled", "worker_cancelled", "parent_aborted", "reconciled"} {
		t.Run(outcome, func(t *testing.T) {
			model := newInlineWorkflowAgentModel()
			close(model.releaseStory)
			images := awaitingComicImageProvider{started: make(chan struct{}), release: make(chan struct{}), client: successfulImageProvider{content: productionPNG(t)}}
			if outcome == "worker_cancelled" {
				images.err = context.Canceled
			}
			env := setupInlineWorkflowTestEnv(t, model)
			if outcome == "failed" {
				images.client = failingImageProvider{}
			}
			env.queue.WithImageClient(images)
			thread, turn := createCoverChat(t, env, model)
			waitTurnStatus(t, env, thread.UUID, agent.TurnWaitingForWorkflow)
			workflow := waitInlineWorkflowKind(t, env, thread.UUID, agent.WorkflowComicSectionImage)
			select {
			case <-images.started:
			case <-time.After(8 * time.Second):
				t.Fatal("image generation did not start")
			}
			if workflow.PresentationMode != string(agent.PresentationInline) || workflow.OriginTurnUUID != turn.UUID || workflow.AwaitStatus != "waiting" || !isUUIDv7(workflow.OriginRunUUID) || !isUUIDv7(workflow.OriginToolCallUUID) || !isUUIDv7(workflow.OriginItemUUID) || len(workflow.Steps) != len(agent.ComicSectionImageStepKeys) {
				t.Fatalf("inline image workflow=%+v", workflow)
			}
			var taskUUID string
			for _, step := range workflow.Steps {
				if step.StepKey == agent.WorkflowStepGenerateSectionImage {
					taskUUID = step.TaskUUID
				}
			}
			if !isUUIDv7(taskUUID) {
				t.Fatalf("missing image task: %+v", workflow.Steps)
			}
			runtime, err := env.queue.runtimeFor(env.project.UUID)
			if err != nil {
				t.Fatal(err)
			}
			// Replaying the exact persisted tool must reuse its task and await.
			request := agent.DomainTaskRequest{
				Kind: KindComicImageGeneration, ResourceUUID: model.sectionUUIDs[0], ChapterUUID: env.chapter.UUID,
				Prompt: "按当前页面脚本生成封面图。",
				Invocation: agent.DomainInvocationContext{Source: agent.InvocationChatTool, PresentationMode: agent.PresentationInline, AwaitCompletion: true,
					ThreadUUID: thread.UUID, TurnUUID: turn.UUID, RunUUID: workflow.OriginRunUUID},
			}
			if err := runtime.sqlDB.QueryRowContext(env.ctx, `SELECT x.uuid,x.idempotency_key FROM workflow_awaits a JOIN agent_tool_executions x ON x.id=a.tool_execution_id JOIN workflows w ON w.id=a.workflow_id WHERE w.uuid=?`, workflow.UUID).Scan(&request.Invocation.ToolExecutionUUID, &request.IdempotencyKey); err != nil {
				t.Fatal(err)
			}
			replayed, err := env.queue.StartDomainTask(env.ctx, env.project.UUID, request)
			if !errors.Is(err, agent.ErrWaitingWorkflow) || replayed.UUID != taskUUID {
				t.Fatalf("image tool replay=%+v err=%v", replayed, err)
			}
			var workflowCount, awaitCount, taskCount, shadowThreads int
			if err := runtime.sqlDB.QueryRowContext(env.ctx, `SELECT (SELECT COUNT(*) FROM workflows),(SELECT COUNT(*) FROM workflow_awaits),(SELECT COUNT(*) FROM production_task_runs),(SELECT COUNT(*) FROM chat_threads WHERE thread_type='workflow')`).Scan(&workflowCount, &awaitCount, &taskCount, &shadowThreads); err != nil {
				t.Fatal(err)
			}
			if workflowCount != 1 || awaitCount != 1 || taskCount != 1 || shadowThreads != 0 {
				t.Fatalf("workflows=%d awaits=%d tasks=%d shadow threads=%d", workflowCount, awaitCount, taskCount, shadowThreads)
			}
			items, err := env.agents.ListItems(env.ctx, env.project.UUID, thread.UUID, "", "", 100)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range items.Items {
				if item.ItemType == "tool_result" || item.ItemType == "assistant_message" {
					t.Fatalf("image task returned before its terminal state: %+v", item)
				}
			}

			terminal := agent.WorkflowCompleted
			switch outcome {
			case "cancelled":
				terminal = agent.WorkflowCancelled
				_, err = env.agents.CancelWorkflow(env.ctx, env.project.UUID, workflow.UUID)
			case "parent_aborted":
				terminal = agent.WorkflowCancelled
				_, err = env.agents.Abort(env.ctx, env.project.UUID, thread.UUID)
			case "worker_cancelled":
				terminal = agent.WorkflowCancelled
				_, err = runtime.sqlDB.ExecContext(env.ctx, `UPDATE production_task_runs SET cancel_requested_at=? WHERE uuid=?`, time.Now().UTC(), taskUUID)
				close(images.release)
			case "reconciled":
				// Simulate an interrupted task whose Workflow terminal projection
				// and Resume were not yet repaired at application open.
				terminal = agent.WorkflowInterrupted
				now := time.Now().UTC()
				_, err = runtime.sqlDB.ExecContext(env.ctx, `UPDATE production_task_runs SET status='interrupted',error_code='unsafe_restart',completed_at=?,updated_at=? WHERE uuid=?`, now, now, taskUUID)
				if err == nil {
					err = reconcileComicImageWorkflows(env.ctx, runtime.sqlDB, runtime.projectID, now)
				}
				if err == nil {
					err = env.agents.ReconcileOnOpen(env.ctx, runtime.store)
				}
			default:
				if outcome == "failed" {
					terminal = agent.WorkflowFailed
				}
				close(images.release)
			}
			if err != nil {
				t.Fatal(err)
			}
			turnStatus := agent.TurnCompleted
			if outcome == "parent_aborted" {
				turnStatus = agent.TurnCancelled
			}
			finished := waitTurnStatus(t, env, thread.UUID, turnStatus)
			if finished.UUID != turn.UUID {
				t.Fatalf("resumed a different turn: %+v", finished)
			}
			// A second open must not insert another await, resume, or reply.
			if err := env.agents.ReconcileOnOpen(env.ctx, runtime.store); err != nil {
				t.Fatal(err)
			}
			workflow, err = env.agents.GetWorkflow(env.ctx, env.project.UUID, workflow.UUID)
			if err != nil || workflow.Status != terminal {
				t.Fatalf("terminal image workflow=%+v err=%v", workflow, err)
			}
			if outcome == "parent_aborted" {
				if workflow.AwaitStatus != "cancelled" {
					t.Fatalf("aborted await=%s", workflow.AwaitStatus)
				}
			} else if workflow.AwaitStatus != "resumed" {
				t.Fatalf("await=%s", workflow.AwaitStatus)
			}
			items, err = env.agents.ListItems(env.ctx, env.project.UUID, thread.UUID, "", "", 100)
			if err != nil {
				t.Fatal(err)
			}
			var results, replies int
			for _, item := range items.Items {
				if item.ItemType == "assistant_message" {
					replies++
				}
				if item.ItemType != "tool_result" {
					continue
				}
				results++
				var payload struct {
					Success bool `json:"success"`
					Data    struct {
						WorkflowUUID string `json:"workflow_uuid"`
						TaskUUID     string `json:"task_uuid"`
						ResourceUUID string `json:"resource_uuid"`
						Status       string `json:"status"`
						Result       struct {
							ImageVariantUUID string `json:"image_variant_uuid"`
						} `json:"result"`
					} `json:"data"`
					Error struct{ Code, Details string } `json:"error"`
				}
				if err := json.Unmarshal([]byte(item.Content), &payload); err != nil || payload.Success != (terminal == agent.WorkflowCompleted) || payload.Data.WorkflowUUID != workflow.UUID || payload.Data.TaskUUID != taskUUID || payload.Data.ResourceUUID != model.sectionUUIDs[0] || payload.Data.Status != terminal {
					t.Fatalf("image terminal result=%s err=%v", item.Content, err)
				}
				if terminal == agent.WorkflowCompleted {
					section, err := production.NewService(runtime.store, nil).GetSection(env.ctx, env.chapter.UUID, model.sectionUUIDs[0])
					if err != nil || section.CurrentImage == nil || !isUUIDv7(payload.Data.Result.ImageVariantUUID) || payload.Data.Result.ImageVariantUUID != section.CurrentImage.UUID {
						t.Fatalf("saved image=%+v terminal=%s err=%v", section, item.Content, err)
					}
				} else if payload.Error.Code == "" {
					t.Fatalf("terminal failure omitted error code: %s", item.Content)
				}
				if outcome == "failed" && !strings.Contains(payload.Error.Details, "InvalidParameter") {
					t.Fatalf("provider diagnostics missing: %s", item.Content)
				}
			}
			wantReplies := 1
			if outcome == "parent_aborted" {
				wantReplies = 0
			}
			if results != wantReplies || replies != wantReplies {
				t.Fatalf("results=%d replies=%d want=%d", results, replies, wantReplies)
			}
			var runCount, requests, resumeJobs int
			if err := runtime.sqlDB.QueryRowContext(env.ctx, `SELECT COUNT(*),COALESCE(SUM(model_request_count),0) FROM chat_runs WHERE thread_id=(SELECT id FROM chat_threads WHERE uuid=?)`, thread.UUID).Scan(&runCount, &requests); err != nil {
				t.Fatal(err)
			}
			if err := runtime.sqlDB.QueryRowContext(env.ctx, `SELECT COUNT(*) FROM river_job WHERE json_extract(args,'$.job_kind')=? AND json_extract(args,'$.thread_uuid')=?`, agent.JobChatResume, thread.UUID).Scan(&resumeJobs); err != nil {
				t.Fatal(err)
			}
			if runCount != 1 || requests != wantReplies+1 || resumeJobs != wantReplies {
				t.Fatalf("runs=%d model requests=%d resume jobs=%d", runCount, requests, resumeJobs)
			}
		})
	}
}

func TestComicImageAwaitCreationFailureRollsBackTask(t *testing.T) {
	model := newInlineWorkflowAgentModel()
	env := setupInlineWorkflowTestEnv(t, model)
	runtime, err := env.queue.runtimeFor(env.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.sqlDB.ExecContext(env.ctx, `CREATE TRIGGER reject_image_await BEFORE INSERT ON workflow_awaits BEGIN SELECT RAISE(ABORT,'injected await failure'); END`); err != nil {
		t.Fatal(err)
	}
	thread, _ := createCoverChat(t, env, model)
	waitTurnStatus(t, env, thread.UUID, agent.TurnCompleted)
	for _, table := range []string{"production_task_runs", "comic_image_generations", "workflows", "workflow_steps", "workflow_events", "workflow_awaits"} {
		var count int64
		if err := runtime.store.DB().Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("partial creation leaked %d %s", count, table)
		}
	}
	var jobs int
	if err := runtime.sqlDB.QueryRowContext(env.ctx, `SELECT COUNT(*) FROM river_job WHERE json_extract(args,'$.task_kind')=?`, KindComicImageGeneration).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("image jobs=%d err=%v", jobs, err)
	}
}

func TestParentWorkflowComicImageDoesNotCreateAnotherWorkflow(t *testing.T) {
	harness := newQueueHarness(t)
	harness.queue.WithImageClient(successfulImageProvider{content: productionPNG(t)})
	ctx := context.Background()
	chapter := harness.createChapter(t, "vol01.ch01")
	runtime := harness.runtime(t)
	section, err := production.NewService(runtime.store, nil).CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "正文页", StoryboardMD: "小狐狸送信。"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.StartDomainTask(ctx, harness.project.UUID, agent.DomainTaskRequest{
		Kind: KindComicImageGeneration, ChapterUUID: chapter.UUID, ResourceUUID: section.UUID,
		IdempotencyKey: "parent-workflow-single-image", Invocation: agent.WorkflowStepInvocationContext(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	for _, table := range []string{"workflows", "workflow_awaits", "chat_threads"} {
		var count int64
		if err := runtime.store.DB().Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("unexpected %s=%d err=%v", table, count, err)
		}
	}
	if _, err := harness.queue.awaitDomainTask(ctx, harness.project.UUID, task, agent.DomainInvocationContext{AwaitCompletion: true}); err == nil || errors.Is(err, agent.ErrWaitingWorkflow) {
		t.Fatalf("missing await silently suspended caller: %v", err)
	}
}

func TestComicImageWorkflowRestoresSavedVariantWithoutRegeneration(t *testing.T) {
	harness := newQueueHarness(t)
	harness.queue.WithImageClient(successfulImageProvider{content: productionPNG(t)})
	ctx := context.Background()
	chapter := harness.createChapter(t, "vol01.ch01")
	runtime := harness.runtime(t)
	service := production.NewService(runtime.store, nil)
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "正文页", StoryboardMD: "小狐狸送信。"})
	if err != nil {
		t.Fatal(err)
	}
	var firstTaskUUID, firstVariantUUID string
	for _, key := range []string{"first-image", "second-image"} {
		task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{IdempotencyKey: key})
		if err != nil {
			t.Fatal(err)
		}
		waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
		section, err = service.GetSection(ctx, chapter.UUID, section.UUID)
		if err != nil || section.CurrentImage == nil {
			t.Fatalf("generated section=%+v err=%v", section, err)
		}
		if key == "first-image" {
			firstTaskUUID, firstVariantUUID = task.UUID, section.CurrentImage.UUID
		}
	}
	if section.CurrentImage.UUID == firstVariantUUID {
		t.Fatal("second generation did not become current")
	}
	// Emulate a crash after saving the image but before writing the Workflow
	// output; a later selected version must not become this task's result.
	if _, err := runtime.sqlDB.ExecContext(ctx, `UPDATE workflow_steps SET output_json='{}' WHERE step_key=? AND workflow_id IN (SELECT workflow_id FROM workflow_steps WHERE task_uuid=?)`, agent.WorkflowStepSaveSectionImage, firstTaskUUID); err != nil {
		t.Fatal(err)
	}
	record, err := getProductionTaskRecord(ctx, runtime.store.DB(), runtime.projectID, firstTaskUUID)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot production.GenerationSnapshot
	if err := json.Unmarshal([]byte(record.InputSnapshot), &snapshot); err != nil {
		t.Fatal(err)
	}
	if err := runtime.generateComicImage(ctx, service, record, snapshot); err != nil {
		t.Fatal(err)
	}
	var restoredUUID string
	if err := runtime.sqlDB.QueryRowContext(ctx, `SELECT json_extract(output_json,'$.image_variant_uuid') FROM workflow_steps WHERE step_key=? AND workflow_id IN (SELECT workflow_id FROM workflow_steps WHERE task_uuid=?)`, agent.WorkflowStepSaveSectionImage, firstTaskUUID).Scan(&restoredUUID); err != nil || restoredUUID != firstVariantUUID {
		t.Fatalf("restored variant=%s want=%s err=%v", restoredUUID, firstVariantUUID, err)
	}
	var variants int
	if err := runtime.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM comic_image_variants`).Scan(&variants); err != nil || variants != 2 {
		t.Fatalf("recovery regenerated images: variants=%d err=%v", variants, err)
	}
}
