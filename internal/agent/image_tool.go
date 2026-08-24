package agent

import (
	"bytes"
	"context"
	"errors"
	"image/gif"
	"image/png"
	"io"
	"path/filepath"
	"strings"
	"time"

	"lumi/internal/files"
	"lumi/internal/imagegen"
	"lumi/internal/llmlog"
	"lumi/internal/project"
)

const imageToolTimeout = 10 * time.Minute

func (service *Service) executeImageGenTool(ctx context.Context, store *project.Store, tc toolContext, execution toolExecutionRecord, args map[string]any) (map[string]any, error) {
	if service.image == nil {
		return nil, domainError(CodeStateConflict, "图片生成服务不可用", "Image client 尚未初始化。", nil)
	}
	prompt, err := validateText(stringArg(args, "prompt"), 64<<10, "图片 Prompt")
	if err != nil {
		return nil, err
	}
	size := stringArg(args, "size")
	if size == "" {
		size = "1536x1024"
	}
	quality := stringArg(args, "quality")
	if quality == "" {
		quality = "medium"
	}
	purpose := "project_chat_asset_image_generation"
	if logicalSceneKey(tc.Thread) == SceneAssetReference {
		purpose = "project_chat_asset_reference_image"
	}
	if existing, found, err := service.generatedImageForExecution(ctx, store, purpose, execution.UUID); err != nil {
		return nil, err
	} else if found {
		return imageToolResult(existing, metadataStringSlice(existing.Metadata, "reference_file_uuids"), metadataText(existing.Metadata, "revised_prompt")), nil
	}

	referenceUUIDs, err := service.resolveImageReferenceUUIDs(ctx, store, tc, stringSliceArg(args, "reference_file_uuids"))
	if err != nil {
		return nil, err
	}
	inputs := make([]imagegen.ImageInput, 0, len(referenceUUIDs))
	fileService := files.NewService(store, service.hub)
	for _, fileUUID := range referenceUUIDs {
		content, err := fileService.OpenContent(ctx, fileUUID)
		if err != nil {
			return nil, domainError(CodeToolValidation, "参考图片不可用", "reference_file_uuids 必须引用当前项目中可读取的图片。", err)
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
		inputs = append(inputs, imagegen.ImageInput{MIMEType: mimeType, Data: data})
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
	logHandle, err := llmlog.Begin(ctx, store, service.hub, llmlog.StartInput{ProjectID: tc.Thread.ProjectID, ChatThreadID: tc.Thread.ID, ChatRunID: tc.Run.ID, SourceType: llmlog.SourceProjectChat, Scenario: "project_chat_asset_image_generation", RequestType: llmlog.RequestImage, Attempt: attempt, ProviderUUID: resolved.UUID, ProviderType: resolved.ProviderType, Model: model, InputSummary: prompt, RequestPayload: requestPayload})
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
	metadata := map[string]any{"source": "project_chat_image_gen", "tool_execution_uuid": execution.UUID, "chat_thread_uuid": tc.Thread.UUID, "chat_run_uuid": tc.Run.UUID, "reference_file_uuids": referenceUUIDs, "revised_prompt": llmlog.Summarize(response.RevisedPrompt, 1000)}
	if logicalSceneKey(tc.Thread) == SceneAssetReference {
		metadata["premise_asset_uuid"] = tc.Thread.SubjectUUID
	}
	asset, err := fileService.CommitReader(ctx, files.CommitInput{Purpose: purpose, OriginalFilename: filename, DisplayName: filename, SourceType: "generated", Metadata: metadata, Reader: bytes.NewReader(generatedBytes)})
	if err != nil {
		return nil, err
	}
	return imageToolResult(asset, referenceUUIDs, response.RevisedPrompt), nil
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

func (service *Service) resolveImageReferenceUUIDs(ctx context.Context, store *project.Store, tc toolContext, explicit []string) ([]string, error) {
	additional := []string{}
	rows, err := store.DB().WithContext(ctx).Raw(`SELECT f.uuid FROM chat_items items JOIN chat_item_file_references refs ON refs.chat_item_id=items.id JOIN files f ON f.id=refs.file_id WHERE items.turn_id=? AND items.item_type='user_message' AND items.id=(SELECT latest.id FROM chat_items latest WHERE latest.turn_id=? AND latest.item_type='user_message' AND EXISTS (SELECT 1 FROM chat_item_file_references latest_refs WHERE latest_refs.chat_item_id=latest.id) ORDER BY latest.sequence DESC,latest.id DESC LIMIT 1) ORDER BY refs.position,refs.id`, tc.Turn.ID, tc.Turn.ID).Rows()
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			_ = rows.Close()
			return nil, err
		}
		additional = append(additional, uuid)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	additional = append(additional, explicit...)
	additional = stableUniqueUUIDs(additional)
	if len(additional) > maxChatImageReferences {
		return nil, domainError(CodeToolValidation, "参考图片过多", "当前消息附件与 reference_file_uuids 合计最多 4 张。", nil)
	}
	if logicalSceneKey(tc.Thread) != SceneAssetReference {
		return additional, nil
	}
	var currentFileUUID string
	err = store.DB().WithContext(ctx).Table("premise_assets AS assets").Select("files.uuid").Joins("JOIN premise_asset_variants variants ON variants.id=assets.current_variant_id").Joins("JOIN files ON files.id=variants.file_id").Where("assets.project_id=? AND assets.uuid=? AND assets.deleted_at IS NULL AND files.deleted_at IS NULL", tc.Thread.ProjectID, tc.Thread.SubjectUUID).Scan(&currentFileUUID).Error
	if err != nil || currentFileUUID == "" {
		return nil, domainError(CodeToolValidation, "当前设定图不可用", "asset_reference 会话必须以当前设定项图片作为第一张参考图。", err)
	}
	result := stableUniqueUUIDs(append([]string{currentFileUUID}, additional...))
	if len(result) > maxChatImageReferences {
		return nil, domainError(CodeToolValidation, "参考图片过多", "当前设定图、当前消息附件与 reference_file_uuids 去重后合计最多 4 张。", nil)
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

func imageToolResult(asset files.Asset, references []string, revisedPrompt string) map[string]any {
	return map[string]any{"file_uuid": asset.UUID, "filename": asset.OriginalFilename, "content_url": asset.ContentURL, "mime_type": asset.MIMEType, "byte_size": asset.ByteSize, "purpose": asset.Purpose, "revised_prompt": revisedPrompt, "reference_file_uuids": references}
}

func metadataText(value any, key string) string {
	metadata, _ := value.(map[string]any)
	text, _ := metadata[key].(string)
	return text
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
