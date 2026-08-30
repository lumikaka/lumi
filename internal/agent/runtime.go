package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"lumi/internal/llm"
	"lumi/internal/project"
	"lumi/internal/provider"

	"gorm.io/gorm"
)

func (service *Service) ExecuteJob(ctx context.Context, store *project.Store, spec JobSpec) error {
	if spec.Version != 1 || spec.ProjectUUID != store.ProjectUUID() || !isUUIDv7(spec.ResourceUUID) || !isUUIDv7(spec.ThreadUUID) {
		return domainError(CodeValidation, "Agent job 参数无效", "Job 只能引用当前项目的公开 UUIDv7。", nil)
	}
	if spec.JobKind == JobWorkflowStep {
		if err := store.RequireReady(); err != nil {
			return err
		}
		return service.ExecuteWorkflowStep(ctx, store, spec.ResourceUUID)
	}
	if spec.JobKind != JobChatTurn && spec.JobKind != JobChatResume {
		return domainError(CodeValidation, "Agent job kind 无效", "不支持的 Agent job。", nil)
	}
	unlockExecution := service.lockExecution(spec.ResourceUUID)
	defer unlockExecution()
	tc, err := service.loadToolContext(ctx, store, spec.ThreadUUID, spec.ResourceUUID)
	if err != nil {
		return err
	}
	if tc.Turn.Status == TurnCompleted || tc.Turn.Status == TurnCancelled {
		return nil
	}
	if ready, err := service.turnFIFOReady(ctx, store, tc); err != nil {
		return err
	} else if !ready {
		return ErrJobNotReady
	}
	if err := service.claimRun(ctx, store, &tc); err != nil {
		return err
	}
	tc.ToolMode, err = service.loadRunToolMode(ctx, store, tc)
	if err != nil {
		return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, err)
	}
	if spec.JobKind == JobChatResume {
		if _, err := service.resumeWorkflowAwait(ctx, store, tc); err != nil {
			return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, err)
		}
	}
	if currentRun, refreshErr := service.refreshRun(ctx, store, tc.Run.ID); refreshErr != nil {
		return refreshErr
	} else {
		tc.Run = currentRun
	}
	if tc.Run.LimitReason != "" && tc.Run.FinalizationAttemptedAt != nil {
		return service.failBudgetRun(context.WithoutCancel(ctx), store, tc)
	}
	resolved, err := service.providers.Resolve(ctx, tc.Run.ProviderUUID)
	if err != nil {
		return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, err)
	}
	tools := llmToolDefinitionsForContext(tc)
	var lastContextThrough int64
