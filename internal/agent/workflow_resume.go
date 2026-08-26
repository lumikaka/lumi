package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"lumi/internal/project"
)

type workflowResumeState struct {
	AwaitID, ToolExecutionID                  int64
	AwaitStatus, WorkflowUUID, WorkflowStatus string
	TaskUUID, ResourceUUID, OutputJSON        string
	ErrorCode                                 string
}

func (service *Service) resumeWorkflowAwait(ctx context.Context, store *project.Store, tc toolContext) (bool, error) {
	var state workflowResumeState
	err := store.DB().WithContext(ctx).Raw(`SELECT a.id,a.tool_execution_id,a.status,w.uuid,w.status,s.task_uuid,s.resource_uuid,s.output_json,w.error_code
		FROM workflow_awaits a
		JOIN workflows w ON w.id=a.workflow_id
		JOIN workflow_steps s ON s.workflow_id=w.id
		WHERE a.chat_run_id=? AND a.status IN ('ready','resuming')
		ORDER BY a.id LIMIT 1`, tc.Run.ID).Row().Scan(
		&state.AwaitID, &state.ToolExecutionID, &state.AwaitStatus, &state.WorkflowUUID, &state.WorkflowStatus,
		&state.TaskUUID, &state.ResourceUUID, &state.OutputJSON, &state.ErrorCode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if state.WorkflowStatus != WorkflowCompleted && state.WorkflowStatus != WorkflowFailed && state.WorkflowStatus != WorkflowCancelled && state.WorkflowStatus != WorkflowInterrupted {
		return false, domainError(CodeStateConflict, "Workflow 尚未终止", "Chat Resume 只能读取持久化 Workflow 终态。", nil)
	}
	now := service.now().UTC()
	if err := store.DB().WithContext(ctx).Table("workflow_awaits").Where("id=? AND status='ready'", state.AwaitID).Updates(map[string]any{"status": "resuming", "updated_at": now}).Error; err != nil {
		return false, err
	}
	var execution toolExecutionRecord
	if err := store.DB().WithContext(ctx).Table("agent_tool_executions").Where("id=? AND run_id=?", state.ToolExecutionID, tc.Run.ID).First(&execution).Error; err != nil {
		return false, err
	}
	result := workflowTerminalToolResult(state)
	if err := service.persistToolResult(ctx, store, tc, execution, result); err != nil {
		return false, err
	}
	if err := store.DB().WithContext(ctx).Table("workflow_awaits").Where("id=? AND status IN ('ready','resuming')", state.AwaitID).Updates(map[string]any{"status": "resumed", "resumed_at": now, "updated_at": now}).Error; err != nil {
		return false, err
	}
	return true, nil
}

func workflowTerminalToolResult(state workflowResumeState) json.RawMessage {
	data := map[string]any{
		"workflow_uuid": state.WorkflowUUID,
		"task_uuid":     state.TaskUUID,
		"resource_uuid": state.ResourceUUID,
		"status":        state.WorkflowStatus,
	}
	if state.WorkflowStatus == WorkflowCompleted {
		var summary any
		sanitized := sanitizeDiagnosticJSON(state.OutputJSON)
		if json.Unmarshal(sanitized, &summary) == nil && summary != nil {
			data["result"] = summary
		}
		encoded, _ := json.Marshal(map[string]any{"success": true, "data": data})
		return encoded
	}
	code := strings.TrimSpace(state.ErrorCode)
	if code == "" {
		code = "workflow_" + state.WorkflowStatus
	}
	encoded, _ := json.Marshal(map[string]any{
		"success": false,
		"data":    data,
		"error": map[string]any{
			"code": code, "message": "异步生成未完成。", "details": "",
		},
	})
	return encoded
}
