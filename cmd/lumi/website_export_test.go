package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Real host-specific tools/list -> tools/call -> shared CLI -> River -> preview.
func TestPluginWebsiteStdioDelivery(t *testing.T) {
	for _, host := range []string{"codex", "claude"} {
		t.Run(host, func(t *testing.T) {
			opt := cliOptions(t)
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			client := mcpHostClient(t, opt, root, host)
			listed, err := client.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, tool := range listed.Tools {
				if tool.Name == "export_website" {
					found = true
				}
			}
			if !found {
				t.Fatal("missing export_website")
			}
			call := func(name string, args map[string]any) map[string]any { return callText(t, client, name, args, "") }
			empty := map[string]any{}
			init := call("project_init", map[string]any{"idea": "A star", "name": "网站验收"})
			projectUUID := field(init, "project")["uuid"].(string)
			setup := call("project_setup", map[string]any{"action": "update", "expected_revision": field(init, "setup")["revision"], "project_name": "Star reader", "overall_style": "Watercolor", "picture_book": map[string]any{"format": "classic_picture_book", "aspect_ratio": map[string]any{"mode": "landscape"}}})
			call("project_setup", map[string]any{"action": "finalize", "expected_revision": setup["revision"]})
			callText(t, mcpHostClient(t, opt, root, host, "--read-only"), "export_website", empty, "read_only_context")
			ch := call("chapters", map[string]any{"action": "create", "chapter_code": "vol01.ch01", "title": "Stars", "content": "Night sky", "expected_revision": field(call("project_overview", empty), "project")["revision"]})["uuid"]
			page := func(title string) map[string]any {
				state := call("pages", map[string]any{"action": "list", "chapter_uuid": ch})
				return call("pages", map[string]any{"action": "create", "chapter_uuid": ch, "title": title, "description_md": "Star overhead", "storyboard_md": "A star", "expected_revision": field(state, "state")["revision"]})
			}
			first := page("First star")
			callText(t, client, "export_website", map[string]any{"allow_missing_images": true}, "comic_export_empty")
			prepared := call("prepare_page_image", map[string]any{"page_uuid": first["uuid"], "setting_uuids": []string{}})
			var buf bytes.Buffer
			if err = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 48, 32))); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "star.png")
			if err = os.WriteFile(file, buf.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			call("import_image", map[string]any{"target_uuid": first["uuid"], "slot": "page_image", "expected_revision": first["revision"], "path": file, "prompt": prepared["generation_prompt"], "preparation": prepared["preparation"]})
			status := cliRun(t, opt, "status", 0)
			ws := joinProject(t, status.Endpoint, projectUUID)
			result := call("export_website", empty)
			expectHint(t, ws, "website:changed")
			if result["done"] != true || result["output_directory"] != filepath.Join(root, "website") || result["preview_url"] != status.Endpoint+"/media/projects/"+projectUUID+"/website/index.html" {
				t.Fatal(result)
			}
			resp, err := http.Get(result["preview_url"].(string))
			if err != nil {
				t.Fatal(err)
			}
			html, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != 200 || !bytes.Contains(html, []byte("./static/reader.js")) {
				t.Fatal("preview unavailable", err)
			}
			// Disconnect and edit before replay. Same invocation must bypass live
			// readiness and return the original task, not create a partial update.
			_ = client.Close()
			client = mcpHostClient(t, opt, root, host)
			lostReply := call("export_website", empty)
			if lostReply["task_uuid"] != result["task_uuid"] {
				t.Fatal("lost terminal reply generated another website", lostReply)
			}
			page("Missing")
			replay := call("export_website", map[string]any{"idempotency_key": result["idempotency_key"]})
			if result["task_uuid"] != replay["task_uuid"] {
				t.Fatal("replay generated again", replay)
			}
			missing := callText(t, client, "export_website", empty, "comic_export_incomplete")
			if !strings.Contains(missing["details"].(string), "1 页") {
				t.Fatal(missing)
			}
			updated := call("export_website", map[string]any{"allow_missing_images": true})
			if updated["done"] != true || updated["generation_uuid"] == result["generation_uuid"] {
				t.Fatal(updated)
			}
			expectHint(t, ws, "website:changed")
			// The desktop card's REST fact read sees the same task and image reuse.
			rest := apiRequest(t, status.Endpoint, "GET", "/api/v1/projects/"+projectUUID+"/website", "", "", "", "", 200)
			var card map[string]any
			if err = json.Unmarshal(rest.data, &card); err != nil || field(card, "latest")["task_uuid"] != updated["task_uuid"] || field(card, "latest")["reused_images"] != float64(1) || card["available"] != true {
				t.Fatal(card, err)
			}
			// A different successful publication must never satisfy a stale replay.
			callText(t, client, "export_website", map[string]any{"idempotency_key": result["idempotency_key"]}, "comic_export_readiness_changed")
		})
	}
}
