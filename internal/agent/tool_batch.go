package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"lumi/internal/llm"
	"lumi/internal/project"
)

// preparedToolIntent is the side-effect-free result of validating one model
// tool call. A complete model response is prepared before any intent is
// inserted, so an invalid sibling can never leave a valid write behind.
type preparedToolIntent struct {
	Call             llm.ToolCall
	Key              string
	Args             map[string]any
	APIRequest       agentAPIRequest
	Existing         toolExecutionRecord
	PersistedResult  json.RawMessage
	Completed        bool
	New              bool
	ArgumentRepairs  []string
	PublicCallUUID   string
	ExecutionUUID    string
	TargetUUID       string
	RouteID          string
	Action           string
	Method           string
	Path             string
	EncodedArguments string
}

func (service *Service) prepareToolIntentBatch(ctx context.Context, store *project.Store, tc toolContext, calls []llm.ToolCall) ([]preparedToolIntent, llm.ToolCall, error) {
	prepared := make([]preparedToolIntent, 0, len(calls))
	seenCallIDs := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
			return nil, call, domainError(CodeStateConflict, "Provider Tool Call 无法配对", "工具调用缺少稳定的 ID 或名称，整批未持久化。", nil)
		}
		if _, exists := seenCallIDs[call.ID]; exists {
			return nil, call, domainError(CodeStateConflict, "Provider Tool Call ID 重复", "同一响应中的工具调用 ID 必须唯一，整批未持久化。", nil)
		}
		seenCallIDs[call.ID] = struct{}{}
		intent, err := service.prepareToolIntent(ctx, store, tc, call)
		if err != nil {
			return nil, call, err
		}
		prepared = append(prepared, intent)
	}
	return prepared, llm.ToolCall{}, nil
}

func (service *Service) prepareToolIntent(ctx context.Context, store *project.Store, tc toolContext, call llm.ToolCall) (preparedToolIntent, error) {
	intent := preparedToolIntent{Call: call, Key: toolCallKey(tc.Run.UUID, call.ID, call.Name)}
	sqlDB, err := store.DB().DB()
	if err != nil {
		return intent, err
	}
	existing, found, err := findToolExecutionByProviderCall(ctx, sqlDB, tc.Run.ID, call.ID, intent.Key)
	if err != nil {
		return intent, err
	}
	args, validationErr := validateToolArgumentsForProtocol(call.Name, call.Arguments, tc.ToolMode, tc.ToolProtocol)
	if validationErr != nil {
		if found {
			return intent, domainError(CodeStateConflict, "Provider Tool Call ID 重用冲突", "既有 Provider Tool Call ID 不能改为无效或无法配对的 arguments，整批未持久化。", validationErr)
		}
		return intent, validationErr
	}
	intent.Args = args
	intent.ArgumentRepairs = projectAPIV4ArgumentRepairs(call.Name, tc.ToolProtocol, tc.ToolMode, call.Arguments, args)
	if found {
		if matchErr := validatePersistedToolCallPair(existing, call, args); matchErr != nil {
			return intent, matchErr
		}
		intent.Existing = existing
		if intent.Existing.State == "completed" && intent.Existing.ResultJSON != nil {
			intent.PersistedResult = json.RawMessage(*intent.Existing.ResultJSON)
			intent.Completed = true
		}
		return intent, nil
	}
	if !toolAllowedForThreadMode(call.Name, tc.Thread, tc.ToolMode) {
		return intent, domainError(CodeToolNotAllowed, "工具不适用于当前 Run", "当前冻结的 Tool Mode 无法使用该工具。", nil)
	}
	if normalizedToolMode(tc.ToolMode) == ToolModeLegacyTyped {
		if err := validateLegacyRecoveryIntent(tc, call.Name, args); err != nil {
			return intent, err
		}
	}
	if call.Name == "request_api" {
		intent.APIRequest, err = service.parseAgentAPIRequest(tc, args)
		if err != nil {
			return intent, err
		}
		intent.ArgumentRepairs = append(intent.ArgumentRepairs, intent.APIRequest.ArgumentRepairs...)
	} else if call.Name == "read_agent_doc" {
		if _, err := service.readAgentDoc(tc, args); err != nil {
			return intent, err
		}
	}
	if normalizedToolMode(tc.ToolMode) == ToolModeLegacyTyped && !legacyRecoveryToolTargetAllowed(call.Name, args, tc.Thread) {
		return intent, domainError(CodeToolNotAllowed, "旧协议工具目标越界", "恢复中的旧设定项引用只能读写其冻结 subject_uuid。", nil)
	}

	intent.PublicCallUUID, err = newUUIDv7()
	if err != nil {
		return intent, err
	}
	intent.ExecutionUUID, err = newUUIDv7()
	if err != nil {
		return intent, err
	}
	intent.Action = call.Name
	switch call.Name {
	case "image_gen":
		intent.TargetUUID = tc.Thread.UUID
	case "request_api":
		intent.TargetUUID = intent.APIRequest.TargetUUID
		intent.RouteID, intent.Action = intent.APIRequest.Route.ID, intent.APIRequest.Route.Action
		intent.Method, intent.Path = intent.APIRequest.Method, intent.APIRequest.Path
	case "read_agent_doc":
		intent.TargetUUID = tc.Thread.UUID
		intent.RouteID, intent.Action, intent.Method = "agent_doc.read", "读取 Agent 文档", "READ"
		intent.Path = stringArg(args, "path")
	default:
		if normalizedToolMode(tc.ToolMode) == ToolModeLegacyTyped {
			intent.TargetUUID = legacyRecoveryTargetUUID(call.Name, args, tc.Thread)
		}
	}
	storedArgs := make(map[string]any, len(args)+8)
	for key, value := range args {
		storedArgs[key] = value
	}
	storedArgs["__provider_call_id"] = call.ID
	if isUUIDv7(tc.RequestUUID) {
		storedArgs["__request_uuid"] = tc.RequestUUID
		storedArgs["__request_ordinal"] = tc.RequestOrdinal
	}
	if intent.RouteID != "" {
		storedArgs["__route_id"], storedArgs["__action"] = intent.RouteID, intent.Action
		storedArgs["__method"], storedArgs["__path"] = intent.Method, intent.Path
		storedArgs["__target_uuid"] = intent.TargetUUID
	}
	encoded, err := json.Marshal(storedArgs)
	if err != nil {
		return intent, err
	}
	intent.EncodedArguments = string(encoded)
	intent.New = true
	return intent, nil
}

