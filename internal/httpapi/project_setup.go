package httpapi

import (
	"errors"
	"net/http"

	"lumi/internal/modelsettings"
	"lumi/internal/picturebook"
	"lumi/internal/project"
	"lumi/internal/realtime"
	"lumi/internal/story"

	"github.com/labstack/echo/v4"
)

type ProjectSetupHandler struct {
	projects *project.Manager
	models   *modelsettings.Resolver
	hub      *realtime.Hub
}

func NewProjectSetupHandler(projects *project.Manager, models *modelsettings.Resolver, hub *realtime.Hub) *ProjectSetupHandler {
	return &ProjectSetupHandler{projects: projects, models: models, hub: hub}
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
	GenerationBrief    *string                   `json:"generation_brief"`
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
			GenerationBrief: request.GenerationBrief,
			PictureBook:     request.PictureBook,
		})
		return err
	})
	if err != nil {
		return ProjectAPIError(err)
	}
	handler.broadcast(state)
	return Success(c, http.StatusOK, state)
}

type updateProjectSetupReferenceRequest struct {
	ExpectedRevision int64   `json:"expected_revision"`
	ReferenceRole    *string `json:"reference_role"`
	Title            *string `json:"title"`
	Instruction      *string `json:"instruction"`
	IncludeInYolo    *bool   `json:"include_in_yolo"`
}

func (handler *ProjectSetupHandler) UpdateReference(c echo.Context) error {
	var request updateProjectSetupReferenceRequest
	if err := decodeUniqueJSON(c, &request); err != nil {
		return err
	}
	var state project.SetupState
	err := handler.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var err error
		state, err = store.UpdateProjectSetupReference(c.Request().Context(), c.Param("reference_uuid"), project.SetupReferencePatchInput{
			ExpectedRevision: request.ExpectedRevision, ReferenceRole: request.ReferenceRole, Title: request.Title,
			Instruction: request.Instruction, IncludeInYolo: request.IncludeInYolo, Source: project.SetupSourceUserConfirmed,
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
		draft, err := store.ProjectSetup(c.Request().Context())
		if err != nil {
			return err
		}
		// Preserve revision/incomplete-draft errors and idempotent finalization.
		// The store rechecks the revision before writing the immutable profile.
		if draft.SetupStatus == project.SetupStatusDraft && draft.Revision == request.ExpectedRevision && len(draft.MissingInformation) == 0 && draft.DraftValues.PictureBook != nil {
			resolved, err := handler.models.Resolve(c.Request().Context(), store, modelsettings.ProjectImage, modelsettings.KindImage, "", "")
			if err != nil {
				return modelSettingsAPIError(err)
			}
			if _, err := picturebook.ResolveImageSize(*draft.DraftValues.PictureBook, resolved.Provider.ProviderType, resolved.Model); err != nil {
				return imageAspectUnsupportedError(err, "请切换图片模型或调整草稿比例后重新定稿；项目设置尚未定稿。")
			}
		}
		state, err = store.FinalizeProjectSetup(c.Request().Context(), request.ExpectedRevision)
		if err != nil {
			return err
		}
		return story.NewService(store).EnsurePromptCatalogVersions(c.Request().Context(), "project_created")
	})
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			return apiErr
		}
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
