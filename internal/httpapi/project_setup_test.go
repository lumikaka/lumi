package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lumi/internal/project"

	"github.com/labstack/echo/v4"
)

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
