package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/textmcp"
)

func TestEmbeddedWorkspaceRESTReadsWritesAndModelDataSeparation(t *testing.T) {
	opt := cliOptions(t)
	root := t.TempDir()
	client := mcpClient(t, opt, root)
	init := callText(t, client, "project_init", map[string]any{"idea": "Shared workspace", "name": "Workspace"}, "")
	// Empty drafts have no canonical profile yet; on-demand summary reads
	// return an empty fact instead of creating/repairing one while browsing.
	empty := callText(t, client, "read_app_resource", map[string]any{"resource": "story-profile", "query": map[string]string{"view": "summary"}}, "")
	if empty["exists"] != false || empty["uuid"] != nil || empty["story_md"] != "" {
		t.Fatal("draft summary", empty)
	}
	setup := callText(t, client, "project_setup", map[string]any{"action": "update", "expected_revision": field(init, "setup")["revision"], "project_name": "Workspace", "overall_style": "Watercolor", "picture_book": map[string]any{"format": "classic_picture_book", "aspect_ratio": map[string]any{"mode": "landscape"}}}, "")
	callText(t, client, "project_setup", map[string]any{"action": "finalize", "expected_revision": setup["revision"]}, "")
	p := callText(t, client, "project_overview", map[string]any{}, "")
	projectUUID := field(p, "project")["uuid"].(string)
	ch := callText(t, client, "chapters", map[string]any{"action": "create", "chapter_code": "vol01.ch01", "title": "Chapter", "expected_revision": field(p, "project")["revision"]}, "")
	chapterUUID := ch["uuid"].(string)
	liveProject := callText(t, client, "read_app_resource", map[string]any{"resource": "project"}, "")
	if liveProject["open_url"] != cliRun(t, opt, "status", 0).Endpoint+"/projects/"+projectUUID+"/" {
		t.Fatal("wrong runtime Web origin", liveProject["open_url"])
	}
	state := callText(t, client, "pages", map[string]any{"action": "list", "chapter_uuid": chapterUUID}, "")
	page := callText(t, client, "pages", map[string]any{"action": "create", "chapter_uuid": chapterUUID, "title": "Page", "description_md": strings.Repeat("Complete text. ", 1000), "storyboard_md": "Scene", "expected_revision": field(state, "state")["revision"]}, "")
	resource := "chapters/" + chapterUUID + "/comic-sections/" + page["uuid"].(string)
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "read_app_resource", Arguments: map[string]any{"resource": resource}})
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	visible, _ := json.Marshal(result.StructuredContent)
	if len(visible) > 300 || strings.Contains(string(visible), "Complete text") || strings.Contains(result.Content[0].(*mcp.TextContent).Text, "Complete text") {
		t.Fatal("detail leaked into model result")
	}
	var response toolEnvelope
	raw, _ := json.Marshal(result.Meta[textmcp.AppResponseMeta])
	if err := json.Unmarshal(raw, &response); err != nil || !response.Success || response.Data["description_md"] != strings.TrimSpace(strings.Repeat("Complete text. ", 1000)) {
		t.Fatal("UI metadata missing", err, response)
	}
	listing := callText(t, client, "read_app_resource", map[string]any{"resource": "chapters", "query": map[string]string{"view": "summary", "limit": "1"}}, "")
	chapters := listing["items"].([]any)
	if len(chapters) != 1 || chapters[0].(map[string]any)["page_count"] != float64(1) {
		t.Fatal(listing)
	}
	summary := callText(t, client, "read_app_resource", map[string]any{"resource": "chapters/" + chapterUUID + "/comic-sections", "query": map[string]string{"view": "summary", "limit": "1"}}, "")
	encoded, _ := json.Marshal(summary)
	if strings.Contains(string(encoded), "Complete text") || strings.Contains(string(encoded), "content_md") {
		t.Fatal("navigation read prose")
	}
	updated := callText(t, client, "write_app_resource", map[string]any{"method": "PATCH", "resource": resource, "body": map[string]any{"title": "Edited in App", "expected_revision": page["revision"]}}, "")
	if updated["title"] != "Edited in App" {
		t.Fatal(updated)
	}
	callText(t, client, "write_app_resource", map[string]any{"method": "PATCH", "resource": resource, "body": map[string]any{"title": "Stale overwrite", "expected_revision": page["revision"]}}, "production_conflict")
	reread := callText(t, client, "pages", map[string]any{"action": "get", "chapter_uuid": chapterUUID, "page_uuid": page["uuid"]}, "")
	if reread["title"] != "Edited in App" {
		t.Fatal("REST and App do not share facts", reread)
	}
	// The same upload endpoint handles a user-selected file; bytes stay in input
	// metadata and never reappear in the model-facing result.
	image, err := os.ReadFile(filepath.Join("..", "..", "codex-plugins", "lumi", "assets", "icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	upload, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "write_app_resource", Arguments: map[string]any{
		"method": "POST", "resource": "asset-uploads", "body": map[string]any{"purpose": "comic_section_image", "file_name": "fixture.png"},
	}, Meta: mcp.Meta{"lumi/upload": map[string]any{"data_base64": base64.StdEncoding.EncodeToString(image)}}})
	if err != nil || upload.IsError {
		t.Fatal("host upload", upload, err)
	}
	payload, _ := json.Marshal(upload.Meta[textmcp.AppResponseMeta])
	var uploaded toolEnvelope
	if err = json.Unmarshal(payload, &uploaded); err != nil || !uploaded.Success || uploaded.Data["uuid"] == nil {
		t.Fatal("upload response", err, string(payload))
	}
	adopted := callText(t, client, "write_app_resource", map[string]any{"method": "POST", "resource": resource + "/images", "body": map[string]any{"upload_uuid": uploaded.Data["uuid"], "expected_revision": updated["revision"]}}, "")
	if adopted["current_image"] == nil {
		t.Fatal("image import did not use existing adoption endpoint", adopted)
	}
	versions := callText(t, client, "pages", map[string]any{"action": "get", "chapter_uuid": chapterUUID, "page_uuid": page["uuid"], "include_versions": true}, "")
	manifest := versions["image_versions"].(map[string]any)["items"].([]any)
	if len(manifest) != 1 || manifest[0].(map[string]any)["uuid"] != field(adopted, "current_image")["uuid"] {
		t.Fatal("explicit page versions", manifest)
	}
	visibleUpload, _ := json.Marshal(upload.StructuredContent)
	if len(visibleUpload) > 300 {
		t.Fatal("upload detail leaked")
	}
	wrongBinding, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "read_app_resource", Arguments: map[string]any{"resource": "project"}, Meta: mcp.Meta{textmcp.ExpectedProjectUUIDMeta: chapterUUID}})
	if err != nil || !wrongBinding.IsError {
		t.Fatal("App identity constraint was ignored", wrongBinding, err)
	}
	var bindingEnvelope toolEnvelope
	bindingWire, _ := json.Marshal(wrongBinding.StructuredContent)
	if json.Unmarshal(bindingWire, &bindingEnvelope) != nil || bindingEnvelope.Error.Code != "project_identity_mismatch" {
		t.Fatal(string(bindingWire))
	}
	callText(t, client, "read_app_resource", map[string]any{"resource": "../projects/" + projectUUID}, "app_resource_forbidden")
	callText(t, client, "read_app_resource", map[string]any{"resource": "project", "project_uuid": projectUUID}, "invalid_arguments")
	for _, view := range []struct {
		name string
		args map[string]any
	}{
		{"review_book", map[string]any{}}, {"review_chapters", map[string]any{}}, {"review_chapter", map[string]any{"chapter_uuid": chapterUUID}}, {"review_page", map[string]any{"chapter_uuid": chapterUUID, "page_uuid": page["uuid"]}},
	} {
		launch := callText(t, client, view.name, view.args, "")
		wire, _ := json.Marshal(launch)
		if len(wire) > 3000 || launch["items"] != nil || launch["story_md"] != nil || len([]rune(launch["summary"].(string))) > 301 {
			t.Fatal("launch not compact")
		}
	}
}
