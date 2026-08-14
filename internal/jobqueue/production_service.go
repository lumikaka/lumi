package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"lumi/internal/modelsettings"
	"lumi/internal/picturebook"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/story"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"gorm.io/gorm"
)

func (manager *Manager) CreatePremiseSettingGeneration(ctx context.Context, projectUUID, sourceUUID string, input CreateProductionGenerationInput) (ProductionTask, error) {
	if err := validateProductionParameters(input.Parameters); err != nil {
		return ProductionTask{}, err
	}
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	generationLanguage, err := loadProjectGenerationLanguage(ctx, runtime.store)
	if err != nil {
		return ProductionTask{}, taskError(CodeTaskPersistenceFailed, "无法读取项目生成语言", "任务尚未创建。", err)
	}
	service := production.NewService(runtime.store, manager.hub)
	sources, err := service.ListPremiseSources(ctx)
	if err != nil {
		return ProductionTask{}, err
	}
	var source *production.PremiseSource
	for index := range sources {
		if sources[index].UUID == sourceUUID {
			source = &sources[index]
			break
		}
	}
	if source == nil {
		return ProductionTask{}, taskError(CodeInvalidTask, "Premise source 不存在", "只能为当前项目 source 创建任务。", nil)
	}
	if source.IgnoredAt != nil {
		return ProductionTask{}, taskError(CodeInvalidTask, "Premise 批次已忽略", "恢复批次后才能继续生成设定总览图。", nil)
	}
	resolved, model, modelSource, err := manager.resolveProductionProvider(ctx, runtime.store, modelsettings.ProjectImage, input.ProviderUUID, input.Model, true)
	if err != nil {
		return ProductionTask{}, err
	}
	inputText, err := boundedProductionPrompt(input.Prompt, source.SourceText)
	if err != nil {
		return ProductionTask{}, err
	}
	storyService := story.NewService(runtime.store)
	template, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupPremise, premiseSettingImagePromptKey)
	if err != nil {
		return ProductionTask{}, err
	}
	languageInstruction, err := storyService.EffectiveLanguageInstruction(ctx)
	if err != nil {
		return ProductionTask{}, err
	}
	prompt := renderPremiseSettingImagePrompt(template, inputText, source.StyleSnapshot, generationLanguage, languageInstruction)
	parameters, _ := json.Marshal(input.Parameters)
	snapshot := production.GenerationSnapshot{Version: 2, Kind: KindPremiseSettingGeneration, ProjectUUID: projectUUID, GenerationLanguage: generationLanguage, ResourceUUID: source.UUID, SourceUUID: source.UUID, Prompt: prompt, PromptTemplate: template, LanguageInstruction: languageInstruction, StyleSnapshot: source.StyleSnapshot, ProviderUUID: resolved.UUID, ProviderType: resolved.ProviderType, ProviderBaseURL: resolved.BaseURL, Model: model, ModelSource: modelSource, Parameters: parameters}
	return manager.createProductionTask(ctx, runtime, snapshot, input.IdempotencyKey, func(tx *sql.Tx, taskID int64, taskUUID string, encoded []byte, now time.Time) error {
		stepUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO premise_generation_steps(uuid,project_id,task_uuid,source_id,step_type,status,input_snapshot,created_at) VALUES(?,?,?,(SELECT id FROM premise_sources WHERE project_id=? AND uuid=?),'setting_generation','queued',?,?)`, stepUUID, runtime.projectID, taskUUID, runtime.projectID, sourceUUID, string(encoded), now)
		return err
	})
}

func (manager *Manager) CreatePremiseBreakdown(ctx context.Context, projectUUID, settingUUID string, input CreateProductionGenerationInput) (ProductionTask, error) {
	if err := validateProductionParameters(input.Parameters); err != nil {
		return ProductionTask{}, err
	}
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	generationLanguage, err := loadProjectGenerationLanguage(ctx, runtime.store)
	if err != nil {
		return ProductionTask{}, taskError(CodeTaskPersistenceFailed, "无法读取项目生成语言", "任务尚未创建。", err)
	}
	service := production.NewService(runtime.store, manager.hub)
	images, err := service.ListSettingImages(ctx)
	if err != nil {
		return ProductionTask{}, err
	}
	var setting *production.SettingImage
	for index := range images {
		if images[index].UUID == settingUUID {
			setting = &images[index]
			break
		}
	}
	if setting == nil {
		return ProductionTask{}, taskError(CodeInvalidTask, "设置图不存在", "只能拆分当前项目的设置图。", nil)
	}
	if setting.SourceUUID != "" {
		sources, sourceErr := service.ListPremiseSources(ctx)
		if sourceErr != nil {
			return ProductionTask{}, sourceErr
		}
		for _, source := range sources {
			if source.UUID == setting.SourceUUID && source.IgnoredAt != nil {
				return ProductionTask{}, taskError(CodeInvalidTask, "Premise 批次已忽略", "恢复批次后才能拆分设定项。", nil)
			}
		}
	}
	resolved, model, modelSource, err := manager.resolveProductionProvider(ctx, runtime.store, modelsettings.ProjectText, input.ProviderUUID, input.Model, false)
	if err != nil {
		return ProductionTask{}, err
	}
	profile, err := service.GetPremise(ctx)
	if err != nil {
		return ProductionTask{}, err
	}
	sourceText := ""
	styleSnapshot := profile.DefaultStyle
	if setting.SourceUUID != "" {
		sources, sourceErr := service.ListPremiseSources(ctx)
		if sourceErr != nil {
			return ProductionTask{}, sourceErr
		}
		for _, source := range sources {
			if source.UUID == setting.SourceUUID {
				sourceText = source.SourceText
				styleSnapshot = source.StyleSnapshot
				break
			}
		}
	}
	sourceText, err = boundedProductionPrompt(sourceText, input.Prompt)
	if err != nil {
		return ProductionTask{}, err
	}
	storyService := story.NewService(runtime.store)
	prompt, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupPremise, premiseAssetBreakdownPromptKey)
	if err != nil {
		return ProductionTask{}, err
	}
	languageInstruction, err := storyService.EffectiveLanguageInstruction(ctx)
	if err != nil {
		return ProductionTask{}, err
	}
	parameters, _ := json.Marshal(input.Parameters)
	snapshot := production.GenerationSnapshot{Version: 2, Kind: KindPremiseAssetBreakdown, ProjectUUID: projectUUID, GenerationLanguage: generationLanguage, ResourceUUID: setting.UUID, SourceUUID: setting.SourceUUID, SourceText: sourceText, Prompt: prompt, PromptTemplate: prompt, LanguageInstruction: languageInstruction, StyleSnapshot: styleSnapshot, ProviderUUID: resolved.UUID, ProviderType: resolved.ProviderType, ProviderBaseURL: resolved.BaseURL, Model: model, ModelSource: modelSource, Parameters: parameters}
	return manager.createProductionTask(ctx, runtime, snapshot, input.IdempotencyKey, func(tx *sql.Tx, taskID int64, taskUUID string, encoded []byte, now time.Time) error {
		stepUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO premise_generation_steps(uuid,project_id,task_uuid,source_id,setting_image_id,step_type,status,input_snapshot,created_at) VALUES(?,?,?,(SELECT source_id FROM premise_setting_images WHERE project_id=? AND uuid=?),(SELECT id FROM premise_setting_images WHERE project_id=? AND uuid=?),'asset_breakdown','queued',?,?)`, stepUUID, runtime.projectID, taskUUID, runtime.projectID, settingUUID, runtime.projectID, settingUUID, string(encoded), now)
		return err
	})
}

