package production

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"
)

// ImportedImageSnapshot preserves observed provenance without inventing a Lumi
// generation, task, provider call or original prompt for an imported image.
// The reference File must already be registered in this project.
type ImportedImageSnapshot struct {
	Kind              string                 `json:"kind"`
	Prompt            string                 `json:"prompt,omitempty"`
	PromptSHA256      string                 `json:"prompt_sha256,omitempty"`
	RevisedPrompt     string                 `json:"revised_prompt,omitempty"`
	OriginalMissing   bool                   `json:"original_missing"`
	ReferenceFileUUID string                 `json:"reference_file_uuid,omitempty"`
	ReferenceMetadata SectionPremiseMetadata `json:"reference_metadata"`
}

func (service *Service) ImportSectionImageWithSnapshot(ctx context.Context, chapterUUID, sectionUUID, uploadUUID string, input ImportedImageSnapshot, expectedRevision int64) (ComicSection, error) {
	input.Kind = "external_image_import"
	if input.ReferenceFileUUID != "" {
		if _, err := service.files.GetAsset(ctx, input.ReferenceFileUUID, false); err != nil {
			return ComicSection{}, err
		}
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return ComicSection{}, err
	}
	return service.importSectionImage(ctx, chapterUUID, sectionUUID, expectedRevision, string(raw), func(bind func(*gorm.DB, int64) error) error {
		_, err := service.files.FinalizeUploadWithBind(ctx, uploadUUID, "comic_section_image", bind)
		return err
	})
}

func (service *Service) importedImagePremise(ctx context.Context, raw string) (*SectionPremise, error) {
	var header struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal([]byte(raw), &header) != nil || header.Kind != "external_image_import" {
		return nil, nil
	}
	var snapshot ImportedImageSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Kind != "external_image_import" || snapshot.ReferenceFileUUID == "" {
		return nil, nil
	}
	asset, err := service.files.GetAsset(ctx, snapshot.ReferenceFileUUID, false)
	if err != nil {
		return nil, err
	}
	result := sectionPremiseFromMetadata(asset, snapshot.ReferenceMetadata)
	return &result, nil
}
