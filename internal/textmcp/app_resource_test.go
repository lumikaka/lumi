package textmcp

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
	"testing"

	"lumi/internal/httpapi"
)

func TestAppResourceBoundaryAndMetaSeparation(t *testing.T) {
	uuid := "01989abc-def0-7000-8000-000000000001"
	for _, r := range []AppResourceRequest{
		{Method: "GET", Resource: "https://evil.test/"}, {Method: "GET", Resource: "../projects/" + uuid},
		{Method: "GET", Resource: "chapters/%2e%2e"}, {Method: "GET", Resource: "projects/" + uuid},
		{Method: "POST", Resource: "chapters/" + uuid + "/generations"},
		{Method: "DELETE", Resource: "chapters/" + uuid + "/permanent"},
		{Method: "GET", Resource: "project", Query: map[string]string{"project_uuid": uuid}},
		{Method: "PATCH", Resource: "project", Body: json.RawMessage(`{"id":1}`)},
	} {
		if ValidateAppResource(r) == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
	for _, r := range []AppResourceRequest{
		{Method: "GET", Resource: "project"}, {Method: "GET", Resource: "chapters", Query: map[string]string{"view": "summary", "after": "20", "limit": "20"}},
		{Method: "PATCH", Resource: "chapters/" + uuid, Body: json.RawMessage(`{"title":"New","expected_revision":3}`)},
		{Method: "POST", Resource: "chapters/" + uuid + "/comic-sections/" + uuid + "/image-variants/" + uuid + "/selections", Body: json.RawMessage(`{"expected_revision":3}`)},
	} {
		if err := ValidateAppResource(r); err != nil {
			t.Fatal(r, err)
		}
	}
	result, err := appResourceToolResult(httpapi.Envelope{Success: true, Data: map[string]any{"story_md": strings.Repeat("secret prose", 1000)}})
	if err != nil || result.IsError {
		t.Fatal(err, result)
	}
	visible, _ := json.Marshal(result.StructuredContent)
	if strings.Contains(string(visible), "secret prose") || strings.Contains(result.Content[0].(*mcp.TextContent).Text, "secret prose") {
		t.Fatal("UI prose leaked into model-visible result")
	}
	payload, _ := json.Marshal(result.Meta[AppResponseMeta])
	if !strings.Contains(string(payload), "secret prose") {
		t.Fatal("UI data missing from _meta")
	}
}

func TestReviewLaunchIsCompactAndKeepsLegacyChapterScope(t *testing.T) {
	s, root, projectUUID := harness(t)
	s.Endpoint = "http://127.0.0.1:49321"
	p := object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)
	chapter := object(t, invoke(t, s, root, "chapters", map[string]any{"action": "create", "chapter_code": "vol01.ch01", "title": "Chapter", "expected_revision": p["revision"]}))
	ch := chapter["uuid"].(string)
	object(t, invoke(t, s, root, "chapters", map[string]any{"action": "write", "chapter_uuid": ch, "expected_revision": chapter["revision"], "content": strings.Repeat("Long story ", 4000), "content_format": "txt"}))
	for _, call := range []struct {
		name   string
		args   map[string]any
		view   string
		target string
	}{
		{"review_book", map[string]any{}, "book", projectUUID},
		{"review_book", map[string]any{"mode": "preview"}, "reader", projectUUID},
		{"review_chapters", map[string]any{}, "chapters", projectUUID},
		{"review_chapter", map[string]any{"chapter_uuid": ch}, "chapter", ch},
		{"review_book", map[string]any{"chapter_uuid": ch}, "reader", projectUUID},
	} {
		result := object(t, invoke(t, s, root, call.name, call.args))
		if call.args["mode"] == "preview" && result["open_url"] != s.Endpoint+"/projects/"+projectUUID+"/" {
			t.Fatal("whole-work preview must link to the existing Web project page", result)
		}
		wire, _ := json.Marshal(result)
		if result["view"] != call.view || result["target_uuid"] != call.target || result["contract_version"] != float64(3) || len(wire) > 3000 || strings.Contains(string(wire), "current_story") || result["items"] != nil {
			t.Fatal(result)
		}
		if call.view == "reader" && call.args["chapter_uuid"] != nil && (result["scope"] != "chapter" || result["request"].(map[string]any)["arguments"].(map[string]any)["chapter_uuid"] != ch) {
			t.Fatal("legacy scope expanded")
		}
	}
	calls := 0
	s.AppRequest = func(ctx context.Context, project string, r AppResourceRequest) (any, error) {
		calls++
		if project != projectUUID {
			t.Fatal("project changed")
		}
		return map[string]any{"uuid": projectUUID}, nil
	}
	object(t, invoke(t, s, root, "read_app_resource", map[string]any{"resource": "project"}))
	if calls != 1 {
		t.Fatal(calls)
	}
	raw := json.RawMessage(`{"method":"PATCH","resource":"project","body":{"name":"Denied","expected_revision":0}}`)
	denied := s.CallReadOnly(t.Context(), root, "write_app_resource", raw)
	if denied.Success || denied.Error.Code != "read_only_context" || calls != 1 {
		t.Fatal("read-only operation reached mutation callback", denied)
	}
	if invoke(t, s, root, "read_app_resource", map[string]any{"resource": "project", "project_uuid": projectUUID}).Success {
		t.Fatal("caller selected project")
	}
}

func TestCandidateLaunchKeepsSuppliedProseOnlyInUIMeta(t *testing.T) {
	s, root, _ := harness(t)
	prose := strings.Repeat("UI candidate prose ", 1000)
	result, err := reviewLaunchToolResult(invoke(t, s, root, "review_candidates", map[string]any{"kind": "text", "candidates": []map[string]string{{"title": "A", "text": prose}, {"title": "B", "text": "Other choice"}}}))
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	visible, _ := json.Marshal(struct {
		Content    any
		Structured any
	}{result.Content, result.StructuredContent})
	if strings.Contains(string(visible), prose) || strings.Contains(string(visible), "_ui_arguments") {
		t.Fatal("candidate prose reached model-visible result")
	}
	ui, _ := json.Marshal(result.Meta["lumi/launch_arguments"])
	if !strings.Contains(string(ui), prose) {
		t.Fatal("candidate body missing from UI channel")
	}
}
