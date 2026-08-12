package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"lumi/internal/project"
	"lumi/internal/story"

	"github.com/labstack/echo/v4"
)

type StoryHandler struct {
	manager *project.Manager
}

func NewStoryHandler(manager *project.Manager) *StoryHandler {
	return &StoryHandler{manager: manager}
}

func (handler *StoryHandler) withService(c echo.Context, operation func(*story.Service) error) error {
	err := handler.manager.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		return operation(story.NewService(store))
	})
	if err != nil {
		return storyAPIError(err)
	}
	return nil
}

func storyAPIError(err error) error {
	var projectErr *project.Error
	if errors.As(err, &projectErr) {
		return projectAPIError(err)
	}
	var domainErr *story.Error
	if !errors.As(err, &domainErr) {
		return NewError(http.StatusInternalServerError, "story_operation_failed", "Story 操作失败", "发生了未预期的本地存储错误。", err)
	}
	status := http.StatusUnprocessableEntity
	switch domainErr.Code {
	case story.CodeChapterNotFound, story.CodePromptVersionNotFound:
		status = http.StatusNotFound
	case story.CodeProjectRevisionConflict, story.CodeChapterConflict, story.CodeChapterRevisionConflict,
		story.CodeChapterStateConflict, story.CodeChapterDeleteBlocked, story.CodeStoryProfileConflict, story.CodeStoryMDConflict,
		story.CodePromptRevisionConflict:
		status = http.StatusConflict
	case story.CodeStoryProjectionFailed:
		status = http.StatusInternalServerError
	}
	return NewError(status, domainErr.Code, domainErr.Message, domainErr.Details, err)
}

func requiredRevision(value *int64) (int64, error) {
	if value == nil || *value < 0 {
		return 0, NewError(http.StatusUnprocessableEntity, "validation_failed", "expected_revision 无效", "请求必须提供非负 expected_revision。", nil)
	}
	return *value, nil
}

func queryRevision(c echo.Context) (int64, error) {
	value := strings.TrimSpace(c.QueryParam("expected_revision"))
	if value == "" {
		return 0, NewError(http.StatusUnprocessableEntity, "validation_failed", "缺少 expected_revision", "删除操作必须在 query 中提供当前 revision。", nil)
	}
	revision, err := strconv.ParseInt(value, 10, 64)
	if err != nil || revision < 0 {
		return 0, NewError(http.StatusUnprocessableEntity, "validation_failed", "expected_revision 无效", "expected_revision 必须是非负整数。", err)
	}
	return revision, nil
}

func (handler *StoryHandler) ShowProject(c echo.Context) error {
	var detail story.ProjectDetail
	if err := handler.withService(c, func(service *story.Service) error {
		var err error
		detail, err = service.GetProject(c.Request().Context())
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, detail)
}

type updateStoryProjectRequest struct {
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	GenerationLanguage *string         `json:"generation_language"`
	ExpectedRevision   *int64          `json:"expected_revision"`
	PictureBook        json.RawMessage `json:"picture_book"`
}

func (handler *StoryHandler) UpdateProject(c echo.Context) error {
	var request updateStoryProjectRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	if request.PictureBook != nil {
		return NewError(http.StatusUnprocessableEntity, project.CodePictureBookImmutable, "绘本形式不可修改", "绘本形式与参数在项目创建后保持不变。", nil)
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var detail story.ProjectDetail
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		detail, operationErr = service.UpdateProject(c.Request().Context(), story.UpdateProjectInput{Name: request.Name, Description: request.Description, GenerationLanguage: request.GenerationLanguage, ExpectedRevision: revision})
		return operationErr
	}); err != nil {
		return err
	}
	if err := handler.manager.SyncProjectName(c.Request().Context(), c.Param("project_uuid")); err != nil {
		return NewError(http.StatusInternalServerError, "project_index_update_failed", "项目信息已保存，但最近项目索引更新失败", "重新打开项目时会按项目数据库修复名称。", err)
	}
	return Success(c, http.StatusOK, detail)
}

