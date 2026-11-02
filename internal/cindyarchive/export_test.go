package cindyarchive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

const selected = "host-task-opaque"

func boolean(v bool) *bool           { return &v }
func stringPointer(v string) *string { return &v }
func content(v any) json.RawMessage  { b, _ := json.Marshal(v); return b }
func fixture() snapshot {
	p := page{Request: queryRequest{SessionIDs: []string{selected}, Roles: []string{"user", "assistant"}, To: "2026-11-02T12:00:00Z", Limit: 20, Order: "desc"}}
	p.Response.OK = true
	p.Response.HasMore = boolean(false)
	p.Response.Sessions = map[string]sourceSession{selected: {Title: "Synthetic acceptance", AgentKind: "codex", WorkingDir: "/selected/example"}}
	p.Response.Query = queryReceipt{SessionIDs: []string{selected}, To: p.Request.To, Roles: []string{"user", "assistant"}, Limit: 20, Order: "desc"}
	p.Response.Messages = []sourceMessage{
		{ID: "message-c", SessionID: selected, Role: "assistant", CreatedAt: "2026-11-02T11:00:00Z", Content: content("  正文保留首尾空白。\n")},
		{ID: "message-b", SessionID: selected, Role: "user", CreatedAt: "2026-11-02T10:00:00Z", Content: content(map[string]any{"text": "再写一页。", "images": []any{}, "files": []any{}, "slashCommandRanges": []any{}})},
		{ID: "message-a", SessionID: selected, Role: "user", CreatedAt: "2026-11-02T10:00:00Z", Content: content("再写一页。")},
	}
	return snapshot{Format: Format, Version: 1, Source: "cindy_helper.get_chat_history", SourceSessionID: selected, CapturedAt: "2026-11-02T12:01:00Z", CutoffAt: p.Request.To, Pages: []page{p}}
}
func rawFixture(t *testing.T, v snapshot) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func parseFixture(t *testing.T, v snapshot) Archive {
	t.Helper()
	a, err := Parse(context.Background(), bytes.NewReader(rawFixture(t, v)), selected)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func wantError(t *testing.T, v snapshot, code string) {
	t.Helper()
	a, err := Parse(context.Background(), bytes.NewReader(rawFixture(t, v)), selected)
	var e *Error
	if !errors.As(err, &e) || e.Code != "cindy_archive_"+code {
		t.Fatalf("code=%v want %s", err, code)
	}
	if len(a.Messages) != 0 {
		t.Fatal("failed parse returned partial messages")
	}
}

func TestStableIdentityTextAndOrdering(t *testing.T) {
	a := parseFixture(t, fixture())
	if len(a.Messages) != 3 || a.Messages[0].SourceMessageID != "message-a" || a.Messages[1].SourceMessageID != "message-b" || a.Messages[2].SourceMessageID != "message-c" {
		t.Fatalf("wrong ordering/count: %s", a.Summary())
	}
	if a.Messages[0].Text != a.Messages[1].Text {
		t.Fatal("same text different identities must both survive")
	}
	if a.Messages[2].Text != "  正文保留首尾空白。\n" {
		t.Fatal("text whitespace changed")
	}
	if len(a.Messages[0].SourceTextSHA256) != 64 || a.SourceSessionID != selected {
		t.Fatal("missing source provenance")
	}
	if strings.Contains(a.Summary(), "正文") || strings.Contains(a.Summary(), selected) || strings.Contains(a.Summary(), a.Title) {
		t.Fatal("summary leaked a body/identity/title")
	}
}

func TestExactDuplicatesAndChangedMessageConflict(t *testing.T) {
	v := fixture()
	v.Pages[0].Response.Messages = []sourceMessage{v.Pages[0].Response.Messages[0], v.Pages[0].Response.Messages[0]}
	a := parseFixture(t, v)
	if len(a.Messages) != 1 || a.DuplicateMessages != 1 {
		t.Fatal(a.Summary())
	}
	v.Pages[0].Response.Messages[1].Content = content("PRIVATE_CREDENTIAL_OR_MESSAGE")
	wantError(t, v, "message_conflict")
	_, err := Parse(context.Background(), bytes.NewReader(rawFixture(t, v)), selected)
	if strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), "message-c") {
		t.Fatal("error included private input")
	}
}

