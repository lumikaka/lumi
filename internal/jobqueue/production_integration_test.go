package jobqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/files"
	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/modelsettings"
	"lumi/internal/picturebook"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/providerdiag"
	"lumi/internal/story"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type successfulImageProvider struct{ content []byte }

func (provider successfulImageProvider) Generate(context.Context, imagegen.Request) (imagegen.Response, error) {
	return imagegen.Response{Bytes: provider.content, MIMEType: "image/png"}, nil
}

func TestPictureBookImageTaskFreezesProfileAndExactOutputSize(t *testing.T) {
	layout := project.ComicLayoutFourPanel
	harness := newQueueHarnessWithPictureBook(t, &project.PictureBookInput{
		Format:      project.PictureBookComicStory,
		AspectRatio: &project.AspectRatioInput{Mode: project.AspectSquare},
		ComicLayout: &layout,
	})
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch31")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Four panels", StoryboardMD: "Exactly four moments."})
	if err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{IdempotencyKey: "picture-book-size-freeze"})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot production.GenerationSnapshot
	if err := json.Unmarshal(task.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 4 || snapshot.PictureBook == nil || snapshot.PictureBook.Format != project.PictureBookComicStory || snapshot.PictureBook.ComicLayout == nil || *snapshot.PictureBook.ComicLayout != project.ComicLayoutFourPanel || snapshot.OutputSize != "1024x1024" {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}

func TestUnsupportedPictureBookImageRatioIsRejectedBeforeTaskPersistence(t *testing.T) {
	harness := newQueueHarnessWithPictureBook(t, &project.PictureBookInput{Format: project.PictureBookClassic})
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch32")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Unsupported", StoryboardMD: "A 4:3 page."})
	if err != nil {
		t.Fatal(err)
	}
	_, err = harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{IdempotencyKey: "unsupported-picture-book-size"})
	var taskErr *Error
	if !errors.As(err, &taskErr) || taskErr.Code != picturebook.CodeAspectRatioUnsupported {
		t.Fatalf("error=%v", err)
	}
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		var count int64
		if err := store.DB().Table("production_task_runs").Where("idempotency_key=?", "unsupported-picture-book-size").Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			t.Fatalf("persisted tasks=%d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestComicImageTaskAndVisibleWorkflowFreezeProjectModelSources(t *testing.T) {
	harness := newQueueHarness(t)
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		_, err := modelsettings.NewResolver(harness.queue.providers).Patch(ctx, store, modelsettings.PatchInput{ExpectedRevision: 0, Changes: map[string]*modelsettings.Selection{
			modelsettings.ProjectImage:            {ProviderUUID: harness.provider.UUID, Model: harness.provider.DefaultImageModel},
			modelsettings.SectionPremiseSelection: {ProviderUUID: harness.provider.UUID, Model: harness.provider.DefaultModel},
		}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch26")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Frozen models", StoryboardMD: "## Section Core Plot Goal\nFreeze models.\n\n## Key Visual Moments\n**Moment 1: Freeze**"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{IdempotencyKey: "comic-project-model-freeze"})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot production.GenerationSnapshot
	if err := json.Unmarshal(task.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if task.ModelSource != modelsettings.SourceProjectImageOverride || snapshot.ModelSource != modelsettings.SourceProjectImageOverride || snapshot.SelectionModelSource != modelsettings.SourceScenarioOverride || snapshot.Model != harness.provider.DefaultImageModel || snapshot.SelectionModel != harness.provider.DefaultModel {
		t.Fatalf("production model snapshot task=%+v snapshot=%+v", task, snapshot)
	}
	type visibleWorkflowModels struct {
		WorkflowSource string
		ThreadSource   string
	}
	var visible visibleWorkflowModels
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return store.DB().Raw(`SELECT w.model_source AS workflow_source,t.model_source AS thread_source FROM workflows w JOIN chat_threads t ON t.id=w.thread_id JOIN workflow_steps s ON s.workflow_id=w.id WHERE s.task_uuid=?`, task.UUID).Scan(&visible).Error
	}); err != nil {
		t.Fatal(err)
	}
	if visible.WorkflowSource != modelsettings.SourceProjectImageOverride || visible.ThreadSource != modelsettings.SourceProjectImageOverride {
		t.Fatalf("visible workflow sources=%+v", visible)
	}
}

type countingImageProvider struct {
	mu    sync.Mutex
	calls int
}

func (provider *countingImageProvider) Generate(context.Context, imagegen.Request) (imagegen.Response, error) {
	provider.mu.Lock()
	provider.calls++
	provider.mu.Unlock()
	return imagegen.Response{MIMEType: "image/png", Bytes: []byte("image")}, nil
}

func (provider *countingImageProvider) callCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

type failingImageProvider struct{}

func (failingImageProvider) Generate(context.Context, imagegen.Request) (imagegen.Response, error) {
	return imagegen.Response{}, &imagegen.Error{
		Code: "image_provider_error", SafeMessage: "图片 Provider 拒绝了请求。",
		Diagnostic: providerdiag.Details{HTTPStatus: 400, ProviderCode: "InvalidParameter", Message: "unsupported image size", RequestID: "image-request-400"},
	}
}

type recordingBreakdownProvider struct {
	mu      sync.Mutex
	request llm.Request
}

func (provider *recordingBreakdownProvider) Check(context.Context, string, string, string) error {
	return nil
}

func (provider *recordingBreakdownProvider) Generate(_ context.Context, request llm.Request, _ func(string) error) (llm.Response, error) {
	provider.mu.Lock()
	provider.request = request
	provider.mu.Unlock()
	return llm.Response{Content: `{"plan":{"layout":"white_background_objects"},"assets":[{"filename":"lighthouse.png","title":"黄昏灯塔","summary":"引导归航的核心地点","tags":["地点","核心地点"],"crop_box":{"x":0,"y":0,"width":1,"height":1},"confidence":0.98}],"quality_checks":["主体完整"]}`}, nil
}

func (provider *recordingBreakdownProvider) snapshot() llm.Request {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.request
}

type recordingImageProvider struct {
	mu       sync.Mutex
	content  []byte
	requests []imagegen.Request
}

type recordingSelectionProvider struct {
	mu        sync.Mutex
	sectionID string
	title     string
	titles    []string
	request   llm.Request
	calls     int
}

func (*recordingSelectionProvider) Check(context.Context, string, string, string) error { return nil }

func (provider *recordingSelectionProvider) Generate(_ context.Context, request llm.Request, _ func(string) error) (llm.Response, error) {
	provider.mu.Lock()
	provider.request = request
	provider.calls++
	titles := append([]string(nil), provider.titles...)
	if len(titles) == 0 {
		titles = []string{provider.title}
	}
	provider.mu.Unlock()
	encoded, _ := json.Marshal(map[string]any{"sectionId": provider.sectionID, "titles": titles, "reason": "直接出现在分镜中"})
	return llm.Response{Content: string(encoded)}, nil
}

func (provider *recordingSelectionProvider) callCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

func (provider *recordingSelectionProvider) snapshot() llm.Request {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.request
}

func (provider *recordingImageProvider) Generate(_ context.Context, request imagegen.Request) (imagegen.Response, error) {
	provider.mu.Lock()
	provider.requests = append(provider.requests, request)
	provider.mu.Unlock()
	return imagegen.Response{Bytes: provider.content, MIMEType: "image/png"}, nil
}

func (provider *recordingImageProvider) snapshot() []imagegen.Request {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return append([]imagegen.Request(nil), provider.requests...)
}

type blockingImageProvider struct{ started chan struct{} }

func (provider blockingImageProvider) Generate(ctx context.Context, _ imagegen.Request) (imagegen.Response, error) {
	close(provider.started)
	<-ctx.Done()
	return imagegen.Response{}, ctx.Err()
}

type restartingImageProvider struct {
	mu       sync.Mutex
	attempts int
	started  chan struct{}
	content  []byte
	requests []imagegen.Request
}

type failFirstRecordingImageProvider struct {
	mu       sync.Mutex
	requests []imagegen.Request
	content  []byte
}

func (provider *failFirstRecordingImageProvider) Generate(_ context.Context, request imagegen.Request) (imagegen.Response, error) {
	provider.mu.Lock()
	provider.requests = append(provider.requests, request)
	attempt := len(provider.requests)
	provider.mu.Unlock()
	if attempt == 1 {
		return imagegen.Response{}, &imagegen.Error{Code: "image_provider_error", SafeMessage: "首次图片调用失败。", Retryable: false}
	}
	return imagegen.Response{Bytes: provider.content, MIMEType: "image/png"}, nil
}

func (provider *failFirstRecordingImageProvider) snapshot() []imagegen.Request {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return append([]imagegen.Request(nil), provider.requests...)
}

func (provider *restartingImageProvider) Generate(ctx context.Context, request imagegen.Request) (imagegen.Response, error) {
	provider.mu.Lock()
	provider.attempts++
	attempt := provider.attempts
	provider.requests = append(provider.requests, request)
	provider.mu.Unlock()
	if attempt == 1 {
		close(provider.started)
		<-ctx.Done()
		return imagegen.Response{}, ctx.Err()
	}
	return imagegen.Response{Bytes: provider.content, MIMEType: "image/png"}, nil
}

