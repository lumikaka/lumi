package agent

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// agentDocFiles is the reviewed, version-controlled source read by
// read_agent_doc. The public /api/v1/agent-docs/... path is mapped only after it
// matches the global embedded-doc registry; no caller-controlled filesystem
// path is ever opened.
//
//go:embed api-docs/*.md api-docs/guides/*.md
var agentDocFiles embed.FS

const (
	agentDocBasePath     = "/api/v1/agent-docs"
	agentDocOverviewPath = agentDocBasePath + "/overview.md"
	storyDocPath         = agentDocBasePath + "/story.md"
	projectDocPath       = agentDocBasePath + "/project.md"
	chapterDocPath       = agentDocBasePath + "/chapter.md"
	premiseDocPath       = agentDocBasePath + "/premise.md"
	premiseAssetDocPath  = agentDocBasePath + "/premise-asset.md"
	comicSectionDocPath  = agentDocBasePath + "/comic-section.md"
	comicDocPath         = agentDocBasePath + "/comic.md"
	comicSnapshotDocPath = agentDocBasePath + "/comic-snapshot.md"
	comicExportDocPath   = agentDocBasePath + "/comic-export.md"
	projectAssetDocPath  = agentDocBasePath + "/project-asset.md"
	storyboardDocPath    = agentDocBasePath + "/storyboard.md"
	generationDocPath    = agentDocBasePath + "/generation.md"
	taskDocPath          = agentDocBasePath + "/task.md"
	maxAgentDocBytes     = 96 << 10

	GuidePremiseAssetCreate   = "premise_asset_create"
	GuidePremiseAssetMaintain = "premise_asset_maintain"
	GuideStoryboardUpdate     = "storyboard_update"
)

type agentGuideDefinition struct {
	ID            string
	Description   string
	RequiredTools []string
	Prerequisites string
	Path          string
}

type agentAPIDocDefinition struct {
	Path        string
	Description string
}

func agentAPIDocDefinitions() []agentAPIDocDefinition {
	return []agentAPIDocDefinition{
		{Path: chapterDocPath, Description: "Chapter、正文版本、导入、回收站与恢复。"},
		{Path: comicExportDocPath, Description: "Comic Export readiness、创建与结果列表。"},
		{Path: comicSectionDocPath, Description: "单个 Comic Section、图片与 variant。"},
		{Path: comicSnapshotDocPath, Description: "Comic Snapshot 列表、详情与恢复。"},
		{Path: comicDocPath, Description: "Comic 状态、Section 集合与排序。"},
		{Path: generationDocPath, Description: "Story、Chapter、Premise 与 Comic 生成入口。"},
		{Path: agentDocOverviewPath, Description: "能力、Guide 与领域 API Contract 总索引。"},
		{Path: premiseAssetDocPath, Description: "Premise Asset、图片 variant 与生命周期。"},
		{Path: premiseDocPath, Description: "Premise、来源与 Setting Image。"},
		{Path: projectAssetDocPath, Description: "项目文件、上传、完整性与资产维护。"},
		{Path: projectDocPath, Description: "项目元数据、模型与 Prompt 设置、诊断及项目级运行资源。"},
		{Path: storyDocPath, Description: "Story Profile、版本、导入与投影。"},
		{Path: storyboardDocPath, Description: "Storyboard variant、全量更新与选择。"},
		{Path: taskDocPath, Description: "Story、Production、Workflow 与维护任务状态。"},
	}
}

func agentGuideDefinitions() []agentGuideDefinition {
	return []agentGuideDefinition{
		{
			ID: GuidePremiseAssetCreate, Description: "从文本、当前消息附件、已有项目图片参考或 ready upload 创建 Premise Asset。",
			RequiredTools: []string{"read_agent_doc", "request_api", "image_gen", "request_user_input"},
			Prerequisites: "当前项目与设定项描述；图片来源四选一，必要时具备当前消息附件、已有项目图片 UUID 或 ready upload UUID。",
			Path:          agentDocBasePath + "/guides/premise-asset-create.md",
		},
		{
			ID: GuidePremiseAssetMaintain, Description: "读取最新 revision，更新 Premise Asset 元数据或图片，派生新设定项，或显式软删除。",
			RequiredTools: []string{"read_agent_doc", "request_api", "image_gen", "request_user_input"},
			Prerequisites: "目标 Premise Asset UUID；写操作前的最新 revision；替换图片时需要绑定资产上下文。",
			Path:          agentDocBasePath + "/guides/premise-asset-maintain.md",
		},
		{
			ID: GuideStoryboardUpdate, Description: "读取完整 Storyboard，以完整 Markdown 全量替换，并处理 revision 冲突与窄化响应。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "目标 Chapter UUID、Section UUID，以及完整当前 Storyboard 和最新 revision。",
			Path:          agentDocBasePath + "/guides/storyboard-update.md",
		},
	}
}

