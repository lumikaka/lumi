package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"lumi/internal/project"
)

const (
	projectIdleGrace         = 5 * time.Minute
	projectIdleCheckInterval = time.Second
	projectBusyCheckTimeout  = 900 * time.Millisecond
	projectIdleErrorLogEvery = 30 * time.Second
)

type idleProjectManager interface {
	OpenProjectUUIDs() []string
	Activity(string) (project.ProjectActivity, bool)
	HasActiveWork(context.Context, string) (bool, error)
	CloseProjectIfIdle(context.Context, string) (bool, error)
}

type projectIdleState struct {
	idleSince    time.Time
	lastActivity time.Time
	wasBusy      bool
	lastErrorLog time.Time
}

type projectLifecycleController struct {
	projects  idleProjectManager
	idleGrace time.Duration
	now       func() time.Time
	states    map[string]*projectIdleState
}

func newProjectLifecycleController(projects idleProjectManager, idleGrace time.Duration) *projectLifecycleController {
	return &projectLifecycleController{projects: projects, idleGrace: idleGrace, now: time.Now, states: make(map[string]*projectIdleState)}
}

func (controller *projectLifecycleController) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := controller.evaluate(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("idle project close deferred", "error", err)
			}
		}
	}
}

func (controller *projectLifecycleController) evaluate(ctx context.Context) error {
	now := controller.now()
	openUUIDs := controller.projects.OpenProjectUUIDs()
	openSet := make(map[string]struct{}, len(openUUIDs))
	var result error
	for _, projectUUID := range openUUIDs {
		openSet[projectUUID] = struct{}{}
		if err := controller.evaluateProject(ctx, projectUUID, now); err != nil {
			result = errors.Join(result, err)
		}
	}
	for projectUUID := range controller.states {
		if _, open := openSet[projectUUID]; !open {
			delete(controller.states, projectUUID)
		}
	}
	return result
}

func (controller *projectLifecycleController) evaluateProject(ctx context.Context, projectUUID string, now time.Time) error {
	activity, open := controller.projects.Activity(projectUUID)
	if !open {
		delete(controller.states, projectUUID)
		return nil
	}
	state := controller.states[projectUUID]
	if state == nil {
		state = &projectIdleState{lastActivity: activity.LastActivity}
		controller.states[projectUUID] = state
	}
	if activity.RequestLeases > 0 || activity.PresenceLeases > 0 {
		state.lastActivity = activity.LastActivity
		state.idleSince = time.Time{}
		state.wasBusy = false
		return nil
	}
	if activity.LastActivity.After(state.lastActivity) {
		state.lastActivity = activity.LastActivity
		state.idleSince = activity.LastActivity
		state.wasBusy = false
	}

	checkContext, cancel := context.WithTimeout(ctx, projectBusyCheckTimeout)
	busy, err := controller.projects.HasActiveWork(checkContext, projectUUID)
	cancel()
	if err != nil {
		state.idleSince = time.Time{}
		if state.lastErrorLog.IsZero() || now.Sub(state.lastErrorLog) >= projectIdleErrorLogEvery {
			state.lastErrorLog = now
			return err
		}
		return nil
	}
	state.lastErrorLog = time.Time{}
	if busy {
		state.wasBusy = true
		state.idleSince = time.Time{}
		return nil
	}
	if state.wasBusy || state.idleSince.IsZero() {
		state.wasBusy = false
		state.idleSince = now
		return nil
	}
	if now.Sub(state.idleSince) < controller.idleGrace {
		return nil
	}

	closeContext, closeCancel := context.WithTimeout(ctx, projectBusyCheckTimeout)
	closed, err := controller.projects.CloseProjectIfIdle(closeContext, projectUUID)
	closeCancel()
	if err != nil {
		state.idleSince = time.Time{}
		return err
	}
	if closed {
		slog.Info("idle project closed", "project_uuid", projectUUID)
		delete(controller.states, projectUUID)
		return nil
	}
	state.idleSince = time.Time{}
	return nil
}