func (provider *restartingImageProvider) snapshot() []imagegen.Request {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return append([]imagegen.Request(nil), provider.requests...)
}

func productionPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	img.Set(0, 0, color.RGBA{R: 200, G: 80, B: 30, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func waitProductionStatus(t *testing.T, manager *Manager, projectUUID, taskUUID, wanted string) ProductionTask {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var last ProductionTask
	var err error
	for time.Now().Before(deadline) {
		last, err = manager.GetProductionTask(context.Background(), projectUUID, taskUUID)
		if err == nil && last.Status == wanted {
			return last
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("production task %s did not reach %s; last=%+v err=%v", taskUUID, wanted, last, err)
	return ProductionTask{}
}

func waitProductionLogStatus(t *testing.T, harness *queueHarness, taskUUID, wantedStatus string, wantedCount int64) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var count int64
	var err error
	for time.Now().Before(deadline) {
		count = 0
		err = harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error {
			return store.DB().Table("llm_logs AS logs").
				Joins("JOIN production_task_runs tasks ON tasks.id=logs.production_task_run_id").
				Where("tasks.uuid=? AND logs.status=?", taskUUID, wantedStatus).
				Count(&count).Error
		})
		if err == nil && count == wantedCount {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("production task %s logs did not reach status=%s count=%d; last count=%d err=%v", taskUUID, wantedStatus, wantedCount, count, err)
}

func TestComicExportOperationTracksProgressAndReusesCanonicalSnapshot(t *testing.T) {
	harness := newQueueHarness(t)
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch31")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Export progress", StoryboardMD: "One frozen image."})
	if err != nil {
		t.Fatal(err)
	}
	upload, err := service.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "comic_section_image", OriginalFilename: "section.png", Reader: bytes.NewReader(productionPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ImportSectionImage(ctx, chapter.UUID, section.UUID, upload.UUID, section.Revision); err != nil {
		t.Fatal(err)
	}

	input := CreateExportInput{Scope: "chapter", ChapterUUID: chapter.UUID, IdempotencyKey: "comic-export-operation-progress"}
	operation, err := harness.queue.CreateComicExport(ctx, harness.project.UUID, input)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Export.TaskUUID != operation.Task.UUID || operation.Export.ChapterUUID != chapter.UUID || operation.Export.SnapshotHash == "" || !strings.HasSuffix(operation.Export.Filename, ".zip") {
		t.Fatalf("comic export operation=%+v", operation)
	}
	encoded, err := json.Marshal(operation)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"export"`) || !strings.Contains(string(encoded), `"task"`) || !strings.Contains(string(encoded), `"filename"`) {
		t.Fatalf("comic export operation response=%s", encoded)
	}
	for _, forbidden := range []string{`"id":`, `"project_id"`, `"chapter_id"`, `"output_file_id"`, `"river_job_id"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("comic export operation leaked %q: %s", forbidden, encoded)
		}
	}
	replayed, err := harness.queue.CreateComicExport(ctx, harness.project.UUID, input)
	if err != nil || replayed.Task.UUID != operation.Task.UUID || replayed.Export.UUID != operation.Export.UUID {
		t.Fatalf("idempotent operation=%+v err=%v", replayed, err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, operation.Task.UUID, StatusCompleted)

	items, _, err := service.ListExportsPage(ctx, production.ExportFilter{TaskUUID: operation.Task.UUID, SnapshotHash: operation.Export.SnapshotHash, Status: "ready"}, 1, 20)
	if err != nil || len(items) != 1 || items[0].OutputAsset == nil || items[0].OutputAsset.ContentURL == "" {
		t.Fatalf("ready export items=%+v err=%v", items, err)
	}
	events, _, err := harness.queue.ListProductionTaskEvents(ctx, harness.project.UUID, operation.Task.UUID, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	progresses := make([]int, 0, len(events))
	for _, event := range events {
		var payload struct {
			Progress *int `json:"progress"`
		}
		if json.Unmarshal(event.Payload, &payload) == nil && payload.Progress != nil {
			progresses = append(progresses, *payload.Progress)
		}
	}
	wanted := []int{0, 5, 10, 80, 90, 95, 100}
	wantedIndex := 0
	for index, progress := range progresses {
		if index > 0 && progress < progresses[index-1] {
			t.Fatalf("comic export progress regressed: %v", progresses)
		}
		if wantedIndex < len(wanted) && progress == wanted[wantedIndex] {
			wantedIndex++
		}
	}
	if wantedIndex != len(wanted) {
		t.Fatalf("comic export progress=%v, want stages=%v", progresses, wanted)
	}

	reuseInput := CreateExportInput{Scope: "chapter", ChapterUUID: chapter.UUID, IdempotencyKey: "comic-export-canonical-reuse"}
	reused, err := harness.queue.CreateComicExport(ctx, harness.project.UUID, reuseInput)
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, reused.Task.UUID, StatusCompleted)
	canonical, _, err := service.ListExportsPage(ctx, production.ExportFilter{SnapshotHash: operation.Export.SnapshotHash, Status: "ready"}, 1, 20)
	if err != nil || len(canonical) != 1 || canonical[0].UUID != items[0].UUID {
		t.Fatalf("canonical exports=%+v err=%v", canonical, err)
	}
	resolved, err := service.ExportForTaskOrReadySnapshot(ctx, reused.Task.UUID, reused.Export.SnapshotHash)
	if err != nil || resolved.UUID != items[0].UUID {
		t.Fatalf("resolved canonical export=%+v err=%v", resolved, err)
	}
	replayedReuse, err := harness.queue.CreateComicExport(ctx, harness.project.UUID, reuseInput)
	if err != nil || replayedReuse.Task.UUID != reused.Task.UUID || replayedReuse.Export.UUID != items[0].UUID || replayedReuse.Export.OutputAsset == nil {
		t.Fatalf("replayed canonical operation=%+v err=%v", replayedReuse, err)
	}
}

func TestComicImageGenerationCreatesVisibleWorkflowAndTracksLifecycle(t *testing.T) {
	harness := newQueueHarness(t)
	harness.queue.WithImageClient(successfulImageProvider{content: productionPNG(t)})
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch20")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Visible workflow", StoryboardMD: "## Section Core Plot Goal\nShow the visible workflow.\n\n## Key Visual Moments\n**Moment 1: Start**"})
	if err != nil {
		t.Fatal(err)
	}
	input := CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-visible-workflow"}
	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, input)
	if err != nil {
		t.Fatal(err)
	}

	type workflowRow struct {
		UUID, ThreadUUID, Kind, WorkflowStatus, ThreadStatus, InputSnapshot string
	}
	var workflow workflowRow
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return store.DB().Raw(`SELECT w.uuid AS uuid,t.uuid AS thread_uuid,w.kind AS kind,w.status AS workflow_status,t.status AS thread_status,w.input_snapshot AS input_snapshot FROM workflows w JOIN chat_threads t ON t.id=w.thread_id JOIN workflow_steps s ON s.workflow_id=w.id AND s.step_key=? WHERE s.task_uuid=?`, agent.WorkflowStepGenerateSectionImage, task.UUID).Scan(&workflow).Error
	}); err != nil {
		t.Fatal(err)
	}
	if workflow.UUID == "" || workflow.ThreadUUID == "" || workflow.Kind != agent.WorkflowComicSectionImage || !strings.Contains(workflow.InputSnapshot, section.UUID) || !strings.Contains(workflow.InputSnapshot, task.UUID) {
		t.Fatalf("visible workflow not created atomically: %+v", workflow)
	}

	replayed, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, input)
	if err != nil || replayed.UUID != task.UUID {
		t.Fatalf("idempotent replay task=%+v err=%v", replayed, err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)

	type stepRow struct{ StepKey, Status, TaskUUID, ResourceUUID string }
	var steps []stepRow
	var workflowCount, threadCount int64
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		if err := store.DB().Raw(`SELECT step_key,status,task_uuid,resource_uuid FROM workflow_steps WHERE workflow_id=(SELECT id FROM workflows WHERE uuid=?) ORDER BY position`, workflow.UUID).Scan(&steps).Error; err != nil {
			return err
		}
		if err := store.DB().Table("workflows").Where("kind=?", agent.WorkflowComicSectionImage).Count(&workflowCount).Error; err != nil {
			return err
		}
		return store.DB().Table("chat_threads").Where("uuid=?", workflow.ThreadUUID).Count(&threadCount).Error
	}); err != nil {
		t.Fatal(err)
	}
	if workflowCount != 1 || threadCount != 1 || len(steps) != len(agent.ComicSectionImageStepKeys) {
		t.Fatalf("workflow count=%d thread count=%d steps=%+v", workflowCount, threadCount, steps)
	}
	for index, step := range steps {
		if step.StepKey != agent.ComicSectionImageStepKeys[index] || step.Status != "completed" || step.ResourceUUID != section.UUID {
			t.Fatalf("step %d=%+v", index, step)
		}
		if (step.StepKey == agent.WorkflowStepGenerateSectionImage) != (step.TaskUUID == task.UUID) {
			t.Fatalf("task linkage leaked to wrong step: %+v", step)
		}
	}

	second, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-visible-regeneration"})
	if err != nil || second.UUID == task.UUID {
		t.Fatalf("regeneration task=%+v err=%v", second, err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, second.UUID, StatusCompleted)
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return store.DB().Table("workflows").Where("kind=?", agent.WorkflowComicSectionImage).Count(&workflowCount).Error
	}); err != nil || workflowCount != 2 {
		t.Fatalf("regeneration workflows=%d err=%v", workflowCount, err)
	}
}

