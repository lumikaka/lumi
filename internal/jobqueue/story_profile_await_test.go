package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/llm"
	"lumi/internal/project"
	"lumi/internal/story"
)

func TestStoryProfileDirectTaskCreatesWorkflow(t *testing.T) {
	for _, kind := range []string{KindStoryProfileGeneration, KindStoryProfileFromChapters} {
		t.Run(kind, func(t *testing.T) {
			harness := newQueueHarness(t)
			harness.createChapter(t, "vol01.ch01")
			task, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, kind, "", CreateStoryWorkflowInput{
				ProviderUUID: harness.provider.UUID, Prompt: "月光信件", IdempotencyKey: "profile-direct",
			})
			if err != nil {
				t.Fatal(err)
			}
			var count int64
			if err := harness.runtime(t).store.DB().Table("workflow_steps").Where("task_uuid=?", task.UUID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("task has %d workflow steps", count)
			}
			_, err = harness.queue.awaitStoryDomainTask(context.Background(), harness.project.UUID, task, agent.DomainInvocationContext{AwaitCompletion: true})
			if err == nil || errors.Is(err, agent.ErrWaitingWorkflow) {
				t.Fatalf("missing await silently suspended caller: %v", err)
			}
		})
	}
}

func TestStoryProfileWorkflowCreationRollsBackOnProjectionFailure(t *testing.T) {
	for _, kind := range []string{KindStoryProfileGeneration, KindStoryProfileFromChapters} {
		t.Run(kind, func(t *testing.T) {
			harness := newQueueHarness(t)
			harness.createChapter(t, "vol01.ch01")
			db := harness.runtime(t).store.DB()
			if err := db.Exec(`CREATE TRIGGER reject_profile_workflow BEFORE INSERT ON workflows BEGIN SELECT RAISE(ABORT,'injected workflow failure'); END`).Error; err != nil {
				t.Fatal(err)
			}
			_, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, kind, "", CreateStoryWorkflowInput{ProviderUUID: harness.provider.UUID, Prompt: "月光信件", IdempotencyKey: "profile-atomic"})
			if err == nil {
				t.Fatal("task created without its workflow")
			}
			for _, table := range []string{"task_runs", "workflows", "workflow_steps", "workflow_awaits", "chat_threads"} {
				var count int64
				if err := db.Table(table).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("partial creation leaked %d %s", count, table)
				}
			}
		})
	}
}

