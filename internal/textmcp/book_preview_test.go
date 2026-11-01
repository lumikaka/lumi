package textmcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFinalBookPreviewScopeOrderPaginationAndHeadlessExport(t *testing.T) {
	s, _, _ := harness(t)
	s.Endpoint = "http://127.0.0.1:12345"
	root := t.TempDir()
	init := object(t, invoke(t, s, root, "project_init", map[string]any{"idea": "A complete book", "name": "Book preview"}))
	setup := object(t, invoke(t, s, root, "project_setup", map[string]any{"action": "update", "project_name": "Book preview", "overall_style": "watercolor", "expected_revision": init["setup"].(map[string]any)["revision"], "picture_book": map[string]any{"format": "classic_picture_book", "aspect_ratio": map[string]any{"mode": "landscape"}}}))
	object(t, invoke(t, s, root, "project_setup", map[string]any{"action": "finalize", "expected_revision": setup["revision"]}))
	chapter := func(code string) string {
		project := object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)
		return object(t, invoke(t, s, root, "chapters", map[string]any{"action": "create", "chapter_code": code, "title": code, "expected_revision": project["revision"]}))["uuid"].(string)
	}
	createPage := func(ch, role string) map[string]any {
		listing := object(t, invoke(t, s, root, "pages", map[string]any{"action": "list", "chapter_uuid": ch}))
		return object(t, invoke(t, s, root, "pages", map[string]any{"action": "create", "chapter_uuid": ch, "page_role": role, "title": role, "description_md": "完整正文", "storyboard_md": "完整画面", "expected_revision": listing["state"].(map[string]any)["revision"]}))
	}
	first := chapter("vol01.ch01")
	// Create out of reading order. The domain must place covers at bookends.
	body := createPage(first, "body")
	back := createPage(first, "back_cover")
	front := createPage(first, "front_cover")
	path := filepath.Join(t.TempDir(), "generated.png")
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 120, 80))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, source.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	for _, page := range []map[string]any{body, back, front} {
		object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": page["uuid"], "slot": "page_image", "expected_revision": page["revision"]}))
	}
	// No review/resource/UI call is needed between import and a complete export.
	for _, format := range []string{"markdown", "zip", "pdf"} {
		object(t, invoke(t, s, root, "export_book", map[string]any{"format": format, "allow_missing_images": false}))
	}
	readBody := func() map[string]any {
		return object(t, invoke(t, s, root, "pages", map[string]any{"action": "get", "chapter_uuid": first, "page_uuid": body["uuid"]}))
	}
	body = readBody()
	adopted := body["current_image"].(map[string]any)
	object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": body["uuid"], "slot": "page_image", "expected_revision": body["revision"]}))
	body = readBody()
	object(t, invoke(t, s, root, "pages", map[string]any{"action": "select_image", "chapter_uuid": first, "page_uuid": body["uuid"], "variant_uuid": adopted["uuid"], "expected_revision": body["revision"]}))
	second := chapter("vol01.ch02")
	for i := 0; i < 22; i++ {
		createPage(second, "body")
	}
	var all []reviewItem
	after := ""
	for {
		args := map[string]any{"limit": 2}
		if after != "" {
			args["after"] = after
		}
		result := object(t, invoke(t, s, root, "review_book", args))
		if result["total"] != float64(25) || result["scope"] != "book" {
			t.Fatal(result)
		}
		encoded, _ := json.Marshal(result["items"])
		var items []reviewItem
		if err := json.Unmarshal(encoded, &items); err != nil {
			t.Fatal(err)
		}
		all = append(all, items...)
		pagination := result["cursor_pagination"].(map[string]any)
		if !pagination["has_more"].(bool) {
			break
		}
		after = pagination["next_cursor"].(string)
	}
	if len(all) != 25 || all[0].UUID != front["uuid"] || all[1].UUID != body["uuid"] || all[2].UUID != back["uuid"] {
		t.Fatalf("order=%+v", all)
	}
	if all[1].AssetUUID != adopted["asset"].(map[string]any)["uuid"] || len(all[1].Variants) != 0 {
		t.Fatal("preview did not use only the adopted asset")
	}
	for i, item := range all[3:] {
		if item.ChapterUUID != second || item.ChapterTitle != "vol01.ch02" || item.PageNumber != i+1 || item.AssetUUID != "" {
			t.Fatalf("page %d=%+v", i, item)
		}
	}
	scoped := invoke(t, s, root, "review_book", map[string]any{"chapter_uuid": first})
	data := object(t, scoped)
	if data["total"] != float64(3) || data["scope"] != "chapter" || !strings.HasSuffix(data["open_url"].(string), "/"+first+"/preview") {
		t.Fatal(data)
	}
	text, err := boundedResult(scoped)
	if err != nil || text.IsError {
		t.Fatal(text, err)
	}
	if !strings.Contains(text.Content[0].(*mcp.TextContent).Text, "完整正文") {
		t.Fatal("missing text fallback")
	}
	if got := code(invoke(t, s, root, "review_book", map[string]any{"chapter_uuid": "01989abc-def0-7000-8000-000000000099"})); got != "invalid_arguments" {
		t.Fatal(got)
	}
	if validate("review_book", json.RawMessage(fmt.Sprintf(`{"chapter_uuid":%q,"limit":2}`, first))) != nil {
		t.Fatal("chapter scope schema rejected")
	}
}