func TestComicImageBatchCreatesIndependentVisibleWorkflows(t *testing.T) {
	harness := newQueueHarness(t)
	harness.queue.WithImageClient(successfulImageProvider{content: productionPNG(t)})
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch24")
	sectionInputs := []production.CreateSectionInput{
		{Title: "Batch one", StoryboardMD: "## Section Core Plot Goal\nCreate batch image one.\n\n## Key Visual Moments\n**Moment 1: One**"},
		{Title: "Batch two", StoryboardMD: "## Section Core Plot Goal\nCreate batch image two.\n\n## Key Visual Moments\n**Moment 1: Two**"},
	}
	taskUUIDs := make([]string, 0, len(sectionInputs))
	for index, sectionInput := range sectionInputs {
		section, err := service.CreateSection(ctx, chapter.UUID, sectionInput)
		if err != nil {
			t.Fatal(err)
		}
		task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{
			ProviderUUID: harness.provider.UUID, IdempotencyKey: fmt.Sprintf("comic-visible-batch-%d", index+1),
		})
		if err != nil {
			t.Fatal(err)
		}
		taskUUIDs = append(taskUUIDs, task.UUID)
	}
	for _, taskUUID := range taskUUIDs {
		waitProductionStatus(t, harness.queue, harness.project.UUID, taskUUID, StatusCompleted)
	}

	type batchCounts struct{ Workflows, Threads, Tasks int64 }
	var counts batchCounts
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return store.DB().Raw(`SELECT COUNT(DISTINCT w.id) AS workflows,COUNT(DISTINCT w.thread_id) AS threads,COUNT(DISTINCT s.task_uuid) AS tasks FROM workflows w JOIN workflow_steps s ON s.workflow_id=w.id WHERE w.kind=? AND s.task_uuid<>''`, agent.WorkflowComicSectionImage).Scan(&counts).Error
	}); err != nil {
		t.Fatal(err)
	}
	if counts.Workflows != 2 || counts.Threads != 2 || counts.Tasks != 2 || taskUUIDs[0] == taskUUIDs[1] {
		t.Fatalf("batch task UUIDs=%v counts=%+v", taskUUIDs, counts)
	}
}

func TestComicImageWorkflowRetryUsesProductionTaskInsteadOfAgentJob(t *testing.T) {
	harness := newQueueHarness(t)
	imageProvider := &failFirstRecordingImageProvider{content: productionPNG(t)}
	harness.queue.WithImageClient(imageProvider)
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch21")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Retry workflow", StoryboardMD: "## Section Core Plot Goal\nRetry the image.\n\n## Key Visual Moments\n**Moment 1: Retry**"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-workflow-retry"})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusFailed)
	agents := agent.NewService(harness.projects, harness.queue.providers, newRiverAgentModel(), harness.queue, nil)
	var workflowUUID string
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return store.DB().Raw(`SELECT w.uuid FROM workflows w JOIN workflow_steps s ON s.workflow_id=w.id WHERE s.task_uuid=?`, task.UUID).Scan(&workflowUUID).Error
	}); err != nil || workflowUUID == "" {
		t.Fatalf("workflow uuid=%q err=%v", workflowUUID, err)
	}
	failed, err := agents.GetWorkflow(ctx, harness.project.UUID, workflowUUID)
	if err != nil || failed.Status != agent.WorkflowFailed {
		t.Fatalf("failed workflow=%+v err=%v", failed, err)
	}
	retried, err := agents.RetryWorkflow(ctx, harness.project.UUID, workflowUUID)
	if err != nil || retried.Status != agent.WorkflowQueued {
		t.Fatalf("retried workflow=%+v err=%v", retried, err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	completed, err := agents.GetWorkflow(ctx, harness.project.UUID, workflowUUID)
	if err != nil || completed.Status != agent.WorkflowCompleted || imageProvider.snapshot() == nil || len(imageProvider.snapshot()) != 2 {
		t.Fatalf("completed workflow=%+v calls=%d err=%v", completed, len(imageProvider.snapshot()), err)
	}
}

func TestComicImageWorkflowCancelTargetsProductionTask(t *testing.T) {
	harness := newQueueHarness(t)
	started := make(chan struct{})
	harness.queue.WithImageClient(blockingImageProvider{started: started})
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch22")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Cancel workflow", StoryboardMD: "## Section Core Plot Goal\nCancel the image.\n\n## Key Visual Moments\n**Moment 1: Cancel**"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-workflow-cancel"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("image provider did not start")
	}
	var workflowUUID string
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return store.DB().Raw(`SELECT w.uuid FROM workflows w JOIN workflow_steps s ON s.workflow_id=w.id WHERE s.task_uuid=?`, task.UUID).Scan(&workflowUUID).Error
	}); err != nil || workflowUUID == "" {
		t.Fatalf("workflow uuid=%q err=%v", workflowUUID, err)
	}
	agents := agent.NewService(harness.projects, harness.queue.providers, newRiverAgentModel(), harness.queue, nil)
	cancelled, err := agents.CancelWorkflow(ctx, harness.project.UUID, workflowUUID)
	if err != nil || cancelled.Status != agent.WorkflowCancelled {
		t.Fatalf("cancelled workflow=%+v err=%v", cancelled, err)
	}
	cancelledTask := waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCancelled)
	if cancelledTask.CancelRequestedAt == nil {
		t.Fatalf("cancelled task missing cancel_requested_at: %+v", cancelledTask)
	}
	for _, step := range cancelled.Steps {
		if step.Status != "completed" && step.Status != "cancelled" {
			t.Fatalf("cancel left active step: %+v", step)
		}
	}
}

func TestComicImageWorkflowCreationRollsBackEveryRecordOnScaffoldFailure(t *testing.T) {
	harness := newQueueHarness(t)
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch23")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Atomic workflow", StoryboardMD: "## Section Core Plot Goal\nCreate atomically.\n\n## Key Visual Moments\n**Moment 1: Transaction**"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return store.DB().Exec(`CREATE TRIGGER reject_comic_workflow BEFORE INSERT ON workflows WHEN NEW.kind='comic_section_image_generation' BEGIN SELECT RAISE(ABORT, 'injected workflow failure'); END`).Error
	}); err != nil {
		t.Fatal(err)
	}
	_, err = harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-atomic-failure"})
	if err == nil {
		t.Fatal("expected injected workflow creation failure")
	}
	var tasks, generations, threads, workflows int64
	if queryErr := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		if err := store.DB().Table("production_task_runs").Where("idempotency_key=?", "comic-atomic-failure").Count(&tasks).Error; err != nil {
			return err
		}
		if err := store.DB().Table("comic_image_generations").Where("comic_section_id=(SELECT id FROM comic_sections WHERE uuid=?)", section.UUID).Count(&generations).Error; err != nil {
			return err
		}
		if err := store.DB().Table("chat_threads").Where("title=? AND subject_uuid=?", comicWorkflowTitle, section.UUID).Count(&threads).Error; err != nil {
			return err
		}
		return store.DB().Table("workflows").Where("kind=?", agent.WorkflowComicSectionImage).Count(&workflows).Error
	}); queryErr != nil || tasks != 0 || generations != 0 || threads != 0 || workflows != 0 {
		t.Fatalf("rollback tasks=%d generations=%d threads=%d workflows=%d err=%v", tasks, generations, threads, workflows, queryErr)
	}
}