func readAgentDoc(tc toolContext, args map[string]any) (map[string]any, error) {
	return readAgentDocWithRoutes(tc, args, agentAPIRoutes())
}

func (service *Service) readAgentDoc(tc toolContext, args map[string]any) (map[string]any, error) {
	return readAgentDocWithRoutes(tc, args, service.requestAPIRoutes())
}

func readAgentDocWithRoutes(tc toolContext, args map[string]any, routes []agentAPIRoute) (map[string]any, error) {
	if normalizedToolMode(tc.ToolMode) != ToolModeProjectAPI {
		return nil, domainError(CodeToolNotAllowed, "read_agent_doc 不适用于当前 Tool Mode", "当前 Run 没有启用 project_api_tools。", nil)
	}
	if _, ok := sceneDefinitionForThread(tc.Thread); !ok {
		return nil, domainError(CodeToolNotAllowed, "Scene 定义无效", "当前 thread 无法读取 Agent Docs。", nil)
	}
	path := stringArg(args, "path")
	if !validAgentDocPath(path) {
		return nil, domainError(CodeToolValidation, "Agent Doc path 无效", "只允许规范的 /api/v1/agent-docs/...md 注册路径；Query、Fragment、编码、反斜杠和路径穿越均被拒绝。", nil)
	}
	if !containsString(registeredAgentDocPathsForRoutes(routes), path) {
		return nil, domainError(CodeToolNotAllowed, "Agent Doc 未注册", "文档路径没有命中全局 Agent Docs Registry。", nil)
	}
	content, err := renderAgentDocWithRoutes(path, routes)
	if err != nil {
		return nil, err
	}
	if len(content) > maxAgentDocBytes {
		return nil, domainError(CodeResultTooLarge, "Agent Doc 过大", "单次文档结果超过限制。", nil)
	}
	return map[string]any{"path": path, "doc_ref": path, "content": content}, nil
}

