package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"lumi/internal/files"
	"lumi/internal/jobqueue"
	"lumi/internal/project"
	"lumi/internal/realtime"

	"github.com/labstack/echo/v4"
)

type FilesHandler struct {
	manager *project.Manager
	hub     *realtime.Hub
	tasks   *jobqueue.Manager
}

func NewFilesHandler(manager *project.Manager, hub *realtime.Hub, taskManagers ...*jobqueue.Manager) *FilesHandler {
	handler := &FilesHandler{manager: manager, hub: hub}
	if len(taskManagers) > 0 {
		handler.tasks = taskManagers[0]
	}
	return handler
}

func (handler *FilesHandler) withService(c echo.Context, operation func(*files.Service) error) error {
	err := handler.manager.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var publisher files.EventPublisher
		if handler.hub != nil {
			publisher = handler.hub
		}
		return operation(files.NewService(store, publisher))
	})
	if err != nil {
		return filesAPIError(err)
	}
	return nil
}

func filesAPIError(err error) error {
	var projectErr *project.Error
	if errors.As(err, &projectErr) {
		return projectAPIError(err)
	}
	var domainErr *files.Error
	if !errors.As(err, &domainErr) {
		return NewError(http.StatusInternalServerError, "asset_operation_failed", "Asset 操作失败", "发生了未预期的本地存储错误。", err)
	}
	status := http.StatusUnprocessableEntity
	switch domainErr.Code {
	case files.CodeUploadNotFound, files.CodeAssetNotFound, files.CodeScanNotFound, files.CodeGCPlanNotFound:
		status = http.StatusNotFound
	case files.CodePurposeMismatch, files.CodeTypeNotAllowed, files.CodeFileTooLarge, files.CodePixelsTooLarge, files.CodeInvalidContent, files.CodeValidationFailed:
		status = http.StatusUnprocessableEntity
	case files.CodeUploadExpired, files.CodeUploadNotReady, files.CodeUploadConsumed, files.CodeInvalidState, files.CodeReferenced, files.CodeGCPlanStale:
		status = http.StatusConflict
	case files.CodeObjectUnavailable:
		status = http.StatusGone
	case files.CodeUnsafePath, files.CodeActorMismatch:
		status = http.StatusForbidden
	case files.CodeOperationUnavailable:
		status = http.StatusServiceUnavailable
	}
	return NewError(status, domainErr.Code, domainErr.Message, domainErr.Details, err)
}

func (handler *FilesHandler) CreateUpload(c echo.Context) error {
	if !strings.HasPrefix(strings.ToLower(c.Request().Header.Get(echo.HeaderContentType)), "multipart/form-data") {
		return NewError(http.StatusUnsupportedMediaType, "unsupported_media_type", "请求格式无效", "Asset 上传必须使用 multipart/form-data。", nil)
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, (257 << 20))
	reader, err := c.Request().MultipartReader()
	if err != nil {
		return NewError(http.StatusBadRequest, "invalid_multipart", "上传请求无效", "无法读取 multipart 边界。", err)
	}
	purpose, displayName := "", ""
	metadata := map[string]any{}
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return NewError(http.StatusBadRequest, "invalid_multipart", "上传请求无效", "无法读取 multipart 内容。", nextErr)
		}
		name := part.FormName()
		if part.FileName() == "" {
			value, readErr := io.ReadAll(io.LimitReader(part, 64<<10))
			_ = part.Close()
			if readErr != nil {
				return readErr
			}
			switch name {
			case "purpose":
				purpose = strings.TrimSpace(string(value))
			case "display_name":
				displayName = strings.TrimSpace(string(value))
			case "metadata":
				if len(value) > 0 && json.Unmarshal(value, &metadata) != nil {
					return NewError(http.StatusUnprocessableEntity, "validation_failed", "metadata 无效", "metadata 必须是 JSON object。", nil)
				}
			}
			continue
		}
		if name != "file" {
			_ = part.Close()
			continue
		}
		if purpose == "" {
			_ = part.Close()
			return NewError(http.StatusUnprocessableEntity, "validation_failed", "缺少 purpose", "multipart 中 purpose 字段必须位于 file 之前。", nil)
		}
		var upload files.Upload
		operationErr := handler.withService(c, func(service *files.Service) error {
			var err error
			upload, err = service.CreateUpload(c.Request().Context(), files.CreateUploadInput{Purpose: purpose, OriginalFilename: part.FileName(), DisplayName: displayName, Metadata: metadata, Reader: part})
			return err
		})
		_ = part.Close()
		if operationErr != nil {
			return operationErr
		}
		return Success(c, http.StatusCreated, upload)
	}
	return NewError(http.StatusUnprocessableEntity, "validation_failed", "缺少上传文件", "multipart 请求必须包含 file。", nil)
}