func TestProductionImageTaskUsesRiverSnapshotAndDomainIdempotency(t *testing.T) {
	harness := newQueueHarness(t)
	harness.queue.WithImageClient(successfulImageProvider{content: productionPNG(t)})
	var service *production.Service
	if err := harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error { service = production.NewService(store, nil); return nil }); err != nil {
		t.Fatal(err)
	}
	source, err := service.CreatePremiseSource(context.Background(), production.CreateSourceInput{SourceText: "A lighthouse at dusk", StyleSnapshot: "paper cutout", SourceType: "manual", Parameters: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	input := CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, Prompt: "draw a setting sheet", IdempotencyKey: "setting-once"}
	created, err := harness.queue.CreatePremiseSettingGeneration(context.Background(), harness.project.UUID, source.UUID, input)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := harness.queue.CreatePremiseSettingGeneration(context.Background(), harness.project.UUID, source.UUID, input)
	if err != nil || duplicate.UUID != created.UUID {
		t.Fatalf("duplicate=%+v err=%v", duplicate, err)
	}
	completed := waitProductionStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCompleted)
	if completed.Progress != 100 || completed.Kind != KindPremiseSettingGeneration {
		t.Fatalf("completed=%+v", completed)
	}
	encoded, err := json.Marshal(completed)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"river_job_id", `"id"`, "project-must-never-contain-this-api-key"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public task contains %q: %s", forbidden, encoded)
		}
	}
	images, err := service.ListSettingImages(context.Background())
	if err != nil || len(images) != 1 || images[0].Asset.ContentURL == "" {
		t.Fatalf("settings=%+v err=%v", images, err)
	}
	var steps, tasks int64
	if _, err := service.Files().GetAsset(context.Background(), images[0].Asset.UUID, false); err != nil {
		t.Fatal(err)
	}
	if err := harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error {
		if err := store.DB().Table("premise_generation_steps").Where("task_uuid=? AND status='completed'", created.UUID).Count(&steps).Error; err != nil {
			return err
		}
		return store.DB().Table("production_task_runs").Where("idempotency_key='setting-once'").Count(&tasks).Error
	}); err != nil {
		t.Fatal(err)
	}
	if steps != 1 || tasks != 1 {
		t.Fatalf("steps=%d tasks=%d", steps, tasks)
	}
	var logStatus, requestType, scenario, requestPayload, responsePayload string
	if err := harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error {
		return store.DB().Raw(`SELECT logs.status,logs.request_type,logs.scenario,logs.request_payload,logs.response FROM llm_logs logs JOIN production_task_runs tasks ON tasks.id=logs.production_task_run_id WHERE tasks.uuid=?`, created.UUID).Row().Scan(&logStatus, &requestType, &scenario, &requestPayload, &responsePayload)
	}); err != nil || logStatus != "completed" || requestType != "image" || scenario != KindPremiseSettingGeneration {
		t.Fatalf("setting log status=%q type=%q scenario=%q err=%v", logStatus, requestType, scenario, err)
	}
	if !json.Valid([]byte(requestPayload)) || !json.Valid([]byte(responsePayload)) || !strings.Contains(requestPayload, "draw a setting sheet") || !strings.Contains(responsePayload, `"mime_type":"image/png"`) || strings.Contains(requestPayload+responsePayload, "iVBOR") {
		t.Fatalf("unsafe or incomplete image snapshots: request=%s response=%s", requestPayload, responsePayload)
	}
	events, pagination, err := harness.queue.ListProductionTaskEvents(context.Background(), harness.project.UUID, created.UUID, 0, 0, 20)
	if err != nil || pagination.HasMore || len(events) < 3 || events[0].EventType != "task_queued" || events[len(events)-1].EventType != "task_completed" {
		t.Fatalf("events=%+v pagination=%+v err=%v", events, pagination, err)
	}
	for _, event := range events {
		if strings.Contains(string(event.Payload), `"id"`) || strings.Contains(string(event.Payload), "river_job_id") {
			t.Fatalf("event leaked internal identity: %s", event.Payload)
		}
	}
}

