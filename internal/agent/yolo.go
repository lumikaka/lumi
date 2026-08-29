package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"lumi/internal/llm"
	"lumi/internal/llmlog"
	"lumi/internal/modelsettings"
	"lumi/internal/picturebook"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/story"

	"gorm.io/gorm"
)

type yoloSnapshot struct {
	Version               int                         `json:"version"`
	ProjectUUID           string                      `json:"project_uuid"`
	GenerationLanguage    string                      `json:"generation_language"`
	StoryPrompt           string                      `json:"story_prompt"`
	ProviderUUID          string                      `json:"provider_uuid"`
	Model                 string                      `json:"model"`
	ModelSource           string                      `json:"model_source"`
	ImageProviderUUID     string                      `json:"image_provider_uuid"`
	ImageModel            string                      `json:"image_model"`
	ImageModelSource      string                      `json:"image_model_source"`
	SelectionProviderUUID string                      `json:"selection_provider_uuid"`
	SelectionModel        string                      `json:"selection_model"`
	SelectionModelSource  string                      `json:"selection_model_source"`
	Prompts               map[string]string           `json:"prompts,omitempty"`
	PictureBook           *project.PictureBookProfile `json:"picture_book,omitempty"`
	OutputSize            string                      `json:"output_size,omitempty"`
}

func (service *Service) CreateYoloWorkflow(ctx context.Context, projectUUID string, input CreateYoloInput) (Workflow, error) {
	title, err := validateText(input.Title, 640, "Workflow 标题")
	if err != nil || len([]rune(title)) > 160 {
		return Workflow{}, domainError(CodeValidation, "Workflow 标题无效", "title 需要 1 到 160 个字符。", err)
	}
	prompt, err := validateText(input.StoryPrompt, 16000, "故事创意")
	if err != nil || len([]rune(prompt)) > 4000 {
		return Workflow{}, domainError(CodeValidation, "故事创意无效", "story_prompt 需要 1 到 4000 个字符。", err)
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if len(key) < 8 || len(key) > 160 || (strings.TrimSpace(input.ProviderUUID) != "" && !isUUIDv7(strings.TrimSpace(input.ProviderUUID))) {
		return Workflow{}, domainError(CodeValidation, "Yolo 参数无效", "idempotency_key 需要 8 到 160 字符。", nil)
	}
	var createdUUID string
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		if err := store.RequireReady(); err != nil {
			return err
		}
		if err := store.DB().WithContext(ctx).Raw(`SELECT workflows.uuid
			FROM workflows JOIN projects ON projects.id=workflows.project_id
			WHERE projects.uuid=? AND workflows.kind=? AND workflows.idempotency_key=?
			LIMIT 1`, projectUUID, WorkflowYolo, key).Scan(&createdUUID).Error; err != nil {
			return err
		}
		if createdUUID != "" {
			return nil
		}
		storyService := story.NewService(store)
		detail, err := storyService.GetProject(ctx)
		if err != nil {
			return err
		}
		resolved, model, modelSource, err := service.resolveProjectModel(ctx, store, modelsettings.StoryText, modelsettings.KindText, input.ProviderUUID, input.Model)
		if err != nil {
			return err
		}
		imageResolved, imageModel, imageModelSource, err := service.resolveProjectModel(ctx, store, modelsettings.ProjectImage, modelsettings.KindImage, input.ProviderUUID, "")
		if err != nil {
			return err
		}
		pictureBook, err := store.RequirePictureBookProfile()
		if err != nil {
			return err
		}
		outputSize, err := picturebook.ResolveImageSize(pictureBook, imageResolved.ProviderType, imageModel)
		if err != nil {
			return domainError(picturebook.CodeAspectRatioUnsupported, "图片模型不支持项目比例", "请切换到支持该精确比例的图片模型后重试；Yolo 尚未创建。", err)
		}
		selectionResolved, selectionModel, selectionModelSource, err := service.resolveProjectModel(ctx, store, modelsettings.SectionPremiseSelection, modelsettings.KindText, input.ProviderUUID, "")
		if err != nil {
			return err
		}
		frozenPrompts := make(map[string]string, 8)
		for _, identity := range [][2]string{{promptcatalog.GroupStory, "json_system"}, {promptcatalog.GroupStory, "story_profile"}, {promptcatalog.GroupStory, "profile_from_chapters"}, {promptcatalog.GroupChapter, "json_system"}, {promptcatalog.GroupChapter, "comic_storyboard"}, {promptcatalog.GroupChapter, "cover_storyboard"}, {promptcatalog.GroupPremiseStyle, "project_overall_style"}, {promptcatalog.GroupRuntime, "project_language_instruction"}} {
			value, loadErr := storyService.EffectivePrompt(ctx, identity[0], identity[1])
			if loadErr != nil {
				return loadErr
			}
			frozenPrompts[identity[0]+"/"+identity[1]] = value
		}
		sqlDB, err := store.DB().DB()
		if err != nil {
			return err
		}
		tx, err := sqlDB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		pid, err := projectIDSQL(ctx, tx, projectUUID)
		if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT uuid FROM workflows WHERE project_id=? AND kind=? AND idempotency_key=?`, pid, WorkflowYolo, key).Scan(&createdUUID); err == nil {
			return tx.Commit()
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		threadUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		workflowUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		now := service.now().UTC()
		result, err := tx.ExecContext(ctx, `INSERT INTO chat_threads(uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(?,?,?,'busy','workflow',?,?,?,1,1,1,?,?)`, threadUUID, pid, "Yolo · "+title, resolved.UUID, model, modelSource, now, now)
		if err != nil {
			return err
		}
		threadID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(yoloSnapshot{Version: 5, ProjectUUID: projectUUID, GenerationLanguage: detail.GenerationLanguage, StoryPrompt: prompt, ProviderUUID: resolved.UUID, Model: model, ModelSource: modelSource, ImageProviderUUID: imageResolved.UUID, ImageModel: imageModel, ImageModelSource: imageModelSource, SelectionProviderUUID: selectionResolved.UUID, SelectionModel: selectionModel, SelectionModelSource: selectionModelSource, Prompts: frozenPrompts, PictureBook: &pictureBook, OutputSize: outputSize.String()})
		result, err = tx.ExecContext(ctx, `INSERT INTO workflows(uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,created_at,updated_at) VALUES(?,?,?,? ,?,'queued',1,?,?,?,?,?,?,?)`, workflowUUID, pid, threadID, WorkflowYolo, title, string(snapshot), key, resolved.UUID, model, modelSource, now, now)
		if err != nil {
			return err
		}
		workflowID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		var firstStepUUID string
		for index, stepKey := range YoloStepKeys {
			stepUUID, err := newUUIDv7()
			if err != nil {
				return err
			}
			status := "pending"
			if index == 0 {
				status = "queued"
				firstStepUUID = stepUUID
			}
			stepIDKey := workflowUUID + ":" + stepKey
			if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,input_json,output_json,created_at,updated_at) VALUES(?,?,?,?,?,?, '{}','{}',?,?)`, stepUUID, workflowID, stepKey, index+1, status, stepIDKey, now, now); err != nil {
				return err
			}
		}
		thread := threadRecord{ID: threadID, UUID: threadUUID, ProjectID: pid, NextItemSequence: 1, NextEventSequence: 1}
		if _, err := appendItemTx(ctx, tx, &thread, nil, nil, "assistant_message", "assistant", "Yolo 快速创作已启动。进度会持久保存，关闭应用后仍可恢复。", "text", "completed", "", "", workflowUUID, map[string]any{"workflow_uuid": workflowUUID}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=? WHERE id=?`, thread.NextItemSequence, threadID); err != nil {
			return err
		}
		if err := appendWorkflowEventTx(ctx, tx, workflowID, nil, "workflow_queued", map[string]any{"project_uuid": projectUUID, "workflow_uuid": workflowUUID, "thread_uuid": threadUUID, "status": WorkflowQueued}, now); err != nil {
			return err
		}
		jobID, err := service.queue.EnqueueAgentTx(ctx, projectUUID, tx, JobSpec{Version: 1, ProjectUUID: projectUUID, JobKind: JobWorkflowStep, ResourceUUID: firstStepUUID, ThreadUUID: threadUUID})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET river_job_id=? WHERE uuid=?`, jobID, firstStepUUID); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		createdUUID = workflowUUID
		return nil
	})
	if err != nil {
		return Workflow{}, err
	}
	workflow, err := service.GetWorkflow(ctx, projectUUID, createdUUID)
	if err == nil {
		service.broadcastWorkflow(projectUUID, workflow, "workflow:queued", "")
	}
	return workflow, err
}

func appendWorkflowEventTx(ctx context.Context, tx *sql.Tx, workflowID int64, stepID *int64, eventType string, payload any, now time.Time) error {
	uuid, err := newUUIDv7()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var sequence int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM workflow_events WHERE workflow_id=?`, workflowID).Scan(&sequence); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO workflow_events(uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at) VALUES(?,?,?,?,?,?,?)`, uuid, workflowID, stepID, sequence, eventType, string(encoded), now)
	return err
}

