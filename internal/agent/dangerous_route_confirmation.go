package agent

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"lumi/internal/project"
)

type dangerousConfirmationBinding struct {
	Route              string `json:"route"`
	ProjectUUID        string `json:"project_uuid"`
	TargetUUID         string `json:"target_uuid"`
	ExpectedRevision   int64  `json:"expected_revision"`
	RequestFingerprint string `json:"request_fingerprint"`
	QuestionID         string `json:"question_id,omitempty"`
	ConfirmOption      int    `json:"confirm_option"`
}

const (
	confirmationReplayProviderCallDomain   = "lumi/provider-call/v1\x00confirmation-replay\x00"
	confirmationRequestProviderCallDomain  = "lumi/provider-call/v1\x00confirmation-request\x00"
	runtimeDangerousConfirmationQuestionID = "confirm_dangerous_action"
)

func confirmationReplayProviderCallID(confirmationRequestUUID string) string {
	digest := sha256.Sum256([]byte(confirmationReplayProviderCallDomain + confirmationRequestUUID))
	return "call_" + hex.EncodeToString(digest[:12])
}

func confirmationRequestProviderCallID(sourceExecutionUUID string) string {
	digest := sha256.Sum256([]byte(confirmationRequestProviderCallDomain + sourceExecutionUUID))
	return "call_" + hex.EncodeToString(digest[:12])
}

func supportsRuntimeDangerousConfirmation(tc toolContext) bool {
	mode := normalizedToolMode(tc.ToolMode)
	return mode == ToolModeProjectAPI || mode == ToolModeLegacyTyped
}

type runtimeDangerousConfirmationCopy struct {
	Header, Question, SafeLabel, SafeDescription, ConfirmLabel, ConfirmDescription string
}

func runtimeDangerousConfirmationPresentation(route agentAPIRoute, language string) (string, runtimeDangerousConfirmationCopy) {
	if language == project.GenerationLanguageEnglish {
		if route.ID == RouteProjectSetupFinalize {
			return bootstrapYoloConfirmationQuestionID, runtimeDangerousConfirmationCopy{
				Header:             "Setup",
				Question:           "Finalize these project settings and start automatic generation?",
				SafeLabel:          "Keep editing (Recommended)",
				SafeDescription:    "Keep the setup as a draft so it can still be changed.",
				ConfirmLabel:       "Finalize and start generation",
				ConfirmDescription: "Finalize the setup and start the automatic generation flow.",
			}
		}
		return runtimeDangerousConfirmationQuestionID, runtimeDangerousConfirmationCopy{
			Header:             "Confirm",
			Question:           "This action may overwrite, delete, or change existing content. Continue?",
			SafeLabel:          "Do not proceed (Recommended)",
			SafeDescription:    "Keep the current state and do not perform this action.",
			ConfirmLabel:       "Proceed",
			ConfirmDescription: "Perform the action bound to the current resource version.",
		}
	}
	if route.ID == RouteProjectSetupFinalize {
		return bootstrapYoloConfirmationQuestionID, runtimeDangerousConfirmationCopy{
			Header:             "创建确认",
			Question:           "是否定稿当前项目设置并开始自动生成？",
			SafeLabel:          "继续修改 (Recommended)",
			SafeDescription:    "保留草稿状态，可以继续调整项目设置。",
			ConfirmLabel:       "定稿并开始生成",
			ConfirmDescription: "定稿当前设置，并开始自动生成流程。",
		}
	}
	return runtimeDangerousConfirmationQuestionID, runtimeDangerousConfirmationCopy{
		Header:             "操作确认",
		Question:           "是否确认" + route.Action + "？",
		SafeLabel:          "暂不执行 (Recommended)",
		SafeDescription:    "保持当前状态，不执行该操作。",
		ConfirmLabel:       "确认执行",
		ConfirmDescription: "执行“" + route.Action + "”，并绑定当前资源版本。",
	}
}

func validateDangerousConfirmationBinding(tc toolContext, binding dangerousConfirmationBinding, inputType string, optionCount int) (agentAPIRoute, error) {
	return validateDangerousConfirmationBindingWithRoutes(tc, binding, inputType, optionCount, agentAPIRoutes())
}

func (service *Service) validateDangerousConfirmationBinding(tc toolContext, binding dangerousConfirmationBinding, inputType string, optionCount int) (agentAPIRoute, error) {
	return validateDangerousConfirmationBindingWithRoutes(tc, binding, inputType, optionCount, service.requestAPIRoutes())
}

func validateDangerousConfirmationBindingWithRoutes(tc toolContext, binding dangerousConfirmationBinding, inputType string, optionCount int, routes []agentAPIRoute) (agentAPIRoute, error) {
	route, ok := agentAPIRouteByIDFromRoutes(strings.TrimSpace(binding.Route), routes)
	if !ok || route.Risk != RiskDangerous || !route.RequiresConfirmation {
		return agentAPIRoute{}, domainError(CodeToolValidation, "危险操作确认 Route 无效", "confirmation.route 必须是要求全局确认的已注册危险 Route。", nil)
	}
	if inputType != "single_choice" || binding.ProjectUUID != tc.ProjectUUID || !isUUIDv7(binding.ProjectUUID) || !isUUIDv7(binding.TargetUUID) || !validDangerousConfirmationRevision(route, binding.ExpectedRevision) || !validAgentRequestFingerprint(binding.RequestFingerprint) || binding.ConfirmOption < 0 || binding.ConfirmOption >= optionCount {
		return agentAPIRoute{}, domainError(CodeToolValidation, "危险操作确认绑定无效", "confirmation 必须绑定当前 Project、具体目标 UUID、稳定 request_fingerprint、非负 revision 和有效的单选确认项。", nil)
	}
	return route, nil
}

