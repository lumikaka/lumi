package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"
)

const (
	RouteStoryProfileGet                = "story_profile.get"
	RouteStoryProfileUpdate             = "story_profile.update"
	RouteChapterList                    = "chapter.list"
	RouteChapterGet                     = "chapter.get"
	RouteChapterStoryUpdate             = "chapter_story.update"
	RoutePremiseGet                     = "premise.get"
	RoutePremiseAssetList               = "premise_asset.list"
	RoutePremiseAssetGet                = "premise_asset.get"
	RoutePremiseAssetCreate             = "premise_asset.create"
	RoutePremiseAssetUpdate             = "premise_asset.update"
	RoutePremiseAssetDelete             = "premise_asset.soft_delete"
	RouteComicSectionGet                = "comic_section.get"
	RouteStoryboardUpdate               = "storyboard.update"
	RouteChapterGenerationCreate        = "generation.chapter.create"
	RoutePremiseSettingGenerationCreate = "generation.premise_setting.create"
	RoutePremiseBreakdownCreate         = "generation.premise_breakdown.create"
	RouteComicImageGenerationCreate     = "generation.comic_image.create"
	RouteStoryTaskGet                   = "task.story.get"
	RouteProductionTaskGet              = "task.production.get"

	RiskLow       = "low"
	RiskWrite     = "write"
	RiskDangerous = "dangerous"
)

type agentAPIRoute struct {
	ID, Action, Method, PathTemplate, Handler, Projector, DocPath string
	RecommendedResponseFilter                                     string
	QuerySchema                                                   map[string]any
	BodySchema                                                    map[string]any
	ReadOnly, Async, ExpectedRevision                             bool
	RequiresConfirmation                                          bool
	Passthrough                                                   bool
	ServerRoute                                                   bool
	Risk                                                          string
}

type agentAPIRequest struct {
	Route          agentAPIRoute
	Method, Path   string
	Query          map[string]any
	Body           map[string]any
	HasBody        bool
	UseDispatcher  bool
	ResponseFilter string
	Params         map[string]string
	TargetUUID     string
}

// agentAPIResponseField defines the allowlisted compact response surface used
// by request_api. Agent-facing API instructions live in reviewed Markdown.
type agentAPIResponseField struct {
	Name, Type, Description string
}

type agentAPIProjector struct {
	Key               string
	Fields            []agentAPIResponseField
	RecommendedFields []string
	List              bool
	ItemProjector     string
}

