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
//go:embed docs/overview.md docs/api/*.md docs/guides/*.md
var agentDocFiles embed.FS

const (
	agentDocBasePath     = "/api/v1/agent-docs"
	agentDocAPIBasePath  = agentDocBasePath + "/api"
	agentDocOverviewPath = agentDocBasePath + "/overview.md"
	storyDocPath         = agentDocAPIBasePath + "/story.md"
	projectDocPath       = agentDocAPIBasePath + "/project.md"
	projectSetupDocPath  = agentDocAPIBasePath + "/project-setup.md"
	workflowDocPath      = agentDocAPIBasePath + "/workflow.md"
	chapterDocPath       = agentDocAPIBasePath + "/chapter.md"
	premiseDocPath       = agentDocAPIBasePath + "/premise.md"
	premiseAssetDocPath  = agentDocAPIBasePath + "/premise-asset.md"
	comicSectionDocPath  = agentDocAPIBasePath + "/comic-section.md"
	comicDocPath         = agentDocAPIBasePath + "/comic.md"
	comicSnapshotDocPath = agentDocAPIBasePath + "/comic-snapshot.md"
	comicExportDocPath   = agentDocAPIBasePath + "/comic-export.md"
	projectAssetDocPath  = agentDocAPIBasePath + "/project-asset.md"
	storyboardDocPath    = agentDocAPIBasePath + "/storyboard.md"
	generationDocPath    = agentDocAPIBasePath + "/generation.md"
	taskDocPath          = agentDocAPIBasePath + "/task.md"
	maxAgentDocBytes     = 96 << 10
)

type agentGuideDefinition struct {
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
		{Path: comicSectionDocPath, Description: "单个 Comic Section 的页面角色与详情。"},
		{Path: comicSnapshotDocPath, Description: "Comic Snapshot 列表、详情与恢复。"},
		{Path: comicDocPath, Description: "Comic 状态、封面/正文页/封底集合与排序。"},
		{Path: generationDocPath, Description: "Story、Chapter、Premise 与 Comic 生成入口。"},
		{Path: premiseAssetDocPath, Description: "Premise Asset、图片 variant 与生命周期。"},
		{Path: premiseDocPath, Description: "Premise、来源与 Setting Image。"},
		{Path: projectAssetDocPath, Description: "项目资产的读取、元数据与生命周期。"},
		{Path: projectDocPath, Description: "项目元数据与生成语言。"},
		{Path: projectSetupDocPath, Description: "对话式项目初始化草稿、字段来源与定稿。"},
		{Path: storyDocPath, Description: "Story Profile、版本、导入与投影。"},
		{Path: storyboardDocPath, Description: "Storyboard variant、全量更新与选择。"},
		{Path: taskDocPath, Description: "Story 与 Production 任务的状态、事件、取消和重试。"},
		{Path: workflowDocPath, Description: "YOLO Workflow 的受控启动、异步展示与停止边界。"},
	}
}

