package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/jobqueue"
	"lumi/internal/llm"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/story"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type httpAgentQueue struct{ next int64 }

func (queue *httpAgentQueue) EnqueueAgentTx(context.Context, string, *sql.Tx, agent.JobSpec) (int64, error) {
	queue.next++
	return queue.next, nil
}

func (*httpAgentQueue) CancelAgentJob(context.Context, string, int64) error { return nil }
func (*httpAgentQueue) CancelAgentWork(string, string)                      {}
func (*httpAgentQueue) StartDomainTask(context.Context, string, agent.DomainTaskRequest) (agent.DomainTask, error) {
	return agent.DomainTask{}, nil
}
func (*httpAgentQueue) StartDomainTaskBatch(context.Context, string, agent.DomainTaskBatchRequest) (agent.DomainTaskBatch, error) {
	return agent.DomainTaskBatch{}, nil
}
func (*httpAgentQueue) GetDomainTask(context.Context, string, string, string) (agent.DomainTask, error) {
	return agent.DomainTask{}, nil
}
func (*httpAgentQueue) ListDomainTasks(context.Context, string, string, string, int) ([]agent.DomainTask, error) {
	return []agent.DomainTask{}, nil
}
func (*httpAgentQueue) ListDomainTaskEvents(context.Context, string, string, string, int64, int64, int) ([]agent.DomainTaskEvent, agent.CursorPagination, error) {
	return []agent.DomainTaskEvent{}, agent.CursorPagination{}, nil
}
func (*httpAgentQueue) CancelDomainTask(context.Context, string, string, string) error { return nil }
func (*httpAgentQueue) RetryDomainTask(context.Context, string, string, string) (agent.DomainTask, error) {
	return agent.DomainTask{}, nil
}

type httpToolModel struct{}

func (httpToolModel) Complete(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "done"}, FinishReason: "stop"}, nil
}

