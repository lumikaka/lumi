package story

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"lumi/internal/promptcatalog"

	"gorm.io/gorm"
)

var promptKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,119}$`)

func (service *Service) promptOptions() promptcatalog.PictureBookOptions {
	profile := service.store.OptionalPictureBookProfile()
	if profile == nil {
		return promptcatalog.PictureBookOptions{}
	}
	options := promptcatalog.PictureBookOptions{
		Format: profile.Format, AspectWidth: profile.AspectRatio.Width, AspectHeight: profile.AspectRatio.Height,
	}
	if profile.LargeImageMinimalText != nil {
		options.LargeImageMinimalText = *profile.LargeImageMinimalText
	}
	if profile.InteractionMode != nil {
		options.InteractionMode = *profile.InteractionMode
	}
	if profile.ComicLayout != nil {
		options.ComicLayout = *profile.ComicLayout
	}
	return options
}

func (service *Service) promptDefinitions(language string) []promptcatalog.Definition {
	return promptcatalog.DefinitionsForPictureBook(language, service.promptOptions())
}

func (service *Service) promptDefinition(group, key, language string) (promptcatalog.Definition, bool) {
	return promptcatalog.LookupForPictureBook(group, key, language, service.promptOptions())
}

func validatePromptIdentity(group, key string) (string, string, error) {
	group = strings.ToLower(strings.TrimSpace(group))
	key = strings.ToLower(strings.TrimSpace(key))
	if group != promptcatalog.GroupStory && group != promptcatalog.GroupChapter && group != promptcatalog.GroupPremise && group != promptcatalog.GroupPremiseStyle && group != promptcatalog.GroupAgent && group != promptcatalog.GroupRuntime {
		return "", "", storyError(CodeValidationFailed, "Prompt 分组无效", "prompt_group 只支持 story、chapter、premise、premise_style、agent 或 runtime。", nil)
	}
	if !promptKeyPattern.MatchString(key) {
		return "", "", storyError(CodeValidationFailed, "Prompt key 无效", "prompt_key 必须使用小写字母、数字、点、短横线或下划线，长度不超过 120。", nil)
	}
	return group, key, nil
}

func (service *Service) latestPromptRecord(ctx context.Context, projectID int64, group, key string) (promptVersionRecord, bool, error) {
	var record promptVersionRecord
	err := service.store.DB().WithContext(ctx).
		Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", projectID, group, key).
		Order("version_no DESC").First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return promptVersionRecord{}, false, nil
	}
	return record, err == nil, err
}

// EffectivePrompt resolves project override -> legacy override -> language
// builtin. It is the only resolution order used by new generation code.
func (service *Service) EffectivePrompt(ctx context.Context, group, key string) (string, error) {
	group, key, err := validatePromptIdentity(group, key)
	if err != nil {
		return "", err
	}
	projectRecord, _, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return "", err
	}
	definition, known := service.promptDefinition(group, key, projectRecord.GenerationLanguage)
	if !known {
		return "", storyError(CodeValidationFailed, "Prompt 不在内置目录中", "新生成链路只能读取规范 prompt key。", nil)
	}
	if current, found, loadErr := service.latestPromptRecord(ctx, projectRecord.ID, group, key); loadErr != nil {
		return "", loadErr
	} else if found && strings.TrimSpace(current.Prompt) != "" {
		return strings.TrimSpace(current.Prompt), nil
	}
	for _, legacyKey := range definition.LegacyKeys {
		if legacy, found, loadErr := service.latestPromptRecord(ctx, projectRecord.ID, group, legacyKey); loadErr != nil {
			return "", loadErr
		} else if found && strings.TrimSpace(legacy.Prompt) != "" {
			return strings.TrimSpace(legacy.Prompt), nil
		}
	}
	return strings.TrimSpace(definition.DefaultValue), nil
}

func (service *Service) ListPromptCatalog(ctx context.Context, group string) ([]PromptCatalogItem, error) {
	group = strings.ToLower(strings.TrimSpace(group))
	if group != "" {
		if _, _, err := validatePromptIdentity(group, "catalog"); err != nil {
			return nil, err
		}
	}
	projectRecord, _, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return nil, err
	}
	definitions := service.promptDefinitions(projectRecord.GenerationLanguage)
	items := make([]PromptCatalogItem, 0, len(definitions))
	for _, definition := range definitions {
		if group != "" && definition.Group != group {
			continue
		}
		current, found, err := service.latestPromptRecord(ctx, projectRecord.ID, definition.Group, definition.Key)
		if err != nil {
			return nil, err
		}
		usingLegacy := ""
		if !found {
			for _, legacyKey := range definition.LegacyKeys {
				legacy, legacyFound, legacyErr := service.latestPromptRecord(ctx, projectRecord.ID, definition.Group, legacyKey)
				if legacyErr != nil {
					return nil, legacyErr
				}
				if legacyFound {
					current, found, usingLegacy = legacy, true, legacyKey
					break
				}
			}
		}
		effective := strings.TrimSpace(definition.DefaultValue)
		var currentDTO *PromptVersion
		isCustom := false
		if found {
			effective = strings.TrimSpace(current.Prompt)
			dto, dtoErr := service.promptDTO(ctx, current)
			if dtoErr != nil {
				return nil, dtoErr
			}
			currentDTO = &dto
			isCustom = current.PromptHash != contentHash(definition.DefaultValue) || usingLegacy != ""
		}
		items = append(items, PromptCatalogItem{
			PromptGroup: definition.Group, PromptKey: definition.Key, Title: definition.Title,
			Description: definition.Description, PromptType: definition.PromptType, DefaultValue: strings.TrimSpace(definition.DefaultValue),
			EffectiveValue: effective, Placeholders: promptcatalog.Placeholders(definition.DefaultValue),
			LegacyKeys: definition.LegacyKeys, IsCustom: isCustom, UsingLegacyKey: usingLegacy,
			CurrentVersion: currentDTO,
		})
	}
	return items, nil
}

func (service *Service) promptDTO(ctx context.Context, record promptVersionRecord) (PromptVersion, error) {
	restoredUUID := ""
	if record.RestoredFromVersionID != nil {
		var restored promptVersionRecord
		if err := service.store.DB().WithContext(ctx).Select("uuid").First(&restored, *record.RestoredFromVersionID).Error; err != nil {
			return PromptVersion{}, err
		}
		restoredUUID = restored.UUID
	}
	return PromptVersion{UUID: record.UUID, PromptGroup: record.PromptGroup, PromptKey: record.PromptKey, VersionNo: record.VersionNo, Prompt: record.Prompt, PromptHash: record.PromptHash, SourceType: record.SourceType, RestoredFromUUID: restoredUUID, CreatedAt: record.CreatedAt}, nil
}

func normalizePagination(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

func (service *Service) ListPromptVersions(ctx context.Context, group, key string, page, perPage int) ([]PromptVersion, Pagination, error) {
	group, key, err := validatePromptIdentity(group, key)
	if err != nil {
		return nil, Pagination{}, err
	}
	page, perPage = normalizePagination(page, perPage)
	projectRecord, _, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return nil, Pagination{}, err
	}
	query := service.store.DB().WithContext(ctx).Model(&promptVersionRecord{}).Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", projectRecord.ID, group, key)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Pagination{}, err
	}
	var records []promptVersionRecord
	if err := query.Order("version_no DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&records).Error; err != nil {
		return nil, Pagination{}, err
	}
	items := make([]PromptVersion, 0, len(records))
	for _, record := range records {
		item, err := service.promptDTO(ctx, record)
		if err != nil {
			return nil, Pagination{}, err
		}
		items = append(items, item)
	}
	lastPage := int((total + int64(perPage) - 1) / int64(perPage))
	if lastPage < 1 {
		lastPage = 1
	}
	return items, Pagination{PerPage: perPage, CurrentPage: page, LastPage: lastPage, Total: total}, nil
}

type CreatePromptInput struct {
	PromptGroup            string
	PromptKey              string
	Prompt                 string
	ExpectedCurrentVersion int
}

func validateCatalogPrompt(definition promptcatalog.Definition, prompt string) error {
	if err := validateText(prompt, maxPromptBytes, "Prompt"); err != nil {
		return err
	}
	allowed := make(map[string]struct{})
	for _, placeholder := range promptcatalog.Placeholders(definition.DefaultValue) {
		allowed[placeholder] = struct{}{}
	}
	unknown := make([]string, 0)
	for _, placeholder := range promptcatalog.Placeholders(prompt) {
		if _, ok := allowed[placeholder]; !ok {
			unknown = append(unknown, placeholder)
		}
	}
	if len(unknown) > 0 {
		return storyError(CodeValidationFailed, "Prompt 占位符无效", "不支持的占位符："+strings.Join(unknown, "、")+"。", nil)
	}
	return nil
}

func (service *Service) CreatePromptVersion(ctx context.Context, input CreatePromptInput) (PromptVersion, error) {
	group, key, err := validatePromptIdentity(input.PromptGroup, input.PromptKey)
	if err != nil {
		return PromptVersion{}, err
	}
	if err := validateText(input.Prompt, maxPromptBytes, "Prompt"); err != nil {
		return PromptVersion{}, err
	}
	var result promptVersionRecord
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		projectRecord, actor, loadErr := service.projectAndActor(ctx, tx)
		if loadErr != nil {
			return loadErr
		}
		definition, known := service.promptDefinition(group, key, projectRecord.GenerationLanguage)
		if known {
			if validationErr := validateCatalogPrompt(definition, input.Prompt); validationErr != nil {
				return validationErr
			}
		}
		var current promptVersionRecord
		loadCurrentErr := tx.WithContext(ctx).Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", projectRecord.ID, group, key).Order("version_no DESC").First(&current).Error
		currentVersion := 0
		if loadCurrentErr == nil {
			currentVersion = current.VersionNo
		} else if !errors.Is(loadCurrentErr, gorm.ErrRecordNotFound) {
			return loadCurrentErr
		}
		if input.ExpectedCurrentVersion != currentVersion {
			return storyError(CodePromptRevisionConflict, "Prompt 版本冲突", "Prompt 历史已更新，请刷新后重试。", nil)
		}
		if currentVersion > 0 && current.PromptHash == contentHash(input.Prompt) {
			result = current
			return nil
		}
		versionUUID, uuidErr := newUUIDv7()
		if uuidErr != nil {
			return uuidErr
		}
		sourceType := "manual_edit"
		if known && contentHash(strings.TrimSpace(input.Prompt)) == contentHash(strings.TrimSpace(definition.DefaultValue)) {
			sourceType = "default_restore"
		}
		now := service.now().UTC()
		result = promptVersionRecord{UUID: versionUUID, ProjectID: projectRecord.ID, ActorID: actor.ID, PromptGroup: group, PromptKey: key, VersionNo: currentVersion + 1, Prompt: input.Prompt, PromptHash: contentHash(input.Prompt), SourceType: sourceType, CreatedAt: now}
		if createErr := tx.WithContext(ctx).Create(&result).Error; createErr != nil {
			if uniqueConflict(createErr) {
				return storyError(CodePromptRevisionConflict, "Prompt 版本冲突", "Prompt 历史已更新，请刷新后重试。", createErr)
			}
			return createErr
		}
		if group == promptcatalog.GroupPremiseStyle && key == "project_overall_style" {
			return syncPremiseStyleProjection(ctx, tx, projectRecord.ID, strings.TrimSpace(input.Prompt), now)
		}
		return nil
	})
	if err != nil {
		return PromptVersion{}, err
	}
	return service.promptDTO(ctx, result)
}

type UpdatePromptGroupInput struct {
	PromptGroup             string
	Prompts                 map[string]string
	ExpectedCurrentVersions map[string]int
}

func (service *Service) UpdatePromptGroup(ctx context.Context, input UpdatePromptGroupInput) ([]PromptCatalogItem, error) {
	group, _, err := validatePromptIdentity(input.PromptGroup, "catalog")
	if err != nil {
		return nil, err
	}
	if len(input.Prompts) == 0 || len(input.Prompts) != len(input.ExpectedCurrentVersions) {
		return nil, storyError(CodeValidationFailed, "Prompt 分组更新无效", "prompts 与 expected_current_versions 必须包含相同的非空 key 集合。", nil)
	}
	keys := make([]string, 0, len(input.Prompts))
	for key := range input.Prompts {
		if key != strings.ToLower(strings.TrimSpace(key)) {
			return nil, storyError(CodeValidationFailed, "Prompt key 无效", "prompts 的 key 必须使用规范小写目录 key。", nil)
		}
		if _, ok := input.ExpectedCurrentVersions[key]; !ok {
			return nil, storyError(CodeValidationFailed, "Prompt 分组更新无效", "prompts 与 expected_current_versions 必须包含相同的 key。", nil)
		}
		keys = append(keys, key)
	}
	for key, version := range input.ExpectedCurrentVersions {
		if key != strings.ToLower(strings.TrimSpace(key)) {
			return nil, storyError(CodeValidationFailed, "Prompt key 无效", "expected_current_versions 的 key 必须使用规范小写目录 key。", nil)
		}
		if _, ok := input.Prompts[key]; !ok || version < 0 {
			return nil, storyError(CodeValidationFailed, "Prompt 分组更新无效", "expected_current_versions 必须与 prompts 对应且版本号不能为负数。", nil)
		}
	}
	sort.Strings(keys)
	now := service.now().UTC()
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		projectRecord, actor, loadErr := service.projectAndActor(ctx, tx)
		if loadErr != nil {
			return loadErr
		}
		styleChanged := false
		styleValue := ""
		for _, key := range keys {
			definition, known := service.promptDefinition(group, key, projectRecord.GenerationLanguage)
			if !known || definition.Group != group {
				return storyError(CodeValidationFailed, "Prompt 不在内置目录中", group+"/"+key+" 不是可编辑的项目 Prompt。", nil)
			}
			prompt := input.Prompts[key]
			if validationErr := validateCatalogPrompt(definition, prompt); validationErr != nil {
				return validationErr
			}
			var current promptVersionRecord
			loadCurrentErr := tx.WithContext(ctx).
				Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", projectRecord.ID, group, key).
				Order("version_no DESC").First(&current).Error
			currentVersion := 0
			if loadCurrentErr == nil {
				currentVersion = current.VersionNo
			} else if !errors.Is(loadCurrentErr, gorm.ErrRecordNotFound) {
				return loadCurrentErr
			}
			if input.ExpectedCurrentVersions[key] != currentVersion {
				return storyError(CodePromptRevisionConflict, "Prompt 版本冲突", group+"/"+key+" 的候选历史已更新，请刷新后重试。", nil)
			}
			promptHash := contentHash(prompt)
			if currentVersion > 0 && current.PromptHash == promptHash {
				continue
			}
			versionUUID, uuidErr := newUUIDv7()
			if uuidErr != nil {
				return uuidErr
			}
			sourceType := "manual_edit"
			if contentHash(strings.TrimSpace(prompt)) == contentHash(strings.TrimSpace(definition.DefaultValue)) {
				sourceType = "default_restore"
			}
			next := promptVersionRecord{UUID: versionUUID, ProjectID: projectRecord.ID, ActorID: actor.ID, PromptGroup: group, PromptKey: key, VersionNo: currentVersion + 1, Prompt: prompt, PromptHash: promptHash, SourceType: sourceType, CreatedAt: now}
			if createErr := tx.WithContext(ctx).Create(&next).Error; createErr != nil {
				if uniqueConflict(createErr) {
					return storyError(CodePromptRevisionConflict, "Prompt 版本冲突", group+"/"+key+" 的候选历史已更新，请刷新后重试。", createErr)
				}
				return createErr
			}
			if group == promptcatalog.GroupPremiseStyle && key == "project_overall_style" {
				styleChanged, styleValue = true, strings.TrimSpace(prompt)
			}
		}
		if styleChanged {
			if updateErr := syncPremiseStyleProjection(ctx, tx, projectRecord.ID, styleValue, now); updateErr != nil {
				return updateErr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return service.ListPromptCatalog(ctx, group)
}

func syncPremiseStyleProjection(ctx context.Context, tx *gorm.DB, projectID int64, style string, now time.Time) error {
	return tx.WithContext(ctx).Exec(`
		UPDATE premise_profiles
		SET default_style = ?, revision = revision + 1, updated_at = ?
		WHERE project_id = ?
	`, style, now, projectID).Error
}

func (service *Service) EnsurePromptCatalogVersions(ctx context.Context, sourceType string) error {
	if sourceType != "project_created" && sourceType != "migration" {
		return storyError(CodeValidationFailed, "Prompt 初始化来源无效", "source_type 只支持 project_created 或 migration。", nil)
	}
	now := service.now().UTC()
	return service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		projectRecord, actor, err := service.projectAndActor(ctx, tx)
		if err != nil {
			return err
		}
		for _, definition := range service.promptDefinitions(projectRecord.GenerationLanguage) {
			var current promptVersionRecord
			currentErr := tx.WithContext(ctx).
				Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", projectRecord.ID, definition.Group, definition.Key).
				Order("version_no DESC").First(&current).Error
			if currentErr == nil {
				currentHash := contentHash(strings.TrimSpace(current.Prompt))
				if currentHash == contentHash(strings.TrimSpace(definition.DefaultValue)) {
					continue
				}
				if !tracksBuiltinPrompt(current.SourceType) {
					continue
				}
				matchesPreviousDefault := false
				for _, previous := range definition.PreviousDefaultValues {
					if currentHash == contentHash(strings.TrimSpace(previous)) {
						matchesPreviousDefault = true
						break
					}
				}
				if !matchesPreviousDefault {
					continue
				}
				versionUUID, uuidErr := newUUIDv7()
				if uuidErr != nil {
					return uuidErr
				}
				value := strings.TrimSpace(definition.DefaultValue)
				next := promptVersionRecord{UUID: versionUUID, ProjectID: projectRecord.ID, ActorID: actor.ID, PromptGroup: definition.Group, PromptKey: definition.Key, VersionNo: current.VersionNo + 1, Prompt: value, PromptHash: contentHash(value), SourceType: "migration", CreatedAt: now}
				if createErr := tx.WithContext(ctx).Create(&next).Error; createErr != nil {
					if uniqueConflict(createErr) {
						continue
					}
					return createErr
				}
				if definition.Group == promptcatalog.GroupPremiseStyle && definition.Key == "project_overall_style" {
					if syncErr := syncPremiseStyleProjection(ctx, tx, projectRecord.ID, value, now); syncErr != nil {
						return syncErr
					}
				}
				continue
			}
			if !errors.Is(currentErr, gorm.ErrRecordNotFound) {
				return currentErr
			}
			value := strings.TrimSpace(definition.DefaultValue)
			for _, legacyKey := range definition.LegacyKeys {
				var legacy promptVersionRecord
				legacyErr := tx.WithContext(ctx).
					Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", projectRecord.ID, definition.Group, legacyKey).
					Order("version_no DESC").First(&legacy).Error
				if legacyErr == nil {
					value = strings.TrimSpace(legacy.Prompt)
					break
				}
				if !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
					return legacyErr
				}
			}
			if definition.Group == promptcatalog.GroupPremiseStyle && definition.Key == "project_overall_style" {
				var legacyStyle string
				if styleErr := tx.WithContext(ctx).Table("premise_profiles").Select("default_style").Where("project_id = ?", projectRecord.ID).Scan(&legacyStyle).Error; styleErr != nil {
					return styleErr
				}
				if strings.TrimSpace(legacyStyle) != "" {
					value = strings.TrimSpace(legacyStyle)
				}
			}
			versionUUID, err := newUUIDv7()
			if err != nil {
				return err
			}
			record := promptVersionRecord{UUID: versionUUID, ProjectID: projectRecord.ID, ActorID: actor.ID, PromptGroup: definition.Group, PromptKey: definition.Key, VersionNo: 1, Prompt: value, PromptHash: contentHash(value), SourceType: sourceType, CreatedAt: now}
			if err := tx.WithContext(ctx).Create(&record).Error; err != nil {
				if uniqueConflict(err) {
					continue
				}
				return err
			}
		}
		return nil
	})
}

func tracksBuiltinPrompt(sourceType string) bool {
	switch sourceType {
	case "project_created", "migration", "project_language_changed", "default_restore":
		return true
	default:
		return false
	}
}

func (service *Service) EffectiveLanguageInstruction(ctx context.Context) (string, error) {
	return service.EffectivePrompt(ctx, promptcatalog.GroupRuntime, "project_language_instruction")
}

func (service *Service) RestorePromptVersion(ctx context.Context, versionUUID string, expectedCurrentVersion int) (PromptVersion, error) {
	if !isUUIDv7(versionUUID) {
		return PromptVersion{}, storyError(CodeValidationFailed, "Prompt 版本 UUID 无效", "版本资源标识必须是 UUIDv7。", nil)
	}
	var next promptVersionRecord
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		projectRecord, actor, loadErr := service.projectAndActor(ctx, tx)
		if loadErr != nil {
			return loadErr
		}
		var restored promptVersionRecord
		if restoreErr := tx.WithContext(ctx).Where("project_id = ? AND uuid = ?", projectRecord.ID, versionUUID).First(&restored).Error; restoreErr != nil {
			return recordNotFound(restoreErr, CodePromptVersionNotFound, "Prompt 版本不存在", "该历史候选不存在或不属于当前项目。")
		}
		var current promptVersionRecord
		if currentErr := tx.WithContext(ctx).Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", projectRecord.ID, restored.PromptGroup, restored.PromptKey).Order("version_no DESC").First(&current).Error; currentErr != nil {
			return currentErr
		}
		if current.VersionNo != expectedCurrentVersion {
			return storyError(CodePromptRevisionConflict, "Prompt 版本冲突", "Prompt 历史已更新，请刷新后重试。", nil)
		}
		versionUUIDNew, uuidErr := newUUIDv7()
		if uuidErr != nil {
			return uuidErr
		}
		now := service.now().UTC()
		next = promptVersionRecord{UUID: versionUUIDNew, ProjectID: projectRecord.ID, ActorID: actor.ID, RestoredFromVersionID: &restored.ID, PromptGroup: restored.PromptGroup, PromptKey: restored.PromptKey, VersionNo: current.VersionNo + 1, Prompt: restored.Prompt, PromptHash: restored.PromptHash, SourceType: "version_restore", CreatedAt: now}
		if createErr := tx.WithContext(ctx).Create(&next).Error; createErr != nil {
			if uniqueConflict(createErr) {
				return storyError(CodePromptRevisionConflict, "Prompt 版本冲突", "Prompt 历史已更新，请刷新后重试。", createErr)
			}
			return createErr
		}
		if restored.PromptGroup == promptcatalog.GroupPremiseStyle && restored.PromptKey == "project_overall_style" {
			return syncPremiseStyleProjection(ctx, tx, projectRecord.ID, strings.TrimSpace(restored.Prompt), now)
		}
		return nil
	})
	if err != nil {
		return PromptVersion{}, err
	}
	return service.promptDTO(ctx, next)
}
