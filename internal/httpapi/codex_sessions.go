package httpapi

import (
	"errors"
	"strconv"

	"github.com/labstack/echo/v4"
	"lumi/internal/sessionimport"
)

func RegisterCodexSessionHistory(api *echo.Group, service *sessionimport.Service) {
	api.GET("/projects/:project_uuid/chat_threads/:thread_uuid/codex_session_items", func(c echo.Context) error {
		limit := 50
		if raw := c.QueryParam("limit"); raw != "" {
			var err error
			limit, err = strconv.Atoi(raw)
			if err != nil {
				return NewError(422, "validation_failed", "分页参数无效", "", err)
			}
		}
		page, err := service.History(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), c.QueryParam("before"), c.QueryParam("after"), limit)
		if errors.Is(err, sessionimport.ErrNotFound) {
			return NewError(404, "codex_session_not_found", "Codex 会话记录不存在", "", nil)
		}
		if errors.Is(err, sessionimport.ErrCursor) {
			return NewError(422, "validation_failed", "分页参数无效", "", nil)
		}
		if err != nil {
			return ProjectAPIError(err)
		}
		return Success(c, 200, page)
	})
}