runLoop:
	for {
		if err := ctx.Err(); err != nil {
			return service.cancelRun(context.WithoutCancel(ctx), store, tc)
		}
		cancelled, err := service.cancelRequested(ctx, store, tc.Run.ID)
		if err != nil {
			return err
		}
		if cancelled {
			return service.cancelRun(context.WithoutCancel(ctx), store, tc)
		}
		if pending, ok, err := service.pendingTool(ctx, store, tc.Run.ID); err != nil {
			return err
		} else if ok {
			pending, err = service.repairPendingConfirmationReplayProviderID(ctx, store, pending)
			if err != nil {
				return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, err)
			}
			if pending.State != "intent" && pending.State != "executing" {
				continue
			}
			if pending.ToolName == "request_user_input" {
				request, requestErr := service.createUserInputRequest(ctx, store, tc, pending)
				if requestErr != nil {
					return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, requestErr)
				}
				if request.Status == "pending" || request.Status == "answered" || request.Status == "resuming" {
					return ErrWaitingInput
				}
			}
			result, err := service.executeToolTracked(ctx, store, tc, pending)
			if err != nil {
				if errors.Is(err, ErrWaitingWorkflow) {
					return err
				}
				return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, err)
			}
			if err := service.persistToolResult(ctx, store, tc, pending, result); err != nil {
				return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, err)
			}
			if err := service.recordRecoveredToolBatchProgress(ctx, store, tc, pending); err != nil {
				return err
			}
			continue
		}
		currentRun, err := service.refreshRun(ctx, store, tc.Run.ID)
		if err != nil {
			return err
		}
		tc.Run = currentRun
		if lastContextThrough > 0 {
			if steered, err := service.hasSteeringAfter(ctx, store, tc.Run.ID, lastContextThrough); err != nil {
				return err
			} else if steered {
				if err := service.resetNoProgress(ctx, store, tc.Run.ID); err != nil {
					return err
				}
				tc.Run.NoProgressStreak = 0
				tc.Run.LastCycleFingerprint = ""
			}
		}
		if tc.Run.LimitReason != "" && tc.Run.FinalizationAttemptedAt != nil {
			return service.failBudgetRun(context.WithoutCancel(ctx), store, tc)
		}
		if reason := budgetReason(tc.Run); reason != "" {
			return service.finalizeBudget(ctx, store, &tc, resolved, reason)
		}
		messages, contextBytes, contextThrough, err := service.buildContext(ctx, store, tc, tools)
		if err != nil {
			return service.failRun(context.WithoutCancel(ctx), store, tc, errorCode(err), safeMessage(err))
		}
		lastContextThrough = contextThrough
		invalidResponses, err := service.consecutiveInvalidProviderResponses(ctx, store, tc.Run.ID)
		if err != nil {
			return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, err)
		}
		if invalidResponses >= maxConsecutiveInvalidProviderResponses {
			return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, invalidProviderResponseExhaustedError())
		}
		requestMessages := messages
		requestContextBytes := contextBytes
		if invalidResponses > 0 {
			requestMessages = providerResponseRetryMessages(messages)
			requestContextBytes = contextRequestBytes(requestMessages, tools)
			if requestContextBytes > MaxContextBytes {
				return service.failRun(context.WithoutCancel(ctx), store, tc, CodeContextTooLarge, "Provider 重试提示加入后上下文超过本机配置限制。")
			}
		}
		var response llm.ChatResponse
		var requestErr error
		physicalInvalidAttempt := invalidResponses
		invalidResponseExhausted := false
		for {
			response, requestErr = service.performChatModelRequest(ctx, store, &tc, resolved, requestMessages, tools, requestContextBytes, "project_chat")
			if requestErr == nil || invalidProviderResponse(requestErr) == nil {
				break
			}
			physicalInvalidAttempt++
			invalidResponses, err = service.consecutiveInvalidProviderResponses(ctx, store, tc.Run.ID)
			if err != nil {
				return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, err)
			}
			if invalidResponses >= maxConsecutiveInvalidProviderResponses || physicalInvalidAttempt >= maxConsecutiveInvalidProviderResponses {
				invalidResponseExhausted = true
				break
			}
			if ctx.Err() != nil {
				return service.cancelRun(context.WithoutCancel(ctx), store, tc)
			}
			cancelled, cancelErr := service.cancelRequested(ctx, store, tc.Run.ID)
			if cancelErr != nil {
				return cancelErr
			}
			if cancelled {
				return service.cancelRun(context.WithoutCancel(ctx), store, tc)
			}
			currentRun, refreshErr := service.refreshRun(ctx, store, tc.Run.ID)
			if refreshErr != nil {
				return refreshErr
			}
			tc.Run = currentRun
			if tc.Run.LimitReason != "" && tc.Run.FinalizationAttemptedAt != nil {
				return service.failBudgetRun(context.WithoutCancel(ctx), store, tc)
			}
			if reason := budgetReason(tc.Run); reason != "" {
				return service.finalizeBudget(ctx, store, &tc, resolved, reason)
			}
			if steered, steeringErr := service.hasSteeringAfter(ctx, store, tc.Run.ID, contextThrough); steeringErr != nil {
				return steeringErr
			} else if steered {
				continue runLoop
			}
			requestMessages = providerResponseRetryMessages(messages)
			requestContextBytes = contextRequestBytes(requestMessages, tools)
			if requestContextBytes > MaxContextBytes {
				return service.failRun(context.WithoutCancel(ctx), store, tc, CodeContextTooLarge, "Provider 重试提示加入后上下文超过本机配置限制。")
			}
		}
		if requestErr != nil {
			if invalidResponseExhausted {
				requestErr = invalidProviderResponseExhaustedError()
			}
			return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, requestErr)
		}
		if steered, err := service.hasSteeringAfter(ctx, store, tc.Run.ID, contextThrough); err != nil {
			return err
		} else if steered {
			if err := service.resetNoProgress(ctx, store, tc.Run.ID); err != nil {
				return err
			}
			continue
		}
		if len(response.Message.ToolCalls) > 0 {
			if mixedRequestUserInputCalls(response.Message.ToolCalls) {
				return service.failRun(context.WithoutCancel(ctx), store, tc, CodeToolValidation, "request_user_input 必须是本次模型响应中唯一的 Tool Call。")
			}
			prepared, failedCall, prepareErr := service.prepareToolIntentBatch(ctx, store, tc, response.Message.ToolCalls)
			if prepareErr != nil {
				if errorCode(prepareErr) == CodeToolValidation {
					repaired, repairResult, persistErr := service.persistRejectedToolCall(ctx, store, tc, failedCall.ID, failedCall.Name, failedCall.Arguments, prepareErr)
					if persistErr != nil {
						return persistErr
					}
					if repaired {
						if err := service.recordToolCycleProgress(ctx, store, tc, []toolCycleEntry{cycleEntry(failedCall.Name, failedCall.Arguments, "", repairResult)}); err != nil {
							return err
						}
						continue
					}
				}
				return service.failRun(context.WithoutCancel(ctx), store, tc, errorCode(prepareErr), safeMessage(prepareErr))
			}
			prepared, err = service.persistPreparedToolIntentBatch(ctx, store, tc, prepared)
			if err != nil {
				return service.failOrRetryRun(context.WithoutCancel(ctx), store, tc, err)
			}
			allCompleted := len(prepared) > 0
			cycle := make([]toolCycleEntry, 0, len(prepared))
			for _, intent := range prepared {
				if !intent.Completed {
					allCompleted = false
					continue
				}
				cycle = append(cycle, cycleEntry(intent.Call.Name, intent.Call.Arguments, intent.Existing.TargetUUID, intent.PersistedResult))
			}
			if allCompleted {
				if err := service.recordToolCycleProgress(ctx, store, tc, cycle); err != nil {
					return err
				}
			}
			// New intents are now durable as one batch. The pending-tool recovery
			// path executes them in insertion order and remains idempotent across
			// worker or process restarts.
			continue
		}
		content := strings.TrimSpace(response.Message.Content)
		if content == "" {
			return service.failRun(context.WithoutCancel(ctx), store, tc, CodeProvider, "Provider 未返回可用回复。")
		}
		if containsUnexecutedToolMarkup(content) {
			return service.completeRun(ctx, store, tc, invalidToolMarkupHandoffMessage, map[string]any{
				"runtime_generated": true,
				"completion_reason": "unexecuted_tool_markup",
			})
		}
		return service.completeRun(ctx, store, tc, content, nil)
	}
}