func pagedFixture() snapshot {
	v := fixture()
	first := v.Pages[0]
	second := v.Pages[0]
	first.Response.Messages = first.Response.Messages[:1]
	first.Response.HasMore = boolean(true)
	first.Response.NextCursor = stringPointer("public-cursor-1")
	second.Request.Cursor = "public-cursor-1"
	second.Response.Messages = second.Response.Messages[1:]
	v.Pages = []page{first, second}
	return v
}
func TestPaginationMustBeCompleteAndScoped(t *testing.T) {
	if len(parseFixture(t, pagedFixture()).Messages) != 3 {
		t.Fatal("lost a cursor page")
	}
	cases := []struct {
		name, code string
		edit       func(*snapshot)
	}{
		{"missing last page", "pages_incomplete", func(v *snapshot) { v.Pages = v.Pages[:1] }},
		{"wrong cursor", "pages_incomplete", func(v *snapshot) { v.Pages[1].Request.Cursor = "other" }},
		{"missing completion flag", "pages_incomplete", func(v *snapshot) { v.Pages[1].Response.HasMore = nil }},
		{"unresolved final cursor", "pages_incomplete", func(v *snapshot) { v.Pages[1].Response.HasMore = boolean(true) }},
		{"empty cursor", "cursor_invalid", func(v *snapshot) { v.Pages[0].Response.NextCursor = stringPointer("") }},
		{"filtered later page", "scope_invalid", func(v *snapshot) { v.Pages[1].Response.Query.EffectiveFrom = stringPointer("2026-11-01") }},
		{"source metadata changed", "source_metadata_changed", func(v *snapshot) {
			v.Pages[1].Response.Sessions = map[string]sourceSession{selected: {Title: "Changed"}}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { v := pagedFixture(); c.edit(&v); wantError(t, v, c.code) })
	}
}

func TestRejectUnknownOrBroaderSource(t *testing.T) {
	cases := []struct {
		name, code string
		edit       func(*snapshot)
	}{
		{"version", "format_unknown", func(v *snapshot) { v.Version = 2 }},
		{"foreign source", "source_mismatch", func(v *snapshot) { v.SourceSessionID = "other" }},
		{"foreign request", "scope_invalid", func(v *snapshot) { v.Pages[0].Request.SessionIDs = []string{"other"} }},
		{"multiple request tasks", "scope_invalid", func(v *snapshot) { v.Pages[0].Request.SessionIDs = append(v.Pages[0].Request.SessionIDs, "other") }},
		{"broader role filter", "scope_invalid", func(v *snapshot) { v.Pages[0].Request.Roles = append(v.Pages[0].Request.Roles, "thinking") }},
		{"changed receipt cutoff", "scope_invalid", func(v *snapshot) { v.Pages[0].Response.Query.To = "2026-11-03T12:00:00Z" }},
		{"rewound records", "scope_invalid", func(v *snapshot) { v.Pages[0].Response.Query.IncludeRewound = true }},
		{"foreign row", "message_scope_invalid", func(v *snapshot) { v.Pages[0].Response.Messages[0].SessionID = "other" }},
		{"opaque id control", "message_scope_invalid", func(v *snapshot) { v.Pages[0].Response.Messages[0].ID = "bad\nidentifier" }},
		{"foreign metadata", "source_mismatch", func(v *snapshot) { v.Pages[0].Response.Sessions["other"] = sourceSession{} }},
		{"system message", "message_scope_invalid", func(v *snapshot) { v.Pages[0].Response.Messages[0].Role = "system" }},
		{"reasoning message", "message_scope_invalid", func(v *snapshot) { v.Pages[0].Response.Messages[0].Role = "thinking" }},
		{"null body", "content_unknown", func(v *snapshot) { v.Pages[0].Response.Messages[0].Content = content(nil) }},
		{"unknown array", "content_unknown", func(v *snapshot) {
			v.Pages[0].Response.Messages[0].Content = content([]any{map[string]any{"type": "reasoning", "text": "PRIVATE"}})
		}},
		{"unknown object", "content_unknown", func(v *snapshot) {
			v.Pages[0].Response.Messages[0].Content = content(map[string]any{"text": "safe", "reasoning": "PRIVATE"})
		}},
		{"oversize text", "message_too_large", func(v *snapshot) {
			v.Pages[0].Response.Messages[0].Content = content(strings.Repeat("x", MaxTextBytes+1))
		}},
		{"cutoff boundary", "timestamp_invalid", func(v *snapshot) { v.Pages[0].Response.Messages[0].CreatedAt = v.CutoffAt }},
		{"bad time", "timestamp_invalid", func(v *snapshot) { v.Pages[0].Response.Messages[0].CreatedAt = "yesterday" }},
		{"out of order", "order_invalid", func(v *snapshot) { m := v.Pages[0].Response.Messages; m[0], m[1] = m[1], m[0] }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { v := fixture(); c.edit(&v); wantError(t, v, c.code) })
	}
	if _, err := Parse(context.Background(), bytes.NewReader(rawFixture(t, fixture())), ""); err == nil {
		t.Fatal("source must be explicitly selected")
	}
}

func TestOnlyTextProjectionWithExplicitOmissions(t *testing.T) {
	v := fixture()
	m := v.Pages[0].Response.Messages
	m[0].Content = content("插画 ![cover](cindy-media://blobs/example.png) data:image/png;base64,PRIVATEBYTES")
	m[1].Content = content(map[string]any{"text": "保留正文", "images": []any{"PRIVATE_IMAGE"}, "files": []any{"PRIVATE_FILE"}})
	m[2].Content = content("# AGENTS.md instructions for /private/example\nPRIVATE_INSTRUCTIONS")
	a := parseFixture(t, v)
	if len(a.Messages) != 2 || a.OmittedMessages != 1 {
		t.Fatal(a.Summary())
	}
	raw, _ := json.Marshal(a)
	if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "cindy-media://") {
		t.Fatal("omitted content leaked")
	}
	if len(a.Warnings) != 3 {
		t.Fatalf("warnings=%v", a.Warnings)
	}
}

