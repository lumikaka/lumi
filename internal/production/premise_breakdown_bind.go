package production

import (
	"gorm.io/gorm"
	"strings"
)

// bindBreakdownAsset binds durable pixels to the same setting/version/tag
// model for both production tasks and host batches. Callers enforce ownership,
// concurrency and replay rules in this transaction before invoking it.
func (service *Service) bindBreakdownAsset(tx *gorm.DB, record *premiseAssetRecord, existing bool, settingID, fileID int64, input CreateAssetInput) (assetVariantRecord, error) {
	var variant assetVariantRecord
	input.AssetType, input.Title, input.Summary = strings.TrimSpace(input.AssetType), strings.TrimSpace(input.Title), strings.TrimSpace(input.Summary)
	tags, err := normalizeTags(input.Tags)
	if err != nil {
		return variant, err
	}
	position, err := encodeJSON(input.Position, "{}")
	if err != nil {
		return variant, err
	}
	crop, err := encodeJSON(input.Crop, "{}")
	if err != nil {
		return variant, err
	}
	now := service.now().UTC()
	if !existing {
		if err := tx.Create(record).Error; err != nil {
			return variant, conflictErr(err)
		}
	}
	variantUUID, err := newUUIDv7()
	if err != nil {
		return variant, err
	}
	var version int
	if err := tx.Model(&assetVariantRecord{}).Where("premise_asset_id = ?", record.ID).Select("COALESCE(MAX(version_no),0)+1").Scan(&version).Error; err != nil {
		return variant, err
	}
	variant = assetVariantRecord{UUID: variantUUID, PremiseAssetID: record.ID, FileID: fileID, SourceSettingImageID: &settingID, VersionNo: version, SourceType: "breakdown", CropJSON: crop, CreatedAt: now}
	if err := tx.Create(&variant).Error; err != nil {
		return variant, err
	}
	updates := map[string]any{"asset_type": input.AssetType, "title": input.Title, "summary": input.Summary, "position_json": position, "crop_json": crop, "current_variant_id": variant.ID, "updated_at": now}
	if existing {
		updates["revision"] = gorm.Expr("revision + 1")
	}
	if err := tx.Model(record).Updates(updates).Error; err != nil {
		return variant, err
	}
	return variant, replaceTags(tx, record.ID, tags, now)
}