const (
	invalidToolMarkupHandoffMessage = "模型返回了未执行的工具调用文本；为避免误操作，本轮已安全停止，且没有执行其中的操作。请回复「继续」以从当前进度重试。"
)

func containsUnexecutedToolMarkup(content string) bool {
	lower := strings.ToLower(content)
	for _, marker := range []string{"<invoke", "</invoke>", "<tool_call", "</tool_call>", "<function=", "<parameter name="} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func (service *Service) hasSteeringAfter(ctx context.Context, store *project.Store, runID, sequence int64) (bool, error) {
	var count int64
	err := store.DB().WithContext(ctx).Table("chat_items").Where("run_id=? AND sequence>? AND item_type='user_message' AND json_extract(metadata_json,'$.steering')=1", runID, sequence).Count(&count).Error
	return count > 0, err
}

func llmToolDefinitions(thread threadRecord) []llm.ToolDefinition {
	return llmToolDefinitionsForMode(thread, ToolModeProjectAPI)
}

func (service *Service) llmToolDefinitions(thread threadRecord, mode string) []llm.ToolDefinition {
	return llmToolDefinitionsForMode(thread, mode)
}

func llmToolDefinitionsForMode(thread threadRecord, mode string) []llm.ToolDefinition {
	return llmToolDefinitionsForProtocol(thread, mode, "")
}

func llmToolDefinitionsForContext(tc toolContext) []llm.ToolDefinition {
	return llmToolDefinitionsForProtocol(tc.Thread, tc.ToolMode, tc.ToolProtocol)
}

func llmToolDefinitionsForProtocol(thread threadRecord, mode, protocol string) []llm.ToolDefinition {
	definitions := toolDefinitionsForProtocol(mode, protocol)
	result := make([]llm.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		name := definition["name"].(string)
		if !toolAllowedForThreadMode(name, thread, mode) {
			continue
		}
		parameters, _ := definition["parameters"].(map[string]any)
		description := definition["description"].(string)
		result = append(result, llm.ToolDefinition{Name: name, Description: description, Parameters: parameters})
	}
	return result
}

func toolDefinitionsForProtocol(mode, protocol string) []map[string]any {
	if normalizedToolMode(mode) == ToolModeLegacyTyped {
		return legacyRecoveryToolDefinitions()
	}
	if normalizedToolMode(mode) == ToolModeProjectAPI {
		switch protocol {
		case ToolProtocolProjectV2:
			return projectAPIV2ToolDefinitions()
		case ToolProtocolProjectV3:
			return projectAPIV3ToolDefinitions()
		}
	}
	return toolDefinitions()
}

// projectAPIV3ToolDefinitions freezes the former one-question user-input
// contract. It must never inherit the active request_user_input definition.
func projectAPIV3ToolDefinitions() []map[string]any {
	definitions := frozenProjectAPIV2V3SharedToolDefinitions()
	return append(definitions, frozenProjectAPIV3ImageGenDefinition(), legacyProjectAPIRequestUserInputDefinition())
}

// projectAPIV2ToolDefinitions freezes both the phase-two image_gen argument
// surface and the former one-question user-input contract.
func projectAPIV2ToolDefinitions() []map[string]any {
	definitions := frozenProjectAPIV2V3SharedToolDefinitions()
	return append(definitions, legacyToolDefinitionByName("image_gen"), legacyProjectAPIRequestUserInputDefinition())
}

// frozenProjectAPIV2V3SharedToolDefinitions is intentionally independent of
// the active definitions. Changes to v4 request_api/read_agent_doc must not
// alter the schema seen by a persisted v2 or v3 Run.
func frozenProjectAPIV2V3SharedToolDefinitions() []map[string]any {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	stringField := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	return []map[string]any{
		{"name": "request_api", "description": "Call any server-registered API route under the current /api/v1/projects/{project_uuid} scope in-process. Reviewed routes retain stricter schemas and optimized domain dispatch; other routes use the application router and its public API contract.", "parameters": object(map[string]any{
			"url":             stringField("Canonical relative /api/v1/projects/{current_project_uuid}/... path"),
			"method":          map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE"}},
			"query":           map[string]any{"type": "object", "additionalProperties": true, "description": "Optional route-specific typed query object; never append a query string to url."},
			"request_body":    map[string]any{"type": "object", "additionalProperties": true},
			"response_filter": map[string]any{"type": "string", "minLength": 1, "maxLength": 2048, "description": "Required safe projection beginning with .data. Select only the fields needed for the current step; use .data only when the complete compact response is necessary."},
		}, "url", "method", "response_filter")},
		{"name": "read_agent_doc", "description": "Read a registered Agent Overview, reusable capability Guide, or Project API contract. Start with /api/v1/agent-docs/overview.md to discover capabilities and routes.", "parameters": object(map[string]any{
			"path": stringField("Registered /api/v1/agent-docs/...md path"),
		}, "path")},
	}
}

func frozenProjectAPIV3ImageGenDefinition() map[string]any {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	stringField := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	return map[string]any{"name": "image_gen", "description": "Generate a project-scoped image synchronously. Select zero to four image-capable References from the current Turn by their resource_uuid; the backend resolves their frozen images in the supplied order.", "parameters": object(map[string]any{
		"prompt": stringField("Detailed image generation prompt"), "reference_uuids": map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{"type": "string"}},
		"size": map[string]any{"type": "string", "enum": []string{"512x512", "1024x1024", "1024x1536", "1536x1024"}}, "quality": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}}, "filename": stringField("Optional output filename"),
	}, "prompt", "reference_uuids")}
}

