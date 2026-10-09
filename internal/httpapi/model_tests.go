package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"lumi/internal/modeltest"
	"lumi/internal/provider"

	"github.com/labstack/echo/v4"
)

type ModelTestHandler struct{ service *modeltest.Service }

func NewModelTestHandler(service *modeltest.Service) *ModelTestHandler {
	return &ModelTestHandler{service: service}
}

func (h *ModelTestHandler) Create(c echo.Context) error {
	var input modeltest.ExecutionInput
	if err := decodeUniqueJSON(c, &input); err != nil {
		return err
	}
	value, err := h.service.Create(c.Request().Context(), input)
	if err != nil {
		return modelTestAPIError(err)
	}
	return Success(c, http.StatusAccepted, value)
}

func (h *ModelTestHandler) Index(c echo.Context) error {
	page, perPage := 1, 20
	var err error
	if raw := c.QueryParam("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil {
			return modelTestAPIError(modeltest.ErrInvalid)
		}
	}
	if raw := c.QueryParam("per_page"); raw != "" {
		perPage, err = strconv.Atoi(raw)
		if err != nil {
			return modelTestAPIError(modeltest.ErrInvalid)
		}
	}
	value, err := h.service.List(c.Request().Context(), page, perPage)
	if err != nil {
		return modelTestAPIError(err)
	}
	return Success(c, http.StatusOK, value)
}

func (h *ModelTestHandler) Show(c echo.Context) error {
	value, err := h.service.Get(c.Request().Context(), c.Param("test_uuid"))
	if err != nil {
		return modelTestAPIError(err)
	}
	return Success(c, http.StatusOK, value)
}

func (h *ModelTestHandler) Execute(c echo.Context) error {
	var input modeltest.ExecutionInput
	if err := decodeUniqueJSON(c, &input); err != nil {
		return err
	}
	value, err := h.service.Execute(c.Request().Context(), c.Param("test_uuid"), input)
	if err != nil {
		return modelTestAPIError(err)
	}
	return Success(c, http.StatusAccepted, value)
}

func (h *ModelTestHandler) Stop(c echo.Context) error {
	value, err := h.service.Stop(c.Request().Context(), c.Param("test_uuid"))
	if err != nil {
		return modelTestAPIError(err)
	}
	return Success(c, http.StatusOK, value)
}

func (h *ModelTestHandler) Content(c echo.Context) error {
	file, step, err := h.service.OpenImage(c.Request().Context(), c.Param("test_uuid"), c.Param("step_uuid"))
	if err != nil {
		return modelTestAPIError(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return modelTestAPIError(err)
	}
	header := c.Response().Header()
	header.Set(echo.HeaderContentType, step.ImageMIMEType)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", "private, max-age=31536000, immutable")
	header.Set("ETag", `"`+step.UUID+`"`)
	http.ServeContent(c.Response(), c.Request(), "model-test-image", info.ModTime(), file)
	return nil
}

func modelTestAPIError(err error) error {
	var providerErr *provider.Error
	if errors.As(err, &providerErr) {
		return providerAPIError(err)
	}
	status, code, message := http.StatusInternalServerError, "model_test_failed", "无法完成模型测试操作。"
	switch {
	case errors.Is(err, modeltest.ErrNotFound):
		status, code, message = http.StatusNotFound, "model_test_not_found", "模型测试记录或图片不存在。"
	case errors.Is(err, modeltest.ErrInvalid):
		status, code, message = http.StatusUnprocessableEntity, "model_test_invalid", "模型选择或分页参数无效。"
	case errors.Is(err, modeltest.ErrConflict):
		status, code, message = http.StatusConflict, "model_test_conflict", "测试已开始，不能更改模型。请新建测试。"
	case errors.Is(err, modeltest.ErrClosed):
		status, code, message = http.StatusServiceUnavailable, "model_test_unavailable", "模型测试服务正在关闭。"
	}
	return NewError(status, code, message, "", err)
}