func (service *Service) validateCodexDangerousConfirmationBinding(tc toolContext, binding dangerousConfirmationBinding, questions []UserInputQuestion) (agentAPIRoute, error) {
	route, ok := agentAPIRouteByIDFromRoutes(strings.TrimSpace(binding.Route), service.requestAPIRoutes())
	if !ok || route.Risk != RiskDangerous || !route.RequiresConfirmation {
		return agentAPIRoute{}, domainError(CodeToolValidation, "危险操作确认 Route 无效", "confirmation.route 必须是要求全局确认的已注册危险 Route。", nil)
	}
	if len(questions) != 1 || binding.QuestionID != questions[0].ID || binding.ProjectUUID != tc.ProjectUUID || !isUUIDv7(binding.ProjectUUID) || !isUUIDv7(binding.TargetUUID) || !validDangerousConfirmationRevision(route, binding.ExpectedRevision) || !validAgentRequestFingerprint(binding.RequestFingerprint) || binding.ConfirmOption <= 0 || binding.ConfirmOption >= len(questions[0].Options) {
		return agentAPIRoute{}, domainError(CodeToolValidation, "危险操作确认绑定无效", "confirmation 必须绑定当前 Project、唯一 question_id、具体目标 UUID、稳定 request_fingerprint、非负 revision 和非推荐的有效确认项；第一项保留为安全选项。", nil)
	}
	return route, nil
}

func validDangerousConfirmationRevision(route agentAPIRoute, revision int64) bool {
	if revision < 0 {
		return false
	}
	if route.RevisionSource == agentAPIRevisionNone {
		return revision == 0
	}
	return route.RevisionSource == agentAPIRevisionBody || route.RevisionSource == agentAPIRevisionQuery
}

func agentAPIRouteByID(routeID string) (agentAPIRoute, bool) {
	return agentAPIRouteByIDFromRoutes(routeID, agentAPIRoutes())
}

func agentAPIRouteByIDFromRoutes(routeID string, routes []agentAPIRoute) (agentAPIRoute, bool) {
	for _, route := range routes {
		if route.ID == routeID {
			return route, true
		}
	}
	return agentAPIRoute{}, false
}

func authorizeDangerousAgentAPIRequest(ctx context.Context, store *project.Store, tc toolContext, request agentAPIRequest) error {
	if request.Route.Risk != RiskDangerous || !request.Route.RequiresConfirmation {
		return nil
	}
	// The explicit-delete wording shortcut is a compatibility rule for the
	// original Premise Asset soft-delete route only. New dangerous routes must
	// never inherit a natural-language bypass merely because they share the
	// global confirmation policy.
	if request.Route.ID == RoutePremiseAssetDelete {
		explicit, err := currentRunExplicitlyRequestsDangerousAction(ctx, store, tc)
		if err != nil {
			return err
		}
		if explicit {
			return nil
		}
	}
	confirmed, err := hasMatchingDangerousConfirmation(ctx, store, tc, request)
	if err != nil {
		return err
	}
	if confirmed {
		return nil
	}
	details, _ := json.Marshal(map[string]any{
		"route": request.Route.ID, "project_uuid": tc.ProjectUUID, "target_uuid": request.TargetUUID,
		"expected_revision": agentAPIRequestExpectedRevision(request), "request_fingerprint": agentRequestFingerprint(request),
		"confirmation_tool": "request_user_input", "execution": "runtime_replays_original_request",
	})
	return domainError(CodeToolConfirmation, "危险操作需要用户确认", string(details), nil)
}

func currentRunExplicitlyRequestsDangerousAction(ctx context.Context, store *project.Store, tc toolContext) (bool, error) {
	var contents []string
	if err := store.DB().WithContext(ctx).Table("chat_items").Where("run_id=? AND item_type='user_message' AND role='user'", tc.Run.ID).Order("sequence,id").Pluck("content", &contents).Error; err != nil {
		return false, err
	}
	for _, content := range contents {
		text := strings.ToLower(strings.TrimSpace(content))
		if text == "" || containsAny(text, []string{"不要删除", "别删除", "不删除", "do not delete", "don't delete", "not delete"}) {
			continue
		}
		if text == "删除" || containsAny(text, []string{"请删除", "确认删除", "删除它", "删除这个", "删除当前", "移入回收站", "放入回收站", "delete it", "delete this", "delete the ", "move it to trash", "move this to trash"}) {
			return true, nil
		}
	}
	return false, nil
}

func containsAny(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

type dangerousConfirmationSourceExecution struct {
	ID, RunID                      int64
	UUID, ToolCallUUID, TargetUUID string
	ArgumentsJSON, ResultJSON      string
}

func dangerousConfirmationFromArguments(raw string) (*dangerousConfirmationBinding, error) {
	var arguments map[string]any
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
		return nil, domainError(CodeStateConflict, "危险操作确认参数损坏", "无法读取持久化的 confirmation。", err)
	}
	// Historical v4 records may contain the old, unambiguous single-question
	// nesting mistake. Repair it only while reading persisted state; live model
	// arguments never pass through this compatibility path.
	if _, exists := arguments["confirmation"]; !exists {
		normalizeProjectAPIV4ConfirmationPlacement(arguments)
	}
	if rawConfirmation, exists := arguments["confirmation"]; exists {
		encoded, err := json.Marshal(rawConfirmation)
		if err != nil {
			return nil, domainError(CodeStateConflict, "危险操作确认参数损坏", "无法读取持久化的 confirmation。", err)
		}
		var binding dangerousConfirmationBinding
		if err := json.Unmarshal(encoded, &binding); err != nil {
			return nil, domainError(CodeStateConflict, "危险操作确认参数损坏", "无法读取持久化的 confirmation。", err)
		}
		return &binding, nil
	}
	return nil, nil
}

