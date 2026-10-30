// Package textmcp implements the CLI-only authoring interface. Protocol only
// registers schemas and forwards calls; Service is hosted by the CLI instance.
package textmcp

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
)

const MaxBytes = 256 << 10

type Forward func(context.Context, string, json.RawMessage) httpapi.Envelope

func Protocol(forward Forward) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "lumi-text", Version: "1.0.0"}, &mcp.ServerOptions{Instructions: "Author the bound cwd project. Start with project_overview; if project_not_initialized call project_init. Complete project_setup before writing. No provider configuration or generation tools are needed: render_prompt returns catalog prompts for YOU to execute. Submit results with the returned target UUID and expected_revision; reread on conflict. For artwork call image_target before host generation; read_image supplies project references in the conversation. Immediately import_image using the actual file path from the host result (possibly output hint text); never assume savedPath, guess names or scan directories. Host image retention is unknown. Ratio mismatch is advisory; keep is the default, crop/pad are optional. Lists are paginated. Never edit project.sqlite directly."})
	for _, spec := range Specs() {
		s.AddTool(&mcp.Tool{Name: spec.Name, Description: spec.Description, InputSchema: spec.Schema}, func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result := forward(ctx, spec.Name, r.Params.Arguments)
			if spec.Name == "read_image" && result.Success {
				return imageToolResult(result)
			}
			return boundedResult(result)
		})
	}
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
