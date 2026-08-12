package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"lumi/internal/agent"
	"lumi/internal/production"
)

const comicWorkflowTitle = agent.WorkflowComicSectionImage

type comicWorkflowSnapshot struct {
	Version            int    `json:"version"`
	ProjectUUID        string `json:"project_uuid"`
	ChapterUUID        string `json:"chapter_uuid"`
	SectionUUID        string `json:"section_uuid"`
	SectionTitle       string `json:"section_title"`
	GenerationUUID     string `json:"generation_uuid"`
	ProductionTaskUUID string `json:"production_task_uuid"`
	ModelSource        string `json:"model_source"`
}

type comicWorkflowRef struct {
	ID         int64
	ThreadID   int64
	UUID       string
	ThreadUUID string
}

func createComicImageWorkflowTx(ctx context.Context, tx *sql.Tx, projectID int64, projectUUID, chapterUUID, generationUUID, taskUUID string, section production.ComicSection, providerUUID, model, modelSource string, now time.Time) error {
	threadUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	workflowUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	threadResult, err := tx.ExecContext(ctx, `INSERT INTO chat_threads(uuid,project_id,title,status,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,scope,scene,subject_uuid,created_at,updated_at) VALUES(?,?,?,'busy',?,?,?,1,1,1,'project','',?, ?,?)`, threadUUID, projectID, comicWorkflowTitle, providerUUID, model, modelSource, section.UUID, now, now)
	if err != nil {
		return err
	}
	threadID, err := threadResult.LastInsertId()
	if err != nil {
		return err
	}
	snapshot, err := json.Marshal(comicWorkflowSnapshot{
		Version: 1, ProjectUUID: projectUUID, ChapterUUID: chapterUUID, SectionUUID: section.UUID,
		SectionTitle: section.Title, GenerationUUID: generationUUID, ProductionTaskUUID: taskUUID, ModelSource: modelSource,
	})
	if err != nil {
		return err
	}
	workflowResult, err := tx.ExecContext(ctx, `INSERT INTO workflows(uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(?,?,?, ?,?,'queued',1,?,?,?,?,?,?,?,?)`, workflowUUID, projectID, threadID, agent.WorkflowComicSectionImage, comicWorkflowTitle, string(snapshot), "comic-image:"+taskUUID, providerUUID, model, modelSource, agent.WorkflowStepSelectReferences, now, now)
	if err != nil {
		return err
	}
	workflowID, err := workflowResult.LastInsertId()
	if err != nil {
		return err
	}
	stepInput, _ := json.Marshal(map[string]any{"chapter_uuid": chapterUUID, "section_uuid": section.UUID})
	for index, stepKey := range agent.ComicSectionImageStepKeys {
		stepUUID, uuidErr := newUUIDv7()
		if uuidErr != nil {
			return uuidErr
		}
		status := "pending"
		if index == 0 {
			status = "queued"
		}
		linkedTaskUUID := ""
		if stepKey == agent.WorkflowStepGenerateSectionImage {
			linkedTaskUUID = taskUUID
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,task_uuid,resource_uuid,input_json,output_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,'{}',?,?)`, stepUUID, workflowID, stepKey, index+1, status, workflowUUID+":"+stepKey, linkedTaskUUID, section.UUID, string(stepInput), now, now); err != nil {
			return err
		}
	}
	return appendComicWorkflowEventTx(ctx, tx, workflowID, "workflow_queued", map[string]any{
		"project_uuid": projectUUID, "workflow_uuid": workflowUUID, "thread_uuid": threadUUID,
		"task_uuid": taskUUID, "resource_uuid": section.UUID, "status": agent.WorkflowQueued,
	}, now)
}

func comicWorkflowRefTx(ctx context.Context, tx *sql.Tx, taskUUID string) (comicWorkflowRef, bool, error) {
	var ref comicWorkflowRef
	err := tx.QueryRowContext(ctx, `SELECT w.id,w.thread_id,w.uuid,t.uuid FROM workflows w JOIN chat_threads t ON t.id=w.thread_id WHERE w.kind=? AND EXISTS(SELECT 1 FROM workflow_steps s WHERE s.workflow_id=w.id AND s.task_uuid=?) LIMIT 1`, agent.WorkflowComicSectionImage, taskUUID).Scan(&ref.ID, &ref.ThreadID, &ref.UUID, &ref.ThreadUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return ref, false, nil
	}
	return ref, err == nil, err
}

func markComicWorkflowRunningTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := comicWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='running',started_at=COALESCE(started_at,?),completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=(SELECT id FROM workflow_steps WHERE workflow_id=? AND status<>'completed' ORDER BY position LIMIT 1)`, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='running',current_step_key=COALESCE((SELECT step_key FROM workflow_steps WHERE workflow_id=? AND status='running' ORDER BY position LIMIT 1),current_step_key),started_at=COALESCE(started_at,?),completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, ref.ID, now, now, ref.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE chat_threads SET status='busy',updated_at=? WHERE id=?`, now, ref.ThreadID)
	return err
}

func markComicReferencesSelected(ctx context.Context, runtime *projectRuntime, record productionTaskRecord, selection sectionReferenceSelection) error {
	return runtime.updateComicWorkflow(ctx, record.UUID, func(tx *sql.Tx, ref comicWorkflowRef, now time.Time) error {
		titles := make([]string, 0, len(selection.References))
		for _, reference := range selection.References {
			titles = append(titles, reference.Title)
		}
		output, _ := json.Marshal(map[string]any{"selected_titles": titles, "selection_reason": selection.Reason, "reference_count": len(selection.References)})
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='completed',output_json=?,started_at=COALESCE(started_at,?),completed_at=?,error_code='',error_message='',updated_at=? WHERE workflow_id=? AND step_key=?`, string(output), now, now, now, ref.ID, agent.WorkflowStepSelectReferences); err != nil {
			return err
		}
		if len(selection.References) == 0 {
			skipped, _ := json.Marshal(map[string]any{"skipped": true, "reason": "no_reference_assets"})
			if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='completed',output_json=?,started_at=COALESCE(started_at,?),completed_at=?,error_code='',error_message='',updated_at=? WHERE workflow_id=? AND step_key=?`, string(skipped), now, now, now, ref.ID, agent.WorkflowStepSaveSectionPremise); err != nil {
				return err
			}
			return setComicWorkflowStepRunningTx(ctx, tx, ref, agent.WorkflowStepGenerateSectionImage, now)
		}
		return setComicWorkflowStepRunningTx(ctx, tx, ref, agent.WorkflowStepSaveSectionPremise, now)
	})
}

func markComicPremiseSaved(ctx context.Context, runtime *projectRuntime, record productionTaskRecord, premiseAssetUUID string) error {
	return runtime.updateComicWorkflow(ctx, record.UUID, func(tx *sql.Tx, ref comicWorkflowRef, now time.Time) error {
		output, _ := json.Marshal(map[string]any{"premise_asset_uuid": premiseAssetUUID})
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='completed',output_json=?,completed_at=?,error_code='',error_message='',updated_at=? WHERE workflow_id=? AND step_key=?`, string(output), now, now, ref.ID, agent.WorkflowStepSaveSectionPremise); err != nil {
			return err
		}
		return setComicWorkflowStepRunningTx(ctx, tx, ref, agent.WorkflowStepGenerateSectionImage, now)
	})
}