func TestJSONBudgetsAndAmbiguity(t *testing.T) {
	valid := rawFixture(t, fixture())
	cases := [][]byte{
		append(append([]byte{}, valid...), []byte(" {}")...),
		[]byte(`{"format":"first","format":"second"}`),
		append([]byte(`{"text":"`), 255, '"', '}'),
		[]byte(strings.Repeat("[", 34) + strings.Repeat("]", 34)),
		[]byte(strings.Repeat(" ", MaxExportBytes+1)),
		bytes.Replace(valid, []byte(`"content":"  正文`), []byte(`"unknown":"  正文`), 1),
	}
	for i, raw := range cases {
		if _, err := Parse(context.Background(), bytes.NewReader(raw), selected); err == nil {
			t.Fatalf("accepted invalid input %d", i)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Parse(ctx, bytes.NewReader(valid), selected); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestEmptyProjectionIsExplicit(t *testing.T) {
	v := fixture()
	v.Pages[0].Response.Messages = nil
	v.Pages[0].Response.Sessions = nil
	a := parseFixture(t, v)
	if len(a.Messages) != 0 || a.SourceSessionID != selected {
		t.Fatal(a.Summary())
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("PRIVATE body and /private/source/path")
}

func TestReaderErrorsCannotLeakInputOrPaths(t *testing.T) {
	_, err := Parse(context.Background(), failingReader{}, selected)
	if err == nil || err.Error() != "cindy_archive_read_failed" {
		t.Fatalf("read failure was not sanitized: %v", err)
	}
}

func TestSanitizedPublicShapeFixture(t *testing.T) {
	// Authored text and IDs only; no real task transcript is checked in.
	f, err := os.Open("testdata/public-history.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	a, err := Parse(context.Background(), f, "example-host-task")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Messages) != 3 || a.Messages[0].Text != a.Messages[1].Text {
		t.Fatal(a.Summary())
	}
}
