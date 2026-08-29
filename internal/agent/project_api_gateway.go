package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const routeProjectAPIDispatch = "project_api.dispatch"

// ProjectAPIRouteSpec describes one project-scoped route registered by the
// application HTTP router. Paths may use Echo's :param notation or the Agent
// registry's {param} notation.
type ProjectAPIRouteSpec struct {
	Method string
	Path   string
}

// ProjectAPIDispatchRequest is the public, transport-neutral request passed to
// the application-owned in-process project API dispatcher.
type ProjectAPIDispatchRequest struct {
	Method  string
	Path    string
	Query   map[string]any
	Body    map[string]any
	HasBody bool
}

// ProjectAPIDispatchResponse contains the standard Lumi API envelope emitted
// by the application router.
type ProjectAPIDispatchResponse struct {
	Status int
	Body   []byte
}

type ProjectAPIDispatcher func(context.Context, ProjectAPIDispatchRequest) (ProjectAPIDispatchResponse, error)

// WithProjectAPIGateway makes the application router the source of truth for
// request_api route availability. Reviewed Agent routes remain overlays for
// stricter schemas, compact projectors, idempotency, and risk policy.
func (service *Service) WithProjectAPIGateway(specs []ProjectAPIRouteSpec, dispatcher ProjectAPIDispatcher) *Service {
	if service == nil {
		return service
	}
	service.projectAPIRoutes = mergeProjectAPIRoutes(specs)
	service.projectAPIDispatcher = dispatcher
	return service
}

func (service *Service) requestAPIRoutes() []agentAPIRoute {
	if service != nil && len(service.projectAPIRoutes) > 0 {
		return service.projectAPIRoutes
	}
	return agentAPIRoutes()
}

// ProjectAPIRoutes returns the effective current-project method/path catalog
// used by request_api. It is intended for application diagnostics and route
// coverage tests; callers receive a defensive copy.
func (service *Service) ProjectAPIRoutes() []ProjectAPIRouteSpec {
	routes := service.requestAPIRoutes()
	result := make([]ProjectAPIRouteSpec, 0, len(routes))
	for _, route := range routes {
		result = append(result, ProjectAPIRouteSpec{Method: route.Method, Path: route.PathTemplate})
	}
	return result
}

func mergeProjectAPIRoutes(specs []ProjectAPIRouteSpec) []agentAPIRoute {
	reviewed := make(map[string]agentAPIRoute, len(agentAPIRoutes()))
	for _, route := range agentAPIRoutes() {
		reviewed[agentAPIRouteKey(route.Method, route.PathTemplate)] = route
	}

	byKey := make(map[string]agentAPIRoute, len(specs))
	for _, spec := range specs {
		method := strings.ToUpper(strings.TrimSpace(spec.Method))
		path, ok := normalizeProjectAPIRouteTemplate(spec.Path)
		if !ok || !supportedProjectAPIMethod(method) {
			continue
		}
		key := agentAPIRouteKey(method, path)
		if route, exists := reviewed[key]; exists {
			route.ServerRoute = true
			byKey[key] = route
			continue
		}
		byKey[key] = discoveredProjectAPIRoute(method, path)
	}

	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	routes := make([]agentAPIRoute, 0, len(keys))
	for _, key := range keys {
		routes = append(routes, byKey[key])
	}
	return routes
}

func supportedProjectAPIMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func normalizeProjectAPIRouteTemplate(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/api/v1/projects/") {
		return "", false
	}
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") && len(segment) > 1 {
			segments[index] = "{" + strings.TrimPrefix(segment, ":") + "}"
		}
	}
	path = strings.Join(segments, "/")
	if path != "/api/v1/projects/{project_uuid}" && !strings.HasPrefix(path, "/api/v1/projects/{project_uuid}/") {
		return "", false
	}
	return path, true
}

