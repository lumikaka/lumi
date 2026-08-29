package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"lumi/internal/llm"
	"lumi/internal/llmlog"
	"lumi/internal/project"
	"lumi/internal/provider"

	"gorm.io/gorm"
)

const budgetFinalizationInstruction = `This is the single, final response for a Turn that reached an internal runtime budget. Do not call tools. Accurately report what was completed, what remains incomplete, and why execution stopped. Never claim that unverified work is complete. Do not ask the user to reply "continue" and do not imply that a new Turn was created.`

type toolCycleEntry struct {
	Name       string `json:"name"`
	Arguments  string `json:"arguments"`
	Status     string `json:"status"`
	ErrorCode  string `json:"error_code,omitempty"`
	TargetUUID string `json:"target_uuid,omitempty"`
	ResultHash string `json:"result_hash"`
}

func budgetReason(run runRecord) string {
	switch {
	case run.MaxNoProgressRounds > 0 && run.NoProgressStreak >= run.MaxNoProgressRounds:
		return BudgetReasonNoProgress
	case run.MaxModelRequests > 1 && run.ModelRequestCount >= run.MaxModelRequests-1:
		return BudgetReasonModelRequests
	case run.MaxTokenUnits > 0 && run.TokenUnits >= run.MaxTokenUnits:
		return BudgetReasonTokens
	case run.MaxActiveDurationMS > 0 && run.ActiveDurationMS >= run.MaxActiveDurationMS:
		return BudgetReasonActiveDuration
	default:
		return ""
	}
}

func (service *Service) executeToolTracked(ctx context.Context, store *project.Store, tc toolContext, execution toolExecutionRecord) (json.RawMessage, error) {
	startedAt := time.Now()
	result, executeErr := service.executeTool(ctx, store, tc, execution)
	recordErr := service.recordActiveDuration(context.WithoutCancel(ctx), store, tc.Run.ID, time.Since(startedAt).Milliseconds())
	if executeErr != nil && recordErr != nil {
		return result, errors.Join(executeErr, recordErr)
	}
	if executeErr != nil {
		return result, executeErr
	}
	return result, recordErr
}

func (service *Service) recordActiveDuration(ctx context.Context, store *project.Store, runID, durationMS int64) error {
	if durationMS < 0 {
		durationMS = 0
	}
	return store.DB().WithContext(ctx).Model(&runRecord{}).Where("id=?", runID).Updates(map[string]any{
		"active_duration_ms": gorm.Expr("active_duration_ms + ?", durationMS),
		"updated_at":         service.now().UTC(),
	}).Error
}

func (service *Service) performChatModelRequest(ctx context.Context, store *project.Store, tc *toolContext, resolved provider.Resolved, messages []llm.ChatMessage, tools []llm.ToolDefinition, contextBytes int, scenario string) (llm.ChatResponse, error) {
	request := llm.ChatRequest{BaseURL: resolved.BaseURL, APIKey: resolved.APIKey, Model: tc.Run.Model, Messages: messages, Tools: tools, MaxTokens: 4096}
	requestPayload, err := llmlog.EncodeChatRequest(request)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	requestOrdinal, err := service.recordModelStart(ctx, store, *tc, contextBytes)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	tc.Run.ModelRequestCount = requestOrdinal
	logHandle, err := llmlog.Begin(ctx, store, service.hub, llmlog.StartInput{
		ProjectID: tc.Thread.ProjectID, ChatThreadID: tc.Thread.ID, ChatRunID: tc.Run.ID,
		SourceType: llmlog.SourceProjectChat, Scenario: scenario, RequestType: llmlog.RequestText, Attempt: requestOrdinal,
		ProviderUUID: tc.Run.ProviderUUID, ProviderType: resolved.ProviderType, Model: tc.Run.Model,
		RequestPayload: requestPayload,
	})
	if err != nil {
		return llm.ChatResponse{}, err
	}
	tc.RequestUUID = logHandle.UUID
	tc.RequestOrdinal = requestOrdinal
	if err := service.recordModelRequestEvent(ctx, store, *tc, "model_request_started", "pending"); err != nil {
		return llm.ChatResponse{}, err
	}
	providerStartedAt := time.Now()
	response, requestErr := service.model.Complete(ctx, request)
	durationMS := time.Since(providerStartedAt).Milliseconds()
	var responsePayload []byte
	if requestErr == nil {
		responsePayload, requestErr = llmlog.EncodeChatResponse(response, request.APIKey)
		if requestErr == nil {
			responsePayload = attachAgentToolLogMetadata(responsePayload, service.agentToolLogMetadata(*tc, response.Message.ToolCalls))
		}
	}
	if len(responsePayload) == 0 {
		responsePayload, _ = json.Marshal(response)
	}
	finishErr := llmlog.Finish(context.WithoutCancel(ctx), store, service.hub, logHandle, llmlog.FinishInput{
		OutputSummary: response.Message.Content, InputTokens: response.Usage.InputTokens, CachedInputTokens: response.Usage.CachedInputTokens, OutputTokens: response.Usage.OutputTokens,
		FinishReason: response.FinishReason, Response: responsePayload, Err: requestErr,
	})
	requestBytes := len(requestPayload)
	if contextBytes > requestBytes {
		requestBytes = contextBytes
	}
	usageErr := service.recordModelUsage(context.WithoutCancel(ctx), store, tc, durationMS, response.Usage, requestBytes+len(responsePayload))
	if finishErr != nil {
		requestErr = errors.Join(requestErr, finishErr)
	}
	if usageErr != nil {
		requestErr = errors.Join(requestErr, usageErr)
	}
	requestStatus := "completed"
	if requestErr != nil {
		requestStatus = "failed"
		if errors.Is(requestErr, context.Canceled) {
			requestStatus = "cancelled"
		}
	}
	_ = store.DB().WithContext(context.WithoutCancel(ctx)).Table("llm_logs").Select("status").Where("uuid=? AND chat_run_id=?", tc.RequestUUID, tc.Run.ID).Scan(&requestStatus).Error
	if eventErr := service.recordModelRequestEvent(context.WithoutCancel(ctx), store, *tc, "model_request_completed", requestStatus); eventErr != nil {
		requestErr = errors.Join(requestErr, eventErr)
	}
	return response, requestErr
}

