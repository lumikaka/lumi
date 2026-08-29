package agent

import (
	"context"
	"encoding/json"
	"strings"

	"lumi/internal/project"
)

const (
	bootstrapYoloConfirmationQuestionID = "confirm_setup_and_start_yolo"
	bootstrapYoloIdempotencyPrefix      = "project-creation-yolo:"
)

func isBootstrapToolContext(tc toolContext) bool {
	return isUUIDv7(strings.TrimSpace(tc.BootstrapCreationSessionUUID))
}

func bootstrapProductionRequiresYoloError() error {
	return domainError(
		CodeBootstrapProductionRequiresYolo,
		"首次项目创建必须通过 YOLO 生产",
		"bootstrap 首个 Turn 定稿后只能读取项目事实或创建受控 YOLO Workflow；不得手工创建 Chapter、Premise、Section、图片、生成或导出任务。",
		nil,
	)
}

func bootstrapYoloNotAuthorizedError() error {
	return domainError(
		CodeBootstrapYoloNotAuthorized,
		"尚未获得启动 YOLO 的明确授权",
		"当前 Run 必须已消费 confirm_setup_and_start_yolo 的确认选项，且绑定的 Project Setup finalization 已成功。",
		nil,
	)
}

type bootstrapConfirmationEvidence struct {
	ExecutionID   int64
	RequestUUID   string
	ArgumentsJSON string
	SchemaVersion string
	RequestJSON   string
	ResponseJSON  string
}

func bootstrapYoloAuthorized(ctx context.Context, store *project.Store, tc toolContext) (bool, error) {
	if store == nil || store.SetupStatus() != project.SetupStatusReady || !isBootstrapToolContext(tc) || tc.Run.ID == 0 {
		return false, nil
	}
	var confirmations []bootstrapConfirmationEvidence
	if err := store.DB().WithContext(ctx).Table("agent_tool_executions AS executions").
		Select("executions.id AS execution_id,requests.uuid AS request_uuid,executions.arguments_json,requests.schema_version,requests.request_json,requests.response_json").
		Joins("JOIN chat_user_input_requests AS requests ON requests.run_id=executions.run_id AND requests.tool_call_uuid=executions.tool_call_uuid").
		Where("executions.run_id=? AND executions.tool_name='request_user_input' AND executions.state='completed' AND requests.status='resumed' AND requests.response_json IS NOT NULL", tc.Run.ID).
		Order("executions.id DESC").
		Scan(&confirmations).Error; err != nil {
		return false, err
	}
	for _, evidence := range confirmations {
		binding, err := dangerousConfirmationFromArguments(evidence.ArgumentsJSON)
		if err != nil || binding == nil ||
			binding.Route != RouteProjectSetupFinalize ||
			binding.ProjectUUID != tc.ProjectUUID ||
			binding.TargetUUID != tc.ProjectUUID ||
			binding.QuestionID != bootstrapYoloConfirmationQuestionID {
			continue
		}
		selected, err := dangerousConfirmationSelected(evidence.SchemaVersion, evidence.RequestJSON, evidence.ResponseJSON, *binding)
		if err != nil || !selected {
			continue
		}
		var replays []struct {
			ArgumentsJSON string
			ResultJSON    string
		}
		if err := store.DB().WithContext(ctx).Table("agent_tool_executions").
			Select("arguments_json,result_json").
			Where("run_id=? AND id>? AND tool_name='request_api' AND state='completed' AND result_json IS NOT NULL", tc.Run.ID, evidence.ExecutionID).
			Order("id").
			Scan(&replays).Error; err != nil {
			return false, err
		}
		for _, replay := range replays {
			var arguments map[string]any
			var result struct {
				Success bool `json:"success"`
			}
			if json.Unmarshal([]byte(replay.ArgumentsJSON), &arguments) != nil ||
				!boolArg(arguments, "__confirmation_auto_replay") ||
				stringArg(arguments, "__confirmation_request_uuid") != evidence.RequestUUID ||
				stringArg(arguments, "__route_id") != RouteProjectSetupFinalize ||
				json.Unmarshal([]byte(replay.ResultJSON), &result) != nil || !result.Success {
				continue
			}
			return true, nil
		}
	}
	return false, nil
}