func appendWorkflowEventGormTx(ctx context.Context, tx *gorm.DB, workflowID int64, stepID *int64, eventType string, payload any, now time.Time) error {
	uuid, err := newUUIDv7()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var sequence int64
	if err := tx.WithContext(ctx).Raw(`SELECT COALESCE(MAX(sequence),0)+1 FROM workflow_events WHERE workflow_id=?`, workflowID).Scan(&sequence).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO workflow_events(uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at) VALUES(?,?,?,?,?,?,?)`, uuid, workflowID, stepID, sequence, eventType, string(encoded), now).Error
}

func (service *Service) ListWorkflows(ctx context.Context, projectUUID string) ([]Workflow, error) {
	var result []Workflow
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		var rows []workflowRecord
		if err := store.DB().WithContext(ctx).Where("project_id=?", pid).Order("created_at DESC,id DESC").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			workflow, err := service.workflowDTO(ctx, store, projectUUID, row)
			if err != nil {
				return err
			}
			result = append(result, workflow)
		}
		return nil
	})
	return result, err
}

func (service *Service) GetWorkflow(ctx context.Context, projectUUID, workflowUUID string) (Workflow, error) {
	if !isUUIDv7(workflowUUID) {
		return Workflow{}, domainError(CodeValidation, "Workflow UUID 无效", "workflow_uuid 必须是 UUIDv7。", nil)
	}
	var result Workflow
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		var row workflowRecord
		if err := store.DB().WithContext(ctx).Where("project_id=? AND uuid=?", pid, workflowUUID).First(&row).Error; err != nil {
			return notFound(err, "Workflow 不存在")
		}
		result, err = service.workflowDTO(ctx, store, projectUUID, row)
		return err
	})
	return result, err
}

func (service *Service) workflowDTO(ctx context.Context, store *project.Store, projectUUID string, row workflowRecord) (Workflow, error) {
	var threadUUID, threadType string
	if row.ThreadID != nil {
		_ = store.DB().WithContext(ctx).Raw(`SELECT uuid,thread_type FROM chat_threads WHERE id=?`, *row.ThreadID).Row().Scan(&threadUUID, &threadType)
	}
	var await struct {
		Status, TurnUUID, RunUUID, ToolCallUUID, ItemUUID string
	}
	_ = store.DB().WithContext(ctx).Raw(`SELECT a.status,t.uuid,r.uuid,x.tool_call_uuid,i.uuid
		FROM workflow_awaits a
		JOIN chat_turns t ON t.id=a.chat_turn_id
		JOIN chat_runs r ON r.id=a.chat_run_id
		JOIN agent_tool_executions x ON x.id=a.tool_execution_id
		JOIN chat_items i ON i.id=x.item_id
		WHERE a.workflow_id=? LIMIT 1`, row.ID).Row().Scan(&await.Status, &await.TurnUUID, &await.RunUUID, &await.ToolCallUUID, &await.ItemUUID)
	presentationMode := string(PresentationNone)
	if await.Status != "" {
		presentationMode = string(PresentationInline)
	} else if threadType == ThreadTypeWorkflow {
		presentationMode = string(PresentationDedicatedThread)
	}
	var steps []workflowStepRecord
	if err := store.DB().WithContext(ctx).Where("workflow_id=?", row.ID).Order("position,id").Find(&steps).Error; err != nil {
		return Workflow{}, err
	}
	progressByTask := make(map[string]int, len(steps))
	taskUUIDs := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.TaskUUID != "" {
			taskUUIDs = append(taskUUIDs, step.TaskUUID)
		}
	}
	if len(taskUUIDs) > 0 {
		var rows []struct {
			UUID     string
			Progress int
		}
		if err := store.DB().WithContext(ctx).Table("task_runs").Select("uuid,progress").Where("project_id=? AND uuid IN ?", row.ProjectID, taskUUIDs).Scan(&rows).Error; err != nil {
			return Workflow{}, err
		}
		for _, task := range rows {
			progressByTask[task.UUID] = task.Progress
		}
	}
	dto := Workflow{UUID: row.UUID, ProjectUUID: projectUUID, ThreadUUID: threadUUID, PresentationMode: presentationMode, OriginTurnUUID: await.TurnUUID, OriginRunUUID: await.RunUUID, OriginToolCallUUID: await.ToolCallUUID, OriginItemUUID: await.ItemUUID, AwaitStatus: await.Status, Kind: row.Kind, Title: row.Title, Status: row.Status, InputVersion: row.InputVersion, InputSnapshot: sanitizeDiagnosticJSON(row.InputSnapshot), ProviderUUID: row.ProviderUUID, Model: row.Model, ModelSource: row.ModelSource, CurrentStepKey: row.CurrentStepKey, ErrorCode: row.ErrorCode, ErrorMessage: publicDiagnosticErrorMessage(row.ErrorCode), CancelRequestedAt: row.CancelRequestedAt, StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	for _, step := range steps {
		progress, found := progressByTask[step.TaskUUID]
		if !found && step.Status == WorkflowCompleted {
			progress = 100
		}
		dto.Steps = append(dto.Steps, workflowStepDTO(step, progress))
	}
	return dto, nil
}

func workflowStepDTO(row workflowStepRecord, progress int) WorkflowStep {
	if progress < 0 {
		progress = 0
	} else if progress > 100 {
		progress = 100
	}
	return WorkflowStep{UUID: row.UUID, StepKey: row.StepKey, Position: row.Position, Status: row.Status, Progress: progress, TaskUUID: row.TaskUUID, ResourceUUID: row.ResourceUUID, Input: sanitizeDiagnosticJSON(row.InputJSON), Output: sanitizeDiagnosticJSON(row.OutputJSON), ErrorCode: row.ErrorCode, ErrorMessage: publicDiagnosticErrorMessage(row.ErrorCode), StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func (service *Service) ExecuteWorkflowStep(ctx context.Context, store *project.Store, stepUUID string) error {
	workflow, step, threadUUID, err := service.loadWorkflowStep(ctx, store, stepUUID)
	if err != nil {
		return err
	}
	if workflow.Status == WorkflowCompleted || workflow.Status == WorkflowCancelled || step.Status == "completed" || step.Status == "cancelled" {
		return nil
	}
	if workflow.CancelRequestedAt != nil {
		return service.finishWorkflowCancelled(context.WithoutCancel(ctx), store, workflow, step, threadUUID)
	}
	if ready, err := service.workflowStepReady(ctx, store, step); err != nil {
		return err
	} else if !ready {
		return ErrJobNotReady
	}
	started, err := service.markWorkflowStepRunning(ctx, store, workflow, step)
	if err != nil {
		return err
	}
	if !started {
		return nil
	}
	workflow.Status, workflow.CurrentStepKey = WorkflowRunning, step.StepKey
	step.Status = "running"
	output, wait, err := service.runYoloStep(ctx, store, workflow, step)
	if err != nil {
		return service.failWorkflowStep(context.WithoutCancel(ctx), store, workflow, step, threadUUID, err)
	}
	if wait {
		waiting, err := service.markWorkflowStepWaiting(ctx, store, workflow, step)
		if err != nil {
			return err
		}
		if !waiting {
			return nil
		}
		return ErrJobNotReady
	}
	return service.completeWorkflowStep(ctx, store, workflow, step, threadUUID, output)
}

func (service *Service) loadWorkflowStep(ctx context.Context, store *project.Store, stepUUID string) (workflowRecord, workflowStepRecord, string, error) {
	var step workflowStepRecord
	if err := store.DB().WithContext(ctx).Where("uuid=?", stepUUID).First(&step).Error; err != nil {
		return workflowRecord{}, step, "", notFound(err, "Workflow step 不存在")
	}
	var workflow workflowRecord
	if err := store.DB().WithContext(ctx).Where("id=?", step.WorkflowID).First(&workflow).Error; err != nil {
		return workflow, step, "", err
	}
	var threadUUID string
	if workflow.ThreadID != nil {
		_ = store.DB().WithContext(ctx).Raw(`SELECT uuid FROM chat_threads WHERE id=?`, *workflow.ThreadID).Scan(&threadUUID).Error
	}
	return workflow, step, threadUUID, nil
}

func (service *Service) workflowStepReady(ctx context.Context, store *project.Store, step workflowStepRecord) (bool, error) {
	var count int64
	err := store.DB().WithContext(ctx).Model(&workflowStepRecord{}).Where("workflow_id=? AND position<? AND status<>'completed'", step.WorkflowID, step.Position).Count(&count).Error
	return count == 0, err
}

var errWorkflowTransitionInactive = errors.New("workflow transition is no longer active")

func (service *Service) markWorkflowStepRunning(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord) (bool, error) {
	now := service.now().UTC()
	err := store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&workflowRecord{}).Where("id=? AND status IN ('queued','running','interrupted')", workflow.ID).Updates(map[string]any{"status": WorkflowRunning, "current_step_key": step.StepKey, "started_at": gorm.Expr("COALESCE(started_at,?)", now), "updated_at": now, "error_code": "", "error_message": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errWorkflowTransitionInactive
		}
		result = tx.Model(&workflowStepRecord{}).Where("id=? AND status IN ('queued','waiting','running','interrupted')", step.ID).Updates(map[string]any{"status": "running", "started_at": gorm.Expr("COALESCE(started_at,?)", now), "updated_at": now, "error_code": "", "error_message": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errWorkflowTransitionInactive
		}
		return appendWorkflowEventGormTx(ctx, tx, workflow.ID, &step.ID, "step_running", map[string]any{"project_uuid": store.ProjectUUID(), "workflow_uuid": workflow.UUID, "step_uuid": step.UUID, "step_key": step.StepKey, "status": "running"}, now)
	})
	if errors.Is(err, errWorkflowTransitionInactive) {
		return false, nil
	}
	return err == nil, err
}

func (service *Service) markWorkflowStepWaiting(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord) (bool, error) {
	now := service.now().UTC()
	if err := store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&workflowStepRecord{}).Where("id=? AND status='running'", step.ID).Updates(map[string]any{"status": "waiting", "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errWorkflowTransitionInactive
		}
		return appendWorkflowEventGormTx(ctx, tx, workflow.ID, &step.ID, "step_waiting", map[string]any{"project_uuid": store.ProjectUUID(), "workflow_uuid": workflow.UUID, "step_uuid": step.UUID, "step_key": step.StepKey, "status": "waiting"}, now)
	}); err != nil {
		if errors.Is(err, errWorkflowTransitionInactive) {
			return false, nil
		}
		return false, err
	}
	service.broadcastWorkflow(store.ProjectUUID(), Workflow{UUID: workflow.UUID, ThreadUUID: "", Status: WorkflowRunning}, "workflow:step_changed", step.UUID)
	return true, nil
}

func (service *Service) runYoloStep(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord) (map[string]any, bool, error) {
	var snapshot yoloSnapshot
	if err := json.Unmarshal([]byte(workflow.InputSnapshot), &snapshot); err != nil || (snapshot.Version != 1 && snapshot.Version != 2 && snapshot.Version != 3 && snapshot.Version != 4 && snapshot.Version != 5) || snapshot.ProjectUUID != store.ProjectUUID() {
		return nil, false, domainError(CodeStateConflict, "Yolo 输入快照损坏", "workflow 无法安全恢复。", err)
	}
	if snapshot.ImageProviderUUID == "" {
		snapshot.ImageProviderUUID, snapshot.ImageModel = snapshot.ProviderUUID, snapshot.Model
		snapshot.ImageModelSource = "legacy_frozen"
	}
	if snapshot.SelectionProviderUUID == "" {
		snapshot.SelectionProviderUUID, snapshot.SelectionModel = snapshot.ProviderUUID, snapshot.Model
		snapshot.SelectionModelSource = "legacy_frozen"
	}
	switch step.StepKey {
	case "project_initialization":
		projectDetail, err := story.NewService(store).GetProject(ctx)
		if err != nil {
			return nil, false, err
		}
		return map[string]any{"project_uuid": projectDetail.UUID}, false, nil
	case "story":
		return service.runYoloStory(ctx, store, workflow, step, snapshot)
	case "story_profile":
		return service.runYoloStoryProfile(ctx, store, workflow, step, snapshot)
	case "premise":
		return service.runYoloPremise(ctx, store, workflow, step, snapshot)
	case "comic_sections":
		return service.runYoloComic(ctx, store, workflow, step, snapshot)
	case "first_section_image":
		return service.runYoloFirstImage(ctx, store, workflow, step, snapshot)
	default:
		return nil, false, domainError(CodeStateConflict, "未知 Yolo step", "step_key 不受支持。", nil)
	}
}

func (service *Service) runYoloStory(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, snapshot yoloSnapshot) (map[string]any, bool, error) {
	storyService := story.NewService(store)
	chapters, err := storyService.ListChapters(ctx, "active")
	if err != nil {
		return nil, false, err
	}
	var chapter story.Chapter
	for _, candidate := range chapters {
		if candidate.ChapterCode == "vol01.ch01" {
			chapter = candidate
			break
		}
	}
	if chapter.UUID == "" {
		template, promptErr := service.yoloPrompt(ctx, store, snapshot, promptcatalog.GroupStory, "story_profile")
		if promptErr != nil {
			return nil, false, promptErr
		}
		rendered, promptErr := promptcatalog.Render(template, map[string]string{"input_prompt": snapshot.StoryPrompt, "chapter_count": "1"})
		if promptErr != nil {
			return nil, false, domainError(CodeValidation, "Story Profile 提示词无法渲染", "项目提示词缺少规范占位符。", promptErr)
		}
		var planned struct {
			StoryMD      string `json:"story_md"`
			ChapterPlans []struct {
				ChapterCode string `json:"chapter_code"`
				Title       string `json:"title"`
				Outline     string `json:"outline"`
			} `json:"chapter_plans"`
		}
		if promptErr := service.completeYoloJSON(ctx, store, workflow, step, snapshot, "story_profile_generation", service.yoloSystemPrompt(ctx, store, snapshot, promptcatalog.GroupStory), rendered, &planned); promptErr != nil {
			return nil, false, promptErr
		}
		if strings.TrimSpace(planned.StoryMD) == "" || len(planned.ChapterPlans) != 1 || planned.ChapterPlans[0].ChapterCode != "vol01.ch01" || strings.TrimSpace(planned.ChapterPlans[0].Title) == "" || strings.TrimSpace(planned.ChapterPlans[0].Outline) == "" {
			return nil, false, domainError(llm.CodeInvalidContent, "Story Profile 生成失败", "模型返回的 story_md 或 chapter_plans 无效。", nil)
		}
		profile, profileErr := storyService.GetStoryProfile(ctx)
		if profileErr != nil {
			return nil, false, profileErr
		}
		if strings.TrimSpace(profile.StoryMD) != strings.TrimSpace(planned.StoryMD) {
			if _, profileErr = storyService.UpdateStoryProfile(ctx, planned.StoryMD, profile.Revision); profileErr != nil {
				return nil, false, profileErr
			}
		}
		plan := planned.ChapterPlans[0]
		chapter, err = storyService.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: plan.ChapterCode, Title: plan.Title, ContentFormat: "txt"})
		if err != nil {
			return nil, false, err
		}
		snapshot.StoryPrompt = snapshot.StoryPrompt + "\n\nChapter plan outline:\n" + plan.Outline
	}
	if chapter.CurrentStory != nil && strings.TrimSpace(chapter.CurrentStory.Content) != "" {
		return map[string]any{"chapter_uuid": chapter.UUID, "story_uuid": chapter.CurrentStory.UUID}, false, nil
	}
	task, err := service.ensureWorkflowDomainTask(ctx, store, step, DomainTaskRequest{Kind: "story_chapter_generation", ResourceUUID: chapter.UUID, ChapterUUID: chapter.UUID, ProviderUUID: snapshot.ProviderUUID, Model: snapshot.Model, Prompt: snapshot.StoryPrompt, IdempotencyKey: step.IdempotencyKey + ":chapter"})
	if err != nil {
		return nil, false, err
	}
	if task.Status == "queued" || task.Status == "running" || task.Status == "waiting_for_input" {
		return nil, true, nil
	}
	if task.Status != "completed" {
		return nil, false, domainError(task.ErrorCode, "Story 生成失败", task.ErrorMessage, nil)
	}
	chapter, err = storyService.GetChapter(ctx, chapter.UUID)
	if err != nil || chapter.CurrentStory == nil {
		return nil, false, domainError(CodeStateConflict, "Story 结果缺失", "任务完成后没有 current story。", err)
	}
	return map[string]any{"chapter_uuid": chapter.UUID, "story_uuid": chapter.CurrentStory.UUID, "task_uuid": task.UUID}, false, nil
}

func (service *Service) runYoloStoryProfile(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, snapshot yoloSnapshot) (map[string]any, bool, error) {
	storyService := story.NewService(store)
	chapterUUID, err := service.workflowOutputUUID(ctx, store, workflow.ID, "story", "chapter_uuid")
	if err != nil {
		return nil, false, err
	}
	chapter, err := storyService.GetChapter(ctx, chapterUUID)
	if err != nil || chapter.CurrentStory == nil {
		return nil, false, domainError(CodeStateConflict, "第一章正文缺失", "无法生成 Story Profile。", err)
	}
	chaptersJSON, _ := json.Marshal([]map[string]string{{"chapter_code": chapter.ChapterCode, "title": chapter.Title, "content": chapter.CurrentStory.Content, "content_format": chapter.CurrentStory.ContentFormat}})
	template, err := service.yoloPrompt(ctx, store, snapshot, promptcatalog.GroupStory, "profile_from_chapters")
	if err != nil {
		return nil, false, err
	}
	rendered, err := promptcatalog.Render(template, map[string]string{"chapters_json": string(chaptersJSON)})
	if err != nil {
		return nil, false, domainError(CodeValidation, "Story Profile 反推提示词无法渲染", "项目提示词缺少规范占位符。", err)
	}
	var generated struct {
		StoryMD string `json:"story_md"`
	}
	if err := service.completeYoloJSON(ctx, store, workflow, step, snapshot, "story_profile_from_chapters", service.yoloSystemPrompt(ctx, store, snapshot, promptcatalog.GroupStory), rendered, &generated); err != nil {
		return nil, false, err
	}
	desired := strings.TrimSpace(generated.StoryMD)
	if desired == "" {
		return nil, false, domainError(llm.CodeInvalidContent, "Story Profile 生成失败", "模型返回的 story_md 为空。", nil)
	}
	profile, err := storyService.GetStoryProfile(ctx)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(profile.StoryMD) != strings.TrimSpace(desired) {
		profile, err = storyService.UpdateStoryProfile(ctx, desired, profile.Revision)
		if err != nil {
			return nil, false, err
		}
	}
	return map[string]any{"story_profile_uuid": profile.UUID}, false, nil
}

func (service *Service) runYoloPremise(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, snapshot yoloSnapshot) (map[string]any, bool, error) {
	styleSnapshot, styleErr := service.yoloPrompt(ctx, store, snapshot, promptcatalog.GroupPremiseStyle, "project_overall_style")
	if styleErr != nil {
		styleSnapshot = production.DefaultPremiseStyle(snapshot.GenerationLanguage)
	}
	storyProfile, err := story.NewService(store).GetStoryProfile(ctx)
	if err != nil {
		return nil, false, err
	}
	productionService := production.NewService(store, service.hub)
	sources, err := productionService.ListPremiseSources(ctx)
	if err != nil {
		return nil, false, err
	}
	var source production.PremiseSource
	for _, candidate := range sources {
		if strings.TrimSpace(candidate.SourceText) == strings.TrimSpace(storyProfile.StoryMD) {
			source = candidate
			break
		}
	}
	if source.UUID == "" {
		source, err = productionService.CreatePremiseSource(ctx, production.CreateSourceInput{SourceText: storyProfile.StoryMD, StyleSnapshot: styleSnapshot, SourceType: "generated", ProviderUUID: snapshot.ProviderUUID, Model: snapshot.Model, Parameters: map[string]any{"workflow_uuid": workflow.UUID}})
		if err != nil {
			return nil, false, err
		}
	}
	images, err := productionService.ListSettingImages(ctx)
	if err != nil {
		return nil, false, err
	}
	var setting production.SettingImage
	for _, candidate := range images {
		if candidate.SourceUUID == source.UUID {
			setting = candidate
			break
		}
	}
	if setting.UUID == "" {
		task, err := service.ensureWorkflowDomainTask(ctx, store, step, DomainTaskRequest{Kind: "premise_setting_generation", ResourceUUID: source.UUID, ProviderUUID: snapshot.ImageProviderUUID, Model: snapshot.ImageModel, Prompt: source.SourceText, IdempotencyKey: step.IdempotencyKey + ":setting"})
		if err != nil {
			return nil, false, err
		}
		if task.Status == "queued" || task.Status == "running" {
			return nil, true, nil
		}
		if task.Status != "completed" {
			return nil, false, domainError(task.ErrorCode, "Premise 设置图生成失败", task.ErrorMessage, nil)
		}
		images, _ = productionService.ListSettingImages(ctx)
		for _, candidate := range images {
			if candidate.SourceUUID == source.UUID {
				setting = candidate
				break
			}
		}
	}
	if setting.UUID == "" {
		return nil, false, domainError(CodeStateConflict, "Premise 设置图缺失", "生成任务完成后没有设置图。", nil)
	}
	assets, err := productionService.ListPremiseAssets(ctx, "", "active")
	if err != nil {
		return nil, false, err
	}
	if len(assets) == 0 {
		task, err := service.ensureWorkflowDomainTask(ctx, store, step, DomainTaskRequest{Kind: "premise_asset_breakdown", ResourceUUID: setting.UUID, ProviderUUID: snapshot.ProviderUUID, Model: snapshot.Model, Prompt: snapshot.StoryPrompt, IdempotencyKey: step.IdempotencyKey + ":breakdown"})
		if err != nil {
			return nil, false, err
		}
		if task.Status == "queued" || task.Status == "running" {
			return nil, true, nil
		}
		if task.Status != "completed" {
			return nil, false, domainError(task.ErrorCode, "Premise 拆分失败", task.ErrorMessage, nil)
		}
		assets, err = productionService.ListPremiseAssets(ctx, "", "active")
		if err != nil {
			return nil, false, err
		}
	}
	assetUUIDs := make([]string, 0, len(assets))
	for _, asset := range assets {
		assetUUIDs = append(assetUUIDs, asset.UUID)
	}
	return map[string]any{"premise_source_uuid": source.UUID, "setting_image_uuid": setting.UUID, "premise_asset_uuids": assetUUIDs}, false, nil
}

func (service *Service) runYoloComic(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, snapshot yoloSnapshot) (map[string]any, bool, error) {
	chapterUUID, err := service.workflowOutputUUID(ctx, store, workflow.ID, "story", "chapter_uuid")
	if err != nil {
		return nil, false, err
	}
	chapter, err := story.NewService(store).GetChapter(ctx, chapterUUID)
	if err != nil || chapter.CurrentStory == nil {
		return nil, false, domainError(CodeStateConflict, "Comic 源章节缺失", "第一章没有 current story。", err)
	}
	productionService := production.NewService(store, service.hub)
	sections, err := productionService.ListSections(ctx, chapterUUID)
	if err != nil {
		return nil, false, err
	}
	bodySections, frontCover := yoloSectionsByRole(sections)
	if len(bodySections) > 6 {
		return nil, false, domainError(CodeStateConflict, "Comic Section 数量冲突", "Yolo 不会覆盖已有的 6 个以上 Section。", nil)
	}
	if len(bodySections) == 0 {
		profile, err := story.NewService(store).GetStoryProfile(ctx)
		if err != nil {
			return nil, false, err
		}
		chapterContext, _ := json.Marshal(map[string]any{"chapter_code": chapter.ChapterCode, "title": chapter.Title, "content": chapter.CurrentStory.Content, "content_format": chapter.CurrentStory.ContentFormat})
		template, err := service.yoloPrompt(ctx, store, snapshot, promptcatalog.GroupChapter, "comic_storyboard")
		if err != nil {
			return nil, false, err
		}
		pictureBook, err := store.RequirePictureBookProfile()
		if err != nil {
			return nil, false, err
		}
		momentPlan, maxMoments := yoloPageMomentPlan(pictureBook)
		rendered, err := promptcatalog.Render(template, map[string]string{
			"chapter_context_json": string(chapterContext), "story_md": profile.StoryMD,
			"input_text": chapter.CurrentStory.Content, "moment_count_plan": momentPlan,
			"chapter_code": chapter.ChapterCode, "max_section_count": "6", "max_moments_per_section": maxMoments,
		})
		if err != nil {
			return nil, false, domainError(CodeValidation, "Comic Storyboard 提示词无法渲染", "项目提示词缺少规范占位符。", err)
		}
		var generated struct {
			ChapterCode string `json:"chapter_code"`
			Sections    []struct {
				SectionNo  int    `json:"section_no"`
				Title      string `json:"title"`
				Storyboard string `json:"storyboard"`
			} `json:"sections"`
		}
		if err := service.completeYoloJSON(ctx, store, workflow, step, snapshot, "comic_storyboard_generation", service.yoloSystemPrompt(ctx, store, snapshot, promptcatalog.GroupChapter), rendered, &generated); err != nil {
			return nil, false, err
		}
		if generated.ChapterCode != chapter.ChapterCode || len(generated.Sections) < 1 || len(generated.Sections) > 6 {
			return nil, false, domainError(llm.CodeInvalidContent, "Comic Storyboard 生成失败", "模型返回的 chapter_code 或 sections 数量无效。", nil)
		}
		generatedSections := make([]production.GeneratedComicSection, len(generated.Sections))
		for index, item := range generated.Sections {
			if item.SectionNo != index+1 || strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Storyboard) == "" {
				return nil, false, domainError(llm.CodeInvalidContent, "Comic Storyboard 生成失败", "模型返回的 section 字段或顺序无效。", nil)
			}
			generatedSections[index] = production.GeneratedComicSection{Title: item.Title, StoryboardMD: item.Storyboard}
		}
		sections, err = productionService.CreateGeneratedSections(ctx, chapterUUID, generatedSections)
		if err != nil {
			return nil, false, err
		}
		bodySections, frontCover = yoloSectionsByRole(sections)
	}
	if len(bodySections) == 0 {
		return nil, false, domainError(CodeStateConflict, "正文页面缺失", "Yolo 页面规划完成后没有 body Section。", nil)
	}
	for _, section := range bodySections {
		if section.CurrentStoryboard == nil {
			return nil, false, domainError(CodeStateConflict, "Storyboard 缺失", "每个 Yolo Section 必须有 current storyboard。", nil)
		}
	}
	if snapshot.Version >= 5 {
		if snapshot.PictureBook == nil {
			return nil, false, domainError(CodeStateConflict, "Yolo 绘本快照缺失", "v5 workflow 无法判断是否需要封面。", nil)
		}
		if snapshot.PictureBook.Format != project.PictureBookVertical {
			frontCover, err = service.ensureYoloFrontCover(ctx, store, workflow, step, snapshot, chapter, bodySections[0], frontCover)
			if err != nil {
				return nil, false, err
			}
		}
	}
	sectionUUIDs := make([]string, 0, len(bodySections))
	for _, section := range bodySections {
		sectionUUIDs = append(sectionUUIDs, section.UUID)
	}
	output := map[string]any{
		"chapter_uuid": chapterUUID, "section_uuids": sectionUUIDs,
		"first_section_uuid": sectionUUIDs[0], "body_section_uuid": sectionUUIDs[0],
	}
	if frontCover.UUID != "" {
		output["cover_section_uuid"] = frontCover.UUID
	}
	return output, false, nil
}

func yoloSectionsByRole(sections []production.ComicSection) ([]production.ComicSection, production.ComicSection) {
	body := make([]production.ComicSection, 0, len(sections))
	var cover production.ComicSection
	for _, section := range sections {
		role := strings.TrimSpace(section.PageRole)
		if role == "" {
			role = production.PageRoleBody
		}
		switch role {
		case production.PageRoleFrontCover:
			if cover.UUID == "" {
				cover = section
			}
		case production.PageRoleBody:
			body = append(body, section)
		}
	}
	return body, cover
}

type yoloCoverDraft struct {
	Title      string `json:"title"`
	Storyboard string `json:"storyboard"`
}

func (service *Service) ensureYoloFrontCover(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, snapshot yoloSnapshot, chapter story.Chapter, firstBody, cover production.ComicSection) (production.ComicSection, error) {
	if cover.UUID != "" && (cover.CurrentStoryboard != nil || cover.CurrentImage != nil) {
		return cover, nil
	}
	draft, found := yoloCoverDraftCheckpoint(step.OutputJSON)
	if !found {
		profile, err := story.NewService(store).GetStoryProfile(ctx)
		if err != nil {
			return production.ComicSection{}, err
		}
		chapterContext, _ := json.Marshal(map[string]any{
			"chapter_code": chapter.ChapterCode, "title": chapter.Title,
			"content": chapter.CurrentStory.Content, "content_format": chapter.CurrentStory.ContentFormat,
		})
		template, err := service.yoloPrompt(ctx, store, snapshot, promptcatalog.GroupChapter, "cover_storyboard")
		if err != nil {
			return production.ComicSection{}, err
		}
		rendered, err := promptcatalog.Render(template, map[string]string{
			"book_title": chapter.Title, "chapter_context_json": string(chapterContext),
			"story_md": profile.StoryMD, "story_prompt": snapshot.StoryPrompt,
			"first_body_storyboard": firstBody.CurrentStoryboard.ContentMD,
		})
		if err != nil {
			return production.ComicSection{}, domainError(CodeValidation, "封面分镜提示词无法渲染", "项目提示词缺少规范占位符。", err)
		}
		if err := service.completeYoloJSON(ctx, store, workflow, step, snapshot, "cover_storyboard_generation", service.yoloSystemPrompt(ctx, store, snapshot, promptcatalog.GroupChapter), rendered, &draft); err != nil {
			return production.ComicSection{}, err
		}
		draft.Title, draft.Storyboard = strings.TrimSpace(draft.Title), strings.TrimSpace(draft.Storyboard)
		if draft.Title != strings.TrimSpace(chapter.Title) || draft.Storyboard == "" || len([]rune(draft.Storyboard)) > 262144 {
			return production.ComicSection{}, domainError(llm.CodeInvalidContent, "封面分镜生成失败", "模型必须逐字返回绘本标题和有效 storyboard。", nil)
		}
		if err := service.checkpointWorkflowStepOutput(ctx, store, step.ID, map[string]any{"cover_storyboard_draft": draft}); err != nil {
			return production.ComicSection{}, err
		}
	}
	productionService := production.NewService(store, service.hub)
	if cover.UUID == "" {
		created, err := productionService.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{
			Title: draft.Title, DescriptionMD: summarizeText(draft.Storyboard, 600),
			PageRole: production.PageRoleFrontCover,
		})
		if err == nil {
			cover = created
		} else {
			sections, reloadErr := productionService.ListSections(ctx, chapter.UUID)
			if reloadErr != nil {
				return production.ComicSection{}, err
			}
			_, cover = yoloSectionsByRole(sections)
			if cover.UUID == "" {
				return production.ComicSection{}, err
			}
			if cover.CurrentStoryboard != nil || cover.CurrentImage != nil {
				return cover, nil
			}
		}
	}
	updated, err := productionService.CreateStoryboard(ctx, chapter.UUID, cover.UUID, draft.Storyboard, "generated", cover.Revision)
	if err == nil {
		return updated, nil
	}
	reloaded, reloadErr := productionService.GetSection(ctx, chapter.UUID, cover.UUID)
	if reloadErr == nil && (reloaded.CurrentStoryboard != nil || reloaded.CurrentImage != nil) {
		return reloaded, nil
	}
	return production.ComicSection{}, err
}

func yoloCoverDraftCheckpoint(raw string) (yoloCoverDraft, bool) {
	var output struct {
		Draft yoloCoverDraft `json:"cover_storyboard_draft"`
	}
	if json.Unmarshal([]byte(raw), &output) != nil {
		return yoloCoverDraft{}, false
	}
	output.Draft.Title = strings.TrimSpace(output.Draft.Title)
	output.Draft.Storyboard = strings.TrimSpace(output.Draft.Storyboard)
	return output.Draft, output.Draft.Title != "" && output.Draft.Storyboard != ""
}

func yoloPageMomentPlan(profile project.PictureBookProfile) (string, string) {
	if profile.Format == project.PictureBookVertical {
		return "Section 1: 1; Section 2: 3; Section 3: 2; Section 4: 1; Section 5: 2; Section 6: 3", "3"
	}
	values := []int{1, 1, 1, 1, 1, 1}
	maxMoments := "1"
	if profile.Format == project.PictureBookComicStory && profile.ComicLayout != nil {
		if *profile.ComicLayout == project.ComicLayoutFourPanel {
			values = []int{4, 4, 4, 4, 4, 4}
			maxMoments = "4"
		} else {
			values = []int{4, 5, 3, 6, 4, 5}
			maxMoments = "6"
		}
	}
	encoded, _ := json.Marshal(values)
	return string(encoded), maxMoments
}

func (service *Service) runYoloFirstImage(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, snapshot yoloSnapshot) (map[string]any, bool, error) {
	if snapshot.Version >= 5 {
		return service.runYoloInitialPageImages(ctx, store, workflow, step, snapshot)
	}
	return service.runYoloLegacyFirstImage(ctx, store, workflow, step, snapshot)
}

func (service *Service) runYoloLegacyFirstImage(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, snapshot yoloSnapshot) (map[string]any, bool, error) {
	chapterUUID, err := service.workflowOutputUUID(ctx, store, workflow.ID, "comic_sections", "chapter_uuid")
	if err != nil {
		return nil, false, err
	}
	sectionUUID, err := service.workflowOutputUUID(ctx, store, workflow.ID, "comic_sections", "first_section_uuid")
	if err != nil {
		return nil, false, err
	}
	productionService := production.NewService(store, service.hub)
	section, err := productionService.GetSection(ctx, chapterUUID, sectionUUID)
	if err != nil {
		return nil, false, err
	}
	if section.CurrentImage != nil {
		return map[string]any{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID, "image_variant_uuid": section.CurrentImage.UUID}, false, nil
	}
	task, err := service.ensureWorkflowDomainTask(ctx, store, step, DomainTaskRequest{Kind: "comic_image_generation", ResourceUUID: sectionUUID, ChapterUUID: chapterUUID, ProviderUUID: snapshot.ImageProviderUUID, Model: snapshot.ImageModel, SelectionProviderUUID: snapshot.SelectionProviderUUID, SelectionModel: snapshot.SelectionModel, Prompt: section.CurrentStoryboard.ContentMD, IdempotencyKey: step.IdempotencyKey + ":image"})
	if err != nil {
		return nil, false, err
	}
	if task.Status == "queued" || task.Status == "running" {
		return nil, true, nil
	}
	if task.Status != "completed" {
		return nil, false, domainError(task.ErrorCode, "首图生成失败", task.ErrorMessage, nil)
	}
	section, err = productionService.GetSection(ctx, chapterUUID, sectionUUID)
	if err != nil || section.CurrentImage == nil {
		return nil, false, domainError(CodeStateConflict, "首图结果缺失", "图片任务完成后没有 current image。", err)
	}
	return map[string]any{"chapter_uuid": chapterUUID, "section_uuid": sectionUUID, "image_variant_uuid": section.CurrentImage.UUID, "task_uuid": task.UUID}, false, nil
}

func (service *Service) runYoloInitialPageImages(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, snapshot yoloSnapshot) (map[string]any, bool, error) {
	if snapshot.PictureBook == nil {
		return nil, false, domainError(CodeStateConflict, "Yolo 绘本快照缺失", "v5 workflow 无法判断初始页面范围。", nil)
	}
	chapterUUID, err := service.workflowOutputUUID(ctx, store, workflow.ID, "comic_sections", "chapter_uuid")
	if err != nil {
		return nil, false, err
	}
	bodyUUID, err := service.workflowOutputUUID(ctx, store, workflow.ID, "comic_sections", "body_section_uuid")
	if err != nil {
		return nil, false, err
	}
	productionService := production.NewService(store, service.hub)
	body, err := productionService.GetSection(ctx, chapterUUID, bodyUUID)
	if err != nil {
		return nil, false, err
	}
	if body.PageRole != production.PageRoleBody {
		return nil, false, domainError(CodeStateConflict, "Yolo 正文页角色冲突", "body_section_uuid 没有引用 body 页面。", nil)
	}
	targets := []production.ComicSection{body}
	var cover production.ComicSection
	if snapshot.PictureBook.Format != project.PictureBookVertical {
		coverUUID, outputErr := service.workflowOutputUUID(ctx, store, workflow.ID, "comic_sections", "cover_section_uuid")
		if outputErr != nil {
			return nil, false, outputErr
		}
		cover, err = productionService.GetSection(ctx, chapterUUID, coverUUID)
		if err != nil {
			return nil, false, err
		}
		if cover.PageRole != production.PageRoleFrontCover {
			return nil, false, domainError(CodeStateConflict, "Yolo 封面角色冲突", "cover_section_uuid 没有引用 front_cover 页面。", nil)
		}
		targets = []production.ComicSection{cover, body}
	}
	missing := make([]string, 0, len(targets))
	for _, section := range targets {
		if section.CurrentImage != nil {
			continue
		}
		if section.CurrentStoryboard == nil {
			return nil, false, domainError(CodeStateConflict, "页面 Storyboard 缺失", "Yolo 初始页面必须有 current storyboard 才能生成图片。", nil)
		}
		missing = append(missing, section.UUID)
	}
	if len(missing) > 0 {
		batch, batchErr := service.ensureWorkflowDomainTaskBatch(ctx, store, step, DomainTaskBatchRequest{
			Kind: "comic_image_generation", ResourceUUIDs: missing, ChapterUUID: chapterUUID,
			ProviderUUID: snapshot.ImageProviderUUID, Model: snapshot.ImageModel,
			SelectionProviderUUID: snapshot.SelectionProviderUUID, SelectionModel: snapshot.SelectionModel,
			IdempotencyKey: step.IdempotencyKey + ":images",
		}, body.UUID)
		if batchErr != nil {
			return nil, false, batchErr
		}
		active, failed := yoloInitialImageBatchState(batch.Tasks)
		if active {
			return nil, true, nil
		}
		if failed != nil {
			code := strings.TrimSpace(failed.ErrorCode)
			if code == "" {
				code = CodeStateConflict
			}
			return nil, false, domainError(code, "初始页面图片生成失败", failed.ErrorMessage, nil)
		}
	}
	for index := range targets {
		targets[index], err = productionService.GetSection(ctx, chapterUUID, targets[index].UUID)
		if err != nil || targets[index].CurrentImage == nil {
			return nil, false, domainError(CodeStateConflict, "初始页面图片结果缺失", "图片任务完成后页面没有 current image。", err)
		}
	}
	if len(targets) == 2 {
		cover, body = targets[0], targets[1]
	} else {
		body = targets[0]
	}
	output := map[string]any{
		"chapter_uuid": chapterUUID,
		"section_uuid": body.UUID, "image_variant_uuid": body.CurrentImage.UUID,
		"body_section_uuid": body.UUID, "body_image_variant_uuid": body.CurrentImage.UUID,
	}
	sectionUUIDs, imageUUIDs := make([]string, 0, len(targets)), make([]string, 0, len(targets))
	for _, section := range targets {
		sectionUUIDs = append(sectionUUIDs, section.UUID)
		imageUUIDs = append(imageUUIDs, section.CurrentImage.UUID)
	}
	output["section_uuids"], output["image_variant_uuids"] = sectionUUIDs, imageUUIDs
	checkpoint, _ := service.workflowStepOutput(ctx, store, step.ID)
	if taskUUIDs := workflowTaskUUIDs(checkpoint, ""); len(taskUUIDs) > 0 {
		output["task_uuids"] = taskUUIDs
	}
	if bySection := workflowTaskUUIDBySection(checkpoint); isUUIDv7(bySection[body.UUID]) {
		output["task_uuid"] = bySection[body.UUID]
	}
	if cover.UUID != "" {
		output["cover_section_uuid"] = cover.UUID
		output["cover_image_variant_uuid"] = cover.CurrentImage.UUID
	}
	return output, false, nil
}

func yoloInitialImageBatchState(tasks []DomainTask) (bool, *DomainTask) {
	active := false
	var failed *DomainTask
	for _, task := range tasks {
		switch task.Status {
		case "queued", "running", "waiting_for_input":
			active = true
		case "completed":
			// Wait for every task to become terminal before evaluating a
			// sibling failure, so result handling is independent of batch order.
		default:
			if failed == nil {
				copy := task
				failed = &copy
			}
		}
	}
	return active, failed
}

func (service *Service) workflowStepOutput(ctx context.Context, store *project.Store, stepID int64) (string, error) {
	var raw string
	if err := store.DB().WithContext(ctx).Model(&workflowStepRecord{}).Select("output_json").Where("id=?", stepID).Scan(&raw).Error; err != nil {
		return "", err
	}
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	return raw, nil
}

func (service *Service) checkpointWorkflowStepOutput(ctx context.Context, store *project.Store, stepID int64, values map[string]any) error {
	raw, err := service.workflowStepOutput(ctx, store, stepID)
	if err != nil {
		return err
	}
	var output map[string]any
	if json.Unmarshal([]byte(raw), &output) != nil || output == nil {
		output = map[string]any{}
	}
	for key, value := range values {
		output[key] = value
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	result := store.DB().WithContext(ctx).Model(&workflowStepRecord{}).Where("id=? AND status IN ('queued','running','waiting')", stepID).Updates(map[string]any{"output_json": string(encoded), "updated_at": service.now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domainError(CodeCancelled, "Yolo 步骤已取消", "workflow step 已不再 active，未保存迟到的中间结果。", errWorkflowTransitionInactive)
	}
	return nil
}

func (service *Service) ensureWorkflowDomainTaskBatch(ctx context.Context, store *project.Store, step workflowStepRecord, request DomainTaskBatchRequest, primaryResourceUUID string) (DomainTaskBatch, error) {
	if request.Invocation.Source == "" {
		var threadUUID string
		if err := store.DB().WithContext(ctx).Raw(`SELECT COALESCE(t.uuid,'') FROM workflows w LEFT JOIN chat_threads t ON t.id=w.thread_id WHERE w.id=?`, step.WorkflowID).Scan(&threadUUID).Error; err != nil {
			return DomainTaskBatch{}, err
		}
		request.Invocation = WorkflowStepInvocationContext(threadUUID)
	}
	batch, batchErr := service.queue.StartDomainTaskBatch(ctx, store.ProjectUUID(), request)
	if len(batch.Tasks) == 0 {
		return batch, batchErr
	}
	checkpointCtx := context.WithoutCancel(ctx)
	raw, err := service.workflowStepOutput(checkpointCtx, store, step.ID)
	if err != nil {
		service.cancelActiveDomainTasks(checkpointCtx, store.ProjectUUID(), request.Kind, batch.Tasks)
		return DomainTaskBatch{}, err
	}
	var output map[string]any
	if json.Unmarshal([]byte(raw), &output) != nil || output == nil {
		output = map[string]any{}
	}
	taskUUIDs := workflowTaskUUIDs(raw, "")
	seen := make(map[string]struct{}, len(taskUUIDs)+len(batch.Tasks))
	for _, taskUUID := range taskUUIDs {
		seen[taskUUID] = struct{}{}
	}
	bySection := workflowTaskUUIDBySection(raw)
	for _, task := range batch.Tasks {
		if !isUUIDv7(task.UUID) || !isUUIDv7(task.ResourceUUID) {
			service.cancelActiveDomainTasks(checkpointCtx, store.ProjectUUID(), request.Kind, batch.Tasks)
			return DomainTaskBatch{}, domainError(CodeStateConflict, "批量图片任务输出无效", "Domain task batch 必须返回公开 UUIDv7。", nil)
		}
		if _, exists := seen[task.UUID]; !exists {
			taskUUIDs = append(taskUUIDs, task.UUID)
			seen[task.UUID] = struct{}{}
		}
		bySection[task.ResourceUUID] = task.UUID
	}
	output["task_uuids"] = taskUUIDs
	output["task_uuid_by_section"] = bySection
	if primaryTaskUUID := bySection[primaryResourceUUID]; isUUIDv7(primaryTaskUUID) {
		output["task_uuid"] = primaryTaskUUID
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		service.cancelActiveDomainTasks(checkpointCtx, store.ProjectUUID(), request.Kind, batch.Tasks)
		return DomainTaskBatch{}, err
	}
	currentTask, currentResource := "", ""
	for _, task := range batch.Tasks {
		if task.Status == "queued" || task.Status == "running" || task.Status == "waiting_for_input" {
			currentTask, currentResource = task.UUID, task.ResourceUUID
			if task.ResourceUUID == primaryResourceUUID {
				break
			}
		}
	}
	if currentTask == "" {
		if primaryTaskUUID := bySection[primaryResourceUUID]; isUUIDv7(primaryTaskUUID) {
			currentTask, currentResource = primaryTaskUUID, primaryResourceUUID
		} else {
			last := batch.Tasks[len(batch.Tasks)-1]
			currentTask, currentResource = last.UUID, last.ResourceUUID
		}
	}
	result := store.DB().WithContext(checkpointCtx).Model(&workflowStepRecord{}).Where("id=? AND status IN ('queued','running','waiting')", step.ID).Updates(map[string]any{
		"task_uuid": currentTask, "resource_uuid": currentResource,
		"output_json": string(encoded), "updated_at": service.now().UTC(),
	})
	if result.Error != nil || result.RowsAffected != 1 {
		service.cancelActiveDomainTasks(checkpointCtx, store.ProjectUUID(), request.Kind, batch.Tasks)
		if result.Error != nil {
			return DomainTaskBatch{}, result.Error
		}
		return DomainTaskBatch{}, domainError(CodeCancelled, "Yolo 步骤已取消", "批量任务创建后 workflow step 已不再 active，已撤销仍在运行的任务。", nil)
	}
	if batchErr != nil {
		service.cancelActiveDomainTasks(checkpointCtx, store.ProjectUUID(), request.Kind, batch.Tasks)
	}
	return batch, batchErr
}

func (service *Service) cancelActiveDomainTasks(ctx context.Context, projectUUID, kind string, tasks []DomainTask) {
	for _, task := range tasks {
		if task.Status != "queued" && task.Status != "running" && task.Status != "waiting_for_input" {
			continue
		}
		_ = service.queue.CancelDomainTask(ctx, projectUUID, kind, task.UUID)
	}
}

func workflowTaskUUIDs(raw, fallback string) []string {
	var output struct {
		TaskUUID  string   `json:"task_uuid"`
		TaskUUIDs []string `json:"task_uuids"`
	}
	_ = json.Unmarshal([]byte(raw), &output)
	values := append([]string(nil), output.TaskUUIDs...)
	values = append(values, output.TaskUUID, fallback)
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !isUUIDv7(value) {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func appendUniqueTaskUUID(values []string, candidate string) []string {
	if !isUUIDv7(candidate) {
		return values
	}
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func yoloInitialImageSectionUUIDs(snapshot yoloSnapshot, comicOutput string) []string {
	var output struct {
		CoverSectionUUID string `json:"cover_section_uuid"`
		BodySectionUUID  string `json:"body_section_uuid"`
		FirstSectionUUID string `json:"first_section_uuid"`
	}
	_ = json.Unmarshal([]byte(comicOutput), &output)
	if !isUUIDv7(output.BodySectionUUID) {
		output.BodySectionUUID = output.FirstSectionUUID
	}
	result := make([]string, 0, 2)
	if snapshot.PictureBook != nil && snapshot.PictureBook.Format != project.PictureBookVertical && isUUIDv7(output.CoverSectionUUID) {
		result = append(result, output.CoverSectionUUID)
	}
	if isUUIDv7(output.BodySectionUUID) {
		result = append(result, output.BodySectionUUID)
	}
	return result
}

func workflowTaskUUIDBySection(raw string) map[string]string {
	var output struct {
		BySection map[string]string `json:"task_uuid_by_section"`
	}
	_ = json.Unmarshal([]byte(raw), &output)
	result := make(map[string]string, len(output.BySection))
	for sectionUUID, taskUUID := range output.BySection {
		if isUUIDv7(sectionUUID) && isUUIDv7(taskUUID) {
			result[sectionUUID] = taskUUID
		}
	}
	return result
}

func (service *Service) ensureWorkflowDomainTask(ctx context.Context, store *project.Store, step workflowStepRecord, request DomainTaskRequest) (DomainTask, error) {
	if request.Invocation.Source == "" {
		var threadUUID string
		if err := store.DB().WithContext(ctx).Raw(`SELECT COALESCE(t.uuid,'') FROM workflows w LEFT JOIN chat_threads t ON t.id=w.thread_id WHERE w.id=?`, step.WorkflowID).Scan(&threadUUID).Error; err != nil {
			return DomainTask{}, err
		}
		request.Invocation = WorkflowStepInvocationContext(threadUUID)
	}
	checkpointCtx := context.WithoutCancel(ctx)
	raw, loadErr := service.workflowStepOutput(ctx, store, step.ID)
	if loadErr != nil {
		return DomainTask{}, loadErr
	}
	var output map[string]any
	_ = json.Unmarshal([]byte(raw), &output)
	taskUUID, _ := output[request.IdempotencyKey].(string)
	if taskUUID != "" && isUUIDv7(taskUUID) {
		return service.queue.GetDomainTask(ctx, store.ProjectUUID(), request.Kind, taskUUID)
	}
	task, err := service.queue.StartDomainTask(ctx, store.ProjectUUID(), request)
	if err != nil {
		return DomainTask{}, err
	}
	if !isUUIDv7(task.UUID) {
		service.cancelActiveDomainTasks(checkpointCtx, store.ProjectUUID(), request.Kind, []DomainTask{task})
		return DomainTask{}, domainError(CodeStateConflict, "Domain task 输出无效", "任务必须返回公开 UUIDv7。", nil)
	}
	// Reload before merging so a duplicate worker cannot erase an earlier
	// checkpoint written after this invocation loaded its stale step record.
	raw, loadErr = service.workflowStepOutput(checkpointCtx, store, step.ID)
	if loadErr != nil {
		service.cancelActiveDomainTasks(checkpointCtx, store.ProjectUUID(), request.Kind, []DomainTask{task})
		return DomainTask{}, loadErr
	}
	_ = json.Unmarshal([]byte(raw), &output)
	if output == nil {
		output = map[string]any{}
	}
	output[request.IdempotencyKey] = task.UUID
	encoded, _ := json.Marshal(output)
	now := service.now().UTC()
	result := store.DB().WithContext(checkpointCtx).Model(&workflowStepRecord{}).Where("id=? AND status IN ('queued','running','waiting')", step.ID).Updates(map[string]any{"task_uuid": task.UUID, "resource_uuid": request.ResourceUUID, "output_json": string(encoded), "updated_at": now})
	if result.Error != nil || result.RowsAffected != 1 {
		service.cancelActiveDomainTasks(checkpointCtx, store.ProjectUUID(), request.Kind, []DomainTask{task})
		if result.Error != nil {
			return DomainTask{}, result.Error
		}
		return DomainTask{}, domainError(CodeCancelled, "Yolo 步骤已取消", "任务创建后 workflow step 已不再 active，已撤销仍在运行的任务。", nil)
	}
	return task, nil
}

func (service *Service) workflowOutputUUID(ctx context.Context, store *project.Store, workflowID int64, stepKey, key string) (string, error) {
	var raw string
	if err := store.DB().WithContext(ctx).Raw(`SELECT output_json FROM workflow_steps WHERE workflow_id=? AND step_key=? AND status='completed'`, workflowID, stepKey).Scan(&raw).Error; err != nil || raw == "" {
		return "", domainError(CodeWorkflowNotReady, "Workflow 前置输出缺失", stepKey+" 尚未完成。", err)
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return "", err
	}
	value, _ := output[key].(string)
	if !isUUIDv7(value) {
		return "", domainError(CodeStateConflict, "Workflow UUID 输出无效", key+" 不是 UUIDv7。", nil)
	}
	return value, nil
}

func (service *Service) yoloPrompt(ctx context.Context, store *project.Store, snapshot yoloSnapshot, group, key string) (string, error) {
	if value := strings.TrimSpace(snapshot.Prompts[group+"/"+key]); value != "" {
		return value, nil
	}
	return story.NewService(store).EffectivePrompt(ctx, group, key)
}

func (service *Service) yoloSystemPrompt(ctx context.Context, store *project.Store, snapshot yoloSnapshot, group string) string {
	value, err := service.yoloPrompt(ctx, store, snapshot, group, "json_system")
	if err != nil {
		definition, _ := promptcatalog.Lookup(group, "json_system", snapshot.GenerationLanguage)
		value = definition.DefaultValue
	}
	languageInstruction, languageErr := service.yoloPrompt(ctx, store, snapshot, promptcatalog.GroupRuntime, "project_language_instruction")
	if languageErr != nil {
		languageInstruction = promptcatalog.LanguageInstruction(snapshot.GenerationLanguage)
	}
	return promptcatalog.WithInstruction(value, languageInstruction)
}

func (service *Service) completeYoloJSON(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, snapshot yoloSnapshot, scenario, systemPrompt, userPrompt string, target any) error {
	resolved, err := service.resolveProvider(ctx, snapshot.ProviderUUID)
	if err != nil {
		return err
	}
	var previous int64
	if err := store.DB().WithContext(ctx).Table("llm_logs").Where("workflow_step_id=? AND scenario=?", step.ID, scenario).Count(&previous).Error; err != nil {
		return err
	}
	request := llm.ChatRequest{BaseURL: resolved.BaseURL, APIKey: resolved.APIKey, Model: snapshot.Model, Messages: []llm.ChatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}}, MaxTokens: 16000}
	requestPayload, err := llmlog.EncodeChatRequest(request)
	if err != nil {
		return err
	}
	logHandle, err := llmlog.Begin(ctx, store, service.hub, llmlog.StartInput{
		ProjectID: workflow.ProjectID, WorkflowID: workflow.ID, WorkflowStepID: step.ID,
		SourceType: llmlog.SourceWorkflow, Scenario: scenario, RequestType: llmlog.RequestText, Attempt: int(previous) + 1,
		ProviderUUID: snapshot.ProviderUUID, ProviderType: resolved.ProviderType, Model: snapshot.Model, InputSummary: userPrompt,
		RequestPayload: requestPayload,
	})
	if err != nil {
		return err
	}
	response, err := service.model.Complete(ctx, request)
	var responsePayload []byte
	if err == nil {
		responsePayload, err = llmlog.EncodeChatResponse(response, request.APIKey)
	}
	finishErr := llmlog.Finish(context.WithoutCancel(ctx), store, service.hub, logHandle, llmlog.FinishInput{
		OutputSummary: response.Message.Content, InputTokens: response.Usage.InputTokens, CachedInputTokens: response.Usage.CachedInputTokens, OutputTokens: response.Usage.OutputTokens,
		FinishReason: response.FinishReason, Response: responsePayload, Err: err,
	})
	if finishErr != nil {
		if err != nil {
			err = errors.Join(err, finishErr)
		} else {
			return finishErr
		}
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(response.Message.Content)), target); err != nil {
		return domainError(llm.CodeInvalidContent, "模型 JSON 无效", "生成步骤没有返回规范 JSON object。", err)
	}
	return nil
}

func summarizeText(value string, max int) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= max {
		return value
	}
	return string([]rune(value)[:max]) + "…"
}

func splitIntoSections(value string, count int) []string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) == 0 {
		runes = []rune("故事继续向前发展。")
	}
	result := make([]string, count)
	for index := 0; index < count; index++ {
		start := index * len(runes) / count
		end := (index + 1) * len(runes) / count
		if start == end {
			result[index] = string(runes)
		} else {
			result[index] = string(runes[start:end])
		}
	}
	return result
}

func (service *Service) completeWorkflowStep(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, threadUUID string, output map[string]any) error {
	sqlDB, err := store.DB().DB()
	if err != nil {
		return err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := service.now().UTC()
	encoded, _ := json.Marshal(output)
	result, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='completed',output_json=?,completed_at=?,updated_at=?,error_code='',error_message='' WHERE id=? AND status='running'`, string(encoded), now, now, step.ID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		// Cancellation or another worker won the terminal transition. Roll the
		// transaction back and do not queue a downstream step.
		return nil
	}
	if err := appendWorkflowEventTx(ctx, tx, workflow.ID, &step.ID, "step_completed", map[string]any{"project_uuid": store.ProjectUUID(), "workflow_uuid": workflow.UUID, "step_uuid": step.UUID, "step_key": step.StepKey, "status": "completed", "resource_uuid": publicOutputResource(output)}, now); err != nil {
		return err
	}
	var next workflowStepRecord
	err = tx.QueryRowContext(ctx, `SELECT id,uuid,workflow_id,step_key,position,status,idempotency_key,river_job_id,task_uuid,resource_uuid,input_json,output_json,error_code,error_message,started_at,completed_at,created_at,updated_at FROM workflow_steps WHERE workflow_id=? AND position>? ORDER BY position LIMIT 1`, workflow.ID, step.Position).Scan(&next.ID, &next.UUID, &next.WorkflowID, &next.StepKey, &next.Position, &next.Status, &next.IdempotencyKey, &next.RiverJobID, &next.TaskUUID, &next.ResourceUUID, &next.InputJSON, &next.OutputJSON, &next.ErrorCode, &next.ErrorMessage, &next.StartedAt, &next.CompletedAt, &next.CreatedAt, &next.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		result, err := tx.ExecContext(ctx, `UPDATE workflows SET status='completed',current_step_key='',completed_at=?,updated_at=?,error_code='',error_message='' WHERE id=? AND status='running'`, now, now, workflow.ID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return domainError(CodeStateConflict, "Yolo 完成状态冲突", "workflow 已不再 running，未提交步骤结果。", nil)
		}
		if workflow.ThreadID != nil {
			completionMessage, err := yoloCompletionMessage(output)
			if err != nil {
				return err
			}
			var thread threadRecord
			if err := tx.QueryRowContext(ctx, `SELECT id,uuid,project_id,title,status,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,archived_at,created_at,updated_at FROM chat_threads WHERE id=?`, *workflow.ThreadID).Scan(&thread.ID, &thread.UUID, &thread.ProjectID, &thread.Title, &thread.Status, &thread.ProviderUUID, &thread.Model, &thread.ModelSource, &thread.NextTurnSequence, &thread.NextItemSequence, &thread.NextEventSequence, &thread.ArchivedAt, &thread.CreatedAt, &thread.UpdatedAt); err != nil {
				return err
			}
			if _, err := appendItemTx(ctx, tx, &thread, nil, nil, "assistant_message", "assistant", completionMessage, "text", "completed", "", "", workflow.UUID, map[string]any{"workflow_uuid": workflow.UUID}, now); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, now, thread.ID); err != nil {
				return err
			}
			if _, err := RecomputeThreadStatusTx(ctx, tx, thread.ID, now); err != nil {
				return err
			}
		}
		if err := appendWorkflowEventTx(ctx, tx, workflow.ID, nil, "workflow_completed", map[string]any{"project_uuid": store.ProjectUUID(), "workflow_uuid": workflow.UUID, "thread_uuid": threadUUID, "status": WorkflowCompleted}, now); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		result, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='queued',updated_at=? WHERE id=? AND status='pending'`, now, next.ID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return domainError(CodeStateConflict, "Yolo 下一步骤状态冲突", "下一个 workflow step 已不再 pending，未重复入队。", nil)
		}
		jobID, err := service.queue.EnqueueAgentTx(ctx, store.ProjectUUID(), tx, JobSpec{Version: 1, ProjectUUID: store.ProjectUUID(), JobKind: JobWorkflowStep, ResourceUUID: next.UUID, ThreadUUID: threadUUID})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET river_job_id=? WHERE id=?`, jobID, next.ID); err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx, `UPDATE workflows SET current_step_key=?,updated_at=? WHERE id=? AND status='running'`, next.StepKey, now, workflow.ID)
		if err != nil {
			return err
		}
		rows, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return domainError(CodeStateConflict, "Yolo Workflow 状态冲突", "workflow 已不再 running，未推进下一步骤。", nil)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	status := WorkflowRunning
	if next.ID == 0 {
		status = WorkflowCompleted
	}
	service.broadcastWorkflow(store.ProjectUUID(), Workflow{UUID: workflow.UUID, ThreadUUID: threadUUID, Status: status}, "workflow:step_changed", step.UUID)
	return nil
}

