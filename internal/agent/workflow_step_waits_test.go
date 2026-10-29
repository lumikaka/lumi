package agent

import (
	"context"
	"errors"
	"testing"

	"lumi/internal/project"
	"lumi/internal/story"
)

type suspendingAgentQueue struct {
	*agentQueueFake
	store *project.Store
	phase string
	tasks []DomainTask
}

func (queue *suspendingAgentQueue) SuspendWorkflowStep(ctx context.Context, _ string, stepUUID, phase string, tasks []DomainTask) error {
	queue.phase, queue.tasks = phase, tasks
	// Model a fast dependency completion which already queued the continuation.
	// The old execution must not subsequently overwrite this with waiting.
	return queue.store.DB().WithContext(ctx).Exec(`UPDATE workflow_steps SET status='queued' WHERE uuid=?`, stepUUID).Error
}

func TestYoloSuspensionDoesNotOverwriteAlreadyQueuedContinuation(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	_, err := story.NewService(harness.store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "第一章", ContentFormat: "txt"})
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := harness.service.CreateYoloWorkflow(ctx, harness.project.UUID, CreateYoloInput{Title: "持久等待", StoryPrompt: "小狐狸寻找月亮。", IdempotencyKey: "suspend-continuation"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.ExecuteWorkflowStep(ctx, harness.store, workflow.Steps[0].UUID); err != nil {
		t.Fatal(err)
	}
	queue := &suspendingAgentQueue{agentQueueFake: harness.queue, store: harness.store}
	harness.service.queue = queue
	if err := harness.service.ExecuteWorkflowStep(ctx, harness.store, workflow.Steps[1].UUID); !errors.Is(err, ErrWaitingDependencies) {
		t.Fatalf("suspension = %v", err)
	}
	current, err := harness.service.GetWorkflow(ctx, harness.project.UUID, workflow.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != WorkflowRunning || current.Steps[1].Status != "queued" || queue.phase != "chapter" || len(queue.tasks) != 1 {
		t.Fatalf("continuation overwritten: workflow=%+v phase=%s tasks=%+v", current, queue.phase, queue.tasks)
	}
}

func TestYoloDurableWaitingSurvivesReplayRecoveryCancellationAndRetry(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	workflow, err := harness.service.CreateYoloWorkflow(ctx, harness.project.UUID, CreateYoloInput{Title: "恢复等待", StoryPrompt: "小狐狸寻找月亮。", IdempotencyKey: "recover-suspended-step"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.ExecuteWorkflowStep(ctx, harness.store, workflow.Steps[0].UUID); err != nil {
		t.Fatal(err)
	}
	stepUUID, waitUUID := workflow.Steps[1].UUID, mustAgentUUID(t)
	now := harness.service.now().UTC()
	if err := harness.store.DB().Exec(`INSERT INTO workflow_step_waits(uuid,workflow_step_id,phase_key,generation,status,created_at,updated_at) SELECT ?,id,'chapter',1,'waiting',?,? FROM workflow_steps WHERE uuid=?`, waitUUID, now, now, stepUUID).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Exec(`UPDATE workflow_steps SET status='waiting',river_job_id=NULL,execution_uuid=NULL,current_wait_id=(SELECT id FROM workflow_step_waits WHERE uuid=?) WHERE uuid=?`, waitUUID, stepUUID).Error; err != nil {
		t.Fatal(err)
	}
	jobs := len(harness.queue.jobs)
	var events int64
	if err := harness.store.DB().Table("workflow_events").Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := harness.service.ExecuteWorkflowStep(ctx, harness.store, stepUUID); !errors.Is(err, ErrWaitingDependencies) {
			t.Fatalf("replayed wait = %v", err)
		}
		if err := harness.service.ReconcileOnOpen(ctx, harness.store); err != nil {
			t.Fatal(err)
		}
	}
	var afterEvents int64
	if err := harness.store.DB().Table("workflow_events").Count(&afterEvents).Error; err != nil {
		t.Fatal(err)
	}
	if len(harness.queue.jobs) != jobs || afterEvents != events || harness.model.calls != 0 {
		t.Fatalf("waiting produced work: jobs=%d/%d events=%d/%d calls=%d", len(harness.queue.jobs), jobs, afterEvents, events, harness.model.calls)
	}
	if _, err := harness.service.CancelWorkflow(ctx, harness.project.UUID, workflow.UUID); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := harness.store.DB().Raw(`SELECT status FROM workflow_step_waits WHERE uuid=?`, waitUUID).Scan(&status).Error; err != nil || status != "cancelled" {
		t.Fatalf("cancelled wait status=%s err=%v", status, err)
	}
	if _, err := harness.service.RetryWorkflow(ctx, harness.project.UUID, workflow.UUID); err != nil {
		t.Fatal(err)
	}
	var hasWait bool
	if err := harness.store.DB().Raw(`SELECT current_wait_id IS NOT NULL FROM workflow_steps WHERE uuid=?`, stepUUID).Scan(&hasWait).Error; err != nil || hasWait {
		t.Fatalf("retry retained stale wait: %v err=%v", hasWait, err)
	}
}

func TestInitializationDefersPreviouslyQueuedChatWithoutCallingModel(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "先排队的消息"})
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := harness.service.CreateYoloWorkflow(ctx, harness.project.UUID, CreateYoloInput{Title: "初始化", StoryPrompt: "小狐狸寻找月亮。", IdempotencyKey: "initialization-defers-chat"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); !errors.Is(err, ErrJobNotReady) || harness.model.calls != 0 {
		t.Fatalf("queued chat ran during initialization: err=%v calls=%d", err, harness.model.calls)
	}
	if _, err := harness.service.CancelWorkflow(ctx, harness.project.UUID, workflow.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.TakeOverInitialization(ctx, workflow.UUID); err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil || harness.model.calls != 1 {
		t.Fatalf("handoff did not release queued chat: err=%v calls=%d", err, harness.model.calls)
	}
}
