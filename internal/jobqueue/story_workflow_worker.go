package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"lumi/internal/llm"
	"lumi/internal/production"
	"lumi/internal/realtime"
	"lumi/internal/story"
)

type workflowChapterPlan struct {
	ChapterCode string `json:"chapter_code"`
	Title       string `json:"title"`
	Outline     string `json:"outline"`
}

func (runtime *projectRuntime) loadStoryPromptResult(ctx context.Context, taskID int64) (string, bool, error) {
	var output string
	err := runtime.sqlDB.QueryRowContext(ctx, `SELECT output_json FROM story_prompt_results WHERE task_run_id=?`, taskID).Scan(&output)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return output, err == nil, err
}

func (runtime *projectRuntime) persistStoryPromptResult(ctx context.Context, record taskRecord, raw string) error {
	raw = strings.TrimSpace(raw)
	if !json.Valid([]byte(raw)) {
		return &llm.Error{Code: llm.CodeInvalidContent, SafeMessage: "模型没有返回规范的 JSON object。", Retryable: false}
	}
	uuid, err := newUUIDv7()
	if err != nil {
		return err
	}
	_, err = runtime.sqlDB.ExecContext(ctx, `INSERT INTO story_prompt_results(uuid,task_run_id,result_kind,output_json,created_at) VALUES(?,?,?,?,?) ON CONFLICT(task_run_id) DO NOTHING`, uuid, record.ID, record.Kind, raw, runtime.manager.now().UTC())
	return err
}

func (runtime *projectRuntime) applyStoryWorkflowResponse(ctx context.Context, record taskRecord, snapshot storyGenerationSnapshot, raw string) (map[string]any, error) {
	storyService := story.NewService(runtime.store)
	switch record.Kind {
	case KindStoryProfileGeneration, KindStoryProfileFromChapters:
		var output struct {
			StoryMD      string                `json:"story_md"`
			ChapterPlans []workflowChapterPlan `json:"chapter_plans"`
		}
		if err := json.Unmarshal([]byte(raw), &output); err != nil || strings.TrimSpace(output.StoryMD) == "" {
			return nil, invalidStoryWorkflowContent("模型返回的 STORY.md JSON 无效。", err)
		}
		if record.Kind == KindStoryProfileGeneration {
			if err := validateWorkflowPlans(output.ChapterPlans, snapshot.ChapterCount, nextChapterCodes(nil, snapshot.ChapterCount)); err != nil {
				return nil, err
			}
		} else if len(output.ChapterPlans) != 0 {
			return nil, invalidStoryWorkflowContent("profile_from_chapters 的 chapter_plans 必须为空数组。", nil)
		}
		profile, err := storyService.ApplyGeneratedStoryProfile(ctx, strings.TrimSpace(output.StoryMD), snapshot.ResourceRevision)
		if err != nil {
			return nil, err
		}
		chapterUUIDs := make([]string, 0, len(output.ChapterPlans))
		if len(output.ChapterPlans) > 0 {
			chapters, listErr := storyService.ListChapters(ctx, "active")
			if listErr != nil {
				return nil, listErr
			}
			byCode := make(map[string]story.Chapter, len(chapters))
			for _, chapter := range chapters {
				byCode[chapter.ChapterCode] = chapter
			}
			for _, plan := range output.ChapterPlans {
				chapter, exists := byCode[plan.ChapterCode]
				if !exists {
					chapter, err = storyService.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: plan.ChapterCode, Title: strings.TrimSpace(plan.Title), ContentFormat: "txt"})
					if err != nil {
						return nil, err
					}
				} else if chapter.Title != strings.TrimSpace(plan.Title) {
					return nil, invalidStoryWorkflowContent("生成计划与已有章节编号冲突。", nil)
				}
				chapterUUIDs = append(chapterUUIDs, chapter.UUID)
			}
		}
		return map[string]any{"project_uuid": runtime.projectUUID, "story_profile_uuid": profile.UUID, "chapter_uuids": chapterUUIDs}, nil

	case KindStoryChapterBatchPlan:
		var output struct {
			ChapterPlans []workflowChapterPlan `json:"chapter_plans"`
		}
		if err := json.Unmarshal([]byte(raw), &output); err != nil {
			return nil, invalidStoryWorkflowContent("模型返回的章节计划 JSON 无效。", err)
		}
		if err := validateWorkflowPlans(output.ChapterPlans, snapshot.ChapterCount, snapshot.TargetChapterCodes); err != nil {
			return nil, err
		}
		chapters, err := storyService.ListChapters(ctx, "active")
		if err != nil {
			return nil, err
		}
		byCode := make(map[string]story.Chapter, len(chapters))
		for _, chapter := range chapters {
			byCode[chapter.ChapterCode] = chapter
		}
		chapterUUIDs := make([]string, 0, len(output.ChapterPlans))
		for _, plan := range output.ChapterPlans {
			chapter, exists := byCode[plan.ChapterCode]
			if !exists {
				chapter, err = storyService.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: plan.ChapterCode, Title: strings.TrimSpace(plan.Title), ContentFormat: "txt"})
				if err != nil {
					return nil, err
				}
			} else if chapter.Title != strings.TrimSpace(plan.Title) {
				return nil, invalidStoryWorkflowContent("批量计划与已有章节编号冲突。", nil)
			}
			chapterUUIDs = append(chapterUUIDs, chapter.UUID)
		}
		return map[string]any{"project_uuid": runtime.projectUUID, "chapter_uuids": chapterUUIDs}, nil

	case KindComicStoryboardGeneration:
		generated, err := parseComicStoryboardResponse(raw, snapshot)
		if err != nil {
			return nil, err
		}
		sections, err := production.NewService(runtime.store, runtime.manager.hub).CreateGeneratedSections(ctx, snapshot.ChapterUUID, generated)
		if err != nil {
			return nil, err
		}
		sectionUUIDs := make([]string, len(sections))
		for index := range sections {
			sectionUUIDs[index] = sections[index].UUID
		}
		return map[string]any{"project_uuid": runtime.projectUUID, "chapter_uuid": snapshot.ChapterUUID, "section_uuids": sectionUUIDs}, nil
	}
	return nil, fmt.Errorf("unsupported story workflow kind %q", record.Kind)
}