func (service *Service) recordModelUsage(ctx context.Context, store *project.Store, tc *toolContext, durationMS int64, usage llm.Usage, fallbackBytes int) error {
	if durationMS < 0 {
		durationMS = 0
	}
	tokenUnits := int64(usage.InputTokens) + int64(usage.OutputTokens)
	if (usage.InputTokens <= 0 || usage.OutputTokens <= 0) && fallbackBytes > 0 {
		tokenUnits = int64((fallbackBytes + 3) / 4)
	}
	err := store.DB().WithContext(ctx).Model(&runRecord{}).Where("id=?", tc.Run.ID).Updates(map[string]any{
		"active_duration_ms": gorm.Expr("active_duration_ms + ?", durationMS),
		"token_units":        gorm.Expr("token_units + ?", tokenUnits),
		"updated_at":         service.now().UTC(),
	}).Error
	if err == nil {
		tc.Run.ActiveDurationMS += durationMS
		tc.Run.TokenUnits += tokenUnits
	}
	return err
}

func cycleEntry(name, rawArguments, targetUUID string, result json.RawMessage) toolCycleEntry {
	arguments := canonicalJSON(rawArguments)
	canonicalResult := canonicalJSON(string(result))
	status, errorCode := "completed", ""
	var envelope struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(result, &envelope) == nil && !envelope.Success {
		status, errorCode = "failed", strings.TrimSpace(envelope.Error.Code)
	}
	digest := sha256.Sum256([]byte(canonicalResult))
	return toolCycleEntry{Name: strings.ToLower(strings.TrimSpace(name)), Arguments: arguments, Status: status, ErrorCode: errorCode, TargetUUID: strings.TrimSpace(targetUUID), ResultHash: hex.EncodeToString(digest[:])}
}

func canonicalJSON(raw string) string {
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return strings.TrimSpace(raw)
	}
	if object, ok := value.(map[string]any); ok {
		for key := range object {
			if strings.HasPrefix(key, "__") {
				delete(object, key)
			}
		}
		value = object
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return string(encoded)
}