func (manager *Manager) CreatePremiseAssetGeneration(ctx context.Context, projectUUID, resourceUUID string, input CreateProductionGenerationInput) (ProductionTask, error) {
	if err := validateProductionParameters(input.Parameters); err != nil {
		return ProductionTask{}, err
	}
	if !isUUIDv7(resourceUUID) {
		return ProductionTask{}, taskError(CodeInvalidTask, "生成目标 UUID 无效", "resource_uuid 必须是 UUIDv7。", nil)
	}
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	generationLanguage, err := loadProjectGenerationLanguage(ctx, runtime.store)
	if err != nil {
		return ProductionTask{}, taskError(CodeTaskPersistenceFailed, "无法读取项目生成语言", "任务尚未创建。", err)
	}
	service := production.NewService(runtime.store, manager.hub)
	profile, err := service.GetPremise(ctx)
	if err != nil {
		return ProductionTask{}, err
	}
	operation := strings.ToLower(strings.TrimSpace(input.AssetOperation))
	if operation == "" {
		operation = "create"
	}
	assetType := strings.ToLower(strings.TrimSpace(input.AssetType))
	assetTitle := strings.TrimSpace(input.AssetTitle)
	assetSummary := strings.TrimSpace(input.AssetSummary)
	assetTags := normalizeProductionTags(input.AssetTags)
	var assetRevision int64
	switch operation {
	case "create":
		if !validPremiseAssetType(assetType) || assetTitle == "" || len([]rune(assetTitle)) > 160 || len([]rune(assetSummary)) > 12000 {
			return ProductionTask{}, taskError(CodeInvalidTask, "AI 设定项信息无效", "asset_type、asset_title 或 asset_summary 不符合限制。", nil)
		}
	case "variant":
		asset, assetErr := service.GetPremiseAsset(ctx, resourceUUID)
		if assetErr != nil || asset.DeletedAt != nil || asset.CurrentVariant == nil {
			return ProductionTask{}, taskError(CodeInvalidTask, "设定项引用无效", "只能为 active 且已有图片的设定项生成新版本。", assetErr)
		}
		assetType, assetTitle, assetSummary, assetTags, assetRevision = asset.AssetType, asset.Title, asset.Summary, asset.Tags, asset.Revision
	default:
		return ProductionTask{}, taskError(CodeInvalidTask, "AI 设定项操作无效", "asset_operation 只支持 create/variant。", nil)
	}
	resolved, model, modelSource, err := manager.resolveProductionProvider(ctx, runtime.store, modelsettings.ProjectImage, input.ProviderUUID, input.Model, true)
	if err != nil {
		return ProductionTask{}, err
	}
	prompt, err := boundedProductionPrompt(input.Prompt, assetTitle)
	if err != nil {
		return ProductionTask{}, err
	}
	storyService := story.NewService(runtime.store)
	template, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupPremise, "single_asset_generation")
	if err != nil {
		return ProductionTask{}, err
	}
	styleSnapshot, err := effectiveProductionStyle(ctx, runtime, storyService, profile.DefaultStyle, generationLanguage)
	if err != nil {
		return ProductionTask{}, err
	}
	premiseText := ""
	if profile.CurrentSource != nil {
		premiseText = profile.CurrentSource.SourceText
	}
	contextJSON, _ := json.Marshal(map[string]any{"operation": operation, "asset_type": assetType, "title": assetTitle, "summary": assetSummary, "tags": assetTags, "premise_text": premiseText})
	rendered, renderErr := promptcatalog.Render(template, map[string]string{"input_text": prompt, "premise_context": string(contextJSON), "style_prompt": styleSnapshot})
	if renderErr != nil {
		return ProductionTask{}, taskError(CodeInvalidTask, "Premise 单项提示词无法渲染", "请检查项目提示词是否保留全部规范占位符。", renderErr)
	}
	languageInstruction, err := storyService.EffectiveLanguageInstruction(ctx)
	if err != nil {
		return ProductionTask{}, err
	}
	prompt = promptcatalog.WithInstruction(rendered, languageInstruction)
	parameters, _ := json.Marshal(input.Parameters)
	snapshot := production.GenerationSnapshot{
		Version: 2, Kind: KindPremiseAssetGeneration, ProjectUUID: projectUUID,
		GenerationLanguage: generationLanguage, ResourceUUID: resourceUUID,
		Prompt: prompt, PromptTemplate: template, LanguageInstruction: languageInstruction, StyleSnapshot: styleSnapshot,
		AssetOperation: operation, AssetType: assetType, AssetTitle: assetTitle,
		AssetSummary: assetSummary, AssetTags: assetTags, AssetRevision: assetRevision,
		ProviderUUID: resolved.UUID, ProviderType: resolved.ProviderType,
		ProviderBaseURL: resolved.BaseURL, Model: model, ModelSource: modelSource, Parameters: parameters,
	}
	return manager.createProductionTask(ctx, runtime, snapshot, input.IdempotencyKey, nil)
}