func markComicImageGenerated(ctx context.Context, runtime *projectRuntime, record productionTaskRecord) error {
	return runtime.updateComicWorkflow(ctx, record.UUID, func(tx *sql.Tx, ref comicWorkflowRef, now time.Time) error {
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='completed',completed_at=?,error_code='',error_message='',updated_at=? WHERE workflow_id=? AND step_key=?`, now, now, ref.ID, agent.WorkflowStepGenerateSectionImage); err != nil {
			return err
		}
		return setComicWorkflowStepRunningTx(ctx, tx, ref, agent.WorkflowStepSaveSectionImage, now)
	})
}

func markComicImageSaved(ctx context.Context, runtime *projectRuntime, record productionTaskRecord, imageVariantUUID string) error {
	return runtime.updateComicWorkflow(ctx, record.UUID, func(tx *sql.Tx, ref comicWorkflowRef, now time.Time) error {
		output, _ := json.Marshal(map[string]any{"image_variant_uuid": imageVariantUUID})
		_, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET output_json=?,updated_at=? WHERE workflow_id=? AND step_key=?`, string(output), now, ref.ID, agent.WorkflowStepSaveSectionImage)
		return err
	})
}

func setComicWorkflowStepRunningTx(ctx context.Context, tx *sql.Tx, ref comicWorkflowRef, stepKey string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='running',started_at=COALESCE(started_at,?),completed_at=NULL,error_code='',error_message='',updated_at=? WHERE workflow_id=? AND step_key=?`, now, now, ref.ID, stepKey); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='running',current_step_key=?,started_at=COALESCE(started_at,?),completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, stepKey, now, now, ref.ID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='busy',updated_at=? WHERE id=?`, now, ref.ThreadID)
	return err
}

func completeComicWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := comicWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='completed',started_at=COALESCE(started_at,?),completed_at=COALESCE(completed_at,?),error_code='',error_message='',updated_at=? WHERE workflow_id=?`, now, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='completed',current_step_key=?,completed_at=?,error_code='',error_message='',updated_at=? WHERE id=?`, agent.WorkflowStepSaveSectionImage, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='completed',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	return appendComicWorkflowEventTx(ctx, tx, ref.ID, "workflow_completed", map[string]any{"workflow_uuid": ref.UUID, "thread_uuid": ref.ThreadUUID, "task_uuid": taskUUID, "status": agent.WorkflowCompleted}, now)
}

func failComicWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID, code, message string, now time.Time) error {
	ref, found, err := comicWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='failed',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=(SELECT id FROM workflow_steps WHERE workflow_id=? AND status<>'completed' ORDER BY position LIMIT 1)`, now, code, message, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='failed',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=?`, now, code, message, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='failed',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	return appendComicWorkflowEventTx(ctx, tx, ref.ID, "workflow_failed", map[string]any{"workflow_uuid": ref.UUID, "thread_uuid": ref.ThreadUUID, "task_uuid": taskUUID, "status": agent.WorkflowFailed, "error_code": code}, now)
}

func cancelComicWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := comicWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='cancelled',completed_at=COALESCE(completed_at,?),error_code='cancelled',error_message='用户已取消。',updated_at=? WHERE workflow_id=? AND status<>'completed'`, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='cancelled',cancel_requested_at=?,completed_at=?,error_code='cancelled',error_message='用户已取消。',updated_at=? WHERE id=?`, now, now, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET status='cancelled',updated_at=? WHERE id=?`, now, ref.ThreadID); err != nil {
		return err
	}
	return appendComicWorkflowEventTx(ctx, tx, ref.ID, "workflow_cancelled", map[string]any{"workflow_uuid": ref.UUID, "thread_uuid": ref.ThreadUUID, "task_uuid": taskUUID, "status": agent.WorkflowCancelled}, now)
}

func queueComicWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID string, now time.Time) error {
	ref, found, err := comicWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	var position int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(position),1) FROM workflow_steps WHERE workflow_id=? AND status<>'completed'`, ref.ID).Scan(&position); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status=CASE WHEN position=? THEN 'queued' ELSE 'pending' END,started_at=CASE WHEN position=? THEN started_at ELSE NULL END,completed_at=NULL,error_code='',error_message='',updated_at=? WHERE workflow_id=? AND position>=? AND status<>'completed'`, position, position, now, ref.ID, position); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='queued',current_step_key=(SELECT step_key FROM workflow_steps WHERE workflow_id=? AND position=?),cancel_requested_at=NULL,completed_at=NULL,error_code='',error_message='',updated_at=? WHERE id=?`, ref.ID, position, now, ref.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE chat_threads SET status='busy',updated_at=? WHERE id=?`, now, ref.ThreadID)
	return err
}

func interruptComicWorkflowTx(ctx context.Context, tx *sql.Tx, taskUUID, code, message string, now time.Time) error {
	ref, found, err := comicWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status='interrupted',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=(SELECT id FROM workflow_steps WHERE workflow_id=? AND status<>'completed' ORDER BY position LIMIT 1)`, now, code, message, now, ref.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET status='interrupted',completed_at=?,error_code=?,error_message=?,updated_at=? WHERE id=?`, now, code, message, now, ref.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE chat_threads SET status='interrupted',updated_at=? WHERE id=?`, now, ref.ThreadID)
	return err
}