func confirmationRequiredToolResult(raw string) bool {
	var result struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	return json.Unmarshal([]byte(raw), &result) == nil && result.Error.Code == CodeToolConfirmation
}

func (service *Service) findDangerousConfirmationSourceTx(ctx context.Context, tx *sql.Tx, runID, beforeExecutionID int64, projectUUID, requiredSourceUUID string, binding dangerousConfirmationBinding) (dangerousConfirmationSourceExecution, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,run_id,uuid,tool_call_uuid,target_uuid,arguments_json,result_json
		FROM agent_tool_executions
		WHERE run_id=? AND id<? AND tool_name='request_api' AND state='completed' AND result_json IS NOT NULL
		ORDER BY id DESC`, runID, beforeExecutionID)
	if err != nil {
		return dangerousConfirmationSourceExecution{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var source dangerousConfirmationSourceExecution
		if err := rows.Scan(&source.ID, &source.RunID, &source.UUID, &source.ToolCallUUID, &source.TargetUUID, &source.ArgumentsJSON, &source.ResultJSON); err != nil {
			return dangerousConfirmationSourceExecution{}, err
		}
		if requiredSourceUUID != "" && source.UUID != requiredSourceUUID {
			continue
		}
		var arguments map[string]any
		if json.Unmarshal([]byte(source.ArgumentsJSON), &arguments) != nil || !confirmationRequiredToolResult(source.ResultJSON) {
			continue
		}
		route, ok := agentAPIRouteByIDFromRoutes(binding.Route, service.requestAPIRoutes())
		if !ok || binding.ProjectUUID != projectUUID || stringArg(arguments, "__route_id") != binding.Route || source.TargetUUID != binding.TargetUUID || storedAgentAPIExpectedRevision(route, arguments) != binding.ExpectedRevision || storedAgentRequestFingerprint(arguments) != binding.RequestFingerprint {
			continue
		}
		return source, nil
	}
	if err := rows.Err(); err != nil {
		return dangerousConfirmationSourceExecution{}, err
	}
	return dangerousConfirmationSourceExecution{}, domainError(
		CodeToolValidation,
		"危险操作确认来源不存在",
		"confirmation 必须绑定当前 Run 中最近一次真实返回 agent_tool_confirmation_required 的 request_api；不要编造或修改 request_fingerprint。",
		nil,
	)
}

// ensureRuntimeDangerousConfirmationSourceTx enforces the durable trust
// boundary for every persisted confirmation. New records already carry both
// markers. Verified pre-upgrade records are backfilled here so frozen runs can
// resume without trusting or re-exposing a model-authored binding.
func (service *Service) ensureRuntimeDangerousConfirmationSourceTx(ctx context.Context, tx *sql.Tx, runID int64, executionID, itemID int64, projectUUID, expectedSourceUUID, rawArguments string, binding dangerousConfirmationBinding) (dangerousConfirmationSourceExecution, error) {
	var arguments map[string]any
	if err := json.Unmarshal([]byte(rawArguments), &arguments); err != nil {
		return dangerousConfirmationSourceExecution{}, domainError(CodeStateConflict, "危险操作确认参数损坏", "无法读取持久化的 confirmation 参数。", err)
	}
	runtimeGenerated := boolArg(arguments, "__runtime_generated_confirmation")
	markedSourceUUID := stringArg(arguments, "__confirmation_source_execution_uuid")
	if expectedSourceUUID != "" && markedSourceUUID != "" && markedSourceUUID != expectedSourceUUID {
		return dangerousConfirmationSourceExecution{}, domainError(CodeStateConflict, "危险操作确认来源不匹配", "持久化 confirmation 的来源 Execution UUID 不一致。", nil)
	}
	requiredSourceUUID := expectedSourceUUID
	if requiredSourceUUID == "" && markedSourceUUID != "" {
		requiredSourceUUID = markedSourceUUID
	}
	if runtimeGenerated && !isUUIDv7(requiredSourceUUID) {
		return dangerousConfirmationSourceExecution{}, domainError(CodeStateConflict, "危险操作确认来源损坏", "运行时 confirmation 必须携带有效的来源 Execution UUID。", nil)
	}
	source, err := service.findDangerousConfirmationSourceTx(ctx, tx, runID, executionID, projectUUID, requiredSourceUUID, binding)
	if err != nil {
		return dangerousConfirmationSourceExecution{}, err
	}
	if itemID > 0 {
		publicArguments, cloneErr := json.Marshal(arguments)
		if cloneErr != nil {
			return dangerousConfirmationSourceExecution{}, cloneErr
		}
		var publicValue map[string]any
		if cloneErr := json.Unmarshal(publicArguments, &publicValue); cloneErr != nil {
			return dangerousConfirmationSourceExecution{}, cloneErr
		}
		stripJSONField(publicValue, "confirmation")
		for key := range publicValue {
			if strings.HasPrefix(key, "__") {
				delete(publicValue, key)
			}
		}
		publicArguments, cloneErr = json.Marshal(publicValue)
		if cloneErr != nil {
			return dangerousConfirmationSourceExecution{}, cloneErr
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_items SET content=?,metadata_json=json_set(COALESCE(metadata_json,'{}'),'$.runtime_generated',json('true'),'$.confirmation_source_execution_uuid',?) WHERE id=?`, string(publicArguments), source.UUID, itemID); err != nil {
			return dangerousConfirmationSourceExecution{}, err
		}
	}
	if runtimeGenerated && markedSourceUUID == source.UUID {
		return source, nil
	}
	arguments["__runtime_generated_confirmation"] = true
	arguments["__confirmation_source_execution_uuid"] = source.UUID
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return dangerousConfirmationSourceExecution{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_tool_executions SET arguments_json=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, string(encoded), executionID); err != nil {
		return dangerousConfirmationSourceExecution{}, err
	}
	return source, nil
}