func validAgentDocPath(path string) bool {
	if path == "" || !strings.HasPrefix(path, agentDocBasePath+"/") || !strings.HasSuffix(path, ".md") || strings.HasPrefix(path, "//") || strings.HasSuffix(path, "/") || strings.ContainsAny(path, "?#%\\") || strings.Contains(path, "//") || strings.Contains(path, "://") {
		return false
	}
	for _, segment := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func renderAgentDoc(path string) (string, error) {
	return renderAgentDocWithRoutes(path, agentAPIRoutes())
}

func renderAgentDocWithRoutes(path string, routes []agentAPIRoute) (string, error) {
	template, err := readAgentDocTemplate(path)
	if err != nil {
		return "", err
	}
	switch path {
	case agentDocOverviewPath:
		return renderAgentDocOverview(template, routes), nil
	}
	for _, guide := range agentGuideDefinitions() {
		if path == guide.Path {
			return strings.TrimSpace(template) + "\n", nil
		}
	}
	docRoutes := routesForAgentDocFromRoutes(path, routes)
	if len(docRoutes) == 0 {
		return "", domainError(CodeToolNotAllowed, "API Doc 没有可用 Route", "全局 Route Registry 没有与该文档对应的 API。", nil)
	}
	return replaceAgentDocTokens(template, map[string]string{
		"route_docs": renderAgentDomainDoc(docRoutes),
	}), nil
}

func readAgentDocTemplate(path string) (string, error) {
	relative := strings.TrimPrefix(path, agentDocBasePath+"/")
	if relative == path || relative == "" {
		return "", domainError(CodeToolValidation, "Agent Doc path 无效", "文档路径无法映射到注册文档。", nil)
	}
	content, err := agentDocFiles.ReadFile("api-docs/" + relative)
	if err != nil {
		return "", domainError(CodeToolNotAllowed, "Agent Doc 未注册", "注册路径没有对应的内嵌 Markdown 文档。", err)
	}
	return string(content), nil
}

func renderAgentDocOverview(template string, routes []agentAPIRoute) string {
	return replaceAgentDocTokens(template, map[string]string{
		"capability_index": renderAgentCapabilityIndex(agentGuideDefinitions()),
		"api_doc_index":    renderAgentAPIDocIndex(agentAPIDocDefinitions()),
	})
}

func replaceAgentDocTokens(template string, values map[string]string) string {
	result := template
	for key, value := range values {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return strings.TrimSpace(result) + "\n"
}

func routesForAgentDoc(path string) []agentAPIRoute {
	return routesForAgentDocFromRoutes(path, agentAPIRoutes())
}

func routesForAgentDocFromRoutes(path string, routes []agentAPIRoute) []agentAPIRoute {
	result := []agentAPIRoute{}
	for _, route := range routes {
		if route.DocPath == path {
			result = append(result, route)
		}
	}
	return result
}

func registeredAgentDocPaths() []string {
	return registeredAgentDocPathsForRoutes(agentAPIRoutes())
}

func registeredAgentDocPathsForRoutes(routes []agentAPIRoute) []string {
	paths := make([]string, 0, len(agentAPIDocDefinitions())+len(agentGuideDefinitions()))
	seen := make(map[string]bool, cap(paths))
	for _, doc := range agentAPIDocDefinitions() {
		paths = append(paths, doc.Path)
		seen[doc.Path] = true
	}
	for _, guide := range agentGuideDefinitions() {
		paths = append(paths, guide.Path)
		seen[guide.Path] = true
	}
	for _, route := range routes {
		if !seen[route.DocPath] {
			paths = append(paths, route.DocPath)
			seen[route.DocPath] = true
		}
	}
	sort.Strings(paths)
	return paths
}

func renderAgentCapabilityIndex(guides []agentGuideDefinition) string {
	rows := make([][]string, 0, len(guides))
	for _, guide := range guides {
		rows = append(rows, []string{
			codeCell(guide.ID), guide.Description, "`" + strings.Join(guide.RequiredTools, "`, `") + "`", guide.Prerequisites, codeCell(guide.Path),
		})
	}
	return renderAgentMarkdownTable([]string{"capability_id", "说明", "所需工具", "上下文/输入前提", "Guide 路径"}, rows)
}

func renderRecommendedGuideList(ids []string) string {
	byID := make(map[string]agentGuideDefinition, len(agentGuideDefinitions()))
	for _, guide := range agentGuideDefinitions() {
		byID[guide.ID] = guide
	}
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		if guide, ok := byID[id]; ok {
			lines = append(lines, "- `"+guide.ID+"`：`"+guide.Path+"`")
		}
	}
	return strings.Join(lines, "\n")
}

func renderAgentAPIDocIndex(docs []agentAPIDocDefinition) string {
	rows := make([][]string, 0, len(docs))
	for _, doc := range docs {
		rows = append(rows, []string{codeCell(doc.Path), doc.Description})
	}
	return renderAgentMarkdownTable([]string{"API Contract 路径", "领域与用途"}, rows)
}

func renderAgentDomainDoc(routes []agentAPIRoute) string {
	sections := []string{
		"## 方法与路径\n\n" + renderAgentMethodTable(routes),
		"## 路径参数\n\n" + renderAgentPathParameterTable(routes),
		"## Query 字段\n\n" + renderAgentQueryTable(routes),
		"## 请求体字段\n\n" + renderAgentRequestBodyTable(routes),
		"## 响应字段\n\n" + renderAgentResponseFieldTable(routes),
		"## 权限\n\n" + renderAgentPermissionTable(routes),
		"## 调用约束\n\n" + renderAgentCallConstraintTable(routes),
		"## 错误与调用示例\n\n" + renderAgentErrorsAndExamples(routes),
	}
	return "以下全局已注册操作使用 `request_api` 调用。\n\n" + strings.Join(sections, "\n\n")
}

func renderAgentQueryTable(routes []agentAPIRoute) string {
	rows := [][]string{}
	for _, route := range routes {
		operation := codeCell(route.Method + " " + route.PathTemplate)
		if route.QuerySchema == nil {
			rows = append(rows, []string{operation, "无", "-", "-", "不要传 query。"})
			continue
		}
		properties, _ := route.QuerySchema["properties"].(map[string]any)
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		required := agentAPIRequiredFields(route.QuerySchema)
		for _, key := range keys {
			schema, _ := properties[key].(map[string]any)
			rows = append(rows, []string{operation, codeCell(key), agentAPISchemaType(schema), agentAPIRequiredLabel(required[key]), agentAPISchemaDescription(schema)})
		}
	}
	return renderAgentMarkdownTable([]string{"操作", "字段", "类型", "必填", "说明"}, rows)
}

func renderAgentMethodTable(routes []agentAPIRoute) string {
	rows := make([][]string, 0, len(routes))
	for _, route := range routes {
		rows = append(rows, []string{codeCell(route.Method), codeCell(route.PathTemplate), route.Action + "（`" + route.ID + "`）。"})
	}
	return renderAgentMarkdownTable([]string{"方法", "路径", "说明"}, rows)
}

func renderAgentPathParameterTable(routes []agentAPIRoute) string {
	parameters := agentAPIPathParameters(routes)
	rows := make([][]string, 0, len(parameters))
	for _, name := range parameters {
		description := agentAPIPathParameterDescription(name)
		rows = append(rows, []string{codeCell(name), "string (UUIDv7)", "是", description})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"无", "-", "-", "该文档中的 Route 没有 Path 参数。"})
	}
	return renderAgentMarkdownTable([]string{"字段", "类型", "必填", "说明"}, rows)
}

