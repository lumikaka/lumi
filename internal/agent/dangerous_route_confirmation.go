package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

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
	if inputType != "single_choice" || binding.ProjectUUID != tc.ProjectUUID || !isUUIDv7(binding.ProjectUUID) || !isUUIDv7(binding.TargetUUID) || binding.ExpectedRevision < 0 || !validAgentRequestFingerprint(binding.RequestFingerprint) || binding.ConfirmOption < 0 || binding.ConfirmOption >= optionCount {
		return agentAPIRoute{}, domainError(CodeToolValidation, "危险操作确认绑定无效", "confirmation 必须绑定当前 Project、具体目标 UUID、稳定 request_fingerprint、非负 revision 和有效的单选确认项。", nil)
	}
	return route, nil
}

func (service *Service) validateCodexDangerousConfirmationBinding(tc toolContext, binding dangerousConfirmationBinding, questions []UserInputQuestion) (agentAPIRoute, error) {
	route, ok := agentAPIRouteByIDFromRoutes(strings.TrimSpace(binding.Route), service.requestAPIRoutes())
	if !ok || route.Risk != RiskDangerous || !route.RequiresConfirmation {
		return agentAPIRoute{}, domainError(CodeToolValidation, "危险操作确认 Route 无效", "confirmation.route 必须是要求全局确认的已注册危险 Route。", nil)
	}
	if len(questions) != 1 || binding.QuestionID != questions[0].ID || binding.ProjectUUID != tc.ProjectUUID || !isUUIDv7(binding.ProjectUUID) || !isUUIDv7(binding.TargetUUID) || binding.ExpectedRevision < 0 || !validAgentRequestFingerprint(binding.RequestFingerprint) || binding.ConfirmOption <= 0 || binding.ConfirmOption >= len(questions[0].Options) {
		return agentAPIRoute{}, domainError(CodeToolValidation, "危险操作确认绑定无效", "confirmation 必须绑定当前 Project、唯一 question_id、具体目标 UUID、稳定 request_fingerprint、非负 revision 和非推荐的有效确认项；第一项保留为安全选项。", nil)
	}
	return route, nil
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
		"expected_revision": intArg(request.Body, "expected_revision"), "request_fingerprint": agentRequestFingerprint(request),
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
	expectedRevision := intArg(request.Body, "expected_revision")
	expectedFingerprint := agentRequestFingerprint(request)
	for _, row := range rows {
		var arguments struct {
			Confirmation *dangerousConfirmationBinding `json:"confirmation"`
		}
		if json.Unmarshal([]byte(row.ArgumentsJSON), &arguments) != nil || arguments.Confirmation == nil {
			continue
		}
		binding := arguments.Confirmation
		if binding.Route != request.Route.ID || binding.ProjectUUID != tc.ProjectUUID || binding.TargetUUID != request.TargetUUID || binding.ExpectedRevision != expectedRevision || binding.RequestFingerprint != expectedFingerprint {
			continue
		}
		confirmed, err := dangerousConfirmationSelected(row.SchemaVersion, row.RequestJSON, row.ResponseJSON, *binding)
		if err == nil && confirmed {
			consumed, err := dangerousConfirmationAlreadyConsumed(ctx, store, tc.Run.ID, row.ExecutionID, expectedFingerprint)
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

func dangerousConfirmationAlreadyConsumed(ctx context.Context, store *project.Store, runID, confirmationExecutionID int64, fingerprint string) (bool, error) {
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
		if storedAgentRequestFingerprint(arguments) == fingerprint {
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
