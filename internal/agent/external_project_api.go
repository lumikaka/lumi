package agent

import (
	"context"
	"encoding/json"
	"strings"
)

// ExternalProjectAPI is a reviewed capability surface, independent of chat
// Thread/Turn/Run and of the automatic internal Agent route discovery.
type ExternalProjectAPI struct {
	dispatcher ProjectAPIDispatcher
	queue      Queue
}

func NewExternalProjectAPI(dispatcher ProjectAPIDispatcher, queue Queue) *ExternalProjectAPI {
	return &ExternalProjectAPI{dispatcher: dispatcher, queue: queue}
}

// New internal routes fail closed: each external route requires review here.
func externalProjectRoutes() []agentAPIRoute {
	allowed := map[string]bool{}
	for _, id := range []string{
		RouteProjectGet, RouteProjectUpdate, RouteStoryProfileGet, RouteStoryProfileUpdate,
		RouteChapterList, RouteChapterGet, RouteChapterCreate, RouteChapterUpdate, RouteChapterStoryUpdate, RouteChapterStoryList, RouteChapterTrash, RouteChapterRestore, RouteChapterPermanentDelete,
		RoutePremiseGet, RoutePremiseUpdate, RoutePremiseAssetList, RoutePremiseAssetGet, RoutePremiseAssetUpdate, RoutePremiseAssetDelete, RoutePremiseAssetRestore, RoutePremiseAssetPermanentDelete,
		RoutePremiseSourceList, RoutePremiseSourceCreate, RoutePremiseSourceUpdate, RouteSettingImageList, RouteSettingImageSelect, RoutePremiseAssetVariantList, RoutePremiseAssetVariantSelect,
		RouteComicStateGet, RouteComicSectionList, RouteComicSectionGet, RouteComicSectionCreate, RouteComicSectionUpdate, RouteComicSectionDelete, RouteComicSectionReorder, RouteStoryboardUpdate, RouteStoryboardList, RouteStoryboardSelect, RouteComicImageVariantList, RouteComicImageVariantSelect,
		RouteComicSnapshotList, RouteComicSnapshotGet, RouteComicSnapshotRestore,
		RouteChapterGenerationCreate, RoutePremiseSettingGenerationCreate, RoutePremiseBreakdownCreate, RouteComicImageGenerationCreate,
		RouteStoryTaskList, RouteStoryTaskGet, RouteStoryTaskEventList, RouteStoryTaskCancel, RouteStoryTaskRetry,
		RouteProductionTaskList, RouteProductionTaskGet, RouteProductionTaskEventList, RouteProductionTaskCancel, RouteProductionTaskRetry,
		RouteProjectAssetList, RouteProjectAssetGet,
	} {
		allowed[id] = true
	}
	var routes []agentAPIRoute
	for _, r := range agentAPIRoutes() {
		if allowed[r.ID] {
			routes = append(routes, r)
		}
	}
	return routes
}

type ExternalRequest struct{ request agentAPIRequest }

func (r ExternalRequest) ReadOnly() bool { return r.request.Route.ReadOnly }
func (r ExternalRequest) Dangerous() bool {
	return r.request.Route.RequiresConfirmation || r.request.Route.Risk == RiskDangerous
}
func (r ExternalRequest) Async() bool {
	return r.request.Route.Async && r.request.Method == "POST" && strings.HasPrefix(r.request.Route.ID, "generation.")
}
func (r ExternalRequest) Action() string  { return r.request.Route.Action }
func (r ExternalRequest) Revision() int64 { return agentAPIRequestExpectedRevision(r.request) }
func (api *ExternalProjectAPI) Prepare(projectUUID string, args map[string]any) (ExternalRequest, error) {
	if !isUUIDv7(projectUUID) {
		return ExternalRequest{}, domainError(CodeToolNotAllowed, "项目授权无效", "项目必须为 UUIDv7。", nil)
	}
	for k := range args {
		switch k {
		case "method", "url", "query", "request_body", "response_filter":
		default:
			return ExternalRequest{}, domainError(CodeToolValidation, "未知工具参数", "不允许传递内部调用或确认元数据。", nil)
		}
	}
	if err := validateExternalMetadata(args); err != nil {
		return ExternalRequest{}, err
	}
	r, err := parseProjectAPIRequest(projectUUID, args, externalProjectRoutes())
	return ExternalRequest{r}, err
}
func validateExternalMetadata(v any) error {
	switch x := v.(type) {
	case map[string]any:
		for k, v := range x {
			switch strings.ToLower(k) {
			case "confirmed", "confirmation", "invocation", "thread_uuid", "turn_uuid", "run_uuid", "tool_execution_uuid", "route_id", "idempotency_key", "provider_uuid":
				return domainError(CodeToolValidation, "不允许的调用元数据", "归属、确认、幂等和服务商由 Lumi 决定。", nil)
			}
			if err := validateExternalMetadata(v); err != nil {
				return err
			}
		}
	case []any:
		for _, v := range x {
			if err := validateExternalMetadata(v); err != nil {
				return err
			}
		}
	}
	return nil
}
func (api *ExternalProjectAPI) ReadDoc(args map[string]any) (map[string]any, error) {
	for k := range args {
		if k != "path" {
			return nil, domainError(CodeToolValidation, "未知文档参数", "只允许 path。", nil)
		}
	}
	result, err := readProjectAgentDoc(args, externalProjectRoutes())
	if err != nil {
		return nil, err
	}
	// The shared contracts also serve the internal Agent; make the external
	// allowlist explicit instead of implying every documented route is exposed.
	var catalog []map[string]any
	for _, r := range externalProjectRoutes() {
		catalog = append(catalog, map[string]any{"method": r.Method, "path": r.PathTemplate, "read_only": r.ReadOnly, "requires_confirmation": r.RequiresConfirmation || r.Risk == RiskDangerous, "response_filter": recommendedAgentAPIResponseFilter(r)})
	}
	result["external_routes"] = catalog
	result["external_instructions"] = "Only external_routes are callable. Shared chat guides do not grant tools or routes. Write idempotency_key is a top-level request_api argument; confirmations happen only in Lumi. Tool results wrap business envelopes inside data.result; data.call records status. Use get_call to recover results and read_media for images."
	return result, nil
}