func rawAgentAPIProjectors() []agentAPIProjector {
	base := []agentAPIProjector{
		{Key: "story_profile", Fields: []agentAPIResponseField{
			{Name: "uuid", Type: "string", Description: "Story Profile 公开 UUIDv7。"},
			{Name: "revision", Type: "integer", Description: "当前乐观并发版本。"},
			{Name: "story_md", Type: "string", Description: "完整 STORY.md 内容。"},
			{Name: "projection_state", Type: "string", Description: "Story Profile 投影状态。"},
		}, RecommendedFields: []string{"uuid", "revision", "projection_state"}},
		{Key: "chapter", Fields: []agentAPIResponseField{
			{Name: "uuid", Type: "string", Description: "Chapter 公开 UUIDv7。"},
			{Name: "chapter_code", Type: "string", Description: "章节业务编号。"},
			{Name: "title", Type: "string", Description: "章节标题。"},
			{Name: "revision", Type: "integer", Description: "当前乐观并发版本。"},
			{Name: "trashed_at", Type: "string | null", Description: "移入回收站的时间；active Chapter 为空。"},
			{Name: "current_story", Type: "object | null", Description: "当前章节正文版本。"},
		}, RecommendedFields: []string{"uuid", "chapter_code", "title", "revision"}},
		{Key: "chapter_list", List: true, ItemProjector: "chapter"},
		{Key: "premise", Fields: []agentAPIResponseField{
			{Name: "uuid", Type: "string", Description: "Premise 公开 UUIDv7。"},
			{Name: "default_style", Type: "string", Description: "当前项目整体画风。"},
			{Name: "current_source", Type: "object | null", Description: "当前 Premise 来源。"},
			{Name: "current_setting_image", Type: "object | null", Description: "当前设定图。"},
			{Name: "revision", Type: "integer", Description: "当前乐观并发版本。"},
		}, RecommendedFields: []string{"uuid", "default_style", "revision"}},
		{Key: "premise_asset", Fields: []agentAPIResponseField{
			{Name: "uuid", Type: "string", Description: "Premise Asset 公开 UUIDv7。"},
			{Name: "asset_type", Type: "string", Description: "设定项类型。"},
			{Name: "title", Type: "string", Description: "设定项标题。"},
			{Name: "summary", Type: "string", Description: "设定项简介。"},
			{Name: "tags", Type: "array<string>", Description: "设定项标签。"},
			{Name: "current_variant", Type: "object | null", Description: "当前图片候选版本。"},
			{Name: "revision", Type: "integer", Description: "当前乐观并发版本。"},
			{Name: "deleted_at", Type: "string | null", Description: "软删除时间；active 资源为空。"},
		}, RecommendedFields: []string{"uuid", "asset_type", "title", "summary", "tags", "revision"}},
		{Key: "premise_asset_list", List: true, ItemProjector: "premise_asset"},
		{Key: "comic_section", Fields: []agentAPIResponseField{
			{Name: "uuid", Type: "string", Description: "Comic Section 公开 UUIDv7。"},
			{Name: "chapter_uuid", Type: "string", Description: "所属 Chapter 公开 UUIDv7。"},
			{Name: "section_no", Type: "integer", Description: "Section 在章节内的序号。"},
			{Name: "title", Type: "string", Description: "Section 标题。"},
			{Name: "description_md", Type: "string", Description: "Section 描述 Markdown。"},
			{Name: "current_storyboard", Type: "object | null", Description: "当前 Storyboard 版本及完整 Markdown。"},
			{Name: "revision", Type: "integer", Description: "当前乐观并发版本。"},
		}, RecommendedFields: []string{"uuid", "chapter_uuid", "section_no", "title", "revision"}},
		{Key: "task", Fields: []agentAPIResponseField{
			{Name: "uuid", Type: "string", Description: "任务公开 UUIDv7。"},
			{Name: "kind", Type: "string", Description: "任务类型。"},
			{Name: "resource_uuid", Type: "string", Description: "目标资源公开 UUIDv7。"},
			{Name: "status", Type: "string", Description: "任务当前状态。"},
			{Name: "error_code", Type: "string", Description: "公开错误码；无错误时为空。"},
			{Name: "error_message", Type: "string", Description: "公开错误信息；无错误时为空。"},
		}, RecommendedFields: []string{"uuid", "kind", "resource_uuid", "status", "error_code", "error_message"}},
	}
	return append(base, phase3AgentAPIProjectors()...)
}

func agentAPIProjectorByKey(key string) (agentAPIProjector, bool) {
	for _, projector := range rawAgentAPIProjectors() {
		if projector.Key == key {
			return projector, true
		}
	}
	return agentAPIProjector{}, false
}

func recommendedAgentAPIResponseFilter(route agentAPIRoute) string {
	if route.RecommendedResponseFilter != "" {
		return route.RecommendedResponseFilter
	}
	projector, ok := agentAPIProjectorByKey(route.Projector)
	if !ok {
		return ".data"
	}
	path := ".data"
	if projector.List {
		path = ".data.items[]"
		projector, ok = agentAPIProjectorByKey(projector.ItemProjector)
		if !ok {
			return ".data"
		}
	}
	fields := projector.RecommendedFields
	if len(fields) == 0 {
		fields = agentAPIProjectorFieldNames(projector)
	}
	if len(fields) == 0 {
		return ".data"
	}
	return path + " | {" + strings.Join(fields, ",") + "}"
}

func apiObject(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
}

