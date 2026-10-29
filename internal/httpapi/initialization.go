package httpapi

import (
	"net/http"

	"lumi/internal/project"
	"lumi/internal/realtime"

	"github.com/labstack/echo/v4"
)

type InitializationHandler struct {
	projects *project.Manager
	hub      *realtime.Hub
}

func NewInitializationHandler(projects *project.Manager, hub *realtime.Hub) *InitializationHandler {
	return &InitializationHandler{projects: projects, hub: hub}
}

func (handler *InitializationHandler) Show(c echo.Context) error {
	var state project.InitializationState
	err := handler.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var err error
		state, err = store.InitializationState(c.Request().Context())
		return err
	})
	if err != nil {
		return ProjectAPIError(err)
	}
	return Success(c, http.StatusOK, state)
}

func (handler *InitializationHandler) TakeOver(c echo.Context) error {
	var request struct {
		WorkflowUUID string `json:"workflow_uuid"`
	}
	if err := decodeUniqueJSON(c, &request); err != nil {
		return err
	}
	var state project.InitializationState
	projectUUID := c.Param("project_uuid")
	err := handler.projects.WithStore(c.Request().Context(), projectUUID, func(store *project.Store) error {
		var err error
		state, err = store.TakeOverInitialization(c.Request().Context(), request.WorkflowUUID)
		return err
	})
	if err != nil {
		return ProjectAPIError(err)
	}
	if handler.hub != nil {
		handler.hub.Broadcast(realtime.ProjectTopic(projectUUID), "project:initialization_changed", map[string]any{
			"project_uuid": projectUUID, "workflow_uuid": state.WorkflowUUID,
		})
	}
	return Success(c, http.StatusOK, state)
}
