package jobqueue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/taskexecution"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver"
	"github.com/riverqueue/river/riverdriver/riversqlite"
	"github.com/riverqueue/river/rivertype"
)

func claimRetryFixture(t *testing.T, r *projectRuntime, job *rivertype.JobRow, clientID string) *rivertype.JobRow {
	t.Helper()
	jobs, err := riversqlite.New(r.sqlDB).GetExecutor().JobGetAvailable(t.Context(), &riverdriver.JobGetAvailableParams{
		ClientID: clientID, MaxAttemptedBy: 10, MaxToLock: 1, Queue: job.Queue,
	})
	if err != nil || len(jobs) != 1 || jobs[0].ID != job.ID || jobs[0].State != rivertype.JobStateRunning {
		t.Fatalf("claim fixture: %+v / %v", jobs, err)
	}
	return jobs[0]
}

func TestRetryReclaimsPreviousRuntimeExecution(t *testing.T) {
	for _, status := range []string{StatusFailed, StatusCancelled, StatusInterrupted} {
		t.Run(status, func(t *testing.T) {
			_, r := stoppedExecutionHarness(t)
			owner := executionFixture(t, r, "production_task_runs", StatusQueued, taskexecution.Binding{})
			job := claimRetryFixture(t, r, enqueueExecutionFixture(t, r, owner), "stopped-runtime")
			if _, err := r.sqlDB.ExecContext(t.Context(), `UPDATE production_task_runs SET status=? WHERE uuid=?`, status, owner.UUID); err != nil {
				t.Fatal(err)
			}
			if err := r.waitExecutionRetryBoundary(t.Context(), owner.Table, owner.UUID); err != nil {
				t.Fatal(err)
			}
			current, err := r.client.JobGet(t.Context(), job.ID)
			if err != nil || current.State != rivertype.JobStateCancelled || current.FinalizedAt == nil {
				t.Fatalf("previous execution not reclaimed: %+v / %v", current, err)
			}
			if record := readExecutionFixture(t, r, owner.Table, owner.UUID); record.Status != status || record.JobID.Valid || record.ExecutionUUID.Valid {
				t.Fatalf("bad recovered owner: %+v", record)
			}
		})
	}
}

func TestRetryKeepsCurrentAndUnknownExecutions(t *testing.T) {
	for _, scenario := range []string{"current_before_registration", "unknown_claimant", "registered_work", "active_owner"} {
		t.Run(scenario, func(t *testing.T) {
			_, r := stoppedExecutionHarness(t)
			owner := executionFixture(t, r, "production_task_runs", StatusQueued, taskexecution.Binding{})
			claimant := "stopped-runtime"
			if scenario == "current_before_registration" {
				claimant = r.client.ID()
			} else if scenario == "unknown_claimant" {
				claimant = ""
			}
			job := claimRetryFixture(t, r, enqueueExecutionFixture(t, r, owner), claimant)
			if scenario == "registered_work" {
				ctx, err := r.executionContext(t.Context(), job)
				if err != nil {
					t.Fatal(err)
				}
				_, cancel := context.WithCancel(ctx)
				defer cancel()
				defer r.registerExecutionWork(ctx, cancel)()
			}
			status := StatusCancelled
			if scenario == "active_owner" {
				status = StatusRunning
			}
			if _, err := r.sqlDB.ExecContext(t.Context(), `UPDATE production_task_runs SET status=? WHERE uuid=?`, status, owner.UUID); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			err := r.waitExecutionRetryBoundary(ctx, owner.Table, owner.UUID)
			if scenario == "active_owner" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var taskErr *Error
				if !errors.As(err, &taskErr) || taskErr.Code != CodeTaskStateConflict {
					t.Fatalf("expected retry conflict: %v", err)
				}
			}
			current, err := r.client.JobGet(t.Context(), job.ID)
			if err != nil || current.State != rivertype.JobStateRunning {
				t.Fatalf("active job changed: %+v / %v", current, err)
			}
			if record := readExecutionFixture(t, r, owner.Table, owner.UUID); record.binding() != executionBindingFor(job) {
				t.Fatalf("active binding changed: %+v", record)
			}
		})
	}
}

