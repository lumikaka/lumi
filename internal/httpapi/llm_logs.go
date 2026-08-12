package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"lumi/internal/llmlog"
	"lumi/internal/project"

	"github.com/labstack/echo/v4"
)

type LLMLogHandler struct {
	manager *project.Manager
}

func NewLLMLogHandler(manager *project.Manager) *LLMLogHandler {
	return &LLMLogHandler{manager: manager}
}

func (handler *LLMLogHandler) Index(c echo.Context) error {
	filter := llmlog.Filter{
		Scope:        strings.ToLower(strings.TrimSpace(c.QueryParam("scope"))),
		ProviderUUID: c.QueryParam("provider_uuid"),
		ProviderType: c.QueryParam("provider_type"),
		Model:        c.QueryParam("model"),
		Scenario:     c.QueryParam("scenario"),
		Status:       c.QueryParam("status"),
		RequestType:  c.QueryParam("request_type"),
		Keyword:      c.QueryParam("keyword"),
	}
	page, err := positiveIntQuery(c, "page", 1)
	if err != nil {
		return err
	}
	perPage, err := positiveIntQuery(c, "per_page", 12)
	if err != nil {
		return err
	}
	var items []llmlog.Log
	var pagination llmlog.Pagination
	var filterGroups llmlog.FilterGroups
	err = handler.manager.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var operationErr error
		items, pagination, filterGroups, operationErr = llmlog.NewService(store).List(c.Request().Context(), filter, page, perPage)
		return operationErr
	})
	if err != nil {
		var projectErr *project.Error
		if errors.As(err, &projectErr) {
			return projectAPIError(err)
		}
		if errors.Is(err, llmlog.ErrInvalidFilter) {
			return NewError(http.StatusUnprocessableEntity, "validation_failed", "LLM Log 筛选无效", "请检查筛选值和长度后重试。", err)
		}
		return NewError(http.StatusInternalServerError, "llm_logs_unavailable", "无法读取 LLM Logs", "项目调用记录查询失败。", err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items, "pagination": pagination, "filter_groups": filterGroups})
}

func (handler *LLMLogHandler) Show(c echo.Context) error {
	var detail llmlog.Detail
	err := handler.manager.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var operationErr error
		detail, operationErr = llmlog.NewService(store).Get(c.Request().Context(), c.Param("log_uuid"))
		return operationErr
	})
	if err != nil {
		var projectErr *project.Error
		if errors.As(err, &projectErr) {
			return projectAPIError(err)
		}
		if errors.Is(err, llmlog.ErrNotFound) {
			return NewError(http.StatusNotFound, "llm_log_not_found", "LLM Log 不存在", "该调用记录不存在或不属于当前项目。", err)
		}
		return NewError(http.StatusInternalServerError, "llm_logs_unavailable", "无法读取 LLM Log", "项目调用记录详情查询失败。", err)
	}
	return Success(c, http.StatusOK, detail)
}
