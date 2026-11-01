package textmcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBoundProjectRejectsDifferentUUIDBeforeInitializationOrSetup(t *testing.T) {
	s, root, uuid := harness(t)
	root, err := filepath.EvalSymlinks(root) // The CLI relay supplies a canonical root.
	if err != nil {
		t.Fatal(err)
	}
	wrong := "01989abc-def0-7000-8000-000000000001"
	before := object(t, invoke(t, s, root, "project_overview", map[string]any{}))
	for _, tool := range []struct{ name, args string }{
		{"project_init", `{"idea":"must not reset","name":"wrong"}`},
		{"project_setup", `{"action":"update","expected_revision":0,"project_name":"wrong"}`},
		{"project_overview", `{}`},
	} {
		result := s.Call(WithExpectedProjectUUID(t.Context(), wrong), root, tool.name, json.RawMessage(tool.args))
		if got := code(result); got != "project_identity_mismatch" {
			t.Fatalf("%s: %s", tool.name, got)
		}
	}
	after := object(t, invoke(t, s, root, "project_overview", map[string]any{}))
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	if string(b) != string(a) {
		t.Fatal("rejected binding changed project")
	}
	correct := s.Call(WithExpectedProjectUUID(t.Context(), uuid), root, "project_overview", json.RawMessage(`{}`))
	if !correct.Success {
		t.Fatalf("correct binding: %+v", correct.Error)
	}
	empty := t.TempDir()
	result := s.Call(WithExpectedProjectUUID(t.Context(), uuid), empty, "project_init", json.RawMessage(`{"idea":"must not create"}`))
	if code(result) != "project_identity_mismatch" {
		t.Fatalf("empty directory accepted old identity: %+v", result)
	}
	entries, _ := os.ReadDir(empty)
	if len(entries) != 0 {
		t.Fatal("identity rejection created project files")
	}
}

func TestBoundProjectRejectsReplacedFilesWithSameUUID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows prevents renaming a directory containing an open SQLite file")
	}
	for _, target := range []string{"directory", "project.sqlite", ".lumi/project.lock"} {
		t.Run(target, func(t *testing.T) { testReplacedProjectFiles(t, target) })
	}
}

func testReplacedProjectFiles(t *testing.T, target string) {
	t.Helper()
	s, root, uuid := harness(t)
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Projects.CloseProject(t.Context(), uuid); err != nil {
		t.Fatal(err)
	}
	copy := filepath.Join(t.TempDir(), "copy")
	if err := os.CopyFS(copy, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	object(t, invoke(t, s, root, "project_overview", map[string]any{}))
	original, replacement := root, copy
	if target != "directory" {
		original, replacement = filepath.Join(root, target), filepath.Join(copy, target)
	}
	moved := original + "-moved"
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	defer func() {
		// Restore the original paths before the test closes its live SQLite
		// handles. The substituted directory is an independent test copy.
		_ = os.RemoveAll(original)
		if err := os.Rename(moved, original); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.Rename(replacement, original); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"", uuid} {
		for _, tool := range []struct{ name, args string }{
			{"project_overview", `{}`},
			{"project_init", `{"idea":"must not change anything"}`},
			{"project_setup", `{"action":"update","expected_revision":0,"project_name":"must not change"}`},
		} {
			result := s.Call(WithExpectedProjectUUID(t.Context(), expected), root, tool.name, json.RawMessage(tool.args))
			if got := code(result); got != "project_identity_mismatch" {
				t.Fatalf("%s reused a store after replacing %s with a same-UUID copy: got %q", tool.name, target, got)
			}
		}
	}
}
