package textmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
	"lumi/internal/project"
)

const MaxAppBytes = 8 << 20
const MaxAppInputBytes = MaxAppBytes + (64 << 10)
const AppResponseMeta = "lumi/app_response"

type AppResourceRequest struct {
	Method   string
	Resource string
	Query    map[string]string
	Body     json.RawMessage
}

type appRoute struct {
	methods string
	pattern *regexp.Regexp
}

var appResourceRoutes = func() []appRoute {
	u := `[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`
	specs := []struct{ methods, path string }{
		{"GET PATCH", `project`},
		{"GET PUT", `story-profile`},
		{"GET", `story-profile/versions`},
		{"POST", `story-profile/(?:imports|projection|versions/UUID/restorations)`},
		{"GET POST", `chapters`}, {"PUT", `chapter-order`},
		{"GET PATCH DELETE", `chapters/UUID`}, {"POST", `chapters/UUID/restorations`},
		{"GET", `chapters/UUID/stories`}, {"PUT", `chapters/UUID/current-story`},
		{"GET", `chapters/UUID/comic`},
		{"GET POST", `chapters/UUID/comic-sections`},
		{"PUT", `chapters/UUID/comic-section-order`},
		{"GET PATCH DELETE", `chapters/UUID/comic-sections/UUID`},
		{"POST", `chapters/UUID/comic-sections/UUID/restorations`},
		{"PUT", `chapters/UUID/comic-sections/UUID/premise-assets`},
		{"GET POST", `chapters/UUID/comic-sections/UUID/storyboard-variants`},
		{"GET", `chapters/UUID/comic-sections/UUID/image-variants`},
		{"POST", `chapters/UUID/comic-sections/UUID/(?:storyboard-variants|image-variants)/UUID/selections`},
		{"GET PATCH", `premise`}, {"GET POST", `premise-assets`},
		{"GET PATCH DELETE", `premise-assets/UUID`}, {"POST", `premise-assets/UUID/restorations`},
		{"GET", `premise-assets/UUID/variants`}, {"POST", `premise-assets/UUID/variants/UUID/selections`},
		{"GET", `(?:production-tasks|tasks)(?:/UUID)?`},
		{"POST", `(?:production-tasks|tasks)/UUID/cancellations`},
		{"GET", `comic-exports(?:/readiness)?`},
		{"POST", `comic-exports`},
		{"POST", `production-tasks/UUID/retries`},
		{"POST", `asset-uploads`},
		{"POST", `premise-assets/UUID/variants`},
		{"POST", `chapters/UUID/comic-sections/UUID/images`},
	}
	result := []appRoute{}
	for _, s := range specs {
		result = append(result, appRoute{s.methods, regexp.MustCompile(`^(?:` + strings.ReplaceAll(s.path, "UUID", u) + `)$`)})
	}
	return result
}()

// Only these project-relative resources exist in the App transport. There is
// no URL, filesystem path, project selector, arbitrary header or HTTP proxy.
func ValidateAppResource(r AppResourceRequest) error {
	allowed := false
	for _, route := range appResourceRoutes {
		if route.pattern.MatchString(r.Resource) && strings.Contains(" "+route.methods+" ", " "+r.Method+" ") {
			allowed = true
			break
		}
	}
	if !allowed {
		return httpapi.NewError(403, "app_resource_forbidden", "此操作不在嵌入式工作区的范围内", "请在 Lumi 中打开同一资源。", nil)
	}
	keys := map[string]bool{"state": true, "view": true, "limit": true, "before": true, "after": true, "expected_revision": true, "page": true, "per_page": true, "scope": true, "chapter_uuid": true, "task_uuid": true, "snapshot_hash": true, "format": true, "status": true, "tag": true}
	for key, value := range r.Query {
		if !keys[key] || len(value) > 4000 {
			return reviewArguments("不支持的工作区查询参数。")
		}
		if strings.HasSuffix(key, "_uuid") && value != "" && !publicProjectUUID.MatchString(value) {
			return reviewArguments("查询资源必须使用 UUIDv7。")
		}
	}
	if r.Method == http.MethodGet && len(r.Body) > 0 && string(r.Body) != "null" {
		return reviewArguments("读取请求不能包含 body。")
	}
	if len(r.Body) > 0 {
		var body map[string]any
		if err := json.Unmarshal(r.Body, &body); err != nil || body == nil {
			return reviewArguments("操作 body 必须是 JSON 对象。")
		}
		var validate func(any) bool
		validate = func(value any) bool {
			switch v := value.(type) {
			case map[string]any:
				for key, child := range v {
					if key == "id" || strings.HasSuffix(key, "_id") {
						return false
					}
					if !validate(child) {
						return false
					}
				}
			case []any:
				for _, child := range v {
					if !validate(child) {
						return false
					}
				}
			}
			return true
		}
		if !validate(body) {
			return reviewArguments("工作区操作不接受内部数据库 id。")
		}
	}
	return nil
}

func (r AppResourceRequest) Path(projectUUID string) string {
	path := "/api/v1/projects/" + projectUUID
	if r.Resource != "project" {
		path += "/" + r.Resource
	}
	query := url.Values{}
	for k, v := range r.Query {
		query.Set(k, v)
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return path
}

func (s *Service) appResource(ctx context.Context, projectUUID, name string, a arguments) (any, error) {
	r := AppResourceRequest{Method: a.Method, Resource: a.Resource, Query: a.Query, Body: a.Body}
	if name == "read_app_resource" {
		r.Method = http.MethodGet
		ctx = project.WithReadOnly(ctx)
	}
	if err := ValidateAppResource(r); err != nil {
		return nil, err
	}
	if s.AppRequest == nil {
		return nil, httpapi.NewError(503, "app_resource_unavailable", "宿主工作区传输不可用", "请在 Lumi 中打开。", nil)
	}
	return s.AppRequest(ctx, projectUUID, r)
}

// _meta is the UI data channel. Neither textual content nor structuredContent
// duplicates the business object. Visibility alone is not a privacy guarantee;
// host delivery must be verified independently.
func appResourceToolResult(result httpapi.Envelope) (*mcp.CallToolResult, error) {
	summary := httpapi.Envelope{Success: result.Success, Error: result.Error}
	if result.Success {
		summary.Data = map[string]any{"state": "available", "channel": "_meta"}
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		return nil, err
	}
	tool := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}, StructuredContent: summary, Meta: mcp.Meta{AppResponseMeta: result}, IsError: !result.Success}
	wire, err := json.Marshal(tool)
	if err != nil {
		return nil, err
	}
	if len(wire) > MaxAppBytes {
		return boundedResult(Failure("result_too_large", "工作区数据超过传输上限", fmt.Sprintf("请按具体资源读取或缩小分页（最多 %d 字节）。", MaxAppBytes)))
	}
	return tool, nil
}
