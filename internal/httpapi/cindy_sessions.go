package httpapi

import (
	"errors"
	"strconv"

	"github.com/labstack/echo/v4"
	"lumi/internal/cindyarchive"
)

func CindyArchiveError(err error) error {
	var archiveError *cindyarchive.Error
	if errors.As(err, &archiveError) {
		status := 422
		switch archiveError.Code {
		case "cindy_archive_not_found":
			status = 404
		case "cindy_archive_source_changed", "cindy_archive_source_missing":
			status = 409
		case "cindy_archive_storage_failed":
			status = 500
		}
		return NewError(status, archiveError.Code, "Cindy 记录操作未完成", "请检查所选来源、文件格式和覆盖范围；原有记录保持不变。", nil)
	}
	return ProjectAPIError(err)
}

func RegisterCindySessionHistory(api *echo.Group, service *cindyarchive.Service) {
	api.GET("/projects/:project_uuid/chat_threads/:thread_uuid/cindy_session_items", func(c echo.Context) error {
		limit := 50
		if raw := c.QueryParam("limit"); raw != "" {
			var err error
			limit, err = strconv.Atoi(raw)
			if err != nil {
				return NewError(422, "validation_failed", "分页参数无效", "", nil)
			}
		}
		page, err := service.History(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), c.QueryParam("before"), c.QueryParam("after"), limit)
		if err != nil {
			return CindyArchiveError(err)
		}
		return Success(c, 200, page)
	})
}
