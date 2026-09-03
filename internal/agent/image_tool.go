package agent

import (
	"bytes"
	"context"
	"errors"
	"image/gif"
	"image/png"
	"io"
	"math"
	"path/filepath"
	"strings"
	"time"

	"lumi/internal/files"
	"lumi/internal/imagegen"
	"lumi/internal/llmlog"
	"lumi/internal/production"
	"lumi/internal/project"

	"gorm.io/gorm"
)

const (
	imageToolTimeout       = 10 * time.Minute
	maxImageGenReferences  = 4
	imageOperationGenerate = "generate"
	imageOperationEdit     = "edit"
	imageOperationRestyle  = "restyle"
)

func (service *Service) executeImageGenTool(ctx context.Context, store *project.Store, tc toolContext, execution toolExecutionRecord, args map[string]any) (map[string]any, error) {
	if err := store.RequireReady(); err != nil {
		return nil, domainError(project.CodeProjectSetupIncomplete, "项目设置尚未定稿", "draft 阶段禁止调用 image_gen。", err)
	}
	if isBootstrapToolContext(tc) {
		return nil, bootstrapProductionRequiresYoloError()
	}
	if service.image == nil {
		return nil, domainError(CodeStateConflict, "图片生成服务不可用", "Image client 尚未初始化。", nil)
	}
	userPrompt, err := validateText(stringArg(args, "prompt"), 64<<10, "图片 Prompt")
	if err != nil {
		return nil, err
	}
	operation, useDefaultStyle, err := imageGenOptions(tc, args)
	if err != nil {
		return nil, err
	}
	size := stringArg(args, "size")
	quality := stringArg(args, "quality")
	if quality == "" {
		quality = "medium"
	}
	purpose := "project_chat_image_generation"
	legacyV2 := tc.ToolProtocol == ToolProtocolProjectV2 || normalizedToolMode(tc.ToolMode) == ToolModeLegacyTyped
	if !legacyV2 {
		if _, exists := args["reference_file_uuids"]; exists {
			return nil, domainError(CodeToolValidation, "image_gen 参数已过期", "新协议只能通过必填 reference_uuids 选择当前 Thread 中出现过的 Reference。", nil)
		}
		if _, exists := args["reference_uuids"]; !exists {
			return nil, domainError(CodeToolValidation, "image_gen 缺少 Reference 选择", "reference_uuids 为必填数组；不使用参考图时传空数组。", nil)
		}
	}
	if existing, found, err := service.generatedImageForExecution(ctx, store, purpose, execution.UUID); err != nil {
		return nil, err
	} else if found {
		result := imageToolResult(existing, metadataStringSlice(existing.Metadata, "reference_uuids"), metadataStringSlice(existing.Metadata, "resolved_file_uuids"), metadataText(existing.Metadata, "revised_prompt"))
		if tc.ToolProtocol == ToolProtocolProjectAPI {
			result["operation"] = metadataText(existing.Metadata, "operation")
			result["use_default_style"] = metadataBool(existing.Metadata, "use_default_style")
		}
		if legacyV2 {
			result["reference_file_uuids"] = metadataStringSlice(existing.Metadata, "resolved_file_uuids")
		}
		return result, nil
	}

	var selectedReferences []selectedImageReference
	if legacyV2 {
		selectedReferences, err = service.resolveProjectAPIV2ImageReferences(ctx, store, tc, stringSliceArg(args, "reference_file_uuids"))
	} else {
		selectedReferences, err = service.resolveImageReferences(ctx, store, tc, stringSliceArg(args, "reference_uuids"))
	}
	if err != nil {
		return nil, err
	}
	if (operation == imageOperationEdit || operation == imageOperationRestyle) && len(selectedReferences) == 0 {
		return nil, domainError(CodeToolValidation, "图片编辑缺少 Reference", "image_gen 的 edit 和 restyle 操作至少需要选择一张当前 Thread 中出现过的图片 Reference。", nil)
	}
	inputs := make([]imagegen.ImageInput, 0, len(selectedReferences))
	fileService := files.NewService(store, service.hub)
	firstReferenceWidth, firstReferenceHeight := 0, 0
	for index, reference := range selectedReferences {
		content, err := fileService.OpenContent(ctx, reference.FileUUID)
		if err != nil {
			return nil, domainError(CodeToolValidation, "Reference 图片不可用", "所选 Reference 的冻结图片当前无法读取。", err)
		}
		if content.Asset.Kind != "image" {
			_ = content.File.Close()
			return nil, domainError(CodeToolValidation, "参考文件不是图片", "image_gen 只接受图片文件。", nil)
		}
		data, readErr := io.ReadAll(io.LimitReader(content.File, (64<<20)+1))
		closeErr := content.File.Close()
		if readErr != nil || closeErr != nil || len(data) == 0 || len(data) > 64<<20 {
			return nil, domainError(CodeToolValidation, "参考图片无法读取", "参考图片为空、过大或本地内容不可用。", errors.Join(readErr, closeErr))
		}
		data, mimeType, err := normalizeImageGenBytes(data, content.Asset.MIMEType)
		if err != nil {
			return nil, domainError(CodeToolValidation, "参考图片格式不受支持", "image_gen 只接受 PNG、JPEG、WebP 或可解码的 GIF 图片。", err)
		}
		if index == 0 && content.Asset.Width != nil && content.Asset.Height != nil {
			firstReferenceWidth, firstReferenceHeight = *content.Asset.Width, *content.Asset.Height
		}
		inputs = append(inputs, imagegen.ImageInput{MIMEType: mimeType, Data: data})
	}
	if size == "" {
		size = defaultImageGenSize(operation, firstReferenceWidth, firstReferenceHeight)
	}
	defaultStyle := ""
	if useDefaultStyle {
		premise, premiseErr := production.NewService(store, service.hub).GetPremise(ctx)
		if premiseErr != nil {
			return nil, premiseErr
		}
		defaultStyle = strings.TrimSpace(premise.DefaultStyle)
	}
	effectivePrompt := userPrompt
	if tc.ToolProtocol == ToolProtocolProjectAPI {
		effectivePrompt = composeImageGenPrompt(operation, userPrompt, defaultStyle)
	}
	prompt, err := validateText(effectivePrompt, 64<<10, "图片 Prompt")
	if err != nil {
		return nil, err
	}
	resolved, err := service.providers.Resolve(ctx, tc.Run.ProviderUUID)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(resolved.DefaultImageModel)
	if model == "" || strings.TrimSpace(resolved.ImageBaseURL) == "" {
		return nil, domainError(CodeProvider, "图片模型未配置", "当前 Provider 缺少默认图片模型或图片 API 地址。", nil)
	}
	request := imagegen.Request{ProviderType: resolved.ProviderType, BaseURL: resolved.ImageBaseURL, APIKey: resolved.APIKey, Model: model, Prompt: prompt, Size: size, Quality: quality, Images: inputs}
	requestPayload, err := llmlog.EncodeImageRequest(request)
	if err != nil {
		return nil, err
	}
	attempt := 1
	if err := store.DB().WithContext(ctx).Table("llm_logs").Select("COALESCE(MAX(attempt),0)+1").Where("chat_run_id=? AND request_type=?", tc.Run.ID, llmlog.RequestImage).Scan(&attempt).Error; err != nil {
		return nil, err
	}
	logHandle, err := llmlog.Begin(ctx, store, service.hub, llmlog.StartInput{ProjectID: tc.Thread.ProjectID, ChatThreadID: tc.Thread.ID, ChatRunID: tc.Run.ID, SourceType: llmlog.SourceProjectChat, Scenario: "project_chat", RequestType: llmlog.RequestImage, Attempt: attempt, ProviderUUID: resolved.UUID, ProviderType: resolved.ProviderType, Model: model, InputSummary: prompt, RequestPayload: requestPayload})
	if err != nil {
		return nil, err
	}
	generateCtx, cancel := context.WithTimeout(ctx, imageToolTimeout)
	response, generateErr := service.image.Generate(generateCtx, request)
	cancel()
	var responsePayload []byte
	if generateErr == nil {
		responsePayload, err = llmlog.EncodeImageResponse(response, resolved.APIKey)
		if err != nil {
			generateErr = err
		}
	}
	finishErr := llmlog.Finish(context.WithoutCancel(ctx), store, service.hub, logHandle, llmlog.FinishInput{OutputSummary: response.RevisedPrompt, Response: responsePayload, Err: generateErr})
	if generateErr != nil {
		if finishErr != nil {
			generateErr = errors.Join(generateErr, finishErr)
		}
		return nil, generateErr
	}
	if finishErr != nil {
		return nil, finishErr
	}
	generatedBytes, generatedMIMEType, err := normalizeImageGenBytes(response.Bytes, response.MIMEType)
	if err != nil {
		return nil, domainError(CodeProvider, "图片 Provider 返回了不受支持的格式", "生成结果必须是 PNG、JPEG、WebP 或可解码的 GIF。", err)
	}
	filename := generatedImageFilename(stringArg(args, "filename"), execution.UUID, generatedMIMEType)
	referenceUUIDs := make([]string, 0, len(selectedReferences))
	referenceTypes := make([]string, 0, len(selectedReferences))
	resolvedFileUUIDs := make([]string, 0, len(selectedReferences))
	for _, reference := range selectedReferences {
		referenceUUIDs = append(referenceUUIDs, reference.ResourceUUID)
		referenceTypes = append(referenceTypes, reference.ResourceType)
		resolvedFileUUIDs = append(resolvedFileUUIDs, reference.FileUUID)
	}
	metadata := map[string]any{"source": "project_chat_image_gen", "tool_execution_uuid": execution.UUID, "chat_thread_uuid": tc.Thread.UUID, "chat_run_uuid": tc.Run.UUID, "operation": operation, "use_default_style": useDefaultStyle, "reference_uuids": referenceUUIDs, "reference_types": referenceTypes, "resolved_file_uuids": resolvedFileUUIDs, "revised_prompt": llmlog.Summarize(response.RevisedPrompt, 1000)}
	if defaultStyle != "" {
		metadata["default_style_snapshot"] = llmlog.Summarize(defaultStyle, 1000)
	}
	asset, err := fileService.CommitReader(ctx, files.CommitInput{Purpose: purpose, OriginalFilename: filename, DisplayName: filename, SourceType: "generated", Metadata: metadata, Reader: bytes.NewReader(generatedBytes)})
	if err != nil {
		return nil, err
	}
	result := imageToolResult(asset, referenceUUIDs, resolvedFileUUIDs, response.RevisedPrompt)
	if tc.ToolProtocol == ToolProtocolProjectAPI {
		result["operation"] = operation
		result["use_default_style"] = useDefaultStyle
	}
	if legacyV2 {
		result["reference_file_uuids"] = resolvedFileUUIDs
	}
	return result, nil
}

