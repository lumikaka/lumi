package production

import (
	"errors"
	"os"
	"strings"
	"testing"

	"lumi/internal/project"
	"lumi/internal/story"
)

type imagePreparationEventHook struct{ call func(string) }

func (hook imagePreparationEventHook) Broadcast(_ string, event string, _ any) { hook.call(event) }

func TestHostPagePreparationIsRecheckedInsideFinalAssetCommit(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	chapter, page := hostPage(t, h)
	path, _ := hostFixture(t, 80, 60, false)
	prepared := prepareHostPageFixture(t, h.service, page.UUID, "Keep the dialogue")
	changed := false
	// File upload completion is after preflight and before the final binding
	// transaction. Change an effective input at that exact boundary.
	service := NewService(h.service.store, imagePreparationEventHook{call: func(event string) {
		if event != "upload/completed" || changed {
			return
		}
		changed = true
		if err := h.service.store.DB().Exec("UPDATE comic_sections SET description_md='changed during upload' WHERE uuid=?", page.UUID).Error; err != nil {
			t.Fatal(err)
		}
	}})
	_, err := service.ImportHostImage(t.Context(), ImportHostImageInput{Path: path, TargetUUID: page.UUID, Slot: "page_image", ExpectedRevision: page.Revision, Prompt: prepared.GenerationPrompt, Preparation: &prepared.Preparation})
	if !changed {
		t.Fatal("test did not change facts after preflight")
	}
	assertHostError(t, err, CodePageImagePreparationStale)
	versions, err := h.service.ListImageVariants(t.Context(), chapter, page.UUID)
	if err != nil || len(versions) != 0 {
		t.Fatal("transaction accepted stale input", versions, err)
	}
}

func TestPreparePageRejectsUnreadableSelectedImage(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	_, page := hostPage(t, h)
	path, _ := hostFixture(t, 80, 60, false)
	premise, err := h.service.GetPremise(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	setting, err := h.service.CreateTextPremiseAsset(t.Context(), CreateAssetInput{AssetType: "character", Title: "Fox", Summary: "Fox"}, premise.Revision)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := h.service.ImportHostImage(t.Context(), ImportHostImageInput{Path: path, TargetUUID: setting.UUID, Slot: "setting_image", ExpectedRevision: setting.Revision})
	if err != nil {
		t.Fatal(err)
	}
	content, err := h.service.files.OpenContent(t.Context(), imported.Asset.UUID)
	if err != nil {
		t.Fatal(err)
	}
	internalPath := content.File.Name()
	_ = content.File.Close()
	if err := os.Remove(internalPath); err != nil {
		t.Fatal(err)
	}
	_, err = h.service.PreparePageImage(t.Context(), PreparePageImageInput{PageUUID: page.UUID, SettingUUIDs: []string{setting.UUID}}, new(PageReferenceFiles))
	var preparationError *Error
	if !errors.As(err, &preparationError) || preparationError.Code != "section_premise_source_unreadable" {
		t.Fatalf("missing source was not reported explicitly: %v", err)
	}
}

func TestHostStripUsesProjectCanvasAndStyle(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookVertical)
	_, page := hostPage(t, h)
	_, err := h.stories.CreatePromptVersion(t.Context(), story.CreatePromptInput{PromptGroup: "premise_style", PromptKey: "project_overall_style", Prompt: "Graphite pencil on textured paper", ExpectedCurrentVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	prepared := prepareHostPageFixture(t, h.service, page.UUID, "")
	if prepared.Target.TargetRatio != (ImageRatio{1, 3}) || strings.Contains(prepared.Target.GenerationHint, "2:3") || !strings.Contains(prepared.Target.GenerationHint, "1:3") {
		t.Fatal(prepared.Target)
	}
	prompt := prepared.GenerationPrompt
	if !strings.Contains(prompt, "Graphite pencil on textured paper") || !strings.Contains(prompt, "1:3") || strings.Contains(prompt, "赛璐璐") || strings.Contains(prompt, "cel shading") {
		t.Fatal(prompt)
	}
}
