package textmcp

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func createReadinessSetting(t *testing.T, s *Service, root, kind, title string) map[string]any {
	t.Helper()
	list := object(t, invoke(t, s, root, "settings", map[string]any{"action": "list"}))
	return object(t, invoke(t, s, root, "settings", map[string]any{"action": "create", "asset_type": kind, "title": title, "summary": title, "expected_revision": list["premise"].(map[string]any)["revision"]}))
}

func TestPageImageReadinessResumesBatchIntoExistingSettings(t *testing.T) {
	s, root, chapter, page := imagePromptPage(t)
	fox := createReadinessSetting(t, s, root, "character", "Fox")
	owl := createReadinessSetting(t, s, root, "character", "Owl")
	createReadinessSetting(t, s, root, "scene", "Unused later chapter")
	createReadinessSetting(t, s, root, "reference", "Source attribution, not artwork")
	request := map[string]any{"target_uuid": page, "slot": "page_image"}
	target := object(t, invoke(t, s, root, "image_target", request))
	inventory := target["setting_readiness"].(map[string]any)
	if inventory["scope"] != "project_candidates" || inventory["status"] != "selection_required" || len(inventory["items"].([]any)) != 3 || inventory["missing_count"] != float64(3) {
		t.Fatalf("inventory=%v", inventory)
	}
	request["setting_uuids"] = []string{owl["uuid"].(string), fox["uuid"].(string)}
	selected := object(t, invoke(t, s, root, "image_target", request))["setting_readiness"].(map[string]any)
	if selected["status"] != "missing_images" || selected["missing_count"] != float64(2) || selected["next_action"] != "prepare_setting_images" {
		t.Fatalf("selected=%v", selected)
	}
	// Import one real image and resume the normal batch breakdown into the two
	// existing text settings. Readiness must observe their new current variants.
	rendered := object(t, invoke(t, s, root, "render_prompt", map[string]any{"prompt_group": "premise", "prompt_key": "setting_image", "target": "setting_board", "variables": map[string]string{"input_text": "A fox and an owl"}}))
	write := rendered["write_back"].(map[string]any)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 300, 200))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "host-setting-board.png")
	if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	board := object(t, invoke(t, s, root, "setting_boards", map[string]any{"action": "import", "path": path, "source_text": write["source_text"], "style_snapshot": write["style_snapshot"], "prompt": rendered["generation_prompt"], "expected_revision": write["expected_revision"], "idempotency_key": "readiness-board"}))
	breakdown := object(t, invoke(t, s, root, "render_prompt", map[string]any{"prompt_group": "premise", "prompt_key": "asset_breakdown", "target": "setting_breakdown", "board_uuid": board["uuid"]}))
	write = breakdown["write_back"].(map[string]any)
	assets := []any{}
	for i, setting := range []map[string]any{fox, owl} {
		assets = append(assets, map[string]any{"type": "character", "title": setting["title"], "summary": setting["title"], "tags": []string{"character"}, "confidence": 0.99, "setting_uuid": setting["uuid"], "expected_revision": setting["revision"], "crop_box": map[string]any{"x": float64(i) / 2, "y": 0, "width": 0.5, "height": 1}})
	}
	object(t, invoke(t, s, root, "apply_setting_breakdown", map[string]any{"board_uuid": board["uuid"], "expected_revision": write["expected_revision"], "expected_premise_revision": write["expected_premise_revision"], "idempotency_key": "readiness-split", "assets": assets}))
	before := object(t, invoke(t, s, root, "settings", map[string]any{"action": "list"}))
	// This is a read-only preparation call, not a generation/import side effect.
	raw, _ := json.Marshal(request)
	target = object(t, s.CallReadOnly(t.Context(), root, "image_target", raw))
	ready := target["setting_readiness"].(map[string]any)
	if ready["scope"] != "selected_settings" || ready["status"] != "ready" || ready["available_count"] != float64(2) || ready["missing_count"] != float64(0) || ready["next_action"] != "read_reference_images" {
		t.Fatalf("ready=%v", ready)
	}
	for i, setting := range []map[string]any{owl, fox} {
		item := ready["items"].([]any)[i].(map[string]any)
		current := object(t, invoke(t, s, root, "settings", map[string]any{"action": "get", "setting_uuid": setting["uuid"]}))
		asset := current["current_variant"].(map[string]any)["asset"].(map[string]any)
		if item["setting_uuid"] != setting["uuid"] || item["asset_uuid"] != asset["uuid"] || item["expected_revision"] != current["revision"] || item["has_image"] != true {
			t.Fatalf("item=%v current=%v", item, current)
		}
		if _, err := imageToolResult(invoke(t, s, root, "read_image", map[string]any{"asset_uuid": item["asset_uuid"]})); err != nil {
			t.Fatal(err)
		}
		if _, exists := item["id"]; exists {
			t.Fatal("internal ID exposed")
		}
	}
	after := object(t, invoke(t, s, root, "settings", map[string]any{"action": "list"}))
	if len(after["items"].([]any)) != 4 || after["premise"].(map[string]any)["revision"] != before["premise"].(map[string]any)["revision"] {
		t.Fatal("preparation changed settings or batch created duplicates")
	}
	pageAfter := object(t, invoke(t, s, root, "pages", map[string]any{"action": "get", "chapter_uuid": chapter, "page_uuid": page}))
	if target["expected_revision"] != float64(0) || pageAfter["revision"] != float64(0) || pageAfter["current_image"] != nil {
		t.Fatal("setting preparation generated or changed a page image")
	}
}

