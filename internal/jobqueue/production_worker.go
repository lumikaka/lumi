package jobqueue

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"strings"

	"lumi/internal/files"
	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/llmlog"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/provider"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const (
	defaultComicImageSize = "1024x1536"
	bailianComicImageSize = "768x2304"
)

type productionWorker struct {
	river.WorkerDefaults[productionArgs]
	runtime *projectRuntime
}

func (worker *productionWorker) Work(ctx context.Context, job *river.Job[productionArgs]) error {
	runtime := worker.runtime
	workCtx, cancel := context.WithCancel(ctx)
	runtime.registerWork(job.Args.TaskUUID, cancel)
	defer func() { cancel(); runtime.unregisterWork(job.Args.TaskUUID) }()
	if job.Args.Version != 1 || job.Args.ProjectUUID != runtime.projectUUID || !validProductionKind(job.Args.TaskKind) || !isUUIDv7(job.Args.TaskUUID) || !isUUIDv7(job.Args.ResourceUUID) {
		return river.JobCancel(taskError(CodeInvalidTask, "River 生产任务参数无效", "任务参数版本或 UUID 不受支持。", nil))
	}
	record, err := getProductionTaskRecord(workCtx, runtime.store.DB(), runtime.projectID, job.Args.TaskUUID)
	if err != nil {
		return err
	}
	if record.Status == StatusCompleted || record.Status == StatusCancelled {
		return nil
	}
	var snapshot production.GenerationSnapshot
	if err := json.Unmarshal([]byte(record.InputSnapshot), &snapshot); err != nil {
		_ = runtime.failProduction(context.WithoutCancel(ctx), record, "invalid_input_snapshot", "生产输入快照损坏。", job.Attempt)
		return river.JobCancel(errors.New("production snapshot mismatch"))
	}
	validVersion := snapshot.Version == 1 || snapshot.Version == 2 || ((snapshot.Version == 3 || snapshot.Version == 4) && snapshot.Kind == KindComicImageGeneration)
	if !validVersion || snapshot.Kind != record.Kind || snapshot.ResourceUUID != record.ResourceUUID {
		_ = runtime.failProduction(context.WithoutCancel(ctx), record, "invalid_input_snapshot", "生产输入快照损坏。", job.Attempt)
		return river.JobCancel(errors.New("production snapshot mismatch"))
	}
	if err := runtime.markProductionRunning(workCtx, record, job.Attempt); err != nil {
		return err
	}
	record.Attempt = job.Attempt
	service := production.NewService(runtime.store, runtime.manager.hub)
	var workErr error
	switch record.Kind {
	case KindPremiseSettingGeneration:
		workErr = runtime.generateSetting(workCtx, service, record, snapshot)
	case KindPremiseAssetBreakdown:
		workErr = runtime.breakdownSetting(workCtx, service, record, snapshot)
	case KindPremiseAssetGeneration:
		workErr = runtime.generatePremiseAsset(workCtx, service, record, snapshot)
	case KindComicImageGeneration:
		workErr = runtime.generateComicImage(workCtx, service, record, snapshot)
	case KindComicExport:
		workErr = runtime.renderExport(workCtx, service, record)
	}
	if workErr != nil {
		code, message, retryable, cancelled := classifyProductionError(workErr)
		if cancelled {
			requested, checkErr := runtime.productionCancelRequested(context.WithoutCancel(ctx), record.ID)
			if checkErr == nil && !requested {
				_ = runtime.pauseProduction(context.WithoutCancel(ctx), record, job.Attempt)
				return workErr
			}
			_ = runtime.cancelProductionProjection(context.WithoutCancel(ctx), record)
			return workErr
		}
		_ = runtime.failProduction(context.WithoutCancel(ctx), record, code, message, job.Attempt)
		if !retryable {
			return river.JobCancel(workErr)
		}
		return workErr
	}
	return runtime.completeProduction(workCtx, record)
}

func (runtime *projectRuntime) generateSetting(ctx context.Context, service *production.Service, record productionTaskRecord, snapshot production.GenerationSnapshot) error {
	var done bool
	_ = runtime.store.DB().WithContext(ctx).Raw(`SELECT status='completed' FROM premise_generation_steps WHERE task_uuid=?`, record.UUID).Scan(&done).Error
	if done {
		return nil
	}
	resolved, err := runtime.manager.providers.Resolve(ctx, snapshot.ProviderUUID)
	if err != nil {
		return err
	}
	prompt := strings.TrimSpace(snapshot.Prompt)
	response, err := runtime.callProductionImage(ctx, record, snapshot, resolved, KindPremiseSettingGeneration, imagegen.Request{ProviderType: snapshot.ProviderType, BaseURL: snapshot.ProviderBaseURL, APIKey: resolved.APIKey, Model: snapshot.Model, Prompt: prompt, Size: "1536x1024"})
	if err != nil {
		return err
	}
	if err := runtime.productionProgress(ctx, record, 75); err != nil {
		return err
	}
	setting, err := service.CommitGeneratedSettingImage(ctx, record.UUID, snapshot.SourceUUID, prompt, bytes.NewReader(response.Bytes))
	if err != nil {
		return err
	}
	now := runtime.manager.now().UTC()
	return runtime.store.DB().WithContext(ctx).Exec(`UPDATE premise_generation_steps SET status='completed',setting_image_id=(SELECT id FROM premise_setting_images WHERE uuid=?),output_json=json_object('setting_image_uuid',?),completed_at=? WHERE task_uuid=?`, setting.UUID, setting.UUID, now, record.UUID).Error
}