func (service *Service) loadRunToolMode(ctx context.Context, store *project.Store, tc toolContext) (string, error) {
	var snapshot struct {
		Mode     string
		Protocol string
	}
	err := store.DB().WithContext(ctx).Raw(`SELECT COALESCE(json_extract(metadata_json,'$.prompt_snapshot.tool_mode'),''),COALESCE(json_extract(metadata_json,'$.prompt_snapshot.tool_protocol'),'') FROM chat_items WHERE run_id=? AND turn_id=? AND item_type='user_message' ORDER BY sequence,id LIMIT 1`, tc.Run.ID, tc.Turn.ID).Row().Scan(&snapshot.Mode, &snapshot.Protocol)
	if err != nil {
		return "", err
	}
	mode := normalizedToolMode(snapshot.Mode)
	switch mode {
	case ToolModeProjectAPI:
		if snapshot.Protocol != ToolProtocolProjectAPI && snapshot.Protocol != ToolProtocolProjectV3 && snapshot.Protocol != ToolProtocolProjectV2 {
			return "", domainError(CodeToolNotAllowed, "Tool Protocol 快照无效", "project_api_tools Run 只恢复 project_api_v2、project_api_v3 或 project_api_v4 快照。", nil)
		}
		return mode, nil
	case ToolModeLegacyTyped:
		if err := service.recordLegacyToolRecoveryUse(ctx, store, tc, "legacy_mode_snapshot"); err != nil {
			return "", err
		}
		return mode, nil
	case "":
		// Tool mode predates the protocol snapshot. This is a recovery-only
		// interpretation for a persisted Run; new Run creation always writes both
		// tool_protocol and tool_mode and cannot enter this branch.
		if err := service.recordLegacyToolRecoveryUse(ctx, store, tc, "missing_mode_snapshot"); err != nil {
			return "", err
		}
		return ToolModeLegacyTyped, nil
	default:
		return "", domainError(CodeToolNotAllowed, "Tool Mode 快照无效", "持久化 Run 使用了不受支持的 Tool Mode。", nil)
	}
}

