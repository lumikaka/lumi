package jobqueue

import (
	"context"
	"errors"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/llm"
	"lumi/internal/taskexecution"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type workflowStepWaitFixture struct {
	harness *queueHarness
	runtime *projectRuntime
	agents  *agent.Service
	step    executionRecord
	job     *rivertype.JobRow
	ctx     context.Context
}

func newWorkflowStepWaitFixture(t *testing.T) workflowStepWaitFixture {
	t.Helper()
	h, r := stoppedExecutionHarness(t)
	agents := agent.NewService(h.projects, h.queue.providers, nil, h.queue, nil)
	thread, err := agents.CreateThread(t.Context(), r.projectUUID, agent.CreateThreadInput{Title: "等待恢复", ProviderUUID: h.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	workflowUUID, _ := newUUIDv7()
	stepUUID, _ := newUUIDv7()
	now := time.Now().UTC()
	_, err = r.sqlDB.ExecContext(t.Context(), `INSERT INTO workflows(uuid,project_id,thread_id,kind,title,status,current_step_key,input_snapshot,idempotency_key,provider_uuid,model,created_at,updated_at)
		SELECT ?,?,id,?,'等待恢复','running','premise','{}',?,?,'frozen-model',?,? FROM chat_threads WHERE uuid=?`, workflowUUID, r.projectID, agent.WorkflowYolo, workflowUUID, h.provider.UUID, now, now, thread.UUID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.sqlDB.ExecContext(t.Context(), `INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,created_at,updated_at)
		SELECT ?,id,'premise',1,'running',?,?,? FROM workflows WHERE uuid=?`, stepUUID, stepUUID, now, now, workflowUUID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := r.sqlDB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	binding, err := r.enqueueAgentTx(t.Context(), r.projectUUID, tx, agent.JobSpec{Version: 1, ProjectUUID: r.projectUUID, JobKind: agent.JobWorkflowStep, ResourceUUID: stepUUID, ThreadUUID: thread.UUID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(t.Context(), `UPDATE workflow_steps SET river_job_id=?,execution_uuid=? WHERE uuid=?`, binding.JobID, binding.NullableUUID(), stepUUID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	job, err := r.client.JobGet(t.Context(), binding.JobID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := r.executionContext(t.Context(), job)
	if err != nil {
		t.Fatal(err)
	}
	return workflowStepWaitFixture{harness: h, runtime: r, agents: agents, step: readExecutionFixture(t, r, "workflow_steps", stepUUID), job: job, ctx: ctx}
}

func waitDependency(record executionRecord) agent.DomainTask {
	return agent.DomainTask{UUID: record.UUID, Kind: record.Kind, ResourceUUID: record.ResourceUUID, Status: record.Status}
}

func (f workflowStepWaitFixture) assertWait(t *testing.T, waitStatus, stepStatus string, jobCount int) string {
	t.Helper()
	var uuid, actualWait, actualStep string
	err := f.runtime.sqlDB.QueryRowContext(t.Context(), `SELECT a.uuid,a.status,s.status FROM workflow_steps s JOIN workflow_step_waits a ON a.id=s.current_wait_id WHERE s.uuid=?`, f.step.UUID).Scan(&uuid, &actualWait, &actualStep)
	if err != nil || actualWait != waitStatus || actualStep != stepStatus {
		t.Fatalf("wait=%q step=%q error=%v, want wait=%q step=%q", actualWait, actualStep, err, waitStatus, stepStatus)
	}
	jobs, err := f.runtime.client.JobList(t.Context(), river.NewJobListParams().First(1000))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, job := range jobs.Jobs {
		identity, err := executionIdentityFor(job)
		if err == nil && identity.JobKind == agent.JobWorkflowStep && identity.ResourceUUID == f.step.UUID {
			count++
		}
	}
	if count != jobCount {
		t.Fatalf("workflow execution count=%d, want %d", count, jobCount)
	}
	return uuid
}

func (f workflowStepWaitFixture) settle(t *testing.T, task executionRecord, status string) {
	t.Helper()
	tx, err := f.runtime.sqlDB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(t.Context(), `UPDATE `+task.Table+` SET status=? WHERE id=?`, status, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.runtime.readyWorkflowStepWaitsTx(t.Context(), tx, task.UUID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowStepWaitLeavesDependenciesWorkerFreeAcrossRecovery(t *testing.T) {
	f := newWorkflowStepWaitFixture(t)
	task := executionFixture(t, f.runtime, "production_task_runs", StatusRunning, taskexecution.Binding{})
	if err := f.harness.queue.SuspendWorkflowStep(f.ctx, f.runtime.projectUUID, f.step.UUID, "premise:setting", []agent.DomainTask{waitDependency(task)}); err != nil {
		t.Fatal(err)
	}
	f.assertWait(t, "waiting", "waiting", 1)
	for range 2 {
		if err := f.runtime.reconcileExecutionBindings(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := f.runtime.reconcileWorkflowStepWaits(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := f.agents.ReconcileWithEnqueue(t.Context(), f.runtime.store, f.runtime.enqueueAgentTx); err != nil {
			t.Fatal(err)
		}
		f.assertWait(t, "waiting", "waiting", 1)
	}
	var events int
	if err := f.runtime.sqlDB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM workflow_events WHERE step_id=?`, f.step.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("idle dependency emitted repeated events: count=%d error=%v", events, err)
	}
}

func TestWorkflowStepWaitHandlesCompletionBeforeRegistrationAndFencesOldWorker(t *testing.T) {
	f := newWorkflowStepWaitFixture(t)
	task := executionFixture(t, f.runtime, "task_runs", StatusCompleted, taskexecution.Binding{})
	stale := waitDependency(task)
	stale.Status = StatusRunning
	if err := f.harness.queue.SuspendWorkflowStep(f.ctx, f.runtime.projectUUID, f.step.UUID, "story:chapter", []agent.DomainTask{stale}); err != nil {
		t.Fatal(err)
	}
	waitUUID := f.assertWait(t, "ready", "queued", 2)
	current := readExecutionFixture(t, f.runtime, "workflow_steps", f.step.UUID)
	job, err := f.runtime.client.JobGet(t.Context(), current.JobID.Int64)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := executionIdentityFor(job)
	if err != nil || identity.WakeupUUID != waitUUID || current.binding() == f.step.binding() {
		t.Fatalf("resume identity=%+v binding=%+v error=%v", identity, current.binding(), err)
	}
	if _, err := f.runtime.executionContext(t.Context(), job); err != nil {
		t.Fatalf("new execution does not own its step: %v", err)
	}
	if tx, err := taskexecution.Begin(context.WithoutCancel(f.ctx), f.runtime.sqlDB); err == nil {
		tx.Rollback()
		t.Fatal("suspended execution can still commit after its wake-up replaced it")
	} else if !errors.Is(err, taskexecution.ErrSuperseded) {
		t.Fatal(err)
	}
	for range 3 {
		f.settle(t, task, StatusCompleted)
	}
	f.assertWait(t, "ready", "queued", 2)
}

func TestWorkflowStepWaitResumesOnlyAfterEveryDependencySettles(t *testing.T) {
	f := newWorkflowStepWaitFixture(t)
	first := executionFixture(t, f.runtime, "production_task_runs", StatusRunning, taskexecution.Binding{})
	second := executionFixture(t, f.runtime, "production_task_runs", StatusRunning, taskexecution.Binding{})
	if err := f.harness.queue.SuspendWorkflowStep(f.ctx, f.runtime.projectUUID, f.step.UUID, "initial_images", []agent.DomainTask{waitDependency(first), waitDependency(second)}); err != nil {
		t.Fatal(err)
	}
	f.settle(t, first, StatusFailed)
	f.assertWait(t, "waiting", "waiting", 1)
	f.settle(t, second, StatusCompleted)
	f.assertWait(t, "ready", "queued", 2)
	var status string
	if err := f.runtime.sqlDB.QueryRowContext(t.Context(), `SELECT status FROM production_task_runs WHERE id=?`, second.ID).Scan(&status); err != nil || status != StatusCompleted {
		t.Fatalf("completed sibling was changed: status=%q error=%v", status, err)
	}
}

func TestWorkflowStepWaitRecoveryRebuildsMissingWakeupWithoutAdoptingOldPhase(t *testing.T) {
	f := newWorkflowStepWaitFixture(t)
	task := executionFixture(t, f.runtime, "production_task_runs", StatusCompleted, taskexecution.Binding{})
	if err := f.harness.queue.SuspendWorkflowStep(f.ctx, f.runtime.projectUUID, f.step.UUID, "premise:setting", []agent.DomainTask{waitDependency(task)}); err != nil {
		t.Fatal(err)
	}
	waitUUID := f.assertWait(t, "ready", "queued", 2)
	before := readExecutionFixture(t, f.runtime, "workflow_steps", f.step.UUID)
	if _, err := f.runtime.client.JobDelete(t.Context(), before.JobID.Int64); err != nil {
		t.Fatal(err)
	}
	// The original phase's Job is deliberately still present. Recovery must
	// not adopt it merely because its step UUID matches the ready continuation.
	if err := f.runtime.reconcileExecutionBindings(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.runtime.reconcileWorkflowStepWaits(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.agents.ReconcileWithEnqueue(t.Context(), f.runtime.store, f.runtime.enqueueAgentTx); err != nil {
		t.Fatal(err)
	}
	f.assertWait(t, "ready", "queued", 2)
	after := readExecutionFixture(t, f.runtime, "workflow_steps", f.step.UUID)
	job, err := f.runtime.client.JobGet(t.Context(), after.JobID.Int64)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := executionIdentityFor(job)
	if err != nil || identity.WakeupUUID != waitUUID || after.binding() == before.binding() || after.binding() == f.step.binding() {
		t.Fatalf("recovered wrong execution: before=%+v after=%+v identity=%+v error=%v", before.binding(), after.binding(), identity, err)
	}
}

func TestWorkflowStepWaitRejectsForeignTaskWithoutParkingTheStep(t *testing.T) {
	f := newWorkflowStepWaitFixture(t)
	_, foreignRuntime := stoppedExecutionHarness(t)
	foreign := executionFixture(t, foreignRuntime, "production_task_runs", StatusRunning, taskexecution.Binding{})
	err := f.harness.queue.SuspendWorkflowStep(f.ctx, f.runtime.projectUUID, f.step.UUID, "premise:setting", []agent.DomainTask{waitDependency(foreign)})
	var taskErr *Error
	if !errors.As(err, &taskErr) || taskErr.Code != CodeTaskNotFound {
		t.Fatalf("foreign task accepted: %v", err)
	}
	var waitCount int
	if err := f.runtime.sqlDB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM workflow_step_waits`).Scan(&waitCount); err != nil || waitCount != 0 {
		t.Fatalf("failed registration left a wait: count=%d error=%v", waitCount, err)
	}
	current := readExecutionFixture(t, f.runtime, "workflow_steps", f.step.UUID)
	if current.Status != "running" || current.binding() != f.step.binding() {
		t.Fatalf("failed registration parked or rebound step: %+v", current)
	}
}

func TestWorkflowStepConsumedWaitRecoveryPreservesWakeupIdentity(t *testing.T) {
	f := newWorkflowStepWaitFixture(t)
	task := executionFixture(t, f.runtime, "production_task_runs", StatusCompleted, taskexecution.Binding{})
	if err := f.harness.queue.SuspendWorkflowStep(f.ctx, f.runtime.projectUUID, f.step.UUID, "setting", []agent.DomainTask{waitDependency(task)}); err != nil {
		t.Fatal(err)
	}
	waitUUID := f.assertWait(t, "ready", "queued", 2)
	before := readExecutionFixture(t, f.runtime, "workflow_steps", f.step.UUID)
	if _, err := f.runtime.sqlDB.ExecContext(t.Context(), `UPDATE workflow_step_waits SET status='consumed' WHERE uuid=?`, waitUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.sqlDB.ExecContext(t.Context(), `UPDATE workflow_steps SET status='running' WHERE id=?`, f.step.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.client.JobDelete(t.Context(), before.JobID.Int64); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := f.runtime.reconcileExecutionBindings(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := f.runtime.reconcileWorkflowStepWaits(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := f.agents.ReconcileWithEnqueue(t.Context(), f.runtime.store, f.runtime.enqueueAgentTx); err != nil {
			t.Fatal(err)
		}
	}
	f.assertWait(t, "consumed", "queued", 2)
	after := readExecutionFixture(t, f.runtime, "workflow_steps", f.step.UUID)
	job, err := f.runtime.client.JobGet(t.Context(), after.JobID.Int64)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := executionIdentityFor(job)
	if err != nil || identity.WakeupUUID != waitUUID || after.binding() == before.binding() {
		t.Fatalf("consumed recovery identity=%+v before=%+v after=%+v error=%v", identity, before.binding(), after.binding(), err)
	}
	if _, err := f.runtime.executionContext(t.Context(), job); err != nil {
		t.Fatalf("recovered execution rejected: %v", err)
	}
}

func TestWorkflowStepNewWaitIgnoresPreviousDependencyAndLateCancellation(t *testing.T) {
	f := newWorkflowStepWaitFixture(t)
	first := executionFixture(t, f.runtime, "production_task_runs", StatusCompleted, taskexecution.Binding{})
	if err := f.harness.queue.SuspendWorkflowStep(f.ctx, f.runtime.projectUUID, f.step.UUID, "setting", []agent.DomainTask{waitDependency(first)}); err != nil {
		t.Fatal(err)
	}
	oldWait := f.assertWait(t, "ready", "queued", 2)
	current := readExecutionFixture(t, f.runtime, "workflow_steps", f.step.UUID)
	job, err := f.runtime.client.JobGet(t.Context(), current.JobID.Int64)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := f.runtime.executionContext(t.Context(), job)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.sqlDB.ExecContext(t.Context(), `UPDATE workflow_step_waits SET status='consumed' WHERE uuid=?`, oldWait); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.sqlDB.ExecContext(t.Context(), `UPDATE workflow_steps SET status='running' WHERE id=?`, f.step.ID); err != nil {
		t.Fatal(err)
	}
	second := executionFixture(t, f.runtime, "production_task_runs", StatusRunning, taskexecution.Binding{})
	if err := f.harness.queue.SuspendWorkflowStep(ctx, f.runtime.projectUUID, f.step.UUID, "breakdown", []agent.DomainTask{waitDependency(second)}); err != nil {
		t.Fatal(err)
	}
	newWait := f.assertWait(t, "waiting", "waiting", 2)
	if newWait == oldWait {
		t.Fatal("separate phases shared a wakeup identity")
	}
	f.settle(t, first, StatusCompleted)
	f.assertWait(t, "waiting", "waiting", 2)
	var workflowUUID string
	if err := f.runtime.sqlDB.QueryRowContext(t.Context(), `SELECT w.uuid FROM workflows w JOIN workflow_steps s ON s.workflow_id=w.id WHERE s.id=?`, f.step.ID).Scan(&workflowUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.agents.CancelWorkflow(t.Context(), f.runtime.projectUUID, workflowUUID); err != nil {
		t.Fatal(err)
	}
	f.settle(t, second, StatusCompleted)
	f.assertWait(t, "cancelled", "cancelled", 2)
}

func TestWorkflowStepWaitDoesNotResumeDuringAutomaticStoryRetry(t *testing.T) {
	f := newWorkflowStepWaitFixture(t)
	task := executionFixture(t, f.runtime, "task_runs", StatusRunning, taskexecution.Binding{})
	if err := f.harness.queue.SuspendWorkflowStep(f.ctx, f.runtime.projectUUID, f.step.UUID, "story:chapter", []agent.DomainTask{waitDependency(task)}); err != nil {
		t.Fatal(err)
	}
	record, err := getTaskRecord(t.Context(), f.runtime.store.DB(), f.runtime.projectID, task.UUID)
	if err != nil {
		t.Fatal(err)
	}
	transient := &llm.Error{Code: llm.CodeNetwork, SafeMessage: "temporary connection failure", Retryable: true}
	if err := f.runtime.handleWorkError(t.Context(), record, transient, 1, 3); !errors.Is(err, transient) {
		t.Fatalf("automatic retry did not preserve its provider error: %v", err)
	}
	current := readExecutionFixture(t, f.runtime, task.Table, task.UUID)
	if current.Status != StatusQueued {
		t.Fatalf("retryable attempt became terminal: %+v", current)
	}
	f.assertWait(t, "waiting", "waiting", 1)
	var failedEvents int
	if err := f.runtime.sqlDB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM task_events WHERE task_run_id=? AND event_type='task_failed'`, task.ID).Scan(&failedEvents); err != nil || failedEvents != 0 {
		t.Fatalf("retry exposed a transient terminal event: count=%d error=%v", failedEvents, err)
	}
	if _, err := f.runtime.sqlDB.ExecContext(t.Context(), `UPDATE task_runs SET status='running' WHERE id=?`, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.runtime.handleWorkError(t.Context(), record, transient, 3, 3); !errors.Is(err, transient) {
		t.Fatalf("exhausted attempt did not preserve its provider error: %v", err)
	}
	current = readExecutionFixture(t, f.runtime, task.Table, task.UUID)
	if current.Status != StatusFailed {
		t.Fatalf("exhausted task did not become terminal: %+v", current)
	}
	f.assertWait(t, "ready", "queued", 2)
}