func (runtime *projectRuntime) generatePremiseAsset(ctx context.Context, service *production.Service, record productionTaskRecord, snapshot production.GenerationSnapshot) error {
	if _, found, err := service.PremiseAssetForGenerationTask(ctx, record.UUID); err != nil {
		return err
	} else if found {
		return nil
	}
	resolved, err := runtime.manager.providers.Resolve(ctx, snapshot.ProviderUUID)
	if err != nil {
		return err
	}
	prompt := strings.TrimSpace(snapshot.Prompt)
	// Version 1 snapshots predate the centralized prompt catalog. Preserve their
	// frozen fields for durable retries while keeping the wrapper bilingual;
	// every newly created task uses the catalog-rendered Version 2 prompt above.
	if snapshot.Version == 1 && snapshot.AssetOperation == "create" {
		if promptcatalog.NormalizeLanguage(snapshot.GenerationLanguage) == promptcatalog.LanguageEnglish {
			prompt += fmt.Sprintf("\n\nSetting item type: %s\nSetting item title: %s", snapshot.AssetType, snapshot.AssetTitle)
		} else {
			prompt += fmt.Sprintf("\n\n设定项类型：%s\n设定项标题：%s", snapshot.AssetType, snapshot.AssetTitle)
		}
		if strings.TrimSpace(snapshot.AssetSummary) != "" {
			if promptcatalog.NormalizeLanguage(snapshot.GenerationLanguage) == promptcatalog.LanguageEnglish {
				prompt += "\nSetting item summary: " + strings.TrimSpace(snapshot.AssetSummary)
			} else {
				prompt += "\n设定项简介：" + strings.TrimSpace(snapshot.AssetSummary)
			}
		}
	} else if snapshot.Version == 1 {
		if promptcatalog.NormalizeLanguage(snapshot.GenerationLanguage) == promptcatalog.LanguageEnglish {
			prompt += fmt.Sprintf("\n\nGenerate a new square candidate image for the existing setting item %q while preserving its identity and core visual features.", snapshot.AssetTitle)
		} else {
			prompt += fmt.Sprintf("\n\n为现有设定项“%s”生成新的正方形候选图，保持其身份与核心视觉特征。", snapshot.AssetTitle)
		}
	}
	if snapshot.Version == 1 && strings.TrimSpace(snapshot.StyleSnapshot) != "" {
		if promptcatalog.NormalizeLanguage(snapshot.GenerationLanguage) == promptcatalog.LanguageEnglish {
			prompt += "\n\nVisual style: " + strings.TrimSpace(snapshot.StyleSnapshot)
		} else {
			prompt += "\n\n视觉风格：" + strings.TrimSpace(snapshot.StyleSnapshot)
		}
	}
	if snapshot.Version == 1 {
		if promptcatalog.NormalizeLanguage(snapshot.GenerationLanguage) == promptcatalog.LanguageEnglish {
			prompt += "\n\nGenerate only one square setting image with a clear subject and complete composition."
		} else {
			prompt += "\n\n只生成单个主体明确、构图完整的正方形设定图。"
		}
		prompt += "\n\n" + project.GenerationLanguageVisualInstruction(snapshot.GenerationLanguage)
	}
	response, err := runtime.callProductionImage(ctx, record, snapshot, resolved, KindPremiseAssetGeneration, imagegen.Request{ProviderType: snapshot.ProviderType, BaseURL: snapshot.ProviderBaseURL, APIKey: resolved.APIKey, Model: snapshot.Model, Prompt: prompt, Size: "1024x1024"})
	if err != nil {
		return err
	}
	if err := runtime.productionProgress(ctx, record, 80); err != nil {
		return err
	}
	if snapshot.AssetOperation == "variant" {
		_, err = service.CommitAIGeneratedPremiseAssetVariant(ctx, record.UUID, snapshot.ResourceUUID, snapshot.AssetRevision, bytes.NewReader(response.Bytes))
		return err
	}
	_, err = service.CommitAIGeneratedPremiseAsset(ctx, record.UUID, production.CreateAssetInput{AssetType: snapshot.AssetType, Title: snapshot.AssetTitle, Summary: snapshot.AssetSummary, Tags: snapshot.AssetTags, SourceType: "ai_asset_thread"}, bytes.NewReader(response.Bytes))
	return err
}

type breakdownItem struct {
	Type       string   `json:"type"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Tags       []string `json:"tags"`
	X          float64  `json:"x"`
	Y          float64  `json:"y"`
	Width      float64  `json:"width"`
	Height     float64  `json:"height"`
	Confidence float64  `json:"confidence"`
	CropBox    struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"crop_box"`
}

