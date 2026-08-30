package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type inlineYoloFixture struct {
	Harness   *agentHarness
	Thread    Thread
	Turn      Turn
	Context   toolContext
	Execution toolExecutionRecord
	Workflow  Workflow
}

func newInlineYoloFixture(t *testing.T) inlineYoloFixture {
	t.Helper()
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "在当前对话启动 YOLO"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.claimRun(ctx, harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	tc.ToolMode, err = harness.service.loadRunToolMode(ctx, harness.store, tc)
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{
		"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/workflows",
		"request_body":    map[string]any{"story_prompt": "一只小狐狸替月亮送信。"},
		"response_filter": ".data | {uuid,thread_uuid,presentation_mode,kind,title,status,current_step_key,steps}",
	}
	raw, _ := json.Marshal(args)
	execution, replay, completed, err := harness.service.persistToolIntent(ctx, harness.store, tc, "inline-yolo-fixture", "request_api", string(raw))
	if err != nil || completed || replay != nil {
		t.Fatalf("persist inline YOLO Tool: execution=%+v completed=%v replay=%s err=%v", execution, completed, replay, err)
	}
	workflow, err := harness.service.CreateYoloWorkflow(ctx, harness.project.UUID, CreateYoloInput{
		Title: "月光邮差", StoryPrompt: "一只小狐狸替月亮送信。", ProviderUUID: harness.provider.UUID,
		IdempotencyKey: "inline-yolo-fixture", Invocation: chatToolInvocationContext(tc, execution),
	})
	if !errors.Is(err, ErrWaitingWorkflow) {
		t.Fatalf("inline YOLO did not wait: workflow=%+v err=%v", workflow, err)
	}
	return inlineYoloFixture{Harness: harness, Thread: thread, Turn: turn, Context: tc, Execution: execution, Workflow: workflow}
}

func countAgentJobs(jobs []JobSpec, kind string) int {
	count := 0
	for _, job := range jobs {
		if job.JobKind == kind {
			count++
		}
	}
	return count
}

func yoloToolResultForTurn(t *testing.T, fixture inlineYoloFixture) map[string]any {
	t.Helper()
	items, err := fixture.Harness.service.ListItems(context.Background(), fixture.Harness.project.UUID, fixture.Thread.UUID, "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	for index := len(items.Items) - 1; index >= 0; index-- {
		item := items.Items[index]
		if item.ItemType != "tool_result" || item.ToolName != "request_api" {
			continue
		}
		var result map[string]any
		if json.Unmarshal([]byte(item.Content), &result) == nil {
			return result
		}
	}
	t.Fatal("YOLO terminal Tool Result was not persisted")
	return nil
}

func assertYoloTerminalSummary(t *testing.T, fixture inlineYoloFixture, result map[string]any, status string, success bool) {
	t.Helper()
	if result["success"] != success {
		t.Fatalf("success=%v result=%+v", success, result)
	}
	data, _ := result["data"].(map[string]any)
	if data["workflow_uuid"] != fixture.Workflow.UUID || data["thread_uuid"] != fixture.Thread.UUID || data["presentation_mode"] != string(PresentationInline) || data["kind"] != WorkflowYolo || data["status"] != status {
		t.Fatalf("terminal data=%+v", data)
	}
	if currentStepKey, _ := data["current_step_key"].(string); currentStepKey == "" {
		t.Fatalf("terminal current_step_key is empty: %+v", data)
	}
	steps, _ := data["steps"].([]any)
	if len(steps) != len(YoloStepKeys) {
		t.Fatalf("terminal steps=%+v", steps)
	}
	for index, raw := range steps {
		step, _ := raw.(map[string]any)
		if step["step_key"] != YoloStepKeys[index] || intArg(step, "position") != int64(index+1) || !isUUIDv7(stringArg(step, "uuid")) {
			t.Fatalf("step[%d]=%+v", index, step)
		}
	}
}

func TestInlineYoloTerminalTransitionsResumeOriginalRunExactlyOnce(t *testing.T) {
	t.Run("completed", func(t *testing.T) {
		fixture := newInlineYoloFixture(t)
		ctx := context.Background()
		var workflow workflowRecord
		if err := fixture.Harness.store.DB().Where("uuid=?", fixture.Workflow.UUID).First(&workflow).Error; err != nil {
			t.Fatal(err)
		}
		var finalStep workflowStepRecord
		if err := fixture.Harness.store.DB().Where("workflow_id=?", workflow.ID).Order("position DESC").First(&finalStep).Error; err != nil {
			t.Fatal(err)
		}
		if err := fixture.Harness.store.DB().Exec(`UPDATE workflow_steps SET status=CASE WHEN id=? THEN 'running' ELSE 'completed' END WHERE workflow_id=?`, finalStep.ID, workflow.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := fixture.Harness.store.DB().Model(&workflowRecord{}).Where("id=?", workflow.ID).Updates(map[string]any{"status": WorkflowRunning, "current_step_key": finalStep.StepKey}).Error; err != nil {
			t.Fatal(err)
		}
		chapterUUID, _ := newUUIDv7()
		sectionUUID, _ := newUUIDv7()
		output := map[string]any{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID}
		if err := fixture.Harness.service.completeWorkflowStep(ctx, fixture.Harness.store, workflow, finalStep, fixture.Thread.UUID, output); err != nil {
			t.Fatal(err)
		}
		if err := fixture.Harness.service.completeWorkflowStep(ctx, fixture.Harness.store, workflow, finalStep, fixture.Thread.UUID, output); err != nil {
			t.Fatal(err)
		}
		if got := countAgentJobs(fixture.Harness.queue.jobs, JobChatResume); got != 1 {
			t.Fatalf("Chat Resume jobs=%d jobs=%+v", got, fixture.Harness.queue.jobs)
		}
		beforeResume, err := fixture.Harness.service.ListItems(ctx, fixture.Harness.project.UUID, fixture.Thread.UUID, "", "", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range beforeResume.Items {
			if item.ItemType == "assistant_message" {
				t.Fatalf("inline YOLO wrote a dedicated completion message: %+v", item)
			}
		}
		if err := fixture.Harness.execute(t, fixture.Thread.UUID, fixture.Turn.UUID, JobChatResume); err != nil {
			t.Fatal(err)
		}
		result := yoloToolResultForTurn(t, fixture)
		assertYoloTerminalSummary(t, fixture, result, WorkflowCompleted, true)
		current, err := fixture.Harness.service.GetWorkflow(ctx, fixture.Harness.project.UUID, fixture.Workflow.UUID)
		if err != nil || current.AwaitStatus != "resumed" {
			t.Fatalf("workflow=%+v err=%v", current, err)
		}
	})

	t.Run("failed", func(t *testing.T) {
		fixture := newInlineYoloFixture(t)
		ctx := context.Background()
		var workflow workflowRecord
		if err := fixture.Harness.store.DB().Where("uuid=?", fixture.Workflow.UUID).First(&workflow).Error; err != nil {
			t.Fatal(err)
		}
		var step workflowStepRecord
		if err := fixture.Harness.store.DB().Where("workflow_id=?", workflow.ID).Order("position").First(&step).Error; err != nil {
			t.Fatal(err)
		}
		if err := fixture.Harness.store.DB().Model(&workflowStepRecord{}).Where("id=?", step.ID).Update("status", "running").Error; err != nil {
			t.Fatal(err)
		}
		if err := fixture.Harness.store.DB().Model(&workflowRecord{}).Where("id=?", workflow.ID).Updates(map[string]any{"status": WorkflowRunning, "current_step_key": step.StepKey}).Error; err != nil {
			t.Fatal(err)
		}
		cause := domainError(CodeProvider, "不得泄露的供应商错误", "secret-provider-payload", nil)
		if err := fixture.Harness.service.failWorkflowStep(ctx, fixture.Harness.store, workflow, step, fixture.Thread.UUID, cause); err != nil {
			t.Fatal(err)
		}
		if err := fixture.Harness.service.failWorkflowStep(ctx, fixture.Harness.store, workflow, step, fixture.Thread.UUID, cause); err != nil {
			t.Fatal(err)
		}
		if got := countAgentJobs(fixture.Harness.queue.jobs, JobChatResume); got != 1 {
			t.Fatalf("Chat Resume jobs=%d jobs=%+v", got, fixture.Harness.queue.jobs)
		}
		if err := fixture.Harness.execute(t, fixture.Thread.UUID, fixture.Turn.UUID, JobChatResume); err != nil {
			t.Fatal(err)
		}
		result := yoloToolResultForTurn(t, fixture)
		assertYoloTerminalSummary(t, fixture, result, WorkflowFailed, false)
		encoded, _ := json.Marshal(result)
		if strings.Contains(string(encoded), "secret-provider-payload") || strings.Contains(string(encoded), "不得泄露") {
			t.Fatalf("unsafe failure detail leaked: %s", encoded)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		fixture := newInlineYoloFixture(t)
		ctx := context.Background()
		if _, err := fixture.Harness.service.CancelWorkflow(ctx, fixture.Harness.project.UUID, fixture.Workflow.UUID); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.Harness.service.CancelWorkflow(ctx, fixture.Harness.project.UUID, fixture.Workflow.UUID); err != nil {
			t.Fatal(err)
		}
		if got := countAgentJobs(fixture.Harness.queue.jobs, JobChatResume); got != 1 {
			t.Fatalf("Chat Resume jobs=%d jobs=%+v", got, fixture.Harness.queue.jobs)
		}
		if err := fixture.Harness.execute(t, fixture.Thread.UUID, fixture.Turn.UUID, JobChatResume); err != nil {
			t.Fatal(err)
		}
		assertYoloTerminalSummary(t, fixture, yoloToolResultForTurn(t, fixture), WorkflowCancelled, false)
	})
}

func TestInlineYoloDoesNotReviveCancelledParentRun(t *testing.T) {
	fixture := newInlineYoloFixture(t)
	ctx := context.Background()
	aborted, err := fixture.Harness.service.Abort(ctx, fixture.Harness.project.UUID, fixture.Thread.UUID)
	if err != nil || aborted.Status != TurnCancelled {
		t.Fatalf("aborted=%+v err=%v", aborted, err)
	}
	if _, err := fixture.Harness.service.CancelWorkflow(ctx, fixture.Harness.project.UUID, fixture.Workflow.UUID); err != nil {
		t.Fatal(err)
	}
	if got := countAgentJobs(fixture.Harness.queue.jobs, JobChatResume); got != 0 {
		t.Fatalf("cancelled parent received %d Chat Resume jobs: %+v", got, fixture.Harness.queue.jobs)
	}
	var awaitStatus, runStatus string
	if err := fixture.Harness.store.DB().Raw(`SELECT a.status,r.status FROM workflow_awaits a JOIN chat_runs r ON r.id=a.chat_run_id WHERE a.workflow_id=(SELECT id FROM workflows WHERE uuid=?)`, fixture.Workflow.UUID).Row().Scan(&awaitStatus, &runStatus); err != nil {
		t.Fatal(err)
	}
	if awaitStatus != "cancelled" || runStatus != TurnCancelled {
		t.Fatalf("await=%s run=%s", awaitStatus, runStatus)
	}
}

func TestInlineYoloTerminalDoesNotQueueRunWhenOnlyParentTurnIsCancelled(t *testing.T) {
	fixture := newInlineYoloFixture(t)
	ctx := context.Background()
	now := fixture.Harness.service.now().UTC()
	if err := fixture.Harness.store.DB().Model(&turnRecord{}).Where("id=?", fixture.Context.Turn.ID).Updates(map[string]any{
		"status": TurnCancelled, "cancel_requested_at": now, "completed_at": now, "updated_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Harness.service.CancelWorkflow(ctx, fixture.Harness.project.UUID, fixture.Workflow.UUID); err != nil {
		t.Fatal(err)
	}
	if got := countAgentJobs(fixture.Harness.queue.jobs, JobChatResume); got != 0 {
		t.Fatalf("cancelled parent Turn received %d Chat Resume jobs: %+v", got, fixture.Harness.queue.jobs)
	}
	var awaitStatus, runStatus string
	if err := fixture.Harness.store.DB().Raw(`SELECT a.status,r.status FROM workflow_awaits a JOIN chat_runs r ON r.id=a.chat_run_id WHERE a.workflow_id=(SELECT id FROM workflows WHERE uuid=?)`, fixture.Workflow.UUID).Row().Scan(&awaitStatus, &runStatus); err != nil {
		t.Fatal(err)
	}
	if awaitStatus != "cancelled" || runStatus != TurnInProgress {
		t.Fatalf("await=%s run=%s", awaitStatus, runStatus)
	}
}

func TestInlineYoloReconcileKeepsWaitingParentWorkerFreeAndReplayIdempotent(t *testing.T) {
	fixture := newInlineYoloFixture(t)
	ctx := context.Background()
	fixture.Harness.queue.mu.Lock()
	fixture.Harness.queue.jobs = nil
	fixture.Harness.queue.mu.Unlock()
	if err := fixture.Harness.service.ReconcileOnOpen(ctx, fixture.Harness.store); err != nil {
		t.Fatal(err)
	}
	fixture.Harness.queue.mu.Lock()
	jobs := append([]JobSpec(nil), fixture.Harness.queue.jobs...)
	fixture.Harness.queue.mu.Unlock()
	for _, job := range jobs {
		if job.JobKind == JobChatTurn || job.JobKind == JobChatResume {
			t.Fatalf("waiting parent was requeued after reconcile: %+v", jobs)
		}
	}
	var awaitStatus, runStatus, turnStatus string
	if err := fixture.Harness.store.DB().Raw(`SELECT a.status,r.status,t.status FROM workflow_awaits a JOIN chat_runs r ON r.id=a.chat_run_id JOIN chat_turns t ON t.id=a.chat_turn_id WHERE a.workflow_id=(SELECT id FROM workflows WHERE uuid=?)`, fixture.Workflow.UUID).Row().Scan(&awaitStatus, &runStatus, &turnStatus); err != nil {
		t.Fatal(err)
	}
	if awaitStatus != "waiting" || runStatus != TurnInProgress || turnStatus != TurnInProgress {
		t.Fatalf("await=%s run=%s turn=%s", awaitStatus, runStatus, turnStatus)
	}
	replayed, err := fixture.Harness.service.CreateYoloWorkflow(ctx, fixture.Harness.project.UUID, CreateYoloInput{
		Title: "月光邮差", StoryPrompt: "重放不得创建第二个 Workflow。", ProviderUUID: fixture.Harness.provider.UUID,
		IdempotencyKey: "inline-yolo-fixture", Invocation: chatToolInvocationContext(fixture.Context, fixture.Execution),
	})
	if !errors.Is(err, ErrWaitingWorkflow) || replayed.UUID != fixture.Workflow.UUID {
		t.Fatalf("replayed=%+v err=%v", replayed, err)
	}
	for table, want := range map[string]int64{"chat_threads": 1, "workflows": 1, "workflow_awaits": 1} {
		var count int64
		if err := fixture.Harness.store.DB().Table(table).Count(&count).Error; err != nil || count != want {
			t.Fatalf("%s count=%d want=%d err=%v", table, count, want, err)
		}
	}
}

func TestInlineYoloCreationRollsBackWorkflowAwaitAndStepsWhenFirstJobFails(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "原子创建 YOLO"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.claimRun(ctx, harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	tc.ToolMode, err = harness.service.loadRunToolMode(ctx, harness.store, tc)
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{
		"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/workflows",
		"request_body":    map[string]any{"story_prompt": "原子事务"},
		"response_filter": ".data | {uuid,thread_uuid,presentation_mode,kind,title,status,current_step_key,steps}",
	}
	raw, _ := json.Marshal(args)
	execution, _, _, err := harness.service.persistToolIntent(ctx, harness.store, tc, "inline-yolo-rollback", "request_api", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	harness.queue.enqueueErr = errors.New("injected first-job failure")
	_, err = harness.service.CreateYoloWorkflow(ctx, harness.project.UUID, CreateYoloInput{
		Title: "原子事务", StoryPrompt: "任何部分记录都不得留下。", ProviderUUID: harness.provider.UUID,
		IdempotencyKey: "inline-yolo-rollback", Invocation: chatToolInvocationContext(tc, execution),
	})
	if err == nil || errors.Is(err, ErrWaitingWorkflow) {
		t.Fatalf("inline YOLO creation error=%v", err)
	}
	for _, table := range []string{"workflows", "workflow_steps", "workflow_awaits"} {
		var count int64
		if dbErr := harness.store.DB().Table(table).Count(&count).Error; dbErr != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, dbErr)
		}
	}
	var threadCount int64
	if err := harness.store.DB().Table("chat_threads").Count(&threadCount).Error; err != nil || threadCount != 1 {
		t.Fatalf("conversation thread count=%d err=%v", threadCount, err)
	}
}
