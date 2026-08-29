package httpapi

import (
	"net/http"

	"lumi/internal/project"
	"lumi/internal/realtime"
	"lumi/internal/story"

	"github.com/labstack/echo/v4"
)

type ProjectSetupHandler struct {
	projects *project.Manager
	hub      *realtime.Hub
}

func NewProjectSetupHandler(projects *project.Manager, hub *realtime.Hub) *ProjectSetupHandler {
	return &ProjectSetupHandler{projects: projects, hub: hub}
}

func (handler *ProjectSetupHandler) Show(c echo.Context) error {
	var state project.SetupState
	err := handler.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var err error
		state, err = store.ProjectSetup(c.Request().Context())
		return err
	})
	if err != nil {
		return ProjectAPIError(err)
	}
	return Success(c, http.StatusOK, state)
}

type updateProjectSetupDraftRequest struct {
	ExpectedRevision   int64                     `json:"expected_revision"`
	ProjectName        *string                   `json:"project_name"`
	GenerationLanguage *string                   `json:"generation_language"`
	OverallStyle       *string                   `json:"overall_style"`
	PictureBook        *project.PictureBookInput `json:"picture_book"`
}

func (handler *ProjectSetupHandler) Update(c echo.Context) error {
	var request updateProjectSetupDraftRequest
	if err := decodeUniqueJSON(c, &request); err != nil {
		return err
	}
	var state project.SetupState
	err := handler.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var err error
		state, err = store.UpdateProjectSetupDraft(c.Request().Context(), project.SetupDraftPatchInput{
			ExpectedRevision: request.ExpectedRevision, ProjectName: request.ProjectName,
			GenerationLanguage: request.GenerationLanguage, OverallStyle: request.OverallStyle,
			PictureBook: request.PictureBook,
		})
		return err
	})
	if err != nil {
		return ProjectAPIError(err)
	}
	handler.broadcast(state)
	return Success(c, http.StatusOK, state)
}

type finalizeProjectSetupRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

func (handler *ProjectSetupHandler) Finalize(c echo.Context) error {
	var request finalizeProjectSetupRequest
	if err := decodeUniqueJSON(c, &request); err != nil {
		return err
	}
	var state project.SetupState
	err := handler.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var err error
		state, err = store.FinalizeProjectSetup(c.Request().Context(), request.ExpectedRevision)
		if err != nil {
			return err
		}
		return story.NewService(store).EnsurePromptCatalogVersions(c.Request().Context(), "project_created")
	})
	if err != nil {
		return ProjectAPIError(err)
	}
	if err := handler.projects.SyncProjectName(c.Request().Context(), c.Param("project_uuid")); err != nil {
		return ProjectAPIError(err)
	}
	handler.broadcast(state)
	return Success(c, http.StatusOK, state)
}

func (handler *ProjectSetupHandler) broadcast(state project.SetupState) {
	if handler.hub == nil {
		return
	}
	handler.hub.Broadcast(realtime.ProjectTopic(state.ProjectUUID), "project:setup_changed", projectSetupChangedPayload(state))
}

func projectSetupChangedPayload(state project.SetupState) map[string]any {
	return map[string]any{
		"project_uuid": state.ProjectUUID, "setup_uuid": state.UUID, "status": state.Status,
		"setup_status": state.SetupStatus, "revision": state.Revision,
	}
}