func (runtime *projectRuntime) breakdownSetting(ctx context.Context, service *production.Service, record productionTaskRecord, snapshot production.GenerationSnapshot) error {
	var done bool
	_ = runtime.store.DB().WithContext(ctx).Raw(`SELECT status='completed' FROM premise_generation_steps WHERE task_uuid=?`, record.UUID).Scan(&done).Error
	if done {
		return nil
	}
	images, err := service.ListSettingImages(ctx)
	if err != nil {
		return err
	}
	var setting *production.SettingImage
	for index := range images {
		if images[index].UUID == snapshot.ResourceUUID {
			setting = &images[index]
			break
		}
	}
	if setting == nil {
		return productionError("setting_not_found", "设置图不存在", false)
	}
	resolved, err := runtime.manager.providers.Resolve(ctx, snapshot.ProviderUUID)
	if err != nil {
		return err
	}
	var parameters GenerationParameters
	if len(snapshot.Parameters) > 0 {
		if err := json.Unmarshal(snapshot.Parameters, &parameters); err != nil {
			return productionError("invalid_input_snapshot", "生成参数快照损坏", false)
		}
	}
	content, err := service.Files().OpenContent(ctx, setting.Asset.UUID)
	if err != nil {
		return err
	}
	imageBytes, err := io.ReadAll(io.LimitReader(content.File, 64<<20+1))
	content.File.Close()
	if err != nil || len(imageBytes) == 0 || len(imageBytes) > 64<<20 {
		return productionError("invalid_setting_image", "设置图为空、过大或无法读取", false)
	}
	source, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return productionError("invalid_setting_image", "设置图无法解码", false)
	}
	imageInfo := map[string]any{
		"filename": content.Filename, "mime_type": content.Asset.MIMEType, "byte_size": len(imageBytes),
		"width": source.Bounds().Dx(), "height": source.Bounds().Dy(),
	}
	breakdownTemplate := snapshot.Prompt
	if snapshot.Version == 1 && !strings.Contains(breakdownTemplate, "{{input_text}}") && !strings.Contains(breakdownTemplate, "{{style_prompt}}") {
		breakdownTemplate += "\n\n本地图像元信息：\n{{image_info_json}}\n\n画风：\n{{style_prompt}}\n\nPremise 文本：\n{{input_text}}"
	}
	analysisPrompt := renderPremiseAssetBreakdownPrompt(breakdownTemplate, snapshot.SourceText, snapshot.StyleSnapshot, snapshot.GenerationLanguage, snapshot.LanguageInstruction, imageInfo)
	response, err := runtime.callProductionText(ctx, record, snapshot, resolved, KindPremiseAssetBreakdown, llm.Request{
		BaseURL: snapshot.ProviderBaseURL, APIKey: resolved.APIKey, Model: snapshot.Model,
		Prompt: analysisPrompt, Images: []llm.ImageInput{{MIMEType: content.Asset.MIMEType, Data: imageBytes, Detail: "high"}},
		Temperature: parameters.Temperature, MaxTokens: parameters.MaxTokens,
	})
	if err != nil {
		return err
	}
	items, err := parseBreakdown(response.Content)
	if err != nil {
		return err
	}
	created := make([]string, 0, len(items))
	for index, item := range items {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		crop, box, err := cropImage(source, item)
		if err != nil {
			return err
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, crop); err != nil {
			return err
		}
		asset, err := service.CommitGeneratedPremiseAsset(ctx, record.UUID, setting.UUID, production.CreateAssetInput{AssetType: item.Type, Title: item.Title, Summary: item.Summary, Tags: item.Tags, Position: map[string]any{"x": item.X, "y": item.Y}, Crop: map[string]any{"x": box.Min.X, "y": box.Min.Y, "width": box.Dx(), "height": box.Dy()}, SourceType: "breakdown"}, bytes.NewReader(encoded.Bytes()))
		if err != nil {
			return err
		}
		created = append(created, asset.UUID)
		_ = runtime.productionProgress(ctx, record, 20+((index+1)*65/len(items)))
	}
	output, _ := json.Marshal(map[string]any{"premise_asset_uuids": created})
	now := runtime.manager.now().UTC()
	return runtime.store.DB().WithContext(ctx).Exec(`UPDATE premise_generation_steps SET status='completed',output_json=?,completed_at=? WHERE task_uuid=?`, string(output), now, record.UUID).Error
}

