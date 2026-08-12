package jobqueue

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"lumi/internal/config"
	"lumi/internal/database"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riversqlite"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
)

type gateArgs struct {
	Key  string `json:"key" river:"unique"`
	Mode string `json:"mode"`
}

func (gateArgs) Kind() string { return "lumi_river_sqlite_gate_v1" }

func (gateArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{Queue: QueueStory} }

type gateWorker struct {
	river.WorkerDefaults[gateArgs]
	mu       sync.Mutex
	attempts map[string]int
	started  chan string
}

func (worker *gateWorker) Work(ctx context.Context, job *river.Job[gateArgs]) error {
	worker.mu.Lock()
	worker.attempts[job.Args.Key]++
	attempt := worker.attempts[job.Args.Key]
	worker.mu.Unlock()
	select {
	case worker.started <- job.Args.Key:
	default:
	}
	switch job.Args.Mode {
	case "fail_once":
		if attempt == 1 {
			return errors.New("intentional compatibility retry")
		}
	case "block":
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

type fastRetry struct{}

func (fastRetry) NextRetry(*rivertype.JobRow) time.Time {
	return time.Now().UTC().Add(15 * time.Millisecond)
}

func TestRiverSQLiteCompatibilityGate(t *testing.T) {
	ctx := t.Context()
	dsn := config.SQLiteDSN(filepath.Join(t.TempDir(), "river-gate.sqlite"))
	gormDB, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB.Stats().MaxOpenConnections != 1 {
		t.Fatalf("MaxOpenConnections = %d", sqlDB.Stats().MaxOpenConnections)
	}
	driver := riversqlite.New(sqlDB)
	migrator, err := rivermigrate.New(driver, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		t.Fatal(err)
	}
	validation, err := migrator.Validate(ctx, nil)
	if err != nil || !validation.OK {
		t.Fatalf("River migration validation = %#v, error = %v", validation, err)
	}

	worker := &gateWorker{attempts: make(map[string]int), started: make(chan string, 20)}
	client := newGateClient(t, driver, worker)
	completed, cancelCompleted := client.Subscribe(river.EventKindJobCompleted)
	defer cancelCompleted()
	failed, cancelFailed := client.Subscribe(river.EventKindJobFailed)
	defer cancelFailed()
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Product state and River insertion share the same database/sql transaction.
	if _, err := sqlDB.ExecContext(ctx, "CREATE TABLE gate_products (uuid TEXT PRIMARY KEY, value TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO gate_products VALUES ('rolled-back', 'no')"); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := client.InsertTx(ctx, tx, gateArgs{Key: "rolled-back"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.JobGet(ctx, rolledBack.Job.ID); !errors.Is(err, rivertype.ErrNotFound) {
		t.Fatalf("rolled-back River job error = %v", err)
	}

	tx, err = sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO gate_products VALUES ('committed', 'yes')"); err != nil {
		t.Fatal(err)
	}
	committed, err := client.InsertTx(ctx, tx, gateArgs{Key: "committed"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	waitJobEvent(t, completed, committed.Job.ID)

	// Worker errors retry through River's retry policy, then complete.
	retryJob, err := client.Insert(ctx, gateArgs{Key: "retry", Mode: "fail_once"}, &river.InsertOpts{MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	waitJobEvent(t, failed, retryJob.Job.ID)
	waitJobEvent(t, completed, retryJob.Job.ID)
	if job, err := client.JobGet(ctx, retryJob.Job.ID); err != nil || job.Attempt != 2 || job.State != rivertype.JobStateCompleted {
		t.Fatalf("retried job = %#v, error = %v", job, err)
	}

	// Scheduled jobs are not claimed early.
	scheduledAt := time.Now().Add(120 * time.Millisecond)
	scheduled, err := client.Insert(ctx, gateArgs{Key: "scheduled"}, &river.InsertOpts{ScheduledAt: scheduledAt})
	if err != nil {
		t.Fatal(err)
	}
	waitJobEvent(t, completed, scheduled.Job.ID)
	if job, err := client.JobGet(ctx, scheduled.Job.ID); err != nil || job.AttemptedAt == nil || job.AttemptedAt.Before(scheduledAt.Add(-10*time.Millisecond)) {
		t.Fatalf("scheduled job = %#v, error = %v", job, err)
	}

	// Active unique jobs collapse duplicates and cancellation reaches work context.
	uniqueOpts := river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled}}
	blocking, err := client.Insert(ctx, gateArgs{Key: "blocking", Mode: "block"}, &river.InsertOpts{UniqueOpts: uniqueOpts, ScheduledAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := client.Insert(ctx, gateArgs{Key: "blocking", Mode: "block"}, &river.InsertOpts{UniqueOpts: uniqueOpts, ScheduledAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.UniqueSkippedAsDuplicate || duplicate.Job.ID != blocking.Job.ID {
		t.Fatalf("duplicate result = %#v", duplicate)
	}
	if _, err := client.JobCancel(ctx, blocking.Job.ID); err != nil {
		t.Fatal(err)
	}
	waitJobState(t, client, blocking.Job.ID, rivertype.JobStateCancelled)

	stopCtx, stopCancel := context.WithTimeout(ctx, 3*time.Second)
	defer stopCancel()
	if err := client.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	// Close and reopen the same real WAL database; migrations and work remain usable.
	reopened, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	reopenedSQL, err := reopened.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedSQL.Close()
	reopenedDriver := riversqlite.New(reopenedSQL)
	reopenedMigrator, err := rivermigrate.New(reopenedDriver, nil)
	if err != nil {
		t.Fatal(err)
	}
	validation, err = reopenedMigrator.Validate(ctx, nil)
	if err != nil || !validation.OK {
		t.Fatalf("reopened validation = %#v, error = %v", validation, err)
	}
	reopenedWorker := &gateWorker{attempts: make(map[string]int), started: make(chan string, 4)}
	reopenedClient := newGateClient(t, reopenedDriver, reopenedWorker)
	reopenedCompleted, cancelReopened := reopenedClient.Subscribe(river.EventKindJobCompleted)
	defer cancelReopened()
	if err := reopenedClient.Start(ctx); err != nil {
		t.Fatal(err)
	}
	reopenedJob, err := reopenedClient.Insert(ctx, gateArgs{Key: "reopened"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitJobEvent(t, reopenedCompleted, reopenedJob.Job.ID)
	if err := reopenedClient.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}

func newGateClient(t *testing.T, driver *riversqlite.Driver, worker *gateWorker) *river.Client[*sql.Tx] {
	t.Helper()
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)
	client, err := river.NewClient(driver, &river.Config{Queues: map[string]river.QueueConfig{QueueStory: {MaxWorkers: 1}}, Workers: workers, FetchPollInterval: 20 * time.Millisecond, FetchCooldown: time.Millisecond, RetryPolicy: fastRetry{}, SoftStopTimeout: 100 * time.Millisecond, JobTimeout: time.Second, RescueStuckJobsAfter: 2 * time.Second, TestOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func waitJobEvent(t *testing.T, events <-chan *river.Event, jobID int64) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event != nil && event.Job != nil && event.Job.ID == jobID {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for River job %d", jobID)
		}
	}
}

func waitStarted(t *testing.T, started <-chan string, key string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case value := <-started:
			if value == key {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %s", key)
		}
	}
}

func waitJobState(t *testing.T, client *river.Client[*sql.Tx], jobID int64, state rivertype.JobState) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastJob *rivertype.JobRow
	var lastErr error
	for time.Now().Before(deadline) {
		job, err := client.JobGet(t.Context(), jobID)
		lastJob, lastErr = job, err
		if err == nil && job.State == state {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for River job %d state %s; last job=%#v error=%v", jobID, state, lastJob, lastErr)
}