func TestPremiseBatchPromptsAndVisualBreakdownUseCatalogContract(t *testing.T) {
	harness := newQueueHarness(t)
	imageProvider := &recordingImageProvider{content: productionPNG(t)}
	breakdownProvider := &recordingBreakdownProvider{}
	harness.queue.WithImageClient(imageProvider)
	harness.queue.llm = breakdownProvider
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	source, err := service.CreatePremiseSource(ctx, production.CreateSourceInput{
		SourceText: "天空城中的黄昏灯塔为迷航者指路。", StyleSnapshot: "纸雕赛璐璐风格", SourceType: "generated", Parameters: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	settingTask, err := harness.queue.CreatePremiseSettingGeneration(ctx, harness.project.UUID, source.UUID, CreateProductionGenerationInput{
		ProviderUUID: harness.provider.UUID, Prompt: source.SourceText, IdempotencyKey: "premise-setting-prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, settingTask.UUID, StatusCompleted)
	imageRequests := imageProvider.snapshot()
	if len(imageRequests) != 1 {
		t.Fatalf("image requests = %d", len(imageRequests))
	}
	settingPrompt := imageRequests[0].Prompt
	for _, required := range []string{"漫画项目的设定图设计师", "纯白背景", "6 到 12 个", source.SourceText, "纸雕赛璐璐风格"} {
		if !strings.Contains(settingPrompt, required) {
			t.Fatalf("setting prompt missing %q: %s", required, settingPrompt)
		}
	}
	if strings.Contains(settingPrompt, "{{input_text}}") || strings.Contains(settingPrompt, "根据 STORY.md") {
		t.Fatalf("setting prompt was not fully rendered: %s", settingPrompt)
	}
	settings, err := service.ListSettingImages(ctx)
	if err != nil || len(settings) != 1 {
		t.Fatalf("settings=%+v err=%v", settings, err)
	}
	breakdownTask, err := harness.queue.CreatePremiseBreakdown(ctx, harness.project.UUID, settings[0].UUID, CreateProductionGenerationInput{
		ProviderUUID: harness.provider.UUID, Prompt: "legacy caller guidance", IdempotencyKey: "premise-breakdown-prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, breakdownTask.UUID, StatusCompleted)
	request := breakdownProvider.snapshot()
	for _, required := range []string{"漫画制作资产整理员", `"assets"`, `"crop_box"`, source.SourceText, "纸雕赛璐璐风格", `"width":8`, `"height":6`} {
		if !strings.Contains(request.Prompt, required) {
			t.Fatalf("breakdown prompt missing %q: %s", required, request.Prompt)
		}
	}
	if len(request.Images) != 1 || request.Images[0].MIMEType != "image/png" || request.Images[0].Detail != "high" || !bytes.Equal(request.Images[0].Data, productionPNG(t)) {
		t.Fatalf("breakdown image input = %+v", request.Images)
	}
	assets, err := service.ListPremiseAssets(ctx, "", "active")
	if err != nil || len(assets) != 1 || assets[0].AssetType != production.AssetScene || assets[0].Title != "黄昏灯塔" {
		t.Fatalf("assets=%+v err=%v", assets, err)
	}
}

func TestEnglishPremiseBreakdownAndComicRequestsUseLocalizedContextAndImages(t *testing.T) {
	harness := newQueueHarness(t)
	ctx := context.Background()
	detail, err := harness.stories.GetProject(ctx)
	if err != nil {
		t.Fatal(err)
	}
	language := project.GenerationLanguageEnglish
	if _, err := harness.stories.UpdateProject(ctx, story.UpdateProjectInput{Name: detail.Name, Description: detail.Description, GenerationLanguage: &language, ExpectedRevision: detail.Revision}); err != nil {
		t.Fatal(err)
	}
	imageProvider := &recordingImageProvider{content: productionPNG(t)}
	breakdownProvider := &recordingBreakdownProvider{}
	harness.queue.WithImageClient(imageProvider)
	harness.queue.llm = breakdownProvider
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	source, err := service.CreatePremiseSource(ctx, production.CreateSourceInput{SourceText: "A dusk lighthouse guides lost travelers.", StyleSnapshot: "paper-cut cel animation", SourceType: "generated"})
	if err != nil {
		t.Fatal(err)
	}
	settingTask, err := harness.queue.CreatePremiseSettingGeneration(ctx, harness.project.UUID, source.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "setting-en"})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, settingTask.UUID, StatusCompleted)
	settingRequest := imageProvider.snapshot()[0]
	for _, required := range []string{"Project language: English", "setting image designer", "pure white background", source.SourceText, source.StyleSnapshot} {
		if !strings.Contains(settingRequest.Prompt, required) {
			t.Fatalf("English setting prompt missing %q: %s", required, settingRequest.Prompt)
		}
	}
	settings, err := service.ListSettingImages(ctx)
	if err != nil || len(settings) != 1 {
		t.Fatalf("settings=%+v err=%v", settings, err)
	}
	breakdownTask, err := harness.queue.CreatePremiseBreakdown(ctx, harness.project.UUID, settings[0].UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "breakdown-en"})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, breakdownTask.UUID, StatusCompleted)
	breakdownRequest := breakdownProvider.snapshot()
	for _, required := range []string{"Project language: English", "comic-production asset organizer", `"crop_box"`, source.SourceText} {
		if !strings.Contains(breakdownRequest.Prompt, required) {
			t.Fatalf("English breakdown prompt missing %q: %s", required, breakdownRequest.Prompt)
		}
	}
	if len(breakdownRequest.Images) != 1 || !bytes.Equal(breakdownRequest.Images[0].Data, productionPNG(t)) {
		t.Fatalf("English breakdown image=%+v", breakdownRequest.Images)
	}
	assets, err := service.ListPremiseAssets(ctx, "", "active")
	if err != nil || len(assets) != 1 {
		t.Fatalf("assets=%+v err=%v", assets, err)
	}
	chapter := harness.createChapter(t, "vol01.ch12")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Lighthouse", StoryboardMD: "## Section Core Plot Goal\nReach the lighthouse.\n\n## Key Visual Moments\n**Moment 1: Arrival**"})
	if err != nil {
		t.Fatal(err)
	}
	selector := &recordingSelectionProvider{sectionID: section.UUID, title: assets[0].Title}
	harness.queue.llm = selector
	comicTask, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-en"})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, comicTask.UUID, StatusCompleted)
	if !strings.Contains(selector.snapshot().Prompt, "comic setting-asset selector") || !strings.Contains(selector.snapshot().Prompt, "Project language: English") {
		t.Fatalf("English selection prompt=%s", selector.snapshot().Prompt)
	}
	requests := imageProvider.snapshot()
	comicRequest := requests[len(requests)-1]
	if !strings.Contains(comicRequest.Prompt, "## Generation Rules") || !strings.Contains(comicRequest.Prompt, "Project language: English") || len(comicRequest.Images) != 1 {
		t.Fatalf("English comic request=%+v", comicRequest)
	}
}

func TestPremiseSingleAssetAndReferencedVariantUseGoQueueAndAssetStore(t *testing.T) {
	harness := newQueueHarness(t)
	provider := &recordingImageProvider{content: productionPNG(t)}
	harness.queue.WithImageClient(provider)
	ctx := context.Background()
	if _, err := harness.stories.CreatePromptVersion(ctx, story.CreatePromptInput{PromptGroup: "premise", PromptKey: "single_asset_generation", Prompt: "统一的 Premise 单项生成模板\n{{input_text}}", ExpectedCurrentVersion: 1}); err != nil {
		t.Fatal(err)
	}
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	threadUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	input := CreateProductionGenerationInput{
		ProviderUUID:   harness.provider.UUID,
		Prompt:         "夜蓝木屋与暖黄窗光",
		AssetOperation: "create",
		AssetType:      production.AssetScene,
		AssetTitle:     "月亮邮局",
		AssetSummary:   "月光邮差的夜间据点",
		AssetTags:      []string{"Night", "post-office", "night"},
		IdempotencyKey: "single-premise-asset",
	}
	created, err := harness.queue.CreatePremiseAssetGeneration(ctx, harness.project.UUID, threadUUID, input)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := harness.queue.CreatePremiseAssetGeneration(ctx, harness.project.UUID, threadUUID, input)
	if err != nil || duplicate.UUID != created.UUID {
		t.Fatalf("duplicate=%+v err=%v", duplicate, err)
	}
	var createSnapshot production.GenerationSnapshot
	if err := json.Unmarshal(created.InputSnapshot, &createSnapshot); err != nil {
		t.Fatal(err)
	}
	if createSnapshot.AssetOperation != "create" || createSnapshot.AssetTitle != "月亮邮局" || !strings.Contains(createSnapshot.Prompt, "统一的 Premise 单项生成模板") {
		t.Fatalf("create snapshot=%+v", createSnapshot)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCompleted)
	assets, err := service.ListPremiseAssets(ctx, "", "active")
	if err != nil || len(assets) != 1 {
		t.Fatalf("assets=%+v err=%v", assets, err)
	}
	asset := assets[0]
	if asset.Title != "月亮邮局" || asset.CurrentVariant == nil || asset.CurrentVariant.VersionNo != 1 || len(asset.Tags) != 2 || asset.Tags[0] != "night" {
		t.Fatalf("created asset=%+v", asset)
	}
	committed, found, err := service.PremiseAssetForGenerationTask(ctx, created.UUID)
	if err != nil || !found || committed.UUID != asset.UUID {
		t.Fatalf("committed=%+v found=%v err=%v", committed, found, err)
	}

	variantTask, err := harness.queue.CreatePremiseAssetGeneration(ctx, harness.project.UUID, asset.UUID, CreateProductionGenerationInput{
		ProviderUUID:   harness.provider.UUID,
		Prompt:         "保持建筑身份，增加铜制冬季装饰",
		AssetOperation: "variant",
		IdempotencyKey: "premise-asset-variant",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, variantTask.UUID, StatusCompleted)
	updated, err := service.GetPremiseAsset(ctx, asset.UUID)
	if err != nil {
		t.Fatal(err)
	}
	variants, err := service.ListAssetVariants(ctx, asset.UUID)
	if err != nil || len(variants) != 2 || updated.CurrentVariant == nil || updated.CurrentVariant.VersionNo != 2 || updated.Revision != asset.Revision+1 {
		t.Fatalf("updated=%+v variants=%+v err=%v", updated, variants, err)
	}
	requests := provider.snapshot()
	if len(requests) != 2 || requests[0].Size != "1024x1024" || requests[1].Size != "1024x1024" || !strings.Contains(requests[1].Prompt, "统一的 Premise 单项生成模板") || !strings.Contains(requests[1].Prompt, "保持建筑身份") || !strings.Contains(requests[1].Prompt, "项目语言：简体中文") {
		t.Fatalf("image requests=%+v", requests)
	}
}

func TestComicGenerationSnapshotFreezesCurrentStoryboardAndPremiseVariants(t *testing.T) {
	harness := newQueueHarness(t)
	imageProvider := &recordingImageProvider{content: productionPNG(t)}
	harness.queue.WithImageClient(imageProvider)
	ctx := context.Background()
	if _, err := harness.stories.UpdatePromptGroup(ctx, story.UpdatePromptGroupInput{
		PromptGroup: "chapter",
		Prompts: map[string]string{
			"before_image":                 "FROZEN BASE IMAGE RULES",
			"section_reference_present":    "FROZEN REFERENCES\n{{reference_titles}}",
			"section_reference_absent":     "FROZEN NO REFERENCES",
			"section_additional_direction": "FROZEN DIRECTION\n{{guidance_prompt}}",
		},
		ExpectedCurrentVersions: map[string]int{
			"before_image": 1, "section_reference_present": 1,
			"section_reference_absent": 1, "section_additional_direction": 1,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.stories.UpdatePromptGroup(ctx, story.UpdatePromptGroupInput{
		PromptGroup:             "runtime",
		Prompts:                 map[string]string{"project_language_instruction": "FROZEN COMIC LANGUAGE"},
		ExpectedCurrentVersions: map[string]int{"project_language_instruction": 1},
	}); err != nil {
		t.Fatal(err)
	}
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch09")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Frozen", StoryboardMD: "A precise frame"})
	if err != nil {
		t.Fatal(err)
	}
	upload, err := service.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "reference.png", Reader: bytes.NewReader(productionPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := service.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetReference, Title: "Frozen reference"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, Prompt: "Keep a silver border", PremiseAssetUUIDs: []string{asset.UUID}, IdempotencyKey: "comic-snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot production.GenerationSnapshot
	if err := json.Unmarshal(task.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 4 || snapshot.PictureBook == nil || snapshot.PictureBook.Format != project.PictureBookVertical || snapshot.OutputSize != "1024x1536" || snapshot.StoryboardUUID != section.CurrentStoryboard.UUID || len(snapshot.PremiseAssets) != 1 || snapshot.PremiseAssets[0].AssetUUID != asset.UUID || snapshot.PremiseAssets[0].VariantUUID != asset.CurrentVariant.UUID || !strings.Contains(snapshot.PromptTemplate, "FROZEN BASE IMAGE RULES") || strings.Contains(snapshot.PromptTemplate, "{{before_image_prompt}}") || snapshot.ReferencePresentPrompt != "FROZEN REFERENCES\n{{reference_titles}}" || snapshot.ReferenceAbsentPrompt != "FROZEN NO REFERENCES" || snapshot.AdditionalDirectionPrompt != "FROZEN DIRECTION\n{{guidance_prompt}}" || snapshot.LanguageInstruction != "FROZEN COMIC LANGUAGE" {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if _, err := harness.stories.UpdatePromptGroup(ctx, story.UpdatePromptGroupInput{
		PromptGroup:             "chapter",
		Prompts:                 map[string]string{"before_image": "NEWER BASE IMAGE RULES"},
		ExpectedCurrentVersions: map[string]int{"before_image": 2},
	}); err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	requests := imageProvider.snapshot()
	if len(requests) != 1 || len(requests[0].Images) != 1 || bytes.Equal(requests[0].Images[0].Data, productionPNG(t)) || !strings.Contains(requests[0].Prompt, "FROZEN BASE IMAGE RULES") || strings.Contains(requests[0].Prompt, "NEWER BASE IMAGE RULES") || !strings.Contains(requests[0].Prompt, "FROZEN REFERENCES") || !strings.Contains(requests[0].Prompt, asset.Title) || !strings.Contains(requests[0].Prompt, "FROZEN DIRECTION") || !strings.Contains(requests[0].Prompt, "Keep a silver border") || !strings.Contains(requests[0].Prompt, "FROZEN COMIC LANGUAGE") {
		t.Fatalf("explicit premise image requests=%d images=%d", len(requests), len(requests[0].Images))
	}
}

func TestComicSectionWithoutPremiseAssetsSendsNoImagesAndLimitIsTwelve(t *testing.T) {
	harness := newQueueHarness(t)
	imageProvider := &recordingImageProvider{content: productionPNG(t)}
	harness.queue.WithImageClient(imageProvider)
	ctx := context.Background()
	if _, err := harness.stories.UpdatePromptGroup(ctx, story.UpdatePromptGroupInput{
		PromptGroup:             "chapter",
		Prompts:                 map[string]string{"section_reference_absent": "CUSTOM NO-REFERENCE INSTRUCTION"},
		ExpectedCurrentVersions: map[string]int{"section_reference_absent": 1},
	}); err != nil {
		t.Fatal(err)
	}
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch12")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "No premise", StoryboardMD: "An empty road"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-no-premise"})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	requests := imageProvider.snapshot()
	if len(requests) != 1 || len(requests[0].Images) != 0 || !strings.Contains(requests[0].Prompt, "CUSTOM NO-REFERENCE INSTRUCTION") {
		t.Fatalf("no-premise image requests=%d images=%d", len(requests), len(requests[0].Images))
	}
	tooMany := make([]string, maxSectionPremiseAssets+1)
	_, limitErr := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, PremiseAssetUUIDs: tooMany, IdempotencyKey: "comic-too-many-premise"})
	var queueErr *Error
	if !errors.As(limitErr, &queueErr) || queueErr.Code != CodeInvalidTask {
		t.Fatalf("too many premise assets error=%v", limitErr)
	}
}

func TestComicReferenceSelectionRejectsMoreThanTwelveBeforeImageProvider(t *testing.T) {
	harness := newQueueHarness(t)
	imageProvider := &recordingImageProvider{content: productionPNG(t)}
	harness.queue.WithImageClient(imageProvider)
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch14")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Too many", StoryboardMD: "Every setting item appears."})
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, 0, maxSectionPremiseAssets+1)
	for index := 0; index <= maxSectionPremiseAssets; index++ {
		upload, uploadErr := service.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "candidate.png", Reader: bytes.NewReader(productionPNG(t))})
		if uploadErr != nil {
			t.Fatal(uploadErr)
		}
		title := fmt.Sprintf("Candidate %02d", index+1)
		if _, importErr := service.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetReference, Title: title}); importErr != nil {
			t.Fatal(importErr)
		}
		titles = append(titles, title)
	}
	selector := &recordingSelectionProvider{sectionID: section.UUID, titles: titles}
	harness.queue.llm = selector
	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "selection-over-limit"})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusFailed)
	if failed.ErrorCode != "invalid_section_reference_selection" || len(imageProvider.snapshot()) != 0 {
		t.Fatalf("over-limit task=%+v image_calls=%d", failed, len(imageProvider.snapshot()))
	}
}

