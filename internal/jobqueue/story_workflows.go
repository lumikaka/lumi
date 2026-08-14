package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"lumi/internal/modelsettings"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/story"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// CreateStoryWorkflow freezes and queues one of the supported planning
// steps which does not directly write a chapter body.
func (manager *Manager) CreateStoryWorkflow(ctx context.Context, projectUUID, kind, chapterUUID string, input CreateStoryWorkflowInput) (Task, error) {
	runtime, err := manager.runtimeFor(projectUUID)
	if err != nil {
		return Task{}, err
	}
	input.ProviderUUID = strings.TrimSpace(input.ProviderUUID)
	input.Model = strings.TrimSpace(input.Model)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	chapterUUID = strings.TrimSpace(chapterUUID)
	if !validStoryWorkflowKind(kind) || (input.ProviderUUID != "" && !isUUIDv7(input.ProviderUUID)) || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 255 || len(input.Prompt) > 256<<10 {
		return Task{}, taskError(CodeInvalidTask, "Story workflow 输入无效", "prompt、idempotency_key 或资源 UUID 不符合要求。", nil)
	}
	if input.Parameters.Temperature != nil && (*input.Parameters.Temperature < 0 || *input.Parameters.Temperature > 2) {
		return Task{}, taskError(CodeInvalidTask, "生成参数无效", "temperature 必须在 0 到 2 之间。", nil)
	}
	if input.Parameters.MaxTokens < 0 || input.Parameters.MaxTokens > 200000 {
		return Task{}, taskError(CodeInvalidTask, "生成参数无效", "max_tokens 超出安全范围。", nil)
	}
	if (kind == KindStoryProfileGeneration || kind == KindStoryChapterBatchPlan) && input.Prompt == "" {
		return Task{}, taskError(CodeInvalidTask, "创作提示词不能为空", "该生成步骤需要非空 prompt。", nil)
	}
	if kind == KindComicStoryboardGeneration && !isUUIDv7(chapterUUID) {
		return Task{}, taskError(CodeInvalidTask, "章节 UUID 无效", "chapter_uuid 必须是 UUIDv7。", nil)
	}
	if kind == KindComicStoryboardGeneration {
		if input.MaxSectionCount == nil {
			value := 6
			input.MaxSectionCount = &value
		} else if *input.MaxSectionCount < 1 || *input.MaxSectionCount > production.MaxGeneratedComicSections {
			return Task{}, taskError(CodeInvalidTask, "漫画 Section 上限无效", fmt.Sprintf("max_section_count 必须在 1 到 %d 之间。", production.MaxGeneratedComicSections), nil)
		}
	}
	if input.ChapterCount == 0 {
		input.ChapterCount = 1
	}
	if input.ChapterCount < 1 || input.ChapterCount > 20 {
		return Task{}, taskError(CodeInvalidTask, "章节数量无效", "chapter_count 必须在 1 到 20 之间。", nil)
	}

	language, err := loadProjectGenerationLanguage(ctx, runtime.store)
	if err != nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "无法读取项目生成语言", "任务尚未创建。", err)
	}
	resolved, model, modelSource, err := manager.resolveProjectModel(ctx, runtime.store, modelsettings.StoryText, modelsettings.KindText, input.ProviderUUID, input.Model)
	if err != nil {
		return Task{}, err
	}
	service := story.NewService(runtime.store)
	snapshot, resourceUUID, err := buildStoryWorkflowSnapshot(ctx, service, projectUUID, kind, chapterUUID, input, language)
	if err != nil {
		return Task{}, taskError(CodeInvalidTask, "Story workflow 提示词无法渲染", "请检查输入和项目提示词的规范占位符。", err)
	}
	snapshot.ProviderUUID = resolved.UUID
	snapshot.ProviderType = resolved.ProviderType
	snapshot.ProviderBaseURL = resolved.BaseURL
	snapshot.Model = model
	snapshot.ModelSource = modelSource
	snapshot.Parameters = input.Parameters
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "无法固化生成输入", "任务尚未创建。", err)
	}
	return manager.persistStoryWorkflowTask(ctx, runtime, kind, resourceUUID, input.IdempotencyKey, input.Prompt, resolved.UUID, model, modelSource, encoded)
}

