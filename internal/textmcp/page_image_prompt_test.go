package textmcp

import (
	"strings"
	"testing"

	"lumi/internal/project"
	"lumi/internal/story"
)

func imagePromptPage(t *testing.T) (*Service, string, string, string) {
	t.Helper()
	s, root, _ := harness(t)
	p := object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)
	chapter := object(t, invoke(t, s, root, "chapters", map[string]any{"action": "create", "chapter_code": "vol01.ch01", "title": "Lantern", "expected_revision": p["revision"]}))
	state := object(t, invoke(t, s, root, "pages", map[string]any{"action": "list", "chapter_uuid": chapter["uuid"]}))["state"].(map[string]any)
	page := object(t, invoke(t, s, root, "pages", map[string]any{"action": "create", "chapter_uuid": chapter["uuid"], "title": "A light", "description_md": "A fox holds a lantern.", "storyboard_md": "ACTUAL SAVED SCENE: a fox in a blue scarf holds a brass lantern.", "expected_revision": state["revision"]}))
	return s, root, chapter["uuid"].(string), page["uuid"].(string)
}

func TestPageImagePromptComposesEffectiveFragmentsWithoutTaskMetadataOrWrites(t *testing.T) {
	s, root, chapter, page := imagePromptPage(t)
	p := object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)
	if err := s.Projects.WithStore(t.Context(), p["uuid"].(string), func(store *project.Store) error {
		for key, prompt := range map[string]string{"before_image": "CUSTOM BASE RULES", "section_reference_absent": "CUSTOM NO REFERENCES"} {
			if _, err := story.NewService(store).CreatePromptVersion(t.Context(), story.CreatePromptInput{PromptGroup: "chapter", PromptKey: key, Prompt: prompt, ExpectedCurrentVersion: 1}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"prompt_group": "chapter", "prompt_key": "section_image", "target": "page", "chapter_uuid": chapter, "page_uuid": page}
	if got := code(invoke(t, s, root, "render_prompt", request)); got != "prompt_inputs_missing" {
		t.Fatalf("missing task inputs=%s", got)
	}
	fragments := map[string]string{}
	for _, key := range []string{"before_image", "section_reference_absent"} {
		fragment := object(t, invoke(t, s, root, "render_prompt", map[string]any{"prompt_group": "chapter", "prompt_key": key, "target": "page", "chapter_uuid": chapter, "page_uuid": page}))
		fragments[key] = fragment["fragment_prompt"].(string)
	}
	if fragments["before_image"] != "CUSTOM BASE RULES" || fragments["section_reference_absent"] != "CUSTOM NO REFERENCES" {
		t.Fatal("project-effective fragment text was not returned verbatim")
	}
	request["variables"] = map[string]string{"before_image_prompt": fragments["before_image"], "reference_usage_text": fragments["section_reference_absent"], "section_id": "FORGED ID", "storyboard": "FORGED SCENE", "style_prompt": "FORGED STYLE"}
	rendered := object(t, invoke(t, s, root, "render_prompt", request))
	generation := rendered["generation_prompt"].(string)
	for _, required := range []string{"CUSTOM BASE RULES", "CUSTOM NO REFERENCES", page, "ACTUAL SAVED SCENE", "Project language: English"} {
		if !strings.Contains(generation, required) {
			t.Fatalf("generation prompt missing %q", required)
		}
	}
	for _, forbidden := range []string{"FORGED", "Task context (JSON", "Write-back contract", "Do not call any provider", "Return one valid JSON object only"} {
		if strings.Contains(generation, forbidden) {
			t.Fatalf("generation prompt contains task metadata or forged input: %s", forbidden)
		}
	}
	write := rendered["image_write_back"].(map[string]any)
	if write["target_uuid"] != page || write["slot"] != "page_image" || write["expected_revision"] != float64(0) {
		t.Fatalf("image write-back=%v", write)
	}
	after := object(t, invoke(t, s, root, "pages", map[string]any{"action": "get", "chapter_uuid": chapter, "page_uuid": page}))
	if after["revision"] != float64(0) || after["current_image"] != nil || after["description_md"] != "A fox holds a lantern." || after["current_storyboard"].(map[string]any)["version_no"] != float64(1) {
		t.Fatal("prompt composition changed the page")
	}
}

func TestPageGenerationPromptLimitRejectsBeforeAnyImageWrite(t *testing.T) {
	s, root, chapter, page := imagePromptPage(t)
	result := invoke(t, s, root, "render_prompt", map[string]any{"prompt_group": "chapter", "prompt_key": "section_image", "target": "page", "chapter_uuid": chapter, "page_uuid": page, "variables": map[string]string{"before_image_prompt": strings.Repeat("x", 12000), "reference_usage_text": "No image references provided."}})
	if got := code(result); got != "prompt_too_large" {
		t.Fatalf("oversize generation prompt=%s", got)
	}
	pageAfter := object(t, invoke(t, s, root, "pages", map[string]any{"action": "get", "chapter_uuid": chapter, "page_uuid": page}))
	if pageAfter["revision"] != float64(0) || pageAfter["current_image"] != nil {
		t.Fatal("oversize prompt changed the image slot")
	}
}