func yoloCompletionMessage(output map[string]any) (string, error) {
	chapterUUID, _ := output["chapter_uuid"].(string)
	bodyUUID, _ := output["body_section_uuid"].(string)
	if bodyUUID == "" {
		bodyUUID, _ = output["section_uuid"].(string)
	}
	if !isCanonicalUUIDv7(chapterUUID) || !isCanonicalUUIDv7(bodyUUID) {
		return "", domainError(CodeStateConflict, "Yolo 完成资源无效", "最终步骤没有返回规范的 Chapter 与 Section UUIDv7。", nil)
	}
	if coverUUID, _ := output["cover_section_uuid"].(string); coverUUID != "" {
		if !isCanonicalUUIDv7(coverUUID) {
			return "", domainError(CodeStateConflict, "Yolo 封面资源无效", "最终步骤没有返回规范的封面 Section UUIDv7。", nil)
		}
		return fmt.Sprintf(
			"Yolo 快速创作已完成：[第一章正文](@project/chapters/%s/body)、[Story Profile](@project/story-profile)、[Premise](@project/premise)、[页面脚本](@project/chapters/%s)、[封面](@project/chapters/%s/sections/%s)和[正文第一页](@project/chapters/%s/sections/%s)均已就绪。",
			chapterUUID, chapterUUID, chapterUUID, coverUUID, chapterUUID, bodyUUID,
		), nil
	}
	if bodyImageUUID, _ := output["body_image_variant_uuid"].(string); bodyImageUUID != "" {
		if !isCanonicalUUIDv7(bodyImageUUID) {
			return "", domainError(CodeStateConflict, "Yolo 正文页图片无效", "最终步骤没有返回规范的正文 Image Variant UUIDv7。", nil)
		}
		return fmt.Sprintf(
			"Yolo 快速创作已完成：[第一章正文](@project/chapters/%s/body)、[Story Profile](@project/story-profile)、[Premise](@project/premise)、[画面段落](@project/chapters/%s)和[首个画面段落](@project/chapters/%s/sections/%s)均已就绪。",
			chapterUUID, chapterUUID, chapterUUID, bodyUUID,
		), nil
	}
	return fmt.Sprintf(
		"Yolo 快速创作已完成：[第一章正文](@project/chapters/%s/body)、[Story Profile](@project/story-profile)、[Premise](@project/premise)、[Comic Sections](@project/chapters/%s)和[首图](@project/chapters/%s/sections/%s)均已就绪。",
		chapterUUID,
		chapterUUID,
		chapterUUID,
		bodyUUID,
	), nil
}

