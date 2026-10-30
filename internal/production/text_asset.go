package production

import (
	"context"
	"strings"

	"gorm.io/gorm"
	"lumi/internal/taskexecution"
)

// CreateTextPremiseAsset creates a setting before its artwork exists. The
// profile revision protects the collection from concurrent authoring plans.
func (service *Service) CreateTextPremiseAsset(ctx context.Context, input CreateAssetInput, expectedRevision int64) (PremiseAsset, error) {
	assetType, title, summary := strings.TrimSpace(input.AssetType), strings.TrimSpace(input.Title), strings.TrimSpace(input.Summary)
	if !validAssetType(assetType) || title == "" || len([]rune(title)) > 160 || len([]rune(summary)) > 12000 {
		return PremiseAsset{}, domainError(CodeValidation, "设定内容无效", "asset_type、title 和 summary 必须满足设定字段约束。", nil)
	}
	tags, err := normalizeTags(input.Tags)
	if err != nil {
		return PremiseAsset{}, err
	}
	uuid, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	var row premiseAssetRecord
	err = taskexecution.Transaction(service.store.DB().WithContext(ctx), func(tx *gorm.DB) error {
		profile, err := service.ensurePremiseProfile(ctx, tx)
		if err != nil {
			return err
		}
		now := service.now().UTC()
		result := tx.Model(&premiseProfileRecord{}).Where("id = ? AND revision = ?", profile.ID, expectedRevision).Updates(map[string]any{"revision": gorm.Expr("revision + 1"), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainError(CodeConflict, "设定集合已被修改", "重新读取 settings list 中的 premise.revision 后创建。", nil)
		}
		p, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		row = premiseAssetRecord{UUID: uuid, ProjectID: p.ID, ActorID: actor.ID, AssetType: assetType, Title: title, Summary: summary, PositionJSON: "{}", CropJSON: "{}", CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return conflictErr(err)
		}
		if err := replaceTags(tx, row.ID, tags, now); err != nil {
			return err
		}
		return appendPremiseAssetEvent(tx, row.ID, "asset_created", map[string]any{"asset_uuid": uuid}, now)
	})
	if err != nil {
		return PremiseAsset{}, err
	}
	dto, err := service.premiseAssetDTO(ctx, row.ID)
	if err == nil {
		service.emit("premise:asset_changed", map[string]any{"premise_asset_uuid": uuid})
	}
	return dto, err
}
