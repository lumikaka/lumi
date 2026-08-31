package agent

import (
	"context"
	"encoding/json"
	"strings"

	"lumi/internal/llm"
	"lumi/internal/project"
)

func executeRequestAPITool(ctx context.Context, service *Service, store *project.Store, tc toolContext, execution toolExecutionRecord, args map[string]any) (any, error) {
	output, err := executeRequestAPIToolWithUIRef(ctx, service, store, tc, execution, args)
	return output.Data, err
}

func executeRequestAPIToolWithUIRef(ctx context.Context, service *Service, store *project.Store, tc toolContext, execution toolExecutionRecord, args map[string]any) (requestAPIToolOutput, error) {
	request, err := service.parseAgentAPIRequest(tc, args)
	if err != nil {
		return requestAPIToolOutput{}, err
	}
	if store != nil && store.SetupStatus() == project.SetupStatusDraft && request.Method != "GET" && request.Route.ID != RouteProjectSetupUpdate && request.Route.ID != RouteProjectSetupReferenceUpdate && request.Route.ID != RouteProjectSetupFinalize {
		return requestAPIToolOutput{}, domainError(project.CodeProjectSetupIncomplete, "项目设置尚未定稿", "draft 阶段只允许读取项目事实和修改或定稿 project setup。", nil)
	}
	if store != nil && store.SetupStatus() == project.SetupStatusReady && isBootstrapToolContext(tc) && request.Method != "GET" {
		switch request.Route.ID {
		case RouteProjectSetupFinalize:
			// A confirmed finalization replay may be recovered after the project
			// has already transitioned to ready.
		case RouteYoloWorkflowCreate:
			authorized, policyErr := bootstrapYoloAuthorized(ctx, store, tc)
			if policyErr != nil {
				return requestAPIToolOutput{}, policyErr
			}
			if !authorized {
				return requestAPIToolOutput{}, bootstrapYoloNotAuthorizedError()
			}
		default:
			return requestAPIToolOutput{}, bootstrapProductionRequiresYoloError()
		}
	}
	if err := authorizeDangerousAgentAPIRequest(ctx, store, tc, request); err != nil {
		return requestAPIToolOutput{}, err
	}
	value, err := executeAgentAPIRoute(ctx, service, store, tc, execution, request)
	if err != nil {
		return requestAPIToolOutput{}, err
	}
	value, err = compactAgentRouteValue(request.Route, value)
	if err != nil {
		return requestAPIToolOutput{}, err
	}
	if err := validateAgentAPIResponse(value); err != nil {
		return requestAPIToolOutput{}, err
	}
	output := requestAPIToolOutput{Data: value, UIRef: projectReferenceForAgentAPI(request, value)}
	if request.ResponseFilter != "" {
		encoded, err := json.Marshal(value)
		if err != nil {
			return requestAPIToolOutput{}, err
		}
		var publicValue any
		if err := json.Unmarshal(encoded, &publicValue); err != nil {
			return requestAPIToolOutput{}, err
		}
		output.Data, err = runResponseFilter(map[string]any{"success": true, "data": publicValue}, request.ResponseFilter)
		if err != nil {
			return requestAPIToolOutput{}, err
		}
	}
	return output, nil
}

func (service *Service) agentToolLogMetadata(tc toolContext, calls []llm.ToolCall) []map[string]any {
	metadata := []map[string]any{}
	for _, call := range calls {
		var args map[string]any
		if json.Unmarshal([]byte(call.Arguments), &args) != nil {
			continue
		}
		switch call.Name {
		case "request_api":
			request, err := service.parseAgentAPIRequest(tc, args)
			if err != nil {
				continue
			}
			metadata = append(metadata, map[string]any{
				"provider_call_id": call.ID, "route_id": request.Route.ID, "action": request.Route.Action,
				"method": request.Method, "path": request.Path, "target_uuid": request.TargetUUID,
			})
		case "read_agent_doc":
			path := strings.TrimSpace(stringArg(args, "path"))
			if validAgentDocPath(path) {
				metadata = append(metadata, map[string]any{
					"provider_call_id": call.ID, "route_id": "agent_doc.read", "action": "读取 Agent 文档",
					"method": "READ", "path": path, "target_uuid": tc.Thread.UUID,
				})
			}
		}
	}
	return metadata
}

func attachAgentToolLogMetadata(payload json.RawMessage, metadata []map[string]any) json.RawMessage {
	if len(payload) == 0 || len(metadata) == 0 {
		return payload
	}
	var snapshot map[string]any
	if json.Unmarshal(payload, &snapshot) != nil {
		return payload
	}
	snapshot["agent_tool_routes"] = metadata
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return payload
	}
	return encoded
}

func (service *Service) hydrateToolExecutionMetadata(tc toolContext, execution *toolExecutionRecord, args map[string]any) {
	if execution == nil {
		return
	}
	if execution.Action == "" {
		execution.Action = execution.ToolName
	}
	switch execution.ToolName {
	case "request_api":
		request, err := service.parseAgentAPIRequest(tc, args)
		if err != nil {
			return
		}
		execution.RouteID, execution.Action = request.Route.ID, request.Route.Action
		execution.Method, execution.Path = request.Method, request.Path
		if execution.TargetUUID == "" {
			execution.TargetUUID = request.TargetUUID
		}
	case "read_agent_doc":
		execution.RouteID, execution.Action, execution.Method = "agent_doc.read", "读取 Agent 文档", "READ"
		execution.Path = stringArg(args, "path")
		if execution.TargetUUID == "" {
			execution.TargetUUID = tc.Thread.UUID
		}
	}
}