func reconcileComicImageWorkflows(ctx context.Context, db *sql.DB, projectID int64, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT tasks.uuid,tasks.status,tasks.error_code,tasks.error_message,w.status FROM production_task_runs tasks JOIN workflow_steps s ON s.task_uuid=tasks.uuid JOIN workflows w ON w.id=s.workflow_id WHERE tasks.project_id=? AND tasks.kind=? AND w.kind=?`, projectID, KindComicImageGeneration, agent.WorkflowComicSectionImage)
	if err != nil {
		return err
	}
	type row struct{ taskUUID, taskStatus, errorCode, errorMessage, workflowStatus string }
	var items []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.taskUUID, &item.taskStatus, &item.errorCode, &item.errorMessage, &item.workflowStatus); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range items {
		switch item.taskStatus {
		case StatusQueued, StatusRunning:
			if err := queueComicWorkflowTx(ctx, tx, item.taskUUID, now); err != nil {
				return err
			}
		case StatusCompleted:
			if item.workflowStatus != agent.WorkflowCompleted {
				if err := completeComicWorkflowTx(ctx, tx, item.taskUUID, now); err != nil {
					return err
				}
			}
		case StatusFailed:
			if item.workflowStatus != agent.WorkflowFailed {
				if err := failComicWorkflowTx(ctx, tx, item.taskUUID, item.errorCode, item.errorMessage, now); err != nil {
					return err
				}
			}
		case StatusCancelled:
			if item.workflowStatus != agent.WorkflowCancelled {
				if err := cancelComicWorkflowTx(ctx, tx, item.taskUUID, now); err != nil {
					return err
				}
			}
		case StatusInterrupted:
			if item.workflowStatus != agent.WorkflowInterrupted {
				if err := interruptComicWorkflowTx(ctx, tx, item.taskUUID, item.errorCode, item.errorMessage, now); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func (runtime *projectRuntime) updateComicWorkflow(ctx context.Context, taskUUID string, update func(*sql.Tx, comicWorkflowRef, time.Time) error) error {
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ref, found, err := comicWorkflowRefTx(ctx, tx, taskUUID)
	if err != nil || !found {
		return err
	}
	if err := update(tx, ref, runtime.manager.now().UTC()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	runtime.broadcastComicWorkflow("workflow:step_changed", taskUUID)
	return nil
}

func appendComicWorkflowEventTx(ctx context.Context, tx *sql.Tx, workflowID int64, eventType string, payload any, now time.Time) error {
	eventUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO workflow_events(uuid,workflow_id,sequence,event_type,payload_json,created_at) SELECT ?,?,COALESCE(MAX(sequence),0)+1,?,?,? FROM workflow_events WHERE workflow_id=?`, eventUUID, workflowID, eventType, string(encoded), now, workflowID)
	return err
}

func (runtime *projectRuntime) broadcastComicWorkflow(event, taskUUID string) {
	if runtime.manager.hub == nil {
		return
	}
	var workflowUUID, threadUUID, resourceUUID, status string
	err := runtime.sqlDB.QueryRowContext(context.Background(), `SELECT w.uuid,t.uuid,s.resource_uuid,w.status FROM workflows w JOIN chat_threads t ON t.id=w.thread_id JOIN workflow_steps s ON s.workflow_id=w.id AND s.step_key=? WHERE w.kind=? AND s.task_uuid=? LIMIT 1`, agent.WorkflowStepGenerateSectionImage, agent.WorkflowComicSectionImage, taskUUID).Scan(&workflowUUID, &threadUUID, &resourceUUID, &status)
	if err != nil {
		return
	}
	runtime.manager.hub.Broadcast("project:"+runtime.projectUUID, event, map[string]any{
		"project_uuid": runtime.projectUUID, "workflow_uuid": workflowUUID, "thread_uuid": threadUUID,
		"task_uuid": taskUUID, "resource_uuid": resourceUUID, "status": status,
	})
}