func (handler *FilesHandler) ShowUpload(c echo.Context) error {
	var upload files.Upload
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		upload, err = service.GetUpload(c.Request().Context(), c.Param("upload_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, upload)
}

type finalizeUploadRequest struct {
	Purpose string `json:"purpose"`
}

func (handler *FilesHandler) FinalizeUpload(c echo.Context) error {
	var request finalizeUploadRequest
	if c.Request().ContentLength != 0 {
		if err := decodeJSONLimit(c, &request, 64<<10); err != nil {
			return err
		}
	}
	request.Purpose = strings.TrimSpace(request.Purpose)
	if request.Purpose == "" {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "缺少 purpose", "finalize 必须重复声明创建暂存上传时的 purpose。", nil)
	}
	var asset files.Asset
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		asset, err = service.FinalizeUpload(c.Request().Context(), c.Param("upload_uuid"), request.Purpose)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, asset)
}
func (handler *FilesHandler) CancelUpload(c echo.Context) error {
	if err := handler.withService(c, func(service *files.Service) error {
		return service.CancelUpload(c.Request().Context(), c.Param("upload_uuid"))
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, nil)
}

func (handler *FilesHandler) ListAssets(c echo.Context) error {
	limit, err := positiveIntQuery(c, "limit", 100)
	if err != nil {
		return err
	}
	trashOnly := c.QueryParam("deleted") == "true"
	filter := files.AssetFilter{Purpose: strings.TrimSpace(c.QueryParam("purpose")), Kind: strings.TrimSpace(c.QueryParam("kind")), IncludeTrashed: trashOnly, TrashedOnly: trashOnly, Limit: limit}
	var items []files.Asset
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		items, err = service.ListAssets(c.Request().Context(), filter)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (handler *FilesHandler) ShowAsset(c echo.Context) error {
	var asset files.Asset
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		asset, err = service.GetAsset(c.Request().Context(), c.Param("asset_uuid"), c.QueryParam("include_trashed") == "true")
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, asset)
}

type updateAssetRequest struct {
	DisplayName *string        `json:"display_name"`
	Metadata    map[string]any `json:"metadata"`
}

func (handler *FilesHandler) UpdateAsset(c echo.Context) error {
	var request updateAssetRequest
	if err := decodeJSONLimit(c, &request, 128<<10); err != nil {
		return err
	}
	var asset files.Asset
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		asset, err = service.UpdateAsset(c.Request().Context(), c.Param("asset_uuid"), files.UpdateAssetInput{DisplayName: request.DisplayName, Metadata: request.Metadata})
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, asset)
}
func (handler *FilesHandler) TrashAsset(c echo.Context) error {
	var asset files.Asset
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		asset, err = service.SoftDelete(c.Request().Context(), c.Param("asset_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, asset)
}
func (handler *FilesHandler) RestoreAsset(c echo.Context) error {
	var asset files.Asset
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		asset, err = service.Restore(c.Request().Context(), c.Param("asset_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, asset)
}

func (handler *FilesHandler) ListScans(c echo.Context) error {
	limit, err := positiveIntQuery(c, "limit", 20)
	if err != nil {
		return err
	}
	var items []files.IntegrityScan
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		items, err = service.ListIntegrityScans(c.Request().Context(), limit)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (handler *FilesHandler) ShowScan(c echo.Context) error {
	var scan files.IntegrityScan
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		scan, err = service.GetIntegrityScan(c.Request().Context(), c.Param("scan_uuid"))
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, scan)
}
func (handler *FilesHandler) CreateScan(c echo.Context) error {
	if handler.tasks != nil {
		task, err := handler.tasks.CreateMaintenanceTask(c.Request().Context(), c.Param("project_uuid"), jobqueue.CreateMaintenanceInput{Kind: jobqueue.KindAssetIntegrityScan})
		if err != nil {
			return taskAPIError(err)
		}
		return Success(c, http.StatusAccepted, task)
	}
	var scan files.IntegrityScan
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		scan, err = service.RunIntegrityScan(c.Request().Context())
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, scan)
}
func (handler *FilesHandler) Reconcile(c echo.Context) error {
	if handler.tasks != nil {
		task, err := handler.tasks.CreateMaintenanceTask(c.Request().Context(), c.Param("project_uuid"), jobqueue.CreateMaintenanceInput{Kind: jobqueue.KindAssetReconcile})
		if err != nil {
			return taskAPIError(err)
		}
		return Success(c, http.StatusAccepted, task)
	}
	var summary files.ReconcileSummary
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		summary, err = service.Reconcile(c.Request().Context(), 1000)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, summary)
}

type gcRequest struct {
	GraceHours int `json:"grace_hours"`
}

func (handler *FilesHandler) GCDryRun(c echo.Context) error {
	var request gcRequest
	if c.Request().ContentLength != 0 {
		if err := decodeJSONLimit(c, &request, 64<<10); err != nil {
			return err
		}
	}
	if request.GraceHours < 0 || request.GraceHours > 24*365 {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "GC grace period 无效", "grace_hours 必须在 0 到 8760 之间。", nil)
	}
	grace := time.Duration(request.GraceHours) * time.Hour
	var plan files.GCPlan
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		plan, err = service.GCDryRun(c.Request().Context(), grace)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusCreated, plan)
}
func (handler *FilesHandler) GCApply(c echo.Context) error {
	var request gcRequest
	if c.Request().ContentLength != 0 {
		if err := decodeJSONLimit(c, &request, 64<<10); err != nil {
			return err
		}
	}
	if request.GraceHours < 0 || request.GraceHours > 24*365 {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "GC grace period 无效", "grace_hours 必须在 0 到 8760 之间。", nil)
	}
	grace := time.Duration(request.GraceHours) * time.Hour
	if handler.tasks != nil {
		task, err := handler.tasks.CreateMaintenanceTask(c.Request().Context(), c.Param("project_uuid"), jobqueue.CreateMaintenanceInput{Kind: jobqueue.KindAssetGCApply, PlanUUID: c.Param("plan_uuid"), GraceHours: request.GraceHours})
		if err != nil {
			return taskAPIError(err)
		}
		return Success(c, http.StatusAccepted, task)
	}
	var plan files.GCPlan
	if err := handler.withService(c, func(service *files.Service) error {
		var err error
		plan, err = service.GCApply(c.Request().Context(), c.Param("plan_uuid"), grace)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, plan)
}

