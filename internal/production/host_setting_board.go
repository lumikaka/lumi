package production

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
	"lumi/internal/files"
	"lumi/internal/taskexecution"
)

type hostSettingBoardRecord struct {
	ID                                  int64 `gorm:"primaryKey"`
	UUID                                string
	ProjectID, SourceID, SettingImageID int64
	IdempotencyKey, InputHash           string
	Revision                            int64
	CreatedAt                           time.Time
}

func (hostSettingBoardRecord) TableName() string { return "host_setting_boards" }

type hostSettingBreakdownRecord struct {
	ID                                               int64 `gorm:"primaryKey"`
	UUID                                             string
	BoardID                                          int64
	IdempotencyKey, InputHash, InputJSON, OutputJSON string
	CreatedAt                                        time.Time
}

func (hostSettingBreakdownRecord) TableName() string { return "host_setting_breakdowns" }

type HostSettingBoard struct {
	UUID            string                `json:"uuid"`
	Revision        int64                 `json:"revision"`
	Source          PremiseSource         `json:"source"`
	SettingImage    SettingImage          `json:"setting_image"`
	LatestBreakdown *HostSettingBreakdown `json:"latest_breakdown"`
	CreatedAt       time.Time             `json:"created_at"`
}

type HostSettingBoardSummary struct {
	UUID      string    `json:"uuid"`
	Revision  int64     `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
}

type ImportHostSettingBoardInput struct {
	Path, SourceText, StyleSnapshot, Prompt, IdempotencyKey string
	ExpectedRevision                                        int64
}

type HostSettingCrop struct {
	Confidence       float64  `json:"confidence"`
	Type             string   `json:"type"`
	Title            string   `json:"title"`
	Summary          string   `json:"summary"`
	Tags             []string `json:"tags"`
	CropBox          CropBox  `json:"crop_box"`
	SettingUUID      string   `json:"setting_uuid,omitempty"`
	ExpectedRevision *int64   `json:"expected_revision,omitempty"`
}

type ApplyHostSettingBreakdownInput struct {
	BoardUUID               string            `json:"board_uuid"`
	IdempotencyKey          string            `json:"idempotency_key"`
	ExpectedRevision        int64             `json:"expected_revision"`
	ExpectedPremiseRevision int64             `json:"expected_premise_revision"`
	Assets                  []HostSettingCrop `json:"assets"`
}

type HostSettingBreakdownItem struct {
	SettingUUID string  `json:"setting_uuid"`
	VariantUUID string  `json:"variant_uuid"`
	AssetUUID   string  `json:"asset_uuid"`
	Revision    int64   `json:"revision"`
	CropBox     CropBox `json:"crop_box"`
}

type HostSettingBreakdown struct {
	UUID            string                     `json:"uuid"`
	BoardUUID       string                     `json:"board_uuid"`
	Revision        int64                      `json:"revision"`
	PremiseRevision int64                      `json:"premise_revision"`
	Items           []HostSettingBreakdownItem `json:"items"`
}

func validHostKey(key string) bool { return strings.TrimSpace(key) != "" && len(key) <= 160 }
func hostConflict() error {
	return domainError(CodeConflict, "批量设定已被修改", "重读 setting_boards get 和 settings list/get，核对内容后使用新的 revision；已成功的请求应使用原幂等键和原输入重试。", nil)
}
func hostReplayConflict() error {
	return domainError(CodeStateConflict, "批量设定幂等输入冲突", "同一 idempotency_key 必须对应完全相同的输入；新操作使用新键。", nil)
}
func (service *Service) requireHostSettingWrite(ctx context.Context) error {
	if err := service.store.RequireReady(); err != nil {
		return err
	}
	return service.store.RequireInitializationAvailable(ctx)
}

func (service *Service) hostBoardRecord(ctx context.Context, db *gorm.DB, uuid string) (hostSettingBoardRecord, error) {
	var row hostSettingBoardRecord
	err := db.WithContext(ctx).Where("uuid=? AND project_id=(SELECT id FROM projects WHERE uuid=?)", uuid, service.store.ProjectUUID()).Take(&row).Error
	return row, notFound(err, "设定总览图批次不存在")
}

func (service *Service) GetHostSettingBoard(ctx context.Context, uuid string) (HostSettingBoard, error) {
	row, err := service.hostBoardRecord(ctx, service.store.DB(), uuid)
	if err != nil {
		return HostSettingBoard{}, err
	}
	var source premiseSourceRecord
	if err := service.store.DB().WithContext(ctx).First(&source, row.SourceID).Error; err != nil {
		return HostSettingBoard{}, err
	}
	setting, err := service.settingImageDTO(ctx, row.SettingImageID)
	if err != nil {
		return HostSettingBoard{}, err
	}
	result := HostSettingBoard{UUID: row.UUID, Revision: row.Revision, Source: sourceDTO(source), SettingImage: setting, CreatedAt: row.CreatedAt}
	var latest hostSettingBreakdownRecord
	err = service.store.DB().WithContext(ctx).Where("board_id=?", row.ID).Order("id DESC").Take(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.LatestBreakdown = &HostSettingBreakdown{}
	err = json.Unmarshal([]byte(latest.OutputJSON), result.LatestBreakdown)
	return result, err
}

func (service *Service) ListHostSettingBoards(ctx context.Context) ([]HostSettingBoardSummary, error) {
	rows := []HostSettingBoardSummary{}
	err := service.store.DB().WithContext(ctx).Model(&hostSettingBoardRecord{}).Select("uuid,revision,created_at").Where("project_id=(SELECT id FROM projects WHERE uuid=?)", service.store.ProjectUUID()).Order("id DESC").Scan(&rows).Error
	return rows, err
}

// ImportHostSettingBoard commits the source, original image and resumable batch
// identity together. A successful key can be replayed after host file expiry.
func (service *Service) ImportHostSettingBoard(ctx context.Context, input ImportHostSettingBoardInput) (HostSettingBoard, error) {
	if err := service.requireHostSettingWrite(ctx); err != nil {
		return HostSettingBoard{}, err
	}
	input.SourceText, input.StyleSnapshot = strings.TrimSpace(input.SourceText), strings.TrimSpace(input.StyleSnapshot)
	if !validHostKey(input.IdempotencyKey) || input.ExpectedRevision < 0 || input.SourceText == "" || len([]rune(input.SourceText)) > 12000 || len([]rune(input.StyleSnapshot)) > 12000 || len([]rune(input.Prompt)) > 12000 || !filepath.IsAbs(input.Path) {
		return HostSettingBoard{}, domainError(CodeValidation, "总览图导入参数无效", "需要真实绝对路径、source_text、有效 revision、幂等键；源描述、画风和提示词各最多 12000 字符。", nil)
	}
	raw, _ := json.Marshal(input)
	fingerprint := hashJSON(raw)
	replay := func(db *gorm.DB) (hostSettingBoardRecord, bool, error) {
		var row hostSettingBoardRecord
		err := db.Where("project_id=(SELECT id FROM projects WHERE uuid=?) AND idempotency_key=?", service.store.ProjectUUID(), input.IdempotencyKey).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, false, nil
		}
		if err != nil {
			return row, false, err
		}
		if row.InputHash != fingerprint {
			return row, false, hostReplayConflict()
		}
		return row, true, nil
	}
	if row, found, err := replay(service.store.DB().WithContext(ctx)); err != nil || found {
		if err != nil {
			return HostSettingBoard{}, err
		}
		return service.GetHostSettingBoard(ctx, row.UUID)
	}
	profile, err := service.GetPremise(ctx)
	if err != nil {
		return HostSettingBoard{}, err
	}
	if profile.Revision != input.ExpectedRevision {
		return HostSettingBoard{}, hostConflict()
	}
	style := input.StyleSnapshot
	if style == "" {
		style = profile.DefaultStyle
	}
	data, _, err := readHostImage(ctx, input.Path)
	if err != nil {
		return HostSettingBoard{}, err
	}
	boardUUID, err := newUUIDv7()
	if err != nil {
		return HostSettingBoard{}, err
	}
	sourceUUID, err := newUUIDv7()
	if err != nil {
		return HostSettingBoard{}, err
	}
	settingUUID, err := newUUIDv7()
	if err != nil {
		return HostSettingBoard{}, err
	}
	_, err = service.files.CommitReader(ctx, files.CommitInput{Purpose: "premise_setting_image", OriginalFilename: filepath.Base(input.Path), SourceType: "imported", Metadata: map[string]any{"origin": "host_generated", "original_prompt": input.Prompt}, Reader: bytes.NewReader(data), Bind: func(tx *gorm.DB, fileID int64) error {
		if row, found, err := replay(tx); err != nil || found {
			if found {
				boardUUID = row.UUID
			}
			return err
		}
		p, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		now := service.now().UTC()
		changed := tx.Model(&premiseProfileRecord{}).Where("project_id=? AND revision=?", p.ID, input.ExpectedRevision).Updates(map[string]any{"revision": gorm.Expr("revision+1"), "updated_at": now})
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return hostConflict()
		}
		source := premiseSourceRecord{UUID: sourceUUID, ProjectID: p.ID, ActorID: actor.ID, SourceType: "generated", SourceText: input.SourceText, StyleSnapshot: style, ParametersJSON: `{"origin":"host_generated"}`, CreatedAt: now}
		if err := tx.Create(&source).Error; err != nil {
			return err
		}
		setting := settingImageRecord{UUID: settingUUID, ProjectID: p.ID, SourceID: &source.ID, FileID: fileID, Origin: "generated", Prompt: input.Prompt, CreatedAt: now}
		if err := tx.Create(&setting).Error; err != nil {
			return err
		}
		// Keep an existing user selection; this batch always addresses its own image.
		if err := tx.Model(&premiseProfileRecord{}).Where("project_id=?", p.ID).Updates(map[string]any{"current_source_id": source.ID, "current_setting_image_id": gorm.Expr("COALESCE(current_setting_image_id, ?)", setting.ID)}).Error; err != nil {
			return err
		}
		return tx.Create(&hostSettingBoardRecord{UUID: boardUUID, ProjectID: p.ID, SourceID: source.ID, SettingImageID: setting.ID, IdempotencyKey: input.IdempotencyKey, InputHash: fingerprint, CreatedAt: now}).Error
	}})
	if err != nil {
		return HostSettingBoard{}, err
	}
	result, err := service.GetHostSettingBoard(ctx, boardUUID)
	if err == nil {
		service.emit("premise:setting_image_changed", map[string]any{"board_uuid": boardUUID, "source_uuid": result.Source.UUID, "setting_image_uuid": result.SettingImage.UUID})
	}
	return result, err
}

func validateHostCrops(items []HostSettingCrop) error {
	if len(items) < 1 || len(items) > 6 {
		return domainError(CodeValidation, "拆分数量无效", "每批提交 1 到 6 个独立主体。", nil)
	}
	titles, targets := map[string]bool{}, map[string]bool{}
	for i, item := range items {
		title := strings.TrimSpace(item.Title)
		key := strings.ToLower(title)
		if !validAssetType(item.Type) || title == "" || title != item.Title || len([]rune(title)) > 160 || len([]rune(item.Summary)) > 12000 || titles[key] || !item.CropBox.valid() || len(item.Tags) < 1 || len(item.Tags) > 3 || !(item.Confidence >= 0.92 && item.Confidence <= 1) {
			return domainError(CodeValidation, "拆分设定无效", "名称需唯一，类型、描述、标签和 crop_box 必须有效。", nil)
		}
		titles[key] = true
		if _, err := normalizeTags(item.Tags); err != nil {
			return err
		}
		if item.SettingUUID != "" {
			if !isUUIDv7(item.SettingUUID) || item.ExpectedRevision == nil || *item.ExpectedRevision < 0 || targets[item.SettingUUID] {
				return domainError(CodeValidation, "已有设定目标无效", "更新时提供唯一 setting_uuid 和最新 expected_revision。", nil)
			}
			targets[item.SettingUUID] = true
		} else if item.ExpectedRevision != nil {
			return domainError(CodeValidation, "新设定不能携带 revision", "只有已有 setting_uuid 需要 expected_revision。", nil)
		}
		for _, other := range items[:i] {
			a, b := item.CropBox, other.CropBox
			if a.X < b.X+b.Width && b.X < a.X+a.Width && a.Y < b.Y+b.Height && b.Y < a.Y+a.Height {
				return domainError(CodeValidation, "裁切框重叠", "每个框应独立包含一个完整主体。", nil)
			}
		}
	}
	return nil
}

func replayHostBreakdown(db *gorm.DB, boardID int64, key, hash string) (HostSettingBreakdown, bool, error) {
	var row hostSettingBreakdownRecord
	var result HostSettingBreakdown
	err := db.Where("board_id=? AND idempotency_key=?", boardID, key).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, false, nil
	}
	if err != nil {
		return result, false, err
	}
	if row.InputHash != hash {
		return result, false, hostReplayConflict()
	}
	err = json.Unmarshal([]byte(row.OutputJSON), &result)
	return result, err == nil, err
}

func (service *Service) hostCropTarget(tx *gorm.DB, projectID int64, item HostSettingCrop) (premiseAssetRecord, error) {
	var row premiseAssetRecord
	if item.SettingUUID != "" {
		err := tx.Where("project_id=? AND uuid=? AND deleted_at IS NULL", projectID, item.SettingUUID).Take(&row).Error
		if err != nil {
			return row, notFound(err, "目标设定不存在")
		}
		if row.Revision != *item.ExpectedRevision {
			return row, hostConflict()
		}
	}
	var count int64
	if err := tx.Model(&premiseAssetRecord{}).Where("project_id=? AND lower(title)=lower(?) AND deleted_at IS NULL AND id<>?", projectID, item.Title, row.ID).Count(&count).Error; err != nil {
		return row, err
	}
	if count > 0 {
		return row, domainError(CodeConflict, "已存在同名设定", "重读设定，明确提供 setting_uuid 与 expected_revision，或使用不同名称。", nil)
	}
	return row, nil
}

// ApplyHostSettingBreakdown stages durable crops before atomically binding the
// complete set. Failed/cancelled staging exposes no partial setting changes.
func (service *Service) ApplyHostSettingBreakdown(ctx context.Context, input ApplyHostSettingBreakdownInput) (HostSettingBreakdown, error) {
	var result HostSettingBreakdown
	if err := service.requireHostSettingWrite(ctx); err != nil {
		return result, err
	}
	if !validHostKey(input.IdempotencyKey) || input.ExpectedRevision < 0 || input.ExpectedPremiseRevision < 0 {
		return result, domainError(CodeValidation, "拆分参数无效", "需要有效 revision 和幂等键。", nil)
	}
	if err := validateHostCrops(input.Assets); err != nil {
		return result, err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return result, err
	}
	fingerprint := hashJSON(raw)
	board, err := service.hostBoardRecord(ctx, service.store.DB(), input.BoardUUID)
	if err != nil {
		return result, err
	}
	if previous, found, err := replayHostBreakdown(service.store.DB().WithContext(ctx), board.ID, input.IdempotencyKey, fingerprint); err != nil || found {
		return previous, err
	}
	check := func(tx *gorm.DB) error {
		current, err := service.hostBoardRecord(ctx, tx, input.BoardUUID)
		if err != nil {
			return err
		}
		profile, err := service.ensurePremiseProfile(ctx, tx)
		if err != nil {
			return err
		}
		if current.Revision != input.ExpectedRevision || profile.Revision != input.ExpectedPremiseRevision {
			return hostConflict()
		}
		for _, item := range input.Assets {
			if _, err := service.hostCropTarget(tx, board.ProjectID, item); err != nil {
				return err
			}
		}
		return nil
	}
	if err := check(service.store.DB().WithContext(ctx)); err != nil {
		return result, err
	}
	setting, err := service.settingImageDTO(ctx, board.SettingImageID)
	if err != nil {
		return result, err
	}
	content, err := service.files.OpenContent(ctx, setting.Asset.UUID)
	if err != nil {
		return result, err
	}
	data, err := io.ReadAll(io.LimitReader(content.File, HostImageMaxBytes+1))
	content.File.Close()
	if err != nil {
		return result, err
	}
	if len(data) > HostImageMaxBytes {
		return result, hostImageError(files.CodeFileTooLarge, "总览图过大", "最多 32 MiB。")
	}
	source, err := decodeHostImage(data)
	if err != nil {
		return result, err
	}
	// Validate all pixel bounds before writing any derived files.
	inputs := make([]CreateAssetInput, len(input.Assets))
	for i, item := range input.Assets {
		box, err := settingCropBounds(source.Bounds(), item.CropBox)
		if err != nil {
			return result, err
		}
		inputs[i] = CreateAssetInput{AssetType: item.Type, Title: item.Title, Summary: item.Summary, Tags: item.Tags, Position: map[string]any{"x": item.CropBox.X, "y": item.CropBox.Y}, Crop: map[string]any{"x": box.Min.X, "y": box.Min.Y, "width": box.Dx(), "height": box.Dy()}}
	}
	staged := make([]files.Asset, len(input.Assets))
	for i, item := range input.Assets {
		cropped, _, err := CropSettingImage(source, item.CropBox)
		if err != nil {
			return result, err
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, cropped); err != nil {
			return result, err
		}
		cropMetadata, err := encodeJSON(inputs[i].Crop, "{}")
		if err != nil {
			return result, err
		}
		staged[i], err = service.files.CommitReader(ctx, files.CommitInput{Purpose: "premise_asset", OriginalFilename: "breakdown-crop.png", DisplayName: item.Title, SourceType: "derived", SourceAssetUUID: setting.Asset.UUID, Metadata: map[string]any{"origin": "host_generated", "board_uuid": board.UUID, "crop_profile": cropMetadata}, Reader: bytes.NewReader(encoded.Bytes())})
		if err != nil {
			return result, err
		}
	}
	err = taskexecution.Transaction(service.store.DB().WithContext(ctx), func(tx *gorm.DB) error {
		if previous, found, err := replayHostBreakdown(tx, board.ID, input.IdempotencyKey, fingerprint); err != nil || found {
			result = previous
			return err
		}
		if err := check(tx); err != nil {
			return err
		}
		p, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		now := service.now().UTC()
		uuid, err := newUUIDv7()
		if err != nil {
			return err
		}
		result = HostSettingBreakdown{UUID: uuid, BoardUUID: board.UUID, Revision: board.Revision + 1, PremiseRevision: input.ExpectedPremiseRevision + 1, Items: []HostSettingBreakdownItem{}}
		for i, item := range input.Assets {
			record, err := service.hostCropTarget(tx, p.ID, item)
			if err != nil {
				return err
			}
			existing := record.ID != 0
			if !existing {
				uuid, err := newUUIDv7()
				if err != nil {
					return err
				}
				record = premiseAssetRecord{UUID: uuid, ProjectID: p.ID, ActorID: actor.ID, AssetType: item.Type, Title: item.Title, Summary: item.Summary, PositionJSON: "{}", CropJSON: "{}", CreatedAt: now, UpdatedAt: now}
			}
			var fileID int64
			if err := tx.Table("files").Select("id").Where("project_id=? AND uuid=? AND deleted_at IS NULL", p.ID, staged[i].UUID).Take(&fileID).Error; err != nil {
				return err
			}
			revision := record.Revision
			if existing {
				revision++
			}
			variant, err := service.bindBreakdownAsset(tx, &record, existing, board.SettingImageID, fileID, inputs[i])
			if err != nil {
				return err
			}
			if err := appendPremiseAssetEvent(tx, record.ID, "asset_saved_from_host_breakdown", map[string]any{"asset_uuid": record.UUID, "variant_uuid": variant.UUID, "setting_image_uuid": setting.UUID, "board_uuid": board.UUID, "breakdown_uuid": result.UUID}, now); err != nil {
				return err
			}
			result.Items = append(result.Items, HostSettingBreakdownItem{SettingUUID: record.UUID, VariantUUID: variant.UUID, AssetUUID: staged[i].UUID, Revision: revision, CropBox: item.CropBox})
		}
		changed := tx.Model(&hostSettingBoardRecord{}).Where("id=? AND revision=?", board.ID, input.ExpectedRevision).Update("revision", result.Revision)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return hostConflict()
		}
		changed = tx.Model(&premiseProfileRecord{}).Where("project_id=? AND revision=?", p.ID, input.ExpectedPremiseRevision).Updates(map[string]any{"revision": result.PremiseRevision, "updated_at": now})
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return hostConflict()
		}
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.Create(&hostSettingBreakdownRecord{UUID: result.UUID, BoardID: board.ID, IdempotencyKey: input.IdempotencyKey, InputHash: fingerprint, InputJSON: string(raw), OutputJSON: string(output), CreatedAt: now}).Error
	})
	if err == nil {
		service.emit("premise:assets_changed", map[string]any{"board_uuid": board.UUID, "setting_image_uuid": setting.UUID, "breakdown_uuid": result.UUID})
	}
	return result, err
}