func publicOutputResource(output map[string]any) string {
	for _, key := range []string{"image_variant_uuid", "first_section_uuid", "setting_image_uuid", "story_profile_uuid", "chapter_uuid", "project_uuid"} {
		if value, ok := output[key].(string); ok && isUUIDv7(value) {
			return value
		}
	}
	return ""
}

func (service *Service) failWorkflowStep(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, threadUUID string, cause error) error {
	code, message := errorCode(cause), safeMessage(cause)
	if code == CodeProvider {
		message = "Yolo 步骤执行失败。"
	}
	now := service.now().UTC()
	transitioned := false
	err := store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&workflowStepRecord{}).Where("id=? AND status IN ('queued','running','waiting','interrupted')", step.ID).Updates(map[string]any{"status": "failed", "error_code": code, "error_message": message, "completed_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errWorkflowTransitionInactive
		}
		result = tx.Model(&workflowRecord{}).Where("id=? AND status IN ('queued','running','interrupted')", workflow.ID).Updates(map[string]any{"status": WorkflowFailed, "error_code": code, "error_message": message, "completed_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errWorkflowTransitionInactive
		}
		if workflow.ThreadID != nil {
			if _, err := recomputeThreadStatusGormTx(ctx, tx, *workflow.ThreadID, now); err != nil {
				return err
			}
		}
		if err := appendWorkflowEventGormTx(ctx, tx, workflow.ID, &step.ID, "step_failed", map[string]any{"project_uuid": store.ProjectUUID(), "workflow_uuid": workflow.UUID, "step_uuid": step.UUID, "step_key": step.StepKey, "status": WorkflowFailed, "error_code": code}, now); err != nil {
			return err
		}
		transitioned = true
		return nil
	})
	if errors.Is(err, errWorkflowTransitionInactive) {
		return nil
	}
	if err == nil && transitioned {
		service.broadcastWorkflow(store.ProjectUUID(), Workflow{UUID: workflow.UUID, ThreadUUID: threadUUID, Status: WorkflowFailed, ErrorCode: code, ErrorMessage: message}, "workflow:failed", step.UUID)
	}
	return err
}

