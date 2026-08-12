package jobqueue

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskRealtimePayloadUsesOnlyPublicProductIdentifiers(t *testing.T) {
	t.Parallel()
	projectUUID := "01989abc-def0-7000-8000-000000000001"
	task := Task{UUID: "01989abc-def0-7000-8000-000000000002", Kind: KindStoryChapterGeneration, ResourceUUID: "01989abc-def0-7000-8000-000000000003", Status: StatusRunning, Progress: 45, Attempt: 1}
	encoded, err := json.Marshal(taskRealtimePayload(projectUUID, task))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{projectUUID, task.UUID, task.ResourceUUID, `"progress":45`} {
		if !strings.Contains(string(encoded), required) {
			t.Fatalf("payload is missing %q: %s", required, encoded)
		}
	}
	for _, forbidden := range []string{"river", "job_id", `"id"`, "root_path"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("payload contains %q: %s", forbidden, encoded)
		}
	}
}