// Execute is only called after the application has durably authorized this exact
// request. REST handlers retain transactions, revision checks and events.
func (api *ExternalProjectAPI) Execute(ctx context.Context, r ExternalRequest, callUUID string) (any, error) {
	req := r.request
	body := map[string]any{}
	for k, v := range req.Body {
		body[k] = v
	}
	if r.Async() {
		body["idempotency_key"] = "mcp:" + callUUID
	}
	var data any
	var err error
	if r.Async() {
		kinds := map[string]string{RouteChapterGenerationCreate: "story_chapter_generation", RoutePremiseSettingGenerationCreate: "premise_setting_generation", RoutePremiseBreakdownCreate: "premise_asset_breakdown", RouteComicImageGenerationCreate: "comic_image_generation"}
		resource := req.Params["chapter_uuid"]
		for _, k := range []string{"source_uuid", "setting_image_uuid", "section_uuid"} {
			if req.Params[k] != "" {
				resource = req.Params[k]
			}
		}
		var task DomainTask
		var taskErr error
		found := false
		if recovery, ok := api.queue.(interface {
			RecoverExternalTask(context.Context, string, string, string) (DomainTask, bool, error)
		}); ok {
			task, found, taskErr = recovery.RecoverExternalTask(ctx, req.Params["project_uuid"], kinds[req.Route.ID], "mcp:"+callUUID)
		}
		if taskErr == nil && !found {
			task, taskErr = api.queue.StartDomainTask(ctx, req.Params["project_uuid"], DomainTaskRequest{Kind: kinds[req.Route.ID], ResourceUUID: resource, ChapterUUID: req.Params["chapter_uuid"], PromptKey: stringArg(body, "prompt_key"), Prompt: stringArg(body, "prompt"), Model: stringArg(body, "model"), PremiseAssetUUIDs: externalStringSlice(body["premise_asset_uuids"]), IdempotencyKey: "mcp:" + callUUID, Invocation: ExternalInvocationContext(callUUID)})
		}
		data, err = task, taskErr
	} else {
		query, hasBody := req.Query, req.HasBody
		// Reviewed Agent contracts use a revision body for soft deletion; REST
		// transports that same revision in query parameters.
		if req.Method == "DELETE" && req.Route.RevisionSource == agentAPIRevisionBody {
			query = map[string]any{"expected_revision": body["expected_revision"]}
			hasBody = false
		}
		response, dispatchErr := api.dispatcher(ctx, ProjectAPIDispatchRequest{Method: req.Method, Path: req.Path, Query: query, Body: body, HasBody: hasBody})
		if dispatchErr != nil {
			return nil, dispatchErr
		}
		var envelope struct {
			Success bool                                     `json:"success"`
			Data    any                                      `json:"data"`
			Error   *struct{ Code, Message, Details string } `json:"error"`
		}
		if err = json.Unmarshal(response.Body, &envelope); err != nil {
			return nil, err
		}
		if !envelope.Success {
			if envelope.Error != nil {
				return nil, domainError(envelope.Error.Code, envelope.Error.Message, envelope.Error.Details, nil)
			}
			return nil, domainError(CodeStateConflict, "项目 API 失败", "", nil)
		}
		data = envelope.Data
		if req.Route.ID == RouteComicSectionDelete && data == nil {
			data = map[string]any{"uuid": req.Params["section_uuid"], "deleted": true}
		}
	}
	if err != nil {
		return nil, err
	}
	value, err := compactAgentRouteValue(req.Route, data)
	if err != nil {
		return nil, err
	}
	if err = validateAgentAPIResponse(value); err != nil {
		return nil, err
	}
	value, err = runResponseFilter(map[string]any{"success": true, "data": value}, req.ResponseFilter)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(encoded) > MaxToolResult-1024 {
		return nil, domainError(CodeResultTooLarge, "结果过大", "请缩小 response_filter 或使用分页。操作可能已成功，请读取调用状态。", nil)
	}
	return value, nil
}

func externalStringSlice(v any) []string {
	var out []string
	if a, ok := v.([]any); ok {
		for _, v := range a {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
