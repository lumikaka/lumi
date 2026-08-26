package production

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"lumi/internal/files"
	"lumi/internal/promptcatalog"

	"gorm.io/gorm"
)

type premiseProfileRecord struct {
	ID                    int64 `gorm:"primaryKey"`
	UUID                  string
	ProjectID             int64
	DefaultStyle          string
	CurrentSourceID       *int64
	CurrentSettingImageID *int64
	Revision              int64
	CreatedAt, UpdatedAt  time.Time
}

func (premiseProfileRecord) TableName() string { return "premise_profiles" }

type premiseSourceRecord struct {
	ID                                                                         int64 `gorm:"primaryKey"`
	UUID                                                                       string
	ProjectID, ActorID                                                         int64
	SourceType, SourceText, StyleSnapshot, ProviderUUID, Model, ParametersJSON string
	IgnoredAt                                                                  *time.Time
	Revision                                                                   int64
	CreatedAt                                                                  time.Time
}

func (premiseSourceRecord) TableName() string { return "premise_sources" }

type settingImageRecord struct {
	ID             int64 `gorm:"primaryKey"`
	UUID           string
	ProjectID      int64
	SourceID       *int64
	FileID         int64
	Origin, Prompt string
	CreatedAt      time.Time
}

func (settingImageRecord) TableName() string { return "premise_setting_images" }

type premiseAssetRecord struct {
	ID                                                int64 `gorm:"primaryKey"`
	UUID                                              string
	ProjectID, ActorID                                int64
	CurrentVariantID                                  *int64
	AssetType, Title, Summary, PositionJSON, CropJSON string
	Revision                                          int64
	DeletedAt                                         *time.Time
	CreatedAt, UpdatedAt                              time.Time
}

func (premiseAssetRecord) TableName() string { return "premise_assets" }

type premiseTagRecord struct {
	ID             int64 `gorm:"primaryKey"`
	PremiseAssetID int64
	Tag            string
	CreatedAt      time.Time
}

func (premiseTagRecord) TableName() string { return "premise_asset_tags" }

type assetVariantRecord struct {
	ID                     int64 `gorm:"primaryKey"`
	UUID                   string
	PremiseAssetID, FileID int64
	SourceSettingImageID   *int64
	VersionNo              int
	SourceType, CropJSON   string
	CreatedAt              time.Time
}

func (assetVariantRecord) TableName() string { return "premise_asset_variants" }

func (service *Service) ensurePremiseProfile(ctx context.Context, db *gorm.DB) (premiseProfileRecord, error) {
	p, _, err := service.projectActor(ctx, db)
	if err != nil {
		return premiseProfileRecord{}, err
	}
	var record premiseProfileRecord
	err = db.WithContext(ctx).Where("project_id = ?", p.ID).First(&record).Error
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return record, err
	}
	uuid, err := newUUIDv7()
	if err != nil {
		return record, err
	}
	now := service.now().UTC()
	record = premiseProfileRecord{UUID: uuid, ProjectID: p.ID, DefaultStyle: DefaultPremiseStyle(p.GenerationLanguage), CreatedAt: now, UpdatedAt: now}
	if err := db.WithContext(ctx).Create(&record).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return service.ensurePremiseProfile(ctx, db)
		}
		return record, err
	}
	return record, nil
}

func (service *Service) GetPremise(ctx context.Context) (PremiseProfile, error) {
	record, err := service.ensurePremiseProfile(ctx, service.store.DB())
	if err != nil {
		return PremiseProfile{}, err
	}
	effectiveStyle := record.DefaultStyle
	var canonicalStyle string
	if err := service.store.DB().WithContext(ctx).Table("project_prompt_versions").
		Select("prompt").
		Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", record.ProjectID, promptcatalog.GroupPremiseStyle, "project_overall_style").
		Order("version_no DESC").Limit(1).Scan(&canonicalStyle).Error; err != nil {
		return PremiseProfile{}, err
	}
	if strings.TrimSpace(canonicalStyle) != "" {
		effectiveStyle = strings.TrimSpace(canonicalStyle)
	}
	result := PremiseProfile{UUID: record.UUID, DefaultStyle: effectiveStyle, Revision: record.Revision, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
	if record.CurrentSourceID != nil {
		var source premiseSourceRecord
		if err := service.store.DB().WithContext(ctx).First(&source, *record.CurrentSourceID).Error; err == nil {
			dto := sourceDTO(source)
			result.CurrentSource = &dto
		}
	}
	if record.CurrentSettingImageID != nil {
		image, imageErr := service.settingImageDTO(ctx, *record.CurrentSettingImageID)
		if imageErr == nil {
			result.CurrentSettingImage = &image
		}
	}
	return result, nil
}

func (service *Service) UpdatePremise(ctx context.Context, input UpdatePremiseInput) (PremiseProfile, error) {
	style := strings.TrimSpace(input.DefaultStyle)
	if len([]rune(style)) > 12000 {
		return PremiseProfile{}, domainError(CodeValidation, "画风说明过长", "default_style 最多 12000 个字符。", nil)
	}
	now := service.now().UTC()
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, err := service.ensurePremiseProfile(ctx, tx)
		if err != nil {
			return err
		}
		if record.Revision != input.ExpectedRevision {
			return domainError(CodeConflict, "Premise 已被修改", "刷新后基于最新 revision 重试。", nil)
		}
		projectRecord, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		if style == "" {
			style = promptcatalog.DefaultProjectStyle(projectRecord.GenerationLanguage)
		}
		type promptCurrent struct {
			VersionNo  int
			PromptHash string
		}
		var current promptCurrent
		loadErr := tx.WithContext(ctx).Table("project_prompt_versions").
			Select("version_no, prompt_hash").
			Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", projectRecord.ID, promptcatalog.GroupPremiseStyle, "project_overall_style").
			Order("version_no DESC").Limit(1).Scan(&current).Error
		if loadErr != nil {
			return loadErr
		}
		styleHash := hashJSON([]byte(style))
		if current.VersionNo == 0 || current.PromptHash != styleHash {
			versionUUID, uuidErr := newUUIDv7()
			if uuidErr != nil {
				return uuidErr
			}
			sourceType := "manual_edit"
			if styleHash == hashJSON([]byte(strings.TrimSpace(promptcatalog.DefaultProjectStyle(projectRecord.GenerationLanguage)))) {
				sourceType = "default_restore"
			}
			if createErr := tx.WithContext(ctx).Exec(`
				INSERT INTO project_prompt_versions(
					uuid, project_id, actor_id, prompt_group, prompt_key, version_no,
					prompt, prompt_hash, source_type, created_at
				) VALUES(?, ?, ?, ?, 'project_overall_style', ?, ?, ?, ?, ?)
			`, versionUUID, projectRecord.ID, actor.ID, promptcatalog.GroupPremiseStyle, current.VersionNo+1, style, styleHash, sourceType, now).Error; createErr != nil {
				return createErr
			}
		}
		result := tx.WithContext(ctx).Model(&premiseProfileRecord{}).
			Where("id = ? AND revision = ?", record.ID, input.ExpectedRevision).
			Updates(map[string]any{"default_style": style, "revision": gorm.Expr("revision + 1"), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainError(CodeConflict, "Premise 已被修改", "刷新后基于最新 revision 重试。", nil)
		}
		return nil
	})
	if err != nil {
		return PremiseProfile{}, err
	}
	profile, err := service.GetPremise(ctx)
	if err == nil {
		service.emit("premise:changed", map[string]any{"premise_uuid": profile.UUID, "revision": profile.Revision})
	}
	return profile, err
}

func (service *Service) CreatePremiseSource(ctx context.Context, input CreateSourceInput) (PremiseSource, error) {
	text := strings.TrimSpace(input.SourceText)
	style := strings.TrimSpace(input.StyleSnapshot)
	sourceType := strings.TrimSpace(input.SourceType)
	if sourceType == "" {
		sourceType = "manual"
	}
	if text == "" || len([]rune(text)) > 262144 || len([]rune(style)) > 12000 || (sourceType != "manual" && sourceType != "generated") {
		return PremiseSource{}, domainError(CodeValidation, "Premise source 无效", "source_text 不能为空，style_snapshot 最多 12000 字符，source_type 只支持 manual/generated。", nil)
	}
	providerUUID := strings.TrimSpace(input.ProviderUUID)
	if providerUUID != "" && !isUUIDv7(providerUUID) {
		return PremiseSource{}, domainError(CodeValidation, "Provider UUID 无效", "provider_uuid 必须为空或 UUIDv7。", nil)
	}
	parameters, err := encodeJSON(input.Parameters, "{}")
	if err != nil {
		return PremiseSource{}, err
	}
	uuid, err := newUUIDv7()
	if err != nil {
		return PremiseSource{}, err
	}
	var created premiseSourceRecord
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		profile, err := service.ensurePremiseProfile(ctx, tx)
		if err != nil {
			return err
		}
		now := service.now().UTC()
		created = premiseSourceRecord{UUID: uuid, ProjectID: p.ID, ActorID: actor.ID, SourceType: sourceType, SourceText: text, StyleSnapshot: style, ProviderUUID: providerUUID, Model: strings.TrimSpace(input.Model), ParametersJSON: parameters, CreatedAt: now}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		return tx.Model(&premiseProfileRecord{}).Where("id = ?", profile.ID).Updates(map[string]any{"current_source_id": created.ID, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error
	})
	if err != nil {
		return PremiseSource{}, err
	}
	dto := sourceDTO(created)
	service.emit("premise:source_created", map[string]any{"source_uuid": dto.UUID})
	return dto, nil
}

