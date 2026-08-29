package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/imagegen"
	"lumi/internal/jobqueue"
	"lumi/internal/llm"
	"lumi/internal/production"
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

type blockingHTTPImageProvider struct{ started chan struct{} }

func (provider blockingHTTPImageProvider) Generate(ctx context.Context, _ imagegen.Request) (imagegen.Response, error) {
	close(provider.started)
	<-ctx.Done()
	return imagegen.Response{}, ctx.Err()
}

type immediateHTTPImageProvider struct{ content []byte }

func (provider immediateHTTPImageProvider) Generate(context.Context, imagegen.Request) (imagegen.Response, error) {
	return imagegen.Response{Bytes: provider.content, MIMEType: "image/png"}, nil
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
	queue.WithImageClient(immediateHTTPImageProvider{content: apiPNG(t)})
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

func TestComicImageBatchHandlerUsesPublicEnvelopeAndPreflightStatuses(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	_, err = providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "test/story-model", DefaultImageModel: "openai/gpt-image-1.5", APIKey: "batch-handler-secret"})
	if err != nil {
		t.Fatal(err)
	}
	queue := jobqueue.NewManager(providers, immediateModelFake{}, nil)
	projects := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen).WithRuntime(queue).WithOpenHook(queue.StartProject)
	createdProject, err := projects.CreateWithInput(ctx, project.CreateInput{Name: "Batch API", PictureBook: &project.PictureBookInput{Format: project.PictureBookVertical}}, project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var chapter story.Chapter
	sections := make([]production.ComicSection, 0, 4)
	if err := projects.WithCurrentStore(ctx, createdProject.UUID, func(store *project.Store) error {
		var createErr error
		chapter, createErr = story.NewService(store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch02", Title: "Batch"})
		if createErr != nil {
			return createErr
		}
		service := production.NewService(store, nil)
		for _, title := range []string{"First", "Second", "Conflict candidate", "Active"} {
			section, sectionErr := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: title, StoryboardMD: title + " storyboard"})
			if sectionErr != nil {
				return sectionErr
			}
			sections = append(sections, section)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = projects.Close(); _ = app.Close() })

	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	handler := NewProductionHandler(projects, queue, nil)
	e.POST("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-image-generation-batches", handler.GenerateChapterImagesBatch)
	url := "/api/v1/projects/" + createdProject.UUID + "/chapters/" + chapter.UUID + "/comic-image-generation-batches"
	response := requestJSON(t, e, "POST", url, map[string]any{
		"section_uuids": []string{sections[1].UUID, sections[0].UUID}, "idempotency_key": "http-comic-batch",
	})
	if response.Code != http.StatusCreated || strings.Contains(response.Body.String(), `"id":`) || strings.Contains(response.Body.String(), "river") || strings.Contains(response.Body.String(), "batch-handler-secret") {
		t.Fatalf("batch response=%d %s", response.Code, response.Body.String())
	}
	data := envelopeData(t, response)
	tasks, ok := data["tasks"].([]any)
	if !ok || data["chapter_uuid"] != chapter.UUID || data["requested_count"] != float64(2) || data["accepted_count"] != float64(2) || len(tasks) != 2 {
		t.Fatalf("batch data=%+v", data)
	}
	for index, raw := range tasks {
		task := raw.(map[string]any)
		if task["resource_uuid"] != []string{sections[1].UUID, sections[0].UUID}[index] || task["kind"] != jobqueue.KindComicImageGeneration || task["status"] != jobqueue.StatusQueued || !productionUUIDv7(task["uuid"]) {
			t.Fatalf("task[%d]=%+v", index, task)
		}
	}
	for _, raw := range tasks {
		taskUUID := raw.(map[string]any)["uuid"].(string)
		deadline := time.Now().Add(5 * time.Second)
		terminal := false
		for time.Now().Before(deadline) {
			task, getErr := queue.GetProductionTask(ctx, createdProject.UUID, taskUUID)
			if getErr == nil && task.Status != jobqueue.StatusQueued && task.Status != jobqueue.StatusRunning {
				terminal = true
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if !terminal {
			t.Fatalf("batch task %s did not leave active status", taskUUID)
		}
	}

	invalid := requestJSON(t, e, "POST", url, map[string]any{"section_uuids": []string{sections[0].UUID, sections[0].UUID}, "idempotency_key": "http-comic-batch-duplicate"})
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"success":false`) || !strings.Contains(invalid.Body.String(), `"data":null`) || !strings.Contains(invalid.Body.String(), `"code":"invalid_task"`) {
		t.Fatalf("invalid response=%d %s", invalid.Code, invalid.Body.String())
	}

	blocker := blockingHTTPImageProvider{started: make(chan struct{})}
	queue.WithImageClient(blocker)
	active, err := queue.CreateComicImageGeneration(ctx, createdProject.UUID, chapter.UUID, sections[3].UUID, jobqueue.CreateProductionGenerationInput{IdempotencyKey: "http-active-image"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocker.started:
	case <-time.After(5 * time.Second):
		t.Fatal("active HTTP image task did not start")
	}
	conflict := requestJSON(t, e, "POST", url, map[string]any{"section_uuids": []string{sections[2].UUID, sections[3].UUID}, "idempotency_key": "http-comic-batch-conflict"})
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"code":"task_conflict"`) || strings.Contains(conflict.Body.String(), active.IdempotencyKey) {
		t.Fatalf("conflict response=%d %s", conflict.Code, conflict.Body.String())
	}
	_, _ = queue.CancelProductionTask(ctx, createdProject.UUID, active.UUID)
}