func apiString(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func apiInteger(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func rawAgentAPIRoutes() []agentAPIRoute {
	project := "/api/v1/projects/{project_uuid}"
	storyUpdate := apiObject(map[string]any{"story_md": apiString("完整 STORY.md 内容。"), "expected_revision": apiInteger("刚读取到的最新 revision。")}, "story_md", "expected_revision")
	chapterUpdate := apiObject(map[string]any{
		"content":           apiString("完整替换章节正文。"),
		"content_format":    map[string]any{"type": "string", "enum": []string{"txt", "md"}, "description": "章节正文格式。"},
		"expected_revision": apiInteger("刚读取到的最新 Chapter revision。"),
	}, "content", "content_format", "expected_revision")
	assetCreate := apiObject(map[string]any{
		"file_uuid": apiString("当前 Thread 的 image_gen 刚返回且尚未消费的通用生成图片 File UUIDv7；已有项目图片或现有设定项 current_variant.asset.uuid 不可直接使用。与 upload_uuid 必须且只能提供一个。"), "upload_uuid": apiString("当前项目已就绪且尚未消费的上传 UUIDv7；与 file_uuid 必须且只能提供一个。"),
		"asset_type": map[string]any{"type": "string", "enum": []string{"character", "scene", "prop", "reference"}, "description": "设定项类型。"},
		"title":      apiString("设定项标题。"), "summary": apiString("可选设定项简介。"),
		"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "可选设定项标签。"},
	}, "asset_type", "title")
	assetUpdate := apiObject(map[string]any{
		"expected_revision": apiInteger("刚读取到的最新 Premise Asset revision。"), "file_uuid": apiString("可选；必须是当前 Thread 中 image_gen 刚返回、用途匹配且尚未消费的文件 UUIDv7。"),
		"asset_type": map[string]any{"type": "string", "enum": []string{"character", "scene", "prop", "reference"}, "description": "设定项类型。"},
		"title":      apiString("设定项标题。"), "summary": apiString("设定项简介。"),
		"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "设定项标签。"},
	}, "expected_revision")
	deleteBody := apiObject(map[string]any{"expected_revision": apiInteger("刚读取到的最新 Premise Asset revision。")}, "expected_revision")
	storyboardBody := apiObject(map[string]any{"content_md": apiString("完整替换 Storyboard Markdown。"), "expected_revision": apiInteger("刚读取到的最新 Comic Section revision。")}, "content_md", "expected_revision")
	generationBody := apiObject(map[string]any{
		"prompt": apiString("生成指令。"), "model": apiString("可选模型覆盖值。"),
		"premise_asset_uuids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "可选 Premise Asset 参考 UUIDv7 列表。"},
	}, "prompt")
	chapterGenerationBody := apiObject(map[string]any{
		"prompt_key": apiEnum("章节正文 Prompt。普通章节使用 story_chapter；创建下一章或继续写作使用 next_story_chapter。", "story_chapter", "next_story_chapter"),
		"prompt":     apiString("生成指令。"), "model": apiString("可选模型覆盖值。"),
	}, "prompt_key", "prompt")
	base := []agentAPIRoute{
		{ID: RouteStoryProfileGet, Action: "读取故事档案", Method: "GET", PathTemplate: project + "/story-profile", Handler: RouteStoryProfileGet, Projector: "story_profile", DocPath: storyDocPath, RecommendedResponseFilter: ".data | {uuid,revision,story_md,projection_state}", ReadOnly: true, Risk: RiskLow},
		{ID: RouteStoryProfileUpdate, Action: "更新故事档案", Method: "PUT", PathTemplate: project + "/story-profile", Handler: RouteStoryProfileUpdate, Projector: "story_profile", DocPath: storyDocPath, BodySchema: storyUpdate, ExpectedRevision: true, Risk: RiskWrite},
		{ID: RouteChapterList, Action: "列出章节", Method: "GET", PathTemplate: project + "/chapters", Handler: RouteChapterList, Projector: "chapter_list", DocPath: chapterDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RouteChapterGet, Action: "读取章节", Method: "GET", PathTemplate: project + "/chapters/{chapter_uuid}", Handler: RouteChapterGet, Projector: "chapter", DocPath: chapterDocPath, RecommendedResponseFilter: ".data | {uuid,chapter_code,title,revision,current_story}", ReadOnly: true, Risk: RiskLow},
		{ID: RouteChapterStoryUpdate, Action: "更新章节正文", Method: "PUT", PathTemplate: project + "/chapters/{chapter_uuid}/current-story", Handler: RouteChapterStoryUpdate, Projector: "chapter", DocPath: chapterDocPath, BodySchema: chapterUpdate, ExpectedRevision: true, Risk: RiskWrite},
		{ID: RoutePremiseGet, Action: "读取当前 Premise", Method: "GET", PathTemplate: project + "/premise", Handler: RoutePremiseGet, Projector: "premise", DocPath: premiseDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RoutePremiseAssetList, Action: "列出设定项", Method: "GET", PathTemplate: project + "/premise-assets", Handler: RoutePremiseAssetList, Projector: "premise_asset_list", DocPath: premiseAssetDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RoutePremiseAssetGet, Action: "读取设定项", Method: "GET", PathTemplate: project + "/premise-assets/{premise_asset_uuid}", Handler: RoutePremiseAssetGet, Projector: "premise_asset", DocPath: premiseAssetDocPath, RecommendedResponseFilter: ".data | {uuid,asset_type,title,summary,tags,current_variant,revision}", ReadOnly: true, Risk: RiskLow},
		{ID: RoutePremiseAssetCreate, Action: "创建设定项", Method: "POST", PathTemplate: project + "/premise-assets", Handler: RoutePremiseAssetCreate, Projector: "premise_asset", DocPath: premiseAssetDocPath, BodySchema: assetCreate, Risk: RiskWrite},
		{ID: RoutePremiseAssetUpdate, Action: "更新设定项", Method: "PATCH", PathTemplate: project + "/premise-assets/{premise_asset_uuid}", Handler: RoutePremiseAssetUpdate, Projector: "premise_asset", DocPath: premiseAssetDocPath, BodySchema: assetUpdate, ExpectedRevision: true, Risk: RiskWrite},
		{ID: RoutePremiseAssetDelete, Action: "将设定项移入回收站", Method: "DELETE", PathTemplate: project + "/premise-assets/{premise_asset_uuid}", Handler: RoutePremiseAssetDelete, Projector: "premise_asset", DocPath: premiseAssetDocPath, BodySchema: deleteBody, ExpectedRevision: true, Risk: RiskDangerous, RequiresConfirmation: true},
		{ID: RouteComicSectionGet, Action: "读取漫画 Section", Method: "GET", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections/{section_uuid}", Handler: RouteComicSectionGet, Projector: "comic_section", DocPath: comicSectionDocPath, RecommendedResponseFilter: ".data | {uuid,chapter_uuid,section_no,title,description_md,current_storyboard,revision}", ReadOnly: true, Risk: RiskLow},
		{ID: RouteStoryboardUpdate, Action: "更新 Storyboard", Method: "POST", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants", Handler: RouteStoryboardUpdate, Projector: "comic_section", DocPath: storyboardDocPath, BodySchema: storyboardBody, ExpectedRevision: true, Risk: RiskWrite},
		{ID: RouteChapterGenerationCreate, Action: "创建章节生成任务", Method: "POST", PathTemplate: project + "/chapters/{chapter_uuid}/generations", Handler: RouteChapterGenerationCreate, Projector: "task", DocPath: generationDocPath, BodySchema: chapterGenerationBody, Async: true, Risk: RiskWrite},
		{ID: RoutePremiseSettingGenerationCreate, Action: "创建 Premise 设定图任务", Method: "POST", PathTemplate: project + "/premise-sources/{source_uuid}/setting-generations", Handler: RoutePremiseSettingGenerationCreate, Projector: "task", DocPath: generationDocPath, BodySchema: generationBody, Async: true, Risk: RiskWrite},
		{ID: RoutePremiseBreakdownCreate, Action: "创建 Premise 拆解任务", Method: "POST", PathTemplate: project + "/premise-setting-images/{setting_image_uuid}/breakdowns", Handler: RoutePremiseBreakdownCreate, Projector: "task", DocPath: generationDocPath, BodySchema: generationBody, Async: true, Risk: RiskWrite},
		{ID: RouteComicImageGenerationCreate, Action: "创建漫画图片任务", Method: "POST", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-generations", Handler: RouteComicImageGenerationCreate, Projector: "task", DocPath: generationDocPath, BodySchema: generationBody, Async: true, Risk: RiskWrite},
		{ID: RouteStoryTaskGet, Action: "读取故事任务状态", Method: "GET", PathTemplate: project + "/tasks/{task_uuid}", Handler: RouteStoryTaskGet, Projector: "task", DocPath: taskDocPath, ReadOnly: true, Async: true, Risk: RiskLow},
		{ID: RouteProductionTaskGet, Action: "读取生产任务状态", Method: "GET", PathTemplate: project + "/production-tasks/{task_uuid}", Handler: RouteProductionTaskGet, Projector: "task", DocPath: taskDocPath, ReadOnly: true, Async: true, Risk: RiskLow},
	}
	return append(base, phase3AgentAPIRoutes()...)
}

func agentAPIRoutes() []agentAPIRoute {
	return rawAgentAPIRoutes()
}

func parseAgentAPIRequest(tc toolContext, args map[string]any) (agentAPIRequest, error) {
	return parseAgentAPIRequestWithRoutes(tc, args, agentAPIRoutes())
}

func (service *Service) parseAgentAPIRequest(tc toolContext, args map[string]any) (agentAPIRequest, error) {
	return parseAgentAPIRequestWithRoutes(tc, args, service.requestAPIRoutes())
}

func parseAgentAPIRequestWithRoutes(tc toolContext, args map[string]any, routes []agentAPIRoute) (agentAPIRequest, error) {
	if normalizedToolMode(tc.ToolMode) != ToolModeProjectAPI || !isUUIDv7(tc.ProjectUUID) {
		return agentAPIRequest{}, domainError(CodeToolNotAllowed, "request_api 不适用于当前 Tool Mode", "当前 Run 没有启用 project_api_tools。", nil)
	}
	method := stringArg(args, "method")
	if method != "GET" && method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
		return agentAPIRequest{}, domainError(CodeToolValidation, "request_api method 无效", "method 必须使用注册路由声明的标准大写 HTTP 方法。", nil)
	}
	path := stringArg(args, "url")
	if err := validateAgentAPIPath(path); err != nil {
		return agentAPIRequest{}, err
	}
	body, hasBody, err := requestBodyArg(args)
	if err != nil {
		return agentAPIRequest{}, err
	}
	if hasBody {
		encoded, _ := json.Marshal(body)
		if len(encoded) > 256<<10 || containsProjectUUIDField(body) {
			return agentAPIRequest{}, domainError(CodeToolValidation, "request_body 越界", "request_body 不得携带 project_uuid，且必须保持在大小限制内。", nil)
		}
	}
	query, hasQuery, err := requestQueryArg(args)
	if err != nil {
		return agentAPIRequest{}, err
	}
	if err := validatePublicArguments(query, "query"); err != nil {
		return agentAPIRequest{}, err
	}
	if err := validatePublicArguments(body, "request_body"); err != nil {
		return agentAPIRequest{}, err
	}
	filter := strings.TrimSpace(stringArg(args, "response_filter"))
	if filter == "" || len(filter) > 2048 {
		return agentAPIRequest{}, invalidResponseFilter(filter)
	}
	if _, err := parseResponseFilter(filter); err != nil {
		return agentAPIRequest{}, invalidResponseFilter(filter)
	}
	var matched *agentAPIRoute
	var params map[string]string
	for _, candidate := range routes {
		if candidate.Method != method {
			continue
		}
		values, routeMatched := matchAgentAPIPath(candidate.PathTemplate, path)
		if routeMatched {
			copy := candidate
			matched, params = &copy, values
			break
		}
	}
	if matched == nil {
		return agentAPIRequest{}, domainError(CodeToolNotAllowed, "项目 API 路由不存在", "method + path 没有匹配当前服务端注册的 Project API Route。", nil)
	}
	if params["project_uuid"] != tc.ProjectUUID {
		return agentAPIRequest{}, domainError(CodeToolNotAllowed, "项目 API 路径越界", "url 只能包含当前 project_uuid。", nil)
	}
	useDispatcher := matched.Passthrough
	if !matched.Passthrough {
		if shapeErr := validateReviewedAgentAPIRequestShape(*matched, query, hasQuery, body, hasBody); shapeErr != nil {
			if !matched.ServerRoute {
				return agentAPIRequest{}, shapeErr
			}
			useDispatcher = true
		}
	}
	request := agentAPIRequest{Route: *matched, Method: method, Path: path, Query: query, Body: body, HasBody: hasBody, UseDispatcher: useDispatcher, ResponseFilter: filter, Params: params}
	request.TargetUUID = routeTargetUUID(request, tc.Thread)
	return request, nil
}

func validateReviewedAgentAPIRequestShape(route agentAPIRoute, query map[string]any, hasQuery bool, body map[string]any, hasBody bool) error {
	if route.QuerySchema == nil && hasQuery {
		return domainError(CodeToolValidation, "query 不适用于当前路由", "当前 Route 没有注册 query schema。", nil)
	}
	if route.QuerySchema != nil && hasQuery {
		if err := validateArgumentShape("query", query, route.QuerySchema); err != nil {
			return err
		}
	}
	if route.BodySchema == nil && hasBody {
		return domainError(CodeToolValidation, "request_body 不适用于当前路由", "只读路由不得携带 request_body。", nil)
	}
	if route.BodySchema != nil && !hasBody {
		return domainError(CodeToolValidation, "request_body 缺失", "当前写路由要求 JSON Object request_body。", nil)
	}
	if hasBody && route.BodySchema != nil {
		if err := validateArgumentShape("request_body", body, route.BodySchema); err != nil {
			return err
		}
	}
	return nil
}

func validateAgentAPIPath(path string) error {
	if path == "" || !strings.HasPrefix(path, "/api/v1/projects/") || strings.HasPrefix(path, "//") || strings.HasSuffix(path, "/") || strings.ContainsAny(path, "?#%\\") || strings.Contains(path, "//") || strings.Contains(path, "://") {
		return invalidAgentAPIPath()
	}
	parsed, err := url.ParseRequestURI(path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Scheme != "" || parsed.Opaque != "" || parsed.Fragment != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.RawPath != "" {
		return invalidAgentAPIPath()
	}
	for _, segment := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return invalidAgentAPIPath()
		}
	}
	return nil
}

func invalidAgentAPIPath() error {
	return domainError(CodeToolValidation, "request_api url 无效", "只允许规范的当前项目相对 API 路径；完整 URL、Query、Fragment、编码路径、反斜杠和路径穿越均被拒绝。", nil)
}

func requestBodyArg(args map[string]any) (map[string]any, bool, error) {
	value, present := args["request_body"]
	if !present {
		return nil, false, nil
	}
	body, ok := value.(map[string]any)
	if !ok || body == nil {
		return nil, true, domainError(CodeToolValidation, "request_body 不是 JSON Object", "request_body 必须是 JSON Object。", nil)
	}
	return body, true, nil
}

func requestQueryArg(args map[string]any) (map[string]any, bool, error) {
	value, present := args["query"]
	if !present {
		return nil, false, nil
	}
	query, ok := value.(map[string]any)
	if !ok || query == nil {
		return nil, true, domainError(CodeToolValidation, "query 不是 JSON Object", "query 必须是注册 Route 接受的 JSON Object。", nil)
	}
	encoded, _ := json.Marshal(query)
	if len(encoded) > 16<<10 {
		return nil, true, domainError(CodeToolValidation, "query 过大", "query 必须保持在 16 KiB 以内。", nil)
	}
	return query, true, nil
}

func containsProjectUUIDField(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(key, "project_uuid") || containsProjectUUIDField(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsProjectUUIDField(child) {
				return true
			}
		}
	}
	return false
}

func matchAgentAPIPath(template, path string) (map[string]string, bool) {
	wanted, actual := strings.Split(strings.TrimPrefix(template, "/"), "/"), strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(wanted) != len(actual) {
		return nil, false
	}
	params := map[string]string{}
	for index, segment := range wanted {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			key := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
			if strings.HasSuffix(key, "_uuid") && !isUUIDv7(actual[index]) {
				return nil, false
			}
			params[key] = actual[index]
			continue
		}
		if segment != actual[index] {
			return nil, false
		}
	}
	return params, true
}

func routeTargetUUID(request agentAPIRequest, thread threadRecord) string {
	segments := strings.Split(request.Route.PathTemplate, "/")
	for index := len(segments) - 1; index >= 0; index-- {
		segment := segments[index]
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			key := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
			if key != "project_uuid" {
				if value := request.Params[key]; isUUIDv7(value) {
					return value
				}
			}
		}
	}
	if value := request.Params["project_uuid"]; isUUIDv7(value) {
		return value
	}
	return thread.UUID
}