func (handler *StoryHandler) ListChapters(c echo.Context) error {
	var items []story.Chapter
	if err := handler.withService(c, func(service *story.Service) error {
		var err error
		items, err = service.ListChapters(c.Request().Context(), c.QueryParam("state"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

type createChapterRequest struct {
	ChapterCode   string `json:"chapter_code"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	ContentFormat string `json:"content_format"`
}

func (handler *StoryHandler) CreateChapter(c echo.Context) error {
	var request createChapterRequest
	if err := decodeJSONLimit(c, &request, 3<<20); err != nil {
		return err
	}
	if request.Content != "" && strings.TrimSpace(request.ContentFormat) == "" {
		request.ContentFormat = "txt"
	}
	var created story.Chapter
	if err := handler.withService(c, func(service *story.Service) error {
		var err error
		created, err = service.CreateChapter(c.Request().Context(), story.CreateChapterInput{ChapterCode: request.ChapterCode, Title: request.Title, Content: request.Content, ContentFormat: request.ContentFormat})
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, created)
}

func (handler *StoryHandler) ShowChapter(c echo.Context) error {
	var chapter story.Chapter
	if err := handler.withService(c, func(service *story.Service) error {
		var err error
		chapter, err = service.GetChapter(c.Request().Context(), c.Param("chapter_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, chapter)
}

type updateChapterRequest struct {
	Title            string `json:"title"`
	ExpectedRevision *int64 `json:"expected_revision"`
}

func (handler *StoryHandler) UpdateChapter(c echo.Context) error {
	var request updateChapterRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var chapter story.Chapter
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		chapter, operationErr = service.UpdateChapter(c.Request().Context(), c.Param("chapter_uuid"), story.UpdateChapterInput{Title: request.Title, ExpectedRevision: revision})
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, chapter)
}

func (handler *StoryHandler) CurrentStory(c echo.Context) error {
	var current *story.ChapterStory
	if err := handler.withService(c, func(service *story.Service) error {
		chapter, err := service.GetChapter(c.Request().Context(), c.Param("chapter_uuid"))
		current = chapter.CurrentStory
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, current)
}

type updateChapterStoryRequest struct {
	Content          string `json:"content"`
	ContentFormat    string `json:"content_format"`
	ExpectedRevision *int64 `json:"expected_revision"`
}

func (handler *StoryHandler) UpdateCurrentStory(c echo.Context) error {
	var request updateChapterStoryRequest
	if err := decodeJSONLimit(c, &request, 3<<20); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	if strings.TrimSpace(request.ContentFormat) == "" {
		request.ContentFormat = "txt"
	}
	var chapter story.Chapter
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		chapter, operationErr = service.UpdateStory(c.Request().Context(), c.Param("chapter_uuid"), story.UpdateStoryInput{Content: request.Content, ContentFormat: request.ContentFormat, ExpectedRevision: revision})
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, chapter)
}

func (handler *StoryHandler) ListChapterStories(c echo.Context) error {
	var items []story.ChapterStory
	if err := handler.withService(c, func(service *story.Service) error {
		var err error
		items, err = service.ListChapterStories(c.Request().Context(), c.Param("chapter_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

func (handler *StoryHandler) TrashChapter(c echo.Context) error {
	revision, err := queryRevision(c)
	if err != nil {
		return err
	}
	var chapter story.Chapter
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		chapter, operationErr = service.TrashChapter(c.Request().Context(), c.Param("chapter_uuid"), revision)
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, chapter)
}

type revisionRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
}

func (handler *StoryHandler) RestoreChapter(c echo.Context) error {
	var request revisionRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var chapter story.Chapter
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		chapter, operationErr = service.RestoreChapter(c.Request().Context(), c.Param("chapter_uuid"), revision)
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, chapter)
}

func (handler *StoryHandler) PermanentlyDeleteChapter(c echo.Context) error {
	revision, err := queryRevision(c)
	if err != nil {
		return err
	}
	if err := handler.withService(c, func(service *story.Service) error {
		return service.PermanentlyDeleteChapter(c.Request().Context(), c.Param("chapter_uuid"), revision)
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, nil)
}

func (handler *StoryHandler) EmptyChapterTrash(c echo.Context) error {
	var result story.EmptyChapterTrashResult
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		result, operationErr = service.EmptyChapterTrash(c.Request().Context())
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, result)
}

func readMultipartFile(header *multipart.FileHeader) (story.ImportFile, error) {
	file, err := header.Open()
	if err != nil {
		return story.ImportFile{}, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil {
		return story.ImportFile{}, err
	}
	return story.ImportFile{Filename: header.Filename, Content: string(content)}, nil
}

func (handler *StoryHandler) ImportChapters(c echo.Context) error {
	if !strings.HasPrefix(strings.ToLower(c.Request().Header.Get(echo.HeaderContentType)), "multipart/form-data") {
		return NewError(http.StatusUnsupportedMediaType, "unsupported_media_type", "请求格式无效", "章节导入必须使用 multipart/form-data。", nil)
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 12<<20)
	if err := c.Request().ParseMultipartForm(12 << 20); err != nil {
		return NewError(http.StatusBadRequest, "invalid_multipart", "导入请求无效", "无法读取上传文件或请求超过大小限制。", err)
	}
	mode := strings.TrimSpace(c.FormValue("mode"))
	var headers []*multipart.FileHeader
	if c.Request().MultipartForm != nil {
		headers = append(headers, c.Request().MultipartForm.File["files"]...)
		headers = append(headers, c.Request().MultipartForm.File["files[]"]...)
		headers = append(headers, c.Request().MultipartForm.File["file"]...)
	}
	if mode == "" {
		if len(headers) == 1 {
			mode = "single"
		} else {
			mode = "batch"
		}
	}
	if mode != "single" && mode != "batch" {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "导入模式无效", "mode 只支持 single 或 batch。", nil)
	}
	if mode == "single" && len(headers) != 1 {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "单章导入文件数量无效", "single 模式必须且只能上传一个文件。", nil)
	}
	files := make([]story.ImportFile, 0, len(headers))
	for _, header := range headers {
		item, err := readMultipartFile(header)
		if err != nil {
			return NewError(http.StatusBadRequest, "invalid_upload", "无法读取导入文件", fmt.Sprintf("文件 %s 读取失败。", header.Filename), err)
		}
		if mode == "single" {
			item.ChapterCode = c.FormValue("chapter_code")
			item.Title = c.FormValue("title")
		}
		files = append(files, item)
	}
	var result story.ImportResult
	if err := handler.withService(c, func(service *story.Service) error {
		var err error
		result, err = service.ImportChapters(c.Request().Context(), files)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, result)
}

func (handler *StoryHandler) ShowStoryProfile(c echo.Context) error {
	var profile story.StoryProfile
	if err := handler.withService(c, func(service *story.Service) error {
		var err error
		profile, err = service.GetStoryProfile(c.Request().Context())
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, profile)
}

type updateStoryProfileRequest struct {
	StoryMD          string `json:"story_md"`
	ExpectedRevision *int64 `json:"expected_revision"`
}

func (handler *StoryHandler) UpdateStoryProfile(c echo.Context) error {
	var request updateStoryProfileRequest
	if err := decodeJSONLimit(c, &request, 3<<20); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var profile story.StoryProfile
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		profile, operationErr = service.UpdateStoryProfile(c.Request().Context(), request.StoryMD, revision)
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, profile)
}

func (handler *StoryHandler) ListStoryProfiles(c echo.Context) error {
	var items []story.StoryProfile
	if err := handler.withService(c, func(service *story.Service) error {
		var err error
		items, err = service.ListStoryProfiles(c.Request().Context())
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

func (handler *StoryHandler) ImportExternalStoryMD(c echo.Context) error {
	return handler.storyProfileAction(c, "import")
}

func (handler *StoryHandler) RegenerateStoryMD(c echo.Context) error {
	return handler.storyProfileAction(c, "regenerate")
}

func (handler *StoryHandler) storyProfileAction(c echo.Context, action string) error {
	var request revisionRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	revision, err := requiredRevision(request.ExpectedRevision)
	if err != nil {
		return err
	}
	var profile story.StoryProfile
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		if action == "import" {
			profile, operationErr = service.ImportExternalStoryMD(c.Request().Context(), revision)
		} else {
			profile, operationErr = service.RegenerateStoryMD(c.Request().Context(), revision)
		}
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, profile)
}

func positiveIntQuery(c echo.Context, name string, fallback int) (int, error) {
	value := strings.TrimSpace(c.QueryParam(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, NewError(http.StatusUnprocessableEntity, "validation_failed", name+" 无效", name+" 必须是正整数。", err)
	}
	return parsed, nil
}

func (handler *StoryHandler) ListPromptVersions(c echo.Context) error {
	page, err := positiveIntQuery(c, "page", 1)
	if err != nil {
		return err
	}
	perPage, err := positiveIntQuery(c, "per_page", 20)
	if err != nil {
		return err
	}
	var items []story.PromptVersion
	var pagination story.Pagination
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		items, pagination, operationErr = service.ListPromptVersions(c.Request().Context(), c.QueryParam("prompt_group"), c.QueryParam("prompt_key"), page, perPage)
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items, "pagination": pagination})
}

func (handler *StoryHandler) ListPromptCatalog(c echo.Context) error {
	var items []story.PromptCatalogItem
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		items, operationErr = service.ListPromptCatalog(c.Request().Context(), c.QueryParam("prompt_group"))
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

type createPromptVersionRequest struct {
	PromptGroup            string `json:"prompt_group"`
	PromptKey              string `json:"prompt_key"`
	Prompt                 string `json:"prompt"`
	ExpectedCurrentVersion *int   `json:"expected_current_version"`
}

func (handler *StoryHandler) CreatePromptVersion(c echo.Context) error {
	var request createPromptVersionRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	if request.ExpectedCurrentVersion == nil || *request.ExpectedCurrentVersion < 0 {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "expected_current_version 无效", "请求必须提供非负 expected_current_version。", nil)
	}
	var version story.PromptVersion
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		version, operationErr = service.CreatePromptVersion(c.Request().Context(), story.CreatePromptInput{PromptGroup: request.PromptGroup, PromptKey: request.PromptKey, Prompt: request.Prompt, ExpectedCurrentVersion: *request.ExpectedCurrentVersion})
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, version)
}

type updatePromptGroupRequest struct {
	Prompts                 map[string]string `json:"prompts"`
	ExpectedCurrentVersions map[string]int    `json:"expected_current_versions"`
}

func (handler *StoryHandler) UpdatePromptGroup(c echo.Context) error {
	var request updatePromptGroupRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	var items []story.PromptCatalogItem
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		items, operationErr = service.UpdatePromptGroup(c.Request().Context(), story.UpdatePromptGroupInput{
			PromptGroup:             c.Param("prompt_group"),
			Prompts:                 request.Prompts,
			ExpectedCurrentVersions: request.ExpectedCurrentVersions,
		})
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

type restorePromptVersionRequest struct {
	ExpectedCurrentVersion *int `json:"expected_current_version"`
}

func (handler *StoryHandler) RestorePromptVersion(c echo.Context) error {
	var request restorePromptVersionRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	if request.ExpectedCurrentVersion == nil || *request.ExpectedCurrentVersion < 1 {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "expected_current_version 无效", "恢复历史版本时必须提供当前正整数版本号。", nil)
	}
	var version story.PromptVersion
	if err := handler.withService(c, func(service *story.Service) error {
		var operationErr error
		version, operationErr = service.RestorePromptVersion(c.Request().Context(), c.Param("version_uuid"), *request.ExpectedCurrentVersion)
		return operationErr
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, version)
}
