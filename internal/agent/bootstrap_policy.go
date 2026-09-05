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
		"自动生成的前置定稿无法验证",
		"当前 bootstrap 必须存在由运行时生成并成功完成的 Project Setup finalization。",
		nil,
	)
}

type bootstrapAuthorizationEvidence struct {
	ExecutionID   int64
	RunID         int64
	EvidenceUUID  string
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
	runtimeGenerated, evidenceUUID, creationSessionUUID := runtimeGeneratedBootstrapWorkflowIntent(execution)
	if runtimeGenerated {
		effectiveSessionUUID := strings.TrimSpace(tc.BootstrapCreationSessionUUID)
		if !isUUIDv7(effectiveSessionUUID) {
			effectiveSessionUUID = strings.TrimSpace(tc.BootstrapLineageCreationSessionUUID)
		}
		if creationSessionUUID != effectiveSessionUUID || !isUUIDv7(evidenceUUID) {
			return false, nil
		}
		// A later Turn deliberately does not inherit origin-only bootstrap
		// authority. Runtime-generated recovery binds that Turn back to the
		// durable thread lineage and the exact successful finalization instead.
		tc.BootstrapCreationSessionUUID = effectiveSessionUUID
	}
	evidence, authorized, err := bootstrapYoloAuthorizationEvidenceMatching(ctx, store, tc, runtimeGenerated, evidenceUUID)
	if err != nil || !authorized {
		return authorized, err
	}
	if runtimeGenerated && evidence.EvidenceUUID != evidenceUUID {
		return false, nil
	}
	return true, nil
}

func bootstrapYoloAuthorizationEvidence(ctx context.Context, store *project.Store, tc toolContext, allowThreadEvidence bool) (bootstrapAuthorizationEvidence, bool, error) {
	return bootstrapYoloAuthorizationEvidenceMatching(ctx, store, tc, allowThreadEvidence, "")
}

func bootstrapYoloAuthorizationEvidenceMatching(ctx context.Context, store *project.Store, tc toolContext, allowThreadEvidence bool, requiredEvidenceUUID string) (bootstrapAuthorizationEvidence, bool, error) {
	if store == nil || store.SetupStatus() != project.SetupStatusReady || !isBootstrapToolContext(tc) || tc.Run.ID == 0 {
		return bootstrapAuthorizationEvidence{}, false, nil
	}
	setup, err := store.ProjectSetup(ctx)
	if err != nil {
		return bootstrapAuthorizationEvidence{}, false, err
	}

	// Current bootstrap runs finalize Setup directly from a trusted runtime
	// intent. The successful execution is the authorization fact for starting
	// the one controlled automatic-generation Workflow; no user confirmation
	// card is needed for this new-project-only transition.
	var finalizations []struct {
		ExecutionID   int64
		RunID         int64
		ExecutionUUID string
		TargetUUID    string
		ArgumentsJSON string
		ResultJSON    string
	}
	finalizationQuery := store.DB().WithContext(ctx).Table("agent_tool_executions AS executions").
		Select("executions.id AS execution_id,executions.run_id,executions.uuid AS execution_uuid,executions.target_uuid,executions.arguments_json,executions.result_json").
		Where("executions.tool_name='request_api' AND executions.state='completed' AND executions.result_json IS NOT NULL")
	if allowThreadEvidence {
		finalizationQuery = finalizationQuery.Where("executions.thread_id=?", tc.Thread.ID)
	} else {
		finalizationQuery = finalizationQuery.Where("executions.run_id=?", tc.Run.ID)
	}
	if err := finalizationQuery.Order("executions.id DESC").Scan(&finalizations).Error; err != nil {
		return bootstrapAuthorizationEvidence{}, false, err
	}
	route, _ := agentAPIRouteByID(RouteProjectSetupFinalize)
	for _, finalization := range finalizations {
		if requiredEvidenceUUID != "" && finalization.ExecutionUUID != requiredEvidenceUUID {
			continue
		}
		var arguments map[string]any
		var result struct {
			Success bool `json:"success"`
		}
		if json.Unmarshal([]byte(finalization.ArgumentsJSON), &arguments) != nil ||
			json.Unmarshal([]byte(finalization.ResultJSON), &result) != nil || !result.Success ||
			!boolArg(arguments, "__runtime_generated_bootstrap") ||
			stringArg(arguments, "__bootstrap_action") != "finalize" ||
			stringArg(arguments, "__bootstrap_creation_session_uuid") != tc.BootstrapCreationSessionUUID ||
			stringArg(arguments, "__route_id") != RouteProjectSetupFinalize ||
			stringArg(arguments, "__target_uuid") != tc.ProjectUUID ||
			finalization.TargetUUID != tc.ProjectUUID ||
			(setup.Revision > 0 && storedAgentAPIExpectedRevision(route, arguments) != setup.Revision) {
			continue
		}
		return bootstrapAuthorizationEvidence{
			ExecutionID: finalization.ExecutionID, RunID: finalization.RunID,
			EvidenceUUID: finalization.ExecutionUUID, ArgumentsJSON: finalization.ArgumentsJSON,
		}, true, nil
	}

	// Compatibility for a bootstrap that reached the former confirmation flow
	// before upgrading. Once that persisted confirmation has been selected and
	// its exact finalization replay has succeeded, it remains valid evidence.
	var confirmations []bootstrapAuthorizationEvidence
	query := store.DB().WithContext(ctx).Table("agent_tool_executions AS executions").
		Select("executions.id AS execution_id,executions.run_id,requests.uuid AS evidence_uuid,executions.arguments_json,requests.schema_version,requests.request_json,requests.response_json").
		Joins("JOIN chat_user_input_requests AS requests ON requests.run_id=executions.run_id AND requests.tool_call_uuid=executions.tool_call_uuid").
		Where("executions.tool_name='request_user_input' AND executions.state='completed' AND requests.status='resumed' AND requests.response_json IS NOT NULL")
	if allowThreadEvidence {
		query = query.Where("executions.thread_id=?", tc.Thread.ID)
	} else {
		query = query.Where("executions.run_id=?", tc.Run.ID)
	}
	if err := query.Order("executions.id DESC").Scan(&confirmations).Error; err != nil {
		return bootstrapAuthorizationEvidence{}, false, err
	}
	for _, evidence := range confirmations {
		if requiredEvidenceUUID != "" && evidence.EvidenceUUID != requiredEvidenceUUID {
			continue
		}
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
			return bootstrapAuthorizationEvidence{}, false, err
		}
		for _, replay := range replays {
			var arguments map[string]any
			var result struct {
				Success bool `json:"success"`
			}
			if json.Unmarshal([]byte(replay.ArgumentsJSON), &arguments) != nil ||
				!boolArg(arguments, "__confirmation_auto_replay") ||
				stringArg(arguments, "__confirmation_request_uuid") != evidence.EvidenceUUID ||
				stringArg(arguments, "__route_id") != RouteProjectSetupFinalize ||
				json.Unmarshal([]byte(replay.ResultJSON), &result) != nil || !result.Success {
				continue
			}
			return evidence, true, nil
		}
	}
	return bootstrapAuthorizationEvidence{}, false, nil
}

