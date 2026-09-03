package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"lumi/internal/llm"
	"lumi/internal/project"
)

const bootstrapRuntimeProviderCallDomain = "lumi/provider-call/v1\x00bootstrap-runtime\x00"

func bootstrapRuntimeProviderCallID(action, creationSessionUUID, discriminator string) string {
	digest := sha256.Sum256([]byte(bootstrapRuntimeProviderCallDomain + action + "\x00" + creationSessionUUID + "\x00" + discriminator))
	return "call_" + hex.EncodeToString(digest[:12])
}

// reconcileBootstrapLifecycle advances server-owned initialization transitions
// before another model request can complete the Turn prematurely. The model
// still proposes setup content, while finalization confirmation and automatic
// Workflow startup are durable runtime intents.
func (service *Service) reconcileBootstrapLifecycle(ctx context.Context, store *project.Store, tc toolContext) (bool, error) {
	originSessionUUID := strings.TrimSpace(tc.BootstrapCreationSessionUUID)
	lineageSessionUUID := strings.TrimSpace(tc.BootstrapLineageCreationSessionUUID)
	if !isUUIDv7(originSessionUUID) && !isUUIDv7(lineageSessionUUID) {
		return false, nil
	}
	setup, err := store.ProjectSetup(ctx)
	if err != nil {
		return false, err
	}
	if setup.SetupStatus == project.SetupStatusDraft {
		if setup.Status != project.SetupDraftStatusPendingConfirmation || len(setup.MissingInformation) != 0 {
			return false, nil
		}
		creationSessionUUID := originSessionUUID
		if !isUUIDv7(creationSessionUUID) {
			if !isUUIDv7(lineageSessionUUID) || !bootstrapExplicitRecoveryRequest(tc.Turn.InputText) {
				return false, nil
			}
			creationSessionUUID = lineageSessionUUID
		}
		attempted, err := bootstrapFinalizationAttempted(ctx, store, tc.Run.ID, setup.Revision)
		if err != nil || attempted {
			return false, err
		}
		arguments, _ := json.Marshal(map[string]any{
			"method": "POST",
			"url":    "/api/v1/projects/" + tc.ProjectUUID + "/project-setup-finalizations",
			"request_body": map[string]any{
				"expected_revision": setup.Revision,
			},
			"response_filter": ".data | {uuid,project_uuid,setup_status,status,revision,draft_values,field_sources,final_picture_book,reference_plan,updated_at}",
		})
		providerCallID := bootstrapRuntimeProviderCallID("finalize", creationSessionUUID, fmt.Sprintf("%d", setup.Revision))
		return service.persistRuntimeBootstrapRequestIntent(ctx, store, tc, providerCallID, string(arguments), map[string]any{
			"__bootstrap_action":                "finalize",
			"__bootstrap_creation_session_uuid": creationSessionUUID,
		}, map[string]any{"bootstrap_action": "finalize"})
	}
	if setup.SetupStatus != project.SetupStatusReady {
		return false, nil
	}

	creationSessionUUID := originSessionUUID
	allowThreadEvidence := false
	if !isUUIDv7(creationSessionUUID) {
		creationSessionUUID = lineageSessionUUID
		allowThreadEvidence = true
	}
	if !isUUIDv7(creationSessionUUID) {
		return false, nil
	}
	exists, err := bootstrapYoloWorkflowExists(ctx, store, tc.Thread.ProjectID, creationSessionUUID)
	if err != nil || exists {
		return false, err
	}
	authorizedContext := tc
	authorizedContext.BootstrapCreationSessionUUID = creationSessionUUID
	evidence, authorized, err := bootstrapYoloAuthorizationEvidence(ctx, store, authorizedContext, allowThreadEvidence)
	if err != nil || !authorized {
		return false, err
	}
	if evidence.RunID != tc.Run.ID && !bootstrapExplicitRecoveryRequest(tc.Turn.InputText) {
		return false, nil
	}
	storyPrompt := bootstrapGenerationBrief(ctx, store, setup, tc.Thread.ID, evidence)
	if storyPrompt == "" {
		return false, domainError(
			CodeBootstrapGenerationBriefMissing,
			"自动生成 Brief 缺失",
			"已定稿的 bootstrap 没有可恢复的 generation_brief，未启动自动生成流程。",
			nil,
		)
	}
	arguments, _ := json.Marshal(map[string]any{
		"method": "POST",
		"url":    "/api/v1/projects/" + tc.ProjectUUID + "/workflows",
		"request_body": map[string]any{
			"story_prompt": storyPrompt,
		},
		"response_filter": ".data | {uuid,thread_uuid,presentation_mode,kind,title,status,current_step_key,steps}",
	})
	runtimeContext := tc
	runtimeContext.BootstrapCreationSessionUUID = creationSessionUUID
	runtimeContext.RequestUUID, runtimeContext.RequestOrdinal = "", 0
	providerCallID := bootstrapRuntimeProviderCallID("start_generation", creationSessionUUID, evidence.RequestUUID)
	return service.persistRuntimeBootstrapRequestIntent(ctx, store, runtimeContext, providerCallID, string(arguments), map[string]any{
		"__bootstrap_action":                    "start_generation",
		"__bootstrap_creation_session_uuid":     creationSessionUUID,
		"__bootstrap_confirmation_request_uuid": evidence.RequestUUID,
	}, map[string]any{
		"bootstrap_action":                    "start_generation",
		"bootstrap_confirmation_request_uuid": evidence.RequestUUID,
	})
}