func agentAPIRouteKey(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

func discoveredProjectAPIRoute(method, path string) agentAPIRoute {
	route := agentAPIRoute{
		ID:           discoveredProjectAPIRouteID(method, path),
		Action:       "调用项目 API：" + method + " " + path,
		Method:       method,
		PathTemplate: path,
		Handler:      routeProjectAPIDispatch,
		DocPath:      projectAPIDocPath(path),
		Passthrough:  true,
		ServerRoute:  true,
	}
	if method == http.MethodGet {
		route.ReadOnly = true
		route.Risk = RiskLow
	} else {
		// Routes without an explicit reviewed overlay are callable, but writes
		// fail closed behind the existing fingerprint-bound confirmation flow.
		route.Risk = RiskDangerous
		route.RequiresConfirmation = true
	}
	return route
}

func projectAPIDocPath(path string) string {
	suffix := strings.TrimPrefix(path, "/api/v1/projects/{project_uuid}")
	switch {
	case suffix == "":
		return projectDocPath
	case strings.HasPrefix(suffix, "/project-setup"):
		return projectSetupDocPath
	case strings.HasPrefix(suffix, "/comic-exports"):
		return comicExportDocPath
	case strings.Contains(suffix, "/comic-snapshots"):
		return comicSnapshotDocPath
	case strings.Contains(suffix, "/storyboard-variants"):
		return storyboardDocPath
	case strings.Contains(suffix, "/image-generations"),
		strings.Contains(suffix, "/comic-image-generation-batches"),
		strings.Contains(suffix, "/setting-generations"),
		strings.Contains(suffix, "/breakdowns"),
		strings.HasSuffix(suffix, "/generations"),
		strings.HasPrefix(suffix, "/chapter-batches"),
		strings.HasPrefix(suffix, "/image-generation-preflights"):
		return generationDocPath
	case strings.HasPrefix(suffix, "/tasks"),
		strings.HasPrefix(suffix, "/production-tasks"),
		strings.HasPrefix(suffix, "/asset-maintenance-tasks"):
		return taskDocPath
	case strings.HasPrefix(suffix, "/workflows"):
		return workflowDocPath
	case strings.HasPrefix(suffix, "/asset-uploads"),
		strings.HasPrefix(suffix, "/assets"),
		strings.HasPrefix(suffix, "/integrity-scans"),
		strings.HasPrefix(suffix, "/asset-reconciliations"),
		strings.HasPrefix(suffix, "/asset-gc-plans"):
		return projectAssetDocPath
	case strings.HasPrefix(suffix, "/premise-assets"):
		return premiseAssetDocPath
	case strings.HasPrefix(suffix, "/premise"):
		return premiseDocPath
	case strings.HasPrefix(suffix, "/story-profile"):
		return storyDocPath
	case strings.HasPrefix(suffix, "/chapter-imports"):
		return chapterDocPath
	case strings.HasPrefix(suffix, "/chapters"):
		sectionSuffix := strings.TrimPrefix(suffix, "/chapters")
		if sectionIndex := strings.Index(sectionSuffix, "/comic-sections"); sectionIndex >= 0 {
			afterCollection := strings.TrimPrefix(sectionSuffix[sectionIndex+len("/comic-sections"):], "/")
			if strings.HasPrefix(afterCollection, "{section_uuid}") {
				return comicSectionDocPath
			}
			return comicDocPath
		}
		if strings.Contains(sectionSuffix, "/comic") {
			return comicDocPath
		}
		return chapterDocPath
	default:
		return projectDocPath
	}
}

func discoveredProjectAPIRouteID(method, path string) string {
	suffix := strings.TrimPrefix(path, "/api/v1/projects/{project_uuid}")
	suffix = strings.Trim(suffix, "/")
	if suffix == "" {
		suffix = "project"
	}
	replacer := strings.NewReplacer("/", ".", "{", "", "}", "", "-", "_")
	return "project_api." + strings.ToLower(method) + "." + replacer.Replace(suffix)
}

func (service *Service) executeDiscoveredProjectAPIRoute(ctx context.Context, request agentAPIRequest) (any, error) {
	if service == nil || service.projectAPIDispatcher == nil {
		return nil, domainError(CodeStateConflict, "项目 API 分发器不可用", "应用尚未装配进程内 Project API dispatcher。", nil)
	}
	response, err := service.projectAPIDispatcher(ctx, ProjectAPIDispatchRequest{
		Method: request.Method, Path: request.Path, Query: request.Query, Body: request.Body, HasBody: request.HasBody,
	})
	if err != nil {
		return nil, err
	}
	if len(response.Body) == 0 {
		return nil, domainError(CodeStateConflict, "项目 API 响应为空", fmt.Sprintf("HTTP %d 没有返回统一 JSON 信封。", response.Status), nil)
	}
	decoder := json.NewDecoder(strings.NewReader(string(response.Body)))
	decoder.UseNumber()
	var envelope struct {
		Success bool `json:"success"`
		Data    any  `json:"data"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details string `json:"details"`
		} `json:"error"`
	}
	if err := decoder.Decode(&envelope); err != nil {
		return nil, domainError(CodeStateConflict, "项目 API 响应无效", "路由没有返回统一 JSON 信封。", err)
	}
	if response.Status < http.StatusOK || response.Status >= http.StatusMultipleChoices || !envelope.Success {
		if envelope.Error != nil {
			code := strings.TrimSpace(envelope.Error.Code)
			if code == "" {
				code = CodeStateConflict
			}
			return nil, domainError(code, envelope.Error.Message, envelope.Error.Details, nil)
		}
		return nil, domainError(CodeStateConflict, "项目 API 调用失败", fmt.Sprintf("HTTP %d 未返回公开错误。", response.Status), nil)
	}
	return envelope.Data, nil
}
