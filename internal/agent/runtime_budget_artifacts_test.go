package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"lumi/internal/llm"
	"lumi/internal/production"
	"lumi/internal/story"
)

func TestBudgetFinalizationReconcilesSavedComicPages(t *testing.T) {
	for _, withStoryboard := range []bool{false, true} {
		t.Run(map[bool]string{false: "structure only", true: "usable storyboard"}[withStoryboard], func(t *testing.T) {
			h := newAgentHarness(t)
			h.service.turnBudget = turnBudgetLimits{MaxModelRequests: 2, MaxTokenUnits: 1_000_000, MaxActiveDurationMS: 1_000_000, MaxNoProgressRounds: 20}
			ctx := context.Background()
			chapter, err := story.NewService(h.store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "预算对账"})
			if err != nil {
				t.Fatal(err)
			}
			thread := h.createThread(t)
			turn, err := h.service.CreateTurn(ctx, h.project.UUID, thread.UUID, CreateTurnInput{InputText: "创建整章绘本"})
			if err != nil {
				t.Fatal(err)
			}
			body := map[string]any{"title": "已保存页"}
			if withStoryboard {
				body["storyboard_md"] = "角色推开门，发现大厅空无一人。"
			}
			arguments, _ := json.Marshal(map[string]any{
				"method": "POST", "url": "/api/v1/projects/" + h.project.UUID + "/chapters/" + chapter.UUID + "/comic-sections",
				"request_body": body,
				// The public UI receipt must still identify the created page when
				// the model deliberately omits uuid and the large storyboard.
				"response_filter": ".data | {revision}",
			})
			h.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
				if call == 1 {
					return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "save-page", Name: "request_api", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
				}
				return llm.ChatResponse{}, &llm.Error{Code: llm.CodeNetwork, SafeMessage: "收尾请求失败"}
			}
			if err := h.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
				t.Fatal(err)
			}
			var run runRecord
			if err := h.store.DB().Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Take(&run).Error; err != nil {
				t.Fatal(err)
			}
			if run.Status != TurnCompleted || run.ErrorCode != "" || run.LimitReason != BudgetReasonModelRequests || h.model.calls != 2 {
				t.Fatalf("run=%+v model calls=%d", run, h.model.calls)
			}
			var item itemRecord
			if err := h.store.DB().Where("run_id=? AND item_type='assistant_message'", run.ID).Take(&item).Error; err != nil {
				t.Fatal(err)
			}
			var metadata struct {
				CompletionStatus string   `json:"completion_status"`
				SectionUUIDs     []string `json:"section_uuids"`
				ReadyCount       int      `json:"storyboard_ready_count"`
				RuntimeGenerated bool     `json:"runtime_generated"`
			}
			if err := json.Unmarshal([]byte(item.MetadataJSON), &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata.CompletionStatus != "partial" || len(metadata.SectionUUIDs) != 1 || metadata.ReadyCount != int(boolInt64(withStoryboard)) || !metadata.RuntimeGenerated || !strings.Contains(item.Content, "仅部分完成") || strings.Contains(item.Content, "仅建结构、需再生成分镜") == withStoryboard {
				t.Fatalf("item=%+v metadata=%+v", item, metadata)
			}
			// Recovered finalization must not emit a second terminal message.
			tc, err := h.service.loadToolContext(ctx, h.store, thread.UUID, turn.UUID)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.service.failBudgetRun(ctx, h.store, tc); err != nil {
				t.Fatal(err)
			}
			var count int64
			if err := h.store.DB().Table("chat_items").Where("run_id=? AND item_type='assistant_message'", run.ID).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("final messages=%d err=%v", count, err)
			}
		})
	}
}

