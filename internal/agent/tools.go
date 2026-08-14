package agent

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"lumi/internal/imagegen"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"

	"gorm.io/gorm"
)

type toolContext struct {
	ProjectUUID string
	Thread      threadRecord
	Turn        turnRecord
	Run         runRecord
}

type toolExecutionRecord struct {
	ID, ThreadID, RunID, TurnID, ItemID                                            int64
	UUID, ToolCallUUID, ToolName, TargetUUID, ArgumentsJSON, IdempotencyKey, State string
	ResultJSON                                                                     *string
	ErrorCode, ErrorMessage                                                        string
	StartedAt, CompletedAt                                                         *time.Time
	CreatedAt, UpdatedAt                                                           time.Time
}

func toolDefinitions() []map[string]any {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	stringField := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	integerField := func(description string) map[string]any {
		return map[string]any{"type": "integer", "description": description}
	}
	return []map[string]any{
		{"name": "get_story_profile", "description": "Read the current project STORY.md profile.", "parameters": object(map[string]any{})},
		{"name": "update_story_profile", "description": "Create a new current STORY.md profile version.", "parameters": object(map[string]any{"story_md": stringField("Complete STORY.md content"), "expected_revision": integerField("Current profile revision")}, "story_md", "expected_revision")},
		{"name": "list_chapters", "description": "List active story chapters with current story summaries.", "parameters": object(map[string]any{})},
		{"name": "get_chapter", "description": "Read one story chapter by UUID.", "parameters": object(map[string]any{"chapter_uuid": stringField("Public chapter UUIDv7")}, "chapter_uuid")},
		{"name": "update_chapter_story", "description": "Append a new current story version to a chapter.", "parameters": object(map[string]any{"chapter_uuid": stringField("Public chapter UUIDv7"), "content": stringField("Complete replacement chapter content"), "content_format": map[string]any{"type": "string", "enum": []string{"txt", "md"}}, "expected_revision": integerField("Current chapter revision")}, "chapter_uuid", "content", "content_format", "expected_revision")},
		{"name": "get_premise", "description": "Read the current premise profile.", "parameters": object(map[string]any{})},
		{"name": "list_premise_assets", "description": "List active premise assets.", "parameters": object(map[string]any{})},
		{"name": "get_premise_asset", "description": "Read one active premise asset by public UUID.", "parameters": object(map[string]any{"premise_asset_uuid": stringField("Public premise asset UUIDv7")}, "premise_asset_uuid")},
		{"name": "request_current_project_api", "description": "Call an allowlisted REST-shaped operation inside the current project. This tool never performs an HTTP loopback and can only access the asset bound to the current reference thread.", "parameters": object(map[string]any{
			"url":    stringField("Allowlisted current-project API path"),
			"method": map[string]any{"type": "string", "enum": []string{"GET", "POST", "PATCH", "DELETE"}},
			"request_body": object(map[string]any{
				"file_uuid":         stringField("Public image file UUIDv7 returned by image_gen"),
				"asset_type":        map[string]any{"type": "string", "enum": []string{"character", "scene", "prop", "reference"}},
				"title":             stringField("Asset title"),
				"summary":           stringField("Asset summary"),
				"tags":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"expected_revision": integerField("Freshly read current asset revision"),
			}),
		}, "url", "method")},
		{"name": "create_premise_asset", "description": "Create a premise asset from exactly one generated file_uuid or ready upload_uuid.", "parameters": object(map[string]any{"file_uuid": stringField("Public generated file UUIDv7 returned by image_gen"), "upload_uuid": stringField("Public ready upload UUIDv7"), "asset_type": map[string]any{"type": "string", "enum": []string{"character", "scene", "prop", "reference"}}, "title": stringField("Asset title"), "summary": stringField("Asset summary"), "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "asset_type", "title")},
		{"name": "update_premise_asset", "description": "Update premise asset metadata and optionally append/select a generated file_uuid as a new image variant.", "parameters": object(map[string]any{"premise_asset_uuid": stringField("Public premise asset UUIDv7"), "expected_revision": integerField("Current premise asset revision"), "file_uuid": stringField("Optional public generated file UUIDv7 returned by image_gen"), "asset_type": map[string]any{"type": "string", "enum": []string{"character", "scene", "prop", "reference"}}, "title": stringField("Asset title"), "summary": stringField("Asset summary"), "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "premise_asset_uuid", "expected_revision")},
		{"name": "image_gen", "description": "Generate a project-scoped image synchronously. Current-message attachments are supplied automatically; asset-reference threads also prepend the current asset image.", "parameters": object(map[string]any{"prompt": stringField("Detailed image generation prompt"), "reference_file_uuids": map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{"type": "string"}}, "size": map[string]any{"type": "string", "enum": []string{"512x512", "1024x1024", "1024x1536", "1536x1024"}}, "quality": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}}, "filename": stringField("Optional output filename")}, "prompt")},
		{"name": "generate_premise_asset", "description": "Generate one square image and create a new premise asset through the Go production queue and Asset Store.", "parameters": object(map[string]any{"asset_type": map[string]any{"type": "string", "enum": []string{"character", "scene", "prop", "reference"}}, "title": stringField("Unique premise asset title"), "summary": stringField("Premise asset summary"), "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "prompt": stringField("Detailed visual generation prompt"), "model": stringField("Optional image model override")}, "asset_type", "title", "prompt")},
		{"name": "generate_premise_asset_variant", "description": "Generate and select a new image variant for the premise asset referenced by this thread.", "parameters": object(map[string]any{"prompt": stringField("Detailed visual generation prompt preserving the referenced asset identity"), "model": stringField("Optional image model override")}, "prompt")},
		{"name": "get_comic_section", "description": "Read one comic section and its current storyboard.", "parameters": object(map[string]any{"chapter_uuid": stringField("Public chapter UUIDv7"), "section_uuid": stringField("Public section UUIDv7")}, "chapter_uuid", "section_uuid")},
		{"name": "update_comic_storyboard", "description": "Append a generated storyboard version and select it as current.", "parameters": object(map[string]any{"chapter_uuid": stringField("Public chapter UUIDv7"), "section_uuid": stringField("Public section UUIDv7"), "content_md": stringField("Complete storyboard markdown"), "expected_revision": integerField("Current section revision")}, "chapter_uuid", "section_uuid", "content_md", "expected_revision")},
		{"name": "start_generation", "description": "Start an existing allowlisted project generation task using this run's Provider snapshot.", "parameters": object(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"story_chapter_generation", "premise_setting_generation", "premise_asset_breakdown", "comic_image_generation"}}, "resource_uuid": stringField("Public target resource UUIDv7"), "chapter_uuid": stringField("Public chapter UUIDv7 for chapter or comic tasks"), "model": stringField("Provider model name; may be empty to use default"), "prompt": stringField("Generation instructions"), "premise_asset_uuids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "kind", "resource_uuid", "prompt")},
		{"name": "request_user_input", "description": "Pause this run and ask the user a bounded choice question.", "parameters": object(map[string]any{"input_type": map[string]any{"type": "string", "enum": []string{"single_choice", "multiple_choice"}}, "question": stringField("Question shown to the user"), "options": map[string]any{"type": "array", "minItems": 2, "maxItems": 8, "items": object(map[string]any{"label": stringField("Short option label"), "description": stringField("Optional explanation")}, "label")}}, "input_type", "question", "options")},
	}
}

func validateToolArguments(name string, raw string) (map[string]any, error) {
	if !json.Valid([]byte(raw)) {
		return nil, domainError(CodeToolValidation, "工具参数不是有效 JSON", "arguments 必须是 JSON object。", nil)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil || args == nil {
		return nil, domainError(CodeToolValidation, "工具参数不是 JSON object", "arguments 必须是 JSON object。", err)
	}
	var parameters map[string]any
	for _, definition := range toolDefinitions() {
		if definition["name"] == name {
			parameters, _ = definition["parameters"].(map[string]any)
			break
		}
	}
	if parameters == nil {
		return nil, domainError(CodeToolNotAllowed, "工具不在 allowlist", "Agent 只能调用当前项目注册的受控工具。", nil)
	}
	if err := validatePublicArguments(args, ""); err != nil {
		return nil, err
	}
	properties, _ := parameters["properties"].(map[string]any)
	for key, value := range args {
		rawSchema, ok := properties[key]
		if !ok {
			return nil, domainError(CodeToolValidation, "工具参数包含未知字段", key+" 不在该工具的参数 schema 中。", nil)
		}
		schema, _ := rawSchema.(map[string]any)
		if err := validateArgumentShape(key, value, schema); err != nil {
			return nil, err
		}
	}
	if required, ok := parameters["required"].([]string); ok {
		for _, key := range required {
			if _, exists := args[key]; !exists {
				return nil, domainError(CodeToolValidation, "工具参数缺少必填字段", key+" 是必填字段。", nil)
			}
		}
	}
	return args, nil
}

func validateArgumentShape(key string, value any, schema map[string]any) error {
	want, _ := schema["type"].(string)
	valid := false
	switch want {
	case "string":
		_, valid = value.(string)
	case "integer":
		number, ok := value.(float64)
		valid = ok && number == float64(int64(number))
	case "array":
		values, ok := value.([]any)
		valid = ok
		if ok {
			if minimum, exists := schema["minItems"].(int); exists && len(values) < minimum {
				valid = false
			}
			if maximum, exists := schema["maxItems"].(int); exists && len(values) > maximum {
				valid = false
			}
			itemSchema, _ := schema["items"].(map[string]any)
			for _, item := range values {
				if err := validateArgumentShape(key, item, itemSchema); err != nil {
					return err
				}
			}
		}
	case "object":
		object, ok := value.(map[string]any)
		valid = ok
		if ok {
			properties, _ := schema["properties"].(map[string]any)
			for childKey, childValue := range object {
				rawChildSchema, exists := properties[childKey]
				if !exists {
					return domainError(CodeToolValidation, "工具参数包含未知字段", childKey+" 不在该工具的参数 schema 中。", nil)
				}
				childSchema, _ := rawChildSchema.(map[string]any)
				if err := validateArgumentShape(childKey, childValue, childSchema); err != nil {
					return err
				}
			}
			if required, exists := schema["required"].([]string); exists {
				for _, childKey := range required {
					if _, present := object[childKey]; !present {
						return domainError(CodeToolValidation, "工具参数缺少必填字段", childKey+" 是必填字段。", nil)
					}
				}
			}
		}
	default:
		valid = true
	}
	if !valid {
		return domainError(CodeToolValidation, "工具参数类型无效", key+" 不符合工具参数 schema。", nil)
	}
	if enum, ok := schema["enum"].([]string); ok {
		text, _ := value.(string)
		matched := false
		for _, candidate := range enum {
			matched = matched || text == candidate
		}
		if !matched {
			return domainError(CodeToolValidation, "工具参数枚举值无效", key+" 不在允许值中。", nil)
		}
	}
	return nil
}

func validatePublicArguments(value any, key string) error {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			lower := strings.ToLower(childKey)
			if lower == "id" || strings.HasSuffix(lower, "_id") || lower == "path" || strings.HasSuffix(lower, "_path") {
				return domainError(CodeToolValidation, "工具参数包含内部字段", "只允许公开 UUID，不允许 id 或磁盘路径。", nil)
			}
			if err := validatePublicArguments(child, childKey); err != nil {
				return err
			}
		}
	case []any:
		if strings.HasSuffix(strings.ToLower(key), "_uuids") {
			for _, child := range typed {
				text, ok := child.(string)
				if !ok || !isUUIDv7(text) {
					return domainError(CodeToolValidation, "工具 UUID 列表无效", key+" 必须是 UUIDv7 数组。", nil)
				}
			}
			return nil
		}
		for _, child := range typed {
			if err := validatePublicArguments(child, key); err != nil {
				return err
			}
		}
	case string:
		lower := strings.ToLower(key)
		if strings.HasSuffix(lower, "_uuid") && typed != "" && !isUUIDv7(typed) {
			return domainError(CodeToolValidation, "工具 UUID 参数无效", key+" 必须是 UUIDv7。", nil)
		}
		if strings.HasSuffix(lower, "_uuids") && typed != "" {
			return domainError(CodeToolValidation, "工具 UUID 列表无效", key+" 必须是 UUIDv7 数组。", nil)
		}
	}
	return nil
}

func toolCallKey(runUUID, providerCallID, toolName string) string {
	digest := sha256.Sum256([]byte(runUUID + "\x00" + providerCallID + "\x00" + toolName))
	return "agent-tool-v1:" + hex.EncodeToString(digest[:])
}

func targetUUIDForTool(name string, args map[string]any) string {
	keys := []string{"premise_asset_uuid", "section_uuid", "chapter_uuid", "resource_uuid", "upload_uuid"}
	for _, key := range keys {
		if value, ok := args[key].(string); ok && isUUIDv7(value) {
			return value
		}
	}
	return ""
}

func (service *Service) persistToolIntent(ctx context.Context, store *project.Store, tc toolContext, providerCallID, name, raw string) (toolExecutionRecord, json.RawMessage, bool, error) {
	key := toolCallKey(tc.Run.UUID, providerCallID, name)
	var existing toolExecutionRecord
	query := store.DB().WithContext(ctx).Where("idempotency_key=?", key).First(&existing)
	if query.Error == nil {
		if existing.State == "completed" && existing.ResultJSON != nil {
			return existing, json.RawMessage(*existing.ResultJSON), true, nil
		}
		return existing, nil, false, nil
	}
	if !errors.Is(query.Error, gorm.ErrRecordNotFound) {
		return existing, nil, false, query.Error
	}
	args, err := validateToolArguments(name, raw)
	if err != nil {
		return existing, nil, false, err
	}
	if !toolAllowedForThread(name, tc.Thread) {
		return existing, nil, false, domainError(CodeToolNotAllowed, "工具不适用于当前场景", "该工具只能在匹配的 Premise scene 中调用。", nil)
	}
	if name == "request_current_project_api" {
		if _, err := parseCurrentProjectAPIRequest(tc, args); err != nil {
			return existing, nil, false, err
		}
	}
	if !toolTargetAllowedForThread(name, args, tc.Thread) {
		return existing, nil, false, domainError(CodeToolNotAllowed, "工具目标不属于当前场景", "设定项引用 thread 只能读写其 subject_uuid。", nil)
	}
	publicCallUUID, err := newUUIDv7()
	if err != nil {
		return existing, nil, false, err
	}
	executionUUID, err := newUUIDv7()
	if err != nil {
		return existing, nil, false, err
	}
	targetUUID := targetUUIDForTool(name, args)
	if name == "image_gen" {
		targetUUID = tc.Thread.UUID
		if tc.Thread.Scene == SceneAssetReference {
			targetUUID = tc.Thread.SubjectUUID
		}
	} else if name == "generate_premise_asset" {
		targetUUID = tc.Thread.UUID
	} else if name == "generate_premise_asset_variant" {
		targetUUID = tc.Thread.SubjectUUID
	} else if name == "request_current_project_api" {
		targetUUID = tc.Thread.SubjectUUID
	}
	storedArgs := make(map[string]any, len(args)+1)
	for key, value := range args {
		storedArgs[key] = value
	}
	storedArgs["__provider_call_id"] = providerCallID
	encodedArgs, _ := json.Marshal(storedArgs)
	sqlDB, err := store.DB().DB()
	if err != nil {
		return existing, nil, false, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return existing, nil, false, err
	}
	defer tx.Rollback()
	thread, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
	if err != nil {
		return existing, nil, false, err
	}
	now := service.now().UTC()
	metadata := map[string]any{"purpose": name, "target_uuid": targetUUID, "provider_call_id": providerCallID}
	item, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "tool_call", "assistant", raw, "json", "in_progress", publicCallUUID, name, targetUUID, metadata, now)
	if err != nil {
		return existing, nil, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO agent_tool_executions(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,'intent',?,?)`, executionUUID, tc.Thread.ID, tc.Run.ID, tc.Turn.ID, item.ID, publicCallUUID, name, targetUUID, string(encodedArgs), key, now, now)
	if err != nil {
		return existing, nil, false, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return existing, nil, false, err
	}
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "tool_intent", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "tool_call_uuid": publicCallUUID, "tool_name": name, "target_uuid": targetUUID}, now); err != nil {
		return existing, nil, false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
		return existing, nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return existing, nil, false, err
	}
	existing = toolExecutionRecord{ID: id, ThreadID: tc.Thread.ID, RunID: tc.Run.ID, TurnID: tc.Turn.ID, ItemID: item.ID, UUID: executionUUID, ToolCallUUID: publicCallUUID, ToolName: name, TargetUUID: targetUUID, ArgumentsJSON: string(encodedArgs), IdempotencyKey: key, State: "intent", CreatedAt: now, UpdatedAt: now}
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:tool_call", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "tool_call_uuid": publicCallUUID, "tool_name": name, "target_uuid": targetUUID, "status": "in_progress"})
	return existing, nil, false, nil
}

func (service *Service) executeTool(ctx context.Context, store *project.Store, tc toolContext, execution toolExecutionRecord) (json.RawMessage, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(execution.ArgumentsJSON), &args); err != nil {
		return nil, domainError(CodeToolValidation, "持久化工具参数损坏", "无法安全恢复工具调用。", err)
	}
	delete(args, "__provider_call_id")
	_ = store.DB().WithContext(ctx).Model(&toolExecutionRecord{}).Table("agent_tool_executions").Where("id=? AND state='intent'", execution.ID).Updates(map[string]any{"state": "executing", "started_at": service.now().UTC(), "updated_at": service.now().UTC()}).Error
	storyService := story.NewService(store)
	productionService := production.NewService(store, service.hub)
	var value any
	var err error
	switch execution.ToolName {
	case "get_story_profile":
		value, err = storyService.GetStoryProfile(ctx)
	case "update_story_profile":
		value, err = updateStoryProfileTool(ctx, storyService, args)
	case "list_chapters":
		var items []story.Chapter
		items, err = storyService.ListChapters(ctx, "active")
		value = map[string]any{"items": items}
	case "get_chapter":
		value, err = storyService.GetChapter(ctx, stringArg(args, "chapter_uuid"))
	case "update_chapter_story":
		value, err = updateChapterStoryTool(ctx, storyService, args)
	case "get_premise":
		value, err = productionService.GetPremise(ctx)
	case "list_premise_assets":
		var items []production.PremiseAsset
		items, err = productionService.ListPremiseAssets(ctx, "", "active")
		value = map[string]any{"items": items}
	case "get_premise_asset":
		value, err = productionService.GetPremiseAsset(ctx, stringArg(args, "premise_asset_uuid"))
	case "request_current_project_api":
		value, err = executeCurrentProjectAPITool(ctx, productionService, tc, execution, args)
	case "create_premise_asset":
		value, err = createPremiseAssetTool(ctx, productionService, tc, execution, args)
	case "update_premise_asset":
		value, err = updatePremiseAssetTool(ctx, productionService, tc, execution, args)
	case "image_gen":
		value, err = service.executeImageGenTool(ctx, store, tc, execution, args)
	case "generate_premise_asset":
		value, err = service.startPremiseAssetGenerationTool(ctx, tc, execution, args, false)
	case "generate_premise_asset_variant":
		value, err = service.startPremiseAssetGenerationTool(ctx, tc, execution, args, true)
	case "get_comic_section":
		value, err = productionService.GetSection(ctx, stringArg(args, "chapter_uuid"), stringArg(args, "section_uuid"))
	case "update_comic_storyboard":
		value, err = updateStoryboardTool(ctx, productionService, args)
	case "start_generation":
		value, err = service.startGenerationTool(ctx, tc, execution, args)
	default:
		err = domainError(CodeToolNotAllowed, "工具不在 allowlist", "工具未注册。", nil)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, ctx.Err()) {
			return nil, err
		}
		return toolErrorResult(err), nil
	}
	return compactToolResult(map[string]any{"success": true, "data": value}, execution.TargetUUID), nil
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func intArg(args map[string]any, key string) int64 {
	switch value := args[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case json.Number:
		result, _ := value.Int64()
		return result
	}
	return 0
}

func stringSliceArg(args map[string]any, key string) []string {
	values, _ := args[key].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func updateStoryProfileTool(ctx context.Context, service *story.Service, args map[string]any) (story.StoryProfile, error) {
	desired := stringArg(args, "story_md")
	current, err := service.GetStoryProfile(ctx)
	if err != nil {
		return story.StoryProfile{}, err
	}
	if strings.TrimSpace(current.StoryMD) == strings.TrimSpace(desired) {
		return current, nil
	}
	return service.UpdateStoryProfile(ctx, desired, intArg(args, "expected_revision"))
}

func updateChapterStoryTool(ctx context.Context, service *story.Service, args map[string]any) (story.Chapter, error) {
	chapterUUID := stringArg(args, "chapter_uuid")
	desired := stringArg(args, "content")
	current, err := service.GetChapter(ctx, chapterUUID)
	if err != nil {
		return story.Chapter{}, err
	}
	if current.CurrentStory != nil && strings.TrimSpace(current.CurrentStory.Content) == strings.TrimSpace(desired) && current.CurrentStory.ContentFormat == stringArg(args, "content_format") {
		return current, nil
	}
	return service.UpdateStory(ctx, chapterUUID, story.UpdateStoryInput{Content: desired, ContentFormat: stringArg(args, "content_format"), ExpectedRevision: intArg(args, "expected_revision")})
}

func createPremiseAssetTool(ctx context.Context, service *production.Service, tc toolContext, execution toolExecutionRecord, args map[string]any) (production.PremiseAsset, error) {
	uploadUUID, fileUUID := stringArg(args, "upload_uuid"), stringArg(args, "file_uuid")
	if (uploadUUID == "") == (fileUUID == "") {
		return production.PremiseAsset{}, domainError(CodeToolValidation, "设定项图片来源无效", "file_uuid 与 upload_uuid 必须且只能提供一个。", nil)
	}
	input := production.CreateAssetInput{UploadUUID: uploadUUID, FileUUID: fileUUID, ToolExecutionUUID: execution.UUID, ChatThreadUUID: tc.Thread.UUID, AssetType: stringArg(args, "asset_type"), Title: stringArg(args, "title"), Summary: stringArg(args, "summary"), Tags: stringSliceArg(args, "tags"), SourceType: "manual"}
	if fileUUID != "" {
		return service.CreatePremiseAssetFromFile(ctx, input)
	}
	return service.ImportPremiseAsset(ctx, input)
}

func updatePremiseAssetTool(ctx context.Context, service *production.Service, tc toolContext, execution toolExecutionRecord, args map[string]any) (production.PremiseAsset, error) {
	uuid := stringArg(args, "premise_asset_uuid")
	current, err := service.GetPremiseAsset(ctx, uuid)
	if err != nil {
		return production.PremiseAsset{}, err
	}
	input := production.UpdateAssetInput{ExpectedRevision: intArg(args, "expected_revision"), FileUUID: stringArg(args, "file_uuid"), ToolExecutionUUID: execution.UUID, ChatThreadUUID: tc.Thread.UUID}
	if value, ok := args["asset_type"].(string); ok {
		input.AssetType = &value
	}
	if value, ok := args["title"].(string); ok {
		input.Title = &value
	}
	if value, ok := args["summary"].(string); ok {
		input.Summary = &value
	}
	if _, ok := args["tags"]; ok {
		value := stringSliceArg(args, "tags")
		input.Tags = &value
	}
	if input.FileUUID != "" {
		return service.UpdatePremiseAssetFromFile(ctx, uuid, input)
	}
	if current.Revision == input.ExpectedRevision && premiseAssetMatches(current, input) {
		return current, nil
	}
	return service.UpdatePremiseAsset(ctx, uuid, input)
}

func premiseAssetMatches(current production.PremiseAsset, input production.UpdateAssetInput) bool {
	if input.AssetType != nil && current.AssetType != strings.TrimSpace(*input.AssetType) {
		return false
	}
	if input.Title != nil && current.Title != strings.TrimSpace(*input.Title) {
		return false
	}
	if input.Summary != nil && current.Summary != strings.TrimSpace(*input.Summary) {
		return false
	}
	if input.Tags != nil {
		currentTags, wanted := append([]string(nil), current.Tags...), append([]string(nil), (*input.Tags)...)
		if strings.Join(currentTags, "\x00") != strings.Join(wanted, "\x00") {
			return false
		}
	}
	return true
}

func updateStoryboardTool(ctx context.Context, service *production.Service, args map[string]any) (production.ComicSection, error) {
	chapterUUID, sectionUUID, desired := stringArg(args, "chapter_uuid"), stringArg(args, "section_uuid"), stringArg(args, "content_md")
	current, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err != nil {
		return production.ComicSection{}, err
	}
	if current.CurrentStoryboard != nil && strings.TrimSpace(current.CurrentStoryboard.ContentMD) == strings.TrimSpace(desired) {
		return current, nil
	}
	return service.CreateStoryboard(ctx, chapterUUID, sectionUUID, desired, "generated", intArg(args, "expected_revision"))
}

func (service *Service) startGenerationTool(ctx context.Context, tc toolContext, execution toolExecutionRecord, args map[string]any) (DomainTask, error) {
	request := DomainTaskRequest{Kind: stringArg(args, "kind"), ResourceUUID: stringArg(args, "resource_uuid"), ChapterUUID: stringArg(args, "chapter_uuid"), ProviderUUID: tc.Run.ProviderUUID, Model: stringArg(args, "model"), Prompt: stringArg(args, "prompt"), PremiseAssetUUIDs: stringSliceArg(args, "premise_asset_uuids"), IdempotencyKey: execution.IdempotencyKey}
	return service.queue.StartDomainTask(ctx, tc.ProjectUUID, request)
}

func (service *Service) startPremiseAssetGenerationTool(ctx context.Context, tc toolContext, execution toolExecutionRecord, args map[string]any, variant bool) (DomainTask, error) {
	request := DomainTaskRequest{
		Kind: KindPremiseAssetGeneration, ResourceUUID: tc.Thread.UUID,
		ProviderUUID: tc.Run.ProviderUUID, Model: stringArg(args, "model"),
		Prompt: stringArg(args, "prompt"), AssetOperation: "create",
		AssetType: stringArg(args, "asset_type"), AssetTitle: stringArg(args, "title"),
		AssetSummary: stringArg(args, "summary"), AssetTags: stringSliceArg(args, "tags"),
		IdempotencyKey: execution.IdempotencyKey,
	}
	if variant {
		if tc.Thread.Scene != SceneAssetReference || !isUUIDv7(tc.Thread.SubjectUUID) {
			return DomainTask{}, domainError(CodeToolNotAllowed, "当前 Thread 没有设定项引用", "请从设定项卡片重新打开引用会话。", nil)
		}
		request.ResourceUUID = tc.Thread.SubjectUUID
		request.AssetOperation = "variant"
	}
	return service.queue.StartDomainTask(ctx, tc.ProjectUUID, request)
}

const KindPremiseAssetGeneration = "premise_asset_generation"

func toolAllowedForThread(name string, thread threadRecord) bool {
	if isStoryboardReferenceThread(thread) {
		return name == "get_comic_section" || name == "update_comic_storyboard" || name == "request_user_input"
	}
	if thread.Scope != ThreadScopePremise || thread.Scene == "" {
		return name != "image_gen" && name != "generate_premise_asset" && name != "generate_premise_asset_variant" && name != "request_current_project_api"
	}
	allowed := map[string]bool{"get_premise": true, "list_premise_assets": true, "get_premise_asset": true, "request_user_input": true}
	if thread.Scene == ScenePremiseAsset {
		allowed["image_gen"] = true
		allowed["create_premise_asset"] = true
		return allowed[name]
	}
	if thread.Scene == SceneAssetReference {
		return name == "request_current_project_api" || name == "image_gen" || name == "request_user_input"
	}
	return false
}

func toolTargetAllowedForThread(name string, args map[string]any, thread threadRecord) bool {
	if isStoryboardReferenceThread(thread) {
		switch name {
		case "get_comic_section", "update_comic_storyboard":
			return stringArg(args, "section_uuid") == thread.SubjectUUID
		default:
			return true
		}
	}
	if thread.Scene != SceneAssetReference {
		return true
	}
	switch name {
	case "get_premise_asset", "update_premise_asset":
		return stringArg(args, "premise_asset_uuid") == thread.SubjectUUID
	default:
		return true
	}
}

func toolErrorResult(err error) json.RawMessage {
	code, message := CodeToolValidation, "工具调用失败。"
	var agentErr *Error
	if errors.As(err, &agentErr) {
		code, message = agentErr.Code, agentErr.Message
	} else {
		var storyErr *story.Error
		var productionErr *production.Error
		var imageErr *imagegen.Error
		switch {
		case errors.As(err, &storyErr):
			code, message = storyErr.Code, storyErr.Message
		case errors.As(err, &productionErr):
			code, message = productionErr.Code, productionErr.Message
		case errors.As(err, &imageErr):
			code, message = imageErr.Code, imageErr.SafeMessage
		}
	}
	encoded, _ := json.Marshal(map[string]any{"success": false, "data": nil, "error": map[string]any{"code": code, "message": message, "details": ""}})
	return encoded
}

func compactToolResult(value any, targetUUID string) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return toolErrorResult(domainError(CodeToolValidation, "工具结果无法编码", "result 不是有效 JSON。", err))
	}
	if len(encoded) <= MaxToolResult {
		return encoded
	}
	previewBytes := encoded
	if len(previewBytes) > 8<<10 {
		previewBytes = previewBytes[:8<<10]
	}
	compacted, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"compacted": true, "target_uuid": targetUUID, "byte_size": len(encoded), "preview": string(previewBytes)}})
	return compacted
}

func (service *Service) persistToolResult(ctx context.Context, store *project.Store, tc toolContext, execution toolExecutionRecord, result json.RawMessage) error {
	if len(result) > MaxToolResult || !json.Valid(result) {
		return domainError(CodeResultTooLarge, "工具结果过大或无效", "结果必须是不超过限制的有效 JSON。", nil)
	}
	sqlDB, err := store.DB().DB()
	if err != nil {
		return err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	thread, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
	if err != nil {
		return err
	}
	var state string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM agent_tool_executions WHERE id=?`, execution.ID).Scan(&state); err != nil {
		return err
	}
	if state == "completed" {
		return tx.Commit()
	}
	now := service.now().UTC()
	metadata := map[string]any{"purpose": execution.ToolName, "target_uuid": execution.TargetUUID}
	var args map[string]any
	_ = json.Unmarshal([]byte(execution.ArgumentsJSON), &args)
	if value, ok := args["__provider_call_id"].(string); ok {
		metadata["provider_call_id"] = value
	}
	item, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "tool_result", "tool", string(result), "json", "completed", execution.ToolCallUUID, execution.ToolName, execution.TargetUUID, metadata, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_tool_executions SET state='completed',result_json=?,completed_at=?,updated_at=?,error_code='',error_message='' WHERE id=?`, string(result), now, now, execution.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_items SET status='completed' WHERE id=? AND status='in_progress'`, execution.ItemID); err != nil {
		return err
	}
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "tool_result", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "tool_call_uuid": execution.ToolCallUUID, "tool_name": execution.ToolName, "target_uuid": execution.TargetUUID, "item_uuid": item.UUID}, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:tool_result", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "tool_call_uuid": execution.ToolCallUUID, "tool_name": execution.ToolName, "target_uuid": execution.TargetUUID, "status": "completed"})
	return nil
}

func (service *Service) createUserInputRequest(ctx context.Context, store *project.Store, tc toolContext, execution toolExecutionRecord) (UserInputRequest, error) {
	var args struct {
		InputType string `json:"input_type"`
		Question  string `json:"question"`
		Options   []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"options"`
	}
	if err := json.Unmarshal([]byte(execution.ArgumentsJSON), &args); err != nil {
		return UserInputRequest{}, domainError(CodeToolValidation, "用户输入参数无效", "无法解析 request_user_input。", err)
	}
	question := strings.TrimSpace(args.Question)
	if (args.InputType != "single_choice" && args.InputType != "multiple_choice") || question == "" || len([]rune(question)) > 4000 || len(args.Options) < 2 || len(args.Options) > 8 {
		return UserInputRequest{}, domainError(CodeToolValidation, "用户输入请求无效", "问题和选项不符合限制。", nil)
	}
	options := make([]UserInputOption, 0, len(args.Options))
	for _, candidate := range args.Options {
		label := strings.TrimSpace(candidate.Label)
		description := strings.TrimSpace(candidate.Description)
		if label == "" || len([]rune(label)) > 160 || len([]rune(description)) > 1000 {
			return UserInputRequest{}, domainError(CodeToolValidation, "用户输入选项无效", "选项标签不能为空且最多 160 字符。", nil)
		}
		uuid, err := newUUIDv7()
		if err != nil {
			return UserInputRequest{}, err
		}
		options = append(options, UserInputOption{UUID: uuid, Label: label, Description: description})
	}
	requestUUID, err := newUUIDv7()
	if err != nil {
		return UserInputRequest{}, err
	}
	sqlDB, err := store.DB().DB()
	if err != nil {
		return UserInputRequest{}, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return UserInputRequest{}, err
	}
	defer tx.Rollback()
	thread, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
	if err != nil {
		return UserInputRequest{}, err
	}
	var existing userInputRow
	err = tx.QueryRowContext(ctx, `SELECT q.id,q.thread_id,q.run_id,q.turn_id,q.item_id,q.uuid,q.tool_call_uuid,q.input_type,q.question,q.options_json,q.response_json,q.status,q.answered_at,q.resumed_at,q.cancelled_at,q.created_at,q.updated_at,r.uuid,t.uuid,i.uuid FROM chat_user_input_requests q JOIN chat_runs r ON r.id=q.run_id JOIN chat_turns t ON t.id=q.turn_id JOIN chat_items i ON i.id=q.item_id WHERE q.run_id=? AND q.tool_call_uuid=?`, tc.Run.ID, execution.ToolCallUUID).Scan(&existing.ID, &existing.ThreadID, &existing.RunID, &existing.TurnID, &existing.ItemID, &existing.UUID, &existing.ToolCallUUID, &existing.InputType, &existing.Question, &existing.OptionsJSON, &existing.ResponseJSON, &existing.Status, &existing.AnsweredAt, &existing.ResumedAt, &existing.CancelledAt, &existing.CreatedAt, &existing.UpdatedAt, &existing.RunUUID, &existing.TurnUUID, &existing.ItemUUID)
	if err == nil {
		existing.ThreadUUID = tc.Thread.UUID
		return existing.DTO(), tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return UserInputRequest{}, err
	}
	now := service.now().UTC()
	optionsJSON, _ := json.Marshal(options)
	content, _ := json.Marshal(map[string]any{"request_uuid": requestUUID, "input_type": args.InputType, "question": question, "options": options})
	item, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "user_input_request", "assistant", string(content), "json", "completed", execution.ToolCallUUID, "request_user_input", requestUUID, map[string]any{"purpose": "request_user_input"}, now)
	if err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO chat_user_input_requests(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,input_type,question,options_json,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,'pending',?,?)`, requestUUID, thread.ID, tc.Run.ID, tc.Turn.ID, item.ID, execution.ToolCallUUID, args.InputType, question, string(optionsJSON), now, now); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_tool_executions SET state='executing',started_at=COALESCE(started_at,?),updated_at=? WHERE id=?`, now, now, execution.ID); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status='waiting_for_input',updated_at=? WHERE id=? AND status='in_progress'`, now, tc.Run.ID); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status='waiting_for_input',updated_at=? WHERE id=? AND status='in_progress'`, now, tc.Turn.ID); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='waiting_for_input',next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "user_input_requested", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "request_uuid": requestUUID, "tool_call_uuid": execution.ToolCallUUID}, now); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_event_sequence=? WHERE id=?`, thread.NextEventSequence, thread.ID); err != nil {
		return UserInputRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return UserInputRequest{}, err
	}
	request := UserInputRequest{UUID: requestUUID, ThreadUUID: tc.Thread.UUID, RunUUID: tc.Run.UUID, TurnUUID: tc.Turn.UUID, ItemUUID: item.UUID, ToolCallUUID: execution.ToolCallUUID, InputType: args.InputType, Question: question, Options: options, Status: "pending", CreatedAt: now, UpdatedAt: now}
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:user_input_requested", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "request_uuid": requestUUID, "status": "pending"})
	return request, nil
}

// Assert the compiler keeps the SQL transaction boundary used by tool intent.
var _ *sql.Tx
var _ = fmt.Sprintf