func (service *Service) CancelWorkflow(ctx context.Context, projectUUID, workflowUUID string) (Workflow, error) {
	workflow, err := service.GetWorkflow(ctx, projectUUID, workflowUUID)
	if err != nil {
		return Workflow{}, err
	}
	if workflow.Status == WorkflowCompleted || workflow.Status == WorkflowCancelled {
		return workflow, nil
	}
	if workflow.Kind == WorkflowComicSectionImage {
		taskUUID := comicWorkflowTaskUUID(workflow)
		if taskUUID == "" {
			return Workflow{}, domainError(CodeStateConflict, "图片生成 Workflow 缺少生产任务", "generate_section_image 步骤没有关联 task_uuid。", nil)
		}
		if err := service.queue.CancelDomainTask(ctx, projectUUID, "comic_image_generation", taskUUID); err != nil {
			return Workflow{}, err
		}
		result, getErr := service.GetWorkflow(ctx, projectUUID, workflowUUID)
		if getErr == nil {
			service.broadcastWorkflow(projectUUID, result, "workflow:cancelled", "")
		}
		return result, getErr
	}
	if taskKind, projected := projectedStoryWorkflowTaskKind(workflow.Kind); projected {
		taskUUID := projectedStoryWorkflowTaskUUID(workflow)
		if taskUUID == "" {
			return Workflow{}, domainError(CodeStateConflict, "Story Workflow 缺少任务", "Workflow 步骤没有关联 task_uuid。", nil)
		}
		if err := service.queue.CancelDomainTask(ctx, projectUUID, taskKind, taskUUID); err != nil {
			return Workflow{}, err
		}
		result, getErr := service.GetWorkflow(ctx, projectUUID, workflowUUID)
		if getErr == nil {
			service.broadcastWorkflow(projectUUID, result, "workflow:cancelled", "")
		}
		return result, getErr
	}
	var riverJobID int64
	var currentTaskUUIDs []string
	var currentTaskKind, workUUID string
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		now := service.now().UTC()
		return store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var row workflowRecord
			if err := tx.Where("uuid=?", workflowUUID).First(&row).Error; err != nil {
				return err
			}
			result := tx.Model(&workflowRecord{}).Where("id=? AND status NOT IN ('completed','cancelled')", row.ID).Updates(map[string]any{"status": WorkflowCancelled, "cancel_requested_at": now, "completed_at": now, "updated_at": now, "error_code": CodeCancelled, "error_message": "用户已取消。"})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errWorkflowTransitionInactive
			}
			var step workflowStepRecord
			if row.CurrentStepKey != "" {
				_ = tx.Where("workflow_id=? AND step_key=? AND status<>'completed'", row.ID, row.CurrentStepKey).First(&step).Error
			}
			if step.ID == 0 {
				_ = tx.Where("workflow_id=? AND status<>'completed'", row.ID).Order("position DESC").First(&step).Error
			}
			if step.ID != 0 {
				riverJobID, workUUID = valueInt64(step.RiverJobID), step.UUID
				currentTaskUUIDs = workflowTaskUUIDs(step.OutputJSON, step.TaskUUID)
				currentTaskKind = yoloStepTaskKind(step.StepKey)
				if row.Kind == WorkflowYolo && step.StepKey == "first_section_image" {
					var snapshot yoloSnapshot
					if json.Unmarshal([]byte(row.InputSnapshot), &snapshot) == nil && snapshot.Version >= 5 {
						var comicStep workflowStepRecord
						if err := tx.Where("workflow_id=? AND step_key='comic_sections' AND status='completed'", row.ID).First(&comicStep).Error; err == nil {
							sectionUUIDs := yoloInitialImageSectionUUIDs(snapshot, comicStep.OutputJSON)
							keys := make([]string, 0, len(sectionUUIDs))
							for _, sectionUUID := range sectionUUIDs {
								keys = append(keys, ComicImageBatchTaskKey(step.IdempotencyKey+":images", sectionUUID))
							}
							if len(keys) > 0 {
								var discovered []struct {
									UUID         string
									ResourceUUID string `gorm:"column:resource_uuid"`
									Status       string
								}
								if err := tx.Table("production_task_runs").Select("uuid,resource_uuid,status").Where("project_id=? AND kind='comic_image_generation' AND idempotency_key IN ?", row.ProjectID, keys).Scan(&discovered).Error; err != nil {
									return err
								}
								taskUUIDs := workflowTaskUUIDs(step.OutputJSON, step.TaskUUID)
								bySection := workflowTaskUUIDBySection(step.OutputJSON)
								for _, task := range discovered {
									taskUUIDs = appendUniqueTaskUUID(taskUUIDs, task.UUID)
									if isUUIDv7(task.ResourceUUID) && isUUIDv7(task.UUID) {
										bySection[task.ResourceUUID] = task.UUID
									}
									if task.Status != "completed" && task.Status != "cancelled" {
										currentTaskUUIDs = appendUniqueTaskUUID(currentTaskUUIDs, task.UUID)
									}
								}
								if len(discovered) > 0 {
									var output map[string]any
									if json.Unmarshal([]byte(step.OutputJSON), &output) != nil || output == nil {
										output = map[string]any{}
									}
									output["task_uuids"] = taskUUIDs
									output["task_uuid_by_section"] = bySection
									primaryResourceUUID := sectionUUIDs[len(sectionUUIDs)-1]
									primaryTaskUUID := bySection[primaryResourceUUID]
									if isUUIDv7(primaryTaskUUID) {
										output["task_uuid"] = primaryTaskUUID
									}
									encoded, err := json.Marshal(output)
									if err != nil {
										return err
									}
									updates := map[string]any{"output_json": string(encoded), "updated_at": now}
									if isUUIDv7(primaryTaskUUID) {
										updates["task_uuid"], updates["resource_uuid"] = primaryTaskUUID, primaryResourceUUID
									}
									if err := tx.Model(&workflowStepRecord{}).Where("id=?", step.ID).Updates(updates).Error; err != nil {
										return err
									}
								}
							}
						}
					}
				}
			}
			if err := tx.Model(&workflowStepRecord{}).Where("workflow_id=? AND status IN ('pending','queued','running','waiting','failed','interrupted')", row.ID).Updates(map[string]any{"status": "cancelled", "error_code": CodeCancelled, "error_message": "用户已取消。", "completed_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
			if row.ThreadID != nil {
				if _, err := recomputeThreadStatusGormTx(ctx, tx, *row.ThreadID, now); err != nil {
					return err
				}
			}
			return appendWorkflowEventGormTx(ctx, tx, row.ID, nil, "workflow_cancelled", map[string]any{"project_uuid": projectUUID, "workflow_uuid": row.UUID, "status": WorkflowCancelled}, now)
		})
	})
	if errors.Is(err, errWorkflowTransitionInactive) {
		return service.GetWorkflow(ctx, projectUUID, workflowUUID)
	}
	if err != nil {
		return Workflow{}, err
	}
	if workUUID != "" {
		service.queue.CancelAgentWork(projectUUID, workUUID)
	}
	if riverJobID > 0 {
		_ = service.queue.CancelAgentJob(context.WithoutCancel(ctx), projectUUID, riverJobID)
	}
	for _, taskUUID := range currentTaskUUIDs {
		if currentTaskKind != "" {
			_ = service.queue.CancelDomainTask(context.WithoutCancel(ctx), projectUUID, currentTaskKind, taskUUID)
		}
	}
	result, err := service.GetWorkflow(ctx, projectUUID, workflowUUID)
	if err == nil {
		service.broadcastWorkflow(projectUUID, result, "workflow:cancelled", "")
	}
	return result, err
}

func valueInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func yoloStepTaskKind(stepKey string) string {
	switch stepKey {
	case "story":
		return "story_chapter_generation"
	case "premise":
		return "production"
	case "first_section_image":
		return "comic_image_generation"
	}
	return ""
}

func (service *Service) finishWorkflowCancelled(ctx context.Context, store *project.Store, workflow workflowRecord, step workflowStepRecord, threadUUID string) error {
	_, err := service.CancelWorkflow(ctx, store.ProjectUUID(), workflow.UUID)
	return err
}

func (service *Service) RetryWorkflow(ctx context.Context, projectUUID, workflowUUID string) (Workflow, error) {
	workflow, err := service.GetWorkflow(ctx, projectUUID, workflowUUID)
	if err != nil {
		return Workflow{}, err
	}
	if workflow.Status != WorkflowFailed && workflow.Status != WorkflowInterrupted && workflow.Status != WorkflowCancelled {
		return Workflow{}, domainError(CodeStateConflict, "Workflow 当前不可重试", "仅 failed、interrupted 或 cancelled workflow 可以重试。", nil)
	}
	if workflow.Kind == WorkflowComicSectionImage {
		taskUUID := comicWorkflowTaskUUID(workflow)
		if taskUUID == "" {
			return Workflow{}, domainError(CodeStateConflict, "图片生成 Workflow 缺少生产任务", "generate_section_image 步骤没有关联 task_uuid。", nil)
		}
		if _, taskErr := service.queue.RetryDomainTask(ctx, projectUUID, "comic_image_generation", taskUUID); taskErr != nil {
			return Workflow{}, taskErr
		}
		result, getErr := service.GetWorkflow(ctx, projectUUID, workflowUUID)
		if getErr == nil {
			service.broadcastWorkflow(projectUUID, result, "workflow:queued", "")
		}
		return result, getErr
	}
	if taskKind, projected := projectedStoryWorkflowTaskKind(workflow.Kind); projected {
		taskUUID := projectedStoryWorkflowTaskUUID(workflow)
		if taskUUID == "" {
			return Workflow{}, domainError(CodeStateConflict, "Story Workflow 缺少任务", "Workflow 步骤没有关联 task_uuid。", nil)
		}
		if _, taskErr := service.queue.RetryDomainTask(ctx, projectUUID, taskKind, taskUUID); taskErr != nil {
			return Workflow{}, taskErr
		}
		result, getErr := service.GetWorkflow(ctx, projectUUID, workflowUUID)
		if getErr == nil {
			service.broadcastWorkflow(projectUUID, result, "workflow:queued", "")
		}
		return result, getErr
	}
	for _, step := range workflow.Steps {
		if step.Status == "completed" {
			continue
		}
		kind := yoloStepTaskKind(step.StepKey)
		if kind != "" {
			for _, taskUUID := range workflowTaskUUIDs(string(step.Output), step.TaskUUID) {
				task, taskErr := service.queue.GetDomainTask(ctx, projectUUID, kind, taskUUID)
				if taskErr != nil {
					return Workflow{}, taskErr
				}
				if task.Status == "failed" || task.Status == "cancelled" || task.Status == "interrupted" {
					if _, taskErr := service.queue.RetryDomainTask(ctx, projectUUID, kind, taskUUID); taskErr != nil {
						return Workflow{}, taskErr
					}
				}
			}
		}
		break
	}
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		sqlDB, err := store.DB().DB()
		if err != nil {
			return err
		}
		tx, err := sqlDB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var row workflowRecord
		if err := tx.QueryRowContext(ctx, `SELECT id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,error_code,error_message,cancel_requested_at,started_at,completed_at,created_at,updated_at FROM workflows WHERE uuid=?`, workflowUUID).Scan(&row.ID, &row.UUID, &row.ProjectID, &row.ThreadID, &row.Kind, &row.Title, &row.Status, &row.InputVersion, &row.InputSnapshot, &row.IdempotencyKey, &row.ProviderUUID, &row.Model, &row.ModelSource, &row.CurrentStepKey, &row.ErrorCode, &row.ErrorMessage, &row.CancelRequestedAt, &row.StartedAt, &row.CompletedAt, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return err
		}
		var step workflowStepRecord
		if err := tx.QueryRowContext(ctx, `SELECT id,uuid,workflow_id,step_key,position,status,idempotency_key,river_job_id,task_uuid,resource_uuid,input_json,output_json,error_code,error_message,started_at,completed_at,created_at,updated_at FROM workflow_steps WHERE workflow_id=? AND status<>'completed' ORDER BY position LIMIT 1`, row.ID).Scan(&step.ID, &step.UUID, &step.WorkflowID, &step.StepKey, &step.Position, &step.Status, &step.IdempotencyKey, &step.RiverJobID, &step.TaskUUID, &step.ResourceUUID, &step.InputJSON, &step.OutputJSON, &step.ErrorCode, &step.ErrorMessage, &step.StartedAt, &step.CompletedAt, &step.CreatedAt, &step.UpdatedAt); err != nil {
			return domainError(CodeStateConflict, "Workflow 没有可重试步骤", "所有步骤已经完成。", err)
		}
		now := service.now().UTC()
		if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='queued',current_step_key=?,cancel_requested_at=NULL,completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, step.StepKey, now, row.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='queued',completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, now, step.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='pending',river_job_id=NULL,task_uuid='',resource_uuid='',input_json='{}',output_json='{}',error_code='',error_message='',started_at=NULL,completed_at=NULL,updated_at=? WHERE workflow_id=? AND position>? AND status IN ('cancelled','failed','interrupted')`, now, row.ID, step.Position); err != nil {
			return err
		}
		var threadUUID string
		if row.ThreadID != nil {
			_ = tx.QueryRowContext(ctx, `SELECT uuid FROM chat_threads WHERE id=?`, *row.ThreadID).Scan(&threadUUID)
			if _, err := RecomputeThreadStatusTx(ctx, tx, *row.ThreadID, now); err != nil {
				return err
			}
		}
		jobID, err := service.queue.EnqueueAgentTx(ctx, projectUUID, tx, JobSpec{Version: 1, ProjectUUID: projectUUID, JobKind: JobWorkflowStep, ResourceUUID: step.UUID, ThreadUUID: threadUUID})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET river_job_id=? WHERE id=?`, jobID, step.ID); err != nil {
			return err
		}
		if err := appendWorkflowEventTx(ctx, tx, row.ID, &step.ID, "workflow_retried", map[string]any{"project_uuid": projectUUID, "workflow_uuid": row.UUID, "thread_uuid": threadUUID, "step_uuid": step.UUID, "step_key": step.StepKey, "status": WorkflowQueued}, now); err != nil {
			return err
		}
		return tx.Commit()
	})
	if err != nil {
		return Workflow{}, err
	}
	result, err := service.GetWorkflow(ctx, projectUUID, workflowUUID)
	if err == nil {
		var stepUUID string
		for _, candidate := range result.Steps {
			if candidate.Status == "queued" {
				stepUUID = candidate.UUID
				break
			}
		}
		service.broadcastWorkflow(projectUUID, result, "workflow:queued", stepUUID)
	}
	return result, err
}

