package jobqueue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"lumi/internal/files"
	"lumi/internal/production"
	"lumi/internal/project"
)

func queueWebsiteFixture(t *testing.T, h *queueHarness) *production.Service {
	t.Helper()
	var s *production.Service
	if err := h.projects.WithCurrentStore(t.Context(), h.project.UUID, func(store *project.Store) error { s = production.NewService(store, h.queue.hub); return nil }); err != nil {
		t.Fatal(err)
	}
	chapter := h.createChapter(t, "vol01.ch01")
	page, err := s.CreateSection(t.Context(), chapter.UUID, production.CreateSectionInput{Title: "Website page", StoryboardMD: "PRIVATE"})
	if err != nil {
		t.Fatal(err)
	}
	upload, err := s.Files().CreateUpload(t.Context(), files.CreateUploadInput{Purpose: "comic_section_image", OriginalFilename: "fixture.png", Reader: bytes.NewReader(productionPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportSectionImage(t.Context(), chapter.UUID, page.UUID, upload.UUID, page.Revision); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestWebsiteQueueCompletionReadinessAndRetry(t *testing.T) {
	h := newQueueHarness(t)
	s := queueWebsiteFixture(t, h)
	chapter := h.createChapter(t, "vol01.ch02")
	if _, err := s.CreateSection(t.Context(), chapter.UUID, production.CreateSectionInput{Title: "Missing", StoryboardMD: "private"}); err != nil {
		t.Fatal(err)
	}
	_, err := h.queue.CreateWebsiteGeneration(t.Context(), h.project.UUID, CreateWebsiteInput{IdempotencyKey: "missing"})
	var domain *production.Error
	if !errors.As(err, &domain) || domain.Code != production.CodeExportIncomplete {
		t.Fatalf("missing image must require consent: %v", err)
	}
	task, err := h.queue.CreateWebsiteGeneration(t.Context(), h.project.UUID, CreateWebsiteInput{IdempotencyKey: "partial", AllowMissingImages: true})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, h.queue, h.project.UUID, task.UUID, StatusCompleted)
	status, err := s.WebsiteStatus(t.Context())
	if err != nil || !status.Available || status.Latest.ProcessedImages != 1 || status.HasUpdates {
		t.Fatalf("status=%+v %v", status, err)
	}
	// Intent arriving after the atomic publication cannot cancel a good build.
	cancelled, err := h.queue.CancelProductionTask(t.Context(), h.project.UUID, task.UUID)
	if err != nil || cancelled.Status != StatusCompleted {
		t.Fatalf("late cancel=%+v %v", cancelled, err)
	}
	duplicate, err := h.queue.CreateWebsiteGeneration(t.Context(), h.project.UUID, CreateWebsiteInput{IdempotencyKey: "partial", AllowMissingImages: true})
	if err != nil || duplicate.UUID != task.UUID {
		t.Fatalf("idempotency=%+v %v", duplicate, err)
	}
	again, err := h.queue.CreateWebsiteGeneration(t.Context(), h.project.UUID, CreateWebsiteInput{IdempotencyKey: "again", AllowMissingImages: true})
	if err != nil {
		t.Fatal(err)
	}
	waitProductionStatus(t, h.queue, h.project.UUID, again.UUID, StatusCompleted)
	status, err = s.WebsiteStatus(t.Context())
	if err != nil || status.Latest.ProcessedImages != 0 || status.Latest.ReusedImages != 1 {
		t.Fatalf("reuse=%+v %v", status, err)
	}
	// Asset export retention is independent of the website publication.
	if _, err = s.CleanupExpiredExports(t.Context(), 100); err != nil {
		t.Fatal(err)
	}
	status, err = s.WebsiteStatus(t.Context())
	if err != nil || !status.Available {
		t.Fatal("website expired", err)
	}
}
func TestWebsiteConcurrentCreationCancellationAndFrozenRetry(t *testing.T) {
	h, r := stoppedExecutionHarness(t)
	s := queueWebsiteFixture(t, h)
	const count = 8
	var wg sync.WaitGroup
	tasks := make(chan ProductionTask, count)
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			task, err := h.queue.CreateWebsiteGeneration(t.Context(), h.project.UUID, CreateWebsiteInput{IdempotencyKey: fmt.Sprintf("click-%d", i)})
			if err != nil {
				errs <- err
			} else {
				tasks <- task
			}
		}(i)
	}
	wg.Wait()
	close(tasks)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	var original ProductionTask
	for task := range tasks {
		if original.UUID != "" && task.UUID != original.UUID {
			t.Fatal("duplicate live task")
		}
		original = task
	}
	var total int64
	if err := r.store.DB().Table("website_generations").Count(&total).Error; err != nil || total != 1 {
		t.Fatalf("generations=%d %v", total, err)
	}
	cancelled, err := h.queue.CancelProductionTask(t.Context(), h.project.UUID, original.UUID)
	if err != nil || cancelled.Status != StatusCancelled {
		t.Fatalf("cancel=%+v %v", cancelled, err)
	}
	current, err := h.queue.CreateWebsiteGeneration(t.Context(), h.project.UUID, CreateWebsiteInput{IdempotencyKey: "new-snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.queue.RetryProductionTask(t.Context(), h.project.UUID, original.UUID); err == nil {
		t.Fatal("retry ran concurrently with newer task")
	}
	if _, err = h.queue.CancelProductionTask(t.Context(), h.project.UUID, current.UUID); err != nil {
		t.Fatal(err)
	}
	if err = r.store.DB().Exec(`UPDATE projects SET name='Changed after click'`).Error; err != nil {
		t.Fatal(err)
	}
	retry, err := h.queue.RetryProductionTask(t.Context(), h.project.UUID, original.UUID)
	if err != nil || retry.UUID != original.UUID {
		t.Fatalf("retry=%+v %v", retry, err)
	}
	var before, after production.GenerationSnapshot
	json.Unmarshal(original.InputSnapshot, &before)
	json.Unmarshal(retry.InputSnapshot, &after)
	if !bytes.Equal(before.Parameters, after.Parameters) {
		t.Fatal("retry refreshed frozen snapshot")
	}
	if err = r.store.DB().Exec(`UPDATE production_task_runs SET status='running' WHERE uuid=?`, retry.UUID).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.RenderWebsite(t.Context(), retry.UUID, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(r.store.Root(), "website/data/site.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("Changed after click")) {
		t.Fatal("retry exported live title")
	}
	status, err := s.WebsiteStatus(t.Context())
	if err != nil || !status.HasUpdates || status.Latest.TaskUUID != original.UUID || status.Latest.Status != StatusCompleted {
		t.Fatalf("updates=%+v %v", status, err)
	}
}
