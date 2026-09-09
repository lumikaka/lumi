package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/modelsettings"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func TestProjectSetupFinalizationChecksImageSizeBeforeFreezingDraft(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	projects := project.NewManager(app)
	t.Cleanup(func() { _ = projects.Close(); providers.Close(); _ = app.Close() })
	configured, err := providers.Create(ctx, provider.CreateInput{
		AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "openai/gpt-5.6-terra", DefaultImageModel: "openai/gpt-image-1.5", APIKey: "test-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := projects.CreateWithInput(ctx, project.CreateInput{Name: "Draft fixture"}, project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	projectUUID := created.UUID
	// Build the draft fixture in the isolated test directory; CreateDraftAt is
	// intentionally restricted to the user's real default project directory.
	if err := projects.WithStore(ctx, projectUUID, func(store *project.Store) error {
		var deleteGuard string
		if err := store.DB().Raw("SELECT sql FROM sqlite_master WHERE name='project_picture_book_profiles_immutable_delete'").Scan(&deleteGuard).Error; err != nil {
			return err
		}
		if err := store.DB().Exec("DROP TRIGGER project_picture_book_profiles_immutable_delete").Error; err != nil {
			return err
		}
		if err := store.DB().Exec("DELETE FROM project_picture_book_profiles").Error; err != nil {
			return err
		}
		if err := store.DB().Exec(deleteGuard).Error; err != nil {
			return err
		}
		if err := store.DB().Exec("UPDATE projects SET setup_status='draft'").Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO project_setup_drafts
			(uuid,project_id,status,revision,original_input,generation_language,generation_brief,created_at,updated_at)
			SELECT ?,id,'draft',1,'A cute mystery by the sea.','zh-Hans','A cute mystery by the sea.',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP FROM projects`, uuid.Must(uuid.NewV7()).String()).Error; err != nil {
			return err
		}
		return store.RefreshProject(ctx)
	}); err != nil {
		t.Fatal(err)
	}
	models := modelsettings.NewResolver(providers)
	handler := NewProjectSetupHandler(projects, models, nil)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	base := "/api/v1/projects/" + projectUUID
	e.GET("/api/v1/projects/:project_uuid/project-setup", handler.Show)
	e.PATCH("/api/v1/projects/:project_uuid/project-setup", handler.Update)
	e.POST("/api/v1/projects/:project_uuid/project-setup-finalizations", handler.Finalize)
	e.POST("/api/v1/projects/:project_uuid/image-generation-preflights", NewProjectImageGenerationPreflightHandler(projects, models).Create)
	updated := requestJSON(t, e, http.MethodPatch, base+"/project-setup", map[string]any{
		"expected_revision": 1, "project_name": "Tidal Mystery", "overall_style": "Cute watercolor",
		"picture_book": map[string]any{"format": "classic_picture_book"},
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("update=%d %s", updated.Code, updated.Body.String())
	}
	finalize := func(revision int) *httptest.ResponseRecorder {
		return requestJSON(t, e, http.MethodPost, base+"/project-setup-finalizations", map[string]any{"expected_revision": revision})
	}
	if stale := finalize(1); stale.Code != http.StatusConflict {
		t.Fatalf("stale revision=%d %s", stale.Code, stale.Body.String())
	}
	blocked := finalize(2)
	if blocked.Code != http.StatusUnprocessableEntity || !strings.Contains(blocked.Body.String(), `"code":"image_aspect_ratio_unsupported"`) || !strings.Contains(blocked.Body.String(), `"data":null`) {
		t.Fatalf("unsupported model=%d %s", blocked.Code, blocked.Body.String())
	}
	shown := requestJSON(t, e, http.MethodGet, base+"/project-setup", nil)
	var state struct {
		Data project.SetupState `json:"data"`
	}
	if err := json.Unmarshal(shown.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Data.SetupStatus != project.SetupStatusDraft || state.Data.Revision != 2 || state.Data.FinalPictureBook != nil {
		t.Fatalf("rejected finalization changed draft: %+v", state.Data)
	}
	if err := projects.WithStore(ctx, projectUUID, func(store *project.Store) error {
		var count int64
		if err := store.DB().Table("project_picture_book_profiles").Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			t.Fatalf("rejected finalization persisted %d profiles", count)
		}
		_, err := models.Patch(ctx, store, modelsettings.PatchInput{ExpectedRevision: 0, Changes: map[string]*modelsettings.Selection{
			modelsettings.ProjectImage: {ProviderUUID: configured.UUID, Model: sitesettings.CloudflareModelTerra},
		}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	completed := finalize(2)
	if completed.Code != http.StatusOK || !strings.Contains(completed.Body.String(), `"setup_status":"ready"`) {
		t.Fatalf("supported model=%d %s", completed.Code, completed.Body.String())
	}
	preflight := requestJSON(t, e, http.MethodPost, base+"/image-generation-preflights", map[string]any{})
	if preflight.Code != http.StatusOK || !strings.Contains(preflight.Body.String(), `"value":"1536x1152"`) {
		t.Fatalf("finalized project preflight=%d %s", preflight.Code, preflight.Body.String())
	}
	// Replaying a completed finalization must not depend on today's model settings.
	if err := projects.WithStore(ctx, projectUUID, func(store *project.Store) error {
		_, err := models.Patch(ctx, store, modelsettings.PatchInput{ExpectedRevision: 1, Changes: map[string]*modelsettings.Selection{modelsettings.ProjectImage: nil}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if replay := finalize(2); replay.Code != http.StatusOK {
		t.Fatalf("replay=%d %s", replay.Code, replay.Body.String())
	}
	if conflict := finalize(3); conflict.Code != http.StatusConflict {
		t.Fatalf("conflict=%d %s", conflict.Code, conflict.Body.String())
	}
}

func TestProjectSetupChangedPayloadContainsOnlyPublicResyncHints(t *testing.T) {
	state := project.SetupState{
		UUID: "019c0000-0000-7000-8000-000000000001", ProjectUUID: "019c0000-0000-7000-8000-000000000002",
		SetupStatus: project.SetupStatusDraft, Status: project.SetupDraftStatusPendingConfirmation, Revision: 3,
		OriginalInput: "must not leak", DraftValues: project.SetupDraftValues{ProjectName: "must not leak either"},
	}
	payload := projectSetupChangedPayload(state)
	if len(payload) != 5 || payload["project_uuid"] != state.ProjectUUID || payload["setup_uuid"] != state.UUID || payload["revision"] != state.Revision {
		t.Fatalf("payload=%+v", payload)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"must not leak", "root_path", "input_text", "original_input", `"id"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("payload leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestProjectSetupStateJSONUsesDraftValues(t *testing.T) {
	state := project.SetupState{
		ProjectUUID: "019c0000-0000-7000-8000-000000000002",
		SetupStatus: project.SetupStatusDraft,
		Status:      project.SetupDraftStatusPendingConfirmation,
		Revision:    3,
		DraftValues: project.SetupDraftValues{ProjectName: "Setup Draft", GenerationBrief: "A fox delivers a letter to the moon."},
		FieldSources: map[string]string{
			"project_name":     project.SetupSourceAgentProposed,
			"generation_brief": project.SetupSourceAgentProposed,
		},
		MissingInformation: []string{},
		ReferencePlan: project.SetupReferencePlan{Items: []project.SetupReference{{
			UUID: "019c0000-0000-7000-8000-000000000003", FileUUID: "019c0000-0000-7000-8000-000000000004", Position: 1,
			ReferenceRole: "character", Title: "Moon fox", Instruction: "Keep the scarf", IncludeInYolo: true,
			PlanSource: project.SetupSourceUserConfirmed, ThumbnailURL: "/media/projects/019c0000-0000-7000-8000-000000000002/assets/019c0000-0000-7000-8000-000000000004/content",
		}}},
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	value := string(encoded)
	if !strings.Contains(value, `"draft_values":{"project_name":"Setup Draft","generation_brief":"A fox delivers a letter to the moon."}`) {
		t.Fatalf("response missing draft_values: %s", value)
	}
	if strings.Contains(value, `"candidate"`) {
		t.Fatalf("response retained legacy candidate field: %s", value)
	}
	for _, expected := range []string{`"reference_plan":{"items":[{`, `"reference_role":"character"`, `"include_in_yolo":true`, `"thumbnail_url":"/media/projects/`} {
		if !strings.Contains(value, expected) {
			t.Fatalf("response missing %q: %s", expected, value)
		}
	}
	for _, forbidden := range []string{`"id":`, `"file_id":`, `root_path`, `/Users/`} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, value)
		}
	}
}

func TestProjectSetupReferenceSystemManagedUsesStableFailureEnvelope(t *testing.T) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodPatch, "/api/v1/projects/project/project-setup/references/reference", strings.NewReader(`{"expected_revision":1,"include_in_yolo":false}`)), recorder)
	ErrorHandler(ProjectAPIError(&project.Error{
		Code: project.CodeProjectSetupReferenceSystemManaged, Message: "视觉参考由系统自动管理",
		Details: "参考图会按附件顺序自动用于画面生成；该兼容端点不再接受计划修改。",
	}), ctx)

	var response Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusUnprocessableEntity || response.Success || response.Data != nil || response.Error == nil || response.Error.Code != project.CodeProjectSetupReferenceSystemManaged {
		t.Fatalf("status=%d response=%+v", recorder.Code, response)
	}
}
