package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/production"
)

func prepareBatchChat(t *testing.T) (inlineWorkflowTestEnv, *inlineWorkflowAgentModel, agent.Thread) {
	t.Helper()
	model := newInlineWorkflowAgentModel()
	env := setupInlineWorkflowTestEnv(t, model)
	runtime, err := env.queue.runtimeFor(env.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	source, err := production.NewService(runtime.store, nil).CreatePremiseSource(env.ctx, production.CreateSourceInput{SourceText: "天空城的黄昏灯塔", StyleSnapshot: "纸雕风格", SourceType: "generated", Parameters: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	model.premiseSourceUUID = source.UUID
	thread, err := env.agents.CreateThread(env.ctx, env.project.UUID, agent.CreateThreadInput{Title: "批量设定", ProviderUUID: env.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	return env, model, thread
}

func beginBatchChat(t *testing.T, env inlineWorkflowTestEnv, thread agent.Thread) agent.Workflow {
	t.Helper()
	if _, err := env.agents.CreateTurn(env.ctx, env.project.UUID, thread.UUID, agent.CreateTurnInput{InputText: "发起批量设定"}); err != nil {
		t.Fatal(err)
	}
	waitTurnStatus(t, env, thread.UUID, agent.TurnWaitingForWorkflow)
	return waitInlineWorkflowKind(t, env, thread.UUID, agent.WorkflowPremiseBatch)
}

func waitBatchStatus(t *testing.T, env inlineWorkflowTestEnv, uuid, status string) agent.Workflow {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		w, err := env.agents.GetWorkflow(env.ctx, env.project.UUID, uuid)
		if err == nil && w.Status == status {
			return w
		}
		time.Sleep(20 * time.Millisecond)
	}
	w, err := env.agents.GetWorkflow(env.ctx, env.project.UUID, uuid)
	t.Fatalf("wanted %s, workflow=%+v error=%v", status, w, err)
	return w
}

func TestPremiseBatchChatLifecycle(t *testing.T) {
	for _, outcome := range []string{"completed", "setting_failed", "setting_cancelled", "parent_aborted", "breakdown_failed", "breakdown_cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			env, model, thread := prepareBatchChat(t)
			images := awaitingComicImageProvider{started: make(chan struct{}), release: make(chan struct{}), client: successfulImageProvider{content: productionPNG(t)}}
			if outcome == "setting_failed" {
				images.client = failingImageProvider{}
			}
			if outcome == "breakdown_failed" {
				model.storyErr = productionError("invalid_breakdown", "无法识别设定", false)
			}
			env.queue.WithImageClient(images)
			workflow := beginBatchChat(t, env, thread)
			if len(workflow.Steps) != 2 || workflow.PresentationMode != "inline" || workflow.AwaitStatus != "waiting" || workflow.Steps[1].TaskUUID != "" {
				t.Fatalf("invalid initial graph: %+v", workflow)
			}
			select {
			case <-images.started:
			case <-time.After(8 * time.Second):
				t.Fatal("setting did not start")
			}
			runtime, _ := env.queue.runtimeFor(env.project.UUID)
			var invocation agent.DomainInvocationContext
			invocation.Source, invocation.PresentationMode, invocation.AwaitCompletion = agent.InvocationChatTool, agent.PresentationInline, true
			invocation.ThreadUUID, invocation.TurnUUID, invocation.RunUUID = thread.UUID, workflow.OriginTurnUUID, workflow.OriginRunUUID
			var key string
			if err := runtime.sqlDB.QueryRow(`SELECT x.uuid,x.idempotency_key FROM workflow_awaits a JOIN workflows w ON w.id=a.workflow_id JOIN agent_tool_executions x ON x.id=a.tool_execution_id WHERE w.uuid=?`, workflow.UUID).Scan(&invocation.ToolExecutionUUID, &key); err != nil {
				t.Fatal(err)
			}
			request := agent.DomainTaskRequest{Kind: KindPremiseSettingGeneration, ResourceUUID: model.premiseSourceUUID, Prompt: "批量生成灯塔设定", IdempotencyKey: key, Invocation: invocation}
			replayed, err := env.queue.StartDomainTask(env.ctx, env.project.UUID, request)
			if !errors.Is(err, agent.ErrWaitingWorkflow) || replayed.UUID != workflow.Steps[0].TaskUUID {
				t.Fatalf("replay=%+v err=%v", replayed, err)
			}
			request.Prompt = "different input"
			if _, err := env.queue.StartDomainTask(env.ctx, env.project.UUID, request); err == nil || errors.Is(err, agent.ErrWaitingWorkflow) {
				t.Fatalf("changed input accepted: %v", err)
			}
			if outcome == "setting_cancelled" {
				if _, err := env.agents.CancelWorkflow(env.ctx, env.project.UUID, workflow.UUID); err != nil {
					t.Fatal(err)
				}
			}
			if outcome == "parent_aborted" {
				if _, err := env.agents.Abort(env.ctx, env.project.UUID, thread.UUID); err != nil {
					t.Fatal(err)
				}
			}
			close(images.release)
			if outcome == "setting_failed" || outcome == "setting_cancelled" || outcome == "parent_aborted" {
				want := agent.WorkflowCancelled
				if outcome == "setting_failed" {
					want = agent.WorkflowFailed
				}
				workflow = waitBatchStatus(t, env, workflow.UUID, want)
				if workflow.Steps[1].TaskUUID != "" {
					t.Fatal("failed/cancelled setting started breakdown")
				}
				if outcome == "parent_aborted" {
					waitTurnStatus(t, env, thread.UUID, agent.TurnCancelled)
					return
				}
			} else {
				select {
				case <-model.storyStarted:
				case <-time.After(8 * time.Second):
					t.Fatal("breakdown did not start automatically")
				}
				waitTurnStatus(t, env, thread.UUID, agent.TurnWaitingForWorkflow)
				workflow, err = env.agents.GetWorkflow(env.ctx, env.project.UUID, workflow.UUID)
				if err != nil {
					t.Fatal(err)
				}
				if workflow.Steps[0].Status != "completed" || workflow.Steps[1].TaskUUID == "" || workflow.CurrentStepKey != agent.WorkflowStepBreakdownAssets {
					t.Fatalf("invalid stage transition: %+v", workflow)
				}
				var output struct {
					SettingUUID string `json:"setting_image_uuid"`
				}
				if err := json.Unmarshal(workflow.Steps[0].Output, &output); err != nil {
					t.Fatal(err)
				}
				if workflow.Steps[1].ResourceUUID != output.SettingUUID {
					t.Fatal("breakdown selected a different image")
				}
				var raw string
				if err := runtime.sqlDB.QueryRow(`SELECT input_snapshot FROM production_task_runs WHERE uuid=?`, workflow.Steps[1].TaskUUID).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var frozen production.GenerationSnapshot
				if err := json.Unmarshal([]byte(raw), &frozen); err != nil {
					t.Fatal(err)
				}
				if frozen.Model != "test/inline-model" || frozen.SourceUUID != model.premiseSourceUUID {
					t.Fatalf("wrong breakdown snapshot: %+v", frozen)
				}
				if outcome == "breakdown_cancelled" {
					if _, err := env.agents.CancelWorkflow(env.ctx, env.project.UUID, workflow.UUID); err != nil {
						t.Fatal(err)
					}
				}
				close(model.releaseStory)
				want := agent.WorkflowCompleted
				if outcome == "breakdown_failed" {
					want = agent.WorkflowFailed
				}
				if outcome == "breakdown_cancelled" {
					want = agent.WorkflowCancelled
				}
				workflow = waitBatchStatus(t, env, workflow.UUID, want)
			}
			waitTurnStatus(t, env, thread.UUID, agent.TurnCompleted)
			var resumes int
			if err := runtime.sqlDB.QueryRow(`SELECT count(*) FROM workflow_awaits a JOIN workflows w ON w.id=a.workflow_id WHERE w.uuid=? AND a.status='resumed'`, workflow.UUID).Scan(&resumes); err != nil || resumes != 1 {
				t.Fatalf("resumes=%d err=%v", resumes, err)
			}
			var result string
			if err := runtime.sqlDB.QueryRow(`SELECT x.result_json FROM agent_tool_executions x JOIN workflow_awaits a ON a.tool_execution_id=x.id JOIN workflows w ON w.id=a.workflow_id WHERE w.uuid=?`, workflow.UUID).Scan(&result); err != nil {
				t.Fatal(err)
			}
			if outcome == "completed" {
				if !strings.Contains(result, "premise_asset_uuids") || !strings.Contains(result, "setting_image_uuid") {
					t.Fatalf("missing batch result: %s", result)
				}
				var selected int
				if err := runtime.sqlDB.QueryRow(`SELECT count(*) FROM premise_profiles WHERE current_setting_image_id IS NOT NULL`).Scan(&selected); err != nil || selected != 0 {
					t.Fatalf("batch changed selection: %d %v", selected, err)
				}
				trajectory, err := env.agents.ListTrajectory(env.ctx, env.project.UUID, thread.UUID, "", "", "", 100)
				if err != nil {
					t.Fatal(err)
				}
				calls := 0
				for _, request := range trajectory.ModelRequests {
					if request.Scenario == KindPremiseSettingGeneration || request.Scenario == KindPremiseAssetBreakdown {
						calls++
						if len(request.WorkflowOrigins) != 1 || request.WorkflowOrigins[0].UUID != workflow.UUID {
							t.Fatalf("missing origin: %+v", request)
						}
						if _, err := env.agents.ListTrajectory(env.ctx, env.project.UUID, thread.UUID, "", "", request.UUID, 1); err != nil {
							t.Fatalf("request anchor failed: %v", err)
						}
					}
				}
				if calls != 2 {
					t.Fatalf("batch model requests missing from trajectory: %d", calls)
				}
				if _, err := env.agents.ListTrajectory(env.ctx, env.project.UUID, thread.UUID, "", "", workflow.UUID, 1); err != nil {
					t.Fatalf("workflow anchor failed: %v", err)
				}
			} else {
				if !strings.Contains(result, `"data":null`) || !strings.Contains(result, `"success":false`) {
					t.Fatalf("invalid failure envelope: %s", result)
				}
				// Retry the same failed stage, without creating another graph or
				// re-running a successful setting task.
				env.queue.WithImageClient(successfulImageProvider{content: productionPNG(t)})
				env.queue.llm = &recordingBreakdownProvider{}
				firstTask := workflow.Steps[0].TaskUUID
				if _, err := env.agents.RetryWorkflow(env.ctx, env.project.UUID, workflow.UUID); err != nil {
					t.Fatal(err)
				}
				workflow = waitBatchStatus(t, env, workflow.UUID, agent.WorkflowCompleted)
				if workflow.Steps[0].TaskUUID != firstTask {
					t.Fatal("retry replaced setting task")
				}
				var count int
				if err := runtime.sqlDB.QueryRow(`SELECT count(*) FROM production_task_runs`).Scan(&count); err != nil || count != 2 {
					t.Fatalf("retry duplicated tasks=%d err=%v", count, err)
				}
			}
		})
	}
}

func TestPremiseBatchCreationRollback(t *testing.T) {
	env, _, thread := prepareBatchChat(t)
	runtime, _ := env.queue.runtimeFor(env.project.UUID)
	if _, err := runtime.sqlDB.Exec(`CREATE TRIGGER reject_premise_await BEFORE INSERT ON workflow_awaits BEGIN SELECT RAISE(ABORT,'injected await failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := env.agents.CreateTurn(env.ctx, env.project.UUID, thread.UUID, agent.CreateTurnInput{InputText: "发起批量设定"}); err != nil {
		t.Fatal(err)
	}
	waitTurnStatus(t, env, thread.UUID, agent.TurnCompleted)
	for _, table := range []string{"workflows", "workflow_steps", "workflow_awaits", "production_task_runs", "premise_generation_steps"} {
		var count int64
		if err := runtime.store.DB().Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("leaked %s=%d err=%v", table, count, err)
		}
	}
	var count int
	if err := runtime.sqlDB.QueryRow(`SELECT count(*) FROM river_job WHERE json_extract(args,'$.task_kind')=?`, KindPremiseSettingGeneration).Scan(&count); err != nil || count != 0 {
		t.Fatalf("leaked jobs=%d err=%v", count, err)
	}
}

func TestPremiseBatchOnlyForChat(t *testing.T) {
	for _, invocation := range []agent.DomainInvocationContext{agent.DirectUIInvocationContext(), agent.WorkflowStepInvocationContext(""), agent.ExternalInvocationContext("01a08b4f-2c90-7d27-9a41-ad411cec0f1e")} {
		h := newQueueHarness(t)
		h.queue.WithImageClient(successfulImageProvider{content: productionPNG(t)})
		runtime := h.runtime(t)
		source, err := production.NewService(runtime.store, nil).CreatePremiseSource(context.Background(), production.CreateSourceInput{SourceText: "灯塔", StyleSnapshot: "纸雕", SourceType: "generated", Parameters: map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		task, err := h.queue.StartDomainTask(context.Background(), h.project.UUID, agent.DomainTaskRequest{Kind: KindPremiseSettingGeneration, ResourceUUID: source.UUID, IdempotencyKey: "standalone-setting", Invocation: invocation})
		if err != nil {
			t.Fatal(err)
		}
		waitProductionStatus(t, h.queue, h.project.UUID, task.UUID, StatusCompleted)
		var count int
		if err := runtime.sqlDB.QueryRow(`SELECT count(*) FROM workflows`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unexpected workflow: %d %v", count, err)
		}
		if err := runtime.sqlDB.QueryRow(`SELECT count(*) FROM production_task_runs WHERE kind=?`, KindPremiseAssetBreakdown).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unexpected breakdown: %d %v", count, err)
		}
	}
}

func TestPremiseBatchStageTransitionRollsBackAndReplays(t *testing.T) {
	env, model, thread := prepareBatchChat(t)
	images := awaitingComicImageProvider{started: make(chan struct{}), release: make(chan struct{}), client: successfulImageProvider{content: productionPNG(t)}}
	env.queue.WithImageClient(images)
	w := beginBatchChat(t, env, thread)
	select {
	case <-images.started:
	case <-time.After(8 * time.Second):
		t.Fatal("setting did not start")
	}
	runtime, _ := env.queue.runtimeFor(env.project.UUID)
	if _, err := runtime.sqlDB.Exec(`CREATE TRIGGER reject_breakdown BEFORE INSERT ON production_task_runs WHEN NEW.kind='premise_asset_breakdown' BEGIN SELECT RAISE(ABORT,'injected breakdown failure'); END`); err != nil {
		t.Fatal(err)
	}
	close(images.release)
	waitProductionStatus(t, env.queue, env.project.UUID, w.Steps[0].TaskUUID, StatusQueued)
	var count int
	if err := runtime.sqlDB.QueryRow(`SELECT count(*) FROM production_task_runs WHERE kind=?`, KindPremiseAssetBreakdown).Scan(&count); err != nil || count != 0 {
		t.Fatalf("leaked breakdown=%d err=%v", count, err)
	}
	if err := runtime.sqlDB.QueryRow(`SELECT count(*) FROM river_job WHERE json_extract(args,'$.task_kind')=?`, KindPremiseAssetBreakdown).Scan(&count); err != nil || count != 0 {
		t.Fatalf("leaked job=%d err=%v", count, err)
	}
	current, err := env.agents.GetWorkflow(env.ctx, env.project.UUID, w.UUID)
	if err != nil || current.Steps[1].TaskUUID != "" || current.AwaitStatus != "waiting" {
		t.Fatalf("partial graph: %+v %v", current, err)
	}
	if _, err := runtime.sqlDB.Exec(`DROP TRIGGER reject_breakdown`); err != nil {
		t.Fatal(err)
	}
	// Replay the durable completion after the failed transaction. The image
	// has already been stored and must not be generated or selected again.
	record, err := getProductionTaskRecord(env.ctx, runtime.store.DB(), runtime.projectID, w.Steps[0].TaskUUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.completeProduction(env.ctx, record); err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.storyStarted:
	case <-time.After(8 * time.Second):
		t.Fatal("replayed transition did not start breakdown")
	}
	if err := runtime.completeProduction(env.ctx, record); err != nil {
		t.Fatal(err)
	}
	close(model.releaseStory)
	waitBatchStatus(t, env, w.UUID, agent.WorkflowCompleted)
	waitTurnStatus(t, env, thread.UUID, agent.TurnCompleted)
	if err := runtime.completeProduction(env.ctx, record); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`SELECT count(*) FROM production_task_runs WHERE kind='premise_asset_breakdown'`, `SELECT count(*) FROM premise_setting_images`, `SELECT count(*) FROM workflow_awaits WHERE status='resumed'`} {
		if err := runtime.sqlDB.QueryRow(q).Scan(&count); err != nil || count != 1 {
			t.Fatalf("completion replay count=%d err=%v query=%s", count, err, q)
		}
	}
}

func TestPremiseBatchReopensDuringBreakdown(t *testing.T) {
	env, model, thread := prepareBatchChat(t)
	w := beginBatchChat(t, env, thread)
	select {
	case <-model.storyStarted:
	case <-time.After(8 * time.Second):
		t.Fatal("breakdown did not start")
	}
	before, err := env.agents.GetWorkflow(env.ctx, env.project.UUID, w.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.projects.CloseProject(env.ctx, env.project.UUID); err != nil {
		t.Fatal(err)
	}
	close(model.releaseStory)
	if _, err := env.projects.OpenRecent(env.ctx, env.project.UUID); err != nil {
		t.Fatal(err)
	}
	after := waitBatchStatus(t, env, w.UUID, agent.WorkflowCompleted)
	waitTurnStatus(t, env, thread.UUID, agent.TurnCompleted)
	for index := range before.Steps {
		if before.Steps[index].TaskUUID != after.Steps[index].TaskUUID {
			t.Fatal("restart replaced persisted task")
		}
	}
}

func TestPremiseBatchesUseOwnImagesWithConcurrentSources(t *testing.T) {
	env, model, firstThread := prepareBatchChat(t)
	runtime, _ := env.queue.runtimeFor(env.project.UUID)
	service := production.NewService(runtime.store, nil)
	secondSource, err := service.CreatePremiseSource(env.ctx, production.CreateSourceInput{SourceText: "另一批灯塔", StyleSnapshot: "纸雕", SourceType: "generated", Parameters: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	secondThread, err := env.agents.CreateThread(env.ctx, env.project.UUID, agent.CreateThreadInput{Title: "第二批", ProviderUUID: env.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	first := beginBatchChat(t, env, firstThread)
	select {
	case <-model.storyStarted:
	case <-time.After(8 * time.Second):
		t.Fatal("first breakdown did not start")
	}
	if _, err := env.agents.CreateTurn(env.ctx, env.project.UUID, secondThread.UUID, agent.CreateTurnInput{InputText: "发起批量设定 " + secondSource.UUID}); err != nil {
		t.Fatal(err)
	}
	waitTurnStatus(t, env, secondThread.UUID, agent.TurnWaitingForWorkflow)
	second := waitInlineWorkflowKind(t, env, secondThread.UUID, agent.WorkflowPremiseBatch)
	// Both workflows are accepted before either breakdown finishes.
	close(model.releaseStory)
	first = waitBatchStatus(t, env, first.UUID, agent.WorkflowCompleted)
	second = waitBatchStatus(t, env, second.UUID, agent.WorkflowCompleted)
	if first.Steps[1].ResourceUUID == second.Steps[1].ResourceUUID {
		t.Fatal("two batches consumed the same setting image")
	}
	for _, w := range []agent.Workflow{first, second} {
		var sourceUUID string
		if err := runtime.sqlDB.QueryRow(`SELECT sources.uuid FROM premise_setting_images settings JOIN premise_sources sources ON sources.id=settings.source_id WHERE settings.uuid=?`, w.Steps[1].ResourceUUID).Scan(&sourceUUID); err != nil {
			t.Fatal(err)
		}
		if sourceUUID != w.Steps[0].ResourceUUID {
			t.Fatal("breakdown crossed source batches")
		}
		var output struct {
			UUIDs []string `json:"premise_asset_uuids"`
		}
		if err := json.Unmarshal(w.Steps[1].Output, &output); err != nil || len(output.UUIDs) != 1 {
			t.Fatalf("batch skipped existing assets: %+v %v", output, err)
		}
	}
	waitTurnStatus(t, env, firstThread.UUID, agent.TurnCompleted)
	waitTurnStatus(t, env, secondThread.UUID, agent.TurnCompleted)
}
