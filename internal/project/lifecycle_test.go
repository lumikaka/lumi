package project

import (
	"context"
	"errors"
	"testing"
	"time"
)

type lifecycleRuntime struct {
	busy       bool
	activeErr  error
	stoppedFor []string
}

func (runtime *lifecycleRuntime) StopProject(_ context.Context, projectUUID string) error {
	runtime.stoppedFor = append(runtime.stoppedFor, projectUUID)
	return nil
}

func (runtime *lifecycleRuntime) HasActiveWork(context.Context, string) (bool, error) {
	return runtime.busy, runtime.activeErr
}

func TestCloseCurrentIfIdleIsBusyAndIdentitySafe(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	runtime := &lifecycleRuntime{busy: true}
	var events []LifecycleEvent
	manager.WithRuntime(runtime).WithLifecycleHook(func(event LifecycleEvent) {
		events = append(events, event)
	})
	created, err := manager.Create(ctx, "Idle lifecycle", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	closed, err := manager.CloseCurrentIfIdle(ctx, created.UUID)
	if err != nil || closed || manager.Current() == nil {
		t.Fatalf("busy close = %t, current = %+v, error = %v", closed, manager.Current(), err)
	}
	runtime.busy = false
	closed, err = manager.CloseCurrentIfIdle(ctx, "01989abc-def0-7000-8000-000000000001")
	if err != nil || closed || manager.Current() == nil {
		t.Fatalf("mismatched close = %t, current = %+v, error = %v", closed, manager.Current(), err)
	}
	closed, err = manager.CloseCurrentIfIdle(ctx, created.UUID)
	if err != nil || !closed || manager.Current() != nil {
		t.Fatalf("idle close = %t, current = %+v, error = %v", closed, manager.Current(), err)
	}
	if len(runtime.stoppedFor) != 1 || runtime.stoppedFor[0] != created.UUID {
		t.Fatalf("stopped projects = %v", runtime.stoppedFor)
	}
	if len(events) != 2 || events[0] != (LifecycleEvent{ProjectUUID: created.UUID, Open: true}) || events[1] != (LifecycleEvent{ProjectUUID: created.UUID, Open: false}) {
		t.Fatalf("lifecycle events = %+v", events)
	}
}

func TestCloseCurrentIfIdleKeepsProjectWhenBusyCheckFails(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	runtime := &lifecycleRuntime{activeErr: errors.New("busy state unavailable")}
	manager.WithRuntime(runtime)
	created, err := manager.Create(ctx, "Busy check", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	closed, err := manager.CloseCurrentIfIdle(ctx, created.UUID)
	if err == nil || closed || manager.Current() == nil {
		t.Fatalf("close = %t, current = %+v, error = %v", closed, manager.Current(), err)
	}
}

type blockingActivityRuntime struct {
	started chan struct{}
	release chan struct{}
}

func (*blockingActivityRuntime) StopProject(context.Context, string) error { return nil }

func (runtime *blockingActivityRuntime) HasActiveWork(context.Context, string) (bool, error) {
	close(runtime.started)
	<-runtime.release
	return false, nil
}

func TestCloseProjectWaitsForInFlightRuntimeActivityCheck(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	runtime := &blockingActivityRuntime{started: make(chan struct{}), release: make(chan struct{})}
	manager.WithRuntime(runtime)
	created, err := manager.Create(ctx, "Activity lease", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	activityDone := make(chan error, 1)
	go func() {
		_, err := manager.HasActiveWork(ctx, created.UUID)
		activityDone <- err
	}()
	<-runtime.started
	closeDone := make(chan error, 1)
	go func() {
		_, err := manager.CloseProject(ctx, created.UUID)
		closeDone <- err
	}()
	deadline := time.Now().Add(time.Second)
	for manager.IsOpen(created.UUID) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.IsOpen(created.UUID) {
		t.Fatal("project did not begin draining")
	}
	select {
	case err := <-closeDone:
		t.Fatalf("close completed during runtime activity query: %v", err)
	default:
	}
	close(runtime.release)
	if err := <-activityDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
}
