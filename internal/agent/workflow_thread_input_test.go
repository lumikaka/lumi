package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestWorkflowThreadRejectsChatInputWithoutWrites(t *testing.T) {
	h := newAgentHarness(t)
	ctx := context.Background()
	for _, status := range []string{"busy", "waiting_for_input", "completed", "failed", "cancelled", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			thread := h.createThread(t)
			turn, err := h.service.CreateTurn(ctx, h.project.UUID, thread.UUID, CreateTurnInput{InputText: "legacy message"})
			if err != nil {
				t.Fatal(err)
			}
			follow, err := h.service.CreateFollowUp(ctx, h.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "legacy follow-up"})
			if err != nil {
				t.Fatal(err)
			}
			if err := h.store.DB().Table("chat_threads").Where("uuid=?", thread.UUID).Updates(map[string]any{"thread_type": ThreadTypeWorkflow, "status": status}).Error; err != nil {
				t.Fatal(err)
			}
			// A live legacy run must not make steering permissible either.
			for _, table := range []string{"chat_turns", "chat_runs"} {
				query := "uuid=?"
				if table == "chat_runs" {
					query = "turn_id=(SELECT id FROM chat_turns WHERE uuid=?)"
				}
				if err := h.store.DB().Table(table).Where(query, turn.UUID).Update("status", "in_progress").Error; err != nil {
					t.Fatal(err)
				}
			}
			before := workflowInputRowCounts(t, h)
			jobs := len(h.queue.jobs)
			for name, call := range map[string]func() error{
				"turn": func() error {
					_, err := h.service.CreateTurn(ctx, h.project.UUID, thread.UUID, CreateTurnInput{InputText: "continue"})
					return err
				},
				"follow-up": func() error {
					_, err := h.service.CreateFollowUp(ctx, h.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "continue"})
					return err
				},
				"steering": func() error {
					_, err := h.service.Steer(ctx, h.project.UUID, thread.UUID, SteeringInput{InputText: "continue"})
					return err
				},
				"queued steering": func() error {
					_, err := h.service.SteerFollowUp(ctx, h.project.UUID, thread.UUID, follow.UUID)
					return err
				},
				"edit follow-up": func() error {
					_, err := h.service.UpdateFollowUp(ctx, h.project.UUID, thread.UUID, follow.UUID, UpdateFollowUpInput{InputText: "changed"})
					return err
				},
			} {
				t.Run(name, func(t *testing.T) {
					var domainErr *Error
					if err := call(); !errors.As(err, &domainErr) || domainErr.Code != CodeWorkflowThreadReadOnly {
						t.Fatalf("expected read-only error, got %v", err)
					}
					if after := workflowInputRowCounts(t, h); !reflect.DeepEqual(before, after) {
						t.Fatalf("rejected input wrote rows: before=%v after=%v", before, after)
					}
					if len(h.queue.jobs) != jobs {
						t.Fatal("rejected input enqueued a job")
					}
				})
			}
			var saved followUpRecord
			if err := h.store.DB().Where("uuid=?", follow.UUID).First(&saved).Error; err != nil {
				t.Fatal(err)
			}
			if saved.InputText != "legacy follow-up" || saved.Status != "queued" {
				t.Fatalf("legacy follow-up changed: %+v", saved)
			}
		})
	}
	if len(h.model.requests) != 0 {
		t.Fatal("rejected input called the model")
	}
}

func workflowInputRowCounts(t *testing.T, h *agentHarness) map[string]int64 {
	t.Helper()
	counts := map[string]int64{}
	for _, table := range []string{"chat_turns", "chat_runs", "chat_items", "chat_events", "chat_follow_ups", "chat_context_references", "agent_tool_executions", "llm_logs"} {
		var count int64
		if err := h.store.DB().Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		counts[table] = count
	}
	return counts
}

func TestWorkflowThreadStopsLegacyChatJobsAndFollowUpPromotion(t *testing.T) {
	for _, kind := range []string{JobChatTurn, JobChatResume} {
		t.Run(kind, func(t *testing.T) {
			h := newAgentHarness(t)
			ctx := context.Background()
			thread := h.createThread(t)
			turn, err := h.service.CreateTurn(ctx, h.project.UUID, thread.UUID, CreateTurnInput{InputText: "legacy message"})
			if err != nil {
				t.Fatal(err)
			}
			follow, err := h.service.CreateFollowUp(ctx, h.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "legacy follow-up"})
			if err != nil {
				t.Fatal(err)
			}
			if err := h.store.DB().Table("chat_threads").Where("uuid=?", thread.UUID).Update("thread_type", ThreadTypeWorkflow).Error; err != nil {
				t.Fatal(err)
			}
			jobs := len(h.queue.jobs)
			if err := h.execute(t, thread.UUID, turn.UUID, kind); err != nil {
				t.Fatal(err)
			}
			beforeReplay := workflowInputRowCounts(t, h)
			if err := h.execute(t, thread.UUID, turn.UUID, kind); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(beforeReplay, workflowInputRowCounts(t, h)) {
				t.Fatal("replayed rejected job wrote duplicate records")
			}
			var run runRecord
			if err := h.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).First(&run).Error; err != nil {
				t.Fatal(err)
			}
			if run.Status != TurnCancelled || run.ErrorCode != CodeWorkflowThreadReadOnly || run.StartedAt != nil {
				t.Fatalf("legacy run was not stopped before execution: %+v", run)
			}
			sqlDB, err := h.store.DB().DB()
			if err != nil {
				t.Fatal(err)
			}
			tx, err := sqlDB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			pid, err := projectIDSQL(ctx, tx, h.project.UUID)
			if err != nil {
				t.Fatal(err)
			}
			record, err := lockThreadSQL(ctx, tx, pid, thread.UUID)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.service.promoteNextFollowUpTx(ctx, tx, h.project.UUID, &record, contextPromptSet{}); err != nil {
				t.Fatal(err)
			}
			// The shared transaction primitive must reject bypasses as well.
			_, _, err = h.service.createTurnTx(ctx, tx, h.project.UUID, &record, "bypass", "prompt", 0, contextPromptSet{}, nil)
			var domainErr *Error
			if !errors.As(err, &domainErr) || domainErr.Code != CodeWorkflowThreadReadOnly {
				t.Fatalf("transaction bypass accepted: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			var saved followUpRecord
			if err := h.store.DB().Where("uuid=?", follow.UUID).First(&saved).Error; err != nil {
				t.Fatal(err)
			}
			if saved.Status != "queued" || len(h.queue.jobs) != jobs || len(h.model.requests) != 0 {
				t.Fatal("legacy input was promoted or executed")
			}
		})
	}
}
