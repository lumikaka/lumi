package httpapi

import (
	"strings"

	"github.com/labstack/echo/v4"
)

const agentToolDispatchContextKey = "lumi.internal.agent_tool_dispatch"

type agentToolDispatchContext struct {
	ExecutionUUID string
	RouteID       string
}

// SetAgentToolDispatchContext attaches trusted in-process dispatcher metadata
// to an Echo context. It is never read from an HTTP header, URL, or body.
func SetAgentToolDispatchContext(c echo.Context, executionUUID, routeID string) {
	if c == nil {
		return
	}
	executionUUID = strings.TrimSpace(executionUUID)
	routeID = strings.TrimSpace(routeID)
	if executionUUID == "" || routeID == "" {
		return
	}
	c.Set(agentToolDispatchContextKey, agentToolDispatchContext{ExecutionUUID: executionUUID, RouteID: routeID})
}

func agentToolExecutionForRoute(c echo.Context, routeID string) string {
	if c == nil {
		return ""
	}
	value, ok := c.Get(agentToolDispatchContextKey).(agentToolDispatchContext)
	if !ok || value.RouteID != routeID {
		return ""
	}
	return value.ExecutionUUID
}
