package server

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"lumi/internal/project"
)

type lifecycleProjectFake struct {
	open       map[string]bool
	activity   map[string]project.ProjectActivity
	busy       map[string]bool
	busyErr    map[string]error
	closeErr   error
	closeCalls []string
}

func newLifecycleProjectFake(projectUUIDs ...string) *lifecycleProjectFake {
	fake := &lifecycleProjectFake{
		open: make(map[string]bool), activity: make(map[string]project.ProjectActivity),
		busy: make(map[string]bool), busyErr: make(map[string]error),
	}
	for _, projectUUID := range projectUUIDs {
		fake.open[projectUUID] = true
	}
	return fake
}

func (fake *lifecycleProjectFake) OpenProjectUUIDs() []string {
	var result []string
	for projectUUID, open := range fake.open {
		if open {
			result = append(result, projectUUID)
		}
	}
	sort.Strings(result)
	return result
}

func (fake *lifecycleProjectFake) Activity(projectUUID string) (project.ProjectActivity, bool) {
	return fake.activity[projectUUID], fake.open[projectUUID]
}

func (fake *lifecycleProjectFake) HasActiveWork(_ context.Context, projectUUID string) (bool, error) {
	return fake.busy[projectUUID], fake.busyErr[projectUUID]
}

func (fake *lifecycleProjectFake) CloseProjectIfIdle(_ context.Context, projectUUID string) (bool, error) {
	fake.closeCalls = append(fake.closeCalls, projectUUID)
	if fake.closeErr != nil {
		return false, fake.closeErr
	}
	activity := fake.activity[projectUUID]
	if !fake.open[projectUUID] || fake.busy[projectUUID] || activity.PresenceLeases > 0 || activity.RequestLeases > 0 {
		return false, nil
	}
	delete(fake.open, projectUUID)
	return true, nil
}

func TestProjectLifecycleControllerClosesEachProjectAfterGrace(t *testing.T) {
	firstUUID := "01989abc-def0-7000-8000-000000000001"
	secondUUID := "01989abc-def0-7000-8000-000000000002"
	projects := newLifecycleProjectFake(firstUUID, secondUUID)
	projects.activity[secondUUID] = project.ProjectActivity{PresenceLeases: 1}
	controller := newProjectLifecycleController(projects, 5*time.Minute)
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	controller.now = func() time.Time { return now }

	if err := controller.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Minute)
	if err := controller.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if projects.open[firstUUID] || !projects.open[secondUUID] || len(projects.closeCalls) != 1 || projects.closeCalls[0] != firstUUID {
		t.Fatalf("open=%v close calls=%v", projects.open, projects.closeCalls)
	}
}

func TestProjectLifecycleControllerResetsForPresenceAndActivity(t *testing.T) {
	projectUUID := "01989abc-def0-7000-8000-000000000001"
	projects := newLifecycleProjectFake(projectUUID)
	controller := newProjectLifecycleController(projects, 5*time.Minute)
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	controller.now = func() time.Time { return now }

	_ = controller.evaluate(context.Background())
	now = now.Add(4 * time.Minute)
	projects.activity[projectUUID] = project.ProjectActivity{PresenceLeases: 2}
	_ = controller.evaluate(context.Background())
	projects.activity[projectUUID] = project.ProjectActivity{LastActivity: now.Add(time.Second)}
	now = now.Add(4 * time.Minute)
	_ = controller.evaluate(context.Background())
	if len(projects.closeCalls) != 0 {
		t.Fatalf("activity did not reset idle timer: %v", projects.closeCalls)
	}
	now = now.Add(5 * time.Minute)
	_ = controller.evaluate(context.Background())
	if len(projects.closeCalls) != 1 || projects.closeCalls[0] != projectUUID {
		t.Fatalf("close calls = %v", projects.closeCalls)
	}
}

func TestProjectLifecycleControllerStartsGraceAfterBusyWorkCompletes(t *testing.T) {
	projectUUID := "01989abc-def0-7000-8000-000000000001"
	projects := newLifecycleProjectFake(projectUUID)
	projects.busy[projectUUID] = true
	controller := newProjectLifecycleController(projects, 5*time.Minute)
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	controller.now = func() time.Time { return now }
	_ = controller.evaluate(context.Background())
	now = now.Add(10 * time.Minute)
	projects.busy[projectUUID] = false
	if err := controller.evaluate(context.Background()); err != nil || !projects.open[projectUUID] {
		t.Fatalf("project closed when work completed: open=%v error=%v", projects.open, err)
	}
	now = now.Add(5 * time.Minute)
	if err := controller.evaluate(context.Background()); err != nil || projects.open[projectUUID] {
		t.Fatalf("project not closed after post-work grace: open=%v error=%v", projects.open, err)
	}
}

func TestProjectLifecycleControllerDefersFailedChecks(t *testing.T) {
	projectUUID := "01989abc-def0-7000-8000-000000000001"
	projects := newLifecycleProjectFake(projectUUID)
	projects.busyErr[projectUUID] = errors.New("status unavailable")
	controller := newProjectLifecycleController(projects, 5*time.Minute)
	if err := controller.evaluate(context.Background()); err == nil || !projects.open[projectUUID] {
		t.Fatalf("failed check open=%v error=%v", projects.open, err)
	}
}

func TestOpenProjectChangedPayloadUsesOnlyPublicLifecycleFields(t *testing.T) {
	payload := openProjectChangedPayload(project.LifecycleEvent{
		ProjectUUID: "01989abc-def0-7000-8000-000000000001",
		Open:        true,
	})
	if len(payload) != 2 || payload["project_uuid"] != "01989abc-def0-7000-8000-000000000001" || payload["open"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	if _, leaked := payload["id"]; leaked {
		t.Fatalf("payload leaked internal id: %#v", payload)
	}
	if _, leaked := payload["root_path"]; leaked {
		t.Fatalf("payload leaked local path: %#v", payload)
	}
}
