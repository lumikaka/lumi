package production

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"lumi/internal/files"

	"gorm.io/gorm"
)

type generationSectionPremiseRecord struct {
	ID              int64
	UUID            string
	ComicSectionID  int64
	PremiseFileID   *int64
	PremiseMetadata string
}

func (service *Service) GenerationSectionPremise(ctx context.Context, generationUUID string) (*SectionPremise, error) {
	if !isUUIDv7(generationUUID) {
		return nil, domainError(CodeValidation, "图片生成 UUID 无效", "generation_uuid 必须是 UUIDv7。", nil)
	}
	var generation generationSectionPremiseRecord
	result := service.store.DB().WithContext(ctx).Table("comic_image_generations").
		Select("id,uuid,comic_section_id,premise_file_id,premise_metadata").
		Where("uuid = ?", generationUUID).Scan(&generation)
	if result.Error != nil {
		return nil, result.Error
	}
	if generation.ID == 0 {
		return nil, notFound(gorm.ErrRecordNotFound, "图片生成记录不存在")
	}
	return service.sectionPremiseDTO(ctx, generation.PremiseFileID, generation.PremiseMetadata)
}

func (service *Service) CommitGeneratedSectionPremise(ctx context.Context, generationUUID, sectionUUID string, metadata SectionPremiseMetadata, reader io.Reader) (SectionPremise, error) {
	if !isUUIDv7(generationUUID) || !isUUIDv7(sectionUUID) {
		return SectionPremise{}, domainError(CodeValidation, "Section 合图关联 UUID 无效", "generation_uuid 与 section_uuid 必须是 UUIDv7。", nil)
	}
	if reader == nil || len(metadata.SelectedAssets) == 0 || len(metadata.SelectedTitles) != len(metadata.SelectedAssets) {
		return SectionPremise{}, domainError(CodeValidation, "Section 合图 metadata 无效", "合图必须包含对应的选中资产、标题和图片信息。", nil)
	}
	encodedMetadata, err := json.Marshal(metadata)
	if err != nil {
		return SectionPremise{}, domainError(CodeValidation, "Section 合图 metadata 无效", "合图 metadata 无法编码。", err)
	}
	asset, err := service.files.CommitReader(ctx, files.CommitInput{
		Purpose:          "comic_section_premise",
		OriginalFilename: "section-premise.png",
		DisplayName:      "Section premise collage",
		SourceType:       "generated",
		Metadata: map[string]any{
			"section_uuid":     sectionUUID,
			"generation_uuid":  generationUUID,
			"composer_version": metadata.ImageInfo.ComposerVersion,
			"selected_titles":  metadata.SelectedTitles,
		},
		Reader: reader,
		Bind: func(tx *gorm.DB, fileID int64) error {
			result := tx.Exec(`UPDATE comic_image_generations SET premise_file_id=?,premise_metadata=? WHERE uuid=? AND comic_section_id=(SELECT id FROM comic_sections WHERE uuid=? AND deleted_at IS NULL)`, fileID, string(encodedMetadata), generationUUID, sectionUUID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return notFound(gorm.ErrRecordNotFound, "Section 图片生成记录不存在")
			}
			return nil
		},
	})
	if err != nil {
		return SectionPremise{}, err
	}
	return sectionPremiseFromMetadata(asset, metadata), nil
}

func (service *Service) sectionPremiseDTO(ctx context.Context, fileID *int64, metadataJSON string) (*SectionPremise, error) {
	if fileID == nil {
		return nil, nil
	}
	var fileUUID string
	if err := service.store.DB().WithContext(ctx).Table("files").Where("id = ?", *fileID).Pluck("uuid", &fileUUID).Error; err != nil {
		return nil, err
	}
	asset, err := service.files.GetAsset(ctx, fileUUID, false)
	if err != nil {
		return nil, err
	}
	var metadata SectionPremiseMetadata
	if err := json.Unmarshal([]byte(strings.TrimSpace(metadataJSON)), &metadata); err != nil {
		return nil, domainError(CodeValidation, "Section 合图 metadata 已损坏", "premise_metadata 不是有效 JSON。", err)
	}
	result := sectionPremiseFromMetadata(asset, metadata)
	return &result, nil
}

func sectionPremiseFromMetadata(asset files.Asset, metadata SectionPremiseMetadata) SectionPremise {
	selectedAssets := append([]PremiseAssetReference(nil), metadata.SelectedAssets...)
	selectedTitles := append([]string(nil), metadata.SelectedTitles...)
	if selectedAssets == nil {
		selectedAssets = []PremiseAssetReference{}
	}
	if selectedTitles == nil {
		selectedTitles = []string{}
	}
	return SectionPremise{
		Asset:           asset,
		SelectedAssets:  selectedAssets,
		SelectedTitles:  selectedTitles,
		SelectionReason: metadata.SelectionReason,
		ImageInfo:       metadata.ImageInfo,
	}
}