func buildStoryWorkflowSnapshot(ctx context.Context, service *story.Service, projectUUID, kind, chapterUUID string, input CreateStoryWorkflowInput, language string) (storyGenerationSnapshot, string, error) {
	profile, err := service.GetStoryProfile(ctx)
	if err != nil {
		return storyGenerationSnapshot{}, "", err
	}
	pictureBook := service.PictureBookProfile()
	snapshot := storyGenerationSnapshot{Version: 3, ProjectUUID: projectUUID, GenerationLanguage: language, WorkflowKind: kind, ResourceRevision: profile.Revision, ChapterCount: input.ChapterCount, PictureBook: &pictureBook}
	group, promptKey, systemKey, resourceUUID := promptcatalog.GroupStory, "", "json_system", projectUUID
	values := map[string]string{}
	switch kind {
	case KindStoryProfileGeneration:
		promptKey = "story_profile"
		values = map[string]string{"input_prompt": input.Prompt, "chapter_count": fmt.Sprintf("%d", input.ChapterCount)}
	case KindStoryProfileFromChapters:
		promptKey = "profile_from_chapters"
		chapters, listErr := service.ListChapters(ctx, "active")
		if listErr != nil {
			return snapshot, "", listErr
		}
		items := make([]map[string]any, 0, len(chapters))
		for _, chapter := range chapters {
			if chapter.CurrentStory == nil || strings.TrimSpace(chapter.CurrentStory.Content) == "" {
				continue
			}
			items = append(items, map[string]any{"chapter_code": chapter.ChapterCode, "title": chapter.Title, "content": chapter.CurrentStory.Content, "content_format": chapter.CurrentStory.ContentFormat})
		}
		if len(items) == 0 {
			return snapshot, "", fmt.Errorf("no generated chapter prose")
		}
		encoded, marshalErr := json.Marshal(items)
		if marshalErr != nil {
			return snapshot, "", marshalErr
		}
		values = map[string]string{"chapters_json": string(encoded)}
	case KindStoryChapterBatchPlan:
		promptKey = "chapter_batch_plan"
		chapters, listErr := service.ListChapters(ctx, "active")
		if listErr != nil {
			return snapshot, "", listErr
		}
		codes := nextChapterCodes(chapters, input.ChapterCount)
		snapshot.TargetChapterCodes = codes
		var previous any
		if len(chapters) > 0 {
			last := chapters[len(chapters)-1]
			if last.CurrentStory != nil {
				previous = map[string]any{"chapter_code": last.ChapterCode, "title": last.Title, "content": last.CurrentStory.Content, "content_format": last.CurrentStory.ContentFormat}
			}
		}
		previousJSON, _ := json.Marshal(previous)
		codesJSON, _ := json.Marshal(codes)
		values = map[string]string{"input_prompt": input.Prompt, "story_md": profile.StoryMD, "previous_chapter_json": string(previousJSON), "target_chapter_codes_json": string(codesJSON), "chapter_count": fmt.Sprintf("%d", input.ChapterCount)}
	case KindComicStoryboardGeneration:
		group, promptKey, systemKey, resourceUUID = promptcatalog.GroupChapter, "comic_storyboard", "json_system", chapterUUID
		chapter, chapterErr := service.GetChapter(ctx, chapterUUID)
		if chapterErr != nil {
			return snapshot, "", chapterErr
		}
		if chapter.CurrentStory == nil || strings.TrimSpace(chapter.CurrentStory.Content) == "" {
			return snapshot, "", fmt.Errorf("chapter has no prose")
		}
		snapshot.ChapterUUID, snapshot.ChapterCode, snapshot.ChapterRevision = chapter.UUID, chapter.ChapterCode, chapter.Revision
		chapterJSON, _ := json.Marshal(map[string]any{"chapter_code": chapter.ChapterCode, "title": chapter.Title, "content_format": chapter.CurrentStory.ContentFormat})
		inputText := input.Prompt
		if inputText == "" {
			inputText = chapter.CurrentStory.Content
		}
		snapshot.MaxSectionCount = *input.MaxSectionCount
		snapshot.MomentCountPlan = pictureBookMomentCountPlan(pictureBook, snapshot.MaxSectionCount)
		planJSON, _ := json.Marshal(snapshot.MomentCountPlan)
		values = map[string]string{"chapter_context_json": string(chapterJSON), "story_md": profile.StoryMD, "input_text": inputText, "moment_count_plan": string(planJSON), "chapter_code": chapter.ChapterCode, "max_section_count": fmt.Sprintf("%d", snapshot.MaxSectionCount), "max_moments_per_section": fmt.Sprintf("%d", maxPictureBookMoments(pictureBook))}
	default:
		return snapshot, "", fmt.Errorf("unsupported workflow kind %q", kind)
	}
	template, err := service.EffectivePrompt(ctx, group, promptKey)
	if err != nil {
		return snapshot, "", err
	}
	systemPrompt, err := service.EffectivePrompt(ctx, group, systemKey)
	if err != nil {
		return snapshot, "", err
	}
	languageInstruction, err := service.EffectiveLanguageInstruction(ctx)
	if err != nil {
		return snapshot, "", err
	}
	rendered, err := promptcatalog.Render(template, values)
	if err != nil {
		return snapshot, "", err
	}
	snapshot.PromptKey, snapshot.PromptTemplate = promptKey, template
	snapshot.SystemPrompt, snapshot.Prompt = promptcatalog.WithInstruction(systemPrompt, languageInstruction), rendered
	return snapshot, resourceUUID, nil
}

