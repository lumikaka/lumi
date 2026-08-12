package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"lumi/internal/files"
	"lumi/internal/jobqueue"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/provider"

	"github.com/labstack/echo/v4"
)

type ProductionHandler struct {
	projects *project.Manager
	tasks    *jobqueue.Manager
	events   production.EventPublisher
}

func NewProductionHandler(projects *project.Manager, tasks *jobqueue.Manager, events production.EventPublisher) *ProductionHandler {
	return &ProductionHandler{projects: projects, tasks: tasks, events: events}
}
func (handler *ProductionHandler) withService(c echo.Context, operation func(*production.Service) error) error {
	err := handler.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error { return operation(production.NewService(store, handler.events)) })
	if err != nil {
		return productionAPIError(err)
	}
	return nil
}

func productionAPIError(err error) error {
	var projectErr *project.Error
	if errors.As(err, &projectErr) {
		return projectAPIError(err)
	}
	var providerErr *provider.Error
	if errors.As(err, &providerErr) {
		return providerAPIError(err)
	}
	var fileErr *files.Error
	if errors.As(err, &fileErr) {
		return filesAPIError(err)
	}
	var taskErr *jobqueue.Error
	if errors.As(err, &taskErr) {
		return taskAPIError(err)
	}
	var domainErr *production.Error
	if !errors.As(err, &domainErr) {
		return NewError(http.StatusInternalServerError, "production_operation_failed", "生产链操作失败", "发生了未预期的本地存储错误。", err)
	}
	status := http.StatusUnprocessableEntity
	switch domainErr.Code {
	case production.CodeNotFound:
		status = http.StatusNotFound
	case production.CodeConflict, production.CodeStateConflict, production.CodeExportEmpty,
		production.CodeExportIncomplete, production.CodeExportChanged, production.CodeDeleteBlocked, production.CodeSnapshotBusy:
		status = http.StatusConflict
	case production.CodeSnapshotInvalid:
		status = http.StatusInternalServerError
	}
	return NewError(status, domainErr.Code, domainErr.Message, domainErr.Details, err)
}

func (handler *ProductionHandler) ShowPremise(c echo.Context) error {
	var value production.PremiseProfile
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.GetPremise(c.Request().Context())
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, value)
}
func (handler *ProductionHandler) UpdatePremise(c echo.Context) error {
	var request struct {
		DefaultStyle     string `json:"default_style"`
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var value production.PremiseProfile
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.UpdatePremise(c.Request().Context(), production.UpdatePremiseInput{DefaultStyle: request.DefaultStyle, ExpectedRevision: revision})
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, value)
}
func (handler *ProductionHandler) ListPremiseSources(c echo.Context) error {
	page, err := positiveIntQuery(c, "page", 1)
	if err != nil {
		return err
	}
	perPage, err := positiveIntQuery(c, "per_page", 20)
	if err != nil {
		return err
	}
	var items []production.PremiseSource
	var pagination production.Pagination
	if err := handler.withService(c, func(service *production.Service) error {
		var operationErr error
		items, pagination, operationErr = service.ListPremiseSourcesPage(c.Request().Context(), page, perPage)
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items, "pagination": pagination})
}
func (handler *ProductionHandler) CreatePremiseSource(c echo.Context) error {
	var request struct {
		SourceText    string `json:"source_text"`
		StyleSnapshot string `json:"style_snapshot"`
		SourceType    string `json:"source_type"`
		Model         string `json:"model"`
		Parameters    any    `json:"parameters"`
	}
	if err := decodeJSONLimit(c, &request, 1<<20); err != nil {
		return err
	}
	var value production.PremiseSource
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.CreatePremiseSource(c.Request().Context(), production.CreateSourceInput{SourceText: request.SourceText, StyleSnapshot: request.StyleSnapshot, SourceType: request.SourceType, Model: request.Model, Parameters: request.Parameters})
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, value)
}