func executeAgentAPIRoute(ctx context.Context, service *Service, store *project.Store, tc toolContext, execution toolExecutionRecord, request agentAPIRequest) (any, error) {
	if request.UseDispatcher {
		return service.executeDiscoveredProjectAPIRoute(ctx, request)
	}
	storyService := story.NewService(store)
	productionService := production.NewService(store, service.hub)
	args := cloneToolArguments(request.Body)
	if value, handled, err := executePhase3AgentAPIRoute(ctx, service, store, tc, execution, request); handled {
		return value, err
	}
	switch request.Route.Handler {
	case routeProjectAPIDispatch:
		return service.executeDiscoveredProjectAPIRoute(ctx, request)
	case RouteStoryProfileGet:
		return storyService.GetStoryProfile(ctx)
	case RouteStoryProfileUpdate:
		return updateStoryProfileTool(ctx, storyService, args)
	case RouteChapterList:
		items, err := storyService.ListChapters(ctx, "active")
		return map[string]any{"items": items}, err
	case RouteChapterGet:
		return storyService.GetChapter(ctx, request.Params["chapter_uuid"])
	case RouteChapterStoryUpdate:
		args["chapter_uuid"] = request.Params["chapter_uuid"]
		return updateChapterStoryTool(ctx, storyService, args)
	case RoutePremiseGet:
		return productionService.GetPremise(ctx)
	case RoutePremiseAssetList:
		items, err := productionService.ListPremiseAssets(ctx, "", "active")
		return map[string]any{"items": items}, err
	case RoutePremiseAssetGet:
		asset, err := productionService.GetPremiseAsset(ctx, request.Params["premise_asset_uuid"])
		if err == nil && asset.DeletedAt != nil {
			err = domainError(CodeToolNotAllowed, "设定项已在回收站", "Agent Project API 只读取 active 设定项。", nil)
		}
		return asset, err
	case RoutePremiseAssetCreate:
		return createAgentPremiseAsset(ctx, productionService, tc, execution, args)
	case RoutePremiseAssetUpdate:
		args["premise_asset_uuid"] = request.Params["premise_asset_uuid"]
		return updatePremiseAssetTool(ctx, productionService, tc, execution, args)
	case RoutePremiseAssetDelete:
		return productionService.SetPremiseAssetTrashedFromTool(ctx, request.Params["premise_asset_uuid"], true, intArg(args, "expected_revision"), execution.UUID)
	case RouteComicSectionGet:
		return productionService.GetSection(ctx, request.Params["chapter_uuid"], request.Params["section_uuid"])
	case RouteStoryboardUpdate:
		args["chapter_uuid"], args["section_uuid"] = request.Params["chapter_uuid"], request.Params["section_uuid"]
		return updateStoryboardTool(ctx, productionService, args)
	case RouteChapterGenerationCreate:
		return startAgentRouteGeneration(ctx, service, tc, execution, args, "story_chapter_generation", request.Params["chapter_uuid"], request.Params["chapter_uuid"])
	case RoutePremiseSettingGenerationCreate:
		return startAgentRouteGeneration(ctx, service, tc, execution, args, "premise_setting_generation", request.Params["source_uuid"], "")
	case RoutePremiseBreakdownCreate:
		return startAgentRouteGeneration(ctx, service, tc, execution, args, "premise_asset_breakdown", request.Params["setting_image_uuid"], "")
	case RouteComicImageGenerationCreate:
		return startAgentRouteGeneration(ctx, service, tc, execution, args, "comic_image_generation", request.Params["section_uuid"], request.Params["chapter_uuid"])
	case RouteStoryTaskGet:
		return service.queue.GetDomainTask(ctx, tc.ProjectUUID, "story_chapter_generation", request.Params["task_uuid"])
	case RouteProductionTaskGet:
		return service.queue.GetDomainTask(ctx, tc.ProjectUUID, "premise_setting_generation", request.Params["task_uuid"])
	default:
		return nil, domainError(CodeToolNotAllowed, "Route handler 未注册", "Agent Project API Route 没有进程内 handler。", nil)
	}
}

