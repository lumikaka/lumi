package cindyarchive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
)

func archiveHarness(t *testing.T) (*Service, string, string) {
	t.Helper()
	data := t.TempDir()
	app, err := appstore.Open(data, config.SQLiteDSN(filepath.Join(data, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	m := project.NewManager(app)
	t.Cleanup(func() { _ = m.Close(); _ = app.Close() })
	root := t.TempDir()
	p, err := m.InitAt(t.Context(), root, project.CreateInput{Name: "Synthetic archive", GenerationLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	return &Service{Projects: m}, root, p.UUID
}

func importFixture(t *testing.T, s *Service, projectUUID string, v snapshot) Result {
	t.Helper()
	r, err := s.Import(t.Context(), projectUUID, v.SourceSessionID, bytes.NewReader(rawFixture(t, v)))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestProjectImportReplayBackfillAndChronologicalPagination(t *testing.T) {
	s, _, p := archiveHarness(t)
	v := fixture()
	v.Pages[0].Response.Messages = v.Pages[0].Response.Messages[:1]
	first := importFixture(t, s, p, v)
	if first.Imported != 1 || first.Replayed || first.ThreadUUID == "" || first.ImportUUID == "" {
		t.Fatalf("first=%+v", first)
	}
	again := importFixture(t, s, p, v)
	if again.Imported != 0 || !again.Replayed || again.ImportUUID != first.ImportUUID || again.ThreadUUID != first.ThreadUUID {
		t.Fatalf("replay=%+v", again)
	}
	// New export backfills earlier messages; identical user text with distinct
	// source IDs survives and sorts by source timestamp + ID, not insert order.
	backfill := importFixture(t, s, p, fixture())
	if backfill.Imported != 2 || backfill.ThreadUUID != first.ThreadUUID {
		t.Fatalf("backfill=%+v", backfill)
	}
	p1, err := s.History(t.Context(), p, first.ThreadUUID, "", "", 2)
	if err != nil || len(p1.Items) != 2 || !p1.CursorPagination.HasMore || p1.Items[0].Text != p1.Items[1].Text {
		t.Fatalf("first page=%+v %v", p1, err)
	}
	p2, err := s.History(t.Context(), p, first.ThreadUUID, "", p1.CursorPagination.NextCursor, 2)
	if err != nil || len(p2.Items) != 1 || p2.CursorPagination.HasMore || p2.Items[0].Kind != "assistant" {
		t.Fatalf("second page=%+v %v", p2, err)
	}
	prev, err := s.History(t.Context(), p, first.ThreadUUID, p2.Items[0].UUID, "", 2)
	if err != nil || len(prev.Items) != 2 || prev.Items[0].UUID != p1.Items[0].UUID {
		t.Fatalf("previous=%+v %v", prev, err)
	}
	if p1.Coverage.Kind != "manual_public_text" || len(p1.Coverage.Warnings) != 0 {
		t.Fatalf("coverage=%+v", p1.Coverage)
	}
	encoded, _ := json.Marshal(p1)
	for _, secret := range []string{`"id":`, "source_message_id", "source_text_sha256", "sort_at", "workingDir", selected} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("private field leaked: %s", secret)
		}
	}
	for _, item := range append(p1.Items, p2.Items...) {
		id, err := uuid.Parse(item.UUID)
		if err != nil || id.Version() != 7 {
			t.Fatal("external item identity is not UUIDv7")
		}
	}
	if err := s.Projects.WithStore(t.Context(), p, func(store *project.Store) error {
		for _, table := range []string{"chat_turns", "chat_runs", "chat_items", "codex_sessions"} {
			var count int64
			if err := store.DB().Table(table).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				t.Fatalf("archive wrote %s", table)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLaterSnapshotAppendsAndOlderFileDoesNotRegressCoverage(t *testing.T) {
	s, _, p := archiveHarness(t)
	first := importFixture(t, s, p, fixture())
	later := fixture()
	later.CutoffAt = "2026-11-02T13:00:00Z"
	later.CapturedAt = "2026-11-02T13:01:00Z"
	later.Pages[0].Request.To = later.CutoffAt
	later.Pages[0].Response.Query.To = later.CutoffAt
	later.Pages[0].Response.Messages = append([]sourceMessage{{ID: "new-message", SessionID: selected, Role: "assistant", CreatedAt: "2026-11-02T12:30:00.000000001Z", Content: content("稍后的新正文")}}, later.Pages[0].Response.Messages...)
	appended := importFixture(t, s, p, later)
	if appended.Imported != 1 || appended.ThreadUUID != first.ThreadUUID {
		t.Fatalf("later import=%+v", appended)
	}
	older := fixture()
	older.CapturedAt = "2026-11-02T12:05:00Z" // Distinct file, not an exact replay.
	if r := importFixture(t, s, p, older); r.Imported != 0 || r.Replayed {
		t.Fatalf("older import=%+v", r)
	}
	page, err := s.History(t.Context(), p, first.ThreadUUID, "", "", 100)
	if err != nil || len(page.Items) != 4 || page.Coverage.CutoffAt.UTC().Format("2006-01-02T15:04:05Z") != later.CutoffAt || page.Items[3].Text != "稍后的新正文" {
		t.Fatalf("coverage/history regressed: %+v %v", page, err)
	}
}

func TestChangedMissingAndCrossTaskImportsAreAtomic(t *testing.T) {
	s, _, p := archiveHarness(t)
	r := importFixture(t, s, p, fixture())
	for _, tc := range []struct {
		name, code string
		change     func(*snapshot)
	}{
		{"changed", "source_changed", func(v *snapshot) { v.Pages[0].Response.Messages[2].Content = content("changed secret") }},
		{"missing", "source_missing", func(v *snapshot) { v.Pages[0].Response.Messages = v.Pages[0].Response.Messages[:2] }},
		{"cross_task", "source_mismatch", func(v *snapshot) { v.SourceSessionID = "unselected-task" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := fixture()
			tc.change(&v)
			result, err := s.Import(t.Context(), p, selected, bytes.NewReader(rawFixture(t, v)))
			if err == nil || err.Error() != "cindy_archive_"+tc.code || result.Imported != 0 || result.ThreadUUID != "" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			page, err := s.History(t.Context(), p, r.ThreadUUID, "", "", 100)
			if err != nil || len(page.Items) != 3 || page.Items[0].Text != "再写一页。" {
				t.Fatal("failed import changed persisted history")
			}
		})
	}
}

func TestStorageFailureAndCancellationLeaveNoPartialThreadOrText(t *testing.T) {
	s, _, p := archiveHarness(t)
	if err := s.Projects.WithStore(t.Context(), p, func(store *project.Store) error {
		return store.DB().Exec(`CREATE TRIGGER reject_archive_item BEFORE INSERT ON cindy_session_items WHEN NEW.kind='assistant' BEGIN SELECT RAISE(ABORT,'PRIVATE_DRIVER_DIAGNOSTIC'); END`).Error
	}); err != nil {
		t.Fatal(err)
	}
	r, err := s.Import(t.Context(), p, selected, bytes.NewReader(rawFixture(t, fixture())))
	if err == nil || err.Error() != "cindy_archive_storage_failed" || r.Imported != 0 {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Import(ctx, p, selected, bytes.NewReader(rawFixture(t, fixture()))); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	if err := s.Projects.WithStore(t.Context(), p, func(store *project.Store) error {
		for _, table := range []string{"chat_threads", "cindy_sessions", "cindy_session_items", "cindy_session_imports"} {
			var count int64
			if err := store.DB().Table(table).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				t.Fatalf("rollback left rows in %s", table)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentImportsAndCursorProjectIsolation(t *testing.T) {
	s, _, p := archiveHarness(t)
	raw := rawFixture(t, fixture())
	var wg sync.WaitGroup
	results := make(chan Result, 4)
	errors := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			r, err := s.Import(t.Context(), p, selected, bytes.NewReader(raw))
			results <- r
			errors <- err
		})
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var thread string
	imported := 0
	for r := range results {
		imported += r.Imported
		if thread != "" && r.ThreadUUID != thread {
			t.Fatal("concurrent import created duplicate threads")
		}
		thread = r.ThreadUUID
	}
	if imported != 3 {
		t.Fatalf("concurrent imported=%d", imported)
	}
	other, err := s.Projects.InitAt(t.Context(), t.TempDir(), project.CreateInput{Name: "Other", GenerationLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	otherImport := importFixture(t, s, other.UUID, fixture())
	otherPage, _ := s.History(t.Context(), other.UUID, otherImport.ThreadUUID, "", "", 1)
	for _, request := range []struct{ project, thread, before, after string }{
		{other.UUID, thread, "", ""},
		{p, thread, "", otherPage.Items[0].UUID},
		{p, thread, "bad", ""},
		{p, thread, otherPage.Items[0].UUID, otherPage.Items[0].UUID},
	} {
		_, err := s.History(t.Context(), request.project, request.thread, request.before, request.after, 50)
		if err == nil || !strings.HasPrefix(err.Error(), "cindy_archive_") {
			t.Fatalf("invalid scope/cursor accepted: %v", err)
		}
	}
	otherTaskRaw := bytes.ReplaceAll(raw, []byte(selected), []byte("explicitly-selected-second-task"))
	otherTask, err := s.Import(t.Context(), p, "explicitly-selected-second-task", bytes.NewReader(otherTaskRaw))
	if err != nil || otherTask.ThreadUUID == thread || otherTask.Imported != 3 {
		t.Fatalf("second task=%+v %v", otherTask, err)
	}
	otherTaskPage, err := s.History(t.Context(), p, otherTask.ThreadUUID, "", "", 1)
	if err != nil || len(otherTaskPage.Items) != 1 {
		t.Fatal("second task history missing")
	}
	if _, err := s.History(t.Context(), p, thread, "", otherTaskPage.Items[0].UUID, 50); err == nil {
		t.Fatal("cursor from another source task accepted")
	}
}

func TestManualImportCannotBypassAnotherProjectOwnersLock(t *testing.T) {
	s, root, p := archiveHarness(t)
	data := t.TempDir()
	app, err := appstore.Open(data, config.SQLiteDSN(filepath.Join(data, "other-app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	otherManager := project.NewManager(app)
	defer otherManager.Close()
	other := &Service{Projects: otherManager}
	_, err = other.ImportAt(t.Context(), root, selected, bytes.NewReader(rawFixture(t, fixture())))
	var pe *project.Error
	if !errors.As(err, &pe) || pe.Code != project.CodeLocked {
		t.Fatalf("lock bypass: %v", err)
	}
	if err := s.Projects.WithStore(t.Context(), p, func(store *project.Store) error {
		var count int64
		if err := store.DB().Table("cindy_sessions").Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("locked project was changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