func (runtime *projectRuntime) generateComicImage(ctx context.Context, service *production.Service, record productionTaskRecord, snapshot production.GenerationSnapshot) error {
	var status, generationUUID string
	_ = runtime.store.DB().WithContext(ctx).Raw(`SELECT status,uuid FROM comic_image_generations WHERE task_uuid=?`, record.UUID).Row().Scan(&status, &generationUUID)
	if status == "completed" {
		return nil
	}
	resolved, err := runtime.manager.providers.Resolve(ctx, snapshot.ProviderUUID)
	if err != nil {
		return err
	}
	selection := sectionReferenceSelection{References: snapshot.PremiseAssets}
	if snapshot.Version >= 2 && len(selection.References) == 0 && len(snapshot.PremiseCandidates) > 0 {
		selection, err = runtime.selectSectionReferences(ctx, resolved, record, snapshot)
		if err != nil {
			return err
		}
	}
	if err := markComicReferencesSelected(ctx, runtime, record, selection); err != nil {
		return err
	}
	references := selection.References
	if len(references) > maxSectionPremiseAssets {
		return productionError("too_many_section_premise_assets", fmt.Sprintf("Section 设定参考图最多允许 %d 个", maxSectionPremiseAssets), false)
	}
	prompt := ""
	if snapshot.Version >= 2 && strings.TrimSpace(snapshot.PromptTemplate) != "" {
		referenceUsage := sectionReferenceUsage(references, snapshot.GenerationLanguage)
		if snapshot.Version >= 3 {
			referenceUsage, err = renderSectionReferenceUsage(references, snapshot.ReferencePresentPrompt, snapshot.ReferenceAbsentPrompt)
			if err != nil {
				return productionError("invalid_prompt_snapshot", "Section 参考图说明提示词快照无法渲染", false)
			}
		}
		prompt, err = promptcatalog.Render(snapshot.PromptTemplate, map[string]string{
			"style_prompt": snapshot.StyleSnapshot, "reference_usage_text": referenceUsage,
			"section_id": snapshot.ResourceUUID, "storyboard": snapshot.StoryboardMD,
		})
		if err != nil {
			return productionError("invalid_prompt_snapshot", "Section 图片提示词快照无法渲染", false)
		}
		languageInstruction := snapshot.LanguageInstruction
		if strings.TrimSpace(languageInstruction) == "" {
			languageInstruction = promptcatalog.LanguageInstruction(snapshot.GenerationLanguage)
		}
		prompt = promptcatalog.WithInstruction(prompt, languageInstruction)
		if guidance := strings.TrimSpace(snapshot.Prompt); guidance != "" && guidance != strings.TrimSpace(snapshot.StoryboardMD) {
			if snapshot.Version >= 3 {
				additional, renderErr := promptcatalog.Render(snapshot.AdditionalDirectionPrompt, map[string]string{"guidance_prompt": guidance})
				if renderErr != nil {
					return productionError("invalid_prompt_snapshot", "Section 用户补充要求提示词快照无法渲染", false)
				}
				prompt += "\n\n" + additional
			} else if promptcatalog.NormalizeLanguage(snapshot.GenerationLanguage) == promptcatalog.LanguageEnglish {
				prompt += "\n\n## Additional user direction\n" + guidance
			} else {
				prompt += "\n\n## 用户补充要求\n" + guidance
			}
		}
	} else {
		// Version 1 Comic snapshots store the pre-catalog prompt pieces rather
		// than a rendered template; keep this deterministic compatibility path
		// solely for retries of those already-durable tasks.
		prompt = strings.TrimSpace(snapshot.Prompt + "\n\nStoryboard:\n" + snapshot.StoryboardMD + "\n\nStyle:\n" + snapshot.StyleSnapshot + "\n\n" + project.GenerationLanguageVisualInstruction(snapshot.GenerationLanguage))
		if len(references) > 0 {
			prompt += fmt.Sprintf("\n\nUse the single Section setting collage to keep visual continuity with its %d labeled premise items.", len(references))
		}
	}
	referenceImages := []imagegen.ImageInput{}
	if len(references) > 0 {
		premise, data, premiseErr := runtime.prepareSectionPremise(ctx, service, record, generationUUID, snapshot.ResourceUUID, selection)
		if premiseErr != nil {
			return premiseErr
		}
		if premise == nil || len(data) == 0 {
			return productionError("invalid_section_premise", "Section 设定参考合图为空或无法读取", false)
		}
		if err := markComicPremiseSaved(ctx, runtime, record, premise.Asset.UUID); err != nil {
			return err
		}
		referenceImages = append(referenceImages, imagegen.ImageInput{MIMEType: "image/png", Data: data})
	}
	imageSize := comicImageSize(snapshot.ProviderType)
	if snapshot.Version >= 4 && strings.TrimSpace(snapshot.OutputSize) != "" {
		imageSize = snapshot.OutputSize
	}
	response, err := runtime.callProductionImage(ctx, record, snapshot, resolved, KindComicImageGeneration, imagegen.Request{ProviderType: snapshot.ProviderType, BaseURL: snapshot.ProviderBaseURL, APIKey: resolved.APIKey, Model: snapshot.Model, Prompt: prompt, Size: imageSize, Images: referenceImages})
	if err != nil {
		return err
	}
	if err := markComicImageGenerated(ctx, runtime, record); err != nil {
		return err
	}
	if err := runtime.productionProgress(ctx, record, 80); err != nil {
		return err
	}
	imageVariant, err := service.CommitGeneratedSectionImage(ctx, snapshot.ChapterUUID, snapshot.ResourceUUID, generationUUID, json.RawMessage(record.InputSnapshot), bytes.NewReader(response.Bytes))
	if err != nil {
		return err
	}
	return markComicImageSaved(ctx, runtime, record, imageVariant.UUID)
}

func comicImageSize(providerType string) string {
	if providerType == provider.TypeAliyunBailian {
		return bailianComicImageSize
	}
	return defaultComicImageSize
}

type sectionReferenceSelection struct {
	References []production.PremiseAssetReference
	Reason     string
}

