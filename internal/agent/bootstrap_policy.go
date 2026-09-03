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
		"首次项目创建必须通过自动生成流程生产",
		"bootstrap 首个 Turn 定稿后只能读取项目事实或创建受控自动生成 Workflow；不得手工创建 Chapter、Premise、Section、图片、生成或导出任务。",
		nil,
	)
}

func bootstrapYoloNotAuthorizedError() error {
	return domainError(
		CodeBootstrapYoloNotAuthorized,
		"尚未获得开始自动生成的明确授权",
		"当前 Run 必须已消费运行时生成的 Project Setup 定稿确认，且绑定的 finalization 已成功。",
		nil,
	)
}

type bootstrapConfirmationEvidence struct {
	ExecutionID   int64
	RunID         int64
	RequestUUID   string
	ArgumentsJSON string
	SchemaVersion string
	RequestJSON   string
	ResponseJSON  string
}

func bootstrapYoloAuthorized(ctx context.Context, store *project.Store, tc toolContext) (bool, error) {
	_, authorized, err := bootstrapYoloAuthorizationEvidence(ctx, store, tc, false)
	return authorized, err
}

func bootstrapYoloAuthorizedForExecution(ctx context.Context, store *project.Store, tc toolContext, execution toolExecutionRecord) (bool, error) {
	runtimeGenerated, confirmationRequestUUID, creationSessionUUID := runtimeGeneratedBootstrapIntent(execution)
	if runtimeGenerated {
		effectiveSessionUUID := strings.TrimSpace(tc.BootstrapCreationSessionUUID)
		if !isUUIDv7(effectiveSessionUUID) {
			effectiveSessionUUID = strings.TrimSpace(tc.BootstrapLineageCreationSessionUUID)
		}
		if creationSessionUUID != effectiveSessionUUID || !isUUIDv7(confirmationRequestUUID) {
			return false, nil
		}
		// A later Turn deliberately does not inherit origin-only bootstrap
		// authority. Runtime-generated recovery binds that Turn back to the
		// durable thread lineage and the exact confirmed request instead.
		tc.BootstrapCreationSessionUUID = effectiveSessionUUID
	}
	evidence, authorized, err := bootstrapYoloAuthorizationEvidence(ctx, store, tc, runtimeGenerated)
	if err != nil || !authorized {
		return authorized, err
	}
	if runtimeGenerated && evidence.RequestUUID != confirmationRequestUUID {
		return false, nil
	}
	return true, nil
}

func bootstrapYoloAuthorizationEvidence(ctx context.Context, store *project.Store, tc toolContext, allowThreadEvidence bool) (bootstrapConfirmationEvidence, bool, error) {
	if store == nil || store.SetupStatus() != project.SetupStatusReady || !isBootstrapToolContext(tc) || tc.Run.ID == 0 {
		return bootstrapConfirmationEvidence{}, false, nil
	}
	setup, err := store.ProjectSetup(ctx)
	if err != nil {
		return bootstrapConfirmationEvidence{}, false, err
	}
	var confirmations []bootstrapConfirmationEvidence
	query := store.DB().WithContext(ctx).Table("agent_tool_executions AS executions").
		Select("executions.id AS execution_id,executions.run_id,requests.uuid AS request_uuid,executions.arguments_json,requests.schema_version,requests.request_json,requests.response_json").
		Joins("JOIN chat_user_input_requests AS requests ON requests.run_id=executions.run_id AND requests.tool_call_uuid=executions.tool_call_uuid").
		Where("executions.tool_name='request_user_input' AND executions.state='completed' AND requests.status='resumed' AND requests.response_json IS NOT NULL")
	if allowThreadEvidence {
		query = query.Where("executions.thread_id=?", tc.Thread.ID)
	} else {
		query = query.Where("executions.run_id=?", tc.Run.ID)
	}
	if err := query.Order("executions.id DESC").Scan(&confirmations).Error; err != nil {
		return bootstrapConfirmationEvidence{}, false, err
	}
	for _, evidence := range confirmations {
		binding, err := dangerousConfirmationFromArguments(evidence.ArgumentsJSON)
		if err != nil || binding == nil ||
			binding.Route != RouteProjectSetupFinalize ||
			binding.ProjectUUID != tc.ProjectUUID ||
			binding.TargetUUID != tc.ProjectUUID ||
			(setup.Revision > 0 && binding.ExpectedRevision != setup.Revision) ||
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
			Where("run_id=? AND id>? AND tool_name='request_api' AND state='completed' AND result_json IS NOT NULL", evidence.RunID, evidence.ExecutionID).
			Order("id").
			Scan(&replays).Error; err != nil {
			return bootstrapConfirmationEvidence{}, false, err
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
			return evidence, true, nil
		}
	}
	return bootstrapConfirmationEvidence{}, false, nil
}

func runtimeGeneratedBootstrapIntent(execution toolExecutionRecord) (bool, string, string) {
	var arguments map[string]any
	if json.Unmarshal([]byte(execution.ArgumentsJSON), &arguments) != nil || !boolArg(arguments, "__runtime_generated_bootstrap") || stringArg(arguments, "__bootstrap_action") != "start_generation" {
		return false, "", ""
	}
	return true, stringArg(arguments, "__bootstrap_confirmation_request_uuid"), stringArg(arguments, "__bootstrap_creation_session_uuid")
}
