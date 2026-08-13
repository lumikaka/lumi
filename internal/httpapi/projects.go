package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"lumi/internal/project"
	"lumi/internal/promptcatalog"

	"github.com/labstack/echo/v4"
)

type ProjectHandler struct {
	manager *project.Manager
}

func NewProjectHandler(manager *project.Manager) *ProjectHandler {
	return &ProjectHandler{manager: manager}
}

type ProjectDefaultsHandler struct {
	resolveParentPath    func() (string, error)
	resolveOverallStyles func() map[string]string
}

func NewProjectDefaultsHandler() *ProjectDefaultsHandler {
	return &ProjectDefaultsHandler{
		resolveParentPath: project.DefaultNewProjectParentPath,
		resolveOverallStyles: func() map[string]string {
			return map[string]string{
				promptcatalog.LanguageChinese: promptcatalog.DefaultProjectStyle(promptcatalog.LanguageChinese),
				promptcatalog.LanguageEnglish: promptcatalog.DefaultProjectStyle(promptcatalog.LanguageEnglish),
			}
		},
	}
}

type projectDefaultsData struct {
	ParentPath           string            `json:"parent_path"`
	DefaultOverallStyles map[string]string `json:"default_overall_styles"`
}

func (handler *ProjectDefaultsHandler) Show(c echo.Context) error {
	parentPath, err := handler.resolveParentPath()
	if err != nil {
		return projectAPIError(err)
	}
	return Success(c, http.StatusOK, projectDefaultsData{ParentPath: parentPath, DefaultOverallStyles: handler.resolveOverallStyles()})
}

type createProjectRequest struct {
	Name               string                    `json:"name"`
	ParentPath         string                    `json:"parent_path"`
	GenerationLanguage string                    `json:"generation_language"`
	PictureBook        *project.PictureBookInput `json:"picture_book"`
	OverallStyle       string                    `json:"overall_style"`
}

func (handler *ProjectHandler) Create(c echo.Context) error {
	var request createProjectRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	created, err := handler.manager.CreateWithInput(c.Request().Context(), project.CreateInput{Name: request.Name, GenerationLanguage: request.GenerationLanguage, PictureBook: request.PictureBook, OverallStyle: request.OverallStyle}, project.ExplicitNewProjectParent(request.ParentPath))
	if err != nil {
		return projectAPIError(err)
	}
	return Success(c, http.StatusCreated, created)
}

type OpenProjectHandler struct {
	manager *project.Manager
}

func NewOpenProjectHandler(manager *project.Manager) *OpenProjectHandler {
	return &OpenProjectHandler{manager: manager}
}

func (handler *OpenProjectHandler) Index(c echo.Context) error {
	items, err := handler.manager.OpenProjects(c.Request().Context())
	if err != nil {
		return NewError(http.StatusInternalServerError, "open_projects_unavailable", "无法读取已打开项目", "应用数据库查询失败。", err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

type openProjectRequest struct {
	RootPath string `json:"root_path"`
}

func (handler *OpenProjectHandler) Create(c echo.Context) error {
	var request openProjectRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	if strings.TrimSpace(request.RootPath) == "" {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "打开项目参数无效", "root_path 不能为空。", nil)
	}
	opened, err := handler.manager.OpenSelected(c.Request().Context(), project.ExplicitExistingDirectory(request.RootPath))
	if err != nil {
		return projectAPIError(err)
	}
	return Success(c, http.StatusOK, opened)
}

func (handler *OpenProjectHandler) Update(c echo.Context) error {
	opened, err := handler.manager.OpenRecent(c.Request().Context(), c.Param("project_uuid"))
	if err != nil {
		return projectAPIError(err)
	}
	return Success(c, http.StatusOK, opened)
}

func (handler *OpenProjectHandler) Delete(c echo.Context) error {
	if _, err := handler.manager.CloseProject(c.Request().Context(), c.Param("project_uuid")); err != nil {
		var projectErr *project.Error
		if errors.As(err, &projectErr) {
			return projectAPIError(err)
		}
		return NewError(http.StatusInternalServerError, "project_close_failed", "关闭项目失败", "Lumi 无法完整停止项目并释放数据库文件。", err)
	}
	return Success(c, http.StatusOK, nil)
}

type RecentProjectHandler struct {
	manager *project.Manager
}

func NewRecentProjectHandler(manager *project.Manager) *RecentProjectHandler {
	return &RecentProjectHandler{manager: manager}
}

func (handler *RecentProjectHandler) Index(c echo.Context) error {
	items, err := handler.manager.RecentProjects(c.Request().Context())
	if err != nil {
		return NewError(http.StatusInternalServerError, "recent_projects_unavailable", "无法读取最近项目", "应用数据库查询失败。", err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

type relocateProjectRequest struct {
	RootPath string `json:"root_path"`
}

func (handler *RecentProjectHandler) Update(c echo.Context) error {
	var request relocateProjectRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	updated, err := handler.manager.Relocate(c.Request().Context(), c.Param("project_uuid"), project.ExplicitExistingDirectory(request.RootPath))
	if err != nil {
		return projectAPIError(err)
	}
	return Success(c, http.StatusOK, updated)
}

func (handler *RecentProjectHandler) Delete(c echo.Context) error {
	if err := handler.manager.Forget(c.Request().Context(), c.Param("project_uuid")); err != nil {
		return projectAPIError(err)
	}
	return Success(c, http.StatusOK, nil)
}

func decodeJSON(c echo.Context, destination any) error {
	return decodeJSONLimit(c, destination, 1<<20)
}

func decodeJSONLimit(c echo.Context, destination any, limit int64) error {
	if !strings.HasPrefix(strings.ToLower(c.Request().Header.Get(echo.HeaderContentType)), echo.MIMEApplicationJSON) {
		return NewError(http.StatusUnsupportedMediaType, "unsupported_media_type", "请求格式无效", "写请求必须使用 application/json。", nil)
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return NewError(http.StatusBadRequest, "invalid_json", "JSON 请求无效", "请检查字段名与 JSON 语法。", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return NewError(http.StatusBadRequest, "invalid_json", "JSON 请求无效", "请求体只能包含一个 JSON 对象。", err)
	}
	return nil
}

func projectAPIError(err error) error {
	return ProjectAPIError(err)
}

// ProjectAPIError maps project lifecycle and storage failures to the public
// API envelope. It is exported so server-level project request leasing can
// apply the same contract before a domain handler runs.
func ProjectAPIError(err error) error {
	var projectErr *project.Error
	if !errors.As(err, &projectErr) {
		return NewError(http.StatusInternalServerError, "project_operation_failed", "项目操作失败", "发生了未预期的本地存储错误。", err)
	}
	status := http.StatusUnprocessableEntity
	switch projectErr.Code {
	case project.CodeProjectNotFound:
		status = http.StatusNotFound
	case project.CodePermissionDenied:
		status = http.StatusForbidden
	case project.CodeProjectNotOpen, project.CodeIdentityMismatch, project.CodeFormatTooNew, project.CodeProjectDirectoryNameExhausted:
		status = http.StatusConflict
	case project.CodeMigrationFailed:
		status = http.StatusInternalServerError
	case project.CodeDefaultProjectParentUnavailable:
		status = http.StatusInternalServerError
	case project.CodeLocked:
		status = http.StatusLocked
	}
	return NewError(status, projectErr.Code, projectErr.Message, projectErr.Details, err)
}
