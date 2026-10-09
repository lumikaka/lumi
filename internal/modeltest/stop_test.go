package modeltest

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func waitForTest(t *testing.T, h *harness, testUUID, status string) Test {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		row, err := h.s.Get(context.Background(), testUUID)
		if err != nil {
			t.Fatal(err)
		}
		if row.Status == status {
			return row
		}
		select {
		case <-h.events.updates:
		case <-timeout.C:
			t.Fatalf("test did not reach %s: %+v", status, row)
		}
	}
}

func TestStopRunningTestCancelsEachStepAndContinuesQueue(t *testing.T) {
	for index, kind := range []string{"connection", "text", "image"} {
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t)
			release := make(chan struct{})
			h.models.block, h.models.entered, h.models.release = kind, make(chan struct{}, 1), release
			h.events.updates = make(chan struct{}, 128)
			if err := h.s.Start(); err != nil {
				t.Fatal(err)
			}
			row := h.queued(t)
			select {
			case <-h.models.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("worker did not start")
			}
			second := h.queued(t)
			stopped, err := h.s.Stop(context.Background(), row.UUID)
			if err != nil || stopped.Status != "interrupted" || stopped.CompletedAt == nil {
				t.Fatalf("stop=%+v err=%v", stopped, err)
			}
			for i, step := range stopped.Steps {
				want := "succeeded"
				if i == index {
					want = "interrupted"
				} else if i > index {
					want = "skipped"
				}
				if step.Status != want || step.CompletedAt == nil {
					t.Fatalf("step=%+v want=%s", step, want)
				}
			}
			// The next test reaches its provider before the first request is
			// released, proving cancellation propagated and the queue kept running.
			select {
			case <-h.models.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("stopping the first request did not release the worker")
			}
			again, err := h.s.Stop(context.Background(), row.UUID)
			if err != nil || !reflect.DeepEqual(again, stopped) {
				t.Fatalf("repeated stop changed result: %+v %v", again, err)
			}
			close(release)
			waitForTest(t, h, second.UUID, "succeeded")
			if err := h.s.Close(); err != nil {
				t.Fatal(err)
			}
			wantCalls := append([]string{"connection", "text", "image"}[:index+1:index+1], "connection", "text", "image")
			if !reflect.DeepEqual(h.models.calls, wantCalls) {
				t.Fatalf("calls=%v want=%v", h.models.calls, wantCalls)
			}
			final, _ := h.s.Get(context.Background(), row.UUID)
			if !reflect.DeepEqual(final, stopped) {
				t.Fatalf("stopped result changed: %+v", final)
			}
		})
	}
}

func TestStopQueuedTestDoesNotCancelRunningTest(t *testing.T) {
	h := newHarness(t)
	release := make(chan struct{})
	h.models.block, h.models.entered, h.models.release = "text", make(chan struct{}, 1), release
	h.events.updates = make(chan struct{}, 128)
	if err := h.s.Start(); err != nil {
		t.Fatal(err)
	}
	first := h.queued(t)
	select {
	case <-h.models.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	queued := h.queued(t)
	stopped, err := h.s.Stop(context.Background(), queued.UUID)
	if err != nil || stopped.Status != "interrupted" || stopped.StartedAt != nil {
		t.Fatalf("stop=%+v err=%v", stopped, err)
	}
	for _, step := range stopped.Steps {
		if step.Status != "skipped" || step.StartedAt != nil {
			t.Fatalf("queued step ran: %+v", step)
		}
	}
	close(release)
	completed := waitForTest(t, h, first.UUID, "succeeded")
	unchanged, err := h.s.Stop(context.Background(), first.UUID)
	if err != nil || !reflect.DeepEqual(unchanged, completed) {
		t.Fatalf("completed result changed: %+v %v", unchanged, err)
	}
	if err := h.s.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(h.models.calls, []string{"connection", "text", "image"}) {
		t.Fatal(h.models.calls)
	}
}

func TestStopRacesWithClaimAndCompletion(t *testing.T) {
	h := newHarness(t)
	for range 20 {
		row := h.queued(t)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := h.s.run(context.Background(), row.UUID); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := h.s.Stop(context.Background(), row.UUID); err != nil {
				t.Error(err)
			}
		}()
		wg.Wait()
		final, err := h.s.Get(context.Background(), row.UUID)
		if err != nil || (final.Status != "interrupted" && final.Status != "succeeded") {
			t.Fatalf("test left unfinished: %+v %v", final, err)
		}
		for _, step := range final.Steps {
			if step.Status == "pending" || step.Status == "running" {
				t.Fatalf("step left unfinished: %+v", step)
			}
		}
	}
}

func TestLateCompletionCannotOverwriteStoppedTest(t *testing.T) {
	h := newHarness(t)
	row := h.queued(t)
	row, ctx, cancel, err := h.s.claim(context.Background(), row.UUID)
	if err != nil || cancel == nil {
		t.Fatal("claim failed", err)
	}
	defer cancel()
	step := row.Steps[0]
	now := time.Now().UTC()
	step.Status, step.StartedAt = "running", &now
	if started, err := h.s.startStep(ctx, step); err != nil || !started {
		t.Fatal("step did not start", err)
	}
	stopped, err := h.s.Stop(context.Background(), row.UUID)
	if err != nil || ctx.Err() != context.Canceled {
		t.Fatal("request was not cancelled", err)
	}
	step.Status, step.CompletedAt, step.OutputText = "succeeded", &now, "late response"
	if err := h.s.finish(row, step); err != nil {
		t.Fatal(err)
	}
	final, err := h.s.Get(context.Background(), row.UUID)
	if err != nil || !reflect.DeepEqual(final, stopped) {
		t.Fatalf("late completion overwrote stop: %+v %v", final, err)
	}
}

func TestStopMissingTest(t *testing.T) {
	h := newHarness(t)
	for _, value := range []string{"1", "01900000-0000-7000-8000-000000000001"} {
		if _, err := h.s.Stop(context.Background(), value); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	}
}
