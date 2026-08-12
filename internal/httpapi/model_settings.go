package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	"lumi/internal/modelsettings"
	"lumi/internal/project"

	"github.com/labstack/echo/v4"
)

type ModelSettingsPublisher interface {
	Broadcast(topic, event string, payload any)
}

type ModelSettingsHandler struct {
	projects *project.Manager
	models   *modelsettings.Resolver
	events   ModelSettingsPublisher
}

func NewModelSettingsHandler(projects *project.Manager, models *modelsettings.Resolver, events ModelSettingsPublisher) *ModelSettingsHandler {
	return &ModelSettingsHandler{projects: projects, models: models, events: events}
}

func (handler *ModelSettingsHandler) Show(c echo.Context) error {
	var value modelsettings.View
	err := handler.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var operationErr error
		value, operationErr = handler.models.Get(c.Request().Context(), store)
		return operationErr
	})
	if err != nil {
		return modelSettingsAPIError(err)
	}
	return Success(c, http.StatusOK, value)
}

func (handler *ModelSettingsHandler) Update(c echo.Context) error {
	var request struct {
		ExpectedRevision *int                       `json:"expected_revision"`
		Overrides        map[string]json.RawMessage `json:"overrides"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	if request.ExpectedRevision == nil || *request.ExpectedRevision < 0 || len(request.Overrides) == 0 {
		return NewError(http.StatusUnprocessableEntity, modelsettings.CodeInvalid, "模型设置更新无效", "必须提供 expected_revision 和至少一个 override。", nil)
	}
	changes := make(map[string]*modelsettings.Selection, len(request.Overrides))
	for key, raw := range request.Overrides {
		if !modelsettings.ValidSettingKey(key) {
			return NewError(http.StatusUnprocessableEntity, modelsettings.CodeInvalid, "模型设置项无效", "不支持设置项 "+key+"。", nil)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			changes[key] = nil
			continue
		}
		var selection modelsettings.Selection
		if err := json.Unmarshal(raw, &selection); err != nil {
			return NewError(http.StatusUnprocessableEntity, modelsettings.CodeInvalid, "模型设置值无效", "override 必须是 provider_uuid/model 对象或 null。", err)
		}
		changes[key] = &selection
	}
	var value modelsettings.View
	err := handler.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var operationErr error
		value, operationErr = handler.models.Patch(c.Request().Context(), store, modelsettings.PatchInput{ExpectedRevision: *request.ExpectedRevision, Changes: changes})
		return operationErr
	})
	if err != nil {
		return modelSettingsAPIError(err)
	}
	if handler.events != nil {
		handler.events.Broadcast("project:"+c.Param("project_uuid"), "project:model_settings_changed", map[string]any{"project_uuid": c.Param("project_uuid"), "revision": value.Revision})
	}
	return Success(c, http.StatusOK, value)
}

func modelSettingsAPIError(err error) error {
	var projectErr *project.Error
	if errors.As(err, &projectErr) {
		return projectAPIError(err)
	}
	var domainErr *modelsettings.Error
	if !errors.As(err, &domainErr) {
		return NewError(http.StatusInternalServerError, "project_model_settings_failed", "项目模型设置操作失败", "发生了未预期的本地存储错误。", err)
	}
	status := http.StatusUnprocessableEntity
	if domainErr.Code == modelsettings.CodeConflict {
		status = http.StatusConflict
	}
	if domainErr.Code == modelsettings.CodeNoModel {
		status = http.StatusServiceUnavailable
	}
	return NewError(status, domainErr.Code, domainErr.Message, domainErr.Details, err)
}