// recoverRuntimeDangerousConfirmation repairs a pre-upgrade gap before the
// next model request. It adopts only historically persisted confirmations
// whose complete binding matches a real source execution; otherwise it creates
// the canonical protocol-specific intent with a deterministic key.
func (service *Service) recoverRuntimeDangerousConfirmation(ctx context.Context, store *project.Store, tc toolContext) (toolExecutionRecord, bool, error) {
	if !supportsRuntimeDangerousConfirmation(tc) {
		return toolExecutionRecord{}, false, nil
	}
	sqlDB, err := store.DB().DB()
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	defer tx.Rollback()
	thread, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,thread_id,run_id,turn_id,item_id,uuid,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,result_json,created_at,updated_at
		FROM agent_tool_executions
		WHERE run_id=? AND tool_name='request_api' AND state='completed' AND result_json IS NOT NULL
		  AND json_extract(result_json,'$.error.code')=?
		ORDER BY id`, tc.Run.ID, CodeToolConfirmation)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	var sources []toolExecutionRecord
	for rows.Next() {
		var source toolExecutionRecord
		if err := rows.Scan(&source.ID, &source.ThreadID, &source.RunID, &source.TurnID, &source.ItemID, &source.UUID, &source.ToolCallUUID, &source.ToolName, &source.TargetUUID, &source.ArgumentsJSON, &source.IdempotencyKey, &source.State, &source.ResultJSON, &source.CreatedAt, &source.UpdatedAt); err != nil {
			rows.Close()
			return toolExecutionRecord{}, false, err
		}
		sources = append(sources, source)
	}
	if err := rows.Close(); err != nil {
		return toolExecutionRecord{}, false, err
	}
	for _, source := range sources {
		var handled int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_tool_executions
			WHERE run_id=? AND tool_name='request_user_input'
			  AND json_extract(arguments_json,'$.__runtime_generated_confirmation')=1
			  AND json_extract(arguments_json,'$.__confirmation_source_execution_uuid')=?`, tc.Run.ID, source.UUID).Scan(&handled); err != nil {
			return toolExecutionRecord{}, false, err
		}
		if handled > 0 {
			continue
		}

		confirmationRows, err := tx.QueryContext(ctx, `SELECT id,item_id,arguments_json FROM agent_tool_executions WHERE run_id=? AND id>? AND tool_name='request_user_input' ORDER BY id`, tc.Run.ID, source.ID)
		if err != nil {
			return toolExecutionRecord{}, false, err
		}
		type historicalConfirmation struct {
			executionID, itemID int64
			argumentsJSON       string
		}
		var historical []historicalConfirmation
		for confirmationRows.Next() {
			var candidate historicalConfirmation
			if err := confirmationRows.Scan(&candidate.executionID, &candidate.itemID, &candidate.argumentsJSON); err != nil {
				confirmationRows.Close()
				return toolExecutionRecord{}, false, err
			}
			historical = append(historical, candidate)
		}
		if err := confirmationRows.Close(); err != nil {
			return toolExecutionRecord{}, false, err
		}
		historicalHandled := false
		for _, candidate := range historical {
			binding, bindingErr := dangerousConfirmationFromArguments(candidate.argumentsJSON)
			if bindingErr != nil || binding == nil {
				continue
			}
			if recoveredSource, recoverErr := service.ensureRuntimeDangerousConfirmationSourceTx(ctx, tx, tc.Run.ID, candidate.executionID, candidate.itemID, tc.ProjectUUID, source.UUID, candidate.argumentsJSON, *binding); recoverErr == nil && recoveredSource.UUID == source.UUID {
				historicalHandled = true
				break
			}
		}
		if historicalHandled {
			continue
		}
		intent, created, err := service.enqueueRuntimeDangerousConfirmationTx(ctx, tx, &thread, tc, source, json.RawMessage(*source.ResultJSON), service.now().UTC())
		if err != nil {
			return toolExecutionRecord{}, false, err
		}
		if !created {
			continue
		}
		now := service.now().UTC()
		if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
			return toolExecutionRecord{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return toolExecutionRecord{}, false, err
		}
		service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:tool_call", map[string]any{
			"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID,
			"tool_call_uuid": intent.ToolCallUUID, "tool_name": intent.ToolName, "action": intent.Action,
			"target_uuid": intent.TargetUUID, "status": "in_progress", "runtime_generated": true,
			"confirmation_source_execution_uuid": source.UUID,
		})
		return intent, true, nil
	}
	if err := tx.Commit(); err != nil {
		return toolExecutionRecord{}, false, err
	}
	return toolExecutionRecord{}, false, nil
}

func publicStoredToolArguments(arguments map[string]any) map[string]any {
	result := make(map[string]any, len(arguments))
	for key, value := range arguments {
		if !strings.HasPrefix(key, "__") {
			result[key] = value
		}
	}
	return result
}

func stripJSONField(value any, field string) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, field)
		for _, child := range typed {
			stripJSONField(child, field)
		}
	case []any:
		for _, child := range typed {
			stripJSONField(child, field)
		}
	}
}

