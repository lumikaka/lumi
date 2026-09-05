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
	AwaitID, ToolExecutionID, WorkflowID                                int64
	AwaitStatus, WorkflowUUID, WorkflowKind, WorkflowStatus, ThreadUUID string
	CurrentStepKey, TaskUUID, ResourceUUID, OutputJSON, ErrorCode       string
	InputSnapshot                                                       string
	Steps                                                               []workflowResumeStep
}

type workflowResumeStep struct {
	UUID, StepKey, Status, TaskUUID, ResourceUUID, OutputJSON, ErrorCode, ErrorMessage string
	Position                                                                           int
}

type workflowTerminalStepSummary struct {
	UUID         string `json:"uuid"`
	StepKey      string `json:"step_key"`
	Position     int    `json:"position"`
	Status       string `json:"status"`
	TaskUUID     string `json:"task_uuid,omitempty"`
	ResourceUUID string `json:"resource_uuid,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
}

func (service *Service) resumeWorkflowAwait(ctx context.Context, store *project.Store, tc toolContext) (bool, error) {
	var state workflowResumeState
	err := store.DB().WithContext(ctx).Raw(`SELECT a.id,a.tool_execution_id,a.status,w.id,w.uuid,w.kind,w.status,th.uuid,w.current_step_key,w.error_code,w.input_snapshot
		FROM workflow_awaits a
		JOIN workflows w ON w.id=a.workflow_id
		JOIN chat_threads th ON th.id=a.chat_thread_id
		WHERE a.chat_run_id=? AND a.status IN ('ready','resuming')
		ORDER BY a.id LIMIT 1`, tc.Run.ID).Row().Scan(
		&state.AwaitID, &state.ToolExecutionID, &state.AwaitStatus, &state.WorkflowID, &state.WorkflowUUID,
		&state.WorkflowKind, &state.WorkflowStatus, &state.ThreadUUID, &state.CurrentStepKey, &state.ErrorCode, &state.InputSnapshot,
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
	if err := store.DB().WithContext(ctx).Raw(`SELECT uuid,step_key,position,status,COALESCE(task_uuid,''),COALESCE(resource_uuid,''),COALESCE(output_json,'{}'),COALESCE(error_code,''),COALESCE(error_message,'')
		FROM workflow_steps WHERE workflow_id=? ORDER BY position,id`, state.WorkflowID).Scan(&state.Steps).Error; err != nil {
		return false, err
	}
	if state.WorkflowKind != WorkflowYolo && len(state.Steps) > 0 {
		step := state.Steps[len(state.Steps)-1]
		state.TaskUUID, state.ResourceUUID, state.OutputJSON = step.TaskUUID, step.ResourceUUID, step.OutputJSON
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
	// Reset before advancing the await marker. If the process exits between
	// these writes, recovery may repeat this idempotent reset but cannot miss it.
	if err := service.resetNoProgress(ctx, store, tc.Run.ID); err != nil {
		return false, err
	}
	if err := store.DB().WithContext(ctx).Table("workflow_awaits").Where("id=? AND status IN ('ready','resuming')", state.AwaitID).Updates(map[string]any{"status": "resumed", "resumed_at": now, "updated_at": now}).Error; err != nil {
		return false, err
	}
	return true, nil
}

func workflowTerminalToolResult(state workflowResumeState) json.RawMessage {
	if state.WorkflowKind == WorkflowYolo {
		currentStepKey := strings.TrimSpace(state.CurrentStepKey)
		if currentStepKey == "" && len(state.Steps) > 0 {
			if state.WorkflowStatus == WorkflowCompleted {
				currentStepKey = state.Steps[len(state.Steps)-1].StepKey
			} else {
				currentStepKey = state.Steps[0].StepKey
			}
		}
		steps := make([]workflowTerminalStepSummary, 0, len(state.Steps))
		for _, step := range state.Steps {
			steps = append(steps, workflowTerminalStepSummary{
				UUID: publicUUIDOrEmpty(step.UUID), StepKey: step.StepKey, Position: step.Position, Status: step.Status,
				TaskUUID: publicUUIDOrEmpty(step.TaskUUID), ResourceUUID: publicUUIDOrEmpty(step.ResourceUUID), ErrorCode: strings.TrimSpace(step.ErrorCode),
			})
		}
		data := map[string]any{
			"workflow_uuid":     publicUUIDOrEmpty(state.WorkflowUUID),
			"thread_uuid":       publicUUIDOrEmpty(state.ThreadUUID),
			"presentation_mode": string(PresentationInline),
			"kind":              state.WorkflowKind,
			"status":            state.WorkflowStatus,
			"current_step_key":  currentStepKey,
			"steps":             steps,
		}
		if state.WorkflowStatus == WorkflowCompleted {
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
			"error":   map[string]any{"code": code, "message": "异步生成未完成。", "details": ""},
		})
		return encoded
	}
	if state.WorkflowKind == WorkflowComicImageBatch {
		var snapshot struct {
			ChapterUUID string `json:"chapter_uuid"`
		}
		_ = json.Unmarshal([]byte(state.InputSnapshot), &snapshot)
		tasks := make([]map[string]any, 0, len(state.Steps))
		for _, step := range state.Steps {
			task := map[string]any{
				"uuid": publicUUIDOrEmpty(step.TaskUUID), "kind": "comic_image_generation",
				"resource_uuid": publicUUIDOrEmpty(step.ResourceUUID), "status": step.Status,
			}
			if code := strings.TrimSpace(step.ErrorCode); code != "" {
				task["error_code"] = code
			}
			if message := strings.TrimSpace(step.ErrorMessage); message != "" {
				task["error_message"] = message
			}
			tasks = append(tasks, task)
		}
		data := map[string]any{
			"workflow_uuid":   publicUUIDOrEmpty(state.WorkflowUUID),
			"chapter_uuid":    publicUUIDOrEmpty(snapshot.ChapterUUID),
			"requested_count": len(tasks), "accepted_count": len(tasks), "tasks": tasks,
		}
		if state.WorkflowStatus == WorkflowCompleted {
			encoded, _ := json.Marshal(map[string]any{"success": true, "data": data})
			return encoded
		}
		code := strings.TrimSpace(state.ErrorCode)
		if code == "" {
			code = "workflow_" + state.WorkflowStatus
		}
		encoded, _ := json.Marshal(map[string]any{
			"success": false, "data": data,
			"error": map[string]any{"code": code, "message": "批量图片生成未全部完成。", "details": ""},
		})
		return encoded
	}
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
