package mcpserver

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
)

func activityHarness(t *testing.T) (*Service, Grant) {
	t.Helper()
	dir := t.TempDir()
	app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	projects := project.NewManager(app)
	t.Cleanup(func() { projects.Close(); app.Close() })
	p, err := projects.Create(t.Context(), "activity", project.ExplicitNewProjectParent(dir))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(app, projects, agent.NewExternalProjectAPI(nil, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := s.CreateGrant(t.Context(), p.UUID, "test client", "edit")
	if err != nil {
		t.Fatal(err)
	}
	return s, g
}
func beginTestActivity(t *testing.T, s *Service, g Grant, action, resource string) activityRecord {
	t.Helper()
	row, err := s.beginActivity(t.Context(), g, "request_api", activitySpec{action, resource, "更新章节"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return row
}
func activityThreadUUID(t *testing.T, s *Service, g Grant, row activityRecord) string {
	t.Helper()
	var result string
	err := s.projects.WithStore(t.Context(), g.ProjectUUID, func(store *project.Store) error {
		return store.DB().Raw("SELECT t.uuid FROM chat_threads t JOIN mcp_threads m ON m.thread_id=t.id WHERE m.id=?", row.MCPThreadID).Scan(&result).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func activityPage(t *testing.T, s *Service, g Grant, row activityRecord) ActivityPage {
	t.Helper()
	p, err := s.Activity(t.Context(), g.ProjectUUID, activityThreadUUID(t, s, g, row), "", "", 200)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestActivityAdmissionOrderAndLateFailure(t *testing.T) {
	s, g := activityHarness(t)
	a := beginTestActivity(t, s, g, "modify:chapter", "one")
	b := beginTestActivity(t, s, g, "modify:chapter", "one")
	c := beginTestActivity(t, s, g, "modify:chapter", "one")
	s.finishActivity(g.ProjectUUID, c, "succeeded", "")
	s.finishActivity(g.ProjectUUID, a, "succeeded", "")
	page := activityPage(t, s, g, a)
	if len(page.Items) != 2 || page.Items[0].UUID != a.UUID || page.Items[1].UUID != c.UUID {
		t.Fatalf("merged across unresolved call: %+v", page.Items)
	}
	s.finishActivity(g.ProjectUUID, b, "failed", "内容版本已变化")
	page = activityPage(t, s, g, a)
	if len(page.Items) != 3 || page.Items[1].UUID != b.UUID || !strings.Contains(page.Items[1].Text, "失败") {
		t.Fatalf("late failure order: %+v", page.Items)
	}
	// Stale reconciliation or duplicate completion cannot erase a failure.
	s.finishActivity(g.ProjectUUID, b, "succeeded", "")
	if page = activityPage(t, s, g, a); page.Items[1].Status != "failed" {
		t.Fatal("failure overwritten")
	}
}

func TestActivitySuccessfulGapMergeAndCursorRevision(t *testing.T) {
	s, g := activityHarness(t)
	a := beginTestActivity(t, s, g, "read", "")
	b := beginTestActivity(t, s, g, "read", "")
	c := beginTestActivity(t, s, g, "read", "")
	s.finishActivity(g.ProjectUUID, a, "succeeded", "")
	s.finishActivity(g.ProjectUUID, c, "succeeded", "")
	u := activityThreadUUID(t, s, g, a)
	first, err := s.Activity(t.Context(), g.ProjectUUID, u, "", "", 1)
	if err != nil || !first.CursorPagination.HasMore {
		t.Fatalf("first page: %+v %v", first, err)
	}
	s.finishActivity(g.ProjectUUID, b, "succeeded", "")
	if _, err = s.Activity(t.Context(), g.ProjectUUID, u, "", first.CursorPagination.NextCursor, 1); !errors.Is(err, ErrActivityChanged) {
		t.Fatalf("stale cursor not rejected: %v", err)
	}
	page := activityPage(t, s, g, a)
	if len(page.Items) != 1 || page.Items[0].UUID != a.UUID {
		t.Fatalf("merge lost oldest identity: %+v", page.Items)
	}
	// Resource switches and operation switches remain separate segments.
	for _, spec := range []activitySpec{{"modify:a", "one", "更新章节"}, {"modify:a", "one", "更新章节"}, {"modify:a", "two", "更新章节"}, {"modify:b", "two", "更新正文"}, {"read", "", "读取章节"}} {
		row, err := s.beginActivity(t.Context(), g, "request_api", spec, nil)
		if err != nil {
			t.Fatal(err)
		}
		s.finishActivity(g.ProjectUUID, row, "succeeded", "")
	}
	page = activityPage(t, s, g, a)
	if len(page.Items) != 5 {
		t.Fatalf("operation/resource boundaries: %+v", page.Items)
	}
	// Every page is selected after merging the full history, not before.
	var got []string
	after := ""
	for {
		p, err := s.Activity(t.Context(), g.ProjectUUID, u, "", after, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range p.Items {
			got = append(got, item.UUID)
		}
		after = p.CursorPagination.NextCursor
		if after == "" {
			break
		}
	}
	if len(got) != 5 {
		t.Fatalf("pagination lost history: %v", got)
	}
}

func TestActivitySessionUsesOnlyAdmissionsAndPersistsAcrossRestart(t *testing.T) {
	s, g := activityHarness(t)
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	a := beginTestActivity(t, s, g, "submit", "one")
	now = now.Add(29*time.Minute + 59*time.Second)
	b := beginTestActivity(t, s, g, "read", "")
	if b.MCPThreadID != a.MCPThreadID {
		t.Fatal("split before timeout")
	}
	now = now.Add(30 * time.Minute)
	// An outstanding call and its late result must not extend the session.
	s.finishActivity(g.ProjectUUID, a, "succeeded", "")
	c := beginTestActivity(t, s, g, "read", "")
	if c.MCPThreadID == a.MCPThreadID || c.Sequence != 1 {
		t.Fatal("late return extended session")
	}
	if page := activityPage(t, s, g, a); !strings.HasPrefix(page.Items[0].Text, "已提交") {
		t.Fatalf("submission not frozen: %+v", page.Items)
	}
	s.finishActivity(g.ProjectUUID, b, "succeeded", "")
	s.finishActivity(g.ProjectUUID, c, "succeeded", "")
	restarted, err := New(s.app, s.projects, s.api, nil)
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = func() time.Time { return now }
	d := beginTestActivity(t, restarted, g, "read", "")
	if d.MCPThreadID != c.MCPThreadID || d.Sequence != 2 {
		t.Fatal("restart lost session sequence")
	}
	other, _, err := s.CreateGrant(t.Context(), g.ProjectUUID, "other", "read")
	if err != nil {
		t.Fatal(err)
	}
	e := beginTestActivity(t, s, other, "read", "")
	if e.MCPThreadID == d.MCPThreadID {
		t.Fatal("mixed grants")
	}
}

func TestConcurrentAdmissionsCreateOneThread(t *testing.T) {
	s, g := activityHarness(t)
	const n = 12
	rows := make(chan activityRecord, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.beginActivity(context.Background(), g, "get_call", activitySpec{action: "read", label: "读取调用结果"}, nil)
			rows <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(rows)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	seqs := map[int64]bool{}
	var tid int64
	for row := range rows {
		if tid == 0 {
			tid = row.MCPThreadID
		}
		if row.MCPThreadID != tid || seqs[row.Sequence] {
			t.Fatal("duplicate session or sequence")
		}
		seqs[row.Sequence] = true
	}
	if len(seqs) != n || !seqs[1] || !seqs[n] {
		t.Fatal(seqs)
	}
}

func TestActivityConfirmationRecoveryAndIsolation(t *testing.T) {
	s, g := activityHarness(t)
	now := s.now().UTC()
	call := Call{UUID: newUUID(), GrantID: g.ID, IdempotencyKey: "confirm", Fingerprint: "fp", Arguments: "{}", Action: "删除章节", Status: "pending_confirmation", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := s.app.DB().Create(&call).Error; err != nil {
		t.Fatal(err)
	}
	row, err := s.beginActivity(t.Context(), g, "request_api", activitySpec{action: "operation:delete", label: "删除章节"}, &call.UUID)
	if err != nil {
		t.Fatal(err)
	}
	s.finishActivity(g.ProjectUUID, row, "pending_confirmation", "")
	if page := activityPage(t, s, g, row); page.Items[0].Status != "pending_confirmation" {
		t.Fatal(page)
	}
	s.now = func() time.Time { return now.Add(2 * time.Minute) }
	if page := activityPage(t, s, g, row); page.Items[0].Status != "expired" {
		t.Fatal(page)
	}
	if _, err = s.Activity(t.Context(), g.ProjectUUID, newUUID(), "", "", 20); !errors.Is(err, ErrActivityNotFound) {
		t.Fatal(err)
	}
	interrupted := beginTestActivity(t, s, g, "read", "")
	restarted, err := New(s.app, s.projects, s.api, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page := activityPage(t, restarted, g, interrupted); page.Items[len(page.Items)-1].Status != "interrupted" {
		t.Fatal(page)
	}
}

func TestUnrecordedAsyncSubmissionShowsUncertaintyWithoutFollowingTask(t *testing.T) {
	s, g := activityHarness(t)
	now := s.now().UTC()
	call := Call{UUID: newUUID(), GrantID: g.ID, IdempotencyKey: "async-crash", Fingerprint: "fp", Arguments: "{}", Action: "生成图片", Status: "executing", Async: true, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := s.app.DB().Create(&call).Error; err != nil {
		t.Fatal(err)
	}
	row, err := s.beginActivity(t.Context(), g, "request_api", activitySpec{action: "submit", label: "生成图片"}, &call.UUID)
	if err != nil {
		t.Fatal(err)
	}
	s.activityRunning.Delete(row.UUID) // process lost after accepting the operation
	page := activityPage(t, s, g, row)
	if len(page.Items) != 1 || page.Items[0].Status != "interrupted" {
		t.Fatalf("hidden unresolved submission: %+v", page.Items)
	}
	var persisted Call
	if err = s.app.DB().First(&persisted, call.ID).Error; err != nil || persisted.Status != "executing" {
		t.Fatal("history changed execution recovery")
	}
	// Once existing MCP recovery confirms submission, the unknown result can be
	// resolved without overwriting a real failure or following task progress.
	call.Status = "succeeded"
	s.syncCallActivity(g.ProjectUUID, call)
	page = activityPage(t, s, g, row)
	if len(page.Items) != 1 || page.Items[0].Status != "succeeded" || !strings.HasPrefix(page.Items[0].Text, "已提交") {
		t.Fatal(page.Items)
	}
}