func sourceDTO(record premiseSourceRecord) PremiseSource {
	return PremiseSource{UUID: record.UUID, SourceType: record.SourceType, SourceText: record.SourceText, StyleSnapshot: record.StyleSnapshot, ProviderUUID: record.ProviderUUID, Model: record.Model, Parameters: json.RawMessage(record.ParametersJSON), IgnoredAt: record.IgnoredAt, Revision: record.Revision, CreatedAt: record.CreatedAt}
}

func (service *Service) ListPremiseSources(ctx context.Context) ([]PremiseSource, error) {
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return nil, err
	}
	var rows []premiseSourceRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = ?", p.ID).Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]PremiseSource, 0, len(rows))
	for _, row := range rows {
		result = append(result, sourceDTO(row))
	}
	return result, nil
}

func (service *Service) ListPremiseSourcesPage(ctx context.Context, page, perPage int) ([]PremiseSource, Pagination, error) {
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return nil, Pagination{}, err
	}
	page, perPage = normalizePage(page, perPage, 20)
	query := service.store.DB().WithContext(ctx).Model(&premiseSourceRecord{}).Where("project_id = ?", p.ID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Pagination{}, err
	}
	var rows []premiseSourceRecord
	if err := query.Order("created_at DESC, id DESC").Limit(perPage).Offset((page - 1) * perPage).Find(&rows).Error; err != nil {
		return nil, Pagination{}, err
	}
	items := make([]PremiseSource, 0, len(rows))
	for _, row := range rows {
		items = append(items, sourceDTO(row))
	}
	return items, pagePagination(page, perPage, total), nil
}

func (service *Service) SetPremiseSourceIgnored(ctx context.Context, sourceUUID string, ignored bool, expectedRevision int64) (PremiseSource, error) {
	if !isUUIDv7(sourceUUID) || expectedRevision < 0 {
		return PremiseSource{}, domainError(CodeValidation, "Premise 批次参数无效", "source_uuid 必须是 UUIDv7，expected_revision 必须为非负整数。", nil)
	}
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return PremiseSource{}, err
	}
	var row premiseSourceRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = ? AND uuid = ?", p.ID, sourceUUID).First(&row).Error; err != nil {
		return PremiseSource{}, notFound(err, "Premise 批次不存在")
	}
	if (ignored && row.IgnoredAt != nil) || (!ignored && row.IgnoredAt == nil) {
		return sourceDTO(row), nil
	}
	if row.Revision != expectedRevision {
		return PremiseSource{}, domainError(CodeConflict, "Premise 批次已被修改", "刷新后基于最新 revision 重试。", nil)
	}
	if ignored {
		var settingCount int64
		if err := service.store.DB().WithContext(ctx).Model(&settingImageRecord{}).Where("project_id = ? AND source_id = ?", p.ID, row.ID).Count(&settingCount).Error; err != nil {
			return PremiseSource{}, err
		}
		if settingCount == 0 {
			return PremiseSource{}, domainError(CodeStateConflict, "批次还没有设定总览图", "生成设定总览图后才能忽略该批次。", nil)
		}
		var activeTasks int64
		if err := service.store.DB().WithContext(ctx).Table("production_task_runs").Where(
			"project_id = ? AND status IN ('queued','running') AND ((kind = 'premise_setting_generation' AND resource_uuid = ?) OR (kind = 'premise_asset_breakdown' AND resource_uuid IN (SELECT uuid FROM premise_setting_images WHERE project_id = ? AND source_id = ?)))",
			p.ID, sourceUUID, p.ID, row.ID,
		).Count(&activeTasks).Error; err != nil {
			return PremiseSource{}, err
		}
		if activeTasks > 0 {
			return PremiseSource{}, domainError(CodeStateConflict, "批次仍在处理中", "等待当前生成或拆分任务结束后再忽略。", nil)
		}
	}
	now := service.now().UTC()
	var ignoredAt any
	if ignored {
		ignoredAt = now
	}
	result := service.store.DB().WithContext(ctx).Model(&premiseSourceRecord{}).Where("id = ? AND revision = ?", row.ID, expectedRevision).Updates(map[string]any{
		"ignored_at": ignoredAt,
		"revision":   gorm.Expr("revision + 1"),
	})
	if result.Error != nil {
		return PremiseSource{}, result.Error
	}
	if result.RowsAffected != 1 {
		return PremiseSource{}, domainError(CodeConflict, "Premise 批次已被修改", "刷新后重试。", nil)
	}
	var refreshed premiseSourceRecord
	if err := service.store.DB().WithContext(ctx).First(&refreshed, row.ID).Error; err != nil {
		return PremiseSource{}, err
	}
	dto := sourceDTO(refreshed)
	service.emit("premise:source_changed", map[string]any{"source_uuid": dto.UUID, "ignored": dto.IgnoredAt != nil, "revision": dto.Revision})
	return dto, nil
}