func TestRetryRecoveryRollsBackWithBindingFailure(t *testing.T) {
	_, r := stoppedExecutionHarness(t)
	owner := executionFixture(t, r, "production_task_runs", StatusQueued, taskexecution.Binding{})
	job := claimRetryFixture(t, r, enqueueExecutionFixture(t, r, owner), "stopped-runtime")
	if _, err := r.sqlDB.ExecContext(t.Context(), `UPDATE production_task_runs SET status='cancelled' WHERE uuid=?`, owner.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.sqlDB.ExecContext(t.Context(), `CREATE TEMP TRIGGER reject_retry_release BEFORE UPDATE OF execution_uuid ON production_task_runs WHEN NEW.execution_uuid IS NULL BEGIN SELECT RAISE(ABORT,'injected release failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := r.waitExecutionRetryBoundary(t.Context(), owner.Table, owner.UUID); err == nil {
		t.Fatal("expected release failure")
	}
	current, err := r.client.JobGet(t.Context(), job.ID)
	if err != nil || current.State != rivertype.JobStateRunning || current.FinalizedAt != nil {
		t.Fatalf("cancellation escaped rollback: %+v / %v", current, err)
	}
	if record := readExecutionFixture(t, r, owner.Table, owner.UUID); record.binding() != executionBindingFor(job) {
		t.Fatalf("binding escaped rollback: %+v", record)
	}
}

func TestWorkflowRetryAfterRestartReclaimsCancelledStory(t *testing.T) {
	h, r := stoppedExecutionHarness(t)
	agents := agent.NewService(h.projects, h.queue.providers, nil, h.queue, nil)
	thread, err := agents.CreateThread(t.Context(), r.projectUUID, agent.CreateThreadInput{Title: "重启后重试", ProviderUUID: h.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	workflowUUID, _ := newUUIDv7()
	storyUUID, _ := newUUIDv7()
	now := time.Now().UTC()
	_, err = r.sqlDB.ExecContext(t.Context(), `INSERT INTO workflows(uuid,project_id,thread_id,kind,title,status,current_step_key,input_snapshot,idempotency_key,provider_uuid,model,created_at,updated_at) SELECT ?,?,id,?,'重试','queued','story','{}',?,?,'frozen-model',?,? FROM chat_threads WHERE uuid=?`, workflowUUID, r.projectID, agent.WorkflowYolo, workflowUUID, h.provider.UUID, now, now, thread.UUID)
	if err != nil {
		t.Fatal(err)
	}
	for i, key := range []string{"project_initialization", "story", "story_profile"} {
		uuid, _ := newUUIDv7()
		status := "pending"
		if i == 0 {
			status = "completed"
		} else if i == 1 {
			uuid, status = storyUUID, "queued"
		}
		_, err := r.sqlDB.ExecContext(t.Context(), `INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,created_at,updated_at) SELECT ?,id,?,?,?,?,?,? FROM workflows WHERE uuid=?`, uuid, key, i+1, status, uuid, now, now, workflowUUID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := agents.ReconcileWithEnqueue(t.Context(), r.store, r.enqueueAgentTx); err != nil {
		t.Fatal(err)
	}
	story := readExecutionFixture(t, r, "workflow_steps", storyUUID)
	job, err := r.client.JobGet(t.Context(), story.JobID.Int64)
	if err != nil {
		t.Fatal(err)
	}
	job = claimRetryFixture(t, r, job, "runtime-before-restart")
	oldCtx, err := r.executionContext(t.Context(), job)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agents.CancelWorkflow(t.Context(), r.projectUUID, workflowUUID); err != nil {
		t.Fatal(err)
	}
	// Startup retains the running River row. Explicit retry must recover it
	// without waiting for the generic stuck-job rescue interval.
	if err := r.reconcileExecutionBindings(t.Context()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := agents.RetryWorkflow(t.Context(), r.projectUUID, workflowUUID)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	workflow, err := agents.GetWorkflow(t.Context(), r.projectUUID, workflowUUID)
	if err != nil || workflow.Status != "queued" || len(workflow.Steps) != 3 || workflow.Steps[0].Status != "completed" || workflow.Steps[1].Status != "queued" || workflow.Steps[2].Status != "pending" {
		t.Fatalf("bad retried workflow: %+v / %v", workflow, err)
	}
	next := readExecutionFixture(t, r, "workflow_steps", storyUUID)
	if !isUUIDv7(next.ExecutionUUID.String) || next.ExecutionUUID == story.ExecutionUUID || next.JobID == story.JobID {
		t.Fatalf("retry did not create a fresh execution: %+v", next)
	}
	jobs, err := r.client.JobList(t.Context(), river.NewJobListParams())
	if err != nil || len(jobs.Jobs) != 2 {
		t.Fatalf("concurrent retries duplicated jobs: %+v / %v", jobs, err)
	}
	oldJob, err := r.client.JobGet(t.Context(), job.ID)
	if err != nil || oldJob.State != rivertype.JobStateCancelled {
		t.Fatalf("old execution survived: %+v / %v", oldJob, err)
	}
	if tx, err := taskexecution.Begin(oldCtx, r.sqlDB); err == nil {
		tx.Rollback()
		t.Fatal("old execution can still write")
	} else if !errors.Is(err, taskexecution.ErrSuperseded) {
		t.Fatal(err)
	}
	if err := r.handleExecutionEvent(t.Context(), &river.Event{Kind: river.EventKindJobCancelled, Job: oldJob}); err != nil {
		t.Fatal(err)
	}
	if after := readExecutionFixture(t, r, "workflow_steps", storyUUID); after.binding() != next.binding() || after.Status != "queued" {
		t.Fatalf("late event changed successor: %+v", after)
	}
	var retries int
	if err := r.sqlDB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM workflow_events WHERE event_type='workflow_retried'`).Scan(&retries); err != nil || retries != 1 {
		t.Fatalf("retry events=%d: %v", retries, err)
	}
}
