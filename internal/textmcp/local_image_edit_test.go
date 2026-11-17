package textmcp

import (
	"lumi/internal/production"
	"testing"
)

func TestMCPLocalImageEditUsesHistoricalVersionAndCandidate(t *testing.T) {
	s, root, chapter, page := imagePromptPage(t)
	path := preparedFixtureImage(t)
	first := object(t, invoke(t, s, root, "import_image", preparedImportArgs(preparePageFixture(t, s, root, page, []string{}, ""), path)))
	current := object(t, invoke(t, s, root, "import_image", preparedImportArgs(preparePageFixture(t, s, root, page, []string{}, ""), path)))
	args := map[string]any{"page_uuid": page, "base_variant_uuid": first["variant_uuid"], "selection": map[string]any{"x": .2, "y": .3, "width": .3, "height": .2}, "additional_direction": "silver badge"}
	prepared := object(t, invoke(t, s, root, "prepare_page_image", args))
	if prepared["reference_mode"] != "local_edit" {
		t.Fatal(prepared)
	}
	// The untouched context crop is a valid host identity output for this protocol test.
	imported := object(t, invoke(t, s, root, "import_image", preparedImportArgs(prepared, prepared["reference_image_path"].(string))))
	if imported["adopted"] != false || imported["version_no"] != float64(3) {
		t.Fatal(imported)
	}
	actual := object(t, invoke(t, s, root, "pages", map[string]any{"action": "get", "chapter_uuid": chapter, "page_uuid": page, "include_versions": true}))
	if actual["current_image"].(map[string]any)["uuid"] != current["variant_uuid"] {
		t.Fatal("candidate was adopted")
	}
	if len(actual["image_versions"].(map[string]any)["items"].([]any)) != 3 {
		t.Fatal(actual)
	}
	for _, key := range []string{"base_variant_uuid", "selection", "additional_direction"} {
		copy := map[string]any{}
		for k, v := range args {
			if k != key {
				copy[k] = v
			}
		}
		if invoke(t, s, root, "prepare_page_image", copy).Success {
			t.Fatal("accepted missing local field", key)
		}
	}
	receipt := prepared["preparation"].(map[string]any)
	receipt["local_edit"].(map[string]any)["crop_pixels"].(map[string]any)["x"] = float64(2)
	forged := preparedImportArgs(prepared, path)
	forged["expected_revision"] = imported["revision"]
	receipt["expected_revision"] = imported["revision"]
	if got := code(invoke(t, s, root, "import_image", forged)); got != production.CodePageImagePreparationStale {
		t.Fatal(got)
	}
	object(t, invoke(t, s, root, "pages", map[string]any{"action": "select_image", "chapter_uuid": chapter, "page_uuid": page, "variant_uuid": imported["variant_uuid"], "expected_revision": imported["revision"]}))
}