func (handler *FilesHandler) CreateMaintenanceTask(c echo.Context) error {
	if handler.tasks == nil {
		return NewError(http.StatusServiceUnavailable, "project_runtime_unavailable", "项目维护运行时未启动", "请重新打开项目后重试。", nil)
	}
	var request jobqueue.CreateMaintenanceInput
	if err := decodeJSONLimit(c, &request, 64<<10); err != nil {
		return err
	}
	task, err := handler.tasks.CreateMaintenanceTask(c.Request().Context(), c.Param("project_uuid"), request)
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusAccepted, task)
}

func (handler *FilesHandler) ListMaintenanceTasks(c echo.Context) error {
	if handler.tasks == nil {
		return NewError(http.StatusServiceUnavailable, "project_runtime_unavailable", "项目维护运行时未启动", "请重新打开项目后重试。", nil)
	}
	limit, err := positiveIntQuery(c, "limit", 50)
	if err != nil {
		return err
	}
	items, err := handler.tasks.ListMaintenanceTasks(c.Request().Context(), c.Param("project_uuid"), limit)
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

func (handler *FilesHandler) ShowMaintenanceTask(c echo.Context) error {
	if handler.tasks == nil {
		return NewError(http.StatusServiceUnavailable, "project_runtime_unavailable", "项目维护运行时未启动", "请重新打开项目后重试。", nil)
	}
	task, err := handler.tasks.GetMaintenanceTask(c.Request().Context(), c.Param("project_uuid"), c.Param("task_uuid"))
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusOK, task)
}