func (runtime *projectRuntime) selectSectionReferences(ctx context.Context, resolved provider.Resolved, record productionTaskRecord, snapshot production.GenerationSnapshot) (sectionReferenceSelection, error) {
	var persisted string
	if err := runtime.sqlDB.QueryRowContext(ctx, `SELECT payload FROM production_task_events WHERE production_task_run_id=? AND event_type='section_references_selected' ORDER BY sequence DESC LIMIT 1`, record.ID).Scan(&persisted); err == nil {
		var selection struct {
			SectionUUID     string   `json:"section_uuid"`
			Titles          []string `json:"titles"`
			SelectionReason string   `json:"selection_reason"`
		}
		if json.Unmarshal([]byte(persisted), &selection) == nil && selection.SectionUUID == snapshot.ResourceUUID {
			references, freezeErr := frozenSectionReferences(snapshot.PremiseCandidates, selection.Titles)
			return sectionReferenceSelection{References: references, Reason: selection.SelectionReason}, freezeErr
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return sectionReferenceSelection{}, err
	}
	response, err := runtime.callProductionText(ctx, record, snapshot, resolved, "comic_reference_selection", llm.Request{BaseURL: snapshot.SelectionBaseURL, APIKey: resolved.APIKey, Model: snapshot.SelectionModel, Prompt: snapshot.SelectionPrompt})
	if err != nil {
		return sectionReferenceSelection{}, err
	}
	var selection struct {
		SectionID string   `json:"sectionId"`
		Titles    []string `json:"titles"`
		Reason    string   `json:"reason"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(response.Content)), &selection); err != nil || selection.SectionID != snapshot.ResourceUUID || len(selection.Titles) > maxSectionPremiseAssets {
		return sectionReferenceSelection{}, productionError("invalid_section_reference_selection", "模型未返回有效的 Section 设定项选择 JSON", false)
	}
	result, err := frozenSectionReferences(snapshot.PremiseCandidates, selection.Titles)
	if err != nil {
		return sectionReferenceSelection{}, err
	}
	if err := runtime.appendProductionEvent(ctx, record.ID, "section_references_selected", map[string]any{"section_uuid": snapshot.ResourceUUID, "titles": selection.Titles, "selection_reason": strings.TrimSpace(selection.Reason)}); err != nil {
		return sectionReferenceSelection{}, err
	}
	return sectionReferenceSelection{References: result, Reason: strings.TrimSpace(selection.Reason)}, nil
}

func (runtime *projectRuntime) callProductionText(ctx context.Context, record productionTaskRecord, snapshot production.GenerationSnapshot, resolved provider.Resolved, scenario string, request llm.Request) (llm.Response, error) {
	requestPayload, err := llmlog.EncodeTextRequest(request)
	if err != nil {
		return llm.Response{}, err
	}
	handle, err := llmlog.Begin(ctx, runtime.store, llmlog.StartInput{
		ProjectID: runtime.projectID, ProductionTaskRunID: record.ID,
		SourceType: llmlog.SourceProduction, Scenario: scenario, RequestType: llmlog.RequestText, Attempt: record.Attempt,
		ProviderUUID: snapshot.ProviderUUID, ProviderType: resolved.ProviderType, Model: request.Model, InputSummary: request.Prompt,
		RequestPayload: requestPayload,
	})
	if err != nil {
		return llm.Response{}, err
	}
	response, callErr := runtime.manager.llm.Generate(ctx, request, nil)
	var responsePayload []byte
	if callErr == nil {
		responsePayload, err = llmlog.EncodeTextResponse(response, request.APIKey)
		if err != nil {
			callErr = err
		}
	}
	finishErr := llmlog.Finish(context.WithoutCancel(ctx), runtime.store, handle, llmlog.FinishInput{
		OutputSummary: response.Content, InputTokens: response.Usage.InputTokens, CachedInputTokens: response.Usage.CachedInputTokens, OutputTokens: response.Usage.OutputTokens,
		FinishReason: response.FinishReason, Response: responsePayload, Err: callErr,
	})
	if finishErr != nil {
		if callErr != nil {
			callErr = errors.Join(callErr, finishErr)
		} else {
			callErr = finishErr
		}
	}
	return response, callErr
}

func (runtime *projectRuntime) callProductionImage(ctx context.Context, record productionTaskRecord, snapshot production.GenerationSnapshot, resolved provider.Resolved, scenario string, request imagegen.Request) (imagegen.Response, error) {
	inputSummary := request.Prompt
	metadata := fmt.Sprintf("[size=%s; reference_images=%d]", request.Size, len(request.Images))
	if strings.TrimSpace(inputSummary) == "" {
		inputSummary = metadata
	} else {
		inputSummary += "\n\n" + metadata
	}
	requestPayload, err := llmlog.EncodeImageRequest(request)
	if err != nil {
		return imagegen.Response{}, err
	}
	handle, err := llmlog.Begin(ctx, runtime.store, llmlog.StartInput{
		ProjectID: runtime.projectID, ProductionTaskRunID: record.ID,
		SourceType: llmlog.SourceProduction, Scenario: scenario, RequestType: llmlog.RequestImage, Attempt: record.Attempt,
		ProviderUUID: snapshot.ProviderUUID, ProviderType: resolved.ProviderType, Model: request.Model, InputSummary: inputSummary,
		RequestPayload: requestPayload,
	})
	if err != nil {
		return imagegen.Response{}, err
	}
	response, callErr := runtime.manager.image.Generate(ctx, request)
	outputSummary := fmt.Sprintf("mime_type=%s; byte_size=%d", response.MIMEType, len(response.Bytes))
	if strings.TrimSpace(response.RevisedPrompt) != "" {
		outputSummary += "\nrevised_prompt=" + response.RevisedPrompt
	}
	var responsePayload []byte
	if callErr == nil {
		responsePayload, err = llmlog.EncodeImageResponse(response, request.APIKey)
		if err != nil {
			callErr = err
		}
	}
	finishErr := llmlog.Finish(context.WithoutCancel(ctx), runtime.store, handle, llmlog.FinishInput{OutputSummary: outputSummary, Response: responsePayload, Err: callErr})
	if finishErr != nil {
		if callErr != nil {
			callErr = errors.Join(callErr, finishErr)
		} else {
			callErr = finishErr
		}
	}
	return response, callErr
}

func frozenSectionReferences(candidates []production.PremiseAssetReference, titles []string) ([]production.PremiseAssetReference, error) {
	byTitle := make(map[string]production.PremiseAssetReference, len(candidates))
	for _, candidate := range candidates {
		byTitle[candidate.Title] = candidate
	}
	result := make([]production.PremiseAssetReference, 0, len(titles))
	seen := make(map[string]struct{}, len(titles))
	for _, title := range titles {
		candidate, ok := byTitle[title]
		if !ok {
			return nil, productionError("invalid_section_reference_selection", "模型选择了候选目录之外的设定项", false)
		}
		if _, duplicate := seen[title]; duplicate {
			continue
		}
		seen[title] = struct{}{}
		result = append(result, candidate)
	}
	return result, nil
}

func sectionReferenceUsage(references []production.PremiseAssetReference, language string) string {
	if len(references) == 0 {
		if promptcatalog.NormalizeLanguage(language) == promptcatalog.LanguageEnglish {
			return "No setting reference images are available for this generation. Generate only from the storyboard and art style, and do not claim consistency with character or scene references that were not provided."
		}
		return "本次没有可用的设定参考图。仅根据 Storyboard 与画风生成，不要声称遵循未提供的角色或场景设定。"
	}
	titles := make([]string, 0, len(references))
	for index, reference := range references {
		titles = append(titles, fmt.Sprintf("%d. %s", index+1, reference.Title))
	}
	if promptcatalog.NormalizeLanguage(language) == promptcatalog.LanguageEnglish {
		return "One Section-specific setting collage image is provided. It contains the following setting items; use their labeled visual references to preserve identity and core visual features:\n" + strings.Join(titles, "\n")
	}
	return "本次提供一张 Section 专属设定拼贴图，其中包含以下带标签的设定项；请依据拼贴图保持它们的身份与核心视觉特征：\n" + strings.Join(titles, "\n")
}

func renderSectionReferenceUsage(references []production.PremiseAssetReference, presentTemplate, absentTemplate string) (string, error) {
	if len(references) == 0 {
		return promptcatalog.Render(absentTemplate, map[string]string{})
	}
	titles := make([]string, 0, len(references))
	for index, reference := range references {
		titles = append(titles, fmt.Sprintf("%d. %s", index+1, reference.Title))
	}
	return promptcatalog.Render(presentTemplate, map[string]string{"reference_titles": strings.Join(titles, "\n")})
}

func (runtime *projectRuntime) appendProductionEvent(ctx context.Context, taskID int64, eventType string, payload any) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := appendProductionEventTx(ctx, tx, taskID, eventType, payload, runtime.manager.now().UTC()); err != nil {
		return err
	}
	return tx.Commit()
}
func (runtime *projectRuntime) renderExport(ctx context.Context, service *production.Service, record productionTaskRecord) error {
	if err := service.MarkExportRunning(ctx, record.UUID); err != nil {
		return err
	}
	if err := runtime.productionProgress(ctx, record, 10); err != nil {
		return err
	}
	_, err := service.RenderAndCommitExport(ctx, record.UUID, func(progress int) error {
		return runtime.productionProgress(ctx, record, progress)
	})
	return err
}

func parseBreakdown(value string) ([]breakdownItem, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "```json")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(value, "```")
	raw := []byte(strings.TrimSpace(value))
	var envelope struct {
		Assets []breakdownItem `json:"assets"`
	}
	var items []breakdownItem
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Assets != nil {
		items = envelope.Assets
	} else if err := json.Unmarshal(raw, &items); err != nil {
		return nil, productionError("invalid_breakdown_response", "模型未返回有效拆分 JSON object", false)
	}
	if len(items) == 0 || len(items) > 16 {
		return nil, productionError("invalid_breakdown_response", "拆分结果数量必须为 1 到 16", false)
	}
	for index := range items {
		item := &items[index]
		if item.CropBox.Width > 0 || item.CropBox.Height > 0 {
			item.X, item.Y, item.Width, item.Height = item.CropBox.X, item.CropBox.Y, item.CropBox.Width, item.CropBox.Height
		}
		item.Type = strings.ToLower(strings.TrimSpace(item.Type))
		if item.Type != "character" && item.Type != "scene" && item.Type != "prop" && item.Type != "reference" {
			item.Type = premiseAssetTypeFromTags(item.Tags)
		}
		item.Title = strings.TrimSpace(item.Title)
		if item.Title == "" {
			item.Title = fmt.Sprintf("asset-%02d", index+1)
		}
		if item.Width <= 0 || item.Height <= 0 || item.X < 0 || item.Y < 0 || item.X+item.Width > 1.0001 || item.Y+item.Height > 1.0001 {
			return nil, productionError("invalid_breakdown_response", "拆分裁剪框越界", false)
		}
	}
	return items, nil
}

func premiseAssetTypeFromTags(tags []string) string {
	if len(tags) == 0 {
		return production.AssetReference
	}
	switch strings.ToLower(strings.TrimSpace(tags[0])) {
	case "角色", "character", "人物", "生物", "creature":
		return production.AssetCharacter
	case "地点", "场景", "place", "scene", "location":
		return production.AssetScene
	case "道具", "prop", "服装", "costume", "载具", "vehicle":
		return production.AssetProp
	default:
		return production.AssetReference
	}
}
func cropImage(source image.Image, item breakdownItem) (image.Image, image.Rectangle, error) {
	bounds := source.Bounds()
	x := bounds.Min.X + int(item.X*float64(bounds.Dx()))
	y := bounds.Min.Y + int(item.Y*float64(bounds.Dy()))
	w := int(item.Width * float64(bounds.Dx()))
	h := int(item.Height * float64(bounds.Dy()))
	box := image.Rect(x, y, x+w, y+h).Intersect(bounds)
	if box.Dx() < 2 || box.Dy() < 2 {
		return nil, box, productionError("invalid_breakdown_crop", "拆分裁剪框过小", false)
	}
	target := image.NewRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	draw.Draw(target, target.Bounds(), source, box.Min, draw.Src)
	return target, box, nil
}

func (runtime *projectRuntime) markProductionRunning(ctx context.Context, record productionTaskRecord, attempt int) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE production_task_runs SET status='running',progress=5,attempt=?,started_at=COALESCE(started_at,?),updated_at=?,error_code='',error_message='' WHERE id=? AND cancel_requested_at IS NULL AND status NOT IN ('completed','cancelled')`, attempt, now, now, record.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return context.Canceled
	}
	if err := appendProductionEventTx(ctx, tx, record.ID, "task_started", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "resource_uuid": record.ResourceUUID, "status": StatusRunning, "progress": 5, "attempt": attempt}, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE premise_generation_steps SET status='running' WHERE task_uuid=? AND status='queued'`, record.UUID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE comic_image_generations SET status='running' WHERE task_uuid=? AND status='queued'`, record.UUID); err != nil {
		return err
	}
	if err := markComicWorkflowRunningTx(ctx, tx, record.UUID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status = StatusRunning
	task.Progress = 5
	task.Attempt = attempt
	task.StartedAt = &now
	task.UpdatedAt = now
	runtime.broadcastProduction("production_task:running", task)
	return nil
}
func (runtime *projectRuntime) productionProgress(ctx context.Context, record productionTaskRecord, progress int) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var cancelled bool
	if err := tx.QueryRowContext(ctx, `SELECT cancel_requested_at IS NOT NULL OR status='cancelled' FROM production_task_runs WHERE id=?`, record.ID).Scan(&cancelled); err != nil {
		return err
	}
	if cancelled {
		return context.Canceled
	}
	now := runtime.manager.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE production_task_runs SET progress=CASE WHEN progress<? THEN ? ELSE progress END,updated_at=? WHERE id=?`, progress, progress, now, record.ID); err != nil {
		return err
	}
	if err := appendProductionEventTx(ctx, tx, record.ID, "task_progress", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "resource_uuid": record.ResourceUUID, "status": StatusRunning, "progress": progress}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status = StatusRunning
	task.Progress = progress
	task.UpdatedAt = now
	runtime.broadcastProduction("production_task:progress", task)
	return nil
}
func (runtime *projectRuntime) completeProduction(ctx context.Context, record productionTaskRecord) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE production_task_runs SET status='completed',progress=100,completed_at=?,updated_at=?,error_code='',error_message='' WHERE id=? AND cancel_requested_at IS NULL AND status<>'cancelled'`, now, now, record.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return context.Canceled
	}
	if err := appendProductionEventTx(ctx, tx, record.ID, "task_completed", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "resource_uuid": record.ResourceUUID, "status": StatusCompleted, "progress": 100}, now); err != nil {
		return err
	}
	if err := completeComicWorkflowTx(ctx, tx, record.UUID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status = StatusCompleted
	task.Progress = 100
	task.CompletedAt = &now
	task.UpdatedAt = now
	runtime.broadcastProduction("production_task:completed", task)
	runtime.broadcastComicWorkflow("workflow:step_changed", record.UUID)
	if runtime.manager.hub != nil {
		runtime.manager.hub.Broadcast("project:"+runtime.projectUUID, "production:resource_changed", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "resource_uuid": record.ResourceUUID, "kind": record.Kind})
	}
	return nil
}
func (runtime *projectRuntime) failProduction(ctx context.Context, record productionTaskRecord, code, message string, attempt int) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE production_task_runs SET status='failed',progress=0,attempt=?,error_code=?,error_message=?,completed_at=?,updated_at=? WHERE id=? AND status<>'cancelled'`, attempt, code, message, now, now, record.ID); err != nil {
		return err
	}
	_ = appendProductionEventTx(ctx, tx, record.ID, "task_failed", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "resource_uuid": record.ResourceUUID, "status": StatusFailed, "error_code": code, "error_message": message}, now)
	_, _ = tx.ExecContext(ctx, `UPDATE premise_generation_steps SET status='failed',error_code=?,completed_at=? WHERE task_uuid=? AND status<>'completed'`, code, now, record.UUID)
	_, _ = tx.ExecContext(ctx, `UPDATE comic_image_generations SET status='failed',error_code=?,completed_at=? WHERE task_uuid=? AND status<>'completed'`, code, now, record.UUID)
	_, _ = tx.ExecContext(ctx, `UPDATE comic_exports SET status='failed',error_code=?,completed_at=? WHERE task_uuid=? AND status NOT IN ('ready','cancelled')`, code, now, record.UUID)
	if err := failComicWorkflowTx(ctx, tx, record.UUID, code, message, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status = StatusFailed
	task.ErrorCode = code
	task.ErrorMessage = message
	task.CompletedAt = &now
	task.UpdatedAt = now
	runtime.broadcastProduction("production_task:failed", task)
	runtime.broadcastComicWorkflow("workflow:failed", record.UUID)
	return nil
}
func (runtime *projectRuntime) cancelProductionProjection(ctx context.Context, record productionTaskRecord) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE production_task_runs SET status='cancelled',completed_at=COALESCE(completed_at,?),updated_at=? WHERE id=? AND status<>'completed'`, now, now, record.ID); err != nil {
		return err
	}
	_, _ = tx.ExecContext(ctx, `UPDATE premise_generation_steps SET status='cancelled',completed_at=? WHERE task_uuid=? AND status<>'completed'`, now, record.UUID)
	_, _ = tx.ExecContext(ctx, `UPDATE comic_image_generations SET status='cancelled',completed_at=? WHERE task_uuid=? AND status<>'completed'`, now, record.UUID)
	_, _ = tx.ExecContext(ctx, `UPDATE comic_exports SET status='cancelled',completed_at=? WHERE task_uuid=? AND status<>'ready'`, now, record.UUID)
	if err := cancelComicWorkflowTx(ctx, tx, record.UUID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	runtime.broadcastComicWorkflow("workflow:cancelled", record.UUID)
	return nil
}

