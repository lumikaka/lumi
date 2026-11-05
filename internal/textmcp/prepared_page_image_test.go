package textmcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/story"
)

func preparedFixtureImage(t *testing.T) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 83, 121))
	for y := 0; y < 121; y++ {
		for x := 0; x < 83; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 3), G: uint8(y * 2), B: 120, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "actual-host-output.png")
	if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func preparePageFixture(t *testing.T, s *Service, root, page string, selection []string, direction string) map[string]any {
	t.Helper()
	prepared := object(t, invoke(t, s, root, "prepare_page_image", map[string]any{"page_uuid": page, "setting_uuids": selection, "setting_reviews": fixtureSettingReviews(t, s, root, page, selection), "additional_direction": direction}))
	if path := prepared["reference_image_path"].(string); path != "" {
		t.Cleanup(func() { _ = os.Remove(path) })
	}
	return prepared
}

// A host-review test double supplies assessments; these synthetic images do
// not claim to test the quality of a real model's visual judgement.
func fixtureSettingReviews(t *testing.T, s *Service, root, page string, selection []string) []any {
	t.Helper()
	readiness := object(t, invoke(t, s, root, "image_target", map[string]any{"target_uuid": page, "slot": "page_image", "setting_uuids": selection}))["setting_readiness"].(map[string]any)
	reviews := []any{}
	for _, raw := range readiness["items"].([]any) {
		item := raw.(map[string]any)
		reviews = append(reviews, map[string]any{"setting_uuid": item["setting_uuid"], "variant_uuid": item["variant_uuid"], "expected_revision": item["expected_revision"], "style_digest": readiness["style_digest"], "verdict": "passed", "reference_basis": "Fixture user style and image", "comparison": "Fixture identity, proportions, contour, palette and texture match the requested reference."})
	}
	return reviews
}

func preparedImportArgs(prepared map[string]any, path string) map[string]any {
	write := prepared["image_write_back"].(map[string]any)
	return map[string]any{"path": path, "target_uuid": write["target_uuid"], "slot": write["slot"], "expected_revision": write["expected_revision"], "prompt": prepared["generation_prompt"], "preparation": prepared["preparation"]}
}