func (service *Service) ImportSettingImage(ctx context.Context, uploadUUID, sourceUUID, prompt string) (SettingImage, error) {
	settingUUID, err := newUUIDv7()
	if err != nil {
		return SettingImage{}, err
	}
	var settingID int64
	_, err = service.files.FinalizeUploadWithBind(ctx, uploadUUID, "premise_setting_image", func(tx *gorm.DB, fileID int64) error {
		p, _, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		profile, err := service.ensurePremiseProfile(ctx, tx)
		if err != nil {
			return err
		}
		var sourceID *int64
		if sourceUUID != "" {
			var source premiseSourceRecord
			if err := tx.Where("project_id = ? AND uuid = ?", p.ID, sourceUUID).First(&source).Error; err != nil {
				return notFound(err, "Premise source 不存在")
			}
			sourceID = &source.ID
		}
		now := service.now().UTC()
		record := settingImageRecord{UUID: settingUUID, ProjectID: p.ID, SourceID: sourceID, FileID: fileID, Origin: "manual", Prompt: strings.TrimSpace(prompt), CreatedAt: now}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		settingID = record.ID
		return tx.Model(&premiseProfileRecord{}).Where("id = ?", profile.ID).Updates(map[string]any{"current_setting_image_id": record.ID, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error
	})
	if err != nil {
		return SettingImage{}, err
	}
	if settingID == 0 { // Idempotent retry after the upload was already consumed.
		asset, getErr := service.files.GetUpload(ctx, uploadUUID)
		if getErr != nil || asset.AssetUUID == "" {
			return SettingImage{}, domainError(CodeStateConflict, "上传已被消费", "该上传没有可绑定的设置图。", getErr)
		}
		if err := service.store.DB().WithContext(ctx).Table("premise_setting_images AS s").Select("s.id").Joins("JOIN files f ON f.id=s.file_id").Where("f.uuid = ?", asset.AssetUUID).Scan(&settingID).Error; err != nil || settingID == 0 {
			return SettingImage{}, domainError(CodeStateConflict, "上传已被其他资源消费", "请重新上传图片。", err)
		}
	}
	dto, err := service.settingImageDTO(ctx, settingID)
	if err == nil {
		service.emit("premise:setting_image_changed", map[string]any{"setting_image_uuid": dto.UUID})
	}
	return dto, err
}

func (service *Service) CommitGeneratedSettingImage(ctx context.Context, taskUUID, sourceUUID, prompt string, reader filesReader) (SettingImage, error) {
	settingUUID, err := newUUIDv7()
	if err != nil {
		return SettingImage{}, err
	}
	var settingID int64
	_, err = service.files.CommitReader(ctx, files.CommitInput{Purpose: "premise_setting_image", OriginalFilename: "generated-setting.png", DisplayName: "Generated setting", SourceType: "generated", Metadata: map[string]any{"generation": "premise"}, Reader: reader, Bind: func(tx *gorm.DB, fileID int64) error {
		if err := ensureProductionTaskRunning(tx, taskUUID); err != nil {
			return err
		}
		p, _, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		profile, err := service.ensurePremiseProfile(ctx, tx)
		if err != nil {
			return err
		}
		var source premiseSourceRecord
		if err := tx.Where("project_id = ? AND uuid = ?", p.ID, sourceUUID).First(&source).Error; err != nil {
			return notFound(err, "Premise source 不存在")
		}
		now := service.now().UTC()
		record := settingImageRecord{UUID: settingUUID, ProjectID: p.ID, SourceID: &source.ID, FileID: fileID, Origin: "generated", Prompt: prompt, CreatedAt: now}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		settingID = record.ID
		if err := tx.Model(&premiseProfileRecord{}).Where("id = ?", profile.ID).Updates(map[string]any{"current_setting_image_id": record.ID, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE premise_generation_steps SET status='completed',setting_image_id=?,output_json=json_object('setting_image_uuid',?),completed_at=? WHERE task_uuid=?`, record.ID, record.UUID, now, taskUUID).Error
	}})
	if err != nil {
		return SettingImage{}, err
	}
	return service.settingImageDTO(ctx, settingID)
}

type filesReader interface{ Read([]byte) (int, error) }

func (service *Service) ListSettingImages(ctx context.Context) ([]SettingImage, error) {
	return service.listSettingImages(ctx, nil)
}

func (service *Service) ListSettingImagesForSources(ctx context.Context, sourceUUIDs []string) ([]SettingImage, error) {
	if len(sourceUUIDs) == 0 {
		return []SettingImage{}, nil
	}
	if len(sourceUUIDs) > 200 {
		return nil, domainError(CodeValidation, "Premise source 筛选过多", "单次最多读取 200 个批次的设定图。", nil)
	}
	seen := make(map[string]struct{}, len(sourceUUIDs))
	filtered := make([]string, 0, len(sourceUUIDs))
	for _, value := range sourceUUIDs {
		value = strings.TrimSpace(value)
		if !isUUIDv7(value) {
			return nil, domainError(CodeValidation, "Premise source UUID 无效", "source_uuid 必须是 UUIDv7。", nil)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		filtered = append(filtered, value)
	}
	return service.listSettingImages(ctx, filtered)
}

func (service *Service) listSettingImages(ctx context.Context, sourceUUIDs []string) ([]SettingImage, error) {
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return nil, err
	}
	var rows []settingImageRecord
	query := service.store.DB().WithContext(ctx).Where("project_id = ?", p.ID)
	if sourceUUIDs != nil {
		query = query.Where("source_id IN (SELECT id FROM premise_sources WHERE project_id = ? AND uuid IN ?)", p.ID, sourceUUIDs)
	}
	if err := query.Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]SettingImage, 0, len(rows))
	for _, row := range rows {
		dto, err := service.settingImageDTO(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, dto)
	}
	return items, nil
}

func normalizePage(page, perPage, fallback int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = fallback
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

func pagePagination(page, perPage int, total int64) Pagination {
	lastPage := int((total + int64(perPage) - 1) / int64(perPage))
	if lastPage < 1 {
		lastPage = 1
	}
	return Pagination{PerPage: perPage, CurrentPage: page, LastPage: lastPage, Total: total}
}

func (service *Service) SelectSettingImage(ctx context.Context, settingUUID string) (PremiseProfile, error) {
	if !isUUIDv7(settingUUID) {
		return PremiseProfile{}, domainError(CodeValidation, "设置图 UUID 无效", "setting_image_uuid 必须是 UUIDv7。", nil)
	}
	profile, err := service.ensurePremiseProfile(ctx, service.store.DB())
	if err != nil {
		return PremiseProfile{}, err
	}
	var image settingImageRecord
	if err := service.store.DB().WithContext(ctx).Where("project_id = ? AND uuid = ?", profile.ProjectID, settingUUID).First(&image).Error; err != nil {
		return PremiseProfile{}, notFound(err, "设置图不存在")
	}
	now := service.now().UTC()
	if err := service.store.DB().WithContext(ctx).Model(&premiseProfileRecord{}).Where("id = ?", profile.ID).Updates(map[string]any{"current_setting_image_id": image.ID, "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
		return PremiseProfile{}, err
	}
	result, err := service.GetPremise(ctx)
	if err == nil {
		service.emit("premise:setting_image_changed", map[string]any{"setting_image_uuid": settingUUID})
	}
	return result, err
}

func (service *Service) settingImageDTO(ctx context.Context, id int64) (SettingImage, error) {
	var row settingImageRecord
	if err := service.store.DB().WithContext(ctx).First(&row, id).Error; err != nil {
		return SettingImage{}, err
	}
	var sourceUUID string
	if row.SourceID != nil {
		if err := service.store.DB().WithContext(ctx).Model(&premiseSourceRecord{}).Select("uuid").Where("id = ?", *row.SourceID).Scan(&sourceUUID).Error; err != nil {
			return SettingImage{}, err
		}
	}
	var fileUUID string
	if err := service.store.DB().WithContext(ctx).Table("files").Select("uuid").Where("id = ?", row.FileID).Scan(&fileUUID).Error; err != nil {
		return SettingImage{}, err
	}
	if fileUUID == "" {
		return SettingImage{}, domainError(CodeNotFound, "设置图文件不存在", "设置图引用的 Asset Store 文件不存在。", nil)
	}
	asset, err := service.files.GetAsset(ctx, fileUUID, false)
	if err != nil {
		return SettingImage{}, err
	}
	return SettingImage{UUID: row.UUID, SourceUUID: sourceUUID, Origin: row.Origin, Prompt: row.Prompt, Asset: asset, CreatedAt: row.CreatedAt}, nil
}

func (service *Service) ImportPremiseAsset(ctx context.Context, input CreateAssetInput) (PremiseAsset, error) {
	assetType := strings.TrimSpace(input.AssetType)
	title := strings.TrimSpace(input.Title)
	if !validAssetType(assetType) || title == "" || len([]rune(title)) > 160 || len([]rune(strings.TrimSpace(input.Summary))) > 12000 {
		return PremiseAsset{}, domainError(CodeValidation, "设定资产无效", "asset_type 与 title 必须有效。", nil)
	}
	tags, err := normalizeTags(input.Tags)
	if err != nil {
		return PremiseAsset{}, err
	}
	position, err := encodeJSON(input.Position, "{}")
	if err != nil {
		return PremiseAsset{}, err
	}
	crop, err := encodeJSON(input.Crop, "{}")
	if err != nil {
		return PremiseAsset{}, err
	}
	assetUUID, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	variantUUID, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	var domainID int64
	_, err = service.files.FinalizeUploadWithBind(ctx, input.UploadUUID, "premise_asset", func(tx *gorm.DB, fileID int64) error {
		p, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		now := service.now().UTC()
		record := premiseAssetRecord{UUID: assetUUID, ProjectID: p.ID, ActorID: actor.ID, AssetType: assetType, Title: title, Summary: strings.TrimSpace(input.Summary), PositionJSON: position, CropJSON: crop, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&record).Error; err != nil {
			return conflictErr(err)
		}
		domainID = record.ID
		variant := assetVariantRecord{UUID: variantUUID, PremiseAssetID: record.ID, FileID: fileID, VersionNo: 1, SourceType: "manual", CropJSON: crop, CreatedAt: now}
		if err := tx.Create(&variant).Error; err != nil {
			return err
		}
		if err := tx.Model(&premiseAssetRecord{}).Where("id = ?", record.ID).Update("current_variant_id", variant.ID).Error; err != nil {
			return err
		}
		if err := replaceTags(tx, record.ID, tags, now); err != nil {
			return err
		}
		return appendPremiseAssetEvent(tx, record.ID, "asset_created", map[string]any{"asset_uuid": record.UUID, "variant_uuid": variant.UUID}, now)
	})
	if err != nil {
		return PremiseAsset{}, err
	}
	if domainID == 0 { // Idempotent retry after the upload was already consumed.
		upload, getErr := service.files.GetUpload(ctx, input.UploadUUID)
		if getErr != nil || upload.AssetUUID == "" {
			return PremiseAsset{}, domainError(CodeStateConflict, "上传已被消费", "该上传没有可绑定的 Premise 资产。", getErr)
		}
		if err := service.store.DB().WithContext(ctx).Table("premise_assets AS a").Select("a.id").Joins("JOIN premise_asset_variants v ON v.premise_asset_id=a.id").Joins("JOIN files f ON f.id=v.file_id").Where("f.uuid = ?", upload.AssetUUID).Order("v.version_no ASC").Limit(1).Scan(&domainID).Error; err != nil || domainID == 0 {
			return PremiseAsset{}, domainError(CodeStateConflict, "上传已被其他资源消费", "请重新上传图片。", err)
		}
	}
	dto, err := service.premiseAssetDTO(ctx, domainID)
	if err == nil {
		service.emit("premise:asset_changed", map[string]any{"premise_asset_uuid": dto.UUID})
	}
	return dto, err
}

type projectChatImageFile struct {
	ID                 int64
	Kind               string
	Purpose            string
	ChatThreadUUID     string
	PremiseAssetUUID   string
	ReferenceUUIDsJSON string
}

func loadProjectChatImageFile(tx *gorm.DB, projectID int64, fileUUID string) (projectChatImageFile, error) {
	var file projectChatImageFile
	err := tx.Table("files").
		Select("id,kind,purpose,COALESCE(json_extract(metadata_json,'$.chat_thread_uuid'),'') AS chat_thread_uuid,COALESCE(json_extract(metadata_json,'$.premise_asset_uuid'),'') AS premise_asset_uuid,COALESCE(json_extract(metadata_json,'$.reference_uuids'),'[]') AS reference_uuids_json").
		Where("project_id=? AND uuid=? AND deleted_at IS NULL", projectID, fileUUID).
		Take(&file).Error
	return file, err
}

func projectChatReferenceUUIDs(raw string) []string {
	result := []string{}
	if json.Unmarshal([]byte(raw), &result) != nil {
		return []string{}
	}
	return result
}

// CreatePremiseAssetFromFile binds an already durable chat-generated image to
// a new premise asset. The tool execution UUID makes recovery after a process
// interruption idempotent without routing chat work through production tasks.
func (service *Service) CreatePremiseAssetFromFile(ctx context.Context, input CreateAssetInput) (PremiseAsset, error) {
	assetType := strings.TrimSpace(input.AssetType)
	title := strings.TrimSpace(input.Title)
	summary := strings.TrimSpace(input.Summary)
	fileUUID := strings.TrimSpace(input.FileUUID)
	executionUUID := strings.TrimSpace(input.ToolExecutionUUID)
	chatThreadUUID := strings.TrimSpace(input.ChatThreadUUID)
	sourceAssetUUID := strings.TrimSpace(input.SourcePremiseAssetUUID)
	if !validAssetType(assetType) || title == "" || len([]rune(title)) > 160 || len([]rune(summary)) > 12000 || !isUUIDv7(fileUUID) || !isUUIDv7(executionUUID) || !isUUIDv7(chatThreadUUID) || (sourceAssetUUID != "" && !isUUIDv7(sourceAssetUUID)) {
		return PremiseAsset{}, domainError(CodeValidation, "AI 设定资产无效", "asset_type、title、file_uuid 或 tool_execution_uuid 不符合限制。", nil)
	}
	if existing, found, err := service.premiseAssetForToolExecution(ctx, executionUUID); err != nil || found {
		return existing, err
	}
	tags, err := normalizeTags(input.Tags)
	if err != nil {
		return PremiseAsset{}, err
	}
	assetUUID, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	variantUUID, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	var domainID int64
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		if sourceAssetUUID != "" {
			var source premiseAssetRecord
			if err := tx.Select("id").Where("project_id=? AND uuid=? AND deleted_at IS NULL", p.ID, sourceAssetUUID).Take(&source).Error; err != nil {
				return notFound(err, "来源设定资产不存在")
			}
		}
		file, err := loadProjectChatImageFile(tx, p.ID, fileUUID)
		if err != nil {
			return notFound(err, "生成图片文件不存在")
		}
		if file.ChatThreadUUID != chatThreadUUID {
			return domainError(CodeValidation, "生成图片来源会话无效", "file_uuid 必须是当前 Thread 的 image_gen 新输出；已有项目图片只能先作为当前 Turn Reference 使用。", nil)
		}
		if sourceAssetUUID != "" && file.Purpose != "project_chat_image_generation" && file.PremiseAssetUUID != sourceAssetUUID {
			return domainError(CodeValidation, "旧图片输出来源不匹配", "兼容恢复的派生写回要求图片来源与 source_premise_asset_uuid 匹配。", nil)
		}
		allowed := file.Purpose == "project_chat_image_generation" || (sourceAssetUUID == "" && file.Purpose == "project_chat_asset_image_generation") || (sourceAssetUUID != "" && file.Purpose == "project_chat_asset_reference_image")
		if file.Kind != "image" || !allowed {
			return domainError(CodeValidation, "生成图片用途无效", "新设定项只能使用当前会话 image_gen 生成的资产图片。", nil)
		}
		var used int64
		if err := tx.Table("premise_asset_variants").Where("file_id=?", file.ID).Count(&used).Error; err != nil {
			return err
		}
		if used > 0 {
			return domainError(CodeStateConflict, "生成图片已被使用", "请重新调用 image_gen 生成新的文件。", nil)
		}
		now := service.now().UTC()
		record := premiseAssetRecord{UUID: assetUUID, ProjectID: p.ID, ActorID: actor.ID, AssetType: assetType, Title: title, Summary: summary, PositionJSON: "{}", CropJSON: "{}", CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&record).Error; err != nil {
			return conflictErr(err)
		}
		domainID = record.ID
		variant := assetVariantRecord{UUID: variantUUID, PremiseAssetID: record.ID, FileID: file.ID, VersionNo: 1, SourceType: "replacement", CropJSON: "{}", CreatedAt: now}
		if err := tx.Create(&variant).Error; err != nil {
			return err
		}
		if err := tx.Model(&record).Update("current_variant_id", variant.ID).Error; err != nil {
			return err
		}
		if err := replaceTags(tx, record.ID, tags, now); err != nil {
			return err
		}
		payload := map[string]any{"asset_uuid": record.UUID, "variant_uuid": variant.UUID, "file_uuid": fileUUID, "tool_execution_uuid": executionUUID, "source_reference_uuids": projectChatReferenceUUIDs(file.ReferenceUUIDsJSON)}
		return appendPremiseAssetEvent(tx, record.ID, "asset_created_from_chat_image", payload, now)
	})
	if err != nil {
		return PremiseAsset{}, err
	}
	result, err := service.premiseAssetDTO(ctx, domainID)
	if err == nil {
		service.emit("premise:asset_changed", map[string]any{"premise_asset_uuid": result.UUID, "tool_execution_uuid": executionUUID})
	}
	return result, err
}

func (service *Service) premiseAssetForToolExecution(ctx context.Context, executionUUID string) (PremiseAsset, bool, error) {
	var assetID int64
	err := service.store.DB().WithContext(ctx).Table("premise_asset_events").Select("premise_asset_id").Where("json_extract(payload, '$.tool_execution_uuid') = ?", executionUUID).Order("id DESC").Limit(1).Scan(&assetID).Error
	if err != nil || assetID == 0 {
		return PremiseAsset{}, false, err
	}
	asset, err := service.premiseAssetDTO(ctx, assetID)
	return asset, err == nil, err
}

// CommitGeneratedPremiseAsset commits a breakdown crop through Asset Store and
// binds it to a domain asset as an immutable image candidate.
// An active asset with the same title is the same logical setting item: append
// and select a new immutable candidate instead of failing the active-title
// uniqueness constraint. A replay of the same task/title returns the candidate
// it already committed so durable task retries do not create extra versions.
func (service *Service) CommitGeneratedPremiseAsset(ctx context.Context, taskUUID, settingUUID string, input CreateAssetInput, reader filesReader) (PremiseAsset, error) {
	assetType := strings.TrimSpace(input.AssetType)
	title := strings.TrimSpace(input.Title)
	if !validAssetType(assetType) || title == "" || len([]rune(title)) > 160 || len([]rune(strings.TrimSpace(input.Summary))) > 12000 {
		return PremiseAsset{}, domainError(CodeValidation, "拆分资产无效", "asset_type 与 title 必须有效。", nil)
	}
	if existing, found, err := service.premiseAssetForBreakdownTask(ctx, taskUUID, title); err != nil || found {
		return existing, err
	}
	tags, err := normalizeTags(input.Tags)
	if err != nil {
		return PremiseAsset{}, err
	}
	position, err := encodeJSON(input.Position, "{}")
	if err != nil {
		return PremiseAsset{}, err
	}
	crop, err := encodeJSON(input.Crop, "{}")
	if err != nil {
		return PremiseAsset{}, err
	}
	assetUUID, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	variantUUID, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	var domainID int64
	_, err = service.files.CommitReader(ctx, files.CommitInput{Purpose: "premise_asset", OriginalFilename: "breakdown-crop.png", DisplayName: title, SourceType: "derived", Metadata: map[string]any{"tags": tags, "crop_profile": crop}, Reader: reader, Bind: func(tx *gorm.DB, fileID int64) error {
		if err := ensureProductionTaskRunning(tx, taskUUID); err != nil {
			return err
		}
		p, actor, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		var setting settingImageRecord
		if err := tx.Where("project_id = ? AND uuid = ?", p.ID, settingUUID).First(&setting).Error; err != nil {
			return notFound(err, "设置图不存在")
		}
		now := service.now().UTC()
		var record premiseAssetRecord
		existingRecord := true
		findErr := tx.Where("project_id = ? AND lower(title) = lower(?) AND deleted_at IS NULL", p.ID, title).Take(&record).Error
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			existingRecord = false
			record = premiseAssetRecord{UUID: assetUUID, ProjectID: p.ID, ActorID: actor.ID, AssetType: assetType, Title: title, Summary: strings.TrimSpace(input.Summary), PositionJSON: position, CropJSON: crop, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&record).Error; err != nil {
				return conflictErr(err)
			}
		} else if findErr != nil {
			return findErr
		}
		domainID = record.ID
		var version int
		if err := tx.Model(&assetVariantRecord{}).Where("premise_asset_id = ?", record.ID).Select("COALESCE(MAX(version_no),0)+1").Scan(&version).Error; err != nil {
			return err
		}
		variant := assetVariantRecord{UUID: variantUUID, PremiseAssetID: record.ID, FileID: fileID, SourceSettingImageID: &setting.ID, VersionNo: version, SourceType: "breakdown", CropJSON: crop, CreatedAt: now}
		if err := tx.Create(&variant).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"asset_type": assetType, "title": title, "summary": strings.TrimSpace(input.Summary),
			"position_json": position, "crop_json": crop, "current_variant_id": variant.ID,
			"revision": gorm.Expr("revision + 1"), "updated_at": now,
		}
		if !existingRecord {
			delete(updates, "revision")
		}
		if err := tx.Model(&premiseAssetRecord{}).Where("id = ?", record.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := replaceTags(tx, record.ID, tags, now); err != nil {
			return err
		}
		eventType := "asset_created_from_breakdown"
		if existingRecord {
			eventType = "asset_candidate_added_from_breakdown"
		}
		return appendPremiseAssetEvent(tx, record.ID, eventType, map[string]any{
			"asset_uuid": record.UUID, "variant_uuid": variant.UUID, "setting_image_uuid": settingUUID,
			"task_uuid": taskUUID, "title_key": strings.ToLower(title),
		}, now)
	}})
	if err != nil {
		return PremiseAsset{}, err
	}
	return service.premiseAssetDTO(ctx, domainID)
}

func (service *Service) premiseAssetForBreakdownTask(ctx context.Context, taskUUID, title string) (PremiseAsset, bool, error) {
	if !isUUIDv7(taskUUID) {
		return PremiseAsset{}, false, domainError(CodeValidation, "拆分任务 UUID 无效", "task_uuid 必须是 UUIDv7。", nil)
	}
	var assetID int64
	err := service.store.DB().WithContext(ctx).Table("premise_asset_events").
		Select("premise_asset_id").
		Where("event_type IN ('asset_created_from_breakdown','asset_candidate_added_from_breakdown') AND json_extract(payload, '$.task_uuid') = ? AND json_extract(payload, '$.title_key') = ?", taskUUID, strings.ToLower(strings.TrimSpace(title))).
		Order("id DESC").Limit(1).Scan(&assetID).Error
	if err != nil || assetID == 0 {
		return PremiseAsset{}, false, err
	}
	asset, err := service.premiseAssetDTO(ctx, assetID)
	return asset, err == nil, err
}

func (service *Service) PremiseAssetForGenerationTask(ctx context.Context, taskUUID string) (PremiseAsset, bool, error) {
	if !isUUIDv7(taskUUID) {
		return PremiseAsset{}, false, domainError(CodeValidation, "生成任务 UUID 无效", "task_uuid 必须是 UUIDv7。", nil)
	}
	var assetID int64
	err := service.store.DB().WithContext(ctx).Table("premise_asset_events").
		Select("premise_asset_id").
		Where("event_type IN ('asset_created_from_ai','image_generated_from_ai') AND json_extract(payload, '$.task_uuid') = ?", taskUUID).
		Order("id DESC").Limit(1).Scan(&assetID).Error
	if err != nil {
		return PremiseAsset{}, false, err
	}
	if assetID == 0 {
		return PremiseAsset{}, false, nil
	}
	asset, err := service.premiseAssetDTO(ctx, assetID)
	return asset, err == nil, err
}

func (service *Service) CommitAIGeneratedPremiseAsset(ctx context.Context, taskUUID string, input CreateAssetInput, reader filesReader) (PremiseAsset, error) {
	if existing, found, err := service.PremiseAssetForGenerationTask(ctx, taskUUID); err != nil || found {
		return existing, err
	}
	assetType := strings.TrimSpace(input.AssetType)
	title := strings.TrimSpace(input.Title)
	summary := strings.TrimSpace(input.Summary)
	if !validAssetType(assetType) || title == "" || len([]rune(title)) > 160 || len([]rune(summary)) > 12000 {
		return PremiseAsset{}, domainError(CodeValidation, "AI 设定资产无效", "asset_type、title 或 summary 不符合限制。", nil)
	}
	tags, err := normalizeTags(input.Tags)
	if err != nil {
		return PremiseAsset{}, err
	}
	assetUUID, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	variantUUID, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	var domainID int64
	_, err = service.files.CommitReader(ctx, files.CommitInput{
		Purpose:          "premise_asset",
		OriginalFilename: "generated-premise-asset.png",
		DisplayName:      title,
		SourceType:       "generated",
		Metadata:         map[string]any{"generation": "premise_asset", "task_uuid": taskUUID},
		Reader:           reader,
		Bind: func(tx *gorm.DB, fileID int64) error {
			if err := ensureProductionTaskRunning(tx, taskUUID); err != nil {
				return err
			}
			p, actor, err := service.projectActor(ctx, tx)
			if err != nil {
				return err
			}
			now := service.now().UTC()
			record := premiseAssetRecord{UUID: assetUUID, ProjectID: p.ID, ActorID: actor.ID, AssetType: assetType, Title: title, Summary: summary, PositionJSON: "{}", CropJSON: "{}", CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&record).Error; err != nil {
				return conflictErr(err)
			}
			domainID = record.ID
			variant := assetVariantRecord{UUID: variantUUID, PremiseAssetID: record.ID, FileID: fileID, VersionNo: 1, SourceType: "replacement", CropJSON: "{}", CreatedAt: now}
			if err := tx.Create(&variant).Error; err != nil {
				return err
			}
			if err := tx.Model(&premiseAssetRecord{}).Where("id = ?", record.ID).Update("current_variant_id", variant.ID).Error; err != nil {
				return err
			}
			if err := replaceTags(tx, record.ID, tags, now); err != nil {
				return err
			}
			return appendPremiseAssetEvent(tx, record.ID, "asset_created_from_ai", map[string]any{"asset_uuid": record.UUID, "variant_uuid": variant.UUID, "task_uuid": taskUUID}, now)
		},
	})
	if err != nil {
		return PremiseAsset{}, err
	}
	dto, err := service.premiseAssetDTO(ctx, domainID)
	if err == nil {
		service.emit("premise:asset_changed", map[string]any{"premise_asset_uuid": dto.UUID, "task_uuid": taskUUID})
	}
	return dto, err
}

func (service *Service) CommitAIGeneratedPremiseAssetVariant(ctx context.Context, taskUUID, assetUUID string, expectedRevision int64, reader filesReader) (PremiseAsset, error) {
	if existing, found, err := service.PremiseAssetForGenerationTask(ctx, taskUUID); err != nil || found {
		return existing, err
	}
	if !isUUIDv7(assetUUID) || expectedRevision < 0 {
		return PremiseAsset{}, domainError(CodeValidation, "设定资产生成参数无效", "asset_uuid 必须是 UUIDv7。", nil)
	}
	variantUUID, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	var domainID int64
	_, err = service.files.CommitReader(ctx, files.CommitInput{
		Purpose:          "premise_asset",
		OriginalFilename: "generated-premise-variant.png",
		DisplayName:      "Generated premise variant",
		SourceType:       "generated",
		Metadata:         map[string]any{"generation": "premise_asset_variant", "task_uuid": taskUUID, "premise_asset_uuid": assetUUID},
		Reader:           reader,
		Bind: func(tx *gorm.DB, fileID int64) error {
			if err := ensureProductionTaskRunning(tx, taskUUID); err != nil {
				return err
			}
			var asset premiseAssetRecord
			if err := tx.Where("uuid = ? AND project_id = (SELECT id FROM projects WHERE uuid = ?) AND deleted_at IS NULL", assetUUID, service.store.ProjectUUID()).First(&asset).Error; err != nil {
				return notFound(err, "设定资产不存在")
			}
			if asset.Revision != expectedRevision {
				return domainError(CodeConflict, "设定资产已被修改", "生成期间资产版本发生变化，请重新发起。", nil)
			}
			var version int
			if err := tx.Model(&assetVariantRecord{}).Where("premise_asset_id = ?", asset.ID).Select("COALESCE(MAX(version_no), 0)").Scan(&version).Error; err != nil {
				return err
			}
			now := service.now().UTC()
			variant := assetVariantRecord{UUID: variantUUID, PremiseAssetID: asset.ID, FileID: fileID, VersionNo: version + 1, SourceType: "replacement", CropJSON: "{}", CreatedAt: now}
			if err := tx.Create(&variant).Error; err != nil {
				return err
			}
			result := tx.Model(&premiseAssetRecord{}).Where("id = ? AND revision = ?", asset.ID, expectedRevision).Updates(map[string]any{"current_variant_id": variant.ID, "revision": gorm.Expr("revision + 1"), "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return domainError(CodeConflict, "设定资产已被修改", "生成期间资产版本发生变化，请重新发起。", nil)
			}
			domainID = asset.ID
			return appendPremiseAssetEvent(tx, asset.ID, "image_generated_from_ai", map[string]any{"asset_uuid": asset.UUID, "variant_uuid": variant.UUID, "task_uuid": taskUUID}, now)
		},
	})
	if err != nil {
		return PremiseAsset{}, err
	}
	dto, err := service.premiseAssetDTO(ctx, domainID)
	if err == nil {
		service.emit("premise:asset_changed", map[string]any{"premise_asset_uuid": dto.UUID, "task_uuid": taskUUID})
	}
	return dto, err
}

func (service *Service) ListPremiseAssets(ctx context.Context, tag, state string) ([]PremiseAsset, error) {
	p, _, err := service.projectActor(ctx, service.store.DB())
	if err != nil {
		return nil, err
	}
	state = strings.ToLower(strings.TrimSpace(state))
	if state == "" {
		state = "active"
	}
	if state != "active" && state != "trashed" {
		return nil, domainError(CodeValidation, "资产状态筛选无效", "state 只支持 active/trashed。", nil)
	}
	query := service.store.DB().WithContext(ctx).Model(&premiseAssetRecord{}).Where("project_id = ?", p.ID)
	if state == "trashed" {
		query = query.Where("deleted_at IS NOT NULL")
	} else {
		query = query.Where("deleted_at IS NULL")
	}
	if normalized := strings.ToLower(strings.TrimSpace(tag)); normalized != "" {
		if len([]rune(normalized)) > 64 {
			return nil, domainError(CodeValidation, "标签筛选过长", "tag 最多 64 个字符。", nil)
		}
		query = query.Where("id IN (SELECT premise_asset_id FROM premise_asset_tags WHERE tag = ?)", normalized)
	}
	var rows []premiseAssetRecord
	if err := query.Order("updated_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]PremiseAsset, 0, len(rows))
	for _, row := range rows {
		dto, err := service.premiseAssetDTO(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, dto)
	}
	return items, nil
}

func (service *Service) GetPremiseAsset(ctx context.Context, assetUUID string) (PremiseAsset, error) {
	var row premiseAssetRecord
	err := service.store.DB().WithContext(ctx).Where("uuid = ? AND project_id=(SELECT id FROM projects WHERE uuid=?)", assetUUID, service.store.ProjectUUID()).First(&row).Error
	if err != nil {
		return PremiseAsset{}, notFound(err, "设定资产不存在")
	}
	return service.premiseAssetDTO(ctx, row.ID)
}

func (service *Service) UpdatePremiseAsset(ctx context.Context, assetUUID string, input UpdateAssetInput) (PremiseAsset, error) {
	executionUUID := strings.TrimSpace(input.ToolExecutionUUID)
	if executionUUID != "" {
		if !isUUIDv7(executionUUID) {
			return PremiseAsset{}, domainError(CodeValidation, "工具执行 UUID 无效", "tool_execution_uuid 必须是 UUIDv7。", nil)
		}
		if existing, found, err := service.premiseAssetForToolExecution(ctx, executionUUID); err != nil || found {
			if found && existing.UUID != assetUUID {
				return PremiseAsset{}, domainError(CodeStateConflict, "工具执行目标不一致", "该工具执行已绑定其他设定资产。", nil)
			}
			return existing, err
		}
	}
	current, err := service.GetPremiseAsset(ctx, assetUUID)
	if err != nil {
		return PremiseAsset{}, err
	}
	if current.Revision != input.ExpectedRevision {
		return PremiseAsset{}, domainError(CodeConflict, "设定资产已被修改", "刷新后重试。", nil)
	}
	updates := map[string]any{"revision": gorm.Expr("revision + 1"), "updated_at": service.now().UTC()}
	if input.AssetType != nil {
		value := strings.TrimSpace(*input.AssetType)
		if !validAssetType(value) {
			return PremiseAsset{}, domainError(CodeValidation, "asset_type 无效", "只支持 character/scene/prop/reference。", nil)
		}
		updates["asset_type"] = value
	}
	if input.Title != nil {
		value := strings.TrimSpace(*input.Title)
		if value == "" || len([]rune(value)) > 160 {
			return PremiseAsset{}, domainError(CodeValidation, "title 无效", "标题不能为空且最多 160 个字符。", nil)
		}
		updates["title"] = value
	}
	if input.Summary != nil {
		value := strings.TrimSpace(*input.Summary)
		if len([]rune(value)) > 12000 {
			return PremiseAsset{}, domainError(CodeValidation, "summary 过长", "简介最多 12000 个字符。", nil)
		}
		updates["summary"] = value
	}
	if input.Position != nil {
		value, err := encodeJSON(input.Position, "{}")
		if err != nil {
			return PremiseAsset{}, err
		}
		updates["position_json"] = value
	}
	if input.Crop != nil {
		value, err := encodeJSON(input.Crop, "{}")
		if err != nil {
			return PremiseAsset{}, err
		}
		updates["crop_json"] = value
	}
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row premiseAssetRecord
		if err := tx.Where("uuid = ? AND revision = ?", assetUUID, input.ExpectedRevision).First(&row).Error; err != nil {
			return domainError(CodeConflict, "设定资产已被修改", "刷新后重试。", err)
		}
		if err := tx.Model(&row).Updates(updates).Error; err != nil {
			return conflictErr(err)
		}
		if input.Tags != nil {
			tags, err := normalizeTags(*input.Tags)
			if err != nil {
				return err
			}
			if err := replaceTags(tx, row.ID, tags, service.now().UTC()); err != nil {
				return err
			}
		}
		payload := map[string]any{"asset_uuid": row.UUID}
		if executionUUID != "" {
			payload["tool_execution_uuid"] = executionUUID
		}
		return appendPremiseAssetEvent(tx, row.ID, "metadata_updated", payload, service.now().UTC())
	})
	if err != nil {
		return PremiseAsset{}, err
	}
	result, err := service.GetPremiseAsset(ctx, assetUUID)
	if err == nil {
		service.emit("premise:asset_changed", map[string]any{"premise_asset_uuid": result.UUID})
	}
	return result, err
}

// UpdatePremiseAssetFromFile atomically appends and selects an immutable image
// variant while applying any requested metadata changes at one revision.
func (service *Service) UpdatePremiseAssetFromFile(ctx context.Context, assetUUID string, input UpdateAssetInput) (PremiseAsset, error) {
	fileUUID := strings.TrimSpace(input.FileUUID)
	executionUUID := strings.TrimSpace(input.ToolExecutionUUID)
	chatThreadUUID := strings.TrimSpace(input.ChatThreadUUID)
	if !isUUIDv7(assetUUID) || !isUUIDv7(fileUUID) || !isUUIDv7(executionUUID) || !isUUIDv7(chatThreadUUID) || input.ExpectedRevision < 0 {
		return PremiseAsset{}, domainError(CodeValidation, "设定资产图片写回参数无效", "资源 UUID 必须是 UUIDv7，expected_revision 必须为非负整数。", nil)
	}
	if existing, found, err := service.premiseAssetForToolExecution(ctx, executionUUID); err != nil || found {
		return existing, err
	}
	variantUUID, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	var domainID int64
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var asset premiseAssetRecord
		if err := tx.Where("uuid=? AND project_id=(SELECT id FROM projects WHERE uuid=?) AND deleted_at IS NULL", assetUUID, service.store.ProjectUUID()).First(&asset).Error; err != nil {
			return notFound(err, "设定资产不存在")
		}
		if asset.Revision != input.ExpectedRevision {
			return domainError(CodeConflict, "设定资产已被修改", "刷新后基于最新 revision 重试。", nil)
		}
		file, err := loadProjectChatImageFile(tx, asset.ProjectID, fileUUID)
		if err != nil {
			return notFound(err, "生成图片文件不存在")
		}
		if file.ChatThreadUUID != chatThreadUUID {
			return domainError(CodeValidation, "生成图片来源会话无效", "file_uuid 必须来自当前 Thread 的 image_gen 输出。", nil)
		}
		if file.Purpose != "project_chat_image_generation" && file.PremiseAssetUUID != assetUUID {
			return domainError(CodeValidation, "旧图片输出来源不匹配", "兼容恢复的旧图片输出必须与目标 Premise Asset 匹配。", nil)
		}
		if file.Kind != "image" || (file.Purpose != "project_chat_image_generation" && file.Purpose != "project_chat_asset_reference_image") {
			return domainError(CodeValidation, "生成图片用途无效", "设定项替换只能使用当前 Thread 的 image_gen 生成图片。", nil)
		}
		var used int64
		if err := tx.Model(&assetVariantRecord{}).Where("file_id=?", file.ID).Count(&used).Error; err != nil {
			return err
		}
		if used > 0 {
			return domainError(CodeStateConflict, "生成图片已被使用", "请重新调用 image_gen 生成新的文件。", nil)
		}
		updates := map[string]any{"revision": gorm.Expr("revision + 1"), "updated_at": service.now().UTC()}
		if input.AssetType != nil {
			value := strings.TrimSpace(*input.AssetType)
			if !validAssetType(value) {
				return domainError(CodeValidation, "asset_type 无效", "只支持 character/scene/prop/reference。", nil)
			}
			updates["asset_type"] = value
		}
		if input.Title != nil {
			value := strings.TrimSpace(*input.Title)
			if value == "" || len([]rune(value)) > 160 {
				return domainError(CodeValidation, "title 无效", "标题不能为空且最多 160 个字符。", nil)
			}
			updates["title"] = value
		}
		if input.Summary != nil {
			value := strings.TrimSpace(*input.Summary)
			if len([]rune(value)) > 12000 {
				return domainError(CodeValidation, "summary 过长", "简介最多 12000 个字符。", nil)
			}
			updates["summary"] = value
		}
		cropJSON := asset.CropJSON
		if input.Crop != nil {
			value, err := encodeJSON(input.Crop, "{}")
			if err != nil {
				return err
			}
			cropJSON = value
			updates["crop_json"] = value
		}
		if input.Position != nil {
			value, err := encodeJSON(input.Position, "{}")
			if err != nil {
				return err
			}
			updates["position_json"] = value
		}
		var version int
		if err := tx.Model(&assetVariantRecord{}).Where("premise_asset_id=?", asset.ID).Select("COALESCE(MAX(version_no),0)+1").Scan(&version).Error; err != nil {
			return err
		}
		now := service.now().UTC()
		variant := assetVariantRecord{UUID: variantUUID, PremiseAssetID: asset.ID, FileID: file.ID, VersionNo: version, SourceType: "replacement", CropJSON: cropJSON, CreatedAt: now}
		if err := tx.Create(&variant).Error; err != nil {
			return err
		}
		updates["current_variant_id"] = variant.ID
		result := tx.Model(&premiseAssetRecord{}).Where("id=? AND revision=?", asset.ID, input.ExpectedRevision).Updates(updates)
		if result.Error != nil || result.RowsAffected != 1 {
			return domainError(CodeConflict, "设定资产已被修改", "刷新后基于最新 revision 重试。", result.Error)
		}
		if input.Tags != nil {
			tags, err := normalizeTags(*input.Tags)
			if err != nil {
				return err
			}
			if err := replaceTags(tx, asset.ID, tags, now); err != nil {
				return err
			}
		}
		domainID = asset.ID
		return appendPremiseAssetEvent(tx, asset.ID, "image_replaced_from_chat", map[string]any{"asset_uuid": asset.UUID, "variant_uuid": variant.UUID, "file_uuid": fileUUID, "tool_execution_uuid": executionUUID, "source_reference_uuids": projectChatReferenceUUIDs(file.ReferenceUUIDsJSON)}, now)
	})
	if err != nil {
		return PremiseAsset{}, err
	}
	result, err := service.premiseAssetDTO(ctx, domainID)
	if err == nil {
		service.emit("premise:asset_changed", map[string]any{"premise_asset_uuid": result.UUID, "tool_execution_uuid": executionUUID})
	}
	return result, err
}

func (service *Service) ListAssetVariants(ctx context.Context, assetUUID string) ([]AssetVariant, error) {
	asset, err := service.GetPremiseAsset(ctx, assetUUID)
	if err != nil {
		return nil, err
	}
	var row premiseAssetRecord
	_ = service.store.DB().WithContext(ctx).Where("uuid = ?", asset.UUID).First(&row).Error
	var variants []assetVariantRecord
	if err := service.store.DB().WithContext(ctx).Where("premise_asset_id = ?", row.ID).Order("version_no DESC").Find(&variants).Error; err != nil {
		return nil, err
	}
	result := make([]AssetVariant, 0, len(variants))
	for _, variant := range variants {
		dto, err := service.assetVariantDTO(ctx, variant)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}
	return result, nil
}

func (service *Service) ImportPremiseAssetVariant(ctx context.Context, assetUUID, uploadUUID string, crop any, expectedRevision int64) (PremiseAsset, error) {
	variantUUID, err := newUUIDv7()
	if err != nil {
		return PremiseAsset{}, err
	}
	cropJSON, err := encodeJSON(crop, "{}")
	if err != nil {
		return PremiseAsset{}, err
	}
	_, err = service.files.FinalizeUploadWithBind(ctx, uploadUUID, "premise_asset", func(tx *gorm.DB, fileID int64) error {
		var asset premiseAssetRecord
		if err := tx.Where("uuid = ? AND project_id=(SELECT id FROM projects WHERE uuid=?) AND deleted_at IS NULL", assetUUID, service.store.ProjectUUID()).First(&asset).Error; err != nil {
			return notFound(err, "设定资产不存在")
		}
		if asset.Revision != expectedRevision {
			return domainError(CodeConflict, "设定资产已被修改", "刷新后重试。", nil)
		}
		var version int
		if err := tx.Model(&assetVariantRecord{}).Where("premise_asset_id = ?", asset.ID).Select("COALESCE(MAX(version_no),0)+1").Scan(&version).Error; err != nil {
			return err
		}
		now := service.now().UTC()
		variant := assetVariantRecord{UUID: variantUUID, PremiseAssetID: asset.ID, FileID: fileID, VersionNo: version, SourceType: "replacement", CropJSON: cropJSON, CreatedAt: now}
		if err := tx.Create(&variant).Error; err != nil {
			return err
		}
		result := tx.Model(&asset).Where("revision = ?", expectedRevision).Updates(map[string]any{"current_variant_id": variant.ID, "crop_json": cropJSON, "revision": gorm.Expr("revision + 1"), "updated_at": now})
		if result.Error != nil || result.RowsAffected != 1 {
			return domainError(CodeConflict, "设定资产已被修改", "刷新后重试。", result.Error)
		}
		return appendPremiseAssetEvent(tx, asset.ID, "image_replaced", map[string]any{"asset_uuid": asset.UUID, "variant_uuid": variant.UUID}, now)
	})
	if err != nil {
		return PremiseAsset{}, err
	}
	result, err := service.GetPremiseAsset(ctx, assetUUID)
	if err == nil {
		service.emit("premise:asset_changed", map[string]any{"premise_asset_uuid": result.UUID})
	}
	return result, err
}

func (service *Service) SelectAssetVariant(ctx context.Context, assetUUID, variantUUID string, expectedRevision int64) (PremiseAsset, error) {
	var asset premiseAssetRecord
	if err := service.store.DB().WithContext(ctx).Where("uuid = ? AND project_id=(SELECT id FROM projects WHERE uuid=?)", assetUUID, service.store.ProjectUUID()).First(&asset).Error; err != nil {
		return PremiseAsset{}, notFound(err, "设定资产不存在")
	}
	var variant assetVariantRecord
	if err := service.store.DB().WithContext(ctx).Where("uuid = ? AND premise_asset_id = ?", variantUUID, asset.ID).First(&variant).Error; err != nil {
		return PremiseAsset{}, notFound(err, "资产版本不存在")
	}
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&premiseAssetRecord{}).Where("id = ? AND revision = ?", asset.ID, expectedRevision).Updates(map[string]any{"current_variant_id": variant.ID, "revision": gorm.Expr("revision + 1"), "updated_at": service.now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainError(CodeConflict, "设定资产已被修改", "刷新后重试。", nil)
		}
		return appendPremiseAssetEvent(tx, asset.ID, "variant_selected", map[string]any{"asset_uuid": asset.UUID, "variant_uuid": variant.UUID}, service.now().UTC())
	})
	if err != nil {
		return PremiseAsset{}, err
	}
	result, err := service.GetPremiseAsset(ctx, assetUUID)
	if err == nil {
		service.emit("premise:asset_changed", map[string]any{"premise_asset_uuid": result.UUID})
	}
	return result, err
}

func (service *Service) SetPremiseAssetTrashed(ctx context.Context, assetUUID string, trashed bool, expectedRevision int64) (PremiseAsset, error) {
	return service.setPremiseAssetTrashed(ctx, assetUUID, trashed, expectedRevision, "")
}

// SetPremiseAssetTrashedFromTool records the public tool execution UUID in the
// domain event so recovery after a committed soft delete is idempotent.
func (service *Service) SetPremiseAssetTrashedFromTool(ctx context.Context, assetUUID string, trashed bool, expectedRevision int64, toolExecutionUUID string) (PremiseAsset, error) {
	toolExecutionUUID = strings.TrimSpace(toolExecutionUUID)
	if !isUUIDv7(toolExecutionUUID) {
		return PremiseAsset{}, domainError(CodeValidation, "工具执行 UUID 无效", "tool_execution_uuid 必须是 UUIDv7。", nil)
	}
	if existing, found, err := service.premiseAssetForToolExecution(ctx, toolExecutionUUID); err != nil || found {
		if found && existing.UUID != assetUUID {
			return PremiseAsset{}, domainError(CodeStateConflict, "工具执行目标不一致", "该工具执行已绑定其他设定资产。", nil)
		}
		return existing, err
	}
	return service.setPremiseAssetTrashed(ctx, assetUUID, trashed, expectedRevision, toolExecutionUUID)
}

func (service *Service) setPremiseAssetTrashed(ctx context.Context, assetUUID string, trashed bool, expectedRevision int64, toolExecutionUUID string) (PremiseAsset, error) {
	var deleted any = nil
	event := "asset_restored"
	if trashed {
		deleted = service.now().UTC()
		event = "asset_trashed"
	}
	var id int64
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row premiseAssetRecord
		if err := tx.Where("uuid = ? AND project_id=(SELECT id FROM projects WHERE uuid=?)", assetUUID, service.store.ProjectUUID()).First(&row).Error; err != nil {
			return notFound(err, "设定资产不存在")
		}
		if (trashed && row.DeletedAt != nil) || (!trashed && row.DeletedAt == nil) {
			return domainError(CodeStateConflict, "设定资产状态未变化", "设定资产已经处于请求的状态。", nil)
		}
		id = row.ID
		result := tx.Model(&row).Where("revision = ?", expectedRevision).Updates(map[string]any{"deleted_at": deleted, "revision": gorm.Expr("revision + 1"), "updated_at": service.now().UTC()})
		if result.Error != nil {
			return conflictErr(result.Error)
		}
		if result.RowsAffected != 1 {
			return domainError(CodeConflict, "设定资产已被修改", "刷新后重试。", nil)
		}
		payload := map[string]any{"asset_uuid": row.UUID}
		if toolExecutionUUID != "" {
			payload["tool_execution_uuid"] = toolExecutionUUID
		}
		return appendPremiseAssetEvent(tx, row.ID, event, payload, service.now().UTC())
	})
	if err != nil {
		return PremiseAsset{}, err
	}
	result, err := service.premiseAssetDTO(ctx, id)
	if err == nil {
		service.emit("premise:asset_changed", map[string]any{"premise_asset_uuid": result.UUID})
	}
	return result, err
}

func (service *Service) PermanentlyDeletePremiseAsset(ctx context.Context, assetUUID string, expectedRevision int64) (PremiseTrashDeleteResult, error) {
	result := PremiseTrashDeleteResult{BlockedItems: []PremiseAssetDeleteBlocker{}}
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row premiseAssetRecord
		if err := tx.Where("uuid = ? AND project_id=(SELECT id FROM projects WHERE uuid=?)", assetUUID, service.store.ProjectUUID()).First(&row).Error; err != nil {
			return notFound(err, "设定资产不存在")
		}
		if row.DeletedAt == nil {
			return domainError(CodeStateConflict, "只能永久删除回收站中的设定资产", "请先将素材移入回收站。", nil)
		}
		if row.Revision != expectedRevision {
			return domainError(CodeConflict, "设定资产已被修改", "刷新后基于最新 revision 重试。", nil)
		}
		locked := tx.Model(&premiseAssetRecord{}).
			Where("id = ? AND revision = ? AND deleted_at IS NOT NULL", row.ID, expectedRevision).
			UpdateColumn("updated_at", gorm.Expr("updated_at"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return domainError(CodeConflict, "设定资产已被修改", "刷新后基于最新 revision 重试。", nil)
		}
		blocked, err := premiseAssetDeleteBlocked(tx, row)
		if err != nil {
			return err
		}
		if blocked {
			return domainError(CodeDeleteBlocked, "设定资产仍被活动任务使用", "请等待相关生成或会话结束后重试。", nil)
		}
		return permanentlyDeletePremiseAsset(tx, row, service.now().UTC(), &result)
	})
	if err != nil {
		return PremiseTrashDeleteResult{}, err
	}
	service.emit("premise:asset_permanently_deleted", map[string]any{"premise_asset_uuid": assetUUID})
	return result, nil
}

func (service *Service) EmptyPremiseAssetTrash(ctx context.Context) (PremiseTrashDeleteResult, error) {
	result := PremiseTrashDeleteResult{BlockedItems: []PremiseAssetDeleteBlocker{}}
	deletedUUIDs := make([]string, 0)
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		project, _, err := service.projectActor(ctx, tx)
		if err != nil {
			return err
		}
		locked := tx.Model(&premiseAssetRecord{}).
			Where("project_id = ? AND deleted_at IS NOT NULL", project.ID).
			UpdateColumn("updated_at", gorm.Expr("updated_at"))
		if locked.Error != nil {
			return locked.Error
		}
		var rows []premiseAssetRecord
		if err := tx.Where("project_id = ? AND deleted_at IS NOT NULL", project.ID).Order("uuid ASC").Find(&rows).Error; err != nil {
			return err
		}
		now := service.now().UTC()
		for _, row := range rows {
			blocked, err := premiseAssetDeleteBlocked(tx, row)
			if err != nil {
				return err
			}
			if blocked {
				result.BlockedItems = append(result.BlockedItems, PremiseAssetDeleteBlocker{UUID: row.UUID, Reason: "active_task"})
				continue
			}
			if err := permanentlyDeletePremiseAsset(tx, row, now, &result); err != nil {
				return err
			}
			deletedUUIDs = append(deletedUUIDs, row.UUID)
		}
		return nil
	})
	if err != nil {
		return PremiseTrashDeleteResult{}, err
	}
	for _, assetUUID := range deletedUUIDs {
		service.emit("premise:asset_permanently_deleted", map[string]any{"premise_asset_uuid": assetUUID})
	}
	if len(deletedUUIDs) > 0 || len(result.BlockedItems) > 0 {
		service.emit("premise:trash_emptied", map[string]any{"deleted_count": result.DeletedCount, "blocked_count": len(result.BlockedItems)})
	}
	return result, nil
}

func premiseAssetDeleteBlocked(tx *gorm.DB, row premiseAssetRecord) (bool, error) {
	var count int64
	err := tx.Raw(`
		SELECT COUNT(*) FROM (
			SELECT t.id
			FROM production_task_runs t
			WHERE t.project_id=? AND t.status IN ('queued','running') AND (
				t.resource_uuid=? OR EXISTS (
					SELECT 1 FROM json_tree(t.input_snapshot) j
					WHERE j.value=?
					   OR j.value IN (SELECT uuid FROM premise_asset_variants WHERE premise_asset_id=?)
					   OR j.value IN (SELECT f.uuid FROM files f JOIN premise_asset_variants v ON v.file_id=f.id WHERE v.premise_asset_id=?)
				)
			)
			UNION ALL
			SELECT w.id
			FROM workflows w
			WHERE w.project_id=? AND w.status IN ('queued','running') AND EXISTS (
				SELECT 1 FROM json_tree(w.input_snapshot) j
				WHERE j.value=?
				   OR j.value IN (SELECT uuid FROM premise_asset_variants WHERE premise_asset_id=?)
				   OR j.value IN (SELECT f.uuid FROM files f JOIN premise_asset_variants v ON v.file_id=f.id WHERE v.premise_asset_id=?)
			)
			UNION ALL
			SELECT turns.id
			FROM chat_turns turns
			JOIN chat_items items ON items.turn_id=turns.id AND items.item_type='user_message'
			JOIN chat_context_references refs ON refs.chat_item_id=items.id
			JOIN chat_threads threads ON threads.id=turns.thread_id
			WHERE threads.project_id=? AND refs.resource_type='premise_asset' AND refs.resource_uuid=?
			  AND turns.status IN ('queued','in_progress','waiting_for_input')
			UNION ALL
			SELECT follow_ups.id
			FROM chat_follow_ups follow_ups
			JOIN chat_context_references refs ON refs.follow_up_id=follow_ups.id
			JOIN chat_threads threads ON threads.id=follow_ups.thread_id
			WHERE threads.project_id=? AND refs.resource_type='premise_asset' AND refs.resource_uuid=?
			  AND follow_ups.status='queued' AND follow_ups.deleted_at IS NULL
		)`,
		row.ProjectID, row.UUID, row.UUID, row.ID, row.ID,
		row.ProjectID, row.UUID, row.ID, row.ID,
		row.ProjectID, row.UUID,
		row.ProjectID, row.UUID,
	).Scan(&count).Error
	return count > 0, err
}

func permanentlyDeletePremiseAsset(tx *gorm.DB, row premiseAssetRecord, now time.Time, result *PremiseTrashDeleteResult) error {
	var fileIDs []int64
	if err := tx.Table("premise_asset_variants").Where("premise_asset_id = ?", row.ID).Order("file_id ASC").Distinct("file_id").Pluck("file_id", &fileIDs).Error; err != nil {
		return err
	}
	if err := tx.Model(&premiseAssetRecord{}).Where("id = ?", row.ID).UpdateColumn("current_variant_id", nil).Error; err != nil {
		return err
	}
	deleted := tx.Delete(&premiseAssetRecord{}, row.ID)
	if deleted.Error != nil {
		return deleted.Error
	}
	if deleted.RowsAffected != 1 {
		return domainError(CodeConflict, "设定资产已被修改", "刷新后重试。", nil)
	}
	result.DeletedCount++
	for _, fileID := range fileIDs {
		retained, err := premiseVariantFileRetained(tx, fileID)
		if err != nil {
			return err
		}
		if retained {
			result.RetainedFileCount++
			continue
		}
		updated := tx.Table("files").Where("id = ? AND deleted_at IS NULL", fileID).Update("deleted_at", now)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 1 {
			result.FileSoftDeletedCount++
		}
	}
	return nil
}

func premiseVariantFileRetained(tx *gorm.DB, fileID int64) (bool, error) {
	var retained int
	err := tx.Raw(`
		SELECT CASE WHEN
			EXISTS (SELECT 1 FROM premise_asset_variants WHERE file_id=?) OR
			EXISTS (SELECT 1 FROM premise_setting_images WHERE file_id=?) OR
			EXISTS (SELECT 1 FROM comic_image_variants WHERE file_id=?) OR
			EXISTS (SELECT 1 FROM comic_image_generations WHERE premise_file_id=?) OR
			EXISTS (SELECT 1 FROM comic_exports WHERE output_file_id=?) OR
			EXISTS (SELECT 1 FROM story_source_items WHERE file_id=?) OR
			EXISTS (SELECT 1 FROM chat_context_references WHERE file_id=? OR image_file_id=?) OR
			EXISTS (SELECT 1 FROM files WHERE source_file_id=?) OR
			EXISTS (SELECT 1 FROM upload_stashed WHERE finalized_file_id=? AND state<>'consumed') OR
			EXISTS (SELECT 1 FROM production_task_runs t, json_tree(t.input_snapshot) j JOIN files f ON f.uuid=j.value WHERE f.id=?) OR
			EXISTS (SELECT 1 FROM premise_generation_steps t, json_tree(t.input_snapshot) j JOIN files f ON f.uuid=j.value WHERE f.id=?) OR
			EXISTS (SELECT 1 FROM premise_generation_steps t, json_tree(t.output_json) j JOIN files f ON f.uuid=j.value WHERE f.id=?) OR
			EXISTS (SELECT 1 FROM comic_image_generations t, json_tree(t.input_snapshot) j JOIN files f ON f.uuid=j.value WHERE f.id=?) OR
			EXISTS (SELECT 1 FROM comic_image_variants t, json_tree(t.input_snapshot) j JOIN files f ON f.uuid=j.value WHERE f.id=?) OR
			EXISTS (SELECT 1 FROM comic_exports t, json_tree(t.snapshot_json) j JOIN files f ON f.uuid=j.value WHERE f.id=?) OR
			EXISTS (SELECT 1 FROM comic_chapter_snapshots t, json_tree(t.snapshot_json) j JOIN files f ON f.uuid=j.value WHERE f.id=?) OR
			EXISTS (SELECT 1 FROM workflows t, json_tree(t.input_snapshot) j JOIN files f ON f.uuid=j.value WHERE f.id=?) OR
			EXISTS (SELECT 1 FROM workflow_steps t, json_tree(t.input_json) j JOIN files f ON f.uuid=j.value WHERE f.id=?) OR
			EXISTS (SELECT 1 FROM workflow_steps t, json_tree(t.output_json) j JOIN files f ON f.uuid=j.value WHERE f.id=?)
		THEN 1 ELSE 0 END`,
		fileID, fileID, fileID, fileID, fileID, fileID, fileID, fileID, fileID,
		fileID, fileID, fileID, fileID, fileID, fileID, fileID, fileID, fileID, fileID, fileID,
	).Scan(&retained).Error
	return retained == 1, err
}

func (service *Service) premiseAssetDTO(ctx context.Context, id int64) (PremiseAsset, error) {
	var row premiseAssetRecord
	if err := service.store.DB().WithContext(ctx).First(&row, id).Error; err != nil {
		return PremiseAsset{}, err
	}
	var tags []string
	if err := service.store.DB().WithContext(ctx).Table("premise_asset_tags").Where("premise_asset_id = ?", row.ID).Order("tag").Pluck("tag", &tags).Error; err != nil {
		return PremiseAsset{}, err
	}
	result := PremiseAsset{UUID: row.UUID, AssetType: row.AssetType, Title: row.Title, Summary: row.Summary, Tags: tags, Position: json.RawMessage(row.PositionJSON), Crop: json.RawMessage(row.CropJSON), Revision: row.Revision, DeletedAt: row.DeletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.CurrentVariantID != nil {
		var variant assetVariantRecord
		if err := service.store.DB().WithContext(ctx).First(&variant, *row.CurrentVariantID).Error; err == nil {
			dto, dtoErr := service.assetVariantDTO(ctx, variant)
			if dtoErr != nil {
				return result, dtoErr
			}
			result.CurrentVariant = &dto
		}
	}
	return result, nil
}

func (service *Service) assetVariantDTO(ctx context.Context, row assetVariantRecord) (AssetVariant, error) {
	var fileRow struct{ UUID string }
	if err := service.store.DB().WithContext(ctx).Table("files").Select("uuid").Where("id = ?", row.FileID).Scan(&fileRow).Error; err != nil {
		return AssetVariant{}, err
	}
	asset, err := service.files.GetAsset(ctx, fileRow.UUID, false)
	if err != nil {
		return AssetVariant{}, err
	}
	var settingUUID string
	if row.SourceSettingImageID != nil {
		_ = service.store.DB().WithContext(ctx).Table("premise_setting_images").Where("id = ?", *row.SourceSettingImageID).Pluck("uuid", &settingUUID).Error
	}
	return AssetVariant{UUID: row.UUID, VersionNo: row.VersionNo, SourceType: row.SourceType, SourceSettingImageUUID: settingUUID, Crop: json.RawMessage(row.CropJSON), Asset: asset, CreatedAt: row.CreatedAt}, nil
}

func replaceTags(tx *gorm.DB, assetID int64, tags []string, now time.Time) error {
	if err := tx.Where("premise_asset_id = ?", assetID).Delete(&premiseTagRecord{}).Error; err != nil {
		return err
	}
	for _, tag := range tags {
		if err := tx.Create(&premiseTagRecord{PremiseAssetID: assetID, Tag: tag, CreatedAt: now}).Error; err != nil {
			return err
		}
	}
	return nil
}
func appendPremiseAssetEvent(tx *gorm.DB, assetID int64, eventType string, payload any, now time.Time) error {
	uuid, err := newUUIDv7()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Exec("INSERT INTO premise_asset_events (uuid,premise_asset_id,sequence,event_type,payload,created_at) SELECT ?,?,COALESCE(MAX(sequence),0)+1,?,?,? FROM premise_asset_events WHERE premise_asset_id=?", uuid, assetID, eventType, string(encoded), now, assetID).Error
}
func conflictErr(err error) error {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return domainError(CodeConflict, "资源名称或顺序冲突", "刷新后选择不同名称或顺序。", err)
	}
	return err
}