func validPremiseAssetType(value string) bool {
	switch value {
	case production.AssetCharacter, production.AssetScene, production.AssetProp, production.AssetReference:
		return true
	}
	return false
}

func normalizeProductionTags(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || len([]rune(value)) > 64 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == 32 {
			break
		}
	}
	return result
}

func (manager *Manager) CreateComicImageGeneration(ctx context.Context, projectUUID, chapterUUID, sectionUUID string, input CreateProductionGenerationInput) (ProductionTask, error) {
	return manager.createComicImageGeneration(ctx, projectUUID, chapterUUID, sectionUUID, input, true)
}

func (manager *Manager) createComicImageGeneration(ctx context.Context, projectUUID, chapterUUID, sectionUUID string, input CreateProductionGenerationInput, createVisibleWorkflow bool) (ProductionTask, error) {
	if err := validateProductionParameters(input.Parameters); err != nil {
		return ProductionTask{}, err
	}
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	generationLanguage, err := loadProjectGenerationLanguage(ctx, runtime.store)
	if err != nil {
		return ProductionTask{}, taskError(CodeTaskPersistenceFailed, "无法读取项目生成语言", "任务尚未创建。", err)
	}
	service := production.NewService(runtime.store, manager.hub)
	section, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	if section.CurrentStoryboard == nil {
		return ProductionTask{}, taskError(CodeInvalidTask, "Section 尚无 Storyboard", "先创建或选择 storyboard 版本。", nil)
	}
	resolved, model, modelSource, err := manager.resolveProductionProvider(ctx, runtime.store, modelsettings.ProjectImage, input.ProviderUUID, input.Model, true)
	if err != nil {
		return ProductionTask{}, err
	}
	pictureBook := runtime.store.PictureBookProfile()
	outputSize, err := picturebook.ResolveImageSize(pictureBook, resolved.ProviderType, model)
	if err != nil {
		return ProductionTask{}, taskError(picturebook.CodeAspectRatioUnsupported, "图片模型不支持项目比例", "请切换到支持该精确比例的图片模型后重试；系统不会自动裁剪或改用近似比例。", err)
	}
	profile, err := service.GetPremise(ctx)
	if err != nil {
		return ProductionTask{}, err
	}
	storyService := story.NewService(runtime.store)
	styleSnapshot, err := effectiveProductionStyle(ctx, runtime, storyService, profile.DefaultStyle, generationLanguage)
	if err != nil {
		return ProductionTask{}, err
	}
	imageTemplate, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupChapter, "section_image")
	if err != nil {
		return ProductionTask{}, err
	}
	beforeImagePrompt, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupChapter, "before_image")
	if err != nil {
		return ProductionTask{}, err
	}
	imageTemplate = strings.ReplaceAll(imageTemplate, "{{before_image_prompt}}", beforeImagePrompt)
	referencePresentPrompt, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupChapter, "section_reference_present")
	if err != nil {
		return ProductionTask{}, err
	}
	referenceAbsentPrompt, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupChapter, "section_reference_absent")
	if err != nil {
		return ProductionTask{}, err
	}
	additionalDirectionPrompt, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupChapter, "section_additional_direction")
	if err != nil {
		return ProductionTask{}, err
	}
	languageInstruction, err := storyService.EffectiveLanguageInstruction(ctx)
	if err != nil {
		return ProductionTask{}, err
	}
	selectionTemplate, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupChapter, "section_premise_selection")
	if err != nil {
		return ProductionTask{}, err
	}
	if len(input.PremiseAssetUUIDs) > maxSectionPremiseAssets {
		return ProductionTask{}, taskError(CodeInvalidTask, "Premise 引用过多", fmt.Sprintf("一次最多引用 %d 个设定资产。", maxSectionPremiseAssets), nil)
	}
	seenReferences := make(map[string]struct{}, len(input.PremiseAssetUUIDs))
	premiseReferences := make([]production.PremiseAssetReference, 0, len(input.PremiseAssetUUIDs))
	for _, uuid := range input.PremiseAssetUUIDs {
		if _, exists := seenReferences[uuid]; exists {
			return ProductionTask{}, taskError(CodeInvalidTask, "Premise 引用重复", "premise_asset_uuids 不得重复。", nil)
		}
		seenReferences[uuid] = struct{}{}
		asset, err := service.GetPremiseAsset(ctx, uuid)
		if err != nil || asset.DeletedAt != nil || asset.CurrentVariant == nil {
			return ProductionTask{}, taskError(CodeInvalidTask, "Premise 引用无效", "只能引用 active 且有 current variant 的设定资产。", err)
		}
		premiseReferences = append(premiseReferences, production.PremiseAssetReference{AssetUUID: asset.UUID, VariantUUID: asset.CurrentVariant.UUID, FileUUID: asset.CurrentVariant.Asset.UUID, Title: asset.Title})
	}
	allAssets, err := service.ListPremiseAssets(ctx, "", "active")
	if err != nil {
		return ProductionTask{}, err
	}
	candidates := make([]production.PremiseAssetReference, 0, len(allAssets))
	for _, asset := range allAssets {
		if asset.CurrentVariant == nil {
			continue
		}
		candidates = append(candidates, production.PremiseAssetReference{AssetUUID: asset.UUID, VariantUUID: asset.CurrentVariant.UUID, FileUUID: asset.CurrentVariant.Asset.UUID, Title: asset.Title})
		if len(candidates) == 200 {
			break
		}
	}
	titles := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		titles = append(titles, candidate.Title)
	}
	titleLines := make([]string, 0, len(titles))
	for _, title := range titles {
		titleLines = append(titleLines, "- "+title)
	}
	selectionPrompt, err := promptcatalog.Render(selectionTemplate, map[string]string{
		"max_files": fmt.Sprintf("%d", maxSectionPremiseAssets), "section_id": section.UUID, "titles": strings.Join(titleLines, "\n"), "storyboard": section.CurrentStoryboard.ContentMD,
	})
	if err != nil {
		return ProductionTask{}, taskError(CodeInvalidTask, "Section 设定项选择提示词无法渲染", "请检查项目提示词是否保留全部规范占位符。", err)
	}
	selectionPrompt = promptcatalog.WithInstruction(selectionPrompt, languageInstruction)
	selectionProvider, selectionModel, selectionModelSource, err := manager.resolveProjectModel(ctx, runtime.store, modelsettings.SectionPremiseSelection, modelsettings.KindText, input.SelectionProviderUUID, input.SelectionModel)
	if err != nil {
		return ProductionTask{}, err
	}
	prompt, err := boundedProductionPrompt(input.Prompt, section.CurrentStoryboard.ContentMD)
	if err != nil {
		return ProductionTask{}, err
	}
	parameters, _ := json.Marshal(input.Parameters)
	snapshot := production.GenerationSnapshot{Version: 4, Kind: KindComicImageGeneration, ProjectUUID: projectUUID, GenerationLanguage: generationLanguage, ResourceUUID: section.UUID, ChapterUUID: chapterUUID,
		Prompt: prompt, PromptTemplate: imageTemplate, LanguageInstruction: languageInstruction, ReferencePresentPrompt: referencePresentPrompt, ReferenceAbsentPrompt: referenceAbsentPrompt, AdditionalDirectionPrompt: additionalDirectionPrompt, SelectionPrompt: selectionPrompt, SelectionProviderUUID: selectionProvider.UUID, SelectionBaseURL: selectionProvider.BaseURL, SelectionModel: selectionModel, SelectionModelSource: selectionModelSource,
		StyleSnapshot: styleSnapshot, StoryboardUUID: section.CurrentStoryboard.UUID, StoryboardMD: section.CurrentStoryboard.ContentMD,
		PremiseAssets: premiseReferences, PremiseCandidates: candidates, ProviderUUID: resolved.UUID, ProviderType: resolved.ProviderType, ProviderBaseURL: resolved.BaseURL, Model: model, ModelSource: modelSource, PictureBook: &pictureBook, OutputSize: outputSize.String(), Parameters: parameters}
	task, err := manager.createProductionTask(ctx, runtime, snapshot, input.IdempotencyKey, func(tx *sql.Tx, taskID int64, taskUUID string, encoded []byte, now time.Time) error {
		generationUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO comic_image_generations(uuid,comic_section_id,task_uuid,status,input_snapshot,created_at) VALUES(?,(SELECT s.id FROM comic_sections s JOIN chapter_comic_states cs ON cs.id=s.chapter_comic_state_id JOIN chapters c ON c.id=cs.chapter_id WHERE s.uuid=? AND c.uuid=?),?,'queued',?,?)`, generationUUID, sectionUUID, chapterUUID, taskUUID, string(encoded), now); err != nil {
			return err
		}
		if !createVisibleWorkflow {
			return nil
		}
		return createComicImageWorkflowTx(ctx, tx, runtime.projectID, projectUUID, chapterUUID, generationUUID, taskUUID, section, resolved.UUID, model, modelSource, now)
	})
	if err == nil && createVisibleWorkflow {
		runtime.broadcastComicWorkflow("workflow:queued", task.UUID)
	}
	return task, err
}

type ComicExportOperation struct {
	Export production.Export `json:"export"`
	Task   ProductionTask    `json:"task"`
}

func (manager *Manager) CreateComicExport(ctx context.Context, projectUUID string, input CreateExportInput) (ComicExportOperation, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return ComicExportOperation{}, err
	}
	service := production.NewService(runtime.store, manager.hub)
	snapshot, hash, err := service.BuildExportSnapshotWithOptions(ctx, input.Scope, input.ChapterUUID, input.AllowMissingImages)
	if err != nil {
		return ComicExportOperation{}, err
	}
	encoded, _ := json.Marshal(snapshot)
	resourceUUID := projectUUID
	if input.Scope == "chapter" {
		resourceUUID = input.ChapterUUID
	}
	generation := production.GenerationSnapshot{Version: 1, Kind: KindComicExport, ProjectUUID: projectUUID, ResourceUUID: resourceUUID, ChapterUUID: input.ChapterUUID, Prompt: hash, Parameters: encoded}
	task, err := manager.createProductionTask(ctx, runtime, generation, input.IdempotencyKey, func(tx *sql.Tx, taskID int64, taskUUID string, taskSnapshot []byte, now time.Time) error {
		transactionSnapshot, transactionHash, err := service.BuildExportSnapshotTx(ctx, tx, input.Scope, input.ChapterUUID, input.AllowMissingImages)
		if err != nil {
			return err
		}
		if transactionHash != hash {
			return &production.Error{Code: production.CodeExportChanged, Message: "导出内容已变化", Details: "Section 或 current image 在任务创建前发生变化，请重新检查导出完整性。"}
		}
		transactionEncoded, err := json.Marshal(transactionSnapshot)
		if err != nil {
			return err
		}
		exportUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		relativePath := production.ExportRelativePath(exportUUID, input.Scope, input.ChapterUUID, transactionHash, transactionSnapshot)
		_, err = tx.ExecContext(ctx, `INSERT INTO comic_exports(uuid,project_id,chapter_id,task_uuid,scope,format,status,snapshot_json,snapshot_hash,relative_path,retention_days,created_at) VALUES(?,?,CASE WHEN ?='chapter' THEN (SELECT id FROM chapters WHERE project_id=? AND uuid=?) ELSE NULL END,?,?,'zip','queued',?,?,?,7,?)`, exportUUID, runtime.projectID, input.Scope, runtime.projectID, input.ChapterUUID, taskUUID, input.Scope, string(transactionEncoded), transactionHash, relativePath, now)
		return err
	})
	if err != nil {
		return ComicExportOperation{}, err
	}
	operationHash := hash
	var frozen production.GenerationSnapshot
	if json.Unmarshal(task.InputSnapshot, &frozen) == nil && frozen.Kind == KindComicExport && len(frozen.Prompt) == 64 {
		operationHash = frozen.Prompt
	}
	export, err := service.ExportForTaskOrReadySnapshot(ctx, task.UUID, operationHash)
	if err != nil {
		return ComicExportOperation{}, err
	}
	return ComicExportOperation{Export: export, Task: task}, nil
}

func (manager *Manager) resolveProductionProvider(ctx context.Context, store *project.Store, settingKey, providerUUID, model string, image bool) (resolvedProvider, string, string, error) {
	kind := modelsettings.KindText
	if image {
		kind = modelsettings.KindImage
	}
	item, model, source, err := manager.resolveProjectModel(ctx, store, settingKey, kind, providerUUID, model)
	if err != nil {
		return resolvedProvider{}, "", "", err
	}
	baseURL := item.BaseURL
	if image {
		baseURL = item.ImageBaseURL
	}
	return resolvedProvider{UUID: item.UUID, ProviderType: item.ProviderType, BaseURL: baseURL}, model, source, nil
}

func boundedProductionPrompt(value, fallback string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = strings.TrimSpace(fallback)
	}
	if value == "" || len([]rune(value)) > 262144 {
		return "", taskError(CodeInvalidTask, "生成 Prompt 无效", "prompt 不能为空且最多 262144 个字符。", nil)
	}
	return value, nil
}

func validateProductionParameters(parameters GenerationParameters) error {
	if parameters.Temperature != nil && (*parameters.Temperature < 0 || *parameters.Temperature > 2) {
		return taskError(CodeInvalidTask, "生成参数无效", "temperature 必须在 0 到 2 之间。", nil)
	}
	if parameters.MaxTokens < 0 || parameters.MaxTokens > 200000 {
		return taskError(CodeInvalidTask, "生成参数无效", "max_tokens 超出安全范围。", nil)
	}
	return nil
}

type resolvedProvider struct{ UUID, ProviderType, BaseURL string }

type productionInsertHook func(*sql.Tx, int64, string, []byte, time.Time) error

func (manager *Manager) createProductionTask(ctx context.Context, runtime *projectRuntime, snapshot production.GenerationSnapshot, idempotencyKey string, hook productionInsertHook) (ProductionTask, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		return ProductionTask{}, taskError(CodeInvalidTask, "idempotency_key 无效", "必须提供 1-255 字符的幂等键。", nil)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return ProductionTask{}, err
	}
	taskUUID, err := newUUIDv7()
	if err != nil {
		return ProductionTask{}, err
	}
	now := manager.now().UTC()
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return ProductionTask{}, err
	}
	defer tx.Rollback()
	if existing, found, err := findProductionTaskTx(ctx, tx, runtime.projectID, snapshot.Kind, idempotencyKey); err != nil {
		return ProductionTask{}, err
	} else if found {
		_ = tx.Commit()
		return existing.DTO(), nil
	}
	var active string
	err = tx.QueryRowContext(ctx, `SELECT uuid FROM production_task_runs WHERE project_id=? AND kind=? AND resource_uuid=? AND status IN ('queued','running') LIMIT 1`, runtime.projectID, snapshot.Kind, snapshot.ResourceUUID).Scan(&active)
	if err == nil {
		return ProductionTask{}, taskError(CodeTaskConflict, "资源已有生产任务", "请等待任务 "+active+" 完成或取消。", nil)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ProductionTask{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,provider_uuid,model,model_source,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,?,?,?,'queued',?,?,?,?,0,0,3,?,?)`, taskUUID, runtime.projectID, snapshot.Kind, snapshot.ResourceUUID, string(encoded), idempotencyKey, snapshot.ProviderUUID, snapshot.Model, snapshot.ModelSource, now, now)
	if err != nil {
		return ProductionTask{}, taskError(CodeTaskPersistenceFailed, "无法持久化生产任务", "任务未创建。", err)
	}
	taskID, err := result.LastInsertId()
	if err != nil {
		return ProductionTask{}, err
	}
	if err := appendProductionEventTx(ctx, tx, taskID, "task_queued", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": taskUUID, "resource_uuid": snapshot.ResourceUUID, "status": StatusQueued, "progress": 0}, now); err != nil {
		return ProductionTask{}, err
	}
	if hook != nil {
		if err := hook(tx, taskID, taskUUID, encoded, now); err != nil {
			return ProductionTask{}, err
		}
	}
	inserted, err := runtime.client.InsertTx(ctx, tx, productionArgs{Version: 1, ProjectUUID: runtime.projectUUID, TaskUUID: taskUUID, TaskKind: snapshot.Kind, ResourceUUID: snapshot.ResourceUUID}, &river.InsertOpts{Queue: QueueProduction, MaxAttempts: 3, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled}}})
	if err != nil {
		return ProductionTask{}, err
	}
	if inserted.UniqueSkippedAsDuplicate {
		return ProductionTask{}, taskError(CodeTaskConflict, "重复生产任务", "River 拒绝了重复任务。", nil)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE production_task_runs SET river_job_id=? WHERE id=?", inserted.Job.ID, taskID); err != nil {
		return ProductionTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProductionTask{}, err
	}
	task, err := manager.GetProductionTask(ctx, runtime.projectUUID, taskUUID)
	if err == nil {
		runtime.broadcastProduction("production_task:queued", task)
	}
	return task, err
}

