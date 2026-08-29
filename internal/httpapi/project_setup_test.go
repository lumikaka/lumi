package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"lumi/internal/project"
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
		DraftValues: project.SetupDraftValues{ProjectName: "Setup Draft"},
		FieldSources: map[string]string{
			"project_name": project.SetupSourceAgentProposed,
		},
		MissingInformation: []string{},
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	value := string(encoded)
	if !strings.Contains(value, `"draft_values":{"project_name":"Setup Draft"}`) {
		t.Fatalf("response missing draft_values: %s", value)
	}
	if strings.Contains(value, `"candidate"`) {
		t.Fatalf("response retained legacy candidate field: %s", value)
	}
}