func (runtime *projectRuntime) productionCancelRequested(ctx context.Context, taskID int64) (bool, error) {
	var cancelled bool
	err := runtime.sqlDB.QueryRowContext(ctx, `SELECT cancel_requested_at IS NOT NULL OR status='cancelled' FROM production_task_runs WHERE id=?`, taskID).Scan(&cancelled)
	return cancelled, err
}

func (runtime *projectRuntime) pauseProduction(ctx context.Context, record productionTaskRecord, attempt int) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE production_task_runs SET status='queued',progress=0,attempt=?,updated_at=?,error_code='',error_message='' WHERE id=? AND status='running' AND cancel_requested_at IS NULL`, attempt, now, record.ID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return context.Canceled
	}
	if _, err := tx.ExecContext(ctx, `UPDATE premise_generation_steps SET status='queued' WHERE task_uuid=? AND status='running'`, record.UUID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE comic_image_generations SET status='queued' WHERE task_uuid=? AND status='running'`, record.UUID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE comic_exports SET status='queued' WHERE task_uuid=? AND status='running'`, record.UUID); err != nil {
		return err
	}
	if err := appendProductionEventTx(ctx, tx, record.ID, "task_paused", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "resource_uuid": record.ResourceUUID, "status": StatusQueued, "attempt": attempt, "reason": "runtime_stopped"}, now); err != nil {
		return err
	}
	if err := queueComicWorkflowTx(ctx, tx, record.UUID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status = StatusQueued
	task.Progress = 0
	task.Attempt = attempt
	task.UpdatedAt = now
	runtime.broadcastProduction("production_task:queued", task)
	runtime.broadcastComicWorkflow("workflow:step_changed", record.UUID)
	return nil
}

func (runtime *projectRuntime) projectProductionRiverEvent(ctx context.Context, event *river.Event, args productionArgs) error {
	record, err := getProductionTaskRecord(ctx, runtime.store.DB(), runtime.projectID, args.TaskUUID)
	if err != nil {
		return err
	}
	if record.Status == StatusCompleted || record.Status == StatusCancelled || (record.Status == StatusFailed && event.Kind == river.EventKindJobCancelled) {
		return nil
	}
	if event.Kind == river.EventKindJobFailed && event.Job.State != rivertype.JobStateDiscarded {
		tx, err := runtime.sqlDB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		now := runtime.manager.now().UTC()
		result, err := tx.ExecContext(ctx, `UPDATE production_task_runs SET status='queued',progress=0,completed_at=NULL,updated_at=? WHERE id=? AND status='failed'`, now, record.ID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE premise_generation_steps SET status='queued',error_code='',completed_at=NULL WHERE task_uuid=? AND status='failed'`, record.UUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE comic_image_generations SET status='queued',error_code='',completed_at=NULL WHERE task_uuid=? AND status='failed'`, record.UUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE comic_exports SET status='queued',error_code='',completed_at=NULL WHERE task_uuid=? AND status='failed'`, record.UUID); err != nil {
			return err
		}
		if err := appendProductionEventTx(ctx, tx, record.ID, "retry_scheduled", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "resource_uuid": record.ResourceUUID, "status": StatusQueued, "attempt": event.Job.Attempt}, now); err != nil {
			return err
		}
		if err := queueComicWorkflowTx(ctx, tx, record.UUID, now); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		task := record.DTO()
		task.Status = StatusQueued
		task.Progress = 0
		task.Attempt = event.Job.Attempt
		task.UpdatedAt = now
		runtime.broadcastProduction("production_task:queued", task)
		runtime.broadcastComicWorkflow("workflow:step_changed", record.UUID)
		return nil
	}
	if event.Kind == river.EventKindJobCancelled {
		// Non-retryable worker errors persist a failed task before River records
		// the cancelled job. An explicit retry can make the task queued before
		// that older River event is consumed; never let the stale event cancel
		// the newly queued attempt.
		if record.CancelRequestedAt == nil {
			return nil
		}
		return runtime.cancelProductionProjection(ctx, record)
	}
	return nil
}