func imageGenOptions(tc toolContext, args map[string]any) (string, bool, error) {
	operation := imageOperationGenerate
	useDefaultStyle := false
	if tc.ToolProtocol != ToolProtocolProjectAPI {
		return operation, useDefaultStyle, nil
	}
	if requested := stringArg(args, "operation"); requested != "" {
		operation = requested
	}
	switch operation {
	case imageOperationGenerate, imageOperationEdit, imageOperationRestyle:
	default:
		return "", false, domainError(CodeToolValidation, "图片操作无效", "image_gen.operation 只接受 generate、edit 或 restyle。", nil)
	}
	useDefaultStyle = true
	if value, exists := args["use_default_style"]; exists {
		selected, ok := value.(bool)
		if !ok {
			return "", false, domainError(CodeToolValidation, "默认画风参数无效", "image_gen.use_default_style 必须是 boolean。", nil)
		}
		useDefaultStyle = selected
	}
	return operation, useDefaultStyle, nil
}

func composeImageGenPrompt(operation, userPrompt, defaultStyle string) string {
	parts := []string{"<operation>\n" + imageOperationInstruction(operation) + "\n</operation>"}
	if strings.TrimSpace(defaultStyle) != "" {
		parts = append(parts, "<project_default_style>\n以下内容只用于决定线条、色彩、材质、光照和渲染方式，不得从中引入人物、场景、物件或构图：\n"+strings.TrimSpace(defaultStyle)+"\n</project_default_style>")
	}
	parts = append(parts, "<user_instruction>\n"+strings.TrimSpace(userPrompt)+"\n</user_instruction>")
	return strings.Join(parts, "\n\n")
}

