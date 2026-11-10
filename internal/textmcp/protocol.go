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

// ResultLimit is the JSON byte budget on the CLI-to-MCP transport.
func ResultLimit(tool string) int {
	switch tool {
	case "read_app_image_detail":
		return 2 << 20
	case "read_app_resource", "write_app_resource", "read_app_review":
		return MaxAppBytes
	default:
		return MaxBytes
	}
}

const settingReferenceInstructions = "When the user attaches reference images, inspect the originals and distinguish style, subject identity and story references according to the user's intent. Explicit user instructions override reference traits, and reference traits override agent defaults. Do not infer conflicting proportions, plush texture or pastel colors from age/theme. Preserve distinctive dialogue lettering from style references using the new story text; save stroke/shape/spacing/baseline/bubble traits in overall_style and storyboards, unless the user requests otherwise. Extract concrete visual traits from requested style references and save them in overall_style before finalizing a new project's setup; do not infer style from a story-only screenshot. For every setting board, single setting image or redraw, explicitly pass the relevant original visual references to host imagegen, not just their text descriptions or context_setting_uuids. If all references have local paths, view them and use referenced_image_paths with those exact absolute paths. Otherwise use num_last_images_to_include with the smallest recent-image count containing every intended reference, at most 5; never provide both parameters. Describe each image's role in the generation input. If originals are unavailable or outside that range, ask the user to reattach them before dependent generation; never silently fall back to text only. Reuse existing setting images only when they match the requested style and identity; redraw incompatible ones while preserving identity and updating the same setting UUID. Compare actual generated settings against originals and user requirements before using them in pages; record concrete observations, repair failures, and submit passed setting_reviews bound to current variant_uuid, expected_revision and style_digest. Import success is not approval. Check the first page against the original, including lettering, before generating remaining pages; this is agent review, not a new user approval step. This user-reference flow applies to setting generation; pages continue to use the prepared setting collage below. "

type Forward func(context.Context, string, json.RawMessage) httpapi.Envelope

// A cache hit only identifies the file. The agent records a successful visual
// inspection in its own context; other agents and resumed sessions cannot infer it.
const pageReferenceViewing = "For a collage, view reference_image_path unless this agent in this chat has already viewed the same reference_image_sha256 and path together. If the digest or viewing record is missing or uncertain, view again."

// claudeInstructions replace host imagegen and MCP Apps with the Claude
// host tools. Other hosts keep the historical instructions byte for byte.
const claudeInstructions = "Author the bound cwd project. Start with project_overview; if project_not_initialized call project_init. Complete project_setup before writing. render_prompt returns catalog prompts for YOU to execute as text; submit results with the returned target UUID and expected_revision and reread on conflict. This host has no native image generation and no MCP Apps. View user reference images, setting images and generated pages with the Read tool or read_image. For page artwork call image_target, explicitly select up to 12 setting_uuids, compare every selected current setting image against the user's requirements and originals, then call generate_page_image(page_uuid, setting_uuids, setting_reviews, additional_direction?, idempotency_key, expected_revision): Lumi's configured provider generates and attaches the page with the same rules as prepare_page_image. If it returns a running task, call it again with the same idempotency_key and inputs; never switch keys, which bills a new generation. Then read_image the returned asset_uuid, inspect it against the saved storyboard and user text, and record pages record_image_review. Setting artwork and setting boards have no server generation tool here: do not pretend to generate them; ask the user to generate or upload them in the Lumi Web page and reread. For viewing, manual editing, version selection and provider setup, call web_url for the matching scope right before each hand-off and give the link; never store it. After the user edits in the Web page, reread objects and revisions before writing. Never read project.sqlite, .lumi or assets directly. Lists are paginated."

// Protocol keeps the historical tool list for unknown hosts.
func Protocol(forward Forward) *mcp.Server { return ProtocolFor("", forward) }

