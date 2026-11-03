package textmcp

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostSettingBoardsThroughMCPWithoutProviders(t *testing.T) {
	s, root, _ := harness(t)
	rendered := object(t, invoke(t, s, root, "render_prompt", map[string]any{"prompt_group": "premise", "prompt_key": "setting_image", "target": "setting_board", "variables": map[string]string{"input_text": "A fox and its lantern"}, "context_chapter_uuids": []string{}, "context_setting_uuids": []string{}}))
	generation := rendered["generation_prompt"].(string)
	write := rendered["write_back"].(map[string]any)
	if write["tool"] != "setting_boards" || write["action"] != "import" || !strings.Contains(generation, "A fox and its lantern") || strings.Contains(generation, "Write-back contract") {
		t.Fatalf("render=%+v", rendered)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 300, 200))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "actual-host.png")
	if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"action": "import", "path": path, "source_text": write["source_text"], "style_snapshot": write["style_snapshot"], "prompt": generation, "expected_revision": write["expected_revision"], "idempotency_key": "board-a"}
	board := object(t, invoke(t, s, root, "setting_boards", args))
	boardUUID := board["uuid"]
	listing := object(t, invoke(t, s, root, "setting_boards", map[string]any{"action": "list", "limit": 1}))
	if len(listing["items"].([]any)) != 1 {
		t.Fatal(listing)
	}
	board = object(t, invoke(t, s, root, "setting_boards", map[string]any{"action": "get", "board_uuid": boardUUID}))
	ref := board["setting_image"].(map[string]any)["asset"].(map[string]any)["uuid"]
	imageResult, err := imageToolResult(invoke(t, s, root, "read_image", map[string]any{"asset_uuid": ref}))
	if err != nil || imageResult.IsError {
		t.Fatalf("reference: %+v %v", imageResult, err)
	}
	analysis := object(t, invoke(t, s, root, "render_prompt", map[string]any{"prompt_group": "premise", "prompt_key": "asset_breakdown", "target": "setting_breakdown", "board_uuid": boardUUID, "variables": map[string]string{"input_text": "FORGED SOURCE", "image_info_json": "FORGED SIZE", "style_prompt": "FORGED STYLE"}}))
	prompt := analysis["prompt"].(string)
	if strings.Contains(prompt, "FORGED") || !strings.Contains(prompt, `"width":300`) || !strings.Contains(prompt, "A fox and its lantern") {
		t.Fatalf("analysis=%s", prompt)
	}
	write = analysis["write_back"].(map[string]any)
	if write["tool"] != "apply_setting_breakdown" || write["reference_asset_uuid"] != ref {
		t.Fatal(write)
	}
	apply := map[string]any{"board_uuid": boardUUID, "expected_revision": write["expected_revision"], "expected_premise_revision": write["expected_premise_revision"], "idempotency_key": "split-a", "assets": []any{map[string]any{"type": "character", "title": "Fox", "summary": "Red fox", "tags": []string{"character"}, "confidence": 0.98, "crop_box": map[string]any{"x": 0, "y": 0, "width": 0.5, "height": 1}}, map[string]any{"type": "prop", "title": "Lantern", "summary": "Blue lantern", "tags": []string{"prop"}, "confidence": 0.99, "crop_box": map[string]any{"x": 0.5, "y": 0, "width": 0.5, "height": 1}}}}
	result := object(t, invoke(t, s, root, "apply_setting_breakdown", apply))
	if len(result["items"].([]any)) != 2 {
		t.Fatal(result)
	}
	repeat := object(t, invoke(t, s, root, "apply_setting_breakdown", apply))
	if repeat["uuid"] != result["uuid"] {
		t.Fatal(repeat)
	}
	review := object(t, invokeReviewDetails(t, s, root, "review_settings", map[string]any{}))
	wire, _ := json.Marshal(review)
	if !strings.Contains(string(wire), "Fox") || !strings.Contains(string(wire), "Lantern") {
		t.Fatalf("review=%s", wire)
	}
}

func TestHostSettingBoardSchemaContracts(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"setting_boards", `{"action":"import","path":"/tmp/image.png","source_text":"Fox","prompt":"Fox","expected_revision":0}`},
		{"setting_boards", `{"action":"get"}`},
		{"render_prompt", `{"prompt_group":"premise","prompt_key":"asset_breakdown","target":"setting_breakdown"}`},
		{"apply_setting_breakdown", `{"board_uuid":"01970000-0000-7000-8000-000000000001","expected_revision":0,"idempotency_key":"one","assets":[]}`},
	} {
		if err := validate(tc.name, json.RawMessage(tc.raw)); err == nil {
			t.Fatalf("accepted %s", tc.raw)
		}
	}
	s, root, _ := harness(t)
	for _, text := range []string{"", strings.Repeat("x", 12001)} {
		invalid := invoke(t, s, root, "render_prompt", map[string]any{"prompt_group": "premise", "prompt_key": "setting_image", "target": "setting_board", "variables": map[string]string{"input_text": text}})
		if code(invalid) != "prompt_inputs_missing" {
			t.Fatalf("invalid source accepted: %+v", invalid)
		}
	}

	result := invoke(t, s, root, "render_prompt", map[string]any{"prompt_group": "premise", "prompt_key": "single_asset_generation", "target": "setting_board"})
	if code(result) != "invalid_arguments" {
		t.Fatalf("wrong template accepted: %+v", result)
	}
}

func TestHostSettingBoardWritesRequireSetupAndStayInBoundProject(t *testing.T) {
	s, root, _ := harness(t)
	draft := t.TempDir()
	object(t, invoke(t, s, draft, "project_init", map[string]any{"idea": "A fox story"}))
	args := map[string]any{"action": "import", "path": "/tmp/not-read-before-setup.png", "source_text": "Fox", "prompt": "Fox", "expected_revision": 0, "idempotency_key": "draft"}
	if got := code(invoke(t, s, draft, "setting_boards", args)); got != "project_setup_incomplete" {
		t.Fatalf("draft import=%s", got)
	}
	object(t, invoke(t, s, draft, "setting_boards", map[string]any{"action": "list"}))
	// An existing public UUID from a different bound project must not resolve.
	path := filepath.Join(t.TempDir(), "board.png")
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 30, 20))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	list := object(t, invoke(t, s, root, "settings", map[string]any{"action": "list"}))
	args["path"], args["expected_revision"] = path, list["premise"].(map[string]any)["revision"]
	board := object(t, invoke(t, s, root, "setting_boards", args))
	other, otherRoot, _ := harness(t)
	if got := code(invoke(t, other, otherRoot, "setting_boards", map[string]any{"action": "get", "board_uuid": board["uuid"]})); got != "production_resource_not_found" {
		t.Fatalf("cross-project board=%s", got)
	}
	apply := map[string]any{"board_uuid": board["uuid"], "expected_revision": 0, "expected_premise_revision": 0, "idempotency_key": "draft-split", "assets": []any{map[string]any{"type": "character", "title": "Fox", "summary": "Fox", "tags": []string{"character"}, "confidence": 1, "crop_box": map[string]any{"x": 0, "y": 0, "width": 1, "height": 1}}}}
	if got := code(invoke(t, s, draft, "apply_setting_breakdown", apply)); got != "project_setup_incomplete" {
		t.Fatalf("draft split=%s", got)
	}
	if got := code(invoke(t, other, otherRoot, "apply_setting_breakdown", apply)); got != "production_resource_not_found" {
		t.Fatalf("cross-project split=%s", got)
	}
}