func (service *Service) recordLegacyToolRecoveryUse(ctx context.Context, store *project.Store, tc toolContext, source string) error {
	var count int64
	if err := store.DB().WithContext(ctx).Table("chat_events").Where("run_id=? AND event_type='legacy_tool_recovery'", tc.Run.ID).Count(&count).Error; err != nil || count > 0 {
		return err
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
	// Recheck while holding the thread write lock so retries cannot duplicate
	// the recovery audit event.
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM chat_events WHERE run_id=? AND event_type='legacy_tool_recovery'`, tc.Run.ID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		now := service.now().UTC()
		if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "legacy_tool_recovery", map[string]any{
			"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID,
			"turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID,
			"source": source, "tool_mode": ToolModeLegacyTyped,
		}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextEventSequence, now, thread.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func mixedRequestUserInputCalls(calls []llm.ToolCall) bool {
	if len(calls) <= 1 {
		return false
	}
	for _, call := range calls {
		if call.Name == "request_user_input" {
			return true
		}
	}
	return false
}

func (service *Service) loadToolContext(ctx context.Context, store *project.Store, threadUUID, turnUUID string) (toolContext, error) {
	pid, err := projectID(ctx, store.DB(), store.ProjectUUID())
	if err != nil {
		return toolContext{}, err
	}
	var thread threadRecord
	if err := store.DB().WithContext(ctx).Where("project_id=? AND uuid=?", pid, threadUUID).First(&thread).Error; err != nil {
		return toolContext{}, notFound(err, "Chat thread 不存在")
	}
	var turn turnRecord
	if err := store.DB().WithContext(ctx).Where("thread_id=? AND uuid=?", thread.ID, turnUUID).First(&turn).Error; err != nil {
		return toolContext{}, notFound(err, "Chat turn 不存在")
	}
	var run runRecord
	if err := store.DB().WithContext(ctx).Where("turn_id=?", turn.ID).First(&run).Error; err != nil {
		return toolContext{}, notFound(err, "Chat run 不存在")
	}
	var metadataJSON string
	if err := store.DB().WithContext(ctx).Table("chat_items").Select("metadata_json").Where("run_id=? AND turn_id=? AND item_type='user_message'", run.ID, turn.ID).Order("sequence,id").Limit(1).Scan(&metadataJSON).Error; err != nil {
		return toolContext{}, err
	}
	var metadata struct {
		PromptSnapshot struct {
			ToolProtocol string `json:"tool_protocol"`
		} `json:"prompt_snapshot"`
		LegacyThreadContext struct {
			Scope       string `json:"scope"`
			Scene       string `json:"scene"`
			SubjectUUID string `json:"subject_uuid"`
		} `json:"legacy_thread_context"`
	}
	tc := toolContext{ProjectUUID: store.ProjectUUID(), Thread: thread, Turn: turn, Run: run}
	if json.Unmarshal([]byte(metadataJSON), &metadata) == nil {
		if metadata.PromptSnapshot.ToolProtocol == ToolProtocolProjectAPI || metadata.PromptSnapshot.ToolProtocol == ToolProtocolProjectV3 || metadata.PromptSnapshot.ToolProtocol == ToolProtocolProjectV2 {
			// Only persisted project-api protocols need to be distinguished here.
			// Legacy typed runs continue to be selected by tool_mode.
			threadProtocol := metadata.PromptSnapshot.ToolProtocol
			thread.Scope = metadata.LegacyThreadContext.Scope
			thread.Scene = metadata.LegacyThreadContext.Scene
			thread.SubjectUUID = metadata.LegacyThreadContext.SubjectUUID
			tc.Thread = thread
			tc.ToolProtocol = threadProtocol
		} else {
			thread.Scope = metadata.LegacyThreadContext.Scope
			thread.Scene = metadata.LegacyThreadContext.Scene
			thread.SubjectUUID = metadata.LegacyThreadContext.SubjectUUID
			tc.Thread = thread
		}
	}
	if err := store.DB().WithContext(ctx).Table("project_creation_bootstraps").
		Select("creation_session_uuid").
		Where("project_id=? AND turn_id=?", pid, turn.ID).
		Limit(1).
		Scan(&tc.BootstrapCreationSessionUUID).Error; err != nil {
		return toolContext{}, err
	}
	return tc, nil
}

func (service *Service) turnFIFOReady(ctx context.Context, store *project.Store, tc toolContext) (bool, error) {
	var count int64
	err := store.DB().WithContext(ctx).Table("chat_turns").Where("thread_id=? AND queue_sequence<? AND status IN ('queued','in_progress','waiting_for_input')", tc.Thread.ID, tc.Turn.QueueSequence).Count(&count).Error
	return count == 0, err
}

func (service *Service) claimRun(ctx context.Context, store *project.Store, tc *toolContext) error {
	if tc.Run.Status == TurnInProgress {
		return nil
	}
	if tc.Run.Status == TurnWaitingForInput {
		return ErrWaitingInput
	}
	if tc.Run.Status != TurnQueued {
		return domainError(CodeStateConflict, "Run 无法启动", "run 已处于稳定状态。", nil)
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
	now := service.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status='in_progress',started_at=COALESCE(started_at,?),updated_at=?,error_code='',error_message='' WHERE id=? AND status='queued' AND cancel_requested_at IS NULL`, now, now, tc.Run.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domainError(CodeStateConflict, "Run 无法领取", "run 已被取消或由其他 worker 领取。", nil)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status='in_progress',started_at=COALESCE(started_at,?),updated_at=? WHERE id=? AND status='queued'`, now, now, tc.Turn.ID); err != nil {
		return err
	}
	inputResult, err := tx.ExecContext(ctx, `UPDATE chat_user_input_requests SET status='resumed',resumed_at=?,updated_at=? WHERE run_id=? AND status='resuming'`, now, now, tc.Run.ID)
	if err != nil {
		return err
	}
	resumedUserInput, err := inputResult.RowsAffected()
	if err != nil {
		return err
	}
	if resumedUserInput > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET no_progress_streak=0,last_cycle_fingerprint='',updated_at=? WHERE id=?`, now, tc.Run.ID); err != nil {
			return err
		}
	}
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "run_started", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "status": TurnInProgress}, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextEventSequence, now, thread.ID); err != nil {
		return err
	}
	if _, err := RecomputeThreadStatusTx(ctx, tx, thread.ID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tc.Run.Status, tc.Run.StartedAt, tc.Run.UpdatedAt = TurnInProgress, &now, now
	if resumedUserInput > 0 {
		tc.Run.NoProgressStreak, tc.Run.LastCycleFingerprint = 0, ""
	}
	tc.Turn.Status, tc.Turn.StartedAt, tc.Turn.UpdatedAt = TurnInProgress, &now, now
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:run_status", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "status": TurnInProgress})
	return nil
}