func agentGuideDefinitions() []agentGuideDefinition {
	return []agentGuideDefinition{
		{
			Description:   "从首页一句话初始化 draft 项目，补齐 Setup Draft 与 YOLO Brief，并在用户确认后定稿和启动受控 YOLO。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "当前项目 setup_status 为 draft；保留用户原始输入并使用最新设置 revision。",
			Path:          agentDocBasePath + "/guides/初始化新项目.md",
		},
		{
			Description:   "编辑、生成、重建、导入或查看故事总纲版本。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "当前项目；写入时需要最新 Story Profile revision。",
			Path:          agentDocBasePath + "/guides/管理故事总纲.md",
		},
		{
			Description:   "手动创建、批量规划或生成章节正文。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "章节标题或生成目标；必要时具备章节编号、正文或章节数量。",
			Path:          agentDocBasePath + "/guides/创建章节.md",
		},
		{
			Description:   "修改章节标题或完整正文，并查看正文版本。",
			RequiredTools: []string{"read_agent_doc", "request_api"},
			Prerequisites: "目标 Chapter UUID 与最新 revision。",
			Path:          agentDocBasePath + "/guides/修改章节.md",
		},
		{
			Description:   "移入、查看、恢复、永久删除或清空章节回收站。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "目标 Chapter UUID；删除或恢复时需要最新 revision。",
			Path:          agentDocBasePath + "/guides/管理章节回收站.md",
		},
		{
			Description:   "维护画风并生成、导入、选择或拆解项目设定图。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "设定描述；更新画风时需要最新 Premise revision。",
			Path:          agentDocBasePath + "/guides/生成项目设定.md",
		},
		{
			Description:   "从生成图片或 ready upload 创建设定资产。",
			RequiredTools: []string{"read_agent_doc", "request_api", "image_gen", "request_user_input"},
			Prerequisites: "设定项类型、标题与图片来源。",
			Path:          agentDocBasePath + "/guides/创建设定资产.md",
		},
		{
			Description:   "更新设定资产元数据、图片版本或当前图片。",
			RequiredTools: []string{"read_agent_doc", "request_api", "image_gen", "request_user_input"},
			Prerequisites: "目标 Premise Asset UUID 与最新 revision。",
			Path:          agentDocBasePath + "/guides/维护设定资产.md",
		},
		{
			Description:   "移入、查看、恢复、永久删除或清空设定资产回收站。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "目标 Premise Asset UUID；删除或恢复时需要最新 revision。",
			Path:          agentDocBasePath + "/guides/管理设定资产回收站.md",
		},
		{
			Description:   "为章节生成漫画分镜并跟踪任务。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "目标 Chapter UUID 与分镜生成要求。",
			Path:          agentDocBasePath + "/guides/生成漫画分镜.md",
		},
		{
			Description:   "创建、修改、排序或删除封面、正文页、封底或画面段落。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "目标 Chapter UUID；修改或删除时需要 Section UUID 与最新 revision。",
			Path:          agentDocBasePath + "/guides/管理漫画段落.md",
		},
		{
			Description:   "编辑完整分镜文本或选择历史分镜版本。",
			RequiredTools: []string{"read_agent_doc", "request_api"},
			Prerequisites: "目标 Chapter 与 Section UUID，以及最新 Section revision。",
			Path:          agentDocBasePath + "/guides/编辑与选择漫画分镜.md",
		},
		{
			Description:   "生成、导入或选择漫画图片版本。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "目标 Chapter 与 Section UUID；导入时需要 ready upload。",
			Path:          agentDocBasePath + "/guides/生成导入与选择漫画图片.md",
		},
		{
			Description:   "查看漫画快照详情并恢复章节漫画状态。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "目标 Chapter UUID 与待恢复 Snapshot UUID。",
			Path:          agentDocBasePath + "/guides/恢复漫画快照.md",
		},
		{
			Description:   "检查导出条件并创建、跟踪漫画导出任务。",
			RequiredTools: []string{"read_agent_doc", "request_api", "request_user_input"},
			Prerequisites: "导出范围与格式；章节导出时需要 Chapter UUID。",
			Path:          agentDocBasePath + "/guides/导出漫画.md",
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
	result := map[string]any{"path": path, "doc_ref": path, "content": content}
	// read_agent_doc must either return the complete reviewed Contract or fail.
	// A compacted preview is not a usable API contract, so validate the exact
	// JSON envelope that executeTool will hand to compactToolResult.
	encoded, err := json.Marshal(map[string]any{"success": true, "data": result})
	if err != nil || len(encoded) > MaxToolResult {
		return nil, domainError(CodeResultTooLarge, "Agent Doc 过大", "完整 Agent Doc 的 JSON 工具结果超过 64 KiB 限制。", err)
	}
	return result, nil
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
	if isStaticAgentDocPath(path) {
		// Guides and API Contracts are reviewed Markdown. Route metadata controls
		// validation and execution, but never generates instructions for the Agent.
		return strings.TrimSpace(template) + "\n", nil
	}
	return "", domainError(CodeToolNotAllowed, "Agent Doc 未注册", "文档没有命中静态 Agent Docs Registry。", nil)
}

func isStaticAgentDocPath(path string) bool {
	for _, doc := range agentAPIDocDefinitions() {
		if path == doc.Path {
			return true
		}
	}
	for _, guide := range agentGuideDefinitions() {
		if path == guide.Path {
			return true
		}
	}
	return false
}

func readAgentDocTemplate(path string) (string, error) {
	relative := strings.TrimPrefix(path, agentDocBasePath+"/")
	if relative == path || relative == "" {
		return "", domainError(CodeToolValidation, "Agent Doc path 无效", "文档路径无法映射到注册文档。", nil)
	}
	content, err := agentDocFiles.ReadFile("docs/" + relative)
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
	paths := make([]string, 0, 1+len(agentAPIDocDefinitions())+len(agentGuideDefinitions()))
	paths = append(paths, agentDocOverviewPath)
	seen := map[string]bool{agentDocOverviewPath: true}
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
			codeCell(guide.Path), guide.Description, "`" + strings.Join(guide.RequiredTools, "`, `") + "`", guide.Prerequisites,
		})
	}
	return renderAgentMarkdownTable([]string{"Guide", "说明", "所需工具", "上下文/输入前提"}, rows)
}

func renderAgentAPIDocIndex(docs []agentAPIDocDefinition) string {
	rows := make([][]string, 0, len(docs))
	for _, doc := range docs {
		rows = append(rows, []string{codeCell(doc.Path), doc.Description})
	}
	return renderAgentMarkdownTable([]string{"API Contract 路径", "领域与用途"}, rows)
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