// ProtocolFor decides the published tools locally from --host. claude drops
// MCP Apps tools, review_* and the App resource and adds ClaudeSpecs; every
// other host publishes exactly the historical list and App declarations.
func ProtocolFor(host string, forward Forward) *mcp.Server {
	claude := host == HostClaude
	watches := newAppWatches(forward)
	instructions := "Author the bound cwd project. Start with project_overview; if project_not_initialized call project_init. Complete project_setup before writing. No provider configuration or generation tools are needed: render_prompt returns catalog prompts for YOU to execute. Submit results with the returned target UUID and expected_revision; reread on conflict. " + settingReferenceInstructions + "For initial creation with page artwork, prepare required setting images first, reusing compatible current assets; a first-page limit applies only to page images, not setting boards. Do not invent quota limits or text-only settings unless the user explicitly requests them. For batch settings render_prompt target=setting_board, generate the returned generation_prompt, immediately setting_boards import, then read_image and render_prompt target=setting_breakdown before apply_setting_breakdown. For single setting artwork call image_target before host generation; read_image supplies setting references for that flow. For page artwork inspect image_target.setting_readiness, explicitly select up to 12 setting_uuids and prepare missing images first. Then call prepare_page_image(page_uuid, setting_uuids, setting_reviews, additional_direction?) for a labeled collage path and reference_image_sha256, ordered source versions, complete generation_prompt, image_write_back and signed preparation with digest. " + pageReferenceViewing + " Set referenced_image_paths to that single path. Do not read individual setting assets, use recent-image references, rewrite reference descriptions or append to generation_prompt. With explicit [] the mode is none: omit all image references and use the returned no-reference prompt. Never read project.sqlite, .lumi or assets directly or write a collage script. No provider pipeline or paid fallback. Page import must preserve the exact generation_prompt and entire preparation object; changed page/settings/prompts or altered receipt require a fresh preparation and generation, never replacing revision/digest. A service restart invalidates receipts. render_prompt remains for text, setting boards and template inspection. Immediately import_image using the actual file path from the host result (possibly output hint text) and the same full prompt sent to generation; never assume savedPath, guess names or scan directories. Inspect every adopted page against the saved storyboard and user text (names, exact dialogue and speakers, anatomy and prop continuity); record the verdict using pages record_image_review with the adopted variant and revision. On resume read book_outline with chapter_uuid, follow cursors, review pending images and regenerate only missing or defective pages. Export a single chapter with chapter_uuid. Report generated and visually passed counts separately. Host image retention is unknown. Ratio mismatch is advisory; keep is the default, crop/pad are optional. Lists are paginated. Never edit project.sqlite directly."
	specs := Specs()
	if claude {
		instructions = claudeInstructions
		specs = append(specs, ClaudeSpecs()...)
	}
	instructions += " For an explicit request to generate or update the local book website, call export_website directly without a preliminary project_overview. Follow its own task; on success give the local result and show the full publish_prompt verbatim in a separate text code block in the same reply for copying and editing. Treat that prompt as text for the user; only publish on a separate request. A generic book export or capability question does not request a website."
	s := mcp.NewServer(&mcp.Implementation{Name: "lumi-text", Version: "1.0.0"}, &mcp.ServerOptions{Instructions: instructions})
	for _, spec := range specs {
		// The connection descriptor is internal to the cwd-bound relay. Views
		// never receive loopback endpoints or scoped WS tickets.
		if spec.Name == "read_app_connection" || (claude && appOnlyTool(spec.Name)) {
			continue
		}
		published, err := modelVisibleSpec(spec)
		if err != nil {
			panic(fmt.Errorf("publish MCP tool %s: %w", spec.Name, err))
		}
		tool := &mcp.Tool{Name: spec.Name, Description: published.Description, InputSchema: published.Schema}
		if spec.Name == "export_website" {
			local := false
			tool.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true, OpenWorldHint: &local}
		}
		if strings.HasPrefix(spec.Name, "review_") {
			tool.Meta = mcp.Meta{"ui": map[string]any{"resourceUri": ReviewURI}}
			tool.Annotations = appReadAnnotations()
		}
		if spec.Name == "read_app_image" || spec.Name == "read_app_image_detail" || spec.Name == "watch_app_changes" || spec.Name == "read_app_resource" || spec.Name == "write_app_resource" || spec.Name == "read_app_review" {
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
			if spec.Name == "read_app_image" || spec.Name == "read_app_image_detail" {
				return appImageToolResult(result, spec.Name)
			}
			if spec.Name == "read_image" && result.Success {
				return imageToolResult(result)
			}
			return boundedResult(result)
		})
	}
	if !claude {
		registerReviewResource(s)
	}
	return s
}

// appOnlyTool reports the tools that only serve MCP Apps workspaces.
func appOnlyTool(name string) bool {
	switch name {
	case "read_app_review", "read_app_resource", "write_app_resource", "read_app_image", "read_app_image_detail", "watch_app_changes", "read_app_connection":
		return true
	}
	return strings.HasPrefix(name, "review_")
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