// enqueueRuntimeDangerousConfirmationTx turns a confirmation-required
// request_api result into a canonical request_user_input intent in the same
// transaction as that Tool Result. The model never authors the security
// binding or option ordering for any supported Project API protocol.
func (service *Service) enqueueRuntimeDangerousConfirmationTx(ctx context.Context, tx *sql.Tx, thread *threadRecord, tc toolContext, source toolExecutionRecord, result json.RawMessage, now time.Time) (toolExecutionRecord, bool, error) {
	if !supportsRuntimeDangerousConfirmation(tc) || source.ToolName != "request_api" || !confirmationRequiredToolResult(string(result)) {
		return toolExecutionRecord{}, false, nil
	}
	if !isUUIDv7(source.UUID) || !isUUIDv7(source.TargetUUID) {
		return toolExecutionRecord{}, false, domainError(CodeStateConflict, "危险操作确认来源损坏", "request_api confirmation source 缺少公开 execution 或 target UUID。", nil)
	}

	var sourceArguments map[string]any
	if err := json.Unmarshal([]byte(source.ArgumentsJSON), &sourceArguments); err != nil {
		return toolExecutionRecord{}, false, domainError(CodeStateConflict, "危险操作确认来源损坏", "无法读取 request_api confirmation source 参数。", err)
	}
	routeID := stringArg(sourceArguments, "__route_id")
	route, ok := agentAPIRouteByIDFromRoutes(routeID, service.requestAPIRoutes())
	if !ok || route.Risk != RiskDangerous || !route.RequiresConfirmation || stringArg(sourceArguments, "__target_uuid") != source.TargetUUID {
		return toolExecutionRecord{}, false, domainError(CodeStateConflict, "危险操作确认来源不匹配", "confirmation-required 结果必须来自已注册危险 Route 的持久化原请求。", nil)
	}

	var generationLanguage string
	if err := tx.QueryRowContext(ctx, `SELECT generation_language FROM projects WHERE uuid=?`, tc.ProjectUUID).Scan(&generationLanguage); err != nil {
		return toolExecutionRecord{}, false, domainError(CodeStateConflict, "危险操作确认语言不可用", "无法读取当前项目的生成语言。", err)
	}
	questionID, copy := runtimeDangerousConfirmationPresentation(route, generationLanguage)
	binding := dangerousConfirmationBinding{
		Route:              route.ID,
		ProjectUUID:        tc.ProjectUUID,
		TargetUUID:         source.TargetUUID,
		ExpectedRevision:   storedAgentAPIExpectedRevision(route, sourceArguments),
		RequestFingerprint: storedAgentRequestFingerprint(sourceArguments),
		ConfirmOption:      1,
	}
	publicArguments := map[string]any{}
	if usesCodexUserInputProtocol(tc) {
		binding.QuestionID = questionID
		questions := []UserInputQuestion{{
			Header: copy.Header, ID: questionID, Question: copy.Question,
			Options: []UserInputOption{
				{Label: copy.SafeLabel, Description: copy.SafeDescription},
				{Label: copy.ConfirmLabel, Description: copy.ConfirmDescription},
			},
		}}
		if _, err := service.validateCodexDangerousConfirmationBinding(tc, binding, questions); err != nil {
			return toolExecutionRecord{}, false, err
		}
		publicArguments = map[string]any{
			"questions": []map[string]any{{
				"header": copy.Header, "id": questionID, "question": copy.Question,
				"options": []map[string]any{
					{"label": copy.SafeLabel, "description": copy.SafeDescription},
					{"label": copy.ConfirmLabel, "description": copy.ConfirmDescription},
				},
			}},
		}
	} else {
		if _, err := service.validateDangerousConfirmationBinding(tc, binding, "single_choice", 2); err != nil {
			return toolExecutionRecord{}, false, err
		}
		publicArguments = map[string]any{
			"input_type": "single_choice",
			"question":   copy.Question,
			"options": []map[string]any{
				{"label": copy.SafeLabel, "description": copy.SafeDescription},
				{"label": copy.ConfirmLabel, "description": copy.ConfirmDescription},
			},
		}
	}
	publicArgumentsJSON, err := json.Marshal(publicArguments)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}

	providerCallID := confirmationRequestProviderCallID(source.UUID)
	idempotencyKey := toolCallKey(tc.Run.UUID, providerCallID, "request_user_input")
	var existingID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM agent_tool_executions WHERE idempotency_key=?`, idempotencyKey).Scan(&existingID); err == nil {
		return toolExecutionRecord{}, false, nil
	} else if err != sql.ErrNoRows {
		return toolExecutionRecord{}, false, err
	}

	publicCallUUID, err := newUUIDv7()
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	executionUUID, err := newUUIDv7()
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	storedArguments := make(map[string]any, len(publicArguments)+4)
	for key, value := range publicArguments {
		storedArguments[key] = value
	}
	storedArguments["confirmation"] = binding
	storedArguments["__provider_call_id"] = providerCallID
	storedArguments["__runtime_generated_confirmation"] = true
	storedArguments["__confirmation_source_execution_uuid"] = source.UUID
	storedArgumentsJSON, err := json.Marshal(storedArguments)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}

	action := "请求危险操作确认"
	metadata := map[string]any{
		"purpose": "request_user_input", "action": action, "target_uuid": source.TargetUUID,
		"provider_call_id": providerCallID, "runtime_generated": true,
		"confirmation_route_id": route.ID, "confirmation_source_execution_uuid": source.UUID,
	}
	item, err := appendItemTx(ctx, tx, thread, &tc.Turn.ID, &tc.Run.ID, "tool_call", "assistant", string(publicArgumentsJSON), "json", "in_progress", publicCallUUID, "request_user_input", source.TargetUUID, metadata, now)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	insert, err := tx.ExecContext(ctx, `INSERT INTO agent_tool_executions(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,'intent',?,?)`, executionUUID, tc.Thread.ID, tc.Run.ID, tc.Turn.ID, item.ID, publicCallUUID, "request_user_input", source.TargetUUID, string(storedArgumentsJSON), idempotencyKey, now, now)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	executionID, err := insert.LastInsertId()
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	event := map[string]any{
		"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID,
		"run_uuid": tc.Run.UUID, "tool_call_uuid": publicCallUUID, "tool_name": "request_user_input",
		"route_id": "", "action": action, "method": "", "path": "", "target_uuid": source.TargetUUID,
		"runtime_generated": true, "confirmation_route_id": route.ID,
		"confirmation_source_execution_uuid": source.UUID,
	}
	if _, err := appendEventTx(ctx, tx, thread, &tc.Run.ID, "tool_intent", event, now); err != nil {
		return toolExecutionRecord{}, false, err
	}
	return toolExecutionRecord{
		ID: executionID, ThreadID: tc.Thread.ID, RunID: tc.Run.ID, TurnID: tc.Turn.ID, ItemID: item.ID,
		UUID: executionUUID, ToolCallUUID: publicCallUUID, ToolName: "request_user_input", TargetUUID: source.TargetUUID,
		ArgumentsJSON: string(storedArgumentsJSON), IdempotencyKey: idempotencyKey, State: "intent",
		Action: action, CreatedAt: now, UpdatedAt: now,
	}, true, nil
}

// enqueueConfirmedRequestReplayTx persists a new request_api intent instead of
// asking the model to reconstruct the confirmed request. The deterministic key
// makes answering and job recovery exactly-once, while the original failed
// execution remains unchanged for audit history.
func (service *Service) enqueueConfirmedRequestReplayTx(ctx context.Context, tx *sql.Tx, thread *threadRecord, row userInputRow, source dangerousConfirmationSourceExecution, now time.Time) (toolExecutionRecord, bool, error) {
	providerReplayKey := "confirmation-replay:" + row.UUID
	idempotencyKey := toolCallKey(row.RunUUID, providerReplayKey, "request_api")
	var existingID int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM agent_tool_executions WHERE idempotency_key=?`, idempotencyKey).Scan(&existingID)
	if err == nil {
		if err := repairPendingConfirmationReplayProviderIDTx(ctx, tx, existingID, row.UUID); err != nil {
			return toolExecutionRecord{}, false, err
		}
		return toolExecutionRecord{}, false, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return toolExecutionRecord{}, false, err
	}

	var storedArguments map[string]any
	if err := json.Unmarshal([]byte(source.ArgumentsJSON), &storedArguments); err != nil {
		return toolExecutionRecord{}, false, domainError(CodeStateConflict, "危险操作原请求损坏", "无法恢复已确认的 request_api 参数。", err)
	}
	publicArguments := publicStoredToolArguments(storedArguments)
	publicArgumentsJSON, err := json.Marshal(publicArguments)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	publicCallUUID, err := newUUIDv7()
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	executionUUID, err := newUUIDv7()
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	providerCallID := confirmationReplayProviderCallID(row.UUID)
	storedArguments["__provider_call_id"] = providerCallID
	delete(storedArguments, "__request_uuid")
	delete(storedArguments, "__request_ordinal")
	storedArguments["__confirmation_auto_replay"] = true
	storedArguments["__confirmation_request_uuid"] = row.UUID
	storedArguments["__confirmation_source_execution_uuid"] = source.UUID
	storedArgumentsJSON, err := json.Marshal(storedArguments)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	routeID := stringArg(storedArguments, "__route_id")
	action := stringArg(storedArguments, "__action")
	method := stringArg(storedArguments, "__method")
	path := stringArg(storedArguments, "__path")
	metadata := map[string]any{
		"purpose": "request_api", "action": action, "target_uuid": source.TargetUUID,
		"provider_call_id": providerCallID, "route_id": routeID, "method": method, "path": path,
		"runtime_generated": true, "confirmation_request_uuid": row.UUID,
		"confirmation_source_execution_uuid": source.UUID,
	}
	item, err := appendItemTx(ctx, tx, thread, &row.TurnID, &row.RunID, "tool_call", "assistant", string(publicArgumentsJSON), "json", "in_progress", publicCallUUID, "request_api", source.TargetUUID, metadata, now)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO agent_tool_executions(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,'intent',?,?)`, executionUUID, row.ThreadID, row.RunID, row.TurnID, item.ID, publicCallUUID, "request_api", source.TargetUUID, string(storedArgumentsJSON), idempotencyKey, now, now)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	executionID, err := result.LastInsertId()
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	event := map[string]any{
		"project_uuid": row.ProjectUUID, "thread_uuid": row.ThreadUUID, "turn_uuid": row.TurnUUID,
		"run_uuid": row.RunUUID, "tool_call_uuid": publicCallUUID, "tool_name": "request_api",
		"route_id": routeID, "action": action, "method": method, "path": path, "target_uuid": source.TargetUUID,
		"runtime_generated": true, "confirmation_request_uuid": row.UUID,
		"confirmation_source_execution_uuid": source.UUID,
	}
	if _, err := appendEventTx(ctx, tx, thread, &row.RunID, "tool_intent", event, now); err != nil {
		return toolExecutionRecord{}, false, err
	}
	return toolExecutionRecord{
		ID: executionID, ThreadID: row.ThreadID, RunID: row.RunID, TurnID: row.TurnID, ItemID: item.ID,
		UUID: executionUUID, ToolCallUUID: publicCallUUID, ToolName: "request_api", TargetUUID: source.TargetUUID,
		ArgumentsJSON: string(storedArgumentsJSON), IdempotencyKey: idempotencyKey, State: "intent",
		RouteID: routeID, Action: action, Method: method, Path: path, CreatedAt: now, UpdatedAt: now,
	}, true, nil
}

func repairPendingConfirmationReplayProviderIDTx(ctx context.Context, tx *sql.Tx, executionID int64, confirmationRequestUUID string) error {
	if !isUUIDv7(confirmationRequestUUID) {
		return domainError(CodeStateConflict, "危险操作重放关联损坏", "未完成 confirmation replay 缺少有效的 confirmation request UUID。", nil)
	}
	var state string
	var itemID int64
	var argumentsJSON string
	if err := tx.QueryRowContext(ctx, `SELECT state,item_id,arguments_json FROM agent_tool_executions WHERE id=?`, executionID).Scan(&state, &itemID, &argumentsJSON); err != nil {
		return err
	}
	if state != "intent" && state != "executing" {
		return nil
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(argumentsJSON), &arguments); err != nil {
		return domainError(CodeStateConflict, "危险操作重放参数损坏", "无法修复未完成 confirmation replay 的 Provider call ID。", err)
	}
	if !boolArg(arguments, "__confirmation_auto_replay") || stringArg(arguments, "__confirmation_request_uuid") != confirmationRequestUUID {
		return domainError(CodeStateConflict, "危险操作重放关联损坏", "未完成 execution 与 confirmation request UUID 不匹配，拒绝改写 Provider call ID。", nil)
	}
	providerCallID := confirmationReplayProviderCallID(confirmationRequestUUID)
	arguments["__provider_call_id"] = providerCallID
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_tool_executions SET arguments_json=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND state IN ('intent','executing')`, string(encoded), executionID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE chat_items SET metadata_json=json_set(COALESCE(metadata_json,'{}'),'$.provider_call_id',?,'$.runtime_generated',json('true'),'$.confirmation_request_uuid',?) WHERE id=?`, providerCallID, confirmationRequestUUID, itemID)
	return err
}

// repairPendingConfirmationReplayProviderID is the recovery boundary for
// replay intents created before Provider-compatible synthetic call IDs were
// introduced. It repairs both durable copies before execution and returns the
// refreshed record so the eventual tool result uses the same Provider ID. The
// public tool_call_uuid is read back but never modified.
func (service *Service) repairPendingConfirmationReplayProviderID(ctx context.Context, store *project.Store, execution toolExecutionRecord) (toolExecutionRecord, error) {
	if execution.ToolName != "request_api" || (execution.State != "intent" && execution.State != "executing") {
		return execution, nil
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(execution.ArgumentsJSON), &arguments); err != nil {
		return execution, domainError(CodeStateConflict, "危险操作重放参数损坏", "无法读取待恢复 request_api 的持久化参数。", err)
	}
	if !boolArg(arguments, "__confirmation_auto_replay") {
		return execution, nil
	}
	confirmationRequestUUID := stringArg(arguments, "__confirmation_request_uuid")
	if !isUUIDv7(confirmationRequestUUID) {
		return execution, domainError(CodeStateConflict, "危险操作重放关联损坏", "未完成 confirmation replay 缺少有效的 confirmation request UUID。", nil)
	}

	sqlDB, err := store.DB().DB()
	if err != nil {
		return execution, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return execution, err
	}
	defer tx.Rollback()
	if err := repairPendingConfirmationReplayProviderIDTx(ctx, tx, execution.ID, confirmationRequestUUID); err != nil {
		return execution, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT state,arguments_json,tool_call_uuid,item_id FROM agent_tool_executions WHERE id=?`, execution.ID).
		Scan(&execution.State, &execution.ArgumentsJSON, &execution.ToolCallUUID, &execution.ItemID); err != nil {
		return execution, err
	}
	if err := tx.Commit(); err != nil {
		return execution, err
	}
	return execution, nil
}