func createAgentPremiseAsset(ctx context.Context, service *production.Service, tc toolContext, execution toolExecutionRecord, args map[string]any) (production.PremiseAsset, error) {
	uploadUUID, fileUUID := stringArg(args, "upload_uuid"), stringArg(args, "file_uuid")
	if (uploadUUID == "") == (fileUUID == "") {
		return production.PremiseAsset{}, domainError(CodeToolValidation, "设定项图片来源无效", "file_uuid 与 upload_uuid 必须且只能提供一个。", nil)
	}
	input := production.CreateAssetInput{
		UploadUUID: uploadUUID, FileUUID: fileUUID, ToolExecutionUUID: execution.UUID, ChatThreadUUID: tc.Thread.UUID,
		AssetType: stringArg(args, "asset_type"), Title: stringArg(args, "title"), Summary: stringArg(args, "summary"), Tags: stringSliceArg(args, "tags"), SourceType: "manual",
	}
	if fileUUID != "" {
		return service.CreatePremiseAssetFromFile(ctx, input)
	}
	return service.ImportPremiseAsset(ctx, input)
}

func startAgentRouteGeneration(ctx context.Context, service *Service, tc toolContext, execution toolExecutionRecord, args map[string]any, kind, resourceUUID, chapterUUID string) (DomainTask, error) {
	idempotencyKey := strings.TrimSpace(execution.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = execution.UUID
	}
	request := DomainTaskRequest{
		Kind: kind, ResourceUUID: resourceUUID, ChapterUUID: chapterUUID, ProviderUUID: tc.Run.ProviderUUID,
		Model: stringArg(args, "model"), PromptKey: stringArg(args, "prompt_key"), Prompt: stringArg(args, "prompt"), PremiseAssetUUIDs: stringSliceArg(args, "premise_asset_uuids"),
		IdempotencyKey: idempotencyKey, Invocation: chatToolInvocationContext(tc, execution),
	}
	return service.queue.StartDomainTask(ctx, tc.ProjectUUID, request)
}