type preparedToolBroadcast struct {
	Call   map[string]any
	Result preparedToolIntent
}

type toolExecutionQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func findToolExecutionByProviderCall(ctx context.Context, queryer toolExecutionQueryer, runID int64, providerCallID, idempotencyKey string) (toolExecutionRecord, bool, error) {
	row := queryer.QueryRowContext(ctx, `SELECT
		id,thread_id,run_id,turn_id,item_id,uuid,tool_call_uuid,tool_name,target_uuid,
		arguments_json,idempotency_key,state,result_json,error_code,error_message,
		started_at,completed_at,created_at,updated_at
		FROM agent_tool_executions
		WHERE run_id=? AND (json_extract(arguments_json,'$.__provider_call_id')=? OR idempotency_key=?)
		ORDER BY id LIMIT 1`, runID, providerCallID, idempotencyKey)
	var record toolExecutionRecord
	var resultJSON sql.NullString
	var startedAt, completedAt sql.NullTime
	if err := row.Scan(
		&record.ID, &record.ThreadID, &record.RunID, &record.TurnID, &record.ItemID,
		&record.UUID, &record.ToolCallUUID, &record.ToolName, &record.TargetUUID,
		&record.ArgumentsJSON, &record.IdempotencyKey, &record.State, &resultJSON,
		&record.ErrorCode, &record.ErrorMessage, &startedAt, &completedAt,
		&record.CreatedAt, &record.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return toolExecutionRecord{}, false, nil
		}
		return toolExecutionRecord{}, false, err
	}
	if resultJSON.Valid {
		record.ResultJSON = &resultJSON.String
	}
	if startedAt.Valid {
		record.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		record.CompletedAt = &completedAt.Time
	}
	return record, true, nil
}

func validatePersistedToolCallPair(existing toolExecutionRecord, call llm.ToolCall, normalizedArgs map[string]any) error {
	var persisted map[string]any
	if json.Unmarshal([]byte(existing.ArgumentsJSON), &persisted) != nil || persisted == nil {
		return domainError(CodeStateConflict, "持久化 Tool Call 无法配对", "既有工具调用参数已损坏，不能安全重放。", nil)
	}
	if existing.ToolName != call.Name {
		return domainError(CodeStateConflict, "Provider Tool Call ID 重用冲突", "同一 Provider Tool Call ID 不能绑定不同工具，整批未持久化。", nil)
	}
	if persistedID, _ := persisted["__provider_call_id"].(string); persistedID != "" && persistedID != call.ID {
		return domainError(CodeStateConflict, "Provider Tool Call ID 重用冲突", "Provider Tool Call ID 与既有调用不匹配，整批未持久化。", nil)
	}
	for key := range persisted {
		if strings.HasPrefix(key, "__") {
			delete(persisted, key)
		}
	}
	persistedJSON, persistedErr := json.Marshal(persisted)
	normalizedJSON, normalizedErr := json.Marshal(normalizedArgs)
	if persistedErr != nil || normalizedErr != nil || string(persistedJSON) != string(normalizedJSON) {
		return domainError(CodeStateConflict, "Provider Tool Call ID 重用冲突", "同一 Provider Tool Call ID 不能改变 arguments，整批未持久化。", errors.Join(persistedErr, normalizedErr))
	}
	return nil
}

