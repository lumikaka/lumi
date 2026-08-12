package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"lumi/internal/jobqueue"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/story"

	"github.com/labstack/echo/v4"
)

type TaskHandler struct{ tasks *jobqueue.Manager }

func NewTaskHandler(tasks *jobqueue.Manager) *TaskHandler { return &TaskHandler{tasks: tasks} }

func (handler *TaskHandler) CreateChapterGeneration(c echo.Context) error {
	var request jobqueue.CreateGenerationInput
	if err := decodeJSONLimit(c, &request, 512<<10); err != nil {
		return err
	}
	request.ChapterUUID = c.Param("chapter_uuid")
	created, err := handler.tasks.CreateChapterGeneration(c.Request().Context(), c.Param("project_uuid"), request)
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusCreated, created)
}

func (handler *TaskHandler) CreateStoryProfileGeneration(c echo.Context) error {
	return handler.createStoryWorkflow(c, jobqueue.KindStoryProfileGeneration, "")
}

func (handler *TaskHandler) CreateStoryProfileFromChapters(c echo.Context) error {
	return handler.createStoryWorkflow(c, jobqueue.KindStoryProfileFromChapters, "")
}

func (handler *TaskHandler) CreateChapterBatchPlan(c echo.Context) error {
	return handler.createStoryWorkflow(c, jobqueue.KindStoryChapterBatchPlan, "")
}

func (handler *TaskHandler) CreateComicStoryboardGeneration(c echo.Context) error {
	return handler.createStoryWorkflow(c, jobqueue.KindComicStoryboardGeneration, c.Param("chapter_uuid"))
}

func (handler *TaskHandler) createStoryWorkflow(c echo.Context, kind, chapterUUID string) error {
	var request jobqueue.CreateStoryWorkflowInput
	if err := decodeJSONLimit(c, &request, 512<<10); err != nil {
		return err
	}
	created, err := handler.tasks.CreateStoryWorkflow(c.Request().Context(), c.Param("project_uuid"), kind, chapterUUID, request)
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusCreated, created)
}

func (handler *TaskHandler) Index(c echo.Context) error {
	limit, err := positiveIntQuery(c, "limit", 50)
	if err != nil {
		return err
	}
	items, err := handler.tasks.ListTasks(c.Request().Context(), c.Param("project_uuid"), strings.TrimSpace(c.QueryParam("status")), limit)
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

func (handler *TaskHandler) Show(c echo.Context) error {
	task, err := handler.tasks.GetTask(c.Request().Context(), c.Param("project_uuid"), c.Param("task_uuid"))
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusOK, task)
}

func (handler *TaskHandler) Events(c echo.Context) error {
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
	if before > 0 && after > 0 {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "事件游标冲突", "before 与 after 不能同时使用。", nil)
	}
	items, pagination, err := handler.tasks.ListTaskEvents(c.Request().Context(), c.Param("project_uuid"), c.Param("task_uuid"), before, after, limit)
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items, "cursor_pagination": pagination})
}

func (handler *TaskHandler) Cancel(c echo.Context) error {
	task, err := handler.tasks.CancelTask(c.Request().Context(), c.Param("project_uuid"), c.Param("task_uuid"))
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusOK, task)
}

func (handler *TaskHandler) Retry(c echo.Context) error {
	task, err := handler.tasks.RetryTask(c.Request().Context(), c.Param("project_uuid"), c.Param("task_uuid"))
	if err != nil {
		return taskAPIError(err)
	}
	return Success(c, http.StatusOK, task)
}

func taskAPIError(err error) error {
	var projectErr *project.Error
	if errors.As(err, &projectErr) {
		return projectAPIError(err)
	}
	var providerErr *provider.Error
	if errors.As(err, &providerErr) {
		return providerAPIError(err)
	}
	var storyErr *story.Error
	if errors.As(err, &storyErr) {
		return storyAPIError(err)
	}
	var domainErr *jobqueue.Error
	if !errors.As(err, &domainErr) {
		return NewError(http.StatusInternalServerError, "task_operation_failed", "任务操作失败", "发生了未预期的本地队列或存储错误。", err)
	}
	status := http.StatusUnprocessableEntity
	switch domainErr.Code {
	case jobqueue.CodeTaskNotFound:
		status = http.StatusNotFound
	case jobqueue.CodeTaskConflict, jobqueue.CodeTaskStateConflict:
		status = http.StatusConflict
	case jobqueue.CodeProjectRuntimeUnavailable:
		status = http.StatusServiceUnavailable
	case jobqueue.CodeTaskPersistenceFailed:
		status = http.StatusInternalServerError
	}
	return NewError(status, domainErr.Code, domainErr.Message, domainErr.Details, err)
}
