package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type grantKey struct{}

func (s *Service) Protocol() http.Handler {
	return s.protocol("")
}

func (s *Service) protocol(resource string) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "lumi-project", Version: "1.0.0"}, &mcp.ServerOptions{Instructions: "Lumi must be running and the authorized project open. Read /api/v1/agent-docs/overview.md first. request_api writes require idempotency_key. Pending confirmations are approved in Lumi; retrieve durable results with get_call. Provider/model settings come from Lumi. Media is read with read_media.", Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}}})
	obj := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	str := map[string]any{"type": "string"}
	schemas := map[string]map[string]any{
		"request_api":    obj(map[string]any{"method": str, "url": str, "query": map[string]any{"type": "object"}, "request_body": map[string]any{"type": "object"}, "response_filter": str, "idempotency_key": str}, "method", "url", "response_filter"),
		"read_agent_doc": obj(map[string]any{"path": str}, "path"),
		"get_call":       obj(map[string]any{"call_uuid": str}, "call_uuid"),
		"read_media":     obj(map[string]any{"file_uuid": str}, "file_uuid"),
	}
	for name, schema := range schemas {
		server.AddTool(&mcp.Tool{Name: name, Description: map[string]string{"request_api": "Call an explicitly allowed API in the authorized project; returns durable call and business result.", "read_agent_doc": "Read shared Lumi API documentation. Start with /api/v1/agent-docs/overview.md.", "get_call": "Retrieve persisted operation/confirmation status and result after reconnect.", "read_media": "Read an authorized project image, at most 4 MiB, by public file UUID."}[name], InputSchema: schema}, func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			g, ok := ctx.Value(grantKey{}).(Grant)
			if !ok {
				return toolResult(failure("mcp_unauthorized", "缺少授权。")), nil
			}
			if !s.valid(ctx, g) {
				return toolResult(failure("mcp_unauthorized", "授权已撤销。")), nil
			}
			var args map[string]any
			if json.Unmarshal(r.Params.Arguments, &args) != nil || args == nil {
				return toolResult(failure("mcp_invalid_arguments", "arguments 必须是对象。")), nil
			}
			props := schema["properties"].(map[string]any)
			for k, v := range args {
				if _, ok := props[k]; !ok {
					return toolResult(failure("mcp_invalid_arguments", "未知参数。")), nil
				}
				if k == "query" || k == "request_body" {
					if _, ok := v.(map[string]any); !ok {
						return toolResult(failure("mcp_invalid_arguments", "参数必须是对象。")), nil
					}
				} else {
					if _, ok := v.(string); !ok {
						return toolResult(failure("mcp_invalid_arguments", "参数必须是字符串。")), nil
					}
				}
			}
			for _, k := range schema["required"].([]string) {
				if _, ok := args[k]; !ok {
					return toolResult(failure("mcp_invalid_arguments", "缺少参数："+k)), nil
				}
			}
			var activity activityRecord
			if name != "request_api" && s.projects.IsOpen(g.ProjectUUID) {
				labels := map[string]string{"read_agent_doc": "读取接口说明", "get_call": "读取调用结果", "read_media": "读取图片"}
				var err error
				activity, err = s.beginActivity(ctx, g, name, activitySpec{action: "read", label: labels[name]}, nil)
				if err != nil {
					return toolResult(publicError(err)), nil
				}
				defer s.activityRunning.Delete(activity.UUID)
			}
			var result map[string]any
			switch name {
			case "request_api":
				key, _ := args["idempotency_key"].(string)
				delete(args, "idempotency_key")
				result = s.Call(ctx, g, args, key)
			case "read_agent_doc":
				value, err := s.api.ReadDoc(args)
				if value != nil {
					value["project_uuid"] = g.ProjectUUID
					value["permission"] = g.Permission
				}
				result = success(value)
				if err != nil {
					result = publicError(err)
				}
			case "get_call":
				result = s.GetCall(ctx, g, args["call_uuid"].(string))
			case "read_media":
				media := s.readMedia(ctx, g, args["file_uuid"].(string))
				if activity.UUID != "" {
					status, message := "succeeded", ""
					if media.IsError {
						status, message = "failed", "图片读取失败"
					}
					s.finishActivity(g.ProjectUUID, activity, status, message)
				}
				return media, nil
			}
			if activity.UUID != "" {
				status, message := "succeeded", ""
				if result["success"] == false {
					status, message = "failed", activityError(result)
				}
				s.finishActivity(g.ProjectUUID, activity, status, message)
			}
			return toolResult(result), nil
		})
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This protocol endpoint uses its own bearer authentication on the shared listener.
		// Browser-origin requests are not supported, even with a valid capability.
		if r.Header.Get("Origin") != "" {
			http.Error(w, "origin forbidden", http.StatusForbidden)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		g, err := s.Authenticate(r.Context(), token)
		if err != nil || (g.OAuthResource != "" && g.OAuthResource != resource) {
			if resource != "" {
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+strings.TrimSuffix(resource, "/mcp")+`/.well-known/oauth-protected-resource/mcp", scope="mcp:project"`)
			}
			http.Error(w, "mcp_unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 320<<10)
		handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), grantKey{}, g)))
	})
}
func toolResult(envelope map[string]any) *mcp.CallToolResult {
	b, _ := json.Marshal(envelope)
	failed := envelope["success"] == false
	if d, ok := envelope["data"].(map[string]any); ok {
		if nested, ok := d["result"].(map[string]any); ok && nested["success"] == false {
			failed = true
		}
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}, StructuredContent: envelope, IsError: failed}
}