func renderAgentRequestBodyTable(routes []agentAPIRoute) string {
	rows := [][]string{}
	for _, route := range routes {
		operation := codeCell(route.Method + " " + route.PathTemplate)
		if route.BodySchema == nil {
			rows = append(rows, []string{operation, "无", "-", "-", "不需要请求体。"})
			continue
		}
		properties, _ := route.BodySchema["properties"].(map[string]any)
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		required := agentAPIRequiredFields(route.BodySchema)
		for _, key := range keys {
			schema, _ := properties[key].(map[string]any)
			rows = append(rows, []string{
				operation, codeCell(key), agentAPISchemaType(schema), agentAPIRequiredLabel(required[key]), agentAPISchemaDescription(schema),
			})
		}
	}
	rows = append(rows, []string{"全部", codeCell("project_uuid"), "string", "禁止", "项目 UUID 只能出现在 URL Path 中，并且必须等于当前 Run 绑定项目。"})
	return renderAgentMarkdownTable([]string{"适用方法", "字段", "类型", "必填", "说明"}, rows)
}

func renderAgentResponseFieldTable(routes []agentAPIRoute) string {
	rows := [][]string{{"全部", codeCell("success"), "boolean", "是否成功。"}}
	for _, route := range routes {
		operation := codeCell(route.Method + " " + route.PathTemplate)
		projector, ok := agentAPIProjectorByKey(route.Projector)
		rows = append(rows, []string{operation, codeCell("data"), "object", route.Action + " 操作的紧凑响应。"})
		if !ok {
			continue
		}
		if projector.List {
			rows = append(rows,
				[]string{operation, codeCell("data.items"), "array", "有上限的资源列表。"},
				[]string{operation, codeCell("data.total"), "integer", "裁剪前的资源数量。"},
				[]string{operation, codeCell("data.truncated"), "boolean", "是否因列表上限而截断。"},
			)
			itemProjector, _ := agentAPIProjectorByKey(projector.ItemProjector)
			for _, field := range itemProjector.Fields {
				rows = append(rows, []string{operation, codeCell("data.items[]." + field.Name), field.Type, field.Description})
			}
			continue
		}
		for _, field := range projector.Fields {
			rows = append(rows, []string{operation, codeCell("data." + field.Name), field.Type, field.Description})
		}
	}
	rows = append(rows, []string{"全部", codeCell("error"), "object | null", "失败时返回公开错误信息；成功响应不包含错误内容。"})
	return renderAgentMarkdownTable([]string{"适用方法", "字段", "类型", "说明"}, rows)
}

func renderAgentPermissionTable(routes []agentAPIRoute) string {
	rows := make([][]string, 0, len(routes))
	for _, route := range routes {
		permission := "可编辑当前项目"
		if route.ReadOnly {
			permission = "可访问当前项目"
		}
		resourceBoundary := "当前项目范围；资源归属由领域服务校验"
		rows = append(rows, []string{codeCell(route.Method + " " + route.PathTemplate), permission, resourceBoundary})
	}
	return renderAgentMarkdownTable([]string{"方法与路径", "项目权限", "资源边界"}, rows)
}

func renderAgentCallConstraintTable(routes []agentAPIRoute) string {
	rows := make([][]string, 0, len(routes))
	for _, route := range routes {
		idempotency := "-"
		if route.Method == "POST" || route.ID == RoutePremiseAssetDelete {
			idempotency = "Tool Execution UUID"
		}
		rows = append(rows, []string{
			codeCell(route.ID), agentAPIBoolLabel(route.ExpectedRevision), agentAPIBoolLabel(route.Async), codeCell(route.Risk),
			agentAPIBoolLabel(route.RequiresConfirmation), idempotency,
		})
	}
	return renderAgentMarkdownTable([]string{"route_id", "expected_revision", "异步", "风险", "需要 request_user_input", "幂等规则"}, rows)
}