func TestComicSectionAutomaticallySelectsReferencesAndSendsFinalRequest(t *testing.T) {
	harness := newQueueHarness(t)
	imageProvider := &recordingImageProvider{content: productionPNG(t)}
	harness.queue.WithImageClient(imageProvider)
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch10")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "灯塔重逢", StoryboardMD: "## Section 核心剧情目标\n月光小狐狸抵达黄昏灯塔。\n\n## 关键视觉瞬间\n**瞬间 1：重逢**"})
	if err != nil {
		t.Fatal(err)
	}
	upload, err := service.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "fox.png", Reader: bytes.NewReader(productionPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := service.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetCharacter, Title: "月光小狐狸", Summary: "月光邮差"})
	if err != nil {
		t.Fatal(err)
	}
	selector := &recordingSelectionProvider{sectionID: section.UUID, title: asset.Title}
	harness.queue.llm = selector
	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-auto-selection"})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	selectionRequest := selector.snapshot()
	for _, required := range []string{"漫画设定项选择器", "最多选择 12", asset.Title, section.UUID, "项目语言：简体中文"} {
		if !strings.Contains(selectionRequest.Prompt, required) {
			t.Fatalf("selection prompt missing %q: %s", required, selectionRequest.Prompt)
		}
	}
	requests := imageProvider.snapshot()
	if len(requests) != 1 {
		t.Fatalf("image requests = %d", len(requests))
	}
	request := requests[0]
	if request.Size != defaultComicImageSize {
		t.Fatalf("Cloudflare AI Gateway section image size=%q", request.Size)
	}
	if request.Model != "openai/gpt-image-1.5" {
		t.Fatalf("Cloudflare AI Gateway section image model=%q", request.Model)
	}
	for _, required := range []string{"## 生成规则", "## 分镜构图要求", "## 设定图约束", "一张 Section 专属设定拼贴图", asset.Title, section.CurrentStoryboard.ContentMD, "项目语言：简体中文"} {
		if !strings.Contains(request.Prompt, required) {
			t.Fatalf("section image prompt missing %q: %s", required, request.Prompt)
		}
	}
	if len(request.Images) != 1 || request.Images[0].MIMEType != "image/png" || bytes.Equal(request.Images[0].Data, productionPNG(t)) {
		t.Fatalf("section image reference count=%d mime=%q was_source=%v", len(request.Images), request.Images[0].MIMEType, bytes.Equal(request.Images[0].Data, productionPNG(t)))
	}
	composite, err := png.Decode(bytes.NewReader(request.Images[0].Data))
	if err != nil || composite.Bounds().Dx() != 408 || composite.Bounds().Dy() != 420 {
		t.Fatalf("section premise bounds=%v err=%v", composite.Bounds(), err)
	}
	if strings.Contains(request.Prompt, "{{") {
		t.Fatalf("section image prompt contains unresolved placeholder: %s", request.Prompt)
	}
	type callLog struct{ Scenario, RequestType, Status string }
	var logs []callLog
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return store.DB().Table("llm_logs AS logs").Select("logs.scenario,logs.request_type,logs.status").Joins("JOIN production_task_runs tasks ON tasks.id=logs.production_task_run_id").Where("tasks.uuid=?", task.UUID).Order("logs.created_at,logs.id").Scan(&logs).Error
	}); err != nil || len(logs) != 2 || logs[0].Scenario != "comic_reference_selection" || logs[0].RequestType != "text" || logs[1].Scenario != KindComicImageGeneration || logs[1].RequestType != "image" || logs[0].Status != "completed" || logs[1].Status != "completed" {
		t.Fatalf("comic call logs=%+v err=%v", logs, err)
	}
	updatedSection, err := service.GetSection(ctx, chapter.UUID, section.UUID)
	if err != nil || updatedSection.CurrentImage == nil || updatedSection.CurrentImage.SectionPremise == nil {
		t.Fatalf("section premise response=%+v err=%v", updatedSection.CurrentImage, err)
	}
	premise := updatedSection.CurrentImage.SectionPremise
	if premise.Asset.Purpose != "comic_section_premise" || premise.Asset.UUID == "" || len(premise.SelectedAssets) != 1 || premise.SelectedAssets[0].AssetUUID != asset.UUID || len(premise.SelectedTitles) != 1 || premise.SelectedTitles[0] != asset.Title || premise.SelectionReason != "直接出现在分镜中" || premise.ImageInfo.Width != 408 || premise.ImageInfo.Height != 420 || premise.ImageInfo.ComposerVersion != sectionPremiseComposerVersion {
		t.Fatalf("section premise=%+v", premise)
	}
	publicJSON, err := json.Marshal(updatedSection.CurrentImage)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"id"`, `"file_id"`, `"premise_file_id"`, `"key_path"`, `"object_path"`} {
		if strings.Contains(string(publicJSON), forbidden) {
			t.Fatalf("public section premise contains %q: %s", forbidden, publicJSON)
		}
	}
	var composedPayload string
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return store.DB().Table("production_task_events AS events").Select("events.payload").Joins("JOIN production_task_runs tasks ON tasks.id=events.production_task_run_id").Where("tasks.uuid=? AND events.event_type='section_premise_composed'", task.UUID).Scan(&composedPayload).Error
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(composedPayload, premise.Asset.UUID) || strings.Contains(composedPayload, `"id"`) || strings.Contains(composedPayload, "path") {
		t.Fatalf("section_premise_composed payload=%s", composedPayload)
	}
}

func TestHistoricalFiveAssetSelectionRecomposesOnePremiseWithoutReselecting(t *testing.T) {
	harness := newQueueHarness(t)
	imageProvider := &failFirstRecordingImageProvider{content: productionPNG(t)}
	harness.queue.WithImageClient(imageProvider)
	ctx := context.Background()
	var service *production.Service
	var storeRoot string
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		storeRoot = store.Root()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch13")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "历史五图", StoryboardMD: "五个设定元素同时出现在画面中。"})
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, 0, 5)
	for index := 0; index < 5; index++ {
		upload, uploadErr := service.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "historical.png", Reader: bytes.NewReader(productionPNG(t))})
		if uploadErr != nil {
			t.Fatal(uploadErr)
		}
		title := []string{"月光小狐狸", "黄昏灯塔", "银色邮包", "山谷入口", "星空地图"}[index]
		if _, importErr := service.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetReference, Title: title}); importErr != nil {
			t.Fatal(importErr)
		}
		titles = append(titles, title)
	}
	selector := &recordingSelectionProvider{sectionID: section.UUID, titles: titles}
	harness.queue.llm = selector
	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "historical-five-selection"})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusFailed)
	// Let River deliver the terminal cancellation event for the discarded first
	// execution before asking it to retry the same durable job.
	time.Sleep(200 * time.Millisecond)
	if stable := waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusFailed); stable.Status != StatusFailed {
		t.Fatalf("first attempt status=%s", stable.Status)
	}
	var originalFileUUID, keyPath string
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return store.DB().Table("comic_image_generations AS generations").
			Select("files.uuid,objects.key_path").
			Joins("JOIN files ON files.id=generations.premise_file_id").
			Joins("JOIN file_objects AS objects ON objects.id=files.file_object_id").
			Where("generations.task_uuid=?", task.UUID).Row().Scan(&originalFileUUID, &keyPath)
	}); err != nil {
		t.Fatal(err)
	}
	if originalFileUUID == "" || keyPath == "" {
		t.Fatal("first attempt did not persist a section premise")
	}
	// Emulate a pre-collage historical generation: retain only its frozen
	// section_references_selected event and clear the newer projection fields.
	var taskRunID int64
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		if err := store.DB().Table("production_task_runs").Select("id").Where("uuid=?", task.UUID).Scan(&taskRunID).Error; err != nil {
			return err
		}
		return store.DB().Exec("UPDATE comic_image_generations SET premise_file_id=NULL,premise_metadata='{}' WHERE task_uuid=?", task.UUID).Error
	}); err != nil {
		t.Fatal(err)
	}
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.appendProductionEvent(ctx, taskRunID, "section_references_selected", map[string]any{"section_uuid": section.UUID, "titles": titles}); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.queue.RetryProductionTask(ctx, harness.project.UUID, task.UUID); err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	if selector.callCount() != 1 {
		t.Fatalf("selection model calls=%d", selector.callCount())
	}
	requests := imageProvider.snapshot()
	if len(requests) != 2 || len(requests[0].Images) != 1 || len(requests[1].Images) != 1 || !bytes.Equal(requests[0].Images[0].Data, requests[1].Images[0].Data) {
		t.Fatalf("historical retry requests=%d images=%d/%d", len(requests), len(requests[0].Images), len(requests[1].Images))
	}
	decoded, err := png.Decode(bytes.NewReader(requests[1].Images[0].Data))
	if err != nil || decoded.Bounds().Dx() != 1176 || decoded.Bounds().Dy() != 816 {
		t.Fatalf("historical collage bounds=%v err=%v", decoded.Bounds(), err)
	}
	if _, err := os.Stat(filepath.Join(storeRoot, "assets", filepath.FromSlash(keyPath))); err != nil {
		t.Fatalf("original persisted premise unexpectedly missing: %v", err)
	}
	updatedSection, err := service.GetSection(ctx, chapter.UUID, section.UUID)
	if err != nil || updatedSection.CurrentImage == nil || updatedSection.CurrentImage.SectionPremise == nil || updatedSection.CurrentImage.SectionPremise.SelectionReason != "" || len(updatedSection.CurrentImage.SectionPremise.SelectedAssets) != 5 {
		t.Fatalf("historical section premise=%+v err=%v", updatedSection.CurrentImage, err)
	}
}

func TestComicImageSizeUsesOneToThreeForBailian(t *testing.T) {
	if size := comicImageSize(provider.TypeAliyunBailian); size != "768x2304" {
		t.Fatalf("Bailian comic image size=%q, want 1:3", size)
	}
	if size := comicImageSize(provider.TypeCloudflareAIGateway); size != defaultComicImageSize {
		t.Fatalf("Cloudflare AI Gateway comic image size=%q", size)
	}
}

func TestComicReferenceSelectionIsReusedAfterWorkerRestart(t *testing.T) {
	harness := newQueueHarness(t)
	imageProvider := &restartingImageProvider{started: make(chan struct{}), content: productionPNG(t)}
	harness.queue.WithImageClient(imageProvider)
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := harness.createChapter(t, "vol01.ch11")
	section, err := service.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "重试分镜", StoryboardMD: "## Section 核心剧情目标\n狐狸走进灯塔。\n\n## 关键视觉瞬间\n**瞬间 1：进入**"})
	if err != nil {
		t.Fatal(err)
	}
	upload, err := service.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "fox.png", Reader: bytes.NewReader(productionPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := service.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetCharacter, Title: "重试小狐狸", Summary: "用于重试验证"})
	if err != nil {
		t.Fatal(err)
	}
	selector := &recordingSelectionProvider{sectionID: section.UUID, title: asset.Title}
	harness.queue.llm = selector
	task, err := harness.queue.CreateComicImageGeneration(ctx, harness.project.UUID, chapter.UUID, section.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-selection-restart"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-imageProvider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("image provider did not start")
	}
	if err := harness.projects.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.projects.OpenRecent(ctx, harness.project.UUID); err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	if selector.callCount() != 1 {
		t.Fatalf("selection model calls=%d, want one frozen selection", selector.callCount())
	}
	requests := imageProvider.snapshot()
	if len(requests) != 2 || len(requests[0].Images) != 1 || len(requests[1].Images) != 1 || !bytes.Equal(requests[0].Images[0].Data, requests[1].Images[0].Data) {
		t.Fatalf("restart image requests=%d references=%d/%d", len(requests), len(requests[0].Images), len(requests[1].Images))
	}
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	updatedSection, err := service.GetSection(ctx, chapter.UUID, section.UUID)
	if err != nil || updatedSection.CurrentImage == nil || updatedSection.CurrentImage.SectionPremise == nil {
		t.Fatalf("reused section premise=%+v err=%v", updatedSection.CurrentImage, err)
	}
	var premiseFiles int64
	var selectionLogs, imageLogs int64
	var recoveredWorkflowStatus, recoveredThreadStatus string
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		if err := store.DB().Table("llm_logs AS logs").Joins("JOIN production_task_runs tasks ON tasks.id=logs.production_task_run_id").Where("tasks.uuid=? AND logs.scenario='comic_reference_selection'", task.UUID).Count(&selectionLogs).Error; err != nil {
			return err
		}
		if err := store.DB().Table("llm_logs AS logs").Joins("JOIN production_task_runs tasks ON tasks.id=logs.production_task_run_id").Where("tasks.uuid=? AND logs.scenario=?", task.UUID, KindComicImageGeneration).Count(&imageLogs).Error; err != nil {
			return err
		}
		if err := store.DB().Table("files").Where("purpose='comic_section_premise'").Count(&premiseFiles).Error; err != nil {
			return err
		}
		return store.DB().Raw(`SELECT w.status,t.status FROM workflows w JOIN chat_threads t ON t.id=w.thread_id JOIN workflow_steps s ON s.workflow_id=w.id WHERE s.task_uuid=?`, task.UUID).Row().Scan(&recoveredWorkflowStatus, &recoveredThreadStatus)
	}); err != nil || selectionLogs != 1 || imageLogs != 2 || premiseFiles != 1 || recoveredWorkflowStatus != agent.WorkflowCompleted || recoveredThreadStatus != agent.ThreadCompleted {
		t.Fatalf("restart logs selection=%d image=%d premise_files=%d workflow=%s thread=%s err=%v", selectionLogs, imageLogs, premiseFiles, recoveredWorkflowStatus, recoveredThreadStatus, err)
	}
}

func TestCancelledProductionImageNeverReplacesCurrentSetting(t *testing.T) {
	harness := newQueueHarness(t)
	started := make(chan struct{})
	harness.queue.WithImageClient(blockingImageProvider{started: started})
	var service *production.Service
	if err := harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error { service = production.NewService(store, nil); return nil }); err != nil {
		t.Fatal(err)
	}
	source, err := service.CreatePremiseSource(context.Background(), production.CreateSourceInput{SourceText: "A quiet forest", StyleSnapshot: "ink", SourceType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	upload, err := service.Files().CreateUpload(context.Background(), files.CreateUploadInput{Purpose: "premise_setting_image", OriginalFilename: "manual.png", Reader: bytes.NewReader(productionPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	manual, err := service.ImportSettingImage(context.Background(), upload.UUID, source.UUID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.CreatePremiseSettingGeneration(context.Background(), harness.project.UUID, source.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, Prompt: "blocked", IdempotencyKey: "cancel-setting"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("image provider did not start")
	}
	var pending int64
	if err := harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error {
		return store.DB().Table("llm_logs AS logs").Joins("JOIN production_task_runs tasks ON tasks.id=logs.production_task_run_id").Where("tasks.uuid=? AND logs.status='pending'", task.UUID).Count(&pending).Error
	}); err != nil || pending != 1 {
		t.Fatalf("pending logs=%d err=%v", pending, err)
	}
	cancelled, err := harness.queue.CancelProductionTask(context.Background(), harness.project.UUID, task.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != StatusCancelled {
		cancelled = waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCancelled)
	}
	profile, err := service.GetPremise(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	images, err := service.ListSettingImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || profile.CurrentSettingImage == nil || profile.CurrentSettingImage.UUID != manual.UUID {
		t.Fatalf("cancel changed current: profile=%+v images=%+v", profile, images)
	}
	waitProductionLogStatus(t, harness, task.UUID, StatusCancelled, 1)
}

func TestFailedProductionImagePersistsProviderDiagnostics(t *testing.T) {
	harness := newQueueHarness(t)
	harness.queue.WithImageClient(failingImageProvider{})
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	source, err := service.CreatePremiseSource(ctx, production.CreateSourceInput{SourceText: "A failed forest", StyleSnapshot: "ink", SourceType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.CreatePremiseSettingGeneration(ctx, harness.project.UUID, source.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, Prompt: "fail safely", IdempotencyKey: "failed-setting-log"})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusFailed)
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.projectProductionRiverEvent(ctx, &river.Event{Kind: river.EventKindJobCancelled}, productionArgs{TaskUUID: task.UUID}); err != nil {
		t.Fatal(err)
	}
	persisted, err := harness.queue.GetProductionTask(ctx, harness.project.UUID, task.UUID)
	if err != nil || persisted.Status != StatusFailed {
		t.Fatalf("River cancellation overwrote failed production task: task=%+v err=%v", persisted, err)
	}
	var status, code, message, requestID, requestPayload string
	var responsePayload *string
	var httpStatus int
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return store.DB().Raw(`SELECT logs.status,logs.provider_error_code,logs.error_message,logs.http_status,logs.provider_request_id,logs.request_payload,logs.response FROM llm_logs logs JOIN production_task_runs tasks ON tasks.id=logs.production_task_run_id WHERE tasks.uuid=?`, task.UUID).Row().Scan(&status, &code, &message, &httpStatus, &requestID, &requestPayload, &responsePayload)
	}); err != nil || status != "failed" || code != "InvalidParameter" || message != "unsupported image size" || httpStatus != 400 || requestID != "image-request-400" {
		t.Fatalf("failed log status=%q code=%q message=%q http=%d request=%q err=%v", status, code, message, httpStatus, requestID, err)
	}
	if !json.Valid([]byte(requestPayload)) || responsePayload != nil {
		t.Fatalf("failed log snapshots request=%s response=%v", requestPayload, responsePayload)
	}
}

func TestCancelledProductionTaskCanBeExplicitlyRetried(t *testing.T) {
	harness := newQueueHarness(t)
	harness.queue.WithImageClient(failingImageProvider{})
	ctx := context.Background()
	var service *production.Service
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	source, err := service.CreatePremiseSource(ctx, production.CreateSourceInput{SourceText: "A recoverable forest", StyleSnapshot: "ink", SourceType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.CreatePremiseSettingGeneration(ctx, harness.project.UUID, source.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, Prompt: "retry cancelled production", IdempotencyKey: "retry-cancelled-production"})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusFailed)
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := getProductionTaskRecord(ctx, runtime.store.DB(), runtime.projectID, task.UUID)
	if err != nil || record.RiverJobID == nil {
		t.Fatalf("production task record=%+v err=%v", record, err)
	}
	waitJobState(t, runtime.client, *record.RiverJobID, rivertype.JobStateCancelled)
	now := time.Now().UTC()
	if err := runtime.store.DB().Exec(`UPDATE production_task_runs SET status=?,cancel_requested_at=?,completed_at=?,updated_at=? WHERE id=?`, StatusCancelled, now, now, now, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	harness.queue.WithImageClient(successfulImageProvider{content: productionPNG(t)})
	if _, err := harness.queue.RetryProductionTask(ctx, harness.project.UUID, task.UUID); err != nil {
		t.Fatal(err)
	}
	completed := waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	if completed.CancelRequestedAt != nil || completed.ErrorCode != "" {
		t.Fatalf("retried production task retained cancellation state: %+v", completed)
	}
}

func TestProjectCloseAndReopenRecoversDurableProduction(t *testing.T) {
	harness := newQueueHarness(t)
	provider := &restartingImageProvider{started: make(chan struct{}), content: productionPNG(t)}
	harness.queue.WithImageClient(provider)
	var service *production.Service
	if err := harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	source, err := service.CreatePremiseSource(context.Background(), production.CreateSourceInput{SourceText: "A city above the clouds", StyleSnapshot: "gouache", SourceType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := harness.queue.CreatePremiseSettingGeneration(context.Background(), harness.project.UUID, source.UUID, CreateProductionGenerationInput{ProviderUUID: harness.provider.UUID, Prompt: "restart this production", IdempotencyKey: "restart-production"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("image provider did not start")
	}
	if err := harness.projects.CloseCurrent(context.Background()); err != nil {
		t.Fatal(err)
	}
	reopened, err := harness.projects.OpenRecent(context.Background(), harness.project.UUID)
	if err != nil || reopened.UUID != harness.project.UUID {
		t.Fatalf("reopened=%+v err=%v", reopened, err)
	}
	if err := harness.projects.WithCurrentStore(context.Background(), harness.project.UUID, func(store *project.Store) error {
		service = production.NewService(store, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	completed := waitProductionStatus(t, harness.queue, harness.project.UUID, task.UUID, StatusCompleted)
	if completed.Attempt < 2 {
		t.Fatalf("recovered task=%+v", completed)
	}
	images, err := service.ListSettingImages(context.Background())
	if err != nil || len(images) != 1 || images[0].Origin != "generated" {
		t.Fatalf("settings=%+v err=%v", images, err)
	}
}

func TestProductionCallDoesNotReachProviderWhenPendingLogCannotBeCreated(t *testing.T) {
	harness := newQueueHarness(t)
	client := &countingImageProvider{}
	harness.queue.WithImageClient(client)
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	taskUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	result, err := runtime.sqlDB.ExecContext(context.Background(), `INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,provider_uuid,model,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'comic_export',?,'{}','completed','log-create-failure',?,'image-model',100,1,1,?,?)`, taskUUID, runtime.projectID, harness.project.UUID, harness.provider.UUID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.sqlDB.ExecContext(context.Background(), `CREATE TRIGGER reject_llm_log_insert BEFORE INSERT ON llm_logs BEGIN SELECT RAISE(ABORT, 'forced log insert failure'); END`); err != nil {
		t.Fatal(err)
	}
	resolved, err := harness.queue.providers.Resolve(context.Background(), harness.provider.UUID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := production.GenerationSnapshot{ProviderUUID: harness.provider.UUID, ProviderType: resolved.ProviderType, Model: "image-model"}
	_, callErr := runtime.callProductionImage(context.Background(), productionTaskRecord{ID: taskID, Attempt: 1}, snapshot, resolved, KindComicImageGeneration, imagegen.Request{Model: "image-model", Prompt: "must not be sent"})
	if _, err := runtime.sqlDB.ExecContext(context.Background(), `DROP TRIGGER reject_llm_log_insert`); err != nil {
		t.Fatal(err)
	}
	if callErr == nil {
		t.Fatal("call unexpectedly succeeded when pending log insert failed")
	}
	if client.callCount() != 0 {
		t.Fatalf("provider calls=%d, want zero", client.callCount())
	}
}

func TestReconcileMarksOrphanedPendingProviderCallFailed(t *testing.T) {
	harness := newQueueHarness(t)
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	taskUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	logUUID, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	result, err := runtime.sqlDB.ExecContext(context.Background(), `INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,provider_uuid,model,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'comic_export',?,'{}','completed','pending-log-recovery',?,'image-model',100,1,1,?,?)`, taskUUID, runtime.projectID, harness.project.UUID, harness.provider.UUID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.sqlDB.ExecContext(context.Background(), `INSERT INTO llm_logs(uuid,project_id,production_task_run_id,source_type,scenario,request_type,attempt,provider_uuid,provider_type,model,status,created_at) VALUES(?,?,?,'production','comic_image_generation','image',1,?,'openai_compatible','image-model','pending',?)`, logUUID, runtime.projectID, taskID, harness.provider.UUID, now); err != nil {
		t.Fatal(err)
	}
	recoveredAt := now.Add(time.Minute)
	if err := reconcileProductTasks(context.Background(), runtime.sqlDB, runtime.projectID, recoveredAt); err != nil {
		t.Fatal(err)
	}
	var status, errorCode, errorMessage string
	var completedAt time.Time
	if err := runtime.sqlDB.QueryRowContext(context.Background(), `SELECT status,error_code,error_message,completed_at FROM llm_logs WHERE uuid=?`, logUUID).Scan(&status, &errorCode, &errorMessage, &completedAt); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || errorCode != "provider_call_interrupted" || errorMessage == "" || !completedAt.Equal(recoveredAt) {
		t.Fatalf("recovered log status=%q code=%q message=%q completed=%s", status, errorCode, errorMessage, completedAt)
	}
}
