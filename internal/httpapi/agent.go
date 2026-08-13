package httpapi

import (
	"errors"
	"net/http"

	"lumi/internal/agent"
	"lumi/internal/jobqueue"
	"lumi/internal/project"
	"lumi/internal/provider"

	"github.com/labstack/echo/v4"
)

type AgentHandler struct {
	agents *agent.Service
}

func NewAgentHandler(agents *agent.Service) *AgentHandler {
	return &AgentHandler{agents: agents}
}

func (handler *AgentHandler) ListThreads(c echo.Context) error {
	page, err := positiveIntQuery(c, "page", 1)
	if err != nil {
		return err
	}
	perPage, err := positiveIntQuery(c, "per_page", 30)
	if err != nil {
		return err
	}
	result, err := handler.agents.ListThreadsPage(c.Request().Context(), c.Param("project_uuid"), page, perPage)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, result)
}

func (handler *AgentHandler) CreateThread(c echo.Context) error {
	var input agent.CreateThreadInput
	if err := decodeJSON(c, &input); err != nil {
		return err
	}
	created, err := handler.agents.CreateThread(c.Request().Context(), c.Param("project_uuid"), input)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusCreated, created)
}

func (handler *AgentHandler) ShowThread(c echo.Context) error {
	thread, err := handler.agents.GetThread(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"))
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, thread)
}

func (handler *AgentHandler) ListTurns(c echo.Context) error {
	items, err := handler.agents.ListTurns(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"))
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

func (handler *AgentHandler) CreateTurn(c echo.Context) error {
	var input agent.CreateTurnInput
	if err := decodeJSONLimit(c, &input, 256<<10); err != nil {
		return err
	}
	created, err := handler.agents.CreateTurn(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), input)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusCreated, created)
}

func (handler *AgentHandler) ListItems(c echo.Context) error {
	limit, err := positiveIntQuery(c, "limit", agent.DefaultItemsPage)
	if err != nil {
		return err
	}
	page, err := handler.agents.ListItems(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), c.QueryParam("before"), c.QueryParam("after"), limit)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, page)
}

func (handler *AgentHandler) ListEvents(c echo.Context) error {
	limit, err := positiveIntQuery(c, "limit", 100)
	if err != nil {
		return err
	}
	page, err := handler.agents.ListEvents(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), c.QueryParam("after"), limit)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, page)
}

func (handler *AgentHandler) ListFollowUps(c echo.Context) error {
	items, err := handler.agents.ListFollowUps(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"))
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

func (handler *AgentHandler) CreateFollowUp(c echo.Context) error {
	var input agent.CreateFollowUpInput
	if err := decodeJSONLimit(c, &input, 256<<10); err != nil {
		return err
	}
	created, err := handler.agents.CreateFollowUp(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), input)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusCreated, created)
}

func (handler *AgentHandler) UpdateFollowUp(c echo.Context) error {
	var input agent.CreateFollowUpInput
	if err := decodeJSONLimit(c, &input, 256<<10); err != nil {
		return err
	}
	updated, err := handler.agents.UpdateFollowUp(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), c.Param("follow_up_uuid"), input)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, updated)
}

type moveFollowUpRequest struct {
	Position int `json:"position"`
}

func (handler *AgentHandler) MoveFollowUp(c echo.Context) error {
	var input moveFollowUpRequest
	if err := decodeJSON(c, &input); err != nil {
		return err
	}
	items, err := handler.agents.MoveFollowUp(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), c.Param("follow_up_uuid"), input.Position)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

func (handler *AgentHandler) DeleteFollowUp(c echo.Context) error {
	if err := handler.agents.DeleteFollowUp(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), c.Param("follow_up_uuid")); err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, nil)
}

func (handler *AgentHandler) Steer(c echo.Context) error {
	var input agent.SteeringInput
	if err := decodeJSONLimit(c, &input, 64<<10); err != nil {
		return err
	}
	created, err := handler.agents.Steer(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), input)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusCreated, created)
}

func (handler *AgentHandler) SteerFollowUp(c echo.Context) error {
	delivery, err := handler.agents.SteerFollowUp(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), c.Param("follow_up_uuid"))
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, delivery)
}

func (handler *AgentHandler) Abort(c echo.Context) error {
	turn, err := handler.agents.Abort(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"))
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, turn)
}