func (service *Service) pendingTool(ctx context.Context, store *project.Store, runID int64) (toolExecutionRecord, bool, error) {
	var record toolExecutionRecord
	err := store.DB().WithContext(ctx).Table("agent_tool_executions").Where("run_id=? AND state IN ('intent','executing')", runID).Order("created_at,id").First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, false, nil
	}
	return record, err == nil, err
}

func (service *Service) refreshRun(ctx context.Context, store *project.Store, runID int64) (runRecord, error) {
	var run runRecord
	err := store.DB().WithContext(ctx).Where("id=?", runID).First(&run).Error
	return run, err
}

func (service *Service) cancelRequested(ctx context.Context, store *project.Store, runID int64) (bool, error) {
	var cancelled bool
	err := store.DB().WithContext(ctx).Raw(`SELECT cancel_requested_at IS NOT NULL OR status='cancelled' FROM chat_runs WHERE id=?`, runID).Scan(&cancelled).Error
	return cancelled, err
}

func (service *Service) recordModelStart(ctx context.Context, store *project.Store, tc toolContext, contextBytes int) (int, error) {
	now := service.now().UTC()
	var ordinal int
	err := store.DB().WithContext(ctx).Raw(`UPDATE chat_runs
		SET context_bytes=?,model_request_count=model_request_count+1,updated_at=?
		WHERE id=? AND status='in_progress'
		RETURNING model_request_count`, contextBytes, now, tc.Run.ID).Row().Scan(&ordinal)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domainError(CodeStateConflict, "Model Request 无法开始", "Run 已不在执行状态。", nil)
	}
	if err != nil {
		return 0, err
	}
	return ordinal, nil
}