func TestBudgetReconciliationDoesNotClaimUnrelatedOrDeletedPages(t *testing.T) {
	for _, scenario := range []string{"unrelated", "deleted", "failed receipt", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			h := newAgentHarness(t)
			ctx := context.Background()
			chapter, err := story.NewService(h.store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "旧内容"})
			if err != nil {
				t.Fatal(err)
			}
			section, err := production.NewService(h.store, nil).CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "已有页", StoryboardMD: "已有分镜"})
			if err != nil {
				t.Fatal(err)
			}
			thread := h.createThread(t)
			turn, err := h.service.CreateTurn(ctx, h.project.UUID, thread.UUID, CreateTurnInput{InputText: "未执行创作"})
			if err != nil {
				t.Fatal(err)
			}
			tc, err := h.service.loadToolContext(ctx, h.store, thread.UUID, turn.UUID)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.service.claimRun(ctx, h.store, &tc); err != nil {
				t.Fatal(err)
			}
			tc.ToolMode, err = h.service.loadRunToolMode(ctx, h.store, tc)
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "unrelated" {
				args, _ := json.Marshal(map[string]any{
					"method": "POST", "url": "/api/v1/projects/" + h.project.UUID + "/chapters/" + chapter.UUID + "/comic-sections/" + section.UUID + "/storyboard-variants",
					"request_body":    map[string]any{"content_md": "新分镜", "expected_revision": section.Revision},
					"response_filter": ".data | {uuid,revision}",
				})
				execution, _, _, err := h.service.persistToolIntent(ctx, h.store, tc, "page-receipt", "request_api", string(args))
				if err != nil {
					t.Fatal(err)
				}
				result, _ := json.Marshal(map[string]any{"success": scenario != "failed receipt", "data": map[string]any{"uuid": section.UUID}})
				if err := h.service.persistToolResult(ctx, h.store, tc, execution, result); err != nil {
					t.Fatal(err)
				}
				if scenario == "deleted" {
					if err := h.store.DB().Exec("UPDATE comic_sections SET deleted_at=CURRENT_TIMESTAMP WHERE uuid=?", section.UUID).Error; err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "cancelled" {
					if err := h.store.DB().Exec("UPDATE chat_runs SET cancel_requested_at=CURRENT_TIMESTAMP WHERE id=?", tc.Run.ID).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := h.service.failBudgetRun(ctx, h.store, tc); err != nil {
				t.Fatal(err)
			}
			run, err := h.service.refreshRun(ctx, h.store, tc.Run.ID)
			wantStatus, wantCode := TurnFailed, CodeTurnBudget
			if scenario == "cancelled" {
				wantStatus, wantCode = TurnCancelled, CodeCancelled
			}
			if err != nil || run.Status != wantStatus || run.ErrorCode != wantCode {
				t.Fatalf("run=%+v err=%v", run, err)
			}
		})
	}
}

func TestBudgetReconciliationFindsAwaitedStoryboardWorkflowArtifacts(t *testing.T) {
	h := newAgentHarness(t)
	ctx := context.Background()
	chapter, err := story.NewService(h.store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "委托分镜"})
	if err != nil {
		t.Fatal(err)
	}
	sections, err := production.NewService(h.store, nil).CreateGeneratedSections(ctx, chapter.UUID, []production.GeneratedComicSection{{Title: "后端生成页", StoryboardMD: "完整可用分镜"}})
	if err != nil {
		t.Fatal(err)
	}
	thread := h.createThread(t)
	turn, err := h.service.CreateTurn(ctx, h.project.UUID, thread.UUID, CreateTurnInput{InputText: "生成整章分镜"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := h.service.loadToolContext(ctx, h.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.claimRun(ctx, h.store, &tc); err != nil {
		t.Fatal(err)
	}
	tc.ToolMode, err = h.service.loadRunToolMode(ctx, h.store, tc)
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"path": agentDocBasePath + "/overview.md"})
	execution, _, _, err := h.service.persistToolIntent(ctx, h.store, tc, "workflow-receipt", "read_agent_doc", string(args))
	if err != nil {
		t.Fatal(err)
	}
	now := h.service.now().UTC()
	workflowID, workflowUUID := insertWorkflowTrajectoryRow(t, h, "workflows", map[string]any{
		"project_id": tc.Thread.ProjectID, "thread_id": tc.Thread.ID, "kind": WorkflowComicStoryboard, "title": "Storyboard", "status": "completed",
		"input_snapshot": "{}", "idempotency_key": "budget:" + tc.Run.UUID, "provider_uuid": h.provider.UUID, "model": "test-model", "created_at": now, "updated_at": now,
	})
	output, _ := json.Marshal(map[string]any{"section_uuids": []string{sections[0].UUID, sections[0].UUID}})
	insertWorkflowTrajectoryRow(t, h, "workflow_steps", map[string]any{
		"workflow_id": workflowID, "step_key": WorkflowStepComicStoryboard, "position": 1, "status": "completed", "output_json": string(output),
		"idempotency_key": workflowUUID + ":storyboard", "created_at": now, "updated_at": now,
	})
	insertWorkflowTrajectoryRow(t, h, "workflow_awaits", map[string]any{
		"workflow_id": workflowID, "chat_thread_id": tc.Thread.ID, "chat_turn_id": tc.Turn.ID, "chat_run_id": tc.Run.ID,
		"tool_execution_id": execution.ID, "status": "resumed", "created_at": now, "updated_at": now,
	})
	if err := h.service.failBudgetRun(ctx, h.store, tc); err != nil {
		t.Fatal(err)
	}
	run, err := h.service.refreshRun(ctx, h.store, tc.Run.ID)
	if err != nil || run.Status != TurnCompleted || run.ErrorCode != "" {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	var item itemRecord
	if err := h.store.DB().Where("run_id=? AND item_type='assistant_message'", run.ID).Take(&item).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(item.MetadataJSON, `"persisted_section_count":1`) || !strings.Contains(item.MetadataJSON, `"storyboard_ready_count":1`) || !strings.Contains(item.MetadataJSON, sections[0].UUID) {
		t.Fatalf("workflow artifacts not reconciled/deduplicated: %s", item.MetadataJSON)
	}
}
