package textmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
)

func TestAppResourceAssociationCSPAndReadOnlyVisibility(t *testing.T) {
	server := Protocol(func(_ context.Context, name string, _ json.RawMessage) httpapi.Envelope {
		if name != "read_app_connection" {
			t.Fatalf("unexpected call %s", name)
		}
		return httpapi.Envelope{Success: true, Data: map[string]any{"websocket_url": "ws://127.0.0.1:12345/api/v1/ws?app_ticket=secret"}}
	})
	a, b := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "app-contract", Version: "1"}, nil).Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	listed, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	reviews, appOnly := 0, 0
	for _, tool := range listed.Tools {
		if strings.HasPrefix(tool.Name, "review_") {
			reviews++
			if tool.Meta["ui"].(map[string]any)["resourceUri"] != ReviewURI {
				t.Fatal(tool.Meta)
			}
		}
		if tool.Name == "read_app_connection" {
			t.Fatal("internal connection descriptor exposed")
		}
		if tool.Name == "read_app_image" || tool.Name == "watch_app_changes" {
			appOnly++
			b, _ := json.Marshal(tool.Meta)
			if !strings.Contains(string(b), `"visibility":["app"]`) {
				t.Fatal(string(b))
			}
		}
		if strings.HasPrefix(tool.Name, "review_") || tool.Name == "read_app_image" || tool.Name == "watch_app_changes" {
			hints := tool.Annotations
			if hints == nil || !hints.ReadOnlyHint || hints.DestructiveHint == nil || *hints.DestructiveHint || hints.OpenWorldHint == nil || *hints.OpenWorldHint {
				t.Fatalf("hints=%+v", hints)
			}
		}
	}
	if reviews != 4 || appOnly != 2 {
		t.Fatalf("reviews=%d app=%d", reviews, appOnly)
	}
	resources, err := client.ListResources(t.Context(), nil)
	if err != nil || len(resources.Resources) != 1 || resources.Resources[0].MIMEType != ReviewMIME {
		t.Fatalf("resources=%+v err=%v", resources, err)
	}
	result, err := client.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: ReviewURI})
	if err != nil {
		t.Fatal(err)
	}
	resource := result.Contents[0]
	if resource.MIMEType != ReviewMIME || !strings.Contains(resource.Text, "connect-src 'none';") || !strings.Contains(resource.Text, "img-src data:;") || strings.Contains(resource.Text, "__LUMI_") || strings.Contains(resource.Text, "app_ticket=secret") || strings.Contains(resource.Text, `src="http`) {
		t.Fatal("invalid bundled resource or CSP")
	}
	meta, _ := json.Marshal(resource.Meta)
	if !strings.Contains(string(meta), `"connectDomains":[]`) || !strings.Contains(string(meta), `"resourceDomains":[]`) {
		t.Fatal(string(meta))
	}
}