func parseComicStoryboardResponse(raw string, snapshot storyGenerationSnapshot) ([]production.GeneratedComicSection, error) {
	maxSectionCount := normalizedComicMaxSectionCount(snapshot.MaxSectionCount)
	var output struct {
		ChapterCode string `json:"chapter_code"`
		Title       string `json:"title"`
		Sections    []struct {
			SectionNo  int    `json:"section_no"`
			Title      string `json:"title"`
			Storyboard string `json:"storyboard"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(raw), &output); err != nil || output.ChapterCode != snapshot.ChapterCode || len(output.Sections) < 1 || len(output.Sections) > maxSectionCount {
		return nil, invalidStoryWorkflowContent("模型返回的 Comic storyboard JSON 无效。", err)
	}
	generated := make([]production.GeneratedComicSection, len(output.Sections))
	for index, section := range output.Sections {
		if section.SectionNo != index+1 || strings.TrimSpace(section.Title) == "" || strings.TrimSpace(section.Storyboard) == "" {
			return nil, invalidStoryWorkflowContent("Comic sections 必须连续编号并包含 title/storyboard。", nil)
		}
		generated[index] = production.GeneratedComicSection{Title: section.Title, StoryboardMD: section.Storyboard}
	}
	return generated, nil
}

// Snapshots created before max_section_count was introduced decode the field
// as zero. Keep those durable tasks executable with the historical limit.
func normalizedComicMaxSectionCount(value int) int {
	if value < 1 || value > production.MaxGeneratedComicSections {
		return 6
	}
	return value
}

func validateWorkflowPlans(plans []workflowChapterPlan, count int, codes []string) error {
	if len(plans) != count || len(codes) != count {
		return invalidStoryWorkflowContent("chapter_plans 数量与任务快照不一致。", nil)
	}
	for index, plan := range plans {
		if strings.TrimSpace(plan.ChapterCode) != codes[index] || strings.TrimSpace(plan.Title) == "" || strings.TrimSpace(plan.Outline) == "" {
			return invalidStoryWorkflowContent("chapter_plans 编号或必填字段与任务快照不一致。", nil)
		}
	}
	return nil
}

func invalidStoryWorkflowContent(message string, cause error) error {
	return &llm.Error{Code: llm.CodeInvalidContent, SafeMessage: message, Retryable: false, Cause: cause}
}

func (runtime *projectRuntime) completeStoryWorkflowTask(ctx context.Context, record taskRecord, resultPayload map[string]any) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := runtime.manager.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE task_runs SET status='completed',progress=100,completed_at=?,updated_at=?,error_code='',error_message='' WHERE id=? AND cancel_requested_at IS NULL AND status<>'cancelled'`, now, now, record.ID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return context.Canceled
	}
	payload := map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": record.UUID, "resource_uuid": record.ResourceUUID, "status": StatusCompleted, "progress": 100, "result": resultPayload}
	if err := appendTaskEventTx(ctx, tx, record.ID, "task_completed", payload, now); err != nil {
		return err
	}
	var threadID, runID int64
	if err := tx.QueryRowContext(ctx, `SELECT agent_thread_id,id FROM agent_runs WHERE task_run_id=?`, record.ID).Scan(&threadID, &runID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_runs SET status='completed',completed_at=?,updated_at=? WHERE id=?`, now, now, runID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_threads SET status='completed',updated_at=? WHERE id=?`, now, threadID); err != nil {
		return err
	}
	if err := appendAgentEventTx(ctx, tx, threadID, &runID, "run_completed", payload, now); err != nil {
		return err
	}
	if isProjectedStoryTaskWorkflow(record.Kind) {
		if err := completeStoryTaskWorkflowTx(ctx, tx, record.UUID, resultPayload, now); err != nil {
			return err
		}
		if err := readyWorkflowAwaitsTx(ctx, runtime, tx, record.UUID, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	task := record.DTO()
	task.Status, task.Progress, task.CompletedAt, task.UpdatedAt = StatusCompleted, 100, &now, now
	runtime.broadcast("task:completed", task)
	if isProjectedStoryTaskWorkflow(record.Kind) {
		runtime.broadcastStoryTaskWorkflow("workflow:step_changed", record.UUID)
	}
	if runtime.manager.hub != nil {
		event := "story:chapters_changed"
		if record.Kind == KindStoryProfileGeneration || record.Kind == KindStoryProfileFromChapters {
			event = "story:profile_changed"
		} else if record.Kind == KindComicStoryboardGeneration {
			event = "comic:section_changed"
		}
		runtime.manager.hub.Broadcast(realtime.ProjectTopic(runtime.projectUUID), event, payload)
	}
	return nil
}
