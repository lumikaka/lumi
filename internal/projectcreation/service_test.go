package projectcreation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"

	"github.com/google/uuid"
)

func creationTestUUID(t *testing.T) string {
	t.Helper()
	value, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return value.String()
}

func creationTestApp(t *testing.T) *appstore.Store {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "app")
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

type creationProjectsFake struct {
	app         *appstore.Store
	planCalls   int
	createCalls int
	openCalls   int
	open        bool
	createErrs  []error
	inputs      []project.DraftCreateInput
}

func (fake *creationProjectsFake) PlanDraftProjectRoot(context.Context) (string, error) {
	fake.planCalls++
	return filepath.Join("/server-default", "Lumi-Draft"), nil
}

func (fake *creationProjectsFake) CreateDraftAt(ctx context.Context, input project.DraftCreateInput) (project.Summary, error) {
	fake.createCalls++
	fake.inputs = append(fake.inputs, input)
	if len(fake.createErrs) > 0 {
		err := fake.createErrs[0]
		fake.createErrs = fake.createErrs[1:]
		if err != nil {
			return project.Summary{}, err
		}
	}
	if err := fake.app.RecordProject(ctx, input.ProjectUUID, project.DraftProjectPlaceholderName, input.RootPath, time.Now().UTC()); err != nil {
		return project.Summary{}, err
	}
	fake.open = true
	return project.Summary{UUID: input.ProjectUUID, RootPath: input.RootPath, SetupStatus: project.SetupStatusDraft}, nil
}

func (fake *creationProjectsFake) IsOpen(string) bool { return fake.open }

func (fake *creationProjectsFake) OpenRecent(_ context.Context, projectUUID string) (project.Summary, error) {
	fake.openCalls++
	fake.open = true
	return project.Summary{UUID: projectUUID, SetupStatus: project.SetupStatusDraft}, nil
}

type creationAgentsFake struct {
	preflightErr  error
	bootstrapErrs []error
	preflight     int
	bootstrap     int
	sessions      []string
	inputs        []string
	threadUUID    string
	turnUUID      string
}

func (fake *creationAgentsFake) ValidateBootstrapTextModel(context.Context) error {
	fake.preflight++
	return fake.preflightErr
}

func (fake *creationAgentsFake) BootstrapConversation(_ context.Context, projectUUID, sessionUUID, input string) (agent.BootstrapConversationResult, error) {
	fake.bootstrap++
	fake.sessions = append(fake.sessions, sessionUUID)
	fake.inputs = append(fake.inputs, input)
	if len(fake.bootstrapErrs) > 0 {
		err := fake.bootstrapErrs[0]
		fake.bootstrapErrs = fake.bootstrapErrs[1:]
		if err != nil {
			return agent.BootstrapConversationResult{}, err
		}
	}
	return agent.BootstrapConversationResult{
		Thread: agent.Thread{UUID: fake.threadUUID, ProjectUUID: projectUUID},
		Turn:   agent.Turn{UUID: fake.turnUUID, ThreadUUID: fake.threadUUID, InputText: input},
	}, nil
}