func (manager *Manager) ListProductionTasks(ctx context.Context, projectUUID, status string, limit int) ([]ProductionTask, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	query := runtime.store.DB().WithContext(ctx).Where("project_id=?", runtime.projectID)
	if status != "" {
		if !validStatus(status) {
			return nil, taskError(CodeInvalidTask, "任务状态无效", "status 不受支持。", nil)
		}
		query = query.Where("status=?", status)
	}
	var rows []productionTaskRecord
	if err := query.Order("created_at DESC,id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]ProductionTask, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.DTO())
	}
	return items, nil
}
func (manager *Manager) GetProductionTask(ctx context.Context, projectUUID, taskUUID string) (ProductionTask, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	record, err := getProductionTaskRecord(ctx, runtime.store.DB(), runtime.projectID, taskUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	return record.DTO(), nil
}

func (manager *Manager) ListProductionTaskEvents(ctx context.Context, projectUUID, taskUUID string, before, after int64, limit int) ([]TaskEvent, CursorPagination, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return nil, CursorPagination{}, err
	}
	record, err := getProductionTaskRecord(ctx, runtime.store.DB(), runtime.projectID, taskUUID)
	if err != nil {
		return nil, CursorPagination{}, err
	}
	if before < 0 || after < 0 || (before > 0 && after > 0) {
		return nil, CursorPagination{}, taskError(CodeInvalidTask, "生产事件游标无效", "before 与 after 必须是非负 sequence，且不能同时使用。", nil)
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := runtime.store.DB().WithContext(ctx).Where("production_task_run_id = ?", record.ID)
	order := "sequence ASC"
	if before > 0 {
		query = query.Where("sequence < ?", before)
		order = "sequence DESC"
	} else if after > 0 {
		query = query.Where("sequence > ?", after)
	}
	var rows []productionTaskEventRecord
	if err := query.Order(order).Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, CursorPagination{}, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	if before > 0 {
		for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
			rows[left], rows[right] = rows[right], rows[left]
		}
	}
	items := make([]TaskEvent, 0, len(rows))
	for _, row := range rows {
		items = append(items, TaskEvent{UUID: row.UUID, Sequence: row.Sequence, EventType: row.EventType, Payload: json.RawMessage(row.Payload), CreatedAt: row.CreatedAt})
	}
	var next, previous *string
	if len(items) > 0 {
		first := fmt.Sprintf("%d", items[0].Sequence)
		last := fmt.Sprintf("%d", items[len(items)-1].Sequence)
		if before > 0 {
			next = &last
			if hasMore {
				previous = &first
			}
		} else {
			if hasMore {
				next = &last
			}
			if after > 0 {
				previous = &first
			}
		}
	}
	return items, CursorPagination{PerPage: limit, NextCursor: next, PrevCursor: previous, HasMore: hasMore}, nil
}