func TestPreparePageFourSettingsHostPathFlow(t *testing.T) {
	s, root, chapter, page := imagePromptPage(t)
	path := preparedFixtureImage(t)
	selection := []string{}
	for _, title := range []string{"月光小狐狸", "黄昏灯塔", "银色邮包", "星空地图"} {
		setting := createReadinessSetting(t, s, root, "character", title)
		object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": setting["revision"]}))
		selection = append(selection, setting["uuid"].(string))
	}
	prepared := preparePageFixture(t, s, root, page, selection, "保留对白及完整主体，四周留白。")
	wireResult, err := boundedResult(invoke(t, s, root, "prepare_page_image", map[string]any{"page_uuid": page, "setting_uuids": selection, "setting_reviews": prepared["preparation"].(map[string]any)["setting_reviews"], "additional_direction": "保留对白及完整主体，四周留白。"}))
	if err != nil || wireResult.IsError || len(wireResult.Content) != 1 {
		t.Fatal(wireResult, err)
	}
	second, _ := json.Marshal(wireResult.StructuredContent)
	var secondEnvelope struct {
		Data production.PreparedPageImage `json:"data"`
	}
	if err := json.Unmarshal(second, &secondEnvelope); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(secondEnvelope.Data.ReferenceImagePath) })
	if prepared["reference_image_path"] != secondEnvelope.Data.ReferenceImagePath || prepared["preparation"].(map[string]any)["digest"] != secondEnvelope.Data.Preparation.Digest {
		t.Fatal("repeated preparation must preserve the reference path and page digest")
	}
	wire, _ := json.Marshal(wireResult)
	for _, forbidden := range []string{"base64", "data:image", "image_data", `"type":"image"`, "project.sqlite", "/assets/", "/.lumi/"} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("MCP leaked image bytes/internal path: %s", forbidden)
		}
	}
	collagePath := prepared["reference_image_path"].(string)
	if !filepath.IsAbs(collagePath) || strings.HasPrefix(collagePath, root+string(os.PathSeparator)) || prepared["reference_mode"] != "collage" {
		t.Fatal(prepared)
	}
	// The host-viewer test double actually opens and decodes this exact path.
	viewed, err := os.ReadFile(collagePath)
	if err != nil {
		t.Fatal(err)
	}
	if digest := fmt.Sprintf("%x", sha256.Sum256(viewed)); prepared["reference_image_sha256"] != digest || secondEnvelope.Data.ReferenceImageSHA256 != digest {
		t.Fatal("reference digest must match the complete PNG")
	}
	decoded, err := png.Decode(bytes.NewReader(viewed))
	if err != nil || decoded.Bounds().Dx() != 1176 || decoded.Bounds().Dy() != 816 {
		t.Fatal("unreadable collage", err)
	}
	for i, item := range prepared["sources"].([]any) {
		source := item.(map[string]any)
		if source["order"] != float64(i+1) || source["setting_uuid"] != selection[i] || source["label"] != fmt.Sprintf("%d. %s", i+1, source["title"]) || source["variant_uuid"] == "" || source["file_uuid"] == "" || len(source["sha256"].(string)) != 64 {
			t.Fatal(source)
		}
	}
	// A host imagegen test double accepts one path only, after viewing it, and
	// records the exact complete prompt. It does not call a paid image model.
	host := func(prompt string, referencedImagePaths []string) string {
		if len(viewed) == 0 || len(referencedImagePaths) != 1 || referencedImagePaths[0] != collagePath || prompt != prepared["generation_prompt"] {
			t.Fatal("host reference/prompt contract changed")
		}
		return path
	}
	output := host(prepared["generation_prompt"].(string), []string{collagePath})
	imported := object(t, invoke(t, s, root, "import_image", preparedImportArgs(prepared, output)))
	if imported["version_no"] != float64(1) || imported["asset"].(map[string]any)["metadata"].(map[string]any)["original_prompt"] != prepared["generation_prompt"] {
		t.Fatal(imported)
	}
	pageAfter := object(t, invoke(t, s, root, "pages", map[string]any{"action": "get", "chapter_uuid": chapter, "page_uuid": page}))
	if pageAfter["description_md"] != "A fox holds a lantern." || pageAfter["current_storyboard"].(map[string]any)["version_no"] != float64(1) {
		t.Fatal("image preparation changed page prose/storyboard")
	}
	if err := s.Projects.WithStore(t.Context(), internalProjectUUID(t, s, root), func(store *project.Store) error {
		for _, table := range []string{"production_task_runs", "comic_image_generations"} {
			var count int64
			if err := store.DB().Table(table).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				t.Fatalf("host flow started provider pipeline: %s=%d", table, count)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPagesShareOnlyTheirReferencePNG(t *testing.T) {
	s, root, chapter, firstPage := imagePromptPage(t)
	input := preparedFixtureImage(t)
	var selection []string
	for _, title := range []string{"Fox", "Lantern"} {
		setting := createReadinessSetting(t, s, root, "prop", title)
		object(t, invoke(t, s, root, "import_image", map[string]any{"path": input, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": setting["revision"]}))
		selection = append(selection, setting["uuid"].(string))
	}
	state := object(t, invoke(t, s, root, "pages", map[string]any{"action": "list", "chapter_uuid": chapter}))["state"].(map[string]any)
	secondPage := object(t, invoke(t, s, root, "pages", map[string]any{"action": "create", "chapter_uuid": chapter, "title": "Home", "description_md": "The fox is home.", "storyboard_md": "Put the lantern on the table.", "expected_revision": state["revision"]}))["uuid"].(string)
	first := preparePageFixture(t, s, root, firstPage, selection, "")
	second := preparePageFixture(t, s, root, secondPage, selection, "")
	if first["reference_image_path"] != second["reference_image_path"] || first["reference_image_sha256"] != second["reference_image_sha256"] {
		t.Fatal("the second page should reuse the same reference PNG")
	}
	if first["generation_prompt"] == second["generation_prompt"] {
		t.Fatal("the second page must have its own prompt")
	}
	wrong := preparedImportArgs(second, input)
	wrong["preparation"] = first["preparation"]
	if got := code(invoke(t, s, root, "import_image", wrong)); got != "page_image_preparation_stale" {
		t.Fatalf("other page receipt: %s", got)
	}
	reordered := preparePageFixture(t, s, root, secondPage, []string{selection[1], selection[0]}, "")
	if reordered["reference_image_sha256"] == second["reference_image_sha256"] || reordered["reference_image_path"] == second["reference_image_path"] {
		t.Fatal("changed reference labels must cause a new preview")
	}
	object(t, invoke(t, s, root, "import_image", preparedImportArgs(first, input)))
	object(t, invoke(t, s, root, "import_image", preparedImportArgs(second, input)))
}

func TestPreparedPageRejectsChangedCurrentSettingImageWithoutWritingVersion(t *testing.T) {
	s, root, chapter, page := imagePromptPage(t)
	path := preparedFixtureImage(t)
	setting := createReadinessSetting(t, s, root, "character", "Fox")
	first := object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": setting["revision"]}))
	prepared := preparePageFixture(t, s, root, page, []string{setting["uuid"].(string)}, "Keep dialogue")
	// Even identical pixels in a NEW adopted version must invalidate the receipt.
	object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": first["revision"]}))
	args := preparedImportArgs(prepared, path)
	if got := code(invoke(t, s, root, "import_image", args)); got != "page_image_preparation_stale" {
		t.Fatal(got)
	}
	fresh := preparePageFixture(t, s, root, page, []string{setting["uuid"].(string)}, "Keep dialogue")
	oldReceipt := args["preparation"].(map[string]any)
	oldReceipt["digest"] = fresh["preparation"].(map[string]any)["digest"]
	if got := code(invoke(t, s, root, "import_image", args)); got != "page_image_preparation_stale" {
		t.Fatalf("digest replacement bypassed signed facts: %s", got)
	}
	current := object(t, invoke(t, s, root, "pages", map[string]any{"action": "get", "chapter_uuid": chapter, "page_uuid": page, "include_versions": true}))
	if current["current_image"] != nil || current["revision"] != float64(0) {
		t.Fatal("stale import wrote image version")
	}
	if err := s.Projects.WithStore(t.Context(), internalProjectUUID(t, s, root), func(store *project.Store) error {
		var count int64
		if err := store.DB().Table("comic_image_variants").Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("stale import created a version")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPreparePageSelectionLimitsAndNoReference(t *testing.T) {
	s, root, _, page := imagePromptPage(t)
	path := preparedFixtureImage(t)
	missing := createReadinessSetting(t, s, root, "character", "Missing image")
	if got := code(invoke(t, s, root, "prepare_page_image", map[string]any{"page_uuid": page, "setting_uuids": []string{missing["uuid"].(string)}})); got != "setting_image_missing" {
		t.Fatal(got)
	}
	other, otherRoot, _ := harness(t)
	foreign := createReadinessSetting(t, other, otherRoot, "prop", "Foreign")
	for _, selection := range [][]string{{foreign["uuid"].(string)}, {missing["uuid"].(string), missing["uuid"].(string)}, {"1"}, nil} {
		if invoke(t, s, root, "prepare_page_image", map[string]any{"page_uuid": page, "setting_uuids": selection}).Success {
			t.Fatalf("accepted invalid selection %v", selection)
		}
	}
	noRef := preparePageFixture(t, s, root, page, []string{}, "Keep exact dialogue")
	if noRef["reference_image_sha256"] != nil || noRef["reference_image_path"] != "" || noRef["reference_mode"] != "none" || noRef["image_info"] != nil || len(noRef["sources"].([]any)) != 0 {
		t.Fatal(noRef)
	}
	selection := []string{}
	for i := 0; i < 13; i++ {
		setting := createReadinessSetting(t, s, root, "prop", fmt.Sprintf("Prop %d", i))
		object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": setting["revision"]}))
		selection = append(selection, setting["uuid"].(string))
	}
	limit := preparePageFixture(t, s, root, page, selection[:12], "")
	if len(limit["sources"].([]any)) != 12 || limit["image_info"].(map[string]any)["height"] != float64(1608) {
		t.Fatal(limit)
	}
	if invoke(t, s, root, "prepare_page_image", map[string]any{"page_uuid": page, "setting_uuids": selection}).Success {
		t.Fatal("accepted 13 references")
	}
	// Explicit no-reference import still works and needs the signed preparation.
	object(t, invoke(t, s, root, "import_image", preparedImportArgs(noRef, path)))
}

func TestPreparedPagePromptMatchesWebAssemblyVerbatim(t *testing.T) {
	s, root, chapter, page := imagePromptPage(t)
	projectUUID := internalProjectUUID(t, s, root)
	for _, role := range []string{"body", "front_cover", "back_cover"} {
		if role != "body" {
			listing := object(t, invoke(t, s, root, "pages", map[string]any{"action": "list", "chapter_uuid": chapter}))
			created := object(t, invoke(t, s, root, "pages", map[string]any{"action": "create", "chapter_uuid": chapter, "title": role, "description_md": "Exact dialogue", "storyboard_md": "ACTUAL SAVED SCENE", "page_role": role, "expected_revision": listing["state"].(map[string]any)["revision"]}))
			page = created["uuid"].(string)
		}

		if err := s.Projects.WithStore(t.Context(), projectUUID, func(store *project.Store) error {
			st := story.NewService(store)
			if role == "body" {
				_, err := st.CreatePromptVersion(t.Context(), story.CreatePromptInput{PromptGroup: "premise_style", PromptKey: "project_overall_style", Prompt: "CANONICAL CUSTOM INK STYLE", ExpectedCurrentVersion: 1})
				if err != nil {
					return err
				}
			}
			prod := production.NewService(store, nil)
			prepared, err := prod.PreparePageImage(t.Context(), production.PreparePageImageInput{PageUUID: page, SettingUUIDs: []string{}, AdditionalDirection: "Keep exact dialogue and margins"}, &s.PageReferences)
			if err != nil {
				return err
			}
			rules, err := prod.SectionImageRules(t.Context(), role)
			if err != nil {
				return err
			}
			section, err := prod.GetSection(t.Context(), chapter, page)
			if err != nil {
				return err
			}
			ref, err := promptcatalog.Render(rules.ReferenceAbsentPrompt, map[string]string{})
			if err != nil {
				return err
			}
			// The pre-refactor Web sequence, retained as a whitespace oracle.
			legacy, err := promptcatalog.Render(rules.PromptTemplate, map[string]string{"style_prompt": rules.StyleSnapshot, "reference_usage_text": ref, "section_id": page, "storyboard": section.CurrentStoryboard.ContentMD})
			if err != nil {
				return err
			}
			legacy = promptcatalog.WithInstruction(legacy, rules.LanguageInstruction)
			additional, err := promptcatalog.Render(rules.AdditionalDirectionPrompt, map[string]string{"guidance_prompt": "Keep exact dialogue and margins"})
			if err != nil {
				return err
			}
			legacy += "\n\n" + additional
			if prepared.GenerationPrompt != legacy || !strings.Contains(legacy, "CANONICAL CUSTOM INK STYLE") || !strings.Contains(legacy, "Project language: English") {
				t.Fatal("prompt differs from Web byte sequence or effective style/language missing")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPreparedPageRejectsPromptRuleAndPageChanges(t *testing.T) {
	for _, change := range []string{"prompt", "style", "language", "rules", "page"} {
		t.Run(change, func(t *testing.T) {
			s, root, chapter, page := imagePromptPage(t)
			prepared := preparePageFixture(t, s, root, page, []string{}, "Keep dialogue")
			args := preparedImportArgs(prepared, preparedFixtureImage(t))
			switch change {
			case "prompt":
				args["prompt"] = prepared["generation_prompt"].(string) + "\nExtra instruction"
			case "page":
				updated := object(t, invoke(t, s, root, "pages", map[string]any{"action": "update", "chapter_uuid": chapter, "page_uuid": page, "description_md": "Changed dialogue", "expected_revision": 0}))
				if got := code(invoke(t, s, root, "import_image", args)); got != production.CodeConflict {
					t.Fatal(got)
				}
				args["expected_revision"] = updated["revision"] // Patching revision must still fail.
			default:
				if err := s.Projects.WithStore(t.Context(), internalProjectUUID(t, s, root), func(store *project.Store) error {
					st := story.NewService(store)
					if change == "language" {
						p, err := st.GetProject(t.Context())
						if err != nil {
							return err
						}
						language := "zh-Hans"
						_, err = st.UpdateProject(t.Context(), story.UpdateProjectInput{Name: p.Name, Description: p.Description, GenerationLanguage: &language, ExpectedRevision: p.Revision})
						return err
					}
					group, key := "premise_style", "project_overall_style"
					if change == "rules" {
						group, key = "chapter", "before_image"
					}
					_, err := st.CreatePromptVersion(t.Context(), story.CreatePromptInput{PromptGroup: group, PromptKey: key, Prompt: "CHANGED RULES OR STYLE", ExpectedCurrentVersion: 1})
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			if got := code(invoke(t, s, root, "import_image", args)); got != production.CodePageImagePreparationStale {
				t.Fatal(got)
			}
			current := object(t, invoke(t, s, root, "pages", map[string]any{"action": "get", "chapter_uuid": chapter, "page_uuid": page}))
			if current["current_image"] != nil {
				t.Fatal("stale import created image version")
			}
		})
	}
}