func (service *Service) prepareConfirmedRequestReplayTx(ctx context.Context, tx *sql.Tx, thread *threadRecord, row userInputRow, responseJSON string, now time.Time) (toolExecutionRecord, bool, error) {
	var confirmationExecutionID, confirmationItemID int64
	var confirmationArguments string
	if err := tx.QueryRowContext(ctx, `SELECT id,item_id,arguments_json FROM agent_tool_executions WHERE run_id=? AND tool_call_uuid=? AND tool_name='request_user_input'`, row.RunID, row.ToolCallUUID).Scan(&confirmationExecutionID, &confirmationItemID, &confirmationArguments); err != nil {
		return toolExecutionRecord{}, false, err
	}
	binding, err := dangerousConfirmationFromArguments(confirmationArguments)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	if binding == nil {
		return toolExecutionRecord{}, false, nil
	}
	markedSourceUUID := metadataString(row.ItemMetadataJSON, "confirmation_source_execution_uuid")
	source, err := service.ensureRuntimeDangerousConfirmationSourceTx(ctx, tx, row.RunID, confirmationExecutionID, confirmationItemID, row.ProjectUUID, markedSourceUUID, confirmationArguments, *binding)
	if err != nil {
		return toolExecutionRecord{}, false, err
	}
	confirmed, err := dangerousConfirmationSelected(row.SchemaVersion, row.RequestJSON, responseJSON, *binding)
	if err != nil {
		return toolExecutionRecord{}, false, domainError(CodeStateConflict, "危险操作确认回答损坏", "无法验证持久化的确认回答。", err)
	}
	if !confirmed {
		return toolExecutionRecord{}, false, nil
	}
	return service.enqueueConfirmedRequestReplayTx(ctx, tx, thread, row, source, now)
}

