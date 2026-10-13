package agent

import (
	"errors"
	"testing"
	"time"

	"lumi/internal/llm"
	"lumi/internal/story"
)

func TestWorkflowStoryboardRetryPreservesCompletedSteps(t *testing.T) {
	for _, finish := range []string{"success", "exhausted", "cancelled", "auth"} {
		t.Run(finish, func(t *testing.T) {
			fixture := newInlineYoloFixture(t)
			h := fixture.Harness
			ctx := t.Context()
			chapter, err := story.NewService(h.store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "测试", Content: "小狐狸出发。", ContentFormat: "txt"})
			if err != nil {
				t.Fatal(err)
			}
			wf := fixture.Workflow
			var step WorkflowStep
			for _, s := range wf.Steps {
				if s.StepKey == "comic_sections" {
					step = s
				}
			}
			if step.UUID == "" {
				t.Fatal("missing storyboard step")
			}
			if err := h.store.DB().Exec(`UPDATE workflow_steps SET status='completed' WHERE workflow_id=(SELECT id FROM workflows WHERE uuid=?) AND position<?`, wf.UUID, step.Position).Error; err != nil {
				t.Fatal(err)
			}
			if err := h.store.DB().Exec(`UPDATE workflow_steps SET output_json=? WHERE workflow_id=(SELECT id FROM workflows WHERE uuid=?) AND step_key='story'`, `{"chapter_uuid":"`+chapter.UUID+`"}`, wf.UUID).Error; err != nil {
				t.Fatal(err)
			}
			if err := h.store.DB().Exec(`UPDATE workflow_steps SET status='queued' WHERE uuid=?`, step.UUID).Error; err != nil {
				t.Fatal(err)
			}
			h.service.WithWorkflowRequestTimeout(20 * time.Minute)
			timeoutErr := &llm.Error{Code: llm.CodeTimeout, SafeMessage: "超时", Retryable: true}
			h.model.respond = func(call int, req llm.ChatRequest) (llm.ChatResponse, error) {
				if req.Timeout != 20*time.Minute {
					t.Fatalf("request timeout=%s", req.Timeout)
				}
				if finish == "auth" {
					return llm.ChatResponse{}, &llm.Error{Code: llm.CodeAuthentication, SafeMessage: "鉴权失败"}
				}
				if call == 1 || finish == "exhausted" {
					return llm.ChatResponse{}, timeoutErr
				}
				return finalResponse(`{"chapter_code":"vol01.ch01","sections":[{"section_no":1,"title":"出发","storyboard":"小狐狸走出家门。"}]}`), nil
			}
			run := func(attempt int) error {
				return h.service.ExecuteJob(ctx, h.store, JobSpec{Version: 1, ProjectUUID: h.project.UUID, JobKind: JobWorkflowStep, ResourceUUID: step.UUID, ThreadUUID: wf.ThreadUUID, Attempt: attempt, MaxAttempts: WorkflowMaxAttempts})
			}
			err = run(1)
			if finish == "auth" {
				if err != nil {
					t.Fatal(err)
				}
				got, e := h.service.GetWorkflow(ctx, h.project.UUID, wf.UUID)
				if e != nil || got.Status != WorkflowFailed || h.model.calls != 1 {
					t.Fatalf("nonretryable result=%+v calls=%d err=%v", got, h.model.calls, e)
				}
				return
			}
			if !errors.Is(err, timeoutErr) {
				t.Fatalf("retry error=%v", err)
			}
			got, err := h.service.GetWorkflow(ctx, h.project.UUID, wf.UUID)
			if err != nil || got.Status != WorkflowQueued {
				t.Fatalf("retry workflow=%+v err=%v", got, err)
			}
			if countAgentJobs(h.queue.jobs, JobChatResume) != 0 {
				t.Fatal("transient timeout resumed parent turn")
			}
			// A recreated service must use River's persisted attempt budget.
			h.service = NewService(h.projects, h.providers, h.model, h.queue, nil).WithWorkflowRequestTimeout(20 * time.Minute)
			if finish == "cancelled" {
				if _, err := h.service.CancelWorkflow(ctx, h.project.UUID, wf.UUID); err != nil {
					t.Fatal(err)
				}
			}
			if err := run(2); err != nil {
				t.Fatal(err)
			}
			got, err = h.service.GetWorkflow(ctx, h.project.UUID, wf.UUID)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range got.Steps {
				if s.Position < step.Position && s.Status != WorkflowCompleted {
					t.Fatalf("rewound %s", s.StepKey)
				}
			}
			switch finish {
			case "success":
				if got.Steps[step.Position-1].Status != WorkflowCompleted || h.model.calls != 2 {
					t.Fatalf("retry failed: %+v calls=%d", got, h.model.calls)
				}
			case "exhausted":
				if countAgentJobs(h.queue.jobs, JobChatResume) != 1 {
					t.Fatal("exhaustion did not resume parent exactly once")
				}
				if got.Status != WorkflowFailed || h.model.calls != 2 {
					t.Fatalf("budget failed: %+v", got)
				}
				if err := run(3); err != nil {
					t.Fatal(err)
				}
				if h.model.calls != 2 {
					t.Fatal("exhausted workflow called provider")
				}
				retried, err := h.service.RetryWorkflow(ctx, h.project.UUID, wf.UUID)
				if err != nil || retried.Steps[step.Position-1].Status != WorkflowQueued {
					t.Fatalf("manual retry=%+v err=%v", retried, err)
				}
				for _, s := range retried.Steps {
					if s.Position < step.Position && s.Status != WorkflowCompleted {
						t.Fatalf("manual retry rewound %s", s.StepKey)
					}
				}
			case "cancelled":
				if got.Status != WorkflowCancelled || h.model.calls != 1 {
					t.Fatal("cancelled workflow retried")
				}
			}
		})
	}
}
