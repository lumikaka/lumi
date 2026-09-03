package projectcreation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/files"
	"lumi/internal/project"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func creationReferencePNG(t *testing.T) []byte {
	t.Helper()
	imageValue := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			imageValue.Set(x, y, color.RGBA{R: uint8(20 + x), G: uint8(40 + y), B: 120, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, imageValue); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

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

func (fake *creationProjectsFake) WithStore(context.Context, string, func(*project.Store) error) error {
	return errors.New("creationProjectsFake does not expose a project store")
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

type creationEvent struct {
	topic   string
	event   string
	payload any
}

type creationPublisherFake struct{ events []creationEvent }

func (publisher *creationPublisherFake) Broadcast(topic, event string, payload any) {
	publisher.events = append(publisher.events, creationEvent{topic: topic, event: event, payload: payload})
}

type panickingCreationPublisher struct{}

func (panickingCreationPublisher) Broadcast(string, string, any) { panic("realtime unavailable") }

func (fake *creationAgentsFake) ValidateBootstrapTextModel(context.Context) error {
	fake.preflight++
	return fake.preflightErr
}

func (fake *creationAgentsFake) BootstrapConversation(_ context.Context, projectUUID, sessionUUID, input string, _ []agent.ReferenceInput) (agent.BootstrapConversationResult, error) {
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

func TestCreationSessionPersistsAndMatchesOrderedReferenceManifest(t *testing.T) {
	ctx := context.Background()
	app := creationTestApp(t)
	projects := &creationProjectsFake{app: app}
	agents := &creationAgentsFake{threadUUID: creationTestUUID(t), turnUUID: creationTestUUID(t)}
	service := NewService(app, projects, agents)
	references := []ReferenceFileInput{
		{OriginalFilename: "first.png", MIMEType: "image/png", ByteSize: 123},
		{OriginalFilename: "second.webp", MIMEType: "IMAGE/WEBP", ByteSize: 456},
	}
	session, err := service.CreateWithReferences(ctx, "Create from two references", "home-reference-manifest", references)
	if err != nil || session.Status != StatusFailed || len(session.References) != 2 {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	if session.References[0].Position != 1 || session.References[0].MIMEType != "image/png" || session.References[0].FileUUID != "" || session.References[0].ReferenceRole != "auto" || session.References[0].Title != "first" || session.References[0].Instruction != "" || !session.References[0].IncludeInYolo || session.References[0].PlanSource != PlanSourceSystemDefault || session.References[1].Position != 2 || session.References[1].MIMEType != "image/webp" || session.References[1].ReferenceRole != "auto" || session.References[1].Title != "second" || !session.References[1].IncludeInYolo || session.References[1].PlanSource != PlanSourceSystemDefault {
		t.Fatalf("references=%+v", session.References)
	}
	replayed, err := service.CreateWithReferences(ctx, "Create from two references", "home-reference-manifest", references)
	if err != nil || replayed.UUID != session.UUID || len(replayed.References) != 2 {
		t.Fatalf("replayed=%+v err=%v", replayed, err)
	}
	changed := append([]ReferenceFileInput(nil), references...)
	changed[1].ByteSize++
	if _, err := service.CreateWithReferences(ctx, "Create from two references", "home-reference-manifest", changed); err == nil {
		t.Fatal("idempotency key accepted another ordered reference manifest")
	} else {
		var creationErr *Error
		if !errors.As(err, &creationErr) || creationErr.Code != CodeIdempotencyConflict {
			t.Fatalf("manifest conflict error=%v", err)
		}
	}
	tooMany := make([]ReferenceFileInput, MaxReferenceFiles+1)
	for index := range tooMany {
		tooMany[index] = ReferenceFileInput{OriginalFilename: "image.png", MIMEType: "image/png", ByteSize: 1}
	}
	if _, err := service.CreateWithReferences(ctx, "Too many", "home-reference-too-many", tooMany); err == nil {
		t.Fatal("reference limit was not enforced")
	} else {
		var creationErr *Error
		if !errors.As(err, &creationErr) || creationErr.Code != CodeInvalidInput {
			t.Fatalf("reference limit error=%v", err)
		}
	}
}

func TestCreationSessionRejectsClientManagedReferencePlans(t *testing.T) {
	ctx := context.Background()
	service := NewService(creationTestApp(t), &creationProjectsFake{}, &creationAgentsFake{})
	excluded := false
	cases := []ReferenceFileInput{
		{OriginalFilename: "fox.png", MIMEType: "image/png", ByteSize: 123, ReferenceRole: "style"},
		{OriginalFilename: "fox.png", MIMEType: "image/png", ByteSize: 123, Title: "Custom title"},
		{OriginalFilename: "fox.png", MIMEType: "image/png", ByteSize: 123, Instruction: "Only use the palette"},
		{OriginalFilename: "fox.png", MIMEType: "image/png", ByteSize: 123, IncludeInYolo: &excluded},
	}
	for index, reference := range cases {
		_, err := service.CreateWithReferences(ctx, "Create from a reference", fmt.Sprintf("system-managed-reference-%d", index), []ReferenceFileInput{reference})
		var creationErr *Error
		if !errors.As(err, &creationErr) || creationErr.Code != CodeReferenceSystemManaged {
			t.Fatalf("case %d error=%v", index, err)
		}
	}
	include := true
	if normalized, err := validateReferenceFiles([]ReferenceFileInput{{OriginalFilename: "fox.png", MIMEType: "image/png", ByteSize: 123, ReferenceRole: "auto", Title: "fox", IncludeInYolo: &include}}); err != nil || normalized[0].ReferenceRole != "auto" || normalized[0].Title != "fox" || !*normalized[0].IncludeInYolo {
		t.Fatalf("legacy defaults were not normalized: values=%+v err=%v", normalized, err)
	}
	longFilename := strings.Repeat("图", 200) + ".png"
	if normalized, err := validateReferenceFiles([]ReferenceFileInput{{OriginalFilename: longFilename, MIMEType: "image/png", ByteSize: 123}}); err != nil || len([]rune(normalized[0].Title)) != 160 {
		t.Fatalf("system-managed title was not safely bounded: values=%+v err=%v", normalized, err)
	}
}

func TestExistingCustomReferenceSessionResumesWithoutRewritingHistoricalPlan(t *testing.T) {
	ctx := context.Background()
	app := creationTestApp(t)
	now := time.Now().UTC()
	sessionRecord, created, err := app.CreateOrGetProjectCreationSession(ctx, appstore.ProjectCreationSession{
		UUID: creationTestUUID(t), IdempotencyKey: "historical-custom-reference", InputText: "Create from the historical session",
		Status: StatusActive, PlannedProjectUUID: creationTestUUID(t), CreatedAt: now, UpdatedAt: now,
	}, []appstore.ProjectCreationReference{{
		UUID: creationTestUUID(t), Position: 1, UploadUUID: creationTestUUID(t), FileUUID: creationTestUUID(t),
		OriginalFilename: "fox.png", DeclaredMIMEType: "image/png", DeclaredByteSize: 123,
		ReferenceRole: ReferenceRoleStyle, Title: "Historical watercolor", Instruction: "Keep only the palette",
		IncludeInYolo: false, PlanSource: PlanSourceUserConfirmed, Status: "ready", CreatedAt: now, UpdatedAt: now,
	}})
	if err != nil || !created {
		t.Fatalf("seed session=%+v created=%v err=%v", sessionRecord, created, err)
	}
	service := NewService(app, &creationProjectsFake{}, &creationAgentsFake{})
	for _, reference := range []ReferenceFileInput{
		{OriginalFilename: "fox.png", MIMEType: "image/png", ByteSize: 123},
		{OriginalFilename: "fox.png", MIMEType: "image/png", ByteSize: 123, ReferenceRole: ReferenceRoleCharacter, Title: "Attempted rewrite"},
	} {
		resumed, err := service.CreateWithReferences(ctx, sessionRecord.InputText, sessionRecord.IdempotencyKey, []ReferenceFileInput{reference})
		if err != nil || resumed.UUID != sessionRecord.UUID || resumed.Status != StatusActive {
			t.Fatalf("resumed=%+v err=%v", resumed, err)
		}
	}
	stored, err := app.ProjectCreationReferences(ctx, sessionRecord.ID)
	if err != nil || len(stored) != 1 || stored[0].ReferenceRole != ReferenceRoleStyle || stored[0].Title != "Historical watercolor" || stored[0].Instruction != "Keep only the palette" || stored[0].IncludeInYolo || stored[0].PlanSource != PlanSourceUserConfirmed {
		t.Fatalf("historical plan changed: references=%+v err=%v", stored, err)
	}
}

func TestProjectReferenceBindingGetsProjectUUIDAndNeverOverwritesSetupPlan(t *testing.T) {
	ctx := context.Background()
	app := creationTestApp(t)
	manager := project.NewManager(app)
	created, err := manager.CreateWithInput(ctx, project.CreateInput{Name: "Binding", PictureBook: &project.PictureBookInput{Format: project.PictureBookVertical}}, project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	err = manager.WithStore(ctx, created.UUID, func(store *project.Store) error {
		fileService := files.NewService(store, nil)
		file, commitErr := fileService.CommitReader(ctx, files.CommitInput{Purpose: "project_chatbot_reference", OriginalFilename: "fox.png", DisplayName: "Fox", SourceType: "imported", Metadata: map[string]any{"source": "project_creation:test"}, Reader: bytes.NewReader(creationReferencePNG(t))})
		if commitErr != nil {
			return commitErr
		}
		var projectID, fileID int64
		if queryErr := store.DB().Table("projects").Where("uuid=?", created.UUID).Pluck("id", &projectID).Error; queryErr != nil {
			return queryErr
		}
		if queryErr := store.DB().Table("files").Where("uuid=?", file.UUID).Pluck("id", &fileID).Error; queryErr != nil {
			return queryErr
		}
		sessionUUID := creationTestUUID(t)
		reference := appstore.ProjectCreationReference{UUID: creationTestUUID(t), Position: 1, ReferenceRole: ReferenceRoleCharacter, Title: "Little Fox", Instruction: "Keep the scarf", IncludeInYolo: true, PlanSource: PlanSourceUserConfirmed}
		if transactionErr := store.DB().Transaction(func(tx *gorm.DB) error {
			return bindProjectReference(tx, projectID, sessionUUID, reference, fileID, time.Now().UTC())
		}); transactionErr != nil {
			return transactionErr
		}
		var bound struct {
			UUID, ReferenceUUID, ReferenceRole, Title, Instruction, PlanSource string
			IncludeInYolo                                                      bool
		}
		if queryErr := store.DB().Table("project_creation_reference_files").Where("reference_uuid=?", reference.UUID).Take(&bound).Error; queryErr != nil {
			return queryErr
		}
		if bound.UUID == reference.UUID || bound.ReferenceUUID != reference.UUID || bound.ReferenceRole != ReferenceRoleCharacter || bound.Title != reference.Title || !bound.IncludeInYolo || bound.PlanSource != PlanSourceUserConfirmed {
			t.Fatalf("initial binding=%+v source=%+v", bound, reference)
		}
		if updateErr := store.DB().Table("project_creation_reference_files").Where("uuid=?", bound.UUID).Updates(map[string]any{"reference_role": ReferenceRoleStyle, "title": "Confirmed style", "instruction": "Only use brushwork", "include_in_yolo": false, "plan_source": "user_confirmed"}).Error; updateErr != nil {
			return updateErr
		}
		if transactionErr := store.DB().Transaction(func(tx *gorm.DB) error {
			return bindProjectReference(tx, projectID, sessionUUID, reference, fileID, time.Now().UTC())
		}); transactionErr != nil {
			return transactionErr
		}
		if queryErr := store.DB().Table("project_creation_reference_files").Where("uuid=?", bound.UUID).Take(&bound).Error; queryErr != nil {
			return queryErr
		}
		if bound.ReferenceRole != ReferenceRoleStyle || bound.Title != "Confirmed style" || bound.Instruction != "Only use brushwork" || bound.IncludeInYolo {
			t.Fatalf("rebind overwrote Setup plan: %+v", bound)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreationSessionRealtimeIsPublicAndCannotUnwindDurableCompletion(t *testing.T) {
	ctx := context.Background()
	app := creationTestApp(t)
	publisher := &creationPublisherFake{}
	projects := &creationProjectsFake{app: app}
	agents := &creationAgentsFake{threadUUID: creationTestUUID(t), turnUUID: creationTestUUID(t)}
	session, err := NewService(app, projects, agents, publisher).Create(ctx, "Realtime project", "home-realtime-project")
	if err != nil || session.Status != StatusActive || len(publisher.events) != 1 {
		t.Fatalf("session=%+v events=%+v err=%v", session, publisher.events, err)
	}
	event := publisher.events[0]
	payload, ok := event.payload.(map[string]any)
	if !ok || event.topic != "system" || event.event != "project_creation_session:changed" || payload["session_uuid"] != session.UUID || payload["project_uuid"] != session.ProjectUUID || payload["thread_uuid"] != session.ThreadUUID {
		t.Fatalf("event=%+v", event)
	}
	for _, forbidden := range []string{"id", "recent_project_id", "planned_root_path"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("realtime payload exposed %s: %+v", forbidden, payload)
		}
	}

	panickingProjects := &creationProjectsFake{app: app}
	panickingAgents := &creationAgentsFake{threadUUID: creationTestUUID(t), turnUUID: creationTestUUID(t)}
	completed, err := NewService(app, panickingProjects, panickingAgents, panickingCreationPublisher{}).Create(ctx, "Publisher panic", "home-realtime-panic")
	if err != nil || completed.Status != StatusActive {
		t.Fatalf("publisher panic unwound completion=%+v err=%v", completed, err)
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