func TestCreationSessionIsIdempotentAndPreservesOriginalInput(t *testing.T) {
	ctx := context.Background()
	app := creationTestApp(t)
	projects := &creationProjectsFake{app: app}
	agents := &creationAgentsFake{threadUUID: creationTestUUID(t), turnUUID: creationTestUUID(t)}
	service := NewService(app, projects, agents)
	input := "  A watercolor fox writes to the moon.\n"
	first, err := service.Create(ctx, input, "home-idempotency-0001")
	if err != nil || first.Status != StatusActive {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := service.Create(ctx, input, "home-idempotency-0001")
	if err != nil || second.UUID != first.UUID || second.ProjectUUID != first.ProjectUUID || second.ThreadUUID != first.ThreadUUID || second.TurnUUID != first.TurnUUID {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if projects.planCalls != 1 || projects.createCalls != 1 || agents.bootstrap != 1 || len(projects.inputs) != 1 || projects.inputs[0].InitialInput != input || agents.inputs[0] != input {
		t.Fatalf("projects=%+v agents=%+v", projects, agents)
	}
	sessions, err := app.ResumableProjectCreationSessions(ctx)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("resumable=%+v err=%v", sessions, err)
	}
	recents, err := app.RecentProjects(ctx)
	if err != nil || len(recents) != 1 || recents[0].UUID != first.ProjectUUID {
		t.Fatalf("recents=%+v err=%v", recents, err)
	}
	if _, err := service.Create(ctx, "different", "home-idempotency-0001"); err == nil {
		t.Fatal("idempotency key accepted a different input")
	} else {
		var creationErr *Error
		if !errors.As(err, &creationErr) || creationErr.Code != CodeIdempotencyConflict {
			t.Fatalf("conflict error=%v", err)
		}
	}
}

func TestCreationSessionPreflightsBeforePlanningOrFilesystemWork(t *testing.T) {
	ctx := context.Background()
	app := creationTestApp(t)
	projects := &creationProjectsFake{app: app}
	agents := &creationAgentsFake{preflightErr: errors.New("no text model"), threadUUID: creationTestUUID(t), turnUUID: creationTestUUID(t)}
	session, err := NewService(app, projects, agents).Create(ctx, "A project", "home-preflight-0001")
	if err != nil || session.Status != StatusFailed || projects.planCalls != 0 || projects.createCalls != 0 || agents.bootstrap != 0 {
		t.Fatalf("session=%+v projects=%+v agents=%+v err=%v", session, projects, agents, err)
	}
}

func TestCreationSessionRecoversProjectAndConversationCheckpoints(t *testing.T) {
	ctx := context.Background()
	app := creationTestApp(t)
	projects := &creationProjectsFake{app: app, createErrs: []error{errors.New("injected project failure"), nil}}
	agents := &creationAgentsFake{threadUUID: creationTestUUID(t), turnUUID: creationTestUUID(t)}
	service := NewService(app, projects, agents)
	failedProject, err := service.Create(ctx, "Recover project", "home-recover-project")
	if err != nil || failedProject.Status != StatusFailed {
		t.Fatalf("failed project=%+v err=%v", failedProject, err)
	}
	recoveredProject, err := service.Resume(ctx, failedProject.UUID)
	if err != nil || recoveredProject.Status != StatusActive || projects.planCalls != 1 || projects.createCalls != 2 || agents.bootstrap != 1 {
		t.Fatalf("recovered project=%+v projects=%+v agents=%+v err=%v", recoveredProject, projects, agents, err)
	}

	projects2 := &creationProjectsFake{app: app}
	agents2 := &creationAgentsFake{bootstrapErrs: []error{errors.New("injected conversation failure"), nil}, threadUUID: creationTestUUID(t), turnUUID: creationTestUUID(t)}
	service2 := NewService(app, projects2, agents2)
	failedConversation, err := service2.Create(ctx, "Recover conversation", "home-recover-conversation")
	if err != nil || failedConversation.Status != StatusFailed || projects2.createCalls != 1 || agents2.bootstrap != 1 {
		t.Fatalf("failed conversation=%+v err=%v", failedConversation, err)
	}
	recoveredConversation, err := service2.Resume(ctx, failedConversation.UUID)
	if err != nil || recoveredConversation.Status != StatusActive || projects2.createCalls != 1 || agents2.bootstrap != 2 || agents2.sessions[0] != agents2.sessions[1] {
		t.Fatalf("recovered conversation=%+v projects=%+v agents=%+v err=%v", recoveredConversation, projects2, agents2, err)
	}
}

func TestStartupReconcileCompletesFailedSessionWithoutDuplicateProject(t *testing.T) {
	ctx := context.Background()
	app := creationTestApp(t)
	projects := &creationProjectsFake{app: app}
	agents := &creationAgentsFake{bootstrapErrs: []error{errors.New("restart now")}, threadUUID: creationTestUUID(t), turnUUID: creationTestUUID(t)}
	firstService := NewService(app, projects, agents)
	failed, err := firstService.Create(ctx, "Resume after restart", "home-restart-0001")
	if err != nil || failed.Status != StatusFailed {
		t.Fatalf("failed=%+v err=%v", failed, err)
	}
	if err := NewService(app, projects, agents).Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	active, err := firstService.Get(ctx, failed.UUID)
	if err != nil || active.Status != StatusActive || projects.createCalls != 1 || agents.bootstrap != 2 {
		t.Fatalf("active=%+v projects=%+v agents=%+v err=%v", active, projects, agents, err)
	}
}