func TestUserInputResponseDecoderRejectsSpoofedAndDuplicateAnswerFields(t *testing.T) {
	for name, raw := range map[string]string{
		"spoofed label": `{"answers":{"style":{"selected_option_uuid":"01990000-0000-7000-8000-000000000901","other_text":"","label":"伪造标签"}}}`,
		"duplicate map": `{"answers":{"style":{"selected_option_uuid":"01990000-0000-7000-8000-000000000901"},"style":{"other_text":"伪造覆盖"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			e := echo.New()
			request := httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(raw))
			request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			context := e.NewContext(request, httptest.NewRecorder())
			var input agent.UserInputResponse
			if err := decodeUniqueJSON(context, &input); err == nil {
				t.Fatal("invalid answer fields were accepted")
			}
		})
	}
}

func TestAgentHandlersExposeProjectScopedResourcesWithoutInternalIDs(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	_, err = providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/agent-model", APIKey: "http-agent-secret"})
	if err != nil {
		t.Fatal(err)
	}
	projects := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen)
	created, err := projects.CreateWithInput(ctx, project.CreateInput{
		Name:        "Agent API",
		PictureBook: &project.PictureBookInput{Format: project.PictureBookVertical},
	}, project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = projects.Close(); _ = app.Close() })
	var storyboardSection production.ComicSection
	if err := projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		chapter, createErr := story.NewService(store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "API Chapter", Content: "Body", ContentFormat: "md"})
		if createErr != nil {
			return createErr
		}
		storyboardSection, createErr = production.NewService(store, nil).CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "API Section", StoryboardMD: "Initial storyboard"})
		return createErr
	}); err != nil {
		t.Fatal(err)
	}
	service := agent.NewService(projects, providers, httpToolModel{}, &httpAgentQueue{}, nil)
	handler := NewAgentHandler(service)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/projects/:project_uuid/chat_threads", handler.CreateThread)
	e.GET("/api/v1/projects/:project_uuid/chat_threads", handler.ListThreads)
	e.POST("/api/v1/projects/:project_uuid/chat_threads/:thread_uuid/turns", handler.CreateTurn)
	e.GET("/api/v1/projects/:project_uuid/chat_threads/:thread_uuid/items", handler.ListItems)
	e.GET("/api/v1/projects/:project_uuid/chat_threads/:thread_uuid/events", handler.ListEvents)
	e.GET("/api/v1/projects/:project_uuid/chat_threads/:thread_uuid/trajectory", handler.ShowTrajectory)
	e.POST("/api/v1/projects/:project_uuid/chat_threads/:thread_uuid/follow_ups", handler.CreateFollowUp)
	e.POST("/api/v1/projects/:project_uuid/chat_threads/:thread_uuid/follow_ups/:follow_up_uuid/steerings", handler.SteerFollowUp)
	e.POST("/api/v1/projects/:project_uuid/workflows", handler.CreateYoloWorkflow)
	e.GET("/api/v1/projects/:project_uuid/workflows/:workflow_uuid/runs", handler.ListWorkflowRuns)
	e.GET("/api/v1/projects/:project_uuid/workflows/:workflow_uuid/events", handler.ListWorkflowEvents)
	e.GET("/api/v1/projects/:project_uuid/workflows/:workflow_uuid/llm-logs", handler.ListWorkflowLLMLogs)

	base := "/api/v1/projects/" + created.UUID
	threadResponse := requestJSON(t, e, http.MethodPost, base+"/chat_threads", map[string]any{"title": "项目助手"})
	if threadResponse.Code != http.StatusCreated {
		t.Fatalf("create thread = %d %s", threadResponse.Code, threadResponse.Body.String())
	}
	threadUUID := envelopeData(t, threadResponse)["uuid"].(string)
	premiseThreadResponse := requestJSON(t, e, http.MethodPost, base+"/chat_threads", map[string]any{"title": "单项生成"})
	if premiseThreadResponse.Code != http.StatusCreated || strings.Contains(premiseThreadResponse.Body.String(), `"scope"`) || strings.Contains(premiseThreadResponse.Body.String(), `"scene"`) || strings.Contains(premiseThreadResponse.Body.String(), `"subject_uuid"`) {
		t.Fatalf("create premise thread = %d %s", premiseThreadResponse.Code, premiseThreadResponse.Body.String())
	}
	projectList := requestJSON(t, e, http.MethodGet, base+"/chat_threads", nil)
	if projectList.Code != http.StatusOK || !strings.Contains(projectList.Body.String(), `"title":"单项生成"`) || !strings.Contains(projectList.Body.String(), `"title":"项目助手"`) || !strings.Contains(projectList.Body.String(), `"total":2`) {
		t.Fatalf("project thread list = %d %s", projectList.Code, projectList.Body.String())
	}
	unfilteredProjectList := requestJSON(t, e, http.MethodGet, base+"/chat_threads", nil)
	if unfilteredProjectList.Code != http.StatusOK || !strings.Contains(unfilteredProjectList.Body.String(), `"title":"单项生成"`) || !strings.Contains(unfilteredProjectList.Body.String(), `"title":"项目助手"`) || !strings.Contains(unfilteredProjectList.Body.String(), `"total":2`) {
		t.Fatalf("project thread list = %d %s", unfilteredProjectList.Code, unfilteredProjectList.Body.String())
	}
	storyboardThreadResponse := requestJSON(t, e, http.MethodPost, base+"/chat_threads", map[string]any{"title": "分镜引用"})
	if storyboardThreadResponse.Code != http.StatusCreated || strings.Contains(storyboardThreadResponse.Body.String(), `"scene"`) || strings.Contains(storyboardThreadResponse.Body.String(), storyboardSection.UUID) {
		t.Fatalf("create storyboard thread = %d %s", storyboardThreadResponse.Code, storyboardThreadResponse.Body.String())
	}
	invalidScene := requestJSON(t, e, http.MethodPost, base+"/chat_threads", map[string]any{"title": "旧协议", "scope": "project", "scene": "premise_asset_generation"})
	if invalidScene.Code != http.StatusBadRequest {
		t.Fatalf("legacy thread fields = %d %s", invalidScene.Code, invalidScene.Body.String())
	}
	turnResponse := requestJSON(t, e, http.MethodPost, base+"/chat_threads/"+threadUUID+"/turns", map[string]any{"input_text": "继续创作", "max_steps": 999, "references": []map[string]any{{"resource_type": "comic_section", "resource_uuid": storyboardSection.UUID}}})
	if turnResponse.Code != http.StatusCreated {
		t.Fatalf("create turn = %d %s", turnResponse.Code, turnResponse.Body.String())
	}
	followResponse := requestJSON(t, e, http.MethodPost, base+"/chat_threads/"+threadUUID+"/follow_ups", map[string]any{"input_text": "再补一个结尾"})
	if followResponse.Code != http.StatusCreated {
		t.Fatalf("create follow-up = %d %s", followResponse.Code, followResponse.Body.String())
	}
	followUUID := envelopeData(t, followResponse)["uuid"].(string)
	steerFallbackResponse := requestJSON(t, e, http.MethodPost, base+"/chat_threads/"+threadUUID+"/follow_ups/"+followUUID+"/steerings", nil)
	if steerFallbackResponse.Code != http.StatusOK || !strings.Contains(steerFallbackResponse.Body.String(), `"delivery_mode":"follow_up"`) {
		t.Fatalf("follow-up steering fallback = %d %s", steerFallbackResponse.Code, steerFallbackResponse.Body.String())
	}
	itemsResponse := requestJSON(t, e, http.MethodGet, base+"/chat_threads/"+threadUUID+"/items?limit=25", nil)
	eventsResponse := requestJSON(t, e, http.MethodGet, base+"/chat_threads/"+threadUUID+"/events?limit=25", nil)
	trajectoryResponse := requestJSON(t, e, http.MethodGet, base+"/chat_threads/"+threadUUID+"/trajectory?limit=25", nil)
	trajectoryItems := envelopeData(t, trajectoryResponse)["items"].([]any)
	trajectoryItemUUID := trajectoryItems[0].(map[string]any)["uuid"].(string)
	trajectoryDeepLinkResponse := requestJSON(t, e, http.MethodGet, base+"/chat_threads/"+threadUUID+"/trajectory?limit=25&item_uuid="+trajectoryItemUUID, nil)
	missingTrajectoryResponse := requestJSON(t, e, http.MethodGet, base+"/chat_threads/"+httpAgentUUIDv7(t)+"/trajectory?limit=25", nil)
	workflowResponse := requestJSON(t, e, http.MethodPost, base+"/workflows", map[string]any{"title": "快速创作", "story_prompt": "一只小熊寻找星光。", "idempotency_key": "http-yolo-one"})
	workflowUUID := envelopeData(t, workflowResponse)["uuid"].(string)
	workflowRunsResponse := requestJSON(t, e, http.MethodGet, base+"/workflows/"+workflowUUID+"/runs?limit=2", nil)
	workflowEventsResponse := requestJSON(t, e, http.MethodGet, base+"/workflows/"+workflowUUID+"/events?limit=20", nil)
	workflowLogsResponse := requestJSON(t, e, http.MethodGet, base+"/workflows/"+workflowUUID+"/llm-logs?page=1&per_page=20", nil)
	workflowSteps := envelopeData(t, workflowResponse)["steps"].([]any)
	workflowStepUUID := workflowSteps[0].(map[string]any)["uuid"].(string)
	filteredWorkflowLogsResponse := requestJSON(t, e, http.MethodGet, base+"/workflows/"+workflowUUID+"/llm-logs?page=1&per_page=20&workflow_step_uuid="+workflowStepUUID, nil)
	allProjectThreads := requestJSON(t, e, http.MethodGet, base+"/chat_threads?per_page=20", nil)
	if allProjectThreads.Code != http.StatusOK || !strings.Contains(allProjectThreads.Body.String(), `"title":"项目助手"`) || !strings.Contains(allProjectThreads.Body.String(), `"title":"单项生成"`) || !strings.Contains(allProjectThreads.Body.String(), `"title":"分镜引用"`) || !strings.Contains(allProjectThreads.Body.String(), `"total":4`) {
		t.Fatalf("mixed project threads = %d %s", allProjectThreads.Code, allProjectThreads.Body.String())
	}
	for name, response := range map[string]string{"thread": threadResponse.Body.String(), "premise_thread": premiseThreadResponse.Body.String(), "project_list": projectList.Body.String(), "unfiltered_project_list": unfilteredProjectList.Body.String(), "all_project_threads": allProjectThreads.Body.String(), "storyboard_thread": storyboardThreadResponse.Body.String(), "turn": turnResponse.Body.String(), "follow_up": followResponse.Body.String(), "steer_fallback": steerFallbackResponse.Body.String(), "items": itemsResponse.Body.String(), "events": eventsResponse.Body.String(), "trajectory": trajectoryResponse.Body.String(), "trajectory_deep_link": trajectoryDeepLinkResponse.Body.String(), "workflow": workflowResponse.Body.String(), "workflow_runs": workflowRunsResponse.Body.String(), "workflow_events": workflowEventsResponse.Body.String(), "workflow_logs": workflowLogsResponse.Body.String(), "filtered_workflow_logs": filteredWorkflowLogsResponse.Body.String()} {
		if strings.Contains(response, `"id":`) || strings.Contains(response, "river_job_id") || strings.Contains(response, "http-agent-secret") || strings.Contains(response, "root_path") {
			t.Fatalf("%s response leaked internal data: %s", name, response)
		}
	}
	if itemsResponse.Code != http.StatusOK || !strings.Contains(itemsResponse.Body.String(), `"cursor_pagination"`) || !strings.Contains(itemsResponse.Body.String(), storyboardSection.UUID) || !strings.Contains(itemsResponse.Body.String(), `"resource_type":"comic_section"`) || eventsResponse.Code != http.StatusOK || trajectoryResponse.Code != http.StatusOK || trajectoryDeepLinkResponse.Code != http.StatusOK || !strings.Contains(trajectoryResponse.Body.String(), `"history_complete":true`) || !strings.Contains(trajectoryResponse.Body.String(), `"model_requests":[]`) || missingTrajectoryResponse.Code != http.StatusNotFound || workflowResponse.Code != http.StatusCreated || workflowRunsResponse.Code != http.StatusOK || workflowEventsResponse.Code != http.StatusOK || workflowLogsResponse.Code != http.StatusOK || filteredWorkflowLogsResponse.Code != http.StatusOK || !strings.Contains(projectList.Body.String(), `"pagination"`) {
		t.Fatalf("items=%d events=%d trajectory=%d deep_link=%d missing_trajectory=%d workflow=%d runs=%d workflow_events=%d logs=%d filtered_logs=%d", itemsResponse.Code, eventsResponse.Code, trajectoryResponse.Code, trajectoryDeepLinkResponse.Code, missingTrajectoryResponse.Code, workflowResponse.Code, workflowRunsResponse.Code, workflowEventsResponse.Code, workflowLogsResponse.Code, filteredWorkflowLogsResponse.Code)
	}
}

func httpAgentUUIDv7(t *testing.T) string {
	t.Helper()
	value, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return value.String()
}

func TestAgentAPIErrorPreservesDomainTaskConflict(t *testing.T) {
	mapped, ok := agentAPIError(&jobqueue.Error{Code: jobqueue.CodeTaskStateConflict, Message: "任务不能重试", Details: "只有可重试状态才能重试。"}).(*APIError)
	if !ok {
		t.Fatalf("mapped error type = %T", mapped)
	}
	if mapped.Status != http.StatusConflict || mapped.Code != jobqueue.CodeTaskStateConflict || mapped.Details != "只有可重试状态才能重试。" {
		t.Fatalf("mapped error = %+v", mapped)
	}
}
