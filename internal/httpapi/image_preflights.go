package httpapi

import (
	"errors"
	"net/http"

	"lumi/internal/modelsettings"
	"lumi/internal/picturebook"
	"lumi/internal/project"
	"lumi/internal/provider"

	"github.com/labstack/echo/v4"
)

type ImageGenerationPreflightHandler struct {
	providers *provider.Service
}

func NewImageGenerationPreflightHandler(providers *provider.Service) *ImageGenerationPreflightHandler {
	return &ImageGenerationPreflightHandler{providers: providers}
}

func imageAspectUnsupportedError(err error, details string) error {
	return NewError(http.StatusUnprocessableEntity, picturebook.CodeAspectRatioUnsupported, "图片模型不支持所选比例", details, err)
}

func (handler *ImageGenerationPreflightHandler) Create(c echo.Context) error {
	var request struct {
		PictureBook *project.PictureBookInput `json:"picture_book"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	profile, err := project.NormalizePictureBookInput(request.PictureBook)
	if err != nil {
		return projectAPIError(err)
	}
	resolved, err := handler.providers.Active(c.Request().Context())
	if err != nil {
		return providerAPIError(err)
	}
	size, err := picturebook.ResolveImageSize(profile, resolved.ProviderType, resolved.DefaultImageModel)
	if err != nil {
		var unsupported *picturebook.UnsupportedError
		if errors.As(err, &unsupported) {
			return imageAspectUnsupportedError(err, "请切换到支持该精确比例的图片模型；系统不会自动裁剪或改用近似比例。")
		}
		return NewError(http.StatusInternalServerError, "image_preflight_failed", "图片生成预检失败", "无法解析图片模型能力。", err)
	}
	return Success(c, http.StatusOK, map[string]any{
		"picture_book": profile, "provider_uuid": resolved.UUID, "provider_type": resolved.ProviderType,
		"model": resolved.DefaultImageModel, "output_size": map[string]any{"width": size.Width, "height": size.Height, "value": size.String()},
	})
}

// ProjectImageGenerationPreflightHandler checks the effective project image
// model, including a project-level override, against the immutable profile.
type ProjectImageGenerationPreflightHandler struct {
	projects *project.Manager
	models   *modelsettings.Resolver
}

func NewProjectImageGenerationPreflightHandler(projects *project.Manager, models *modelsettings.Resolver) *ProjectImageGenerationPreflightHandler {
	return &ProjectImageGenerationPreflightHandler{projects: projects, models: models}
}

func (handler *ProjectImageGenerationPreflightHandler) Create(c echo.Context) error {
	var response map[string]any
	err := handler.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		resolved, err := handler.models.Resolve(c.Request().Context(), store, modelsettings.ProjectImage, modelsettings.KindImage, "", "")
		if err != nil {
			return err
		}
		profile, err := store.RequirePictureBookProfile()
		if err != nil {
			return err
		}
		size, err := picturebook.ResolveImageSize(profile, resolved.Provider.ProviderType, resolved.Model)
		if err != nil {
			return err
		}
		response = map[string]any{
			"picture_book": profile, "provider_uuid": resolved.Provider.UUID, "provider_type": resolved.Provider.ProviderType,
			"model": resolved.Model, "model_source": resolved.Source,
			"output_size": map[string]any{"width": size.Width, "height": size.Height, "value": size.String()},
		}
		return nil
	})
	if err != nil {
		var unsupported *picturebook.UnsupportedError
		if errors.As(err, &unsupported) {
			return imageAspectUnsupportedError(err, "当前项目图片模型无法精确生成目标比例；请在项目模型设置中切换模型。")
		}
		return modelSettingsAPIError(err)
	}
	return Success(c, http.StatusOK, response)
}
