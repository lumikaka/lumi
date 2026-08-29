package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"lumi/internal/files"
	"lumi/internal/projectcreation"

	"github.com/labstack/echo/v4"
)

type ProjectCreationSessionHandler struct{ service *projectcreation.Service }

func NewProjectCreationSessionHandler(service *projectcreation.Service) *ProjectCreationSessionHandler {
	return &ProjectCreationSessionHandler{service: service}
}

type createProjectCreationSessionRequest struct {
	InputText      string                               `json:"input_text"`
	IdempotencyKey string                               `json:"idempotency_key"`
	ReferenceFiles []projectcreation.ReferenceFileInput `json:"reference_files"`
}

func (handler *ProjectCreationSessionHandler) Create(c echo.Context) error {
	var request createProjectCreationSessionRequest
	if err := decodeUniqueJSON(c, &request); err != nil {
		return err
	}
	session, err := handler.service.CreateWithReferences(c.Request().Context(), request.InputText, request.IdempotencyKey, request.ReferenceFiles)
	if err != nil {
		return projectCreationSessionError(err)
	}
	return Success(c, http.StatusCreated, session)
}

func (handler *ProjectCreationSessionHandler) UploadReference(c echo.Context) error {
	if !strings.HasPrefix(strings.ToLower(c.Request().Header.Get(echo.HeaderContentType)), "multipart/form-data") {
		return NewError(http.StatusUnsupportedMediaType, "unsupported_media_type", "请求格式无效", "参考图上传必须使用 multipart/form-data。", nil)
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, (33 << 20))
	reader, err := c.Request().MultipartReader()
	if err != nil {
		return NewError(http.StatusBadRequest, "invalid_multipart", "参考图上传请求无效", "无法读取 multipart 边界。", err)
	}
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return NewError(http.StatusBadRequest, "invalid_multipart", "参考图上传请求无效", "无法读取 multipart 内容。", nextErr)
		}
		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			continue
		}
		session, uploadErr := handler.service.UploadReference(c.Request().Context(), c.Param("session_uuid"), c.Param("reference_uuid"), part)
		_ = part.Close()
		if uploadErr != nil {
			var fileErr *files.Error
			if errors.As(uploadErr, &fileErr) {
				return filesAPIError(uploadErr)
			}
			return projectCreationSessionError(uploadErr)
		}
		return Success(c, http.StatusCreated, session)
	}
	return NewError(http.StatusUnprocessableEntity, "validation_failed", "缺少参考图", "multipart 请求必须包含 file。", nil)
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
	case projectcreation.CodeNotFound, projectcreation.CodeReferenceNotFound:
		status = http.StatusNotFound
	case projectcreation.CodeIdempotencyConflict:
		status = http.StatusConflict
	case projectcreation.CodeReferenceNotReady:
		status = http.StatusConflict
	}
	return NewError(status, creationErr.Code, creationErr.Message, creationErr.Details, err)
}