func TestPageImageReadinessDoesNotApproveUnplannedOrInvalidSelections(t *testing.T) {
	s, root, _, page := imagePromptPage(t)
	createReadinessSetting(t, s, root, "reference", "Credits")
	request := map[string]any{"target_uuid": page, "slot": "page_image"}
	readiness := object(t, invoke(t, s, root, "image_target", request))["setting_readiness"].(map[string]any)
	if readiness["status"] != "selection_required" || readiness["next_action"] != "plan_visual_settings" || len(readiness["items"].([]any)) != 0 {
		t.Fatal(readiness)
	}
	request["setting_uuids"] = []string{}
	readiness = object(t, invoke(t, s, root, "image_target", request))["setting_readiness"].(map[string]any)
	if readiness["status"] != "no_settings_selected" || readiness["next_action"] != "review_setting_scope" {
		t.Fatal(readiness)
	}
	local := createReadinessSetting(t, s, root, "character", "Fox")
	other, otherRoot, _ := harness(t)
	foreign := createReadinessSetting(t, other, otherRoot, "character", "Foreign")
	for _, selection := range [][]string{{foreign["uuid"].(string)}, {local["uuid"].(string), local["uuid"].(string)}, {"1"}} {
		request["setting_uuids"] = selection
		if invoke(t, s, root, "image_target", request).Success {
			t.Fatalf("accepted invalid selection %v", selection)
		}
	}
	object(t, invoke(t, s, root, "settings", map[string]any{"action": "delete", "setting_uuid": local["uuid"], "expected_revision": local["revision"]}))
	request["setting_uuids"] = []string{local["uuid"].(string)}
	if invoke(t, s, root, "image_target", request).Success {
		t.Fatal("accepted deleted setting")
	}
	note := createReadinessSetting(t, s, root, "reference", "Visual reference to draw")
	request["setting_uuids"] = []string{note["uuid"].(string)}
	readiness = object(t, invoke(t, s, root, "image_target", request))["setting_readiness"].(map[string]any)
	if readiness["status"] != "missing_images" || readiness["missing_count"] != float64(1) {
		t.Fatal("explicit reference selection must be checked", readiness)
	}
	request["slot"], request["target_uuid"] = "setting_image", note["uuid"]
	if code(invoke(t, s, root, "image_target", request)) != "invalid_arguments" {
		t.Fatal("setting selection accepted for a setting image")
	}
	delete(request, "setting_uuids")
	if _, exists := object(t, invoke(t, s, root, "image_target", request))["setting_readiness"]; exists {
		t.Fatal("setting generation should not depend on page readiness")
	}
}
