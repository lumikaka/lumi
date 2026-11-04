package production

import (
	"os"
	"testing"

	"lumi/internal/project"
	"lumi/internal/story"
)

type pageImportEventHook func(string)

func (hook pageImportEventHook) Broadcast(_ string, event string, _ any) { hook(event) }

// Change a fact after the initial import check, while the output is being
// staged. Only validation inside the final SQLite transaction can catch this.
func TestPageImageImportRechecksInputsAtCommit(t *testing.T) {
	for _, mutation := range []string{"setting_version", "project_style", "language_instruction"} {
		t.Run(mutation, func(t *testing.T) {
			h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
			chapter, page := hostPage(t, h)
			path, _ := hostFixture(t, 80, 60, false)
			premise, err := h.service.GetPremise(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			setting, err := h.service.CreateTextPremiseAsset(t.Context(), CreateAssetInput{AssetType: "character", Title: "Fox", Summary: "A fox"}, premise.Revision)
			if err != nil {
				t.Fatal(err)
			}
			first, err := h.service.ImportHostImage(t.Context(), ImportHostImageInput{Path: path, TargetUUID: setting.UUID, Slot: "setting_image", ExpectedRevision: setting.Revision})
			if err != nil {
				t.Fatal(err)
			}
			style, err := h.service.HostSettingReviewStyle(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			reviews := []HostSettingReview{{SettingUUID: setting.UUID, VariantUUID: first.VariantUUID, ExpectedRevision: first.Revision, StyleDigest: SettingReviewStyleDigest(style), Verdict: "passed", ReferenceBasis: "Fixture style", Comparison: "Fixture proportions and linework match"}}
			prepared, err := h.service.PreparePageImage(t.Context(), PreparePageImageInput{PageUUID: page.UUID, SettingUUIDs: []string{setting.UUID}, AdditionalDirection: "Keep all dialogue", SettingReviews: reviews})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Remove(prepared.ReferenceImagePath) })
			changed := false
			var changeErr error
			hook := pageImportEventHook(func(event string) {
				if event != "upload/completed" || changed {
					return
				}
				changed = true
				switch mutation {
				case "setting_version":
					_, changeErr = h.service.ImportHostImage(t.Context(), ImportHostImageInput{Path: path, TargetUUID: setting.UUID, Slot: "setting_image", ExpectedRevision: first.Revision})
				case "project_style":
					_, changeErr = h.stories.CreatePromptVersion(t.Context(), story.CreatePromptInput{PromptGroup: "premise_style", PromptKey: "project_overall_style", Prompt: "Changed project art style", ExpectedCurrentVersion: 1})
				case "language_instruction":
					_, changeErr = h.stories.CreatePromptVersion(t.Context(), story.CreatePromptInput{PromptGroup: "runtime", PromptKey: "project_language_instruction", Prompt: "Use the updated language instruction.", ExpectedCurrentVersion: 1})
				}
			})
			service := NewService(h.service.store, hook)
			_, err = service.ImportHostImage(t.Context(), ImportHostImageInput{Path: path, TargetUUID: page.UUID, Slot: "page_image", ExpectedRevision: page.Revision, Prompt: prepared.GenerationPrompt, Preparation: &prepared.Preparation})
			if !changed || changeErr != nil {
				t.Fatalf("mutation did not occur between checks: changed=%v error=%v", changed, changeErr)
			}
			assertHostError(t, err, "page_image_preparation_stale")
			versions, err := h.service.ListImageVariants(t.Context(), chapter, page.UUID)
			if err != nil || len(versions) != 0 {
				t.Fatalf("stale import committed a page image version: %v, %v", versions, err)
			}
			current, err := h.service.GetSection(t.Context(), chapter, page.UUID)
			if err != nil || current.Revision != page.Revision || current.CurrentImage != nil {
				t.Fatalf("stale import changed the page: %+v, %v", current, err)
			}
		})
	}
}
