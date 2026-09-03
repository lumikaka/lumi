package production

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type projectCreationReferenceBinding struct {
	ID, FileID, PremiseAssetID                                                    int64
	UUID, FileUUID, ReferenceRole, Title, Instruction, Purpose, Kind, ObjectState string
	Position                                                                      int
	IncludeInYolo                                                                 bool
	FileDeletedAt                                                                 *time.Time
}

func projectReferenceAssetType(role string) string {
	switch role {
	case "character", "scene", "prop":
		return role
	default:
		return "reference"
	}
}

func projectReferenceSummary(role, instruction string) string {
	roleSummary := map[string]string{
		"character": "首页创建人物参考；保留身份、外貌、服装和标志性特征。",
		"scene":     "首页创建场景参考；参考空间、建筑、材质、光照和环境气氛。",
		"prop":      "首页创建道具参考；参考形态、结构、材质和颜色。",
		"style":     "首页创建画风参考；只参考线条、色彩、纹理和光照。",
		"auto":      "首页创建通用视觉参考。",
	}[role]
	if roleSummary == "" {
		roleSummary = "首页创建通用视觉参考。"
	}
	if instruction = strings.TrimSpace(instruction); instruction != "" {
		return roleSummary + " 用户说明：" + instruction
	}
	return roleSummary
}

func projectReferenceTitle(base, suffix string) string {
	const maximum = 160
	runes := []rune(strings.TrimSpace(base))
	suffixRunes := []rune(suffix)
	if len(runes)+len(suffixRunes) > maximum {
		runes = runes[:maximum-len(suffixRunes)]
	}
	return strings.TrimSpace(string(runes)) + suffix
}

func loadProjectCreationReferenceBinding(ctx context.Context, tx *gorm.DB, projectID int64, reference GenerationReferenceFile) (projectCreationReferenceBinding, error) {
	var row projectCreationReferenceBinding
	err := tx.WithContext(ctx).Table("project_creation_reference_files AS refs").
		Select(`refs.id,refs.uuid,refs.position,refs.file_id,COALESCE(refs.premise_asset_id,0) AS premise_asset_id,
			refs.reference_role,refs.title,refs.instruction,refs.include_in_yolo,
			files.uuid AS file_uuid,files.purpose,files.kind,files.deleted_at AS file_deleted_at,objects.state AS object_state`).
		Joins("JOIN files ON files.id=refs.file_id").Joins("JOIN file_objects AS objects ON objects.id=files.file_object_id").
		Where("refs.project_id=? AND refs.uuid=? AND files.uuid=?", projectID, reference.ReferenceUUID, reference.FileUUID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, domainError(CodeValidation, "创建参考图不存在", "自动生成快照中的 reference_uuid/file_uuid 不属于当前项目。", err)
	}
	return row, err
}