func renderAgentErrorsAndExamples(routes []agentAPIRoute) string {
	var builder strings.Builder
	builder.WriteString("常见错误：`tool_validation`、`tool_not_allowed`、`not_found`、revision/state conflict。\n\n")
	for _, route := range routes {
		encoded, _ := json.Marshal(minimalAgentAPIRequest(route))
		fmt.Fprintf(&builder, "- `%s`：`%s`\n", route.ID, encoded)
	}
	return strings.TrimSpace(builder.String())
}

func renderAgentMarkdownTable(headers []string, rows [][]string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "| %s |\n", strings.Join(headers, " | "))
	separators := make([]string, len(headers))
	for index := range separators {
		separators[index] = "---"
	}
	fmt.Fprintf(&builder, "| %s |\n", strings.Join(separators, " | "))
	for _, row := range rows {
		cells := make([]string, len(row))
		for index, cell := range row {
			cells[index] = agentMarkdownTableCell(cell)
		}
		fmt.Fprintf(&builder, "| %s |\n", strings.Join(cells, " | "))
	}
	return strings.TrimSpace(builder.String())
}

func agentMarkdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func codeCell(value string) string {
	return "`" + value + "`"
}

func agentAPIPathParameters(routes []agentAPIRoute) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, route := range routes {
		for _, segment := range strings.Split(route.PathTemplate, "/") {
			if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
			if name != "" && !seen[name] {
				result = append(result, name)
				seen[name] = true
			}
		}
	}
	return result
}

func agentAPIPathParameterDescription(name string) string {
	return map[string]string{
		"project_uuid":       "当前 Run 绑定项目的公开 UUIDv7。",
		"chapter_uuid":       "目标 Chapter 的公开 UUIDv7。",
		"premise_asset_uuid": "目标 Premise Asset 的公开 UUIDv7。",
		"section_uuid":       "目标 Comic Section 的公开 UUIDv7。",
		"source_uuid":        "目标 Premise Source 的公开 UUIDv7。",
		"setting_image_uuid": "目标 Premise Setting Image 的公开 UUIDv7。",
		"task_uuid":          "目标任务的公开 UUIDv7。",
	}[name]
}

func agentAPIRequiredFields(schema map[string]any) map[string]bool {
	result := map[string]bool{}
	values, _ := schema["required"].([]string)
	for _, value := range values {
		result[value] = true
	}
	return result
}

func agentAPISchemaType(schema map[string]any) string {
	typeName, _ := schema["type"].(string)
	if typeName != "array" {
		return typeName
	}
	items, _ := schema["items"].(map[string]any)
	itemType, _ := items["type"].(string)
	if itemType == "" {
		itemType = "object"
	}
	return "array<" + itemType + ">"
}

func agentAPISchemaDescription(schema map[string]any) string {
	description, _ := schema["description"].(string)
	if values, ok := schema["enum"].([]string); ok && len(values) > 0 {
		description += " 可选值：`" + strings.Join(values, "`、`") + "`。"
	}
	if description == "" {
		description = "已注册请求字段。"
	}
	return description
}

func agentAPIRequiredLabel(required bool) string {
	if required {
		return "是"
	}
	return "否"
}

func agentAPIBoolLabel(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

func minimalAgentAPIRequest(route agentAPIRoute) map[string]any {
	request := map[string]any{"url": route.PathTemplate, "method": route.Method, "response_filter": recommendedAgentAPIResponseFilter(route)}
	if route.BodySchema == nil {
		return request
	}
	body := map[string]any{}
	for key := range agentAPIRequiredFields(route.BodySchema) {
		body[key] = minimalAgentAPIFieldValue(key)
	}
	if route.ID == RoutePremiseAssetCreate {
		body["file_uuid"] = "00000000-0000-7000-8000-000000000000"
	}
	request["request_body"] = body
	return request
}

func minimalAgentAPIFieldValue(key string) any {
	switch key {
	case "expected_revision":
		return 3
	case "content_format":
		return "md"
	case "asset_type":
		return "character"
	case "story_md":
		return "# Story"
	case "content":
		return "# Chapter"
	case "content_md":
		return "# Storyboard"
	case "title":
		return "Asset title"
	case "prompt":
		return "Generation instructions"
	default:
		return "..."
	}
}
