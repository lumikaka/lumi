package textmcp

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/files"
	"lumi/internal/project"
)

func TestImageReferenceContentIsBoundedAndNotDuplicatedInJSON(t *testing.T) {
	s, root, _ := harness(t)
	list := object(t, invoke(t, s, root, "settings", map[string]any{"action": "list"}))
	setting := object(t, invoke(t, s, root, "settings", map[string]any{"action": "create", "asset_type": "character", "title": "Fox", "summary": "A yellow raincoat", "expected_revision": list["premise"].(map[string]any)["revision"]}))
	// High entropy source forces the JPEG byte cap to shrink the preview.
	source := image.NewNRGBA(image.Rect(0, 0, 1600, 1400))
	state := uint32(42)
	for i := 0; i < len(source.Pix); i += 4 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		source.Pix[i], source.Pix[i+1], source.Pix[i+2], source.Pix[i+3] = byte(state), byte(state>>8), byte(state>>16), 255
	}
	source.SetNRGBA(0, 0, color.NRGBA{A: 0})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "actual-host-result.png")
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	imported := object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": setting["revision"], "prompt": "Keep the fox"}))
	asset := imported["asset"].(map[string]any)
	internal := invoke(t, s, root, "read_image", map[string]any{"asset_uuid": asset["uuid"]})
	result, err := imageToolResult(internal)
	if err != nil || result.IsError || len(result.Content) != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	structured, _ := json.Marshal(result.StructuredContent)
	for _, data := range []string{text, string(structured)} {
		if strings.Contains(data, "image_data") || strings.Contains(data, path) {
			t.Fatal("binary or source path leaked into JSON")
		}
	}
	preview := result.Content[1].(*mcp.ImageContent)
	config, _, err := image.DecodeConfig(bytes.NewReader(preview.Data))
	if err != nil || max(config.Width, config.Height) > 1024 || len(preview.Data) > maxPreviewBytes {
		t.Fatalf("unbounded preview=%v bytes=%d err=%v", config, len(preview.Data), err)
	}
	wire, _ := json.Marshal(result)
	if len(wire) > MaxBytes {
		t.Fatalf("wire size=%d", len(wire))
	}
	if got := code(invoke(t, s, root, "read_image", map[string]any{"asset_uuid": "01989abc-def0-7000-8000-000000000001"})); got != files.CodeAssetNotFound {
		t.Fatalf("missing asset=%s", got)
	}
	// A non-image Asset remains unreadable even with a valid public UUID.
	var textAsset files.Asset
	if err := s.Projects.WithStore(t.Context(), internalProjectUUID(t, s, root), func(store *project.Store) error {
		var err error
		textAsset, err = files.NewService(store, nil).CommitReader(t.Context(), files.CommitInput{Purpose: "story_import", OriginalFilename: "story.txt", SourceType: "imported", Reader: strings.NewReader("Text only")})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := code(invoke(t, s, root, "read_image", map[string]any{"asset_uuid": textAsset.UUID})); got != files.CodeTypeNotAllowed {
		t.Fatalf("text read=%s", got)
	}
}

func internalProjectUUID(t *testing.T, s *Service, root string) string {
	t.Helper()
	return object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)["uuid"].(string)
}

func TestImageSchemasRequireUUIDsRevisionsAndPaths(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"import_image", `{"path":"/some.png","target_uuid":"01989abc-def0-7000-8000-000000000001","slot":"page_image"}`},
		{"import_image", `{"path":"/some.png","target_uuid":"01989abc-def0-7000-8000-000000000001","slot":"page_image","expected_revision":0,"base64":"abc"}`},
		{"image_target", `{"target_uuid":"1","slot":"page_image"}`},
		{"image_target", `{"target_uuid":"01989abc-def0-7000-8000-000000000001","slot":"other"}`},
		{"read_image", `{"asset_uuid":"01989abc-def0-7000-8000-000000000001","max_edge":10000}`},
		{"read_image", `{"path":"/some.png"}`},
	} {
		if err := validate(tc.name, json.RawMessage(tc.raw)); err == nil {
			t.Fatalf("accepted %s %s", tc.name, tc.raw)
		}
	}
}