// BindProjectCreationReferenceAsset reuses an immutable home-page reference
// File as a Premise Asset. It is intentionally internal and never accepts an
// arbitrary public API file UUID.
func (service *Service) BindProjectCreationReferenceAsset(ctx context.Context, reference GenerationReferenceFile) (PremiseAsset, error) {
	if !isUUIDv7(strings.TrimSpace(reference.ReferenceUUID)) || !isUUIDv7(strings.TrimSpace(reference.FileUUID)) {
		return PremiseAsset{}, domainError(CodeValidation, "创建参考图 UUID 无效", "reference_uuid 与 file_uuid 必须是 UUIDv7。", nil)
	}
	var domainID int64
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		projectRecord, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		binding, err := loadProjectCreationReferenceBinding(ctx, tx, projectRecord.ID, reference)
		if err != nil {
			return err
		}
		if !binding.IncludeInYolo || binding.Purpose != "project_chatbot_reference" || binding.Kind != "image" || binding.ObjectState != "ready" || binding.FileDeletedAt != nil {
			return domainError(CodeValidation, "创建参考图不可用", "参考图必须 included、已 finalize、未删除且用途为 project_chatbot_reference。", nil)
		}
		now := service.now().UTC()
		if binding.PremiseAssetID != 0 {
			var existing premiseAssetRecord
			if err := tx.First(&existing, binding.PremiseAssetID).Error; err != nil {
				return err
			}
			if existing.DeletedAt != nil {
				if err := tx.Model(&existing).Updates(map[string]any{"deleted_at": nil, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
					return err
				}
				if err := appendPremiseAssetEvent(tx, existing.ID, "asset_restored_for_project_reference", map[string]any{"asset_uuid": existing.UUID, "reference_uuid": binding.UUID, "file_uuid": binding.FileUUID}, now); err != nil {
					return err
				}
			}
			domainID = existing.ID
			return tx.Table("project_creation_reference_files").Where("id=?", binding.ID).Updates(map[string]any{"imported_at": now, "updated_at": now}).Error
		}

		role := strings.ToLower(strings.TrimSpace(reference.ReferenceRole))
		if role != "auto" && role != "character" && role != "scene" && role != "prop" && role != "style" {
			return domainError(CodeValidation, "创建参考图用途无效", "reference_role 不受支持。", nil)
		}
		title := strings.TrimSpace(reference.Title)
		if title == "" || len([]rune(title)) > 160 {
			return domainError(CodeValidation, "创建参考图标题无效", "title 必须是 1 到 160 个字符。", nil)
		}
		candidates := []string{
			title,
			projectReferenceTitle(title, fmt.Sprintf(" · 参考图 %d", reference.Position)),
			projectReferenceTitle(title, fmt.Sprintf(" · 参考图 %d-%s", reference.Position, reference.ReferenceUUID[:8])),
		}
		availableTitle := ""
		for _, candidate := range candidates {
			var count int64
			if err := tx.Model(&premiseAssetRecord{}).Where("project_id=? AND lower(title)=lower(?) AND deleted_at IS NULL", projectRecord.ID, candidate).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				availableTitle = candidate
				break
			}
		}
		if availableTitle == "" {
			return domainError(CodeStateConflict, "创建参考图标题冲突", "确定性的参考图标题已经存在，请调整 Setup 中的标题后重试。", nil)
		}
		title = availableTitle
		assetUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		variantUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		asset := premiseAssetRecord{UUID: assetUUID, ProjectID: projectRecord.ID, ActorID: actor.ID, AssetType: projectReferenceAssetType(role), Title: title, Summary: projectReferenceSummary(role, reference.Instruction), PositionJSON: "{}", CropJSON: "{}", CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&asset).Error; err != nil {
			return conflictErr(err)
		}
		variant := assetVariantRecord{UUID: variantUUID, PremiseAssetID: asset.ID, FileID: binding.FileID, VersionNo: 1, SourceType: "manual", CropJSON: "{}", CreatedAt: now}
		if err := tx.Create(&variant).Error; err != nil {
			return err
		}
		if err := tx.Model(&asset).Update("current_variant_id", variant.ID).Error; err != nil {
			return err
		}
		if err := replaceTags(tx, asset.ID, []string{"project-creation-reference", "reference-role-" + role}, now); err != nil {
			return err
		}
		if err := appendPremiseAssetEvent(tx, asset.ID, "asset_created_from_project_reference", map[string]any{"asset_uuid": asset.UUID, "variant_uuid": variant.UUID, "reference_uuid": binding.UUID, "file_uuid": binding.FileUUID, "reference_role": role, "position": reference.Position}, now); err != nil {
			return err
		}
		result := tx.Table("project_creation_reference_files").Where("id=? AND premise_asset_id IS NULL", binding.ID).Updates(map[string]any{"premise_asset_id": asset.ID, "imported_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainError(CodeStateConflict, "创建参考图绑定已变化", "请重试 Premise 步骤。", nil)
		}
		domainID = asset.ID
		return nil
	})
	if err != nil {
		return PremiseAsset{}, err
	}
	result, err := service.premiseAssetDTO(ctx, domainID)
	if err == nil {
		service.emit("premise:asset_changed", map[string]any{"premise_asset_uuid": result.UUID, "reference_uuid": reference.ReferenceUUID})
	}
	return result, err
}

func (service *Service) projectReferenceAssetByBreakdownTitle(ctx context.Context, taskUUID, title string) (PremiseAsset, bool, error) {
	var assetID int64
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("project_creation_reference_files AS refs").Select("assets.id").
			Joins("JOIN premise_assets AS assets ON assets.id=refs.premise_asset_id").
			Where("refs.project_id=(SELECT project_id FROM production_task_runs WHERE uuid=?) AND assets.deleted_at IS NULL AND lower(assets.title)=lower(?)", taskUUID, strings.TrimSpace(title)).
			Limit(1).Scan(&assetID).Error; err != nil {
			return err
		}
		if assetID == 0 {
			return nil
		}
		var exists bool
		if err := tx.Table("premise_asset_events").Select("COUNT(*) > 0").Where("premise_asset_id=? AND event_type='breakdown_matched_project_reference' AND json_extract(payload,'$.task_uuid')=? AND json_extract(payload,'$.title_key')=?", assetID, taskUUID, strings.ToLower(strings.TrimSpace(title))).Scan(&exists).Error; err != nil {
			return err
		}
		if !exists {
			var assetUUID string
			if err := tx.Model(&premiseAssetRecord{}).Select("uuid").Where("id=?", assetID).Scan(&assetUUID).Error; err != nil {
				return err
			}
			if err := appendPremiseAssetEvent(tx, assetID, "breakdown_matched_project_reference", map[string]any{"asset_uuid": assetUUID, "task_uuid": taskUUID, "title_key": strings.ToLower(strings.TrimSpace(title))}, service.now().UTC()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil || assetID == 0 {
		return PremiseAsset{}, false, err
	}
	asset, err := service.premiseAssetDTO(ctx, assetID)
	return asset, err == nil, err
}
