package httpapi

import (
	"net/http/httptest"
	"testing"

	"lumi/internal/agentcheckpoint"

	"github.com/labstack/echo/v4"
)

func TestAgentToolDispatchContextIsRouteBoundAndInProcessOnly(t *testing.T) {
	e := echo.New()
	request := httptest.NewRequest("DELETE", "/api/v1/projects/project/chapters/trash", nil)
	request.Header.Set("X-Lumi-Agent-Tool-Execution", "must-not-be-read")
	c := e.NewContext(request, httptest.NewRecorder())
	if got := agentToolExecutionForRoute(c, agentcheckpoint.RouteChapterTrashEmpty); got != "" {
		t.Fatalf("HTTP header injected execution context %q", got)
	}
	SetAgentToolDispatchContext(c, "execution-uuid", agentcheckpoint.RouteChapterTrashEmpty)
	if got := agentToolExecutionForRoute(c, agentcheckpoint.RouteChapterTrashEmpty); got != "execution-uuid" {
		t.Fatalf("execution context=%q", got)
	}
	if got := agentToolExecutionForRoute(c, agentcheckpoint.RoutePremiseAssetTrashEmpty); got != "" {
		t.Fatalf("route-mismatched execution context=%q", got)
	}
}