func (service *Service) recordModelRequestEvent(ctx context.Context, store *project.Store, tc toolContext, eventType, status string) error {
	if !isUUIDv7(tc.RequestUUID) || tc.RequestOrdinal < 1 {
		return domainError(CodeStateConflict, "Model Request 关联无效", "request_uuid 与 request_ordinal 必须来自已持久化的 LLM Log。", nil)
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
	now := service.now().UTC()
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, eventType, map[string]any{
		"project_uuid":    tc.ProjectUUID,
		"thread_uuid":     tc.Thread.UUID,
		"turn_uuid":       tc.Turn.UUID,
		"run_uuid":        tc.Run.UUID,
		"request_uuid":    tc.RequestUUID,
		"request_ordinal": tc.RequestOrdinal,
		"status":          status,
	}, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextEventSequence, now, thread.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:model_request_changed", map[string]any{
		"project_uuid":    tc.ProjectUUID,
		"thread_uuid":     tc.Thread.UUID,
		"turn_uuid":       tc.Turn.UUID,
		"run_uuid":        tc.Run.UUID,
		"request_uuid":    tc.RequestUUID,
		"request_ordinal": tc.RequestOrdinal,
		"status":          status,
	})
	return nil
}

func (service *Service) completeRun(ctx context.Context, store *project.Store, tc toolContext, content string, completionMetadata map[string]any) error {
	followUpPromptSnapshot := contextPromptSet{}
	var queuedFollowUps int64
	if err := store.DB().WithContext(ctx).Table("chat_follow_ups").Where("thread_id=? AND status='queued' AND deleted_at IS NULL", tc.Thread.ID).Count(&queuedFollowUps).Error; err != nil {
		return err
	}
	if queuedFollowUps > 0 {
		items, err := loadContextItems(ctx, store, tc.Thread.ID, tc.Turn.ID, tc.Turn.QueueSequence)
		if err != nil {
			return err
		}
		var frozen bool
		followUpPromptSnapshot, frozen = frozenContextPrompts(items, tc.Turn.ID)
		if !frozen {
			followUpPromptSnapshot, err = service.loadContextPrompts(ctx, store, tc.Thread)
			if err != nil {
				return err
			}
		}
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
	now := service.now().UTC()
	metadata := map[string]any{}
	for key, value := range completionMetadata {
		metadata[key] = value
	}
	runtimeGenerated, _ := metadata["runtime_generated"].(bool)
	if !runtimeGenerated && isUUIDv7(tc.RequestUUID) {
		metadata["request_uuid"] = tc.RequestUUID
		metadata["request_ordinal"] = tc.RequestOrdinal
	}
	item, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "assistant_message", "assistant", content, "text", "completed", "", "", "", metadata, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status='completed',completed_at=?,updated_at=?,error_code='',error_message='' WHERE id=? AND status='in_progress' AND cancel_requested_at IS NULL`, now, now, tc.Run.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status='completed',completed_at=?,updated_at=?,error_code='',error_message='' WHERE id=? AND status='in_progress' AND cancel_requested_at IS NULL`, now, now, tc.Turn.ID); err != nil {
		return err
	}
	completedPayload := map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "item_uuid": item.UUID, "status": TurnCompleted}
	if runtimeGenerated {
		completedPayload["runtime_generated"] = true
		if reason, _ := metadata["completion_reason"].(string); reason != "" {
			completedPayload["completion_reason"] = reason
		}
	} else if isUUIDv7(tc.RequestUUID) {
		completedPayload["request_uuid"] = tc.RequestUUID
		completedPayload["request_ordinal"] = tc.RequestOrdinal
	}
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "run_completed", completedPayload, now); err != nil {
		return err
	}
	if err := service.promoteNextFollowUpTx(ctx, tx, tc.ProjectUUID, &thread, followUpPromptSnapshot); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
		return err
	}
	if _, err := RecomputeThreadStatusTx(ctx, tx, thread.ID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:run_status", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "item_uuid": item.UUID, "status": TurnCompleted})
	return nil
}