func (handler *AgentHandler) ListUserInputRequests(c echo.Context) error {
	items, err := handler.agents.ListUserInputRequests(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"))
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

func (handler *AgentHandler) RespondUserInput(c echo.Context) error {
	var input agent.UserInputResponse
	if err := decodeJSON(c, &input); err != nil {
		return err
	}
	request, err := handler.agents.RespondUserInput(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), c.Param("request_uuid"), input)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, request)
}

func (handler *AgentHandler) CancelUserInput(c echo.Context) error {
	request, err := handler.agents.CancelUserInput(c.Request().Context(), c.Param("project_uuid"), c.Param("thread_uuid"), c.Param("request_uuid"))
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, request)
}

func (handler *AgentHandler) ListWorkflows(c echo.Context) error {
	items, err := handler.agents.ListWorkflows(c.Request().Context(), c.Param("project_uuid"))
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

func (handler *AgentHandler) CreateYoloWorkflow(c echo.Context) error {
	var input agent.CreateYoloInput
	if err := decodeJSONLimit(c, &input, 32<<10); err != nil {
		return err
	}
	created, err := handler.agents.CreateYoloWorkflow(c.Request().Context(), c.Param("project_uuid"), input)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusCreated, created)
}

func (handler *AgentHandler) ShowWorkflow(c echo.Context) error {
	workflow, err := handler.agents.GetWorkflow(c.Request().Context(), c.Param("project_uuid"), c.Param("workflow_uuid"))
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, workflow)
}

func (handler *AgentHandler) ListWorkflowRuns(c echo.Context) error {
	limit, err := positiveIntQuery(c, "limit", 50)
	if err != nil {
		return err
	}
	page, err := handler.agents.ListWorkflowRuns(c.Request().Context(), c.Param("project_uuid"), c.Param("workflow_uuid"), c.QueryParam("before"), limit)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, page)
}

func (handler *AgentHandler) ListWorkflowEvents(c echo.Context) error {
	limit, err := positiveIntQuery(c, "limit", 100)
	if err != nil {
		return err
	}
	page, err := handler.agents.ListWorkflowEvents(c.Request().Context(), c.Param("project_uuid"), c.Param("workflow_uuid"), c.QueryParam("before"), c.QueryParam("after"), limit)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, page)
}

func (handler *AgentHandler) ListWorkflowLLMLogs(c echo.Context) error {
	page, err := positiveIntQuery(c, "page", 1)
	if err != nil {
		return err
	}
	perPage, err := positiveIntQuery(c, "per_page", 20)
	if err != nil {
		return err
	}
	result, err := handler.agents.ListWorkflowLLMLogs(c.Request().Context(), c.Param("project_uuid"), c.Param("workflow_uuid"), c.QueryParam("workflow_step_uuid"), page, perPage)
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, result)
}

func (handler *AgentHandler) CancelWorkflow(c echo.Context) error {
	workflow, err := handler.agents.CancelWorkflow(c.Request().Context(), c.Param("project_uuid"), c.Param("workflow_uuid"))
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, workflow)
}

func (handler *AgentHandler) RetryWorkflow(c echo.Context) error {
	workflow, err := handler.agents.RetryWorkflow(c.Request().Context(), c.Param("project_uuid"), c.Param("workflow_uuid"))
	if err != nil {
		return agentAPIError(err)
	}
	return Success(c, http.StatusOK, workflow)
}

func agentAPIError(err error) error {
	var projectErr *project.Error
	if errors.As(err, &projectErr) {
		return projectAPIError(err)
	}
	var providerErr *provider.Error
	if errors.As(err, &providerErr) {
		return providerAPIError(err)
	}
	var taskErr *jobqueue.Error
	if errors.As(err, &taskErr) {
		return taskAPIError(err)
	}
	var domainErr *agent.Error
	if !errors.As(err, &domainErr) {
		return NewError(http.StatusInternalServerError, "agent_operation_failed", "Agent 操作失败", "发生了未预期的本地编排或存储错误。", err)
	}
	status := http.StatusUnprocessableEntity
	switch domainErr.Code {
	case agent.CodeNotFound:
		status = http.StatusNotFound
	case agent.CodeBusy, agent.CodeStateConflict:
		status = http.StatusConflict
	case agent.CodeProvider:
		status = http.StatusBadGateway
	case agent.CodeInterrupted:
		status = http.StatusServiceUnavailable
	case agent.CodeCancelled:
		status = http.StatusConflict
	}
	return NewError(status, domainErr.Code, domainErr.Message, domainErr.Details, err)
}