func (service *Service) recordToolCycleProgress(ctx context.Context, store *project.Store, tc toolContext, entries []toolCycleEntry) error {
	if len(entries) == 0 {
		return service.resetNoProgress(ctx, store, tc.Run.ID)
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(digest[:])
	return store.DB().WithContext(ctx).Model(&runRecord{}).Where("id=? AND status='in_progress'", tc.Run.ID).Updates(map[string]any{
		"no_progress_streak":     gorm.Expr("CASE WHEN last_cycle_fingerprint = ? THEN no_progress_streak + 1 ELSE 1 END", fingerprint),
		"last_cycle_fingerprint": fingerprint,
		"updated_at":             service.now().UTC(),
	}).Error
}

func (service *Service) recordRecoveredToolBatchProgress(ctx context.Context, store *project.Store, tc toolContext, recovered toolExecutionRecord) error {
	var arguments map[string]any
	_ = json.Unmarshal([]byte(recovered.ArgumentsJSON), &arguments)
	requestUUID, _ := arguments["__request_uuid"].(string)
	if !isUUIDv7(requestUUID) {
		result := json.RawMessage("null")
		if recovered.ResultJSON != nil {
			result = json.RawMessage(*recovered.ResultJSON)
		} else {
			var stored string
			if err := store.DB().WithContext(ctx).Table("agent_tool_executions").Select("result_json").Where("id=?", recovered.ID).Scan(&stored).Error; err != nil {
				return err
			}
			result = json.RawMessage(stored)
		}
		return service.recordToolCycleProgress(ctx, store, tc, []toolCycleEntry{cycleEntry(recovered.ToolName, recovered.ArgumentsJSON, recovered.TargetUUID, result)})
	}
	var remaining int64
	if err := store.DB().WithContext(ctx).Table("agent_tool_executions").Where("run_id=? AND json_extract(arguments_json,'$.__request_uuid')=? AND state<>'completed'", tc.Run.ID, requestUUID).Count(&remaining).Error; err != nil || remaining > 0 {
		return err
	}
	var executions []toolExecutionRecord
	if err := store.DB().WithContext(ctx).Table("agent_tool_executions").Where("run_id=? AND json_extract(arguments_json,'$.__request_uuid')=?", tc.Run.ID, requestUUID).Order("id").Find(&executions).Error; err != nil {
		return err
	}
	entries := make([]toolCycleEntry, 0, len(executions))
	for _, execution := range executions {
		if execution.ResultJSON == nil {
			continue
		}
		entries = append(entries, cycleEntry(execution.ToolName, execution.ArgumentsJSON, execution.TargetUUID, json.RawMessage(*execution.ResultJSON)))
	}
	return service.recordToolCycleProgress(ctx, store, tc, entries)
}

func (service *Service) resetNoProgress(ctx context.Context, store *project.Store, runID int64) error {
	return store.DB().WithContext(ctx).Model(&runRecord{}).Where("id=?", runID).Updates(map[string]any{
		"no_progress_streak":     0,
		"last_cycle_fingerprint": "",
		"updated_at":             service.now().UTC(),
	}).Error
}

func (service *Service) beginBudgetFinalization(ctx context.Context, store *project.Store, tc *toolContext, reason string) (bool, error) {
	sqlDB, err := store.DB().DB()
	if err != nil {
		return false, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	thread, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
	if err != nil {
		return false, err
	}
	var existingReason string
	var attemptedAt *time.Time
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT limit_reason,finalization_attempted_at,status FROM chat_runs WHERE id=?`, tc.Run.ID).Scan(&existingReason, &attemptedAt, &status); err != nil {
		return false, err
	}
	if attemptedAt != nil || status != TurnInProgress {
		return false, tx.Commit()
	}
	if existingReason != "" {
		reason = existingReason
	}
	now := service.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE chat_runs SET limit_reason=?,finalization_attempted_at=?,updated_at=? WHERE id=? AND status='in_progress' AND finalization_attempted_at IS NULL`, reason, now, now, tc.Run.ID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return false, err
	}
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "run_budget_exhausted", map[string]any{
		"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID,
		"limit_reason": reason, "model_request_count": tc.Run.ModelRequestCount, "active_duration_ms": tc.Run.ActiveDurationMS,
		"token_units": tc.Run.TokenUnits, "no_progress_streak": tc.Run.NoProgressStreak,
	}, now); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextEventSequence, now, thread.ID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	tc.Run.LimitReason, tc.Run.FinalizationAttemptedAt = reason, &now
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:run_status", map[string]any{
		"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID,
		"status": TurnInProgress, "limit_reason": reason,
	})
	return true, nil
}

func (service *Service) finalizeBudget(ctx context.Context, store *project.Store, tc *toolContext, resolved provider.Resolved, reason string) error {
	claimed, err := service.beginBudgetFinalization(ctx, store, tc, reason)
	if err != nil {
		return err
	}
	if !claimed {
		return service.failBudgetRun(context.WithoutCancel(ctx), store, *tc)
	}
	messages, contextBytes, _, err := service.buildContext(ctx, store, *tc, nil)
	if err != nil {
		return service.failBudgetRun(context.WithoutCancel(ctx), store, *tc)
	}
	if len(messages) == 0 || messages[0].Role != "system" {
		return service.failBudgetRun(context.WithoutCancel(ctx), store, *tc)
	}
	messages[0].Content = strings.TrimSpace(messages[0].Content) + "\n\n" + budgetFinalizationInstruction
	contextBytes = contextRequestBytes(messages, nil)
	if contextBytes > MaxContextBytes {
		return service.failBudgetRun(context.WithoutCancel(ctx), store, *tc)
	}
	response, err := service.performChatModelRequest(ctx, store, tc, resolved, messages, nil, contextBytes, "project_chat_finalization")
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return service.cancelRun(context.WithoutCancel(ctx), store, *tc)
		}
		return service.failBudgetRun(context.WithoutCancel(ctx), store, *tc)
	}
	content := strings.TrimSpace(response.Message.Content)
	if len(response.Message.ToolCalls) > 0 || content == "" || containsUnexecutedToolMarkup(content) {
		return service.failBudgetRun(context.WithoutCancel(ctx), store, *tc)
	}
	return service.completeRun(ctx, store, *tc, content, map[string]any{
		"completion_reason": "budget_limit",
		"budget_reason":     tc.Run.LimitReason,
	})
}

func (service *Service) failBudgetRun(ctx context.Context, store *project.Store, tc toolContext) error {
	return service.failRun(ctx, store, tc, CodeTurnBudget, "Agent 已达到本轮安全预算，且未能生成可靠的最终说明。")
}
