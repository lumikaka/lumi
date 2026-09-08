package agent

import (
	"context"
	"encoding/json"
	"strings"

	"lumi/internal/project"
	"lumi/internal/providerdiag"
)

// WorkflowProviderError exposes only the bounded diagnostic fields from a
// failed request, never its prompt, response body, or internal database IDs.
type WorkflowProviderError struct {
	ModelRequestUUID string `json:"model_request_uuid"`
	HTTPStatus       int    `json:"http_status,omitempty"`
	Code             string `json:"code,omitempty"`
	Message          string `json:"message,omitempty"`
}

func workflowProviderErrors(ctx context.Context, store *project.Store, workflowID int64) (map[string]*WorkflowProviderError, error) {
	var rows []struct {
		StepKey          string
		ModelRequestUUID string
		HTTPStatus       int
		Code             string
		Message          string
	}
	// Use the latest request of the current step attempt, including successful
	// requests, so an earlier transient failure cannot explain a later failure.
	err := store.DB().WithContext(ctx).Raw(`SELECT s.step_key,l.uuid AS model_request_uuid,l.http_status,l.provider_error_code AS code,l.error_message AS message
 FROM workflow_steps s JOIN workflows w ON w.id=s.workflow_id
 JOIN llm_logs l ON l.id=(SELECT candidate.id FROM llm_logs candidate
   WHERE candidate.project_id=w.project_id
     AND (s.started_at IS NULL OR candidate.created_at>=s.started_at)
     AND ((candidate.workflow_id=w.id AND candidate.workflow_step_id=s.id)
       OR candidate.production_task_run_id IN (SELECT id FROM production_task_runs WHERE uuid=s.task_uuid AND project_id=w.project_id)
       OR candidate.task_run_id IN (SELECT id FROM task_runs WHERE uuid=s.task_uuid AND project_id=w.project_id))
   ORDER BY candidate.created_at DESC,candidate.id DESC LIMIT 1)
 WHERE w.id=? AND s.status='failed' AND l.status='failed' AND l.error_code=s.error_code
   AND (l.http_status>0 OR l.provider_error_code<>'')`, workflowID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]*WorkflowProviderError, len(rows))
	for _, row := range rows {
		result[row.StepKey] = &WorkflowProviderError{
			ModelRequestUUID: publicUUIDOrEmpty(row.ModelRequestUUID), HTTPStatus: row.HTTPStatus,
			Code:    providerdiag.RedactPreview(row.Code, "", 255),
			Message: providerdiag.RedactPreview(row.Message, "", 2000),
		}
	}
	return result, nil
}

func (failure *WorkflowProviderError) summary() string {
	if failure.Code == "IPInfringementSuspect" {
		return "图片生成被拒绝：输出疑似涉及知识产权侵权。"
	}
	return "生成失败，具体原因请查看 Provider 诊断信息。"
}

func workflowTerminalToolResult(state workflowResumeState) json.RawMessage {
	result := workflowTerminalToolResultBase(state)
	if state.WorkflowStatus != WorkflowFailed || state.ProviderError == nil {
		return result
	}
	var payload map[string]any
	if json.Unmarshal(result, &payload) != nil {
		return result
	}
	failure, ok := payload["error"].(map[string]any)
	if !ok {
		return result
	}
	failure["message"] = state.ProviderError.summary()
	failure["details"] = strings.TrimSpace(state.ProviderError.Code + "\n" + state.ProviderError.Message)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return result
	}
	return encoded
}
