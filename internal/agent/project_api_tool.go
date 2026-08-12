package agent

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"lumi/internal/production"
)

const currentProjectAPIToolName = "request_current_project_api"

type currentProjectAPIRoute string

const (
	currentProjectAPIPremise     currentProjectAPIRoute = "premise"
	currentProjectAPIAssetList   currentProjectAPIRoute = "premise_asset_list"
	currentProjectAPIAssetGet    currentProjectAPIRoute = "premise_asset_get"
	currentProjectAPIAssetCreate currentProjectAPIRoute = "premise_asset_create"
	currentProjectAPIAssetUpdate currentProjectAPIRoute = "premise_asset_update"
	currentProjectAPIAssetDelete currentProjectAPIRoute = "premise_asset_delete"
)

type currentProjectAPIRequest struct {
	Route            currentProjectAPIRoute
	Method           string
	Body             map[string]any
	ExpectedRevision int64
}

func parseCurrentProjectAPIRequest(tc toolContext, args map[string]any) (currentProjectAPIRequest, error) {
	if tc.Thread.Scope != ThreadScopePremise || tc.Thread.Scene != SceneAssetReference || !isUUIDv7(tc.ProjectUUID) || !isUUIDv7(tc.Thread.SubjectUUID) {
		return currentProjectAPIRequest{}, domainError(CodeToolNotAllowed, "通用项目工具不适用于当前场景", "request_current_project_api 只允许用于当前设定项引用会话。", nil)
	}
	method := stringArg(args, "method")
	if method != "GET" && method != "POST" && method != "PATCH" && method != "DELETE" {
		return currentProjectAPIRequest{}, domainError(CodeToolValidation, "项目 API method 无效", "method 只允许 GET、POST、PATCH 或 DELETE。", nil)
	}
	rawURL := stringArg(args, "url")
	if rawURL == "" || !strings.HasPrefix(rawURL, "/") || strings.HasPrefix(rawURL, "//") || strings.Contains(rawURL, "#") {
		return currentProjectAPIRequest{}, invalidCurrentProjectAPIURL()
	}
	pathPart := strings.SplitN(rawURL, "?", 2)[0]
	if strings.HasSuffix(pathPart, "/") || strings.Contains(pathPart, "%") || strings.Contains(pathPart, "\\") || strings.Contains(pathPart, "//") {
		return currentProjectAPIRequest{}, invalidCurrentProjectAPIURL()
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Scheme != "" || parsed.Opaque != "" || parsed.Fragment != "" {
		return currentProjectAPIRequest{}, invalidCurrentProjectAPIURL()
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for _, segment := range segments {
		if segment == "." || segment == ".." || segment == "" {
			return currentProjectAPIRequest{}, invalidCurrentProjectAPIURL()
		}
	}
	if len(segments) < 5 || segments[0] != "api" || segments[1] != "v1" || segments[2] != "projects" || segments[3] != tc.ProjectUUID {
		return currentProjectAPIRequest{}, domainError(CodeToolNotAllowed, "项目 API 路径越界", "url 必须使用当前项目 UUID 和允许的相对路径。", nil)
	}
	body, hasBody, err := currentProjectAPIRequestBody(args)
	if err != nil {
		return currentProjectAPIRequest{}, err
	}
	request := currentProjectAPIRequest{Method: method, Body: body}
	query := parsed.Query()
	switch {
	case len(segments) == 5 && segments[4] == "premise" && method == "GET":
		if hasBody || len(query) != 0 {
			return currentProjectAPIRequest{}, invalidCurrentProjectAPIRoute()
		}
		request.Route = currentProjectAPIPremise
	case len(segments) == 5 && segments[4] == "premise-assets" && method == "GET":
		if hasBody || len(query) != 0 {
			return currentProjectAPIRequest{}, invalidCurrentProjectAPIRoute()
		}
		request.Route = currentProjectAPIAssetList
	case len(segments) == 5 && segments[4] == "premise-assets" && method == "POST":
		if !hasBody || len(query) != 0 || !bodyKeysAllowed(body, "file_uuid", "asset_type", "title", "summary", "tags") || !bodyKeysPresent(body, "file_uuid", "asset_type", "title") {
			return currentProjectAPIRequest{}, invalidCurrentProjectAPIRequestBody()
		}
		request.Route = currentProjectAPIAssetCreate
	case len(segments) == 6 && segments[4] == "premise-assets" && segments[5] == tc.Thread.SubjectUUID && method == "GET":
		if hasBody || len(query) != 0 {
			return currentProjectAPIRequest{}, invalidCurrentProjectAPIRoute()
		}
		request.Route = currentProjectAPIAssetGet
	case len(segments) == 6 && segments[4] == "premise-assets" && segments[5] == tc.Thread.SubjectUUID && method == "PATCH":
		if !hasBody || len(query) != 0 || !bodyKeysAllowed(body, "expected_revision", "file_uuid", "asset_type", "title", "summary", "tags") || !bodyKeysPresent(body, "expected_revision") {
			return currentProjectAPIRequest{}, invalidCurrentProjectAPIRequestBody()
		}
		request.ExpectedRevision = intArg(body, "expected_revision")
		if request.ExpectedRevision < 0 {
			return currentProjectAPIRequest{}, domainError(CodeToolValidation, "expected_revision 无效", "expected_revision 必须是非负整数。", nil)
		}
		request.Route = currentProjectAPIAssetUpdate
	case len(segments) == 6 && segments[4] == "premise-assets" && segments[5] == tc.Thread.SubjectUUID && method == "DELETE":
		if hasBody || !onlyQueryKeys(query, "expected_revision") || !singleQueryValues(query) || query.Get("expected_revision") == "" {
			return currentProjectAPIRequest{}, invalidCurrentProjectAPIRoute()
		}
		revision, parseErr := strconv.ParseInt(query.Get("expected_revision"), 10, 64)
		if parseErr != nil || revision < 0 {
			return currentProjectAPIRequest{}, domainError(CodeToolValidation, "expected_revision 无效", "expected_revision 必须是非负整数。", parseErr)
		}
		request.ExpectedRevision = revision
		request.Route = currentProjectAPIAssetDelete
	default:
		if len(segments) == 6 && segments[4] == "premise-assets" && segments[5] != tc.Thread.SubjectUUID {
			return currentProjectAPIRequest{}, domainError(CodeToolNotAllowed, "项目 API subject 越界", "asset_reference 只能读取、更新或移入回收站当前 subject_uuid。", nil)
		}
		return currentProjectAPIRequest{}, invalidCurrentProjectAPIRoute()
	}
	return request, nil
}

func executeCurrentProjectAPITool(ctx context.Context, service *production.Service, tc toolContext, execution toolExecutionRecord, args map[string]any) (any, error) {
	request, err := parseCurrentProjectAPIRequest(tc, args)
	if err != nil {
		return nil, err
	}
	current, err := service.GetPremiseAsset(ctx, tc.Thread.SubjectUUID)
	if err != nil {
		return nil, err
	}
	if current.DeletedAt != nil {
		if request.Route == currentProjectAPIAssetDelete {
			return service.SetPremiseAssetTrashedFromTool(ctx, tc.Thread.SubjectUUID, true, request.ExpectedRevision, execution.UUID)
		}
		return nil, domainError(CodeToolNotAllowed, "引用设定项已在回收站", "该引用会话不能继续读取、创建、更新或生成图片。", nil)
	}
	switch request.Route {
	case currentProjectAPIPremise:
		return service.GetPremise(ctx)
	case currentProjectAPIAssetList:
		items, err := service.ListPremiseAssets(ctx, "", "active")
		return map[string]any{"items": items}, err
	case currentProjectAPIAssetGet:
		return current, nil
	case currentProjectAPIAssetCreate:
		return service.CreatePremiseAssetFromFile(ctx, production.CreateAssetInput{
			FileUUID:               stringArg(request.Body, "file_uuid"),
			ToolExecutionUUID:      execution.UUID,
			ChatThreadUUID:         tc.Thread.UUID,
			SourcePremiseAssetUUID: tc.Thread.SubjectUUID,
			AssetType:              stringArg(request.Body, "asset_type"),
			Title:                  stringArg(request.Body, "title"),
			Summary:                stringArg(request.Body, "summary"),
			Tags:                   stringSliceArg(request.Body, "tags"),
			SourceType:             "manual",
		})
	case currentProjectAPIAssetUpdate:
		updateArgs := cloneToolArguments(request.Body)
		updateArgs["premise_asset_uuid"] = tc.Thread.SubjectUUID
		return updatePremiseAssetTool(ctx, service, tc, execution, updateArgs)
	case currentProjectAPIAssetDelete:
		return service.SetPremiseAssetTrashedFromTool(ctx, tc.Thread.SubjectUUID, true, request.ExpectedRevision, execution.UUID)
	default:
		return nil, invalidCurrentProjectAPIRoute()
	}
}

func currentProjectAPIRequestBody(args map[string]any) (map[string]any, bool, error) {
	value, present := args["request_body"]
	if !present {
		return nil, false, nil
	}
	body, ok := value.(map[string]any)
	if !ok {
		return nil, true, invalidCurrentProjectAPIRequestBody()
	}
	return body, true, nil
}

func cloneToolArguments(args map[string]any) map[string]any {
	result := make(map[string]any, len(args)+1)
	for key, value := range args {
		result[key] = value
	}
	return result
}

func bodyKeysAllowed(body map[string]any, allowed ...string) bool {
	allowlist := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowlist[key] = true
	}
	for key := range body {
		if !allowlist[key] {
			return false
		}
	}
	return true
}

func bodyKeysPresent(body map[string]any, required ...string) bool {
	for _, key := range required {
		if _, ok := body[key]; !ok {
			return false
		}
	}
	return true
}

func onlyQueryKeys(values url.Values, allowed ...string) bool {
	allowlist := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowlist[key] = true
	}
	for key := range values {
		if !allowlist[key] {
			return false
		}
	}
	return true
}

func singleQueryValues(values url.Values) bool {
	for _, candidates := range values {
		if len(candidates) != 1 {
			return false
		}
	}
	return true
}

func invalidCurrentProjectAPIURL() error {
	return domainError(CodeToolValidation, "项目 API url 无效", "只允许规范的相对 API 路径；绝对 URL、路径穿越、编码路径与 fragment 均被拒绝。", nil)
}

func invalidCurrentProjectAPIRoute() error {
	return domainError(CodeToolNotAllowed, "项目 API 路由不在 allowlist", "只允许当前 asset_reference 场景声明的 GET、POST、PATCH 与软删除路由。", nil)
}

func invalidCurrentProjectAPIRequestBody() error {
	return domainError(CodeToolValidation, "项目 API request_body 无效", "request_body 缺少必填字段、包含未知字段或不适用于当前路由。", nil)
}