func TestReviewTextFallbackAndConversationCandidateSubmission(t *testing.T) {
	s, root, _ := harness(t)
	s.Endpoint = "http://127.0.0.1:12345"
	list := object(t, invoke(t, s, root, "settings", map[string]any{"action": "list"}))
	setting := object(t, invoke(t, s, root, "settings", map[string]any{"action": "create", "asset_type": "character", "title": "狐狸", "summary": "完整角色说明", "expected_revision": list["premise"].(map[string]any)["revision"]}))
	path := filepath.Join(t.TempDir(), "host.png")
	var source bytes.Buffer
	_ = png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 300, 200)))
	_ = os.WriteFile(path, source.Bytes(), 0600)
	for i := 0; i < 2; i++ {
		object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": setting["revision"]}))
		setting = object(t, invoke(t, s, root, "settings", map[string]any{"action": "get", "setting_uuid": setting["uuid"]}))
	}
	review := invoke(t, s, root, "review_candidates", map[string]any{"kind": "setting", "setting_uuid": setting["uuid"]})
	result, err := boundedResult(review)
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	// Never read the UI resource or decode an image; all submission facts remain text.
	var envelope struct {
		Data struct {
			Submit map[string]any `json:"submit"`
			Items  []reviewItem   `json:"items"`
		} `json:"data"`
	}
	if err = json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
		t.Fatal(err)
	}
	variants := envelope.Data.Items[0].Variants
	if len(variants) != 2 || variants[0].Number != 2 || variants[1].Number != 1 || variants[1].Text != "完整角色说明" {
		t.Fatalf("variants=%+v", variants)
	}
	missing := invoke(t, s, root, "read_app_image", map[string]any{"asset_uuid": "01989abc-def0-7000-8000-000000000099"})
	failed, _ := appImageToolResult(missing)
	if !failed.IsError {
		t.Fatal("expected image fallback")
	}
	submit := envelope.Data.Submit
	delete(submit, "tool")
	submit["variant_uuid"] = variants[1].UUID
	selected := object(t, invoke(t, s, root, "settings", submit))
	if selected["current_variant"].(map[string]any)["uuid"] != variants[1].UUID {
		t.Fatal("conversation selection failed")
	}
	if got := code(invoke(t, s, root, "settings", submit)); got != "production_conflict" {
		t.Fatalf("repeated stale submission=%s", got)
	}
	// The number remains stable after selection and text settings remain complete.
	after := object(t, invoke(t, s, root, "review_settings", map[string]any{}))
	if after["items"].([]any)[0].(map[string]any)["text"] != "完整角色说明" {
		t.Fatal(after)
	}
}

func TestAppImageEnvelopeBoundsIsolationAndFailures(t *testing.T) {
	s, root, _ := harness(t)
	// Reuse the high entropy fixture's real production image pipeline in image_test.
	for _, raw := range []string{`{"asset_uuid":"1"}`, `{"asset_uuid":"01989abc-def0-7000-8000-000000000001","max_edge":255}`, `{"asset_uuid":"01989abc-def0-7000-8000-000000000001","path":"/tmp/a"}`} {
		if validate("read_app_image", json.RawMessage(raw)) == nil {
			t.Fatal(raw)
		}
	}
	missing := invoke(t, s, root, "read_app_image", map[string]any{"asset_uuid": "01989abc-def0-7000-8000-000000000099"})
	failure, err := appImageToolResult(missing)
	if err != nil || !failure.IsError {
		t.Fatal(failure, err)
	}
	for _, invalid := range []imageResult{{}, {ImageData: make([]byte, maxPreviewBytes+1)}} {
		r, _ := appImageToolResult(httpapi.Envelope{Success: true, Data: invalid})
		if !r.IsError {
			t.Fatal("invalid internal image accepted")
		}
	}
}

func TestTextReviewOnlyPublishesExistingUUIDv7Resources(t *testing.T) {
	s, root, _ := harness(t)
	result := object(t, invoke(t, s, root, "review_candidates", map[string]any{
		"kind":       "text",
		"candidates": []map[string]any{{"title": "星光", "text": "星星照亮森林。"}, {"title": "月光", "text": "月亮照亮回家的路。"}},
	}))
	var check func(any)
	check = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if key == "uuid" || strings.HasSuffix(key, "_uuid") {
					text, ok := child.(string)
					parsed, err := uuid.Parse(text)
					if !ok || err != nil || parsed.Version() != 7 {
						t.Fatalf("invalid public resource %s=%v", key, child)
					}
				}
				check(child)
			}
		case []any:
			for _, child := range value {
				check(child)
			}
		}
	}
	check(result)
	variants := result["items"].([]any)[0].(map[string]any)["variants"].([]any)
	for index, raw := range variants {
		candidate := raw.(map[string]any)
		if candidate["number"] != float64(index+1) || candidate["text"] == "" {
			t.Fatalf("text fallback lost candidate content: %v", candidate)
		}
	}
}
