package httpapi

import (
	"errors"
	"net/http"

	"lumi/internal/projectcreation"

	"github.com/labstack/echo/v4"
)

type ProjectCreationSessionHandler struct{ service *projectcreation.Service }

func NewProjectCreationSessionHandler(service *projectcreation.Service) *ProjectCreationSessionHandler {
	return &ProjectCreationSessionHandler{service: service}
}

type createProjectCreationSessionRequest struct {
	InputText      string `json:"input_text"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (handler *ProjectCreationSessionHandler) Create(c echo.Context) error {
	var request createProjectCreationSessionRequest
	if err := decodeUniqueJSON(c, &request); err != nil {
		return err
	}
	session, err := handler.service.Create(c.Request().Context(), request.InputText, request.IdempotencyKey)
	if err != nil {
		return projectCreationSessionError(err)
	}
	return Success(c, http.StatusCreated, session)
}

func (handler *ProjectCreationSessionHandler) Show(c echo.Context) error {
	session, err := handler.service.Get(c.Request().Context(), c.Param("session_uuid"))
	if err != nil {
		return projectCreationSessionError(err)
	}
	return Success(c, http.StatusOK, session)
}

func (handler *ProjectCreationSessionHandler) Retry(c echo.Context) error {
	var request struct{}
	if err := decodeUniqueJSON(c, &request); err != nil {
		return err
	}
	session, err := handler.service.Resume(c.Request().Context(), c.Param("session_uuid"))
	if err != nil {
		return projectCreationSessionError(err)
	}
	return Success(c, http.StatusOK, session)
}

func projectCreationSessionError(err error) error {
	var creationErr *projectcreation.Error
	if !errors.As(err, &creationErr) {
		return NewError(http.StatusInternalServerError, "project_creation_failed", "项目创建失败", "创建会话无法继续。", err)
	}
	status := http.StatusUnprocessableEntity
	switch creationErr.Code {
	case projectcreation.CodeNotFound:
		status = http.StatusNotFound
	case projectcreation.CodeIdempotencyConflict:
		status = http.StatusConflict
	}
	return NewError(status, creationErr.Code, creationErr.Message, creationErr.Details, err)
}