func comicMomentCountPlan(maxSectionCount int) []int {
	base := [...]int{2, 3, 1}
	result := make([]int, maxSectionCount)
	for index := range result {
		result[index] = base[index%len(base)]
	}
	return result
}

func pictureBookMomentCountPlan(profile project.PictureBookProfile, maxSectionCount int) []int {
	if profile.Format == project.PictureBookVertical {
		return comicMomentCountPlan(maxSectionCount)
	}
	base := []int{1}
	if profile.Format == project.PictureBookComicStory && profile.ComicLayout != nil {
		if *profile.ComicLayout == project.ComicLayoutFourPanel {
			base = []int{4}
		} else {
			base = []int{4, 5, 3, 6}
		}
	}
	result := make([]int, maxSectionCount)
	for index := range result {
		result[index] = base[index%len(base)]
	}
	return result
}

func maxPictureBookMoments(profile project.PictureBookProfile) int {
	if profile.Format == project.PictureBookVertical {
		return 3
	}
	if profile.Format == project.PictureBookComicStory && profile.ComicLayout != nil {
		if *profile.ComicLayout == project.ComicLayoutFourPanel {
			return 4
		}
		return 6
	}
	return 1
}

func nextChapterCodes(chapters []story.Chapter, count int) []string {
	volume, number := 1, 0
	for _, chapter := range chapters {
		if chapter.VolumeNo > volume || (chapter.VolumeNo == volume && chapter.ChapterNo > number) {
			volume, number = chapter.VolumeNo, chapter.ChapterNo
		}
	}
	result := make([]string, count)
	for index := range result {
		result[index] = fmt.Sprintf("vol%02d.ch%02d", volume, number+index+1)
	}
	return result
}

