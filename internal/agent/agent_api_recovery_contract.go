package agent

import (
	"encoding/json"
	"strings"
)

const agentAPIRecoveryContractMarker = "RECOVERY_CONTRACT_JSON:"

type agentAPIRecoveryContract struct {
	RouteID      string                  `json:"route_id"`
	Method       string                  `json:"method"`
	PathTemplate string                  `json:"path_template"`
	DocPath      string                  `json:"doc_path"`
	Input        agentAPIRecoveryInput   `json:"input"`
	Output       agentAPIRecoveryOutput  `json:"output"`
	Policy       agentAPIRecoveryPolicy  `json:"policy"`
	Violation    toolValidationViolation `json:"violation"`
}

type agentAPIRecoveryInput struct {
	QuerySchema       map[string]any `json:"query_schema"`
	RequestBodySchema map[string]any `json:"request_body_schema"`
}

type agentAPIRecoveryOutput struct {
	DataShape                 string   `json:"data_shape"`
	AllowedFields             []string `json:"allowed_fields"`
	ItemFields                []string `json:"item_fields"`
	RecommendedResponseFilter string   `json:"recommended_response_filter"`
}

type agentAPIRecoveryPolicy struct {
	Risk                 string `json:"risk"`
	ReadOnly             bool   `json:"read_only"`
	Async                bool   `json:"async"`
	ExpectedRevision     bool   `json:"expected_revision"`
	RevisionSource       string `json:"revision_source"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
}

type agentAPIRecoveryFallback struct {
	RouteID                   string                  `json:"route_id"`
	Method                    string                  `json:"method"`
	PathTemplate              string                  `json:"path_template"`
	DocPath                   string                  `json:"doc_path"`
	RecommendedResponseFilter string                  `json:"recommended_response_filter"`
	Violation                 toolValidationViolation `json:"violation"`
}

type rejectedAgentAPICallRecovery struct {
	Contract   agentAPIRecoveryContract
	Route      agentAPIRoute
	Path       string
	TargetUUID string
}

func (service *Service) buildRejectedAgentAPICallRecovery(tc toolContext, name, raw string, cause error) (*rejectedAgentAPICallRecovery, bool) {
	if name != "request_api" || errorCode(cause) != CodeToolValidation || normalizedToolMode(tc.ToolMode) != ToolModeProjectAPI {
		return nil, false
	}
	var args map[string]any
	if !json.Valid([]byte(raw)) || json.Unmarshal([]byte(raw), &args) != nil || args == nil {
		return nil, false
	}
	method, path := stringArg(args, "method"), stringArg(args, "url")
	for _, candidate := range service.requestAPIRoutes() {
		if candidate.Method != method {
			continue
		}
		params, matched := matchAgentAPIPath(candidate.PathTemplate, path)
		if !matched || params["project_uuid"] != tc.ProjectUUID {
			continue
		}
		violation, ok := toolValidationViolationFromError(cause)
		if !ok {
			violation = toolValidationViolation{Rule: "validation"}
		}
		request := agentAPIRequest{Route: candidate, Method: method, Path: path, Params: params}
		return &rejectedAgentAPICallRecovery{
			Contract: agentAPIRecoveryContract{
				RouteID: candidate.ID, Method: candidate.Method, PathTemplate: candidate.PathTemplate, DocPath: candidate.DocPath,
				Input:  agentAPIRecoveryInput{QuerySchema: candidate.QuerySchema, RequestBodySchema: candidate.BodySchema},
				Output: recoveryOutputContract(candidate),
				Policy: agentAPIRecoveryPolicy{
					Risk: candidate.Risk, ReadOnly: candidate.ReadOnly, Async: candidate.Async,
					ExpectedRevision: candidate.ExpectedRevision, RevisionSource: candidate.RevisionSource, RequiresConfirmation: candidate.RequiresConfirmation,
				},
				Violation: violation,
			},
			Route: candidate, Path: path, TargetUUID: routeTargetUUID(request, tc.Thread),
		}, true
	}
	return nil, false
}

func recoveryOutputContract(route agentAPIRoute) agentAPIRecoveryOutput {
	result := agentAPIRecoveryOutput{
		DataShape: "unknown", AllowedFields: []string{}, ItemFields: []string{},
		RecommendedResponseFilter: recommendedAgentAPIResponseFilter(route),
	}
	projector, ok := agentAPIProjectorByKey(route.Projector)
	if !ok {
		return result
	}
	if !projector.List {
		if projector.NullData {
			result.DataShape = "null"
			return result
		}
		result.DataShape = "object"
		result.AllowedFields = agentAPIProjectorFieldNames(projector)
		return result
	}
	result.DataShape = "list"
	result.AllowedFields = []string{"items", "pagination", "cursor_pagination", "filter_groups"}
	if itemProjector, found := agentAPIProjectorByKey(projector.ItemProjector); found {
		result.ItemFields = agentAPIProjectorFieldNames(itemProjector)
	}
	return result
}

func toolErrorResultWithRecovery(cause error, recovery *rejectedAgentAPICallRecovery) (json.RawMessage, bool) {
	base := toolErrorResult(cause)
	if recovery == nil {
		return base, false
	}
	if result, ok := appendRecoveryContract(base, recovery.Contract); ok && len(result) <= MaxToolResult {
		return result, true
	}
	fallback := agentAPIRecoveryFallback{
		RouteID: recovery.Contract.RouteID, Method: recovery.Contract.Method,
		PathTemplate: recovery.Contract.PathTemplate, DocPath: recovery.Contract.DocPath,
		RecommendedResponseFilter: recovery.Contract.Output.RecommendedResponseFilter,
		Violation:                 recovery.Contract.Violation,
	}
	if result, ok := appendRecoveryContract(base, fallback); ok && len(result) <= MaxToolResult {
		return result, true
	}
	return base, false
}

func appendRecoveryContract(base json.RawMessage, contract any) (json.RawMessage, bool) {
	var envelope map[string]any
	if json.Unmarshal(base, &envelope) != nil {
		return nil, false
	}
	errorValue, ok := envelope["error"].(map[string]any)
	if !ok {
		return nil, false
	}
	contractJSON, err := json.Marshal(contract)
	if err != nil {
		return nil, false
	}
	details, _ := errorValue["details"].(string)
	details = strings.TrimSpace(details)
	if details != "" {
		details += "\n"
	}
	errorValue["details"] = details + agentAPIRecoveryContractMarker + string(contractJSON)
	result, err := json.Marshal(envelope)
	return result, err == nil
}
