package textmcp

import (
	"encoding/json"
	"strings"
	"testing"

	"lumi/internal/project"
	"lumi/internal/story"
)

func TestSettingReviewGateAndSignedEvidence(t *testing.T) {
	s, root, chapter, page := imagePromptPage(t)
	path := preparedFixtureImage(t)
	setting := createReadinessSetting(t, s, root, "character", "Fox")
	object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": setting["revision"]}))
	selection := []string{setting["uuid"].(string)}
	reviews := fixtureSettingReviews(t, s, root, page, selection)
	for _, test := range []struct {
		name   string
		change func([]any) []any
		code   string
	}{
		{"missing", func(_ []any) []any { return nil }, "setting_image_review_required"},
		{"failed", func(r []any) []any { r[0].(map[string]any)["verdict"] = "needs_revision"; return r }, "setting_image_review_required"},
		{"blank evidence", func(r []any) []any { r[0].(map[string]any)["comparison"] = " \n "; return r }, "setting_image_review_required"},
		{"wrong setting", func(r []any) []any { r[0].(map[string]any)["setting_uuid"] = page; return r }, "setting_image_review_required"},
		{"wrong variant", func(r []any) []any { r[0].(map[string]any)["variant_uuid"] = page; return r }, "setting_image_review_stale"},
		{"old revision", func(r []any) []any { r[0].(map[string]any)["expected_revision"] = 0; return r }, "setting_image_review_stale"},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded, _ := json.Marshal(reviews)
			var copy []any
			_ = json.Unmarshal(encoded, &copy)
			args := map[string]any{"page_uuid": page, "setting_uuids": selection}
			if changed := test.change(copy); changed != nil {
				args["setting_reviews"] = changed
			}
			if got := code(invoke(t, s, root, "prepare_page_image", args)); got != test.code {
				t.Fatalf("got %s, want %s", got, test.code)
			}
		})
	}
	prepared := preparePageFixture(t, s, root, page, selection, "")
	args := preparedImportArgs(prepared, path)
	receipt := args["preparation"].(map[string]any)
	review := receipt["setting_reviews"].([]any)[0].(map[string]any)
	original := review["comparison"]
	review["comparison"] = "Tampered approval evidence"
	if got := code(invoke(t, s, root, "import_image", args)); got != "page_image_preparation_stale" {
		t.Fatalf("unsigned evidence accepted: %s", got)
	}
	current := object(t, invoke(t, s, root, "pages", map[string]any{"action": "get", "chapter_uuid": chapter, "page_uuid": page}))
	if current["current_image"] != nil || current["revision"] != float64(0) {
		t.Fatal("failed review changed page")
	}
	review["comparison"] = original
	object(t, invoke(t, s, root, "import_image", args))
}

func TestSettingReviewMustBeRepeatedAfterStyleOrImageChanges(t *testing.T) {
	s, root, _, page := imagePromptPage(t)
	path := preparedFixtureImage(t)
	setting := createReadinessSetting(t, s, root, "character", "Fox")
	first := object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": setting["revision"]}))
	selection := []string{setting["uuid"].(string)}
	args := map[string]any{"page_uuid": page, "setting_uuids": selection, "setting_reviews": fixtureSettingReviews(t, s, root, page, selection)}
	if err := s.Projects.WithStore(t.Context(), internalProjectUUID(t, s, root), func(store *project.Store) error {
		_, err := story.NewService(store).CreatePromptVersion(t.Context(), story.CreatePromptInput{PromptGroup: "premise_style", PromptKey: "project_overall_style", Prompt: "User explicitly requests bright colors and narrow handwritten lettering with an uneven baseline.", ExpectedCurrentVersion: 1})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := code(invoke(t, s, root, "prepare_page_image", args)); got != "setting_image_review_stale" {
		t.Fatalf("old style assessment accepted: %s", got)
	}
	args["setting_reviews"] = fixtureSettingReviews(t, s, root, page, selection)
	object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": first["revision"]}))
	if got := code(invoke(t, s, root, "prepare_page_image", args)); got != "setting_image_review_stale" {
		t.Fatalf("old image assessment accepted: %s", got)
	}
	prepared := preparePageFixture(t, s, root, page, selection, "User requests a readable handwritten caption")
	if !strings.Contains(prepared["generation_prompt"].(string), "narrow handwritten lettering with an uneven baseline") || !strings.Contains(prepared["generation_prompt"].(string), "User requests a readable handwritten caption") {
		t.Fatal("saved lettering style and explicit user direction must reach the page image prompt")
	}
}