func (handler *ProductionHandler) UpdatePremiseSource(c echo.Context) error {
	var request struct {
		Ignored          *bool  `json:"ignored"`
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	if request.Ignored == nil || request.ExpectedRevision == nil || *request.ExpectedRevision < 0 {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "批次更新参数无效", "ignored 必须是布尔值，expected_revision 必须是非负整数。", nil)
	}
	var value production.PremiseSource
	if err := handler.withService(c, func(service *production.Service) error {
		var operationErr error
		value, operationErr = service.SetPremiseSourceIgnored(c.Request().Context(), c.Param("source_uuid"), *request.Ignored, *request.ExpectedRevision)
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, value)
}
func (handler *ProductionHandler) ListSettingImages(c echo.Context) error {
	var items []production.SettingImage
	sourceUUIDs := c.QueryParams()["source_uuid"]
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		if len(sourceUUIDs) > 0 {
			items, err = service.ListSettingImagesForSources(c.Request().Context(), sourceUUIDs)
		} else {
			items, err = service.ListSettingImages(c.Request().Context())
		}
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (handler *ProductionHandler) ImportSettingImage(c echo.Context) error {
	var request struct {
		UploadUUID string `json:"upload_uuid"`
		SourceUUID string `json:"source_uuid"`
		Prompt     string `json:"prompt"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	var value production.SettingImage
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.ImportSettingImage(c.Request().Context(), request.UploadUUID, request.SourceUUID, request.Prompt)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, value)
}
func (handler *ProductionHandler) SelectSettingImage(c echo.Context) error {
	var value production.PremiseProfile
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.SelectSettingImage(c.Request().Context(), c.Param("setting_image_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, value)
}
func (handler *ProductionHandler) GenerateSettingImage(c echo.Context) error {
	var request jobqueue.CreateProductionGenerationInput
	if err := decodeJSONLimit(c, &request, 512<<10); err != nil {
		return err
	}
	task, err := handler.tasks.CreatePremiseSettingGeneration(c.Request().Context(), c.Param("project_uuid"), c.Param("source_uuid"), request)
	if err != nil {
		return productionAPIError(err)
	}
	return Success(c, http.StatusCreated, task)
}
func (handler *ProductionHandler) BreakdownSettingImage(c echo.Context) error {
	var request jobqueue.CreateProductionGenerationInput
	if err := decodeJSONLimit(c, &request, 512<<10); err != nil {
		return err
	}
	task, err := handler.tasks.CreatePremiseBreakdown(c.Request().Context(), c.Param("project_uuid"), c.Param("setting_image_uuid"), request)
	if err != nil {
		return productionAPIError(err)
	}
	return Success(c, http.StatusCreated, task)
}

type premiseAssetRequest struct {
	UploadUUID string   `json:"upload_uuid"`
	AssetType  string   `json:"asset_type"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Tags       []string `json:"tags"`
	Position   any      `json:"position"`
	Crop       any      `json:"crop"`
}

func (handler *ProductionHandler) ListPremiseAssets(c echo.Context) error {
	var items []production.PremiseAsset
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		items, err = service.ListPremiseAssets(c.Request().Context(), c.QueryParam("tag"), c.QueryParam("state"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (handler *ProductionHandler) CreatePremiseAsset(c echo.Context) error {
	var request premiseAssetRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	var value production.PremiseAsset
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.ImportPremiseAsset(c.Request().Context(), production.CreateAssetInput{UploadUUID: request.UploadUUID, AssetType: request.AssetType, Title: request.Title, Summary: request.Summary, Tags: request.Tags, Position: request.Position, Crop: request.Crop})
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, value)
}
func (handler *ProductionHandler) ShowPremiseAsset(c echo.Context) error {
	var value production.PremiseAsset
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.GetPremiseAsset(c.Request().Context(), c.Param("premise_asset_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, value)
}
func (handler *ProductionHandler) UpdatePremiseAsset(c echo.Context) error {
	var request struct {
		AssetType        *string   `json:"asset_type"`
		Title            *string   `json:"title"`
		Summary          *string   `json:"summary"`
		Tags             *[]string `json:"tags"`
		Position         any       `json:"position"`
		Crop             any       `json:"crop"`
		ExpectedRevision *int64    `json:"expected_revision"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var value production.PremiseAsset
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.UpdatePremiseAsset(c.Request().Context(), c.Param("premise_asset_uuid"), production.UpdateAssetInput{AssetType: request.AssetType, Title: request.Title, Summary: request.Summary, Tags: request.Tags, Position: request.Position, Crop: request.Crop, ExpectedRevision: revision})
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, value)
}
func (handler *ProductionHandler) TrashPremiseAsset(c echo.Context) error {
	return handler.setAssetTrash(c, true)
}
func (handler *ProductionHandler) RestorePremiseAsset(c echo.Context) error {
	return handler.setAssetTrash(c, false)
}
func (handler *ProductionHandler) PermanentlyDeletePremiseAsset(c echo.Context) error {
	value := strings.TrimSpace(c.QueryParam("expected_revision"))
	revision, err := strconv.ParseInt(value, 10, 64)
	if err != nil || revision < 0 {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "expected_revision 无效", "必须提供当前非负 revision。", err)
	}
	var result production.PremiseTrashDeleteResult
	if err := handler.withService(c, func(service *production.Service) error {
		var operationErr error
		result, operationErr = service.PermanentlyDeletePremiseAsset(c.Request().Context(), c.Param("premise_asset_uuid"), revision)
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, result)
}
func (handler *ProductionHandler) EmptyPremiseAssetTrash(c echo.Context) error {
	var result production.PremiseTrashDeleteResult
	if err := handler.withService(c, func(service *production.Service) error {
		var operationErr error
		result, operationErr = service.EmptyPremiseAssetTrash(c.Request().Context())
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, result)
}
func (handler *ProductionHandler) setAssetTrash(c echo.Context, trashed bool) error {
	var request struct {
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if !trashed {
		if err := decodeJSON(c, &request); err != nil {
			return err
		}
	} else {
		value := strings.TrimSpace(c.QueryParam("expected_revision"))
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return NewError(http.StatusUnprocessableEntity, "validation_failed", "expected_revision 无效", "必须提供当前 revision。", err)
		}
		request.ExpectedRevision = &parsed
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var value production.PremiseAsset
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.SetPremiseAssetTrashed(c.Request().Context(), c.Param("premise_asset_uuid"), trashed, revision)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, value)
}
func (handler *ProductionHandler) ListAssetVariants(c echo.Context) error {
	var items []production.AssetVariant
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		items, err = service.ListAssetVariants(c.Request().Context(), c.Param("premise_asset_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (handler *ProductionHandler) CreateAssetVariant(c echo.Context) error {
	var request struct {
		UploadUUID       string `json:"upload_uuid"`
		Crop             any    `json:"crop"`
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var value production.PremiseAsset
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.ImportPremiseAssetVariant(c.Request().Context(), c.Param("premise_asset_uuid"), request.UploadUUID, request.Crop, revision)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, value)
}
func (handler *ProductionHandler) SelectAssetVariant(c echo.Context) error {
	var request struct {
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var value production.PremiseAsset
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.SelectAssetVariant(c.Request().Context(), c.Param("premise_asset_uuid"), c.Param("variant_uuid"), revision)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, value)
}

func (handler *ProductionHandler) ShowComicState(c echo.Context) error {
	var value production.ComicState
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.GetComicState(c.Request().Context(), c.Param("chapter_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, value)
}
func (handler *ProductionHandler) ListSections(c echo.Context) error {
	var items []production.ComicSection
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		items, err = service.ListSections(c.Request().Context(), c.Param("chapter_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (handler *ProductionHandler) CreateSection(c echo.Context) error {
	var request struct {
		Title         string `json:"title"`
		DescriptionMD string `json:"description_md"`
		StoryboardMD  string `json:"storyboard_md"`
	}
	if err := decodeJSONLimit(c, &request, 1<<20); err != nil {
		return err
	}
	var value production.ComicSection
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.CreateSection(c.Request().Context(), c.Param("chapter_uuid"), production.CreateSectionInput{Title: request.Title, DescriptionMD: request.DescriptionMD, StoryboardMD: request.StoryboardMD})
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, value)
}
func (handler *ProductionHandler) ShowSection(c echo.Context) error {
	var value production.ComicSection
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.GetSection(c.Request().Context(), c.Param("chapter_uuid"), c.Param("section_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, value)
}
func (handler *ProductionHandler) UpdateSection(c echo.Context) error {
	var request struct {
		Title            *string `json:"title"`
		DescriptionMD    *string `json:"description_md"`
		ExpectedRevision *int64  `json:"expected_revision"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var value production.ComicSection
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.UpdateSection(c.Request().Context(), c.Param("chapter_uuid"), c.Param("section_uuid"), production.UpdateSectionInput{Title: request.Title, DescriptionMD: request.DescriptionMD, ExpectedRevision: revision})
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, value)
}
func (handler *ProductionHandler) DeleteSection(c echo.Context) error {
	revision, err := queryRevision(c)
	if err != nil {
		return err
	}
	if err := handler.withService(c, func(service *production.Service) error {
		return service.DeleteSection(c.Request().Context(), c.Param("chapter_uuid"), c.Param("section_uuid"), revision)
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, nil)
}
func (handler *ProductionHandler) ReorderSections(c echo.Context) error {
	var request struct {
		SectionUUIDs []string `json:"section_uuids"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	var items []production.ComicSection
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		items, err = service.ReorderSections(c.Request().Context(), c.Param("chapter_uuid"), request.SectionUUIDs)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (handler *ProductionHandler) ListStoryboards(c echo.Context) error {
	var items []production.StoryboardVariant
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		items, err = service.ListStoryboards(c.Request().Context(), c.Param("chapter_uuid"), c.Param("section_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (handler *ProductionHandler) CreateStoryboard(c echo.Context) error {
	var request struct {
		ContentMD        string `json:"content_md"`
		SourceType       string `json:"source_type"`
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if err := decodeJSONLimit(c, &request, 1<<20); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var value production.ComicSection
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.CreateStoryboard(c.Request().Context(), c.Param("chapter_uuid"), c.Param("section_uuid"), request.ContentMD, request.SourceType, revision)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, value)
}
func (handler *ProductionHandler) SelectStoryboard(c echo.Context) error {
	var request struct {
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var value production.ComicSection
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.SelectStoryboard(c.Request().Context(), c.Param("chapter_uuid"), c.Param("section_uuid"), c.Param("variant_uuid"), revision)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, value)
}
func (handler *ProductionHandler) ImportSectionImage(c echo.Context) error {
	var request struct {
		UploadUUID       string `json:"upload_uuid"`
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var value production.ComicSection
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.ImportSectionImage(c.Request().Context(), c.Param("chapter_uuid"), c.Param("section_uuid"), request.UploadUUID, revision)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, value)
}
func (handler *ProductionHandler) ListImageVariants(c echo.Context) error {
	var items []production.ImageVariant
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		items, err = service.ListImageVariants(c.Request().Context(), c.Param("chapter_uuid"), c.Param("section_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (handler *ProductionHandler) SelectImageVariant(c echo.Context) error {
	var request struct {
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var value production.ComicSection
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.SelectImageVariant(c.Request().Context(), c.Param("chapter_uuid"), c.Param("section_uuid"), c.Param("variant_uuid"), revision)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, value)
}
func (handler *ProductionHandler) GenerateSectionImage(c echo.Context) error {
	var request jobqueue.CreateProductionGenerationInput
	if err := decodeJSONLimit(c, &request, 512<<10); err != nil {
		return err
	}
	task, err := handler.tasks.CreateComicImageGeneration(c.Request().Context(), c.Param("project_uuid"), c.Param("chapter_uuid"), c.Param("section_uuid"), request)
	if err != nil {
		return productionAPIError(err)
	}
	return Success(c, http.StatusCreated, task)
}
func (handler *ProductionHandler) ListSnapshots(c echo.Context) error {
	var items []production.ChapterSnapshot
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		items, err = service.ListChapterSnapshots(c.Request().Context(), c.Param("chapter_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (handler *ProductionHandler) ShowSnapshot(c echo.Context) error {
	var value production.ChapterSnapshotDetail
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		value, err = service.GetChapterSnapshot(c.Request().Context(), c.Param("chapter_uuid"), c.Param("snapshot_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, value)
}
func (handler *ProductionHandler) RestoreSnapshot(c echo.Context) error {
	var items []production.ComicSection
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		items, err = service.RestoreChapterSnapshot(c.Request().Context(), c.Param("chapter_uuid"), c.Param("snapshot_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, map[string]any{"items": items})
}

func (handler *ProductionHandler) ListExports(c echo.Context) error {
	page, err := positiveIntQuery(c, "page", 1)
	if err != nil {
		return err
	}
	perPage, err := positiveIntQuery(c, "per_page", 20)
	if err != nil {
		return err
	}
	var items []production.Export
	var pagination production.Pagination
	if err := handler.withService(c, func(service *production.Service) error {
		var operationErr error
		items, pagination, operationErr = service.ListExportsPage(c.Request().Context(), production.ExportFilter{
			Scope: c.QueryParam("scope"), ChapterUUID: c.QueryParam("chapter_uuid"), TaskUUID: c.QueryParam("task_uuid"),
			SnapshotHash: c.QueryParam("snapshot_hash"), Status: c.QueryParam("status"),
		}, page, perPage)
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items, "pagination": pagination})
}
func (handler *ProductionHandler) ExportReadiness(c echo.Context) error {
	var readiness production.ExportReadiness
	if err := handler.withService(c, func(service *production.Service) error {
		var err error
		readiness, err = service.ExportReadiness(c.Request().Context(), c.QueryParam("scope"), c.QueryParam("chapter_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, readiness)
}
func (handler *ProductionHandler) CreateExport(c echo.Context) error {
	var request jobqueue.CreateExportInput
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	operation, err := handler.tasks.CreateComicExport(c.Request().Context(), c.Param("project_uuid"), request)
	if err != nil {
		return productionAPIError(err)
	}
	return Success(c, http.StatusCreated, operation)
}
func (handler *ProductionHandler) ListProductionTasks(c echo.Context) error {
	limit, err := positiveIntQuery(c, "limit", 50)
	if err != nil {
		return err
	}
	items, err := handler.tasks.ListProductionTasks(c.Request().Context(), c.Param("project_uuid"), c.QueryParam("status"), limit)
	if err != nil {
		return productionAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (handler *ProductionHandler) ShowProductionTask(c echo.Context) error {
	task, err := handler.tasks.GetProductionTask(c.Request().Context(), c.Param("project_uuid"), c.Param("task_uuid"))
	if err != nil {
		return productionAPIError(err)
	}
	return Success(c, http.StatusOK, task)
}
func (handler *ProductionHandler) ProductionTaskEvents(c echo.Context) error {
	limit, err := positiveIntQuery(c, "limit", 50)
	if err != nil {
		return err
	}
	parseCursor := func(name string) (int64, error) {
		value := strings.TrimSpace(c.QueryParam(name))
		if value == "" {
			return 0, nil
		}
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil || parsed < 0 {
			return 0, NewError(http.StatusUnprocessableEntity, "validation_failed", name+" 无效", name+" 必须是非负事件 sequence。", parseErr)
		}
		return parsed, nil
	}
	before, err := parseCursor("before")
	if err != nil {
		return err
	}
	after, err := parseCursor("after")
	if err != nil {
		return err
	}
	items, pagination, err := handler.tasks.ListProductionTaskEvents(c.Request().Context(), c.Param("project_uuid"), c.Param("task_uuid"), before, after, limit)
	if err != nil {
		return productionAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items, "cursor_pagination": pagination})
}
func (handler *ProductionHandler) CancelProductionTask(c echo.Context) error {
	task, err := handler.tasks.CancelProductionTask(c.Request().Context(), c.Param("project_uuid"), c.Param("task_uuid"))
	if err != nil {
		return productionAPIError(err)
	}
	return Success(c, http.StatusOK, task)
}
func (handler *ProductionHandler) RetryProductionTask(c echo.Context) error {
	task, err := handler.tasks.RetryProductionTask(c.Request().Context(), c.Param("project_uuid"), c.Param("task_uuid"))
	if err != nil {
		return productionAPIError(err)
	}
	return Success(c, http.StatusOK, task)
}