func hasMatchingDangerousConfirmation(ctx context.Context, store *project.Store, tc toolContext, request agentAPIRequest) (bool, error) {
	var rows []struct {
		ExecutionID   int64
		ArgumentsJSON string
		SchemaVersion string
		RequestJSON   string
		ResponseJSON  string
	}
	err := store.DB().WithContext(ctx).Table("agent_tool_executions AS executions").
		Select("executions.id AS execution_id,executions.arguments_json,requests.schema_version,requests.request_json,requests.response_json").
		Joins("JOIN chat_user_input_requests AS requests ON requests.run_id=executions.run_id AND requests.tool_call_uuid=executions.tool_call_uuid").
		Where("executions.run_id=? AND executions.tool_name='request_user_input' AND executions.state='completed' AND requests.response_json IS NOT NULL", tc.Run.ID).
		Order("executions.id DESC").Scan(&rows).Error
	if err != nil {
		return false, err
	}
	expectedRevision := agentAPIRequestExpectedRevision(request)
	expectedFingerprint := agentRequestFingerprint(request)
	for _, row := range rows {
		var arguments map[string]any
		if json.Unmarshal([]byte(row.ArgumentsJSON), &arguments) != nil || !boolArg(arguments, "__runtime_generated_confirmation") || !isUUIDv7(stringArg(arguments, "__confirmation_source_execution_uuid")) {
			continue
		}
		binding, bindingErr := dangerousConfirmationFromArguments(row.ArgumentsJSON)
		if bindingErr != nil || binding == nil {
			continue
		}
		if binding.Route != request.Route.ID || binding.ProjectUUID != tc.ProjectUUID || binding.TargetUUID != request.TargetUUID || binding.ExpectedRevision != expectedRevision || binding.RequestFingerprint != expectedFingerprint {
			continue
		}
		confirmed, err := dangerousConfirmationSelected(row.SchemaVersion, row.RequestJSON, row.ResponseJSON, *binding)
		if err == nil && confirmed {
			consumed, err := dangerousConfirmationAlreadyConsumed(ctx, store, tc.Run.ID, row.ExecutionID, request.Route, binding.ExpectedRevision, expectedFingerprint)
			if err != nil {
				return false, err
			}
			if !consumed {
				return true, nil
			}
		}
	}
	return false, nil
}