func comicWorkflowTaskUUID(workflow Workflow) string {
	for _, step := range workflow.Steps {
		if step.StepKey == WorkflowStepGenerateSectionImage && step.TaskUUID != "" {
			return step.TaskUUID
		}
	}
	return ""
}

func projectedStoryWorkflowTaskKind(workflowKind string) (string, bool) {
	switch workflowKind {
	case WorkflowStoryChapter, WorkflowStoryChapterBatchPlan, WorkflowComicStoryboard:
		return workflowKind, true
	default:
		return "", false
	}
}

func projectedStoryWorkflowTaskUUID(workflow Workflow) string {
	for _, step := range workflow.Steps {
		if step.TaskUUID != "" {
			return step.TaskUUID
		}
	}
	return ""
}

func comicStoryboardWorkflowTaskUUID(workflow Workflow) string {
	return projectedStoryWorkflowTaskUUID(workflow)
}

func (service *Service) broadcastWorkflow(projectUUID string, workflow Workflow, event, stepUUID string) {
	if service.hub == nil {
		return
	}
	payload := map[string]any{"project_uuid": projectUUID, "workflow_uuid": workflow.UUID, "thread_uuid": workflow.ThreadUUID, "status": workflow.Status}
	if _, projected := projectedStoryWorkflowTaskKind(workflow.Kind); projected {
		for _, step := range workflow.Steps {
			if step.TaskUUID == "" {
				continue
			}
			payload["task_uuid"] = step.TaskUUID
			payload["resource_uuid"] = step.ResourceUUID
			payload["progress"] = step.Progress
			if stepUUID == "" {
				stepUUID = step.UUID
			}
			break
		}
	}
	if stepUUID != "" {
		payload["step_uuid"] = stepUUID
	}
	if workflow.ErrorCode != "" {
		payload["error_code"] = workflow.ErrorCode
	}
	service.hub.Broadcast("project:"+projectUUID, event, publicRealtimePayload(payload))
}
