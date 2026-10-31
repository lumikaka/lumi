package sessionimport

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
)

func harness(t *testing.T) (*Service, string, string) {
	t.Helper()
	data := t.TempDir()
	app, err := appstore.Open(data, config.SQLiteDSN(filepath.Join(data, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(app)
	t.Cleanup(func() { _ = manager.Close(); _ = app.Close() })
	root := t.TempDir()
	p, err := manager.InitAt(t.Context(), root, project.CreateInput{Name: "Session test", GenerationLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	return &Service{Projects: manager}, root, p.UUID
}

func fixture(t *testing.T, root string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/desktop-recorded.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	path, _ := json.Marshal(root)
	return bytes.ReplaceAll(b, []byte(`"PROJECT_ROOT"`), path)
}

func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRecordedDesktopRolloutIncrementalIdempotentAndPrivate(t *testing.T) {
	s, root, projectUUID := harness(t)
	b := fixture(t, root)
	lines := bytes.SplitAfter(b, []byte("\n"))
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	write(t, path, bytes.Join(lines[:3], nil))
	first, err := s.Import(t.Context(), root, path)
	if err != nil || first.Imported != 2 || first.Skipped {
		t.Fatalf("first=%+v %v", first, err)
	}
	write(t, path, b)
	next, err := s.Import(t.Context(), root, path)
	if err != nil || next.Imported != 1 || next.ThreadUUID != first.ThreadUUID {
		t.Fatalf("next=%+v %v", next, err)
	}
	again, err := s.Import(t.Context(), root, path)
	if err != nil || again.Imported != 0 || again.ThreadUUID != first.ThreadUUID {
		t.Fatalf("again=%+v %v", again, err)
	}
	extra, err := os.ReadFile("testdata/synthetic-edges.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	write(t, path, append(b, extra...))
	edges, err := s.Import(t.Context(), root, path)
	if err != nil || edges.Imported != 3 || !slices.Contains(edges.Warnings, "record_type_unknown") {
		t.Fatalf("edges=%+v %v", edges, err)
	}
	page, err := s.History(t.Context(), projectUUID, first.ThreadUUID, "", "", 100)
	if err != nil || len(page.Items) != 6 {
		t.Fatalf("page=%+v %v", page, err)
	}
	encoded, _ := json.Marshal(page)
	for _, secret := range []string{"DO_NOT_STORE", "private.png", `"id":`, "rollout_path", "source_offset", "PROJECT_ROOT"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("leaked %q", secret)
		}
	}
	if page.Items[2].Text != "小狐狸把月亮的倒影送回湖面，安心回家。" {
		t.Fatal("final response missing")
	}
	if page.Items[3].Text != "Lumi · pages" || page.Items[4].Text != "Lumi · import_image" {
		t.Fatal("tool summary changed")
	}
	if err := s.Projects.WithStore(t.Context(), projectUUID, func(store *project.Store) error {
		for _, table := range []string{"chat_turns", "chat_runs", "chat_items"} {
			var count int64
			if err := store.DB().Table(table).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				t.Fatalf("wrote %s", table)
			}
		}
		var source string
		if err := store.DB().Raw("SELECT rollout_path FROM codex_sessions").Scan(&source).Error; err != nil {
			return err
		}
		if source != path {
			t.Fatal("original path reference lost")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownMissingTruncatedRewrittenAndPartialRollouts(t *testing.T) {
	s, root, p := harness(t)
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	for _, tc := range []struct{ name, content, warning string }{
		{"unknown", "{\"type\":\"future\"}\n", "rollout_format_unknown"},
		{"invalid", "not json\n", "rollout_format_unknown"},
		{"empty", "", "rollout_format_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			write(t, path, []byte(tc.content))
			r, err := s.Import(t.Context(), root, path)
			if err != nil || !r.Skipped || r.Imported != 0 || !slices.Contains(r.Warnings, tc.warning) {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
	for _, path := range []string{"", filepath.Join(t.TempDir(), "gone.jsonl")} {
		r, err := s.Import(t.Context(), root, path)
		if err != nil || !r.Skipped || len(r.Warnings) != 1 {
			t.Fatalf("%+v %v", r, err)
		}
	}
	b := fixture(t, root)
	write(t, path, b)
	first, err := s.Import(t.Context(), root, path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.SplitAfter(b, []byte("\n"))
	write(t, path, lines[0])
	r, err := s.Import(t.Context(), root, path)
	if err != nil || !slices.Contains(r.Warnings, "rollout_truncated") {
		t.Fatalf("%+v %v", r, err)
	}
	write(t, path, bytes.ReplaceAll(b, []byte("小狐狸"), []byte("大狐狸")))
	r, err = s.Import(t.Context(), root, path)
	if err != nil || !slices.Contains(r.Warnings, "rollout_rewritten") {
		t.Fatalf("%+v %v", r, err)
	}
	partial := []byte(`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"恢复后的回复"}]}}`)
	write(t, path, append(bytes.Clone(b), partial[:20]...))
	r, err = s.Import(t.Context(), root, path)
	if err != nil || r.Imported != 0 || !slices.Contains(r.Warnings, "rollout_incomplete_tail") {
		t.Fatalf("%+v %v", r, err)
	}
	write(t, path, append(bytes.Clone(b), partial...))
	r, err = s.Import(t.Context(), root, path)
	if err != nil || r.Imported != 0 {
		t.Fatalf("unterminated JSON=%+v %v", r, err)
	}
	write(t, path, append(append(bytes.Clone(b), partial...), '\n'))
	r, err = s.Import(t.Context(), root, path)
	if err != nil || r.Imported != 1 {
		t.Fatalf("resume=%+v %v", r, err)
	}
	page, err := s.History(t.Context(), p, first.ThreadUUID, "", "", 100)
	if err != nil || len(page.Items) != 4 {
		t.Fatalf("history lost: %+v %v", page, err)
	}
	// Moving the same file is safe; changed prefixes are never silently merged.
	moved := filepath.Join(t.TempDir(), "moved.jsonl")
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	r, err = s.Import(t.Context(), root, moved)
	if err != nil || r.Imported != 0 || r.ThreadUUID != first.ThreadUUID {
		t.Fatalf("moved=%+v %v", r, err)
	}
}

func TestProjectIsolationPaginationAndConcurrentImports(t *testing.T) {
	s, root, p := harness(t)
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	write(t, path, fixture(t, t.TempDir()))
	r, err := s.Import(t.Context(), root, path)
	if err != nil || !r.Skipped || !slices.Contains(r.Warnings, "session_project_mismatch") {
		t.Fatalf("%+v %v", r, err)
	}
	write(t, path, fixture(t, root))
	var wg sync.WaitGroup
	errors := make(chan error, 3)
	for range 3 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Import(t.Context(), root, path); errors <- err }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err = s.Import(t.Context(), root, path)
	if err != nil || r.Imported != 0 {
		t.Fatalf("concurrent imports: %+v %v", r, err)
	}
	page1, err := s.History(t.Context(), p, r.ThreadUUID, "", "", 2)
	if err != nil || len(page1.Items) != 2 || !page1.CursorPagination.HasMore {
		t.Fatalf("%+v %v", page1, err)
	}
	page2, err := s.History(t.Context(), p, r.ThreadUUID, "", page1.CursorPagination.NextCursor, 2)
	if err != nil || len(page2.Items) != 1 || page2.CursorPagination.HasMore {
		t.Fatalf("%+v %v", page2, err)
	}
	previous, err := s.History(t.Context(), p, r.ThreadUUID, page2.CursorPagination.PrevCursor, "", 2)
	if err != nil || len(previous.Items) != 2 || previous.Items[0].UUID != page1.Items[0].UUID {
		t.Fatalf("%+v %v", previous, err)
	}
	_, err = s.History(t.Context(), p, r.ThreadUUID, "", "019a0000-0000-7000-8000-000000000099", 2)
	if err != ErrCursor {
		t.Fatalf("unknown cursor: %v", err)
	}
	_, err = s.History(t.Context(), p, r.ThreadUUID, "", "", 101)
	if err != ErrCursor {
		t.Fatalf("unbounded page: %v", err)
	}
}

func TestOversizedAndAnalysisRecordsAreNotImported(t *testing.T) {
	s, root, _ := harness(t)
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	b := fixture(t, root)
	b = append(b, []byte(strings.Repeat("x", maxRecordBytes+1)+"\n")...)
	b = append(b, []byte("{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"assistant\",\"channel\":\"analysis\",\"content\":[{\"type\":\"output_text\",\"text\":\"private reasoning\"}]}}\n")...)
	write(t, path, b)
	r, err := s.Import(t.Context(), root, path)
	if err != nil || r.Imported != 3 || !slices.Contains(r.Warnings, "record_too_large") {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = s.Import(t.Context(), root, path)
	if err != nil || r.Imported != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestConcatenatedSessionMetadataRollsBack(t *testing.T) {
	s, root, p := harness(t)
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	b := fixture(t, root)
	write(t, path, b)
	first, err := s.Import(t.Context(), root, path)
	if err != nil {
		t.Fatal(err)
	}
	other := bytes.ReplaceAll(b, []byte("019a0000-0000-7000-8000-000000000001"), []byte("019a0000-0000-7000-8000-000000000002"))
	write(t, path, append(b, other...))
	if result, err := s.Import(t.Context(), root, path); err == nil || result.Imported != 0 {
		t.Fatalf("mixed sessions accepted: %+v %v", result, err)
	}
	page, err := s.History(t.Context(), p, first.ThreadUUID, "", "", 50)
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("rollback lost history: %+v %v", page, err)
	}
}