func chatToolInvocationContext(tc toolContext, execution toolExecutionRecord) DomainInvocationContext {
	return DomainInvocationContext{
		Source: InvocationChatTool, PresentationMode: PresentationInline, AwaitCompletion: true,
		ThreadUUID: tc.Thread.UUID, TurnUUID: tc.Turn.UUID, RunUUID: tc.Run.UUID, ToolExecutionUUID: execution.UUID,
	}
}

func compactAgentRouteValue(route agentAPIRoute, value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	projector, ok := agentAPIProjectorByKey(route.Projector)
	if !ok {
		return decoded, nil
	}
	if projector.List {
		root, _ := decoded.(map[string]any)
		items, _ := root["items"].([]any)
		itemProjector, _ := agentAPIProjectorByKey(projector.ItemProjector)
		projected := make([]any, 0, len(items))
		for _, item := range items {
			projected = append(projected, pickPublicFields(item, agentAPIProjectorFieldNames(itemProjector)))
		}
		result := map[string]any{"items": projected}
		for _, key := range []string{"pagination", "cursor_pagination", "filter_groups"} {
			if value, ok := root[key]; ok {
				result[key] = value
			}
		}
		return result, nil
	}
	if len(projector.Fields) > 0 {
		return pickPublicFields(decoded, agentAPIProjectorFieldNames(projector)), nil
	}
	return decoded, nil
}