func dangerousConfirmationAlreadyConsumed(ctx context.Context, store *project.Store, runID, confirmationExecutionID int64, route agentAPIRoute, expectedRevision int64, fingerprint string) (bool, error) {
	var executions []struct {
		ArgumentsJSON string
	}
	if err := store.DB().WithContext(ctx).Table("agent_tool_executions").
		Select("arguments_json").
		Where("run_id=? AND id>? AND tool_name='request_api' AND state='completed'", runID, confirmationExecutionID).
		Order("id").Scan(&executions).Error; err != nil {
		return false, err
	}
	for _, execution := range executions {
		var arguments map[string]any
		if json.Unmarshal([]byte(execution.ArgumentsJSON), &arguments) != nil {
			continue
		}
		if stringArg(arguments, "__route_id") == route.ID && storedAgentAPIExpectedRevision(route, arguments) == expectedRevision && storedAgentRequestFingerprint(arguments) == fingerprint {
			return true, nil
		}
	}
	return false, nil
}

func storedAgentRequestFingerprint(arguments map[string]any) string {
	query, _ := arguments["query"].(map[string]any)
	body, _ := arguments["request_body"].(map[string]any)
	return agentRequestFingerprintParts(stringArg(arguments, "__route_id"), stringArg(arguments, "__method"), stringArg(arguments, "__path"), query, body)
}

func dangerousConfirmationSelected(schemaVersion, requestJSON, responseJSON string, binding dangerousConfirmationBinding) (bool, error) {
	if schemaVersion == userInputSchemaCodexQuestions {
		var request struct {
			Questions []UserInputQuestion `json:"questions"`
		}
		var response struct {
			Answers map[string]UserInputAnswer `json:"answers"`
		}
		if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
			return false, err
		}
		if err := json.Unmarshal([]byte(responseJSON), &response); err != nil {
			return false, err
		}
		if len(request.Questions) != 1 || request.Questions[0].ID != binding.QuestionID || binding.ConfirmOption <= 0 || binding.ConfirmOption >= len(request.Questions[0].Options) {
			return false, nil
		}
		answer, ok := response.Answers[binding.QuestionID]
		return ok && strings.TrimSpace(answer.OtherText) == "" && answer.SelectedOptionUUID == request.Questions[0].Options[binding.ConfirmOption].UUID, nil
	}
	var legacyRequest struct {
		Options []UserInputOption `json:"options"`
	}
	var legacyResponse struct {
		SelectedOptionUUIDs []string `json:"selected_option_uuids"`
	}
	if err := json.Unmarshal([]byte(requestJSON), &legacyRequest); err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(responseJSON), &legacyResponse); err != nil {
		return false, err
	}
	if binding.ConfirmOption < 0 || binding.ConfirmOption >= len(legacyRequest.Options) {
		return false, nil
	}
	confirmedUUID := legacyRequest.Options[binding.ConfirmOption].UUID
	for _, selected := range legacyResponse.SelectedOptionUUIDs {
		if selected == confirmedUUID {
			return true, nil
		}
	}
	return false, nil
}

func agentRequestFingerprint(request agentAPIRequest) string {
	return agentRequestFingerprintParts(request.Route.ID, request.Method, request.Path, request.Query, request.Body)
}

func agentRequestFingerprintParts(routeID, method, path string, query, body map[string]any) string {
	canonical, _ := json.Marshal(map[string]any{
		"route": routeID, "method": method, "path": path,
		"query": query, "request_body": body,
	})
	digest := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validAgentRequestFingerprint(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