func (handler *FilesHandler) CancelMaintenanceTask(c echo.Context) error {
	if handler.tasks == nil {
		return NewError(http.StatusServiceUnavailable, "project_runtime_unavailable", "项目维护运行时未启动", "请重新打开项目后重试。", nil)
	}
	task, err := handler.tasks.CancelMaintenanceTask(c.Request().Context(), c.Param("project_uuid"), c.Param("task_uuid"))
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusOK, task)
}

func (handler *FilesHandler) MaintenanceTaskEvents(c echo.Context) error {
	if handler.tasks == nil {
		return NewError(http.StatusServiceUnavailable, "project_runtime_unavailable", "项目维护运行时未启动", "请重新打开项目后重试。", nil)
	}
	limit, err := positiveIntQuery(c, "limit", 50)
	if err != nil {
		return err
	}
	parse := func(name string) (int64, error) {
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
	before, err := parse("before")
	if err != nil {
		return err
	}
	after, err := parse("after")
	if err != nil {
		return err
	}
	if before > 0 && after > 0 {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "事件游标冲突", "before 与 after 不能同时使用。", nil)
	}
	items, pagination, err := handler.tasks.ListMaintenanceTaskEvents(c.Request().Context(), c.Param("project_uuid"), c.Param("task_uuid"), before, after, limit)
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items, "cursor_pagination": pagination})
}

func (handler *FilesHandler) CreateThumbnail(c echo.Context) error {
	profile := strings.TrimSpace(c.QueryParam("profile"))
	if err := handler.withService(c, func(service *files.Service) error {
		_, err := service.EnsureThumbnail(c.Request().Context(), c.Param("asset_uuid"), profile)
		return err
	}); err != nil {
		return err
	}
	return Success(c, http.StatusOK, map[string]any{"asset_uuid": c.Param("asset_uuid"), "profile": profile, "status": "ready"})
}

func (handler *FilesHandler) Content(c echo.Context) error {
	err := handler.manager.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var publisher files.EventPublisher
		if handler.hub != nil {
			publisher = handler.hub
		}
		content, err := files.NewService(store, publisher).OpenContent(c.Request().Context(), c.Param("asset_uuid"))
		if err != nil {
			return err
		}
		defer content.File.Close()
		headers := c.Response().Header()
		headers.Set(echo.HeaderContentType, content.Asset.MIMEType)
		headers.Set(echo.HeaderContentLength, strconv.FormatInt(content.Asset.ByteSize, 10))
		headers.Set("ETag", content.ETag)
		headers.Set("Last-Modified", content.LastModified.Format(http.TimeFormat))
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("Cache-Control", "private, max-age=31536000, immutable")
		disposition := "inline"
		if content.Asset.Kind == "text" || content.Asset.Kind == "archive" || content.Asset.Kind == "document" || content.Asset.Kind == "binary" {
			disposition = "attachment"
		}
		headers.Set("Content-Disposition", disposition+`; filename="asset.`+safeHeaderExtension(content.Asset.MIMEType)+`"; filename*=`+filesContentDisposition(content.Filename))
		http.ServeContent(c.Response(), c.Request(), content.Filename, content.LastModified, content.File)
		return nil
	})
	if err != nil {
		return filesAPIError(err)
	}
	return nil
}

func safeHeaderExtension(mimeType string) string {
	switch mimeType {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "text/plain":
		return "txt"
	case "text/markdown":
		return "md"
	case "application/pdf":
		return "pdf"
	case "application/zip":
		return "zip"
	default:
		return "bin"
	}
}
func filesContentDisposition(filename string) string {
	return "UTF-8''" + strings.ReplaceAll(url.PathEscape(filename), "+", "%20")
}