func imageOperationInstruction(operation string) string {
	switch operation {
	case imageOperationEdit:
		return "基于第一张参考图进行编辑。只修改用户明确要求改变的内容；保留未要求修改的主体数量、身份特征、姿势、服装、道具、构图、镜头角度、裁切范围、背景空间关系和画面比例。其余参考图只能作为补充视觉依据。"
	case imageOperationRestyle:
		return "以第一张参考图作为唯一的内容和构图来源。保留主体数量、身份特征、姿势、服装、道具、镜头角度、裁切范围、背景空间关系和画面比例，只改变渲染风格。其余参考图只能辅助风格或细节，不得替换第一张图的内容与构图。禁止新增人物、文字、标题、标签、卡片、分栏和设定板布局。"
	default:
		return "生成一张新的项目图片。"
	}
}

func defaultImageGenSize(operation string, width, height int) string {
	if operation != imageOperationEdit && operation != imageOperationRestyle {
		return "1536x1024"
	}
	if width <= 0 || height <= 0 {
		return "1536x1024"
	}
	ratio := float64(width) / float64(height)
	candidates := []struct {
		size  string
		ratio float64
	}{{"1024x1024", 1}, {"1024x1536", 2.0 / 3.0}, {"1536x1024", 3.0 / 2.0}}
	best := candidates[0]
	bestDistance := math.Abs(math.Log(ratio / best.ratio))
	for _, candidate := range candidates[1:] {
		distance := math.Abs(math.Log(ratio / candidate.ratio))
		if distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best.size
}

func normalizeImageGenBytes(data []byte, mimeType string) ([]byte, string, error) {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png", "image/jpeg", "image/webp":
		return data, strings.ToLower(strings.TrimSpace(mimeType)), nil
	case "image/gif":
		decoded, err := gif.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, "", err
		}
		var converted bytes.Buffer
		if err := png.Encode(&converted, decoded); err != nil {
			return nil, "", err
		}
		return converted.Bytes(), "image/png", nil
	default:
		return nil, "", errors.New("unsupported image MIME type")
	}
}