func (manager *Manager) persistStoryWorkflowTask(ctx context.Context, runtime *projectRuntime, kind, resourceUUID, idempotencyKey, summary, providerUUID, model, modelSource string, snapshot []byte) (Task, error) {
	taskUUID, err := newUUIDv7()
	if err != nil {
		return Task{}, err
	}
	now := manager.now().UTC()
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	if existing, found, findErr := findTaskTx(ctx, tx, runtime.projectID, kind, idempotencyKey); findErr != nil {
		return Task{}, findErr
	} else if found {
		if err := tx.Commit(); err != nil {
			return Task{}, err
		}
		return existing.DTO(), nil
	}
	if active, found, findErr := findActiveStoryWorkflowTx(ctx, tx, runtime.projectID, kind, resourceUUID); findErr != nil {
		return Task{}, findErr
	} else if found {
		return Task{}, taskError(CodeTaskConflict, "资源已有进行中的生成任务", "请等待任务 "+active.UUID+" 完成。", nil)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO task_runs (uuid,project_id,river_job_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,retryable,provider_uuid,model,model_source,progress,attempt,max_attempts,error_code,error_message,created_at,updated_at) VALUES (?,?,NULL,?,?,2,?,'queued',?,1,?,?,?,0,0,3,'','',?,?)`, taskUUID, runtime.projectID, kind, resourceUUID, string(snapshot), idempotencyKey, providerUUID, model, modelSource, now, now)
	if err != nil {
		return Task{}, taskError(CodeTaskPersistenceFailed, "无法持久化 Story workflow", "任务与 River job 均未创建。", err)
	}
	taskID, err := result.LastInsertId()
	if err != nil {
		return Task{}, err
	}
	if err := appendTaskEventTx(ctx, tx, taskID, "task_queued", map[string]any{"project_uuid": runtime.projectUUID, "task_uuid": taskUUID, "resource_uuid": resourceUUID, "status": StatusQueued, "progress": 0}, now); err != nil {
		return Task{}, err
	}
	if _, _, err := createAgentAuditTx(ctx, tx, runtime.projectID, taskID, taskUUID, resourceUUID, providerUUID, model, summary, now); err != nil {
		return Task{}, err
	}
	if isProjectedStoryTaskWorkflow(kind) {
		if err := createStoryTaskWorkflowTx(ctx, tx, runtime.projectID, runtime.projectUUID, kind, resourceUUID, taskUUID, providerUUID, model, modelSource, snapshot, now); err != nil {
			return Task{}, err
		}
	}
	inserted, err := runtime.client.InsertTx(ctx, tx, riverArgs{Version: 1, ProjectUUID: runtime.projectUUID, TaskUUID: taskUUID, TaskKind: kind, ResourceUUID: resourceUUID}, &river.InsertOpts{Queue: QueueStory, MaxAttempts: 3, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled}}})
	if err != nil {
		return Task{}, err
	}
	if inserted.UniqueSkippedAsDuplicate {
		return Task{}, taskError(CodeTaskConflict, "资源已有互斥生成任务", "River unique job 拒绝了重复任务。", nil)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE task_runs SET river_job_id=? WHERE id=?", inserted.Job.ID, taskID); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	task, err := manager.GetTask(ctx, runtime.projectUUID, taskUUID)
	if err == nil {
		runtime.broadcast("task:queued", task)
		if isProjectedStoryTaskWorkflow(kind) {
			runtime.broadcastStoryTaskWorkflow("workflow:queued", taskUUID)
		}
	}
	return task, err
}

func findActiveStoryWorkflowTx(ctx context.Context, tx *sql.Tx, projectID int64, kind, resourceUUID string) (taskRecord, bool, error) {
	return scanTask(tx.QueryRowContext(ctx, taskSelectSQL+" WHERE project_id=? AND kind=? AND resource_uuid=? AND status IN ('queued','running','waiting_for_input') LIMIT 1", projectID, kind, resourceUUID))
}

func validStoryWorkflowKind(kind string) bool {
	switch kind {
	case KindStoryProfileGeneration, KindStoryProfileFromChapters, KindStoryChapterBatchPlan, KindComicStoryboardGeneration:
		return true
	}
	return false
}

func validStoryTaskKind(kind string) bool {
	return kind == KindStoryChapterGeneration || validStoryWorkflowKind(kind)
}
