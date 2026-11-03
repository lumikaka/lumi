// Package textmcp implements the CLI-only authoring interface. Protocol only
// registers schemas and forwards calls; Service is hosted by the CLI instance.
package textmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
)

const MaxBytes = 256 << 10

type Forward func(context.Context, string, json.RawMessage) httpapi.Envelope

func Protocol(forward Forward) *mcp.Server {
	watches := newAppWatches(forward)
	s := mcp.NewServer(&mcp.Implementation{Name: "lumi-text", Version: "1.0.0"}, &mcp.ServerOptions{Instructions: "Author the bound cwd project. Start with project_overview; if project_not_initialized call project_init. Complete project_setup before writing. No provider configuration or generation tools are needed: render_prompt returns catalog prompts for YOU to execute. Submit results with the returned target UUID and expected_revision; reread on conflict. For initial creation with page artwork, prepare required setting images first, reusing current assets; a first-page limit applies only to page images, not setting boards. Do not invent quota limits or text-only settings unless the user explicitly requests them. For batch settings render_prompt target=setting_board, generate the returned generation_prompt, immediately setting_boards import, then read_image and render_prompt target=setting_breakdown before apply_setting_breakdown. For single artwork call image_target before host generation; read_image supplies project references in the conversation. For page images inspect image_target.setting_readiness, select required setting_uuids, prepare missing images, and read available references. A candidate inventory or empty selection is not readiness approval; only an explicit user instruction waives setting artwork. Then render the effective base fragment for the page role and actual reference-state fragment, then render_prompt chapter/section_image target=page with before_image_prompt/reference_usage_text. Use the complete generation_prompt and image_write_back; do not rewrite the storyboard into a separate image prompt or send the outer task metadata to imagegen. image_target alone is not an image prompt. Immediately import_image using the actual file path from the host result (possibly output hint text) and the same full prompt sent to generation; never assume savedPath, guess names or scan directories. Host image retention is unknown. Ratio mismatch is advisory; keep is the default, crop/pad are optional. Lists are paginated. Never edit project.sqlite directly."})
	for _, spec := range Specs() {
		// The connection descriptor is internal to the cwd-bound relay. Views
		// never receive loopback endpoints or scoped WS tickets.
		if spec.Name == "read_app_connection" {
			continue
		}
		published, err := modelVisibleSpec(spec)
		if err != nil {
			panic(fmt.Errorf("publish MCP tool %s: %w", spec.Name, err))
		}
		tool := &mcp.Tool{Name: spec.Name, Description: published.Description, InputSchema: published.Schema}
		if strings.HasPrefix(spec.Name, "review_") {
			tool.Meta = mcp.Meta{"ui": map[string]any{"resourceUri": ReviewURI}}
			tool.Annotations = appReadAnnotations()
		}
		if spec.Name == "read_app_image" || spec.Name == "watch_app_changes" || spec.Name == "read_app_resource" || spec.Name == "write_app_resource" || spec.Name == "read_app_review" {
			tool.Meta = mcp.Meta{"ui": map[string]any{"visibility": []string{"app"}}}
			tool.Annotations = appReadAnnotations()
			if spec.Name == "write_app_resource" {
				destructive := false
				openWorld := false
				tool.Annotations = &mcp.ToolAnnotations{DestructiveHint: &destructive, OpenWorldHint: &openWorld}
			}
		}
		s.AddTool(tool, func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// A local adapter may constrain the cwd identity. This never selects
			// a directory or grants access; ordinary Codex calls omit it.
			if expected, ok := r.Params.Meta[ExpectedProjectUUIDMeta]; ok {
				uuid, valid := expected.(string)
				if !valid || !publicProjectUUID.MatchString(uuid) {
					return boundedResult(Failure("invalid_arguments", "项目绑定无效", "重新读取当前任务项目。"))
				}
				ctx = WithExpectedProjectUUID(ctx, uuid)
			}
			if spec.Name == "watch_app_changes" {
				return watches.call(ctx, r)
			}
			args := r.Params.Arguments
			if upload, ok := r.Params.Meta["lumi/upload"]; ok {
				if spec.Name != "write_app_resource" {
					return boundedResult(Failure("app_resource_forbidden", "文件只允许由用户导入当前项目", ""))
				}
				var input map[string]any
				if json.Unmarshal(args, &input) != nil || input["resource"] != "asset-uploads" || input["method"] != "POST" {
					return boundedResult(Failure("app_resource_forbidden", "文件请求超出范围", ""))
				}
				encoded, _ := json.Marshal(upload)
				var attachment struct {
					Data string `json:"data_base64"`
				}
				if json.Unmarshal(encoded, &attachment) != nil || len(attachment.Data) > 8<<20 {
					return boundedResult(Failure("arguments_too_large", "文件传输超限", "单次支持 6 MiB 文件。"))
				}
				body, ok := input["body"].(map[string]any)
				if !ok {
					return boundedResult(Failure("invalid_arguments", "缺少文件元数据", ""))
				}
				body["upload_data"] = attachment.Data
				args, _ = json.Marshal(input)
			}
			result := forward(ctx, spec.Name, args)
			if spec.Name == "read_app_resource" || spec.Name == "write_app_resource" || spec.Name == "read_app_review" {
				return appResourceToolResult(result)
			}
			if strings.HasPrefix(spec.Name, "review_") {
				return reviewLaunchToolResult(result)
			}
			if spec.Name == "read_app_image" {
				return appImageToolResult(result)
			}
			if spec.Name == "read_image" && result.Success {
				return imageToolResult(result)
			}
			return boundedResult(result)
		})
	}
	registerReviewResource(s)
	return s
}

func boundedResult(result httpapi.Envelope, extra ...mcp.Content) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	tool := &mcp.CallToolResult{IsError: !result.Success, Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}, StructuredContent: result}
	tool.Content = append(tool.Content, extra...)
	wire, err := json.Marshal(tool)
	if err != nil {
		return nil, err
	}
	if len(wire) > MaxBytes-1024 {
		return boundedResult(Failure("result_too_large", "MCP 结果超过 256 KiB", "减小 limit 或用 context_chapter_uuids / context_setting_uuids 缩小范围；大文本可用 export_book 导出。"))
	}
	return tool, nil
}

func Failure(code, message, details string) httpapi.Envelope {
	return httpapi.Envelope{Error: &httpapi.ErrorBody{Code: code, Message: message, Details: details}}
}