func (service *Service) generatedImageForExecution(ctx context.Context, store *project.Store, purpose, executionUUID string) (files.Asset, bool, error) {
	var fileUUID string
	err := store.DB().WithContext(ctx).Table("files").Select("uuid").Where("project_id=(SELECT id FROM projects WHERE uuid=?) AND purpose=? AND deleted_at IS NULL AND json_extract(metadata_json,'$.tool_execution_uuid')=?", store.ProjectUUID(), purpose, executionUUID).Order("id").Limit(1).Scan(&fileUUID).Error
	if err != nil || fileUUID == "" {
		return files.Asset{}, false, err
	}
	asset, err := files.NewService(store, service.hub).GetAsset(ctx, fileUUID, false)
	return asset, err == nil, err
}

type selectedImageReference struct {
	ResourceUUID string
	ResourceType string
	FileUUID     string
}

func (service *Service) resolveImageReferences(ctx context.Context, store *project.Store, tc toolContext, selected []string) ([]selectedImageReference, error) {
	if len(selected) > maxImageGenReferences {
		return nil, domainError(CodeToolValidation, "Reference 过多", "image_gen.reference_uuids 最多选择 4 项。", nil)
	}
	result := make([]selectedImageReference, 0, len(selected))
	seen := map[string]bool{}
	for _, resourceUUID := range selected {
		resourceUUID = strings.TrimSpace(resourceUUID)
		if !isUUIDv7(resourceUUID) {
			return nil, domainError(CodeToolValidation, "Reference UUID 无效", "reference_uuids 只能包含当前 Thread 中出现过的 Reference UUIDv7。", nil)
		}
		if seen[resourceUUID] {
			return nil, domainError(CodeToolValidation, "Reference 选择重复", "reference_uuids 不能包含重复项。", nil)
		}
		seen[resourceUUID] = true
		var row struct {
			ResourceUUID string
			ResourceType string
			ImageFileID  *int64
			FileUUID     string
			DeletedAt    *time.Time
			ObjectState  string
		}
		query := store.DB().WithContext(ctx).Table("chat_context_references AS refs").
			Select("refs.resource_uuid,refs.resource_type,refs.image_file_id,COALESCE(files.uuid,'') AS file_uuid,files.deleted_at,COALESCE(objects.state,'') AS object_state").
			Joins("JOIN chat_items AS items ON items.id=refs.chat_item_id").
			Joins("LEFT JOIN files ON files.id=refs.image_file_id").
			Joins("LEFT JOIN file_objects AS objects ON objects.id=files.file_object_id")
		missingTitle := "Reference 不在当前 Turn"
		missingDetails := "reference_uuids 不能选择历史、未知或其他 Turn 的 Reference。"
		if tc.ToolProtocol == ToolProtocolProjectAPI {
			query = query.Joins("JOIN chat_turns AS turns ON turns.id=items.turn_id").
				Where("items.thread_id=? AND turns.queue_sequence<=? AND items.item_type='user_message' AND refs.resource_uuid=?", tc.Thread.ID, tc.Turn.QueueSequence, resourceUUID).
				Order("items.sequence DESC,refs.position DESC,refs.id DESC")
			missingTitle = "Reference 不在当前 Thread"
			missingDetails = "reference_uuids 只能选择当前 Thread 内截至本次调用前出现过的 Reference。"
		} else {
			query = query.Where("items.turn_id=? AND items.item_type='user_message' AND refs.resource_uuid=?", tc.Turn.ID, resourceUUID).
				Order("items.sequence DESC,refs.position DESC,refs.id DESC")
		}
		err := query.Limit(1).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainError(CodeToolValidation, missingTitle, missingDetails, err)
		}
		if err != nil {
			return nil, err
		}
		if row.ImageFileID == nil || row.FileUUID == "" || row.DeletedAt != nil || row.ObjectState != files.ObjectReady {
			return nil, domainError(CodeToolValidation, "Reference 没有可用图片", "所选 Reference 不包含冻结图片，无法用于 image_gen。", nil)
		}
		result = append(result, selectedImageReference{ResourceUUID: row.ResourceUUID, ResourceType: row.ResourceType, FileUUID: row.FileUUID})
	}
	return result, nil
}