func runtimeGeneratedBootstrapFinalizationIntent(execution toolExecutionRecord) (bool, string) {
	var arguments map[string]any
	if json.Unmarshal([]byte(execution.ArgumentsJSON), &arguments) != nil ||
		!boolArg(arguments, "__runtime_generated_bootstrap") ||
		stringArg(arguments, "__bootstrap_action") != "finalize" {
		return false, ""
	}
	return true, stringArg(arguments, "__bootstrap_creation_session_uuid")
}

func runtimeBootstrapFinalizationSkipsConfirmation(tc toolContext, execution toolExecutionRecord, request agentAPIRequest) bool {
	runtimeGenerated, creationSessionUUID := runtimeGeneratedBootstrapFinalizationIntent(execution)
	if !runtimeGenerated || request.Route.ID != RouteProjectSetupFinalize ||
		execution.ID == 0 || execution.RunID != tc.Run.ID || execution.ThreadID != tc.Thread.ID ||
		execution.TargetUUID != tc.ProjectUUID || !isUUIDv7(execution.UUID) || !isUUIDv7(creationSessionUUID) {
		return false
	}
	if creationSessionUUID == strings.TrimSpace(tc.BootstrapCreationSessionUUID) {
		return true
	}
	return creationSessionUUID == strings.TrimSpace(tc.BootstrapLineageCreationSessionUUID) && bootstrapExplicitRecoveryRequest(tc.Turn.InputText)
}

func runtimeGeneratedBootstrapWorkflowIntent(execution toolExecutionRecord) (bool, string, string) {
	var arguments map[string]any
	if json.Unmarshal([]byte(execution.ArgumentsJSON), &arguments) != nil || !boolArg(arguments, "__runtime_generated_bootstrap") || stringArg(arguments, "__bootstrap_action") != "start_generation" {
		return false, "", ""
	}
	evidenceUUID := stringArg(arguments, "__bootstrap_finalization_execution_uuid")
	if evidenceUUID == "" {
		// Compatibility with Workflow intents persisted by the former
		// confirmation-based bootstrap implementation.
		evidenceUUID = stringArg(arguments, "__bootstrap_confirmation_request_uuid")
	}
	return true, evidenceUUID, stringArg(arguments, "__bootstrap_creation_session_uuid")
}