type productionClassifiedError struct {
	code, message string
	retryable     bool
}

func (err *productionClassifiedError) Error() string { return err.code }
func productionError(code, message string, retryable bool) error {
	return &productionClassifiedError{code: code, message: message, retryable: retryable}
}
func classifyProductionError(err error) (string, string, bool, bool) {
	if errors.Is(err, context.Canceled) {
		return "cancelled", "任务已取消。", false, true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout", "任务超时。", true, false
	}
	var imageErr *imagegen.Error
	if errors.As(err, &imageErr) {
		return imageErr.Code, imageErr.SafeMessage, imageErr.Retryable, imageErr.Code == "image_cancelled"
	}
	var llmErr *llm.Error
	if errors.As(err, &llmErr) {
		return llmErr.Code, llmErr.SafeMessage, llmErr.Retryable, llmErr.Code == llm.CodeCancelled
	}
	var providerErr *provider.Error
	if errors.As(err, &providerErr) {
		return providerErr.Code, providerErr.Message, providerErr.Code == provider.CodeSecretStoreFailed, false
	}
	var fileErr *files.Error
	if errors.As(err, &fileErr) {
		retryable := fileErr.Code == files.CodeOperationUnavailable || fileErr.Code == files.CodeUploadNotReady
		return fileErr.Code, fileErr.Message, retryable, false
	}
	var domainErr *production.Error
	if errors.As(err, &domainErr) {
		return domainErr.Code, domainErr.Message, false, false
	}
	var classified *productionClassifiedError
	if errors.As(err, &classified) {
		return classified.code, classified.message, classified.retryable, false
	}
	return "production_failed", "生产任务执行失败。", true, false
}
func validProductionKind(value string) bool {
	switch value {
	case KindPremiseSettingGeneration, KindPremiseAssetBreakdown, KindPremiseAssetGeneration, KindComicImageGeneration, KindComicExport:
		return true
	}
	return false
}