func (service *Service) persistPreparedToolIntentBatch(ctx context.Context, store *project.Store, tc toolContext, prepared []preparedToolIntent) ([]preparedToolIntent, error) {
	hasNew := false
	for _, intent := range prepared {
		hasNew = hasNew || intent.New
	}
	if !hasNew {
		return prepared, nil
	}
	sqlDB, err := store.DB().DB()
	if err != nil {
		return nil, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	thread, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
	if err != nil {
		return nil, err
	}
	now := service.now().UTC()
	broadcasts := make([]preparedToolBroadcast, 0, len(prepared))
	for index := range prepared {
		intent := &prepared[index]
		if !intent.New {
			continue
		}
		if existing, found, findErr := findToolExecutionByProviderCall(ctx, tx, tc.Run.ID, intent.Call.ID, intent.Key); findErr != nil {
			return nil, findErr
		} else if found {
			if matchErr := validatePersistedToolCallPair(existing, intent.Call, intent.Args); matchErr != nil {
				return nil, matchErr
			}
			intent.Existing, intent.New = existing, false
			if existing.State == "completed" && existing.ResultJSON != nil {
				intent.PersistedResult = json.RawMessage(*existing.ResultJSON)
				intent.Completed = true
			}
			continue
		}
		metadata := map[string]any{"purpose": intent.Call.Name, "action": intent.Action, "target_uuid": intent.TargetUUID, "provider_call_id": intent.Call.ID}
		addArgumentRepairMetadata(metadata, intent.ArgumentRepairs)
		if isUUIDv7(tc.RequestUUID) {
			metadata["request_uuid"], metadata["request_ordinal"] = tc.RequestUUID, tc.RequestOrdinal
		}
		if intent.RouteID != "" {
			metadata["route_id"], metadata["method"], metadata["path"] = intent.RouteID, intent.Method, intent.Path
		}
		item, appendErr := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "tool_call", "assistant", intent.Call.Arguments, "json", "in_progress", intent.PublicCallUUID, intent.Call.Name, intent.TargetUUID, metadata, now)
		if appendErr != nil {
			return nil, appendErr
		}
		result, insertErr := tx.ExecContext(ctx, `INSERT INTO agent_tool_executions(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,'intent',?,?)`, intent.ExecutionUUID, tc.Thread.ID, tc.Run.ID, tc.Turn.ID, item.ID, intent.PublicCallUUID, intent.Call.Name, intent.TargetUUID, intent.EncodedArguments, intent.Key, now, now)
		if insertErr != nil {
			return nil, insertErr
		}
		id, insertErr := result.LastInsertId()
		if insertErr != nil {
			return nil, insertErr
		}
		intent.Existing = toolExecutionRecord{ID: id, ThreadID: tc.Thread.ID, RunID: tc.Run.ID, TurnID: tc.Turn.ID, ItemID: item.ID, UUID: intent.ExecutionUUID, ToolCallUUID: intent.PublicCallUUID, ToolName: intent.Call.Name, TargetUUID: intent.TargetUUID, ArgumentsJSON: intent.EncodedArguments, IdempotencyKey: intent.Key, RouteID: intent.RouteID, Action: intent.Action, Method: intent.Method, Path: intent.Path, State: "intent", CreatedAt: now, UpdatedAt: now}
		toolEvent := map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "tool_call_uuid": intent.PublicCallUUID, "tool_name": intent.Call.Name, "route_id": intent.RouteID, "action": intent.Action, "method": intent.Method, "path": intent.Path, "target_uuid": intent.TargetUUID}
		addArgumentRepairMetadata(toolEvent, intent.ArgumentRepairs)
		if isUUIDv7(tc.RequestUUID) {
			toolEvent["request_uuid"], toolEvent["request_ordinal"] = tc.RequestUUID, tc.RequestOrdinal
		}
		if _, appendErr = appendEventTx(ctx, tx, &thread, &tc.Run.ID, "tool_intent", toolEvent, now); appendErr != nil {
			return nil, appendErr
		}
		broadcasts = append(broadcasts, preparedToolBroadcast{Call: toolEvent, Result: *intent})
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for _, broadcast := range broadcasts {
		payload := broadcast.Call
		payload["status"] = "in_progress"
		service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:tool_call", payload)
	}
	return prepared, nil
}
