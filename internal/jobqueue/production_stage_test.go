package jobqueue

import (
	"context"
	"errors"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/production"
)

func TestImageStagePersistsAndCancellationIntentIsRecovered(t *testing.T) {
	h := newQueueHarness(t)
	started := make(chan struct{})
	h.queue.WithImageClient(blockingImageProvider{started: started})
	ctx := context.Background()
	runtime := h.runtime(t)
	service := production.NewService(runtime.store, nil)
	chapter := h.createChapter(t, "vol01.ch42")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Stage", StoryboardMD: "A quiet forest."})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := h.queue.CreateComicImageGenerationBatch(ctx, h.project.UUID, chapter.UUID, CreateComicImageGenerationBatchInput{SectionUUIDs: []string{section.UUID}, IdempotencyKey: "stage-cancel-recovery"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("image call did not start")
	}
	task, err := h.queue.GetProductionTask(ctx, h.project.UUID, batch.Tasks[0].UUID)
	if err != nil || task.Stage != "generating" || task.StageStartedAt == nil {
		t.Fatalf("task=%+v err=%v", task, err)
	}
	agents := agent.NewService(h.projects, h.queue.providers, newRiverAgentModel(), h.queue, nil)
	workflow, err := agents.GetWorkflow(ctx, h.project.UUID, batch.WorkflowUUID)
	if err != nil || len(workflow.Steps) != 1 || workflow.Steps[0].Stage != task.Stage || !workflow.Steps[0].StageStartedAt.Equal(*task.StageStartedAt) {
		t.Fatalf("workflow=%+v err=%v", workflow, err)
	}
	var stages []string
	if err := runtime.store.DB().Raw(`SELECT json_extract(payload,'$.stage') FROM production_task_events WHERE production_task_run_id=(SELECT id FROM production_task_runs WHERE uuid=?) AND event_type='task_stage_changed' ORDER BY sequence`, task.UUID).Scan(&stages).Error; err != nil {
		t.Fatal(err)
	}
	if len(stages) != 3 || stages[0] != "selecting_references" || stages[1] != "preparing_references" || stages[2] != "generating" {
		t.Fatalf("stages=%v", stages)
	}
	// Fail final cancellation, after the separate intent transaction commits.
	if _, err := runtime.sqlDB.ExecContext(ctx, `CREATE TRIGGER reject_cancellation BEFORE UPDATE OF status ON production_task_runs WHEN NEW.status='cancelled' BEGIN SELECT RAISE(ABORT,'temporary cancellation failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.queue.CancelProductionTask(ctx, h.project.UUID, task.UUID); err == nil {
		t.Fatal("expected cancellation failure")
	}
	pending, err := h.queue.GetProductionTask(ctx, h.project.UUID, task.UUID)
	if err != nil || pending.CancelRequestedAt == nil {
		t.Fatalf("intent was lost: %+v err=%v", pending, err)
	}
	record, err := getProductionTaskRecord(ctx, runtime.store.DB(), runtime.projectID, task.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.productionStage(ctx, record, "saving"); !errors.Is(err, context.Canceled) {
		t.Fatalf("late stage advanced cancelled work: %v", err)
	}
	if _, err := runtime.sqlDB.ExecContext(ctx, `DROP TRIGGER reject_cancellation`); err != nil {
		t.Fatal(err)
	}
	// Exercise the same recovery used by the live runtime, without another click.
	if err := runtime.reconcileProductionCancellations(ctx); err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, h.queue, h.project.UUID, task.UUID, StatusCancelled)
	workflow, err = agents.GetWorkflow(ctx, h.project.UUID, batch.WorkflowUUID)
	if err != nil || workflow.Status != agent.WorkflowCancelled {
		t.Fatalf("workflow did not converge: %+v err=%v", workflow, err)
	}
	page, err := service.GetSection(ctx, chapter.UUID, section.UUID)
	if err != nil || page.CurrentImage != nil {
		t.Fatalf("cancelled generation committed an image: %+v err=%v", page, err)
	}
	// A user retry can clear the intent after the reconciler has read its list.
	// That stale recovery command must not cancel the next attempt.
	if _, err := runtime.sqlDB.ExecContext(ctx, `UPDATE production_task_runs SET status='queued',cancel_requested_at=NULL WHERE uuid=?`, task.UUID); err != nil {
		t.Fatal(err)
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runtime.registerWork(task.UUID, cancel)
	defer runtime.unregisterWork(task.UUID)
	result, err := runtime.finishProductionCancellation(ctx, task.UUID)
	if err != nil || result.Status != StatusQueued || workCtx.Err() != nil {
		t.Fatalf("stale cancellation reached new attempt: %+v err=%v work=%v", result, err, workCtx.Err())
	}
}
