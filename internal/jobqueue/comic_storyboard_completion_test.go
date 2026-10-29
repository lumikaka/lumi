package jobqueue

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"lumi/internal/llm"
	"lumi/internal/production"
)

type comicCompletionModel struct {
	*fakeProviderClient
	content string
	calls   atomic.Int32
}

func (model *comicCompletionModel) Generate(context.Context, llm.Request, func(string) error) (llm.Response, error) {
	model.calls.Add(1)
	return llm.Response{Content: model.content, FinishReason: "stop", Usage: llm.Usage{InputTokens: 1000, OutputTokens: 5000}}, nil
}

func TestComicStoryboardWorkflowCompletesAllPagesOrSavesNone(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		count int
		fault string
	}{
		{name: "14 pages", count: 14},
		{name: "16 pages", count: 16},
		{name: "missing last script", count: 16, fault: "empty"},
		{name: "last script cannot persist", count: 16, fault: "insert"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			h := newQueueHarness(t)
			ctx := context.Background()
			chapter := h.createChapter(t, "vol01.ch01")
			sections := make([]map[string]any, scenario.count)
			for index := range sections {
				sections[index] = map[string]any{"section_no": index + 1, "title": "Page " + jsonNumber(index+1), "storyboard": "Usable storyboard " + jsonNumber(index+1)}
			}
			if scenario.fault == "empty" {
				sections[len(sections)-1]["storyboard"] = " \n\t "
			}
			raw, _ := json.Marshal(map[string]any{"chapter_code": chapter.ChapterCode, "title": "整章绘本", "sections": sections})
			model := &comicCompletionModel{fakeProviderClient: newFakeProviderClient(), content: string(raw)}
			h.queue.llm = model
			if scenario.fault == "insert" {
				if err := h.runtime(t).store.DB().Exec(`CREATE TRIGGER reject_last_storyboard BEFORE INSERT ON comic_storyboard_variants WHEN NEW.content_md='Usable storyboard 16' BEGIN SELECT RAISE(ABORT,'injected storyboard failure'); END`).Error; err != nil {
					t.Fatal(err)
				}
				// Exercise the content transaction directly so this test does not
				// wait through River's intentional persistence-error backoff.
				_, err := h.runtime(t).applyStoryWorkflowResponse(ctx, taskRecord{Kind: KindComicStoryboardGeneration}, storyGenerationSnapshot{
					ChapterUUID: chapter.UUID, ChapterCode: chapter.ChapterCode, MaxSectionCount: scenario.count,
				}, string(raw))
				if err == nil {
					t.Fatal("injected storyboard write unexpectedly succeeded")
				}
				for _, table := range []string{"comic_sections", "comic_storyboard_variants"} {
					var count int64
					if err := h.runtime(t).store.DB().Table(table).Count(&count).Error; err != nil || count != 0 {
						t.Fatalf("partial %s rows=%d err=%v", table, count, err)
					}
				}
				return
			}
			task, err := h.queue.CreateStoryWorkflow(ctx, h.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{
				ProviderUUID: h.provider.UUID, MaxSectionCount: &scenario.count, IdempotencyKey: "complete-comic-" + scenario.fault + jsonNumber(scenario.count),
			})
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := StatusCompleted
			if scenario.fault != "" {
				wantStatus = StatusFailed
			}
			waitTaskStatus(t, h.queue, h.project.UUID, task.UUID, wantStatus)
			workflowStatus, stepStatus, threadStatus := comicStoryboardWorkflowState(t, h, task.UUID)
			if workflowStatus != wantStatus || stepStatus != wantStatus || threadStatus != wantStatus {
				t.Fatalf("terminal states=%s/%s/%s want=%s", workflowStatus, stepStatus, threadStatus, wantStatus)
			}
			stored, err := production.NewService(h.runtime(t).store, nil).ListSections(ctx, chapter.UUID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario.fault == "" {
				if len(stored) != scenario.count {
					t.Fatalf("saved %d pages, want %d", len(stored), scenario.count)
				}
				for _, section := range stored {
					if section.PageRole != production.PageRoleBody || !section.HasStoryboard || section.CurrentStoryboard == nil || strings.TrimSpace(section.CurrentStoryboard.ContentMD) == "" {
						t.Fatalf("completed workflow left an unusable page: %+v", section)
					}
				}
			} else if len(stored) != 0 {
				t.Fatalf("failed workflow left %d shell/partial pages", len(stored))
			}
			if model.calls.Load() != 1 {
				t.Fatalf("chapter generation used %d model calls", model.calls.Load())
			}
			var questions int64
			if err := h.runtime(t).store.DB().Table("chat_user_input_requests").Count(&questions).Error; err != nil || questions != 0 {
				t.Fatalf("ordinary questions=%d err=%v", questions, err)
			}
		})
	}
}