func effectiveProductionStyle(ctx context.Context, runtime *projectRuntime, storyService *story.Service, legacyStyle, language string) (string, error) {
	style, err := storyService.EffectivePrompt(ctx, promptcatalog.GroupPremiseStyle, "project_overall_style")
	if err != nil {
		return "", err
	}
	var canonicalVersions int64
	if err := runtime.store.DB().WithContext(ctx).Table("project_prompt_versions").Where("project_id=? AND prompt_group=? AND prompt_key=?", runtime.projectID, promptcatalog.GroupPremiseStyle, "project_overall_style").Count(&canonicalVersions).Error; err != nil {
		return "", err
	}
	legacyStyle = strings.TrimSpace(legacyStyle)
	if canonicalVersions == 0 && legacyStyle != "" && legacyStyle != strings.TrimSpace(production.DefaultPremiseStyle(language)) {
		return legacyStyle, nil
	}
	return style, nil
}

func (manager *Manager) CancelProductionTask(ctx context.Context, projectUUID, taskUUID string) (ProductionTask, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	runtime.cancelWork(taskUUID)
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return ProductionTask{}, err
	}
	defer tx.Rollback()
	record, found, err := findProductionByUUIDTx(ctx, tx, runtime.projectID, taskUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	if !found {
		return ProductionTask{}, taskError(CodeTaskNotFound, "任务不存在", "生产任务不存在。", nil)
	}
	if record.Status == StatusCompleted || record.Status == StatusCancelled {
		_ = tx.Commit()
		return record.DTO(), nil
	}
	if record.RiverJobID == nil {
		return ProductionTask{}, taskError(CodeTaskPersistenceFailed, "任务缺少 River job", "无法安全取消。", nil)
	}
	now := manager.now().UTC()
	if _, err := runtime.client.JobCancelTx(ctx, tx, *record.RiverJobID); err != nil {
		return ProductionTask{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE production_task_runs SET status='cancelled',cancel_requested_at=?,completed_at=?,updated_at=? WHERE id=?`, now, now, now, record.ID); err != nil {
		return ProductionTask{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE premise_generation_steps SET status='cancelled',completed_at=? WHERE task_uuid=? AND status<>'completed'`, now, taskUUID); err != nil {
		return ProductionTask{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE comic_image_generations SET status='cancelled',completed_at=? WHERE task_uuid=? AND status<>'completed'`, now, taskUUID); err != nil {
		return ProductionTask{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE comic_exports SET status='cancelled',completed_at=?,expires_at=? WHERE task_uuid=? AND status IN ('queued','running')`, now, production.ExportExpiresAt(now), taskUUID); err != nil {
		return ProductionTask{}, err
	}
	if err := appendProductionEventTx(ctx, tx, record.ID, "task_cancelled", map[string]any{"project_uuid": projectUUID, "task_uuid": taskUUID, "resource_uuid": record.ResourceUUID, "status": StatusCancelled}, now); err != nil {
		return ProductionTask{}, err
	}
	if err := cancelComicWorkflowTx(ctx, tx, taskUUID, now); err != nil {
		return ProductionTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProductionTask{}, err
	}
	task, err := manager.GetProductionTask(ctx, projectUUID, taskUUID)
	if err == nil {
		runtime.broadcastProduction("production_task:cancelled", task)
		runtime.broadcastComicWorkflow("workflow:cancelled", taskUUID)
	}
	return task, err
}
func (manager *Manager) RetryProductionTask(ctx context.Context, projectUUID, taskUUID string) (ProductionTask, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	current, err := getProductionTaskRecord(ctx, runtime.store.DB(), runtime.projectID, taskUUID)
	if err != nil {
		return ProductionTask{}, err
	}
	if current.Status != StatusFailed && current.Status != StatusInterrupted && current.Status != StatusCancelled {
		return ProductionTask{}, taskError(CodeTaskStateConflict, "任务不能重试", "只有 failed、interrupted 或 cancelled 任务可重试。", nil)
	}
	if current.RiverJobID == nil {
		return ProductionTask{}, taskError(CodeTaskPersistenceFailed, "任务缺少 River job", "无法安全重试。", nil)
	}
	if err := waitProductionJobRetryable(ctx, runtime, *current.RiverJobID); err != nil {
		return ProductionTask{}, err
	}
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return ProductionTask{}, err
	}
	defer tx.Rollback()
	record, found, err := findProductionByUUIDTx(ctx, tx, runtime.projectID, taskUUID)
	if err != nil || !found {
		if err == nil {
			err = taskError(CodeTaskNotFound, "任务不存在", "生产任务不存在。", nil)
		}
		return ProductionTask{}, err
	}
	if record.Status != StatusFailed && record.Status != StatusInterrupted && record.Status != StatusCancelled {
		return ProductionTask{}, taskError(CodeTaskStateConflict, "任务不能重试", "只有 failed、interrupted 或 cancelled 任务可重试。", nil)
	}
	if record.RiverJobID == nil {
		return ProductionTask{}, taskError(CodeTaskPersistenceFailed, "任务缺少 River job", "无法安全重试。", nil)
	}
	now := manager.now().UTC()
	if record.Kind == KindComicExport {
		var retained bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM comic_exports WHERE task_uuid=? AND status IN ('failed','cancelled') AND expires_at>?)`, taskUUID, now).Scan(&retained); err != nil {
			return ProductionTask{}, err
		}
		if !retained {
			return ProductionTask{}, taskError(CodeTaskStateConflict, "导出已过期", "失败或取消的导出只保留 7 天；请创建新的导出任务。", nil)
		}
	}
	if _, err := runtime.client.JobRetryTx(ctx, tx, *record.RiverJobID); err != nil {
		return ProductionTask{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE production_task_runs SET status='queued',progress=0,error_code='',error_message='',cancel_requested_at=NULL,completed_at=NULL,updated_at=? WHERE id=?`, now, record.ID); err != nil {
		return ProductionTask{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE premise_generation_steps SET status='queued',error_code='',completed_at=NULL WHERE task_uuid=? AND status IN ('failed','cancelled')`, taskUUID); err != nil {
		return ProductionTask{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE comic_image_generations SET status='queued',error_code='',completed_at=NULL WHERE task_uuid=? AND status IN ('failed','cancelled')`, taskUUID); err != nil {
		return ProductionTask{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE comic_exports SET status='queued',error_code='',completed_at=NULL,expires_at=NULL WHERE task_uuid=? AND status IN ('failed','cancelled')`, taskUUID); err != nil {
		return ProductionTask{}, err
	}
	if err := appendProductionEventTx(ctx, tx, record.ID, "retry_requested", map[string]any{"project_uuid": projectUUID, "task_uuid": taskUUID, "resource_uuid": record.ResourceUUID, "status": StatusQueued}, now); err != nil {
		return ProductionTask{}, err
	}
	if err := queueComicWorkflowTx(ctx, tx, taskUUID, now); err != nil {
		return ProductionTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProductionTask{}, err
	}
	task, err := manager.GetProductionTask(ctx, projectUUID, taskUUID)
	if err == nil {
		runtime.broadcastProduction("production_task:queued", task)
		runtime.broadcastComicWorkflow("workflow:queued", taskUUID)
	}
	return task, err
}

func waitProductionJobRetryable(ctx context.Context, runtime *projectRuntime, jobID int64) error {
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for {
		job, err := runtime.client.JobGet(waitCtx, jobID)
		if err != nil {
			return err
		}
		if job.State != rivertype.JobStateRunning {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return taskError(CodeTaskStateConflict, "任务仍在结束上一次执行", "请稍后重试。", waitCtx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func getProductionTaskRecord(ctx context.Context, db *gorm.DB, projectID int64, uuid string) (productionTaskRecord, error) {
	if !isUUIDv7(uuid) {
		return productionTaskRecord{}, taskError(CodeInvalidTask, "任务 UUID 无效", "task_uuid 必须是 UUIDv7。", nil)
	}
	var row productionTaskRecord
	err := db.WithContext(ctx).Where("project_id=? AND uuid=?", projectID, uuid).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, taskError(CodeTaskNotFound, "任务不存在", "生产任务不存在。", err)
	}
	return row, err
}

const productionSelect = `SELECT id,uuid,project_id,river_job_id,kind,resource_uuid,input_snapshot,status,idempotency_key,provider_uuid,model,model_source,progress,attempt,max_attempts,error_code,error_message,cancel_requested_at,started_at,completed_at,created_at,updated_at FROM production_task_runs`

func findProductionTaskTx(ctx context.Context, tx *sql.Tx, projectID int64, kind, key string) (productionTaskRecord, bool, error) {
	return scanProduction(tx.QueryRowContext(ctx, productionSelect+` WHERE project_id=? AND kind=? AND idempotency_key=? LIMIT 1`, projectID, kind, key))
}
func findProductionByUUIDTx(ctx context.Context, tx *sql.Tx, projectID int64, uuid string) (productionTaskRecord, bool, error) {
	return scanProduction(tx.QueryRowContext(ctx, productionSelect+` WHERE project_id=? AND uuid=? LIMIT 1`, projectID, uuid))
}
func scanProduction(row rowScanner) (productionTaskRecord, bool, error) {
	var r productionTaskRecord
	err := row.Scan(&r.ID, &r.UUID, &r.ProjectID, &r.RiverJobID, &r.Kind, &r.ResourceUUID, &r.InputSnapshot, &r.Status, &r.IdempotencyKey, &r.ProviderUUID, &r.Model, &r.ModelSource, &r.Progress, &r.Attempt, &r.MaxAttempts, &r.ErrorCode, &r.ErrorMessage, &r.CancelRequestedAt, &r.StartedAt, &r.CompletedAt, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}
func appendProductionEventTx(ctx context.Context, tx *sql.Tx, taskID int64, eventType string, payload any, now time.Time) error {
	uuid, err := newUUIDv7()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO production_task_events(uuid,production_task_run_id,sequence,event_type,payload,created_at) SELECT ?,?,COALESCE(MAX(sequence),0)+1,?,?,? FROM production_task_events WHERE production_task_run_id=?`, uuid, taskID, eventType, string(encoded), now, taskID)
	return err
}
func (runtime *projectRuntime) broadcastProduction(event string, task ProductionTask) {
	if runtime.manager.hub == nil {
		return
	}
	runtime.manager.hub.Broadcast("project:"+runtime.projectUUID, event, map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": task.UUID, "kind": task.Kind, "resource_uuid": task.ResourceUUID, "status": task.Status, "progress": task.Progress, "attempt": task.Attempt, "error_code": task.ErrorCode, "error_message": task.ErrorMessage})
}
