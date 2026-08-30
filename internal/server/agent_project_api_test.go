package server

import (
	"context"
	"net/http"
	"testing"

	"lumi/internal/agent"
	"lumi/internal/httpapi"

	"github.com/labstack/echo/v4"
)

func TestEchoProjectAPIDispatcherUsesRegisteredRoute(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = httpapi.ErrorHandler
	api := e.Group("/api/v1")
	checkpointContextSeen := false
	api.PATCH("/projects/:project_uuid/prompt-groups/:prompt_group", func(c echo.Context) error {
		checkpointContextSeen = c.Get("lumi.internal.agent_tool_dispatch") != nil
		var body map[string]any
		if err := c.Bind(&body); err != nil {
			return err
		}
		return httpapi.Success(c, http.StatusOK, map[string]any{
			"project_uuid": c.Param("project_uuid"), "prompt_group": c.Param("prompt_group"),
			"enabled": body["enabled"], "tags": c.QueryParams()["tag"],
		})
	})

	specs := projectAPIRouteSpecs(e)
	if len(specs) != 1 || specs[0].Method != http.MethodPatch || specs[0].Path != "/api/v1/projects/:project_uuid/prompt-groups/:prompt_group" {
		t.Fatalf("specs=%+v", specs)
	}
	dispatch := echoProjectAPIDispatcher(e)
	response, err := dispatch(context.Background(), agent.ProjectAPIDispatchRequest{
		Method: http.MethodPatch,
		Path:   "/api/v1/projects/01989abc-def0-7000-8000-000000000001/prompt-groups/story-profile",
		Query:  map[string]any{"tag": []any{"one", "two"}}, Body: map[string]any{"enabled": true}, HasBody: true,
		ToolExecutionUUID: "01989abc-def0-7000-8000-000000000002", RouteID: "test.route",
	})
	if err != nil || response.Status != http.StatusOK || len(response.Body) == 0 {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	if !checkpointContextSeen {
		t.Fatal("in-process Agent tool context was not attached to the Echo dispatch")
	}
}