func bootstrapFinalizationAttempted(ctx context.Context, store *project.Store, runID, revision int64) (bool, error) {
	var argumentsJSON []string
	if err := store.DB().WithContext(ctx).Table("agent_tool_executions").
		Where("run_id=? AND tool_name='request_api' AND json_extract(arguments_json,'$.__route_id')=?", runID, RouteProjectSetupFinalize).
		Pluck("arguments_json", &argumentsJSON).Error; err != nil {
		return false, err
	}
	route, _ := agentAPIRouteByID(RouteProjectSetupFinalize)
	for _, raw := range argumentsJSON {
		var arguments map[string]any
		if json.Unmarshal([]byte(raw), &arguments) == nil && storedAgentAPIExpectedRevision(route, arguments) == revision {
			return true, nil
		}
	}
	return false, nil
}

func bootstrapYoloWorkflowExists(ctx context.Context, store *project.Store, projectID int64, creationSessionUUID string) (bool, error) {
	var count int64
	err := store.DB().WithContext(ctx).Table("workflows").
		Where("project_id=? AND kind=? AND idempotency_key=?", projectID, WorkflowYolo, bootstrapYoloIdempotencyPrefix+creationSessionUUID).
		Count(&count).Error
	return count > 0, err
}

func bootstrapGenerationBrief(ctx context.Context, store *project.Store, setup project.SetupState, threadID int64, evidence bootstrapConfirmationEvidence) string {
	brief := strings.TrimSpace(setup.DraftValues.GenerationBrief)
	if setup.FieldSources["generation_brief"] == project.SetupSourceSystemDefault {
		if recovered := persistedBootstrapStoryPrompt(ctx, store, threadID, evidence); recovered != "" {
			brief = recovered
		}
	}
	if brief == "" {
		brief = strings.TrimSpace(setup.OriginalInput)
	}
	runes := []rune(brief)
	if len(runes) > 4000 {
		runes = runes[:4000]
	}
	return strings.TrimSpace(string(runes))
}

func persistedBootstrapStoryPrompt(ctx context.Context, store *project.Store, threadID int64, evidence bootstrapConfirmationEvidence) string {
	var rows []struct {
		ArgumentsJSON string
	}
	if err := store.DB().WithContext(ctx).Table("agent_tool_executions").
		Select("arguments_json").
		Where("thread_id=? AND run_id=? AND id>? AND tool_name='request_api' AND json_extract(arguments_json,'$.__route_id')=?", threadID, evidence.RunID, evidence.ExecutionID, RouteYoloWorkflowCreate).
		Order("id DESC").Scan(&rows).Error; err != nil {
		return ""
	}
	for _, row := range rows {
		var arguments map[string]any
		if json.Unmarshal([]byte(row.ArgumentsJSON), &arguments) != nil {
			continue
		}
		body, _ := arguments["request_body"].(map[string]any)
		value := strings.TrimSpace(stringArg(body, "story_prompt"))
		if value != "" && len([]rune(value)) <= 4000 {
			return value
		}
	}
	return ""
}

func bootstrapExplicitRecoveryRequest(input string) bool {
	value := strings.ToLower(strings.Trim(strings.TrimSpace(input), "。.!！?？"))
	switch value {
	case "继续", "继续生成", "继续开始生成", "开始生成", "重试", "重试生成", "continue", "continue generation", "start generation", "retry", "retry generation":
		return true
	default:
		return false
	}
}

func (service *Service) persistRuntimeBootstrapRequestIntent(ctx context.Context, store *project.Store, tc toolContext, providerCallID, raw string, internalArguments, runtimeMetadata map[string]any) (bool, error) {
	call := llm.ToolCall{ID: providerCallID, Name: "request_api", Arguments: raw}
	intent, err := service.prepareToolIntent(ctx, store, tc, call)
	if err != nil {
		return false, err
	}
	if !intent.New {
		var persisted map[string]any
		if json.Unmarshal([]byte(intent.Existing.ArgumentsJSON), &persisted) != nil || !boolArg(persisted, "__runtime_generated_bootstrap") {
			return false, domainError(CodeStateConflict, "Bootstrap 运行时调用身份冲突", "运行时 Provider call ID 已被非运行时 Tool Intent 占用。", nil)
		}
		for key, expected := range internalArguments {
			expectedText, ok := expected.(string)
			if !ok || stringArg(persisted, key) != expectedText {
				return false, domainError(CodeStateConflict, "Bootstrap 运行时调用绑定冲突", "已持久化的 Bootstrap Tool Intent 与当前 session、action 或确认请求不匹配。", nil)
			}
		}
		return !intent.Completed, nil
	}
	var storedArguments map[string]any
	if err := json.Unmarshal([]byte(intent.EncodedArguments), &storedArguments); err != nil {
		return false, err
	}
	storedArguments["__runtime_generated_bootstrap"] = true
	for key, value := range internalArguments {
		if !strings.HasPrefix(key, "__bootstrap_") {
			return false, domainError(CodeStateConflict, "Bootstrap 内部参数无效", "运行时 Bootstrap 参数必须使用保留前缀。", nil)
		}
		storedArguments[key] = value
	}
	encoded, err := json.Marshal(storedArguments)
	if err != nil {
		return false, err
	}
	intent.EncodedArguments = string(encoded)
	intent.RuntimeGenerated = true
	intent.RuntimeMetadata = runtimeMetadata
	persisted, err := service.persistPreparedToolIntentBatch(ctx, store, tc, []preparedToolIntent{intent})
	if err != nil {
		return false, err
	}
	return len(persisted) == 1 && !persisted[0].Completed, nil
}
