package httpapi

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/jobqueue"
	"lumi/internal/llm"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/story"

	"github.com/labstack/echo/v4"
)

type immediateModelFake struct{}

func (immediateModelFake) Check(context.Context, string, string, string) error { return nil }

func (immediateModelFake) Generate(_ context.Context, _ llm.Request, onDelta func(string) error) (llm.Response, error) {
	if onDelta != nil {
		if err := onDelta("API 生成正文"); err != nil {
			return llm.Response{}, err
		}
	}
	return llm.Response{Content: "API 生成正文", FinishReason: "stop"}, nil
}

func TestTaskHandlersExposeProductUUIDsAndCursorRecovery(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	_, err = providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/story-model", APIKey: "task-handler-secret"})
	if err != nil {
		t.Fatal(err)
	}
	queue := jobqueue.NewManager(providers, immediateModelFake{}, nil)
	projects := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen).WithRuntime(queue).WithOpenHook(queue.StartProject)
	createdProject, err := projects.Create(ctx, "Task API", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var chapter story.Chapter
	if err := projects.WithCurrentStore(ctx, createdProject.UUID, func(store *project.Store) error {
		var createErr error
		chapter, createErr = story.NewService(store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Opening", Content: "Old", ContentFormat: "md"})
		return createErr
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = projects.Close(); _ = app.Close() })

	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	handler := NewTaskHandler(queue)
	base := "/api/v1/projects/" + createdProject.UUID
	e.POST("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/generations", handler.CreateChapterGeneration)
	e.GET("/api/v1/projects/:project_uuid/tasks/:task_uuid", handler.Show)
	e.GET("/api/v1/projects/:project_uuid/tasks/:task_uuid/events", handler.Events)
	legacy := requestJSON(t, e, "POST", base+"/chapters/"+chapter.UUID+"/generations", map[string]any{"provider_uuid": chapter.UUID, "prompt": "write", "idempotency_key": "http-legacy", "parameters": map[string]any{}})
	if legacy.Code != 400 || !strings.Contains(legacy.Body.String(), `"code":"invalid_json"`) {
		t.Fatalf("legacy provider selection was accepted: %d %s", legacy.Code, legacy.Body.String())
	}
	response := requestJSON(t, e, "POST", base+"/chapters/"+chapter.UUID+"/generations", map[string]any{"prompt": "write", "idempotency_key": "http-one", "parameters": map[string]any{}})
	if response.Code != 201 || strings.Contains(response.Body.String(), "river") || strings.Contains(response.Body.String(), `"id":`) || strings.Contains(response.Body.String(), "task-handler-secret") {
		t.Fatalf("create task response = %d %s", response.Code, response.Body.String())
	}
	taskUUID := envelopeData(t, response)["uuid"].(string)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		shown := requestJSON(t, e, "GET", base+"/tasks/"+taskUUID, nil)
		if strings.Contains(shown.Body.String(), `"status":"completed"`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	events := requestJSON(t, e, "GET", base+"/tasks/"+taskUUID+"/events?after=0&limit=2", nil)
	if events.Code != 200 || !strings.Contains(events.Body.String(), `"cursor_pagination":{"per_page":2`) || strings.Contains(events.Body.String(), "river_job_id") || strings.Contains(events.Body.String(), `"id":`) {
		t.Fatalf("events response = %d %s", events.Code, events.Body.String())
	}
}