func agentAPIProjectorFieldNames(projector agentAPIProjector) []string {
	result := make([]string, 0, len(projector.Fields))
	for _, field := range projector.Fields {
		result = append(result, field.Name)
	}
	return result
}

func pickPublicFields(value any, fields []string) map[string]any {
	input, _ := value.(map[string]any)
	result := make(map[string]any, len(fields))
	for _, field := range fields {
		if item, ok := input[field]; ok {
			result[field] = sanitizeAgentAPIValue(item)
		}
	}
	return result
}

func sanitizeAgentAPIValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			lower := strings.ToLower(key)
			if lower == "id" || strings.HasSuffix(lower, "_id") || strings.HasSuffix(lower, "_path") ||
				lower == "metadata" || lower == "content_url" || lower == "download_url" || lower == "relative_path" || lower == "root_path" ||
				lower == "authorization" || lower == "cookie" || lower == "password" || strings.HasSuffix(lower, "_password") ||
				lower == "secret" || strings.HasSuffix(lower, "_secret") || lower == "credential" || strings.HasSuffix(lower, "_credential") ||
				lower == "token" || strings.HasSuffix(lower, "_token") || lower == "api_key" || strings.HasSuffix(lower, "_api_key") {
				continue
			}
			result[key] = sanitizeAgentAPIValue(child)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			result = append(result, sanitizeAgentAPIValue(child))
		}
		return result
	default:
		return value
	}
}

func validateAgentAPIResponse(value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return err
	}
	if internalResponseField(decoded) != "" {
		return domainError(CodeToolValidation, "项目 API 响应包含内部字段", "Route projector 拒绝暴露 bigint id 或内部关联字段。", nil)
	}
	return nil
}

func internalResponseField(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lower := strings.ToLower(key)
			if lower == "id" || strings.HasSuffix(lower, "_id") {
				return key
			}
			if found := internalResponseField(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := internalResponseField(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func describeAgentAPIRoute(route agentAPIRoute) string {
	return fmt.Sprintf("%s %s (%s)", route.Method, route.PathTemplate, route.ID)
}