// resolveProjectAPIV2ImageReferences is recovery-only. It translates the old
// automatic attachment plus explicit file UUID contract onto migrated frozen
// references, while keeping v3's active schema free of file UUID arguments.
func (service *Service) resolveProjectAPIV2ImageReferences(ctx context.Context, store *project.Store, tc toolContext, explicit []string) ([]selectedImageReference, error) {
	result := make([]selectedImageReference, 0, maxImageGenReferences)
	appendReference := func(reference selectedImageReference) {
		for _, existing := range result {
			if existing.FileUUID == reference.FileUUID {
				return
			}
		}
		result = append(result, reference)
	}
	loadRows := func(query *gorm.DB) error {
		rows, err := query.Rows()
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var reference selectedImageReference
			if err := rows.Scan(&reference.ResourceUUID, &reference.ResourceType, &reference.FileUUID); err != nil {
				return err
			}
			appendReference(reference)
		}
		return rows.Err()
	}

	if tc.Thread.Scene == SceneAssetReference && isUUIDv7(tc.Thread.SubjectUUID) {
		query := store.DB().WithContext(ctx).Table("chat_context_references AS refs").
			Select("refs.resource_uuid,refs.resource_type,files.uuid").
			Joins("JOIN chat_items AS items ON items.id=refs.chat_item_id").
			Joins("JOIN files ON files.id=refs.image_file_id AND files.deleted_at IS NULL").
			Joins("JOIN file_objects AS objects ON objects.id=files.file_object_id AND objects.state=?", files.ObjectReady).
			Where("items.turn_id=? AND items.item_type='user_message' AND refs.resource_type=? AND refs.resource_uuid=?", tc.Turn.ID, ReferenceTypePremiseAsset, tc.Thread.SubjectUUID).
			Order("items.sequence DESC,refs.position,refs.id").Limit(1)
		if err := loadRows(query); err != nil {
			return nil, err
		}
		if len(result) == 0 {
			return nil, domainError(CodeToolValidation, "当前设定图不可用", "冻结的 v2 asset_reference 上下文没有可用图片。", nil)
		}
	}

	latestFileItem := store.DB().WithContext(ctx).Table("chat_items AS latest").
		Select("latest.id").
		Where("latest.turn_id=? AND latest.item_type='user_message' AND EXISTS (SELECT 1 FROM chat_context_references latest_refs WHERE latest_refs.chat_item_id=latest.id AND latest_refs.resource_type=?)", tc.Turn.ID, ReferenceTypeFile).
		Order("latest.sequence DESC,latest.id DESC").Limit(1)
	query := store.DB().WithContext(ctx).Table("chat_context_references AS refs").
		Select("refs.resource_uuid,refs.resource_type,files.uuid").
		Joins("JOIN files ON files.id=refs.image_file_id AND files.deleted_at IS NULL").
		Joins("JOIN file_objects AS objects ON objects.id=files.file_object_id AND objects.state=?", files.ObjectReady).
		Where("refs.chat_item_id=(?) AND refs.resource_type=?", latestFileItem, ReferenceTypeFile).
		Order("refs.position,refs.id")
	if err := loadRows(query); err != nil {
		return nil, err
	}

	fileService := files.NewService(store, service.hub)
	for _, fileUUID := range explicit {
		fileUUID = strings.TrimSpace(fileUUID)
		if !isUUIDv7(fileUUID) {
			return nil, domainError(CodeToolValidation, "参考文件 UUID 无效", "冻结的 project_api_v2 reference_file_uuids 只能包含 UUIDv7。", nil)
		}
		asset, err := fileService.GetAsset(ctx, fileUUID, false)
		if err != nil || asset.Kind != "image" {
			return nil, domainError(CodeToolValidation, "参考图片不可用", "冻结的 project_api_v2 reference_file_uuids 必须引用当前项目的 active 图片。", err)
		}
		appendReference(selectedImageReference{ResourceUUID: fileUUID, ResourceType: ReferenceTypeFile, FileUUID: fileUUID})
	}
	if len(result) > maxImageGenReferences {
		return nil, domainError(CodeToolValidation, "参考图片过多", "冻结的 project_api_v2 图片参考去重后最多 4 张。", nil)
	}
	return result, nil
}

func stableUniqueUUIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func generatedImageFilename(requested, executionUUID, mimeType string) string {
	ext := "png"
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		ext = "jpg"
	case "image/webp":
		ext = "webp"
	}
	name := filepath.Base(strings.TrimSpace(requested))
	if name == "" || name == "." {
		name = "chat-image-" + executionUUID
	}
	return strings.TrimSuffix(name, filepath.Ext(name)) + "." + ext
}

func imageToolResult(asset files.Asset, references, resolvedFiles []string, revisedPrompt string) map[string]any {
	return map[string]any{"file_uuid": asset.UUID, "filename": asset.OriginalFilename, "content_url": asset.ContentURL, "mime_type": asset.MIMEType, "byte_size": asset.ByteSize, "purpose": asset.Purpose, "revised_prompt": revisedPrompt, "reference_uuids": references, "resolved_file_uuids": resolvedFiles}
}

func metadataText(value any, key string) string {
	metadata, _ := value.(map[string]any)
	text, _ := metadata[key].(string)
	return text
}

func metadataBool(value any, key string) bool {
	metadata, _ := value.(map[string]any)
	selected, _ := metadata[key].(bool)
	return selected
}

func metadataStringSlice(value any, key string) []string {
	metadata, _ := value.(map[string]any)
	values, _ := metadata[key].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	if typed, ok := metadata[key].([]string); ok {
		return typed
	}
	return result
}
