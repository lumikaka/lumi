package textmcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"lumi/internal/project"
)

func TestReadOnlyAuthoringReadsCommittedTextWithoutProjectionRepair(t *testing.T) {
	s, root, uuid := harness(t)
	profile := object(t, invoke(t, s, root, "story_profile", map[string]any{"action": "get"}))
	object(t, invoke(t, s, root, "story_profile", map[string]any{"action": "write", "story_md": "# Committed book", "expected_revision": profile["revision"]}))
	read := func(name, raw string) map[string]any {
		t.Helper()
		return object(t, s.CallReadOnly(WithExpectedProjectUUID(t.Context(), uuid), root, name, json.RawMessage(raw)))
	}
	// A normal read reconciles this file. A read-only read must leave both
	// the committed profile and the external file untouched.
	external := "# Unreconciled external file\n"
	if err := os.WriteFile(filepath.Join(root, "STORY.md"), []byte(external), 0600); err != nil {
		t.Fatal(err)
	}
	before := read("story_profile", `{"action":"get"}`)
	if before["story_md"] != "# Committed book" || before["projection_state"] != "synced" {
		t.Fatalf("read-only profile changed: %v", before)
	}
	for _, tool := range []struct{ name, raw string }{
		{"project_overview", `{}`}, {"project_setup", `{"action":"get"}`},
		{"chapters", `{"action":"list"}`}, {"settings", `{"action":"list"}`},
		{"book_outline", `{}`}, {"prompt_catalog", `{}`},
		{"render_prompt", `{"prompt_group":"story","prompt_key":"story_profile","target":"story_profile","variables":{"chapter_count":"1"}}`},
	} {
		read(tool.name, tool.raw)
	}
	after := read("story_profile", `{"action":"get"}`)
	if after["revision"] != before["revision"] || after["projection_state"] != before["projection_state"] {
		t.Fatal("read-only call reconciled the stored projection")
	}
	bytes, err := os.ReadFile(filepath.Join(root, "STORY.md"))
	if err != nil || string(bytes) != external {
		t.Fatalf("external file changed: %q %v", bytes, err)
	}
	if _, err := s.Projects.CloseProject(t.Context(), uuid); err != nil {
		t.Fatal(err)
	}
	r := s.CallReadOnly(t.Context(), root, "project_overview", json.RawMessage(`{}`))
	if code(r) != "read_only_project_not_open" || s.Projects.IsOpen(uuid) {
		t.Fatalf("read-only opened a closed project: %+v", r.Error)
	}
}

func TestReadOnlyDeniesAllWritesBeforeDispatchAndRestoresConnection(t *testing.T) {
	s, root, uuid := harness(t)
	for _, tool := range []struct{ name, raw string }{
		{"project_init", `{"idea":"no"}`},
		{"project_setup", `{"action":"finalize","expected_revision":0}`},
		{"story_profile", `{"action":"write","story_md":"no","expected_revision":0}`},
		{"export_book", `{"format":"markdown","allow_missing_images":true}`},
		{"pages", `{"action":"select_image","chapter_uuid":"` + uuid + `","page_uuid":"` + uuid + `","variant_uuid":"` + uuid + `","expected_revision":0}`},
	} {
		empty := t.TempDir()
		if got := code(s.CallReadOnly(t.Context(), empty, tool.name, json.RawMessage(tool.raw))); got != "read_only_context" {
			t.Fatalf("%s: %s", tool.name, got)
		}
		entries, err := os.ReadDir(empty)
		if err != nil || len(entries) != 0 {
			t.Fatal("rejected write created files")
		}
	}
	err := s.Projects.WithStore(t.Context(), uuid, func(store *project.Store) error {
		ctx, cancel := context.WithCancel(t.Context())
		err := store.ReadOnly(ctx, func(view *project.Store) error {
			if err := view.DB().Exec("UPDATE projects SET name = 'must not commit'").Error; err == nil {
				t.Fatal("read-only connection accepted a write")
			}
			cancel()
			return ctx.Err()
		})
		if err == nil {
			t.Fatal("cancelled callback unexpectedly succeeded")
		}
		// Same pool, after cancellation: query_only must be reset.
		return store.DB().Exec("UPDATE projects SET name = name").Error
	})
	if err != nil {
		t.Fatal(err)
	}
	if object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)["name"] == "must not commit" {
		t.Fatal("read-only write changed the project")
	}
}