func (service *Service) promoteNextFollowUpTx(ctx context.Context, tx *sql.Tx, projectUUID string, thread *threadRecord, promptSnapshot contextPromptSet) error {
	var follow followUpRecord
	err := tx.QueryRowContext(ctx, `SELECT id,uuid,thread_id,input_text,position,status,promoted_turn_id,created_at,updated_at,deleted_at FROM chat_follow_ups WHERE thread_id=? AND status='queued' AND deleted_at IS NULL ORDER BY position,id LIMIT 1`, thread.ID).Scan(&follow.ID, &follow.UUID, &follow.ThreadID, &follow.InputText, &follow.Position, &follow.Status, &follow.PromotedTurnID, &follow.CreatedAt, &follow.UpdatedAt, &follow.DeletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	references, err := loadFollowUpReferencesTx(ctx, tx, follow.ID)
	if err != nil {
		return err
	}
	turn, _, err := service.createTurnTx(ctx, tx, projectUUID, thread, follow.InputText, "follow_up", follow.ID, promptSnapshot, references)
	if err != nil {
		return err
	}
	now := service.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE chat_follow_ups SET status='promoted',promoted_turn_id=?,updated_at=? WHERE id=? AND status='queued'`, turn.ID, now, follow.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_follow_ups SET position=position-1,updated_at=? WHERE thread_id=? AND status='queued' AND deleted_at IS NULL AND position>?`, now, thread.ID, follow.Position); err != nil {
		return err
	}
	return nil
}

func (service *Service) failRun(ctx context.Context, store *project.Store, tc toolContext, code, message string) error {
	return service.finishRunWithStatus(ctx, store, tc, TurnFailed, code, message)
}

func (service *Service) cancelRun(ctx context.Context, store *project.Store, tc toolContext) error {
	return service.finishRunWithStatus(ctx, store, tc, TurnCancelled, CodeCancelled, "用户已取消。")
}

func (service *Service) finishRunWithStatus(ctx context.Context, store *project.Store, tc toolContext, status, code, message string) error {
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
	now := service.now().UTC()
	item, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "error", "system", message, "text", "failed", "", "", "", map[string]any{"error_code": code}, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status=?,error_code=?,error_message=?,completed_at=?,updated_at=? WHERE id=? AND status NOT IN ('completed','cancelled')`, status, code, message, now, now, tc.Run.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status=?,error_code=?,error_message=?,completed_at=?,updated_at=? WHERE id=? AND status NOT IN ('completed','cancelled')`, status, code, message, now, now, tc.Turn.ID); err != nil {
		return err
	}
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "run_"+status, map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "item_uuid": item.UUID, "status": status, "error_code": code}, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
		return err
	}
	if _, err := RecomputeThreadStatusTx(ctx, tx, thread.ID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:run_status", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "status": status, "error_code": code})
	return nil
}

func (service *Service) failOrRetryRun(ctx context.Context, store *project.Store, tc toolContext, cause error) error {
	retryable := false
	var llmErr *llm.Error
	var providerErr *provider.Error
	var agentErr *Error
	switch {
	case errors.As(cause, &llmErr):
		retryable = llmErr.Retryable
	case errors.As(cause, &providerErr):
		retryable = providerErr.Code == provider.CodeSecretStoreFailed
	case errors.As(cause, &agentErr):
		retryable = agentErr.Retryable
	}
	if retryable {
		now := service.now().UTC()
		if err := store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&runRecord{}).Where("id=? AND status='in_progress'", tc.Run.ID).Updates(map[string]any{"status": TurnQueued, "error_code": errorCode(cause), "error_message": safeMessage(cause), "updated_at": now}).Error; err != nil {
				return err
			}
			return tx.Model(&turnRecord{}).Where("id=? AND status='in_progress'", tc.Turn.ID).Updates(map[string]any{"status": TurnQueued, "error_code": errorCode(cause), "error_message": safeMessage(cause), "updated_at": now}).Error
		}); err != nil {
			return err
		}
		return cause
	}
	if err := service.failRun(ctx, store, tc, errorCode(cause), safeMessage(cause)); err != nil {
		return err
	}
	return &Error{Code: errorCode(cause), Message: safeMessage(cause), Cause: cause}
}

func errorCode(err error) string {
	var agentErr *Error
	var llmErr *llm.Error
	var providerErr *provider.Error
	switch {
	case errors.As(err, &agentErr):
		return agentErr.Code
	case errors.As(err, &llmErr):
		return llmErr.Code
	case errors.As(err, &providerErr):
		return providerErr.Code
	case errors.Is(err, context.Canceled):
		return CodeCancelled
	}
	return CodeProvider
}

func safeMessage(err error) string {
	var agentErr *Error
	var llmErr *llm.Error
	var providerErr *provider.Error
	switch {
	case errors.As(err, &agentErr):
		return agentErr.Message
	case errors.As(err, &llmErr):
		return llmErr.SafeMessage
	case errors.As(err, &providerErr):
		return providerErr.Message
	case errors.Is(err, context.Canceled):
		return "用户已取消。"
	}
	return "Agent 运行失败。"
}

// recordModelFinish uses this migration-backed type without exporting database IDs.
var _ = json.RawMessage{}