func startProfileChat(t *testing.T, model *inlineWorkflowAgentModel) (inlineWorkflowTestEnv, agent.Thread, agent.Workflow) {
	t.Helper()
	env := setupInlineWorkflowTestEnv(t, model)
	if err := env.projects.WithCurrentStore(env.ctx, env.project.UUID, func(store *project.Store) error {
		_, err := story.NewService(store).UpdateStory(env.ctx, env.chapter.UUID, story.UpdateStoryInput{Content: "小狐狸送出一封月光信件。", ContentFormat: "txt", ExpectedRevision: env.chapter.Revision})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	thread, err := env.agents.CreateThread(env.ctx, env.project.UUID, agent.CreateThreadInput{Title: "总纲生成", ProviderUUID: env.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.agents.CreateTurn(env.ctx, env.project.UUID, thread.UUID, agent.CreateTurnInput{InputText: "请发起总纲任务"}); err != nil {
		t.Fatal(err)
	}
	if model.profileKind == KindStoryProfileFromChapters {
		waitTurnStatus(t, env, thread.UUID, agent.TurnWaitingForInput)
		requests, err := env.agents.ListUserInputRequests(env.ctx, env.project.UUID, thread.UUID)
		if err != nil || len(requests) != 1 {
			t.Fatalf("confirmation=%+v error=%v", requests, err)
		}
		question := requests[0].Questions[0]
		if _, err := env.agents.RespondUserInput(env.ctx, env.project.UUID, thread.UUID, requests[0].UUID, agent.UserInputResponse{
			Answers: map[string]agent.UserInputAnswer{question.ID: {SelectedOptionUUID: question.Options[1].UUID}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-model.storyStarted:
	case <-time.After(8 * time.Second):
		items, _ := env.agents.ListItems(env.ctx, env.project.UUID, thread.UUID, "", "", 100)
		for _, item := range items.Items {
			t.Logf("%s: %s", item.ItemType, item.Content)
		}
		t.Fatal("profile generation did not start")
	}
	waitTurnStatus(t, env, thread.UUID, agent.TurnWaitingForWorkflow)
	workflow := waitInlineWorkflowKind(t, env, thread.UUID, model.profileKind)
	if workflow.AwaitStatus != "waiting" || workflow.PresentationMode != string(agent.PresentationInline) || len(workflow.Steps) != 1 {
		t.Fatalf("profile workflow=%+v", workflow)
	}
	return env, thread, workflow
}

func TestStoryProfileChatTerminalStatesResume(t *testing.T) {
	for _, kind := range []string{KindStoryProfileGeneration, KindStoryProfileFromChapters} {
		for _, outcome := range []string{"completed", "failed", "cancelled"} {
			t.Run(kind+"/"+outcome, func(t *testing.T) {
				model := newInlineWorkflowAgentModel()
				model.profileKind = kind
				if outcome == "failed" {
					if kind == KindStoryProfileFromChapters {
						model.invalidProfile = true
					} else {
						model.storyErr = &llm.Error{Code: "test_generation_failed", SafeMessage: "测试生成失败。", Retryable: false}
					}
				}
				env, thread, workflow := startProfileChat(t, model)
				if outcome == "cancelled" {
					if _, err := env.agents.CancelWorkflow(env.ctx, env.project.UUID, workflow.UUID); err != nil {
						t.Fatal(err)
					}
				} else {
					close(model.releaseStory)
				}
				waitTurnStatus(t, env, thread.UUID, agent.TurnCompleted)
				workflow, err := env.agents.GetWorkflow(env.ctx, env.project.UUID, workflow.UUID)
				if err != nil || workflow.Status != outcome || workflow.AwaitStatus != "resumed" {
					t.Fatalf("terminal workflow=%+v error=%v", workflow, err)
				}
				if outcome != "completed" {
					assertProfileFailureResult(t, env, thread.UUID, workflow.Steps[0].TaskUUID, outcome == "failed" && model.invalidProfile)
				}
			})
		}
	}
}

func assertProfileFailureResult(t *testing.T, env inlineWorkflowTestEnv, threadUUID, taskUUID string, invalidPlans bool) {
	t.Helper()
	err := env.projects.WithCurrentStore(env.ctx, env.project.UUID, func(store *project.Store) error {
		var result string
		if err := store.DB().Raw(`SELECT x.result_json FROM agent_tool_executions x JOIN task_runs tasks ON tasks.idempotency_key=x.idempotency_key WHERE tasks.uuid=?`, taskUUID).Row().Scan(&result); err != nil {
			return err
		}
		var envelope struct {
			Success bool                           `json:"success"`
			Data    any                            `json:"data"`
			Error   struct{ Code, Message string } `json:"error"`
		}
		if err := json.Unmarshal([]byte(result), &envelope); err != nil || envelope.Success || envelope.Data != nil || envelope.Error.Code == "" || envelope.Error.Message == "" {
			t.Fatalf("failure envelope=%s error=%v", result, err)
		}
		if invalidPlans && (envelope.Error.Code != "llm_invalid_content" || !strings.Contains(envelope.Error.Message, "chapter_plans")) {
			t.Fatalf("missing validation reason: %s", result)
		}
		profile, err := story.NewService(store).GetStoryProfile(env.ctx)
		if err != nil {
			return err
		}
		var snapshotJSON string
		if err := store.DB().Raw("SELECT input_snapshot FROM task_runs WHERE uuid=?", taskUUID).Row().Scan(&snapshotJSON); err != nil {
			return err
		}
		var snapshot storyGenerationSnapshot
		if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
			return err
		}
		if profile.Revision != snapshot.ResourceRevision {
			t.Fatalf("failed generation changed profile: %+v", profile)
		}
		var count int64
		if err := store.DB().Table("chat_threads").Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("created shadow threads: %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStoryProfileReconcileRepairsMissingAwaitWithoutRegeneration(t *testing.T) {
	model := newInlineWorkflowAgentModel()
	model.profileKind, model.invalidProfile = KindStoryProfileFromChapters, true
	env, thread, workflow := startProfileChat(t, model)
	taskUUID := workflow.Steps[0].TaskUUID
	// Reproduce the old version's durable state: a running Chat tool and task,
	// but no projected Workflow or await to deliver its terminal result.
	if err := env.projects.WithCurrentStore(env.ctx, env.project.UUID, func(store *project.Store) error {
		for _, table := range []string{"workflow_awaits", "workflow_events", "workflow_steps"} {
			if err := store.DB().Exec("DELETE FROM "+table+" WHERE workflow_id=(SELECT id FROM workflows WHERE uuid=?)", workflow.UUID).Error; err != nil {
				return err
			}
		}
		return store.DB().Exec("DELETE FROM workflows WHERE uuid=?", workflow.UUID).Error
	}); err != nil {
		t.Fatal(err)
	}
	close(model.releaseStory)
	waitTaskStatus(t, env.queue, env.project.UUID, taskUUID, StatusFailed)
	if err := env.projects.WithCurrentStore(env.ctx, env.project.UUID, func(store *project.Store) error {
		db, err := store.DB().DB()
		if err != nil {
			return err
		}
		var projectID int64
		if err := store.DB().Raw("SELECT id FROM projects WHERE uuid=?", env.project.UUID).Scan(&projectID).Error; err != nil {
			return err
		}
		// A legacy tool can still say executing after the user cancels its
		// owner. Neither a terminal owner nor a pending cancellation may revive it.
		for _, table := range []string{"chat_runs", "chat_turns"} {
			var originalStatus string
			if err := store.DB().Raw("SELECT status FROM "+table+" WHERE thread_id=(SELECT id FROM chat_threads WHERE uuid=?)", thread.UUID).Row().Scan(&originalStatus); err != nil {
				return err
			}
			for _, assignment := range []string{"status='cancelled'", "cancel_requested_at=CURRENT_TIMESTAMP"} {
				if err := store.DB().Exec("UPDATE "+table+" SET "+assignment+" WHERE thread_id=(SELECT id FROM chat_threads WHERE uuid=?)", thread.UUID).Error; err != nil {
					return err
				}
				if err := reconcileStoryTaskWorkflows(env.ctx, db, projectID, time.Now().UTC()); err != nil {
					return err
				}
				var count int64
				if err := store.DB().Table("workflows").Count(&count).Error; err != nil {
					return err
				}
				if count != 0 {
					t.Fatalf("recovery revived cancelled owner: %s %s", table, assignment)
				}
				if err := store.DB().Exec("UPDATE "+table+" SET status=?,cancel_requested_at=NULL WHERE thread_id=(SELECT id FROM chat_threads WHERE uuid=?)", originalStatus, thread.UUID).Error; err != nil {
					return err
				}
			}
		}
		for range 2 {
			if err := reconcileStoryTaskWorkflows(env.ctx, db, projectID, time.Now().UTC()); err != nil {
				return err
			}
		}
		return env.agents.ReconcileOnOpen(env.ctx, store)
	}); err != nil {
		t.Fatal(err)
	}
	waitTurnStatus(t, env, thread.UUID, agent.TurnCompleted)
	assertProfileFailureResult(t, env, thread.UUID, taskUUID, true)
	if err := env.projects.WithCurrentStore(env.ctx, env.project.UUID, func(store *project.Store) error {
		if err := env.agents.ReconcileOnOpen(env.ctx, store); err != nil {
			return err
		}
		for table, where := range map[string]string{
			"workflow_awaits": "status='resumed'",
			"task_runs":       "kind='story_profile_from_chapters'",
			"llm_logs":        "scenario='story_profile_from_chapters'",
		} {
			var count int64
			if err := store.DB().Table(table).Where(where).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				t.Fatalf("%s count=%d, recovery duplicated work", table, count)
			}
		}
		var count int64
		if err := store.DB().Table("chat_items").Where("item_type='assistant_message'").Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("recovery replies=%d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
