package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"lumi/internal/project"
)

func (service *Service) ListFollowUps(ctx context.Context, projectUUID, threadUUID string) ([]FollowUp, error) {
	var result []FollowUp
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		var thread threadRecord
		if err := store.DB().WithContext(ctx).Where("project_id=? AND uuid=?", pid, threadUUID).First(&thread).Error; err != nil {
			return notFound(err, "Chat thread 不存在")
		}
		var rows []followUpRecord
		if err := store.DB().WithContext(ctx).Where("thread_id=? AND deleted_at IS NULL AND status='queued'", thread.ID).Order("position,id").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			item := followUpDTO(row, threadUUID, "")
			item.References, err = service.followUpReferences(ctx, store, row.ID)
			if err != nil {
				return err
			}
			result = append(result, item)
		}
		return nil
	})
	return result, err
}

func (service *Service) CreateFollowUp(ctx context.Context, projectUUID, threadUUID string, input CreateFollowUpInput) (FollowUp, error) {
	text, err := validateText(input.InputText, 256<<10, "Follow-up")
	if err != nil {
		return FollowUp{}, err
	}
	if !isUUIDv7(threadUUID) {
		return FollowUp{}, domainError(CodeValidation, "Thread UUID 无效", "thread_uuid 必须是 UUIDv7。", nil)
	}
	uuid, err := newUUIDv7()
	if err != nil {
		return FollowUp{}, err
	}
	now := service.now().UTC()
	var result FollowUp
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		var thread threadRecord
		if err := store.DB().WithContext(ctx).Where("project_id=? AND uuid=? AND archived_at IS NULL", pid, threadUUID).First(&thread).Error; err != nil {
			return notFound(err, "Chat thread 不存在")
		}
		references, err := service.resolveContextReferences(ctx, store, thread.ProjectID, input.References)
		if err != nil {
			return err
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
		var active int64
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM chat_turns WHERE thread_id=? AND status IN ('queued','in_progress','waiting_for_input')`, thread.ID).Scan(&active); err != nil {
			return err
		}
		if active == 0 {
			return domainError(CodeStateConflict, "当前没有可跟进的 turn", "Follow-up 只能排在尚未完成的 turn 之后；空闲时请发送新 turn。", nil)
		}
		var position int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position),0)+1 FROM chat_follow_ups WHERE thread_id=? AND deleted_at IS NULL AND status='queued'`, thread.ID).Scan(&position); err != nil {
			return err
		}
		insert, err := tx.ExecContext(ctx, `INSERT INTO chat_follow_ups(uuid,thread_id,input_text,position,status,created_at,updated_at) VALUES(?,?,?,?,'queued',?,?)`, uuid, thread.ID, text, position, now, now)
		if err != nil {
			return err
		}
		followUpID, err := insert.LastInsertId()
		if err != nil {
			return err
		}
		if err := attachFollowUpReferencesTx(ctx, tx, followUpID, references, now); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		record := followUpRecord{ID: followUpID, UUID: uuid, ThreadID: thread.ID, InputText: text, Position: position, Status: "queued", CreatedAt: now, UpdatedAt: now}
		result = followUpDTO(record, threadUUID, "")
		result.References, err = service.followUpReferences(ctx, store, record.ID)
		if err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		service.broadcastThread(projectUUID, threadUUID, "chat:follow_up_changed", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "follow_up_uuid": result.UUID, "action": "created"})
	}
	return result, err
}

func followUpDTO(row followUpRecord, threadUUID, promotedTurnUUID string) FollowUp {
	return FollowUp{UUID: row.UUID, ThreadUUID: threadUUID, InputText: row.InputText, Position: row.Position, Status: row.Status, PromotedTurnUUID: promotedTurnUUID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt}
}

func (service *Service) UpdateFollowUp(ctx context.Context, projectUUID, threadUUID, followUpUUID string, input UpdateFollowUpInput) (FollowUp, error) {
	text, err := validateText(input.InputText, 256<<10, "Follow-up")
	if err != nil {
		return FollowUp{}, err
	}
	var result FollowUp
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		var thread threadRecord
		if err := store.DB().WithContext(ctx).Where("project_id=? AND uuid=? AND archived_at IS NULL", pid, threadUUID).First(&thread).Error; err != nil {
			return notFound(err, "Chat thread 不存在")
		}
		var replacement []storedContextReference
		if input.References != nil {
			replacement, err = service.resolveContextReferences(ctx, store, thread.ProjectID, *input.References)
			if err != nil {
				return err
			}
		}
		now := service.now().UTC()
		var row followUpRecord
		err = store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("uuid=? AND thread_id=? AND status='queued' AND deleted_at IS NULL", followUpUUID, thread.ID).First(&row).Error; err != nil {
				return domainError(CodeStateConflict, "Follow-up 无法修改", "只有 queued follow-up 可以修改。", err)
			}
			if err := tx.Model(&row).Updates(map[string]any{"input_text": text, "updated_at": now}).Error; err != nil {
				return err
			}
			if input.References == nil {
				return nil
			}
			if err := tx.Exec("DELETE FROM chat_context_references WHERE follow_up_id=?", row.ID).Error; err != nil {
				return err
			}
			sqlTx, ok := tx.Statement.ConnPool.(*sql.Tx)
			if !ok {
				return domainError(CodeStateConflict, "Follow-up Reference 无法更新", "数据库事务连接不可用。", nil)
			}
			return attachFollowUpReferencesTx(ctx, sqlTx, row.ID, replacement, now)
		})
		if err != nil {
			return err
		}
		row.InputText, row.UpdatedAt = text, now
		result = followUpDTO(row, threadUUID, "")
		result.References, err = service.followUpReferences(ctx, store, row.ID)
		if err != nil {
			return err
		}
		return nil
	})
	return result, err
}

func (service *Service) MoveFollowUp(ctx context.Context, projectUUID, threadUUID, followUpUUID string, target int) ([]FollowUp, error) {
	if target < 1 {
		return nil, domainError(CodeValidation, "目标位置无效", "position 必须从 1 开始。", nil)
	}
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		return store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var thread threadRecord
			if err := tx.Where("project_id=? AND uuid=?", pid, threadUUID).First(&thread).Error; err != nil {
				return notFound(err, "Chat thread 不存在")
			}
			var rows []followUpRecord
			if err := tx.Where("thread_id=? AND status='queued' AND deleted_at IS NULL", thread.ID).Order("position,id").Find(&rows).Error; err != nil {
				return err
			}
			if target > len(rows) {
				target = len(rows)
			}
			index := -1
			for i := range rows {
				if rows[i].UUID == followUpUUID {
					index = i
					break
				}
			}
			if index < 0 {
				return domainError(CodeNotFound, "Follow-up 不存在", "只能移动 queued follow-up。", nil)
			}
			row := rows[index]
			rows = append(rows[:index], rows[index+1:]...)
			targetIndex := target - 1
			rows = append(rows, followUpRecord{})
			copy(rows[targetIndex+1:], rows[targetIndex:])
			rows[targetIndex] = row
			now := service.now().UTC()
			for i := range rows {
				if err := tx.Model(&followUpRecord{}).Where("id=?", rows[i].ID).Updates(map[string]any{"position": i + 1, "updated_at": now}).Error; err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	service.broadcastThread(projectUUID, threadUUID, "chat:follow_up_changed", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "follow_up_uuid": followUpUUID, "action": "moved"})
	return service.ListFollowUps(ctx, projectUUID, threadUUID)
}

func (service *Service) DeleteFollowUp(ctx context.Context, projectUUID, threadUUID, followUpUUID string) error {
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		return store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var followUpID int64
			if err := tx.Table("chat_follow_ups").Select("id").Where("uuid=? AND thread_id=(SELECT id FROM chat_threads WHERE project_id=? AND uuid=?) AND status='queued' AND deleted_at IS NULL", followUpUUID, pid, threadUUID).Scan(&followUpID).Error; err != nil {
				return err
			}
			if followUpID == 0 {
				return domainError(CodeStateConflict, "Follow-up 无法删除", "只有 queued follow-up 可以删除。", nil)
			}
			now := service.now().UTC()
			result := tx.Table("chat_follow_ups").Where("id=? AND status='queued' AND deleted_at IS NULL", followUpID).Updates(map[string]any{"status": "deleted", "deleted_at": now, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return domainError(CodeStateConflict, "Follow-up 无法删除", "只有 queued follow-up 可以删除。", nil)
			}
			if err := tx.Exec("DELETE FROM chat_context_references WHERE follow_up_id=?", followUpID).Error; err != nil {
				return err
			}
			return compactFollowUps(tx, threadUUID)
		})
	})
	if err == nil {
		service.broadcastThread(projectUUID, threadUUID, "chat:follow_up_changed", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "follow_up_uuid": followUpUUID, "action": "deleted"})
	}
	return err
}

func compactFollowUps(db *gorm.DB, threadUUID string) error {
	var rows []followUpRecord
	if err := db.Where("thread_id=(SELECT id FROM chat_threads WHERE uuid=?) AND status='queued' AND deleted_at IS NULL", threadUUID).Order("position,id").Find(&rows).Error; err != nil {
		return err
	}
	for index, row := range rows {
		if row.Position != index+1 {
			if err := db.Model(&followUpRecord{}).Where("id=?", row.ID).Update("position", index+1).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (service *Service) Steer(ctx context.Context, projectUUID, threadUUID string, input SteeringInput) (Item, error) {
	text, err := validateText(input.InputText, 64<<10, "Steering")
	if err != nil {
		return Item{}, err
	}
	var result Item
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		var promptThread threadRecord
		if err := store.DB().WithContext(ctx).Where("project_id=? AND uuid=? AND archived_at IS NULL", pid, threadUUID).First(&promptThread).Error; err != nil {
			return notFound(err, "Chat thread 不存在")
		}
		references, err := service.resolveContextReferences(ctx, store, promptThread.ProjectID, input.References)
		if err != nil {
			return err
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
		thread, err := lockThreadSQL(ctx, tx, pid, threadUUID)
		if err != nil {
			return err
		}
		var runID, turnID int64
		var runUUID, turnUUID, runStatus string
		if err := tx.QueryRowContext(ctx, `SELECT r.id,r.uuid,r.status,t.id,t.uuid FROM chat_runs r JOIN chat_turns t ON t.id=r.turn_id WHERE r.thread_id=? AND r.status='in_progress' AND NOT EXISTS(SELECT 1 FROM workflow_awaits a WHERE a.chat_run_id=r.id AND a.status='waiting') ORDER BY r.created_at DESC,r.id DESC LIMIT 1`, thread.ID).Scan(&runID, &runUUID, &runStatus, &turnID, &turnUUID); err != nil {
			return domainError(CodeBusy, "当前没有可 Steering 的运行", "Steering 只能在 run 的安全边界注入。", err)
		}
		now := service.now().UTC()
		row, err := appendItemTx(ctx, tx, &thread, &turnID, &runID, "user_message", "user", text, "text", "completed", "", "", "", map[string]any{"steering": true}, now)
		if err != nil {
			return err
		}
		if err := attachItemReferencesTx(ctx, tx, row.ID, references, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET no_progress_streak=0,last_cycle_fingerprint='',updated_at=? WHERE id=?`, now, runID); err != nil {
			return err
		}
		if _, err := appendEventTx(ctx, tx, &thread, &runID, "steering_queued", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "turn_uuid": turnUUID, "run_uuid": runUUID, "item_uuid": row.UUID}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		result = itemDTO(row, threadUUID, turnUUID, runUUID)
		result.References, err = service.itemReferences(ctx, store, row.ID)
		if err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		service.broadcastThread(projectUUID, threadUUID, "chat:steering_queued", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "item_uuid": result.UUID})
	}
	return result, err
}

// SteerFollowUp promotes one persisted queued follow-up into the currently
// running turn. If the safe steering window has already closed, the queue row
// is deliberately left unchanged so the user's message is still delivered by
// normal FIFO promotion.
func (service *Service) SteerFollowUp(ctx context.Context, projectUUID, threadUUID, followUpUUID string) (FollowUpDelivery, error) {
	if !isUUIDv7(threadUUID) || !isUUIDv7(followUpUUID) {
		return FollowUpDelivery{}, domainError(CodeValidation, "Follow-up UUID 无效", "thread_uuid 与 follow_up_uuid 必须是 UUIDv7。", nil)
	}
	var result FollowUpDelivery
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
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
		thread, err := lockThreadSQL(ctx, tx, pid, threadUUID)
		if err != nil {
			return err
		}
		var followUp followUpRecord
		if err := tx.QueryRowContext(ctx, `SELECT id,thread_id,uuid,input_text,position,status,promoted_turn_id,created_at,updated_at,deleted_at FROM chat_follow_ups WHERE thread_id=? AND uuid=? AND status='queued' AND deleted_at IS NULL`, thread.ID, followUpUUID).Scan(&followUp.ID, &followUp.ThreadID, &followUp.UUID, &followUp.InputText, &followUp.Position, &followUp.Status, &followUp.PromotedTurnID, &followUp.CreatedAt, &followUp.UpdatedAt, &followUp.DeletedAt); err != nil {
			return notFound(err, "Follow-up 不存在")
		}

		var runID, turnID int64
		var runUUID, turnUUID string
		runErr := tx.QueryRowContext(ctx, `SELECT r.id,r.uuid,t.id,t.uuid FROM chat_runs r JOIN chat_turns t ON t.id=r.turn_id WHERE r.thread_id=? AND r.status='in_progress' AND t.status='in_progress' AND NOT EXISTS(SELECT 1 FROM workflow_awaits a WHERE a.chat_run_id=r.id AND a.status='waiting') ORDER BY r.created_at DESC,r.id DESC LIMIT 1`, thread.ID).Scan(&runID, &runUUID, &turnID, &turnUUID)
		if errors.Is(runErr, sql.ErrNoRows) {
			if err := tx.Commit(); err != nil {
				return err
			}
			queued := followUpDTO(followUp, threadUUID, "")
			queued.References, err = service.followUpReferences(ctx, store, followUp.ID)
			if err != nil {
				return err
			}
			result = FollowUpDelivery{DeliveryMode: "follow_up", FollowUp: &queued}
			return nil
		}
		if runErr != nil {
			return runErr
		}
		references, err := loadFollowUpReferencesTx(ctx, tx, followUp.ID)
		if err != nil {
			return err
		}
		now := service.now().UTC()
		row, err := appendItemTx(ctx, tx, &thread, &turnID, &runID, "user_message", "user", followUp.InputText, "text", "completed", "", "", "", map[string]any{"steering": true, "follow_up_uuid": followUp.UUID}, now)
		if err != nil {
			return err
		}
		if err := attachItemReferencesTx(ctx, tx, row.ID, references, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET no_progress_streak=0,last_cycle_fingerprint='',updated_at=? WHERE id=?`, now, runID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM chat_context_references WHERE follow_up_id=?`, followUp.ID); err != nil {
			return err
		}
		promoted, err := tx.ExecContext(ctx, `UPDATE chat_follow_ups SET status='promoted',promoted_turn_id=?,updated_at=? WHERE id=? AND status='queued' AND deleted_at IS NULL`, turnID, now, followUp.ID)
		if err != nil {
			return err
		}
		if affected, err := promoted.RowsAffected(); err != nil || affected != 1 {
			return domainError(CodeStateConflict, "Follow-up 引导窗口已变化", "排队项已被其他操作处理，请刷新后重试。", err)
		}
		if err := compactFollowUpsSQLTx(ctx, tx, thread.ID, now); err != nil {
			return err
		}
		if _, err := appendEventTx(ctx, tx, &thread, &runID, "steering_queued", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "turn_uuid": turnUUID, "run_uuid": runUUID, "item_uuid": row.UUID, "follow_up_uuid": followUp.UUID}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		item := itemDTO(row, threadUUID, turnUUID, runUUID)
		item.References, err = service.itemReferences(ctx, store, row.ID)
		if err != nil {
			return err
		}
		result = FollowUpDelivery{DeliveryMode: "steering", Item: &item}
		return nil
	})
	if err == nil && result.DeliveryMode == "steering" && result.Item != nil {
		service.broadcastThread(projectUUID, threadUUID, "chat:steering_queued", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "item_uuid": result.Item.UUID, "follow_up_uuid": followUpUUID})
	}
	return result, err
}

func compactFollowUpsSQLTx(ctx context.Context, tx *sql.Tx, threadID int64, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM chat_follow_ups WHERE thread_id=? AND status='queued' AND deleted_at IS NULL ORDER BY position,id`, threadID)
	if err != nil {
		return err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for index, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE chat_follow_ups SET position=?,updated_at=? WHERE id=?`, index+1, now, id); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) Abort(ctx context.Context, projectUUID, threadUUID string) (Turn, error) {
	var result Turn
	var jobID int64
	var awaitedTasks []struct{ Kind, UUID string }
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
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
		thread, err := lockThreadSQL(ctx, tx, pid, threadUUID)
		if err != nil {
			return err
		}
		var turn turnRecord
		var runID int64
		var runUUID string
		err = tx.QueryRowContext(ctx, `SELECT t.id,t.uuid,t.thread_id,t.source_type,COALESCE(t.source_follow_up_id,0),t.queue_sequence,t.input_text,t.status,t.river_job_id,t.error_code,t.error_message,t.cancel_requested_at,t.started_at,t.completed_at,t.created_at,t.updated_at,r.id,r.uuid FROM chat_turns t JOIN chat_runs r ON r.turn_id=t.id WHERE t.thread_id=? AND t.status IN ('queued','in_progress','waiting_for_input') ORDER BY CASE t.status WHEN 'in_progress' THEN 0 WHEN 'waiting_for_input' THEN 1 ELSE 2 END,t.queue_sequence LIMIT 1`, thread.ID).Scan(&turn.ID, &turn.UUID, &turn.ThreadID, &turn.SourceType, &turn.SourceFollowUpID, &turn.QueueSequence, &turn.InputText, &turn.Status, &turn.RiverJobID, &turn.ErrorCode, &turn.ErrorMessage, &turn.CancelRequestedAt, &turn.StartedAt, &turn.CompletedAt, &turn.CreatedAt, &turn.UpdatedAt, &runID, &runUUID)
		if err != nil {
			return domainError(CodeStateConflict, "Thread 当前没有可取消的 turn", "所有 turn 已处于稳定状态。", err)
		}
		now := service.now().UTC()
		awaitRows, err := tx.QueryContext(ctx, `SELECT w.kind,s.task_uuid
			FROM workflow_awaits a
			JOIN workflows w ON w.id=a.workflow_id
			JOIN workflow_steps s ON s.workflow_id=w.id
			WHERE a.chat_run_id=? AND a.status IN ('waiting','ready','resuming') AND s.task_uuid<>''`, runID)
		if err != nil {
			return err
		}
		for awaitRows.Next() {
			var task struct{ Kind, UUID string }
			if err := awaitRows.Scan(&task.Kind, &task.UUID); err != nil {
				awaitRows.Close()
				return err
			}
			awaitedTasks = append(awaitedTasks, task)
		}
		if err := awaitRows.Close(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status='cancelled',cancel_requested_at=?,completed_at=?,updated_at=?,error_code='agent_cancelled',error_message='用户已取消。' WHERE id=? AND status IN ('queued','in_progress','waiting_for_input')`, now, now, now, turn.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status='cancelled',cancel_requested_at=?,completed_at=?,updated_at=?,error_code='agent_cancelled',error_message='用户已取消。' WHERE id=? AND status IN ('queued','in_progress','waiting_for_input')`, now, now, now, runID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_user_input_requests SET status='cancelled',cancelled_at=?,updated_at=? WHERE run_id=? AND status IN ('pending','answered','resuming')`, now, now, runID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_awaits SET status='cancelled',cancelled_at=COALESCE(cancelled_at,?),updated_at=? WHERE chat_run_id=? AND status IN ('waiting','ready','resuming')`, now, now, runID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflows SET cancel_requested_at=COALESCE(cancel_requested_at,?),updated_at=? WHERE id IN (SELECT workflow_id FROM workflow_awaits WHERE chat_run_id=?) AND status IN ('queued','running')`, now, now, runID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE task_runs SET cancel_requested_at=COALESCE(cancel_requested_at,?),updated_at=? WHERE uuid IN (SELECT s.task_uuid FROM workflow_awaits a JOIN workflow_steps s ON s.workflow_id=a.workflow_id WHERE a.chat_run_id=?) AND status IN ('queued','running','waiting_for_input')`, now, now, runID); err != nil {
			return err
		}
		if _, err := appendEventTx(ctx, tx, &thread, &runID, "abort_requested", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "turn_uuid": turn.UUID, "run_uuid": runUUID, "status": TurnCancelled}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextEventSequence, now, thread.ID); err != nil {
			return err
		}
		if _, err := RecomputeThreadStatusTx(ctx, tx, thread.ID, now); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		turn.Status, turn.CancelRequestedAt, turn.CompletedAt, turn.UpdatedAt = TurnCancelled, &now, &now, now
		turn.ErrorCode, turn.ErrorMessage = CodeCancelled, "用户已取消。"
		result = turnDTO(turn, threadUUID, "")
		if turn.RiverJobID != nil {
			jobID = *turn.RiverJobID
		}
		return nil
	})
	if err != nil {
		return Turn{}, err
	}
	service.queue.CancelAgentWork(projectUUID, result.UUID)
	if jobID > 0 {
		_ = service.queue.CancelAgentJob(context.WithoutCancel(ctx), projectUUID, jobID)
	}
	for _, task := range awaitedTasks {
		_ = service.queue.CancelDomainTask(context.WithoutCancel(ctx), projectUUID, task.Kind, task.UUID)
	}
	service.broadcastThread(projectUUID, threadUUID, "chat:turn_cancelled", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "turn_uuid": result.UUID, "status": result.Status})
	return result, nil
}

func (service *Service) ListUserInputRequests(ctx context.Context, projectUUID, threadUUID string) ([]UserInputRequest, error) {
	var result []UserInputRequest
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		var rows []userInputRow
		if err := store.DB().WithContext(ctx).Table("chat_user_input_requests AS q").Select(`q.*,th.uuid AS thread_uuid,r.uuid AS run_uuid,t.uuid AS turn_uuid,i.uuid AS item_uuid,i.metadata_json AS item_metadata_json`).Joins("JOIN chat_threads th ON th.id=q.thread_id").Joins("JOIN chat_runs r ON r.id=q.run_id").Joins("JOIN chat_turns t ON t.id=q.turn_id").Joins("JOIN chat_items i ON i.id=q.item_id").Where("th.project_id=? AND th.uuid=?", pid, threadUUID).Order("q.created_at,q.id").Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			result = append(result, row.DTO())
		}
		return nil
	})
	return result, err
}

type userInputRow struct {
	ID, ThreadID, RunID, TurnID, ItemID                          int64
	UUID, ToolCallUUID, SchemaVersion, RequestJSON               string
	ResponseJSON                                                 *string
	Status, ProjectUUID, ThreadUUID, RunUUID, TurnUUID, ItemUUID string
	ItemMetadataJSON                                             string
	AnsweredAt, ResumedAt, CancelledAt                           *time.Time
	CreatedAt, UpdatedAt                                         time.Time
}

func (row userInputRow) DTO() UserInputRequest {
	var stored struct {
		InputType string              `json:"input_type"`
		Question  string              `json:"question"`
		Options   []UserInputOption   `json:"options"`
		Questions []UserInputQuestion `json:"questions"`
	}
	_ = json.Unmarshal([]byte(row.RequestJSON), &stored)
	var response json.RawMessage
	if row.ResponseJSON != nil {
		response = json.RawMessage(*row.ResponseJSON)
	}
	return UserInputRequest{UUID: row.UUID, ThreadUUID: row.ThreadUUID, RunUUID: row.RunUUID, TurnUUID: row.TurnUUID, ItemUUID: row.ItemUUID, ToolCallUUID: row.ToolCallUUID, SchemaVersion: row.SchemaVersion, Questions: stored.Questions, InputType: stored.InputType, Question: stored.Question, Options: stored.Options, Response: response, Status: row.Status, AnsweredAt: row.AnsweredAt, ResumedAt: row.ResumedAt, CancelledAt: row.CancelledAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func (service *Service) RespondUserInput(ctx context.Context, projectUUID, threadUUID, requestUUID string, input UserInputResponse) (UserInputRequest, error) {
	if !isUUIDv7(requestUUID) {
		return UserInputRequest{}, domainError(CodeValidation, "Request UUID 无效", "request_uuid 必须是 UUIDv7。", nil)
	}
	var result UserInputRequest
	broadcastAnswered := false
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
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
		thread, err := lockThreadSQL(ctx, tx, pid, threadUUID)
		if err != nil {
			return err
		}
		var row userInputRow
		err = tx.QueryRowContext(ctx, `SELECT q.id,q.thread_id,q.run_id,q.turn_id,q.item_id,q.uuid,q.tool_call_uuid,q.schema_version,q.request_json,q.response_json,q.status,q.answered_at,q.resumed_at,q.cancelled_at,q.created_at,q.updated_at,r.uuid,t.uuid,i.uuid,i.metadata_json FROM chat_user_input_requests q JOIN chat_runs r ON r.id=q.run_id JOIN chat_turns t ON t.id=q.turn_id JOIN chat_items i ON i.id=q.item_id WHERE q.thread_id=? AND q.uuid=?`, thread.ID, requestUUID).Scan(&row.ID, &row.ThreadID, &row.RunID, &row.TurnID, &row.ItemID, &row.UUID, &row.ToolCallUUID, &row.SchemaVersion, &row.RequestJSON, &row.ResponseJSON, &row.Status, &row.AnsweredAt, &row.ResumedAt, &row.CancelledAt, &row.CreatedAt, &row.UpdatedAt, &row.RunUUID, &row.TurnUUID, &row.ItemUUID, &row.ItemMetadataJSON)
		if err != nil {
			return notFound(err, "用户输入请求不存在")
		}
		row.ProjectUUID, row.ThreadUUID = projectUUID, threadUUID
		if row.Status == "resuming" || row.Status == "resumed" {
			response, _, validationErr := validateUserInputResponse(row, input)
			if validationErr != nil {
				return validationErr
			}
			encoded, _ := json.Marshal(response)
			if row.ResponseJSON == nil || canonicalJSON(*row.ResponseJSON) != canonicalJSON(string(encoded)) {
				return domainError(CodeStateConflict, "用户输入请求已使用其他回答", "request 已进入恢复阶段，只允许幂等重放完全相同的回答。", nil)
			}
			result = row.DTO()
			return tx.Commit()
		}
		if row.Status != "pending" {
			return domainError(CodeStateConflict, "用户输入请求不可回答", "request 已回答、取消或正在恢复。", nil)
		}
		response, toolResult, err := validateUserInputResponse(row, input)
		if err != nil {
			return err
		}
		now := service.now().UTC()
		encoded, _ := json.Marshal(response)
		encodedToolResult, _ := json.Marshal(toolResult)
		if _, err := tx.ExecContext(ctx, `UPDATE chat_user_input_requests SET response_json=?,status='resuming',answered_at=?,updated_at=? WHERE id=? AND status='pending'`, string(encoded), now, now, row.ID); err != nil {
			return err
		}
		metadata := map[string]any{"user_input_request_uuid": row.UUID}
		var providerCallID string
		_ = tx.QueryRowContext(ctx, `SELECT json_extract(arguments_json,'$.__provider_call_id') FROM agent_tool_executions WHERE run_id=? AND tool_call_uuid=?`, row.RunID, row.ToolCallUUID).Scan(&providerCallID)
		if providerCallID != "" {
			metadata["provider_call_id"] = providerCallID
		}
		item, err := appendItemTx(ctx, tx, &thread, &row.TurnID, &row.RunID, "tool_result", "tool", string(encodedToolResult), "json", "completed", row.ToolCallUUID, "request_user_input", row.UUID, metadata, now)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE agent_tool_executions SET state='completed',result_json=?,completed_at=?,updated_at=? WHERE run_id=? AND tool_call_uuid=? AND state IN ('intent','executing')`, string(encodedToolResult), now, now, row.RunID, row.ToolCallUUID); err != nil {
			return err
		}
		replay, replayCreated, err := service.prepareConfirmedRequestReplayTx(ctx, tx, &thread, row, string(encoded), now)
		if err != nil {
			return err
		}
		if _, err := appendEventTx(ctx, tx, &thread, &row.RunID, "user_input_answered", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "turn_uuid": row.TurnUUID, "run_uuid": row.RunUUID, "request_uuid": row.UUID, "item_uuid": item.UUID}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status='queued',updated_at=? WHERE id=? AND status='waiting_for_input'`, now, row.RunID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status='queued',updated_at=? WHERE id=? AND status='waiting_for_input'`, now, row.TurnID); err != nil {
			return err
		}
		jobID, err := service.queue.EnqueueAgentTx(ctx, projectUUID, tx, JobSpec{Version: 1, ProjectUUID: projectUUID, JobKind: JobChatResume, ResourceUUID: row.TurnUUID, ThreadUUID: threadUUID, WakeupUUID: row.UUID})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET river_job_id=? WHERE id=?`, jobID, row.TurnID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
			return err
		}
		if _, err := RecomputeThreadStatusTx(ctx, tx, thread.ID, now); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		responseText := string(encoded)
		row.ResponseJSON, row.Status, row.AnsweredAt, row.UpdatedAt = &responseText, "resuming", &now, now
		result = row.DTO()
		broadcastAnswered = true
		if replayCreated {
			service.broadcastThread(projectUUID, threadUUID, "chat:tool_call", map[string]any{
				"project_uuid": projectUUID, "thread_uuid": threadUUID, "turn_uuid": row.TurnUUID, "run_uuid": row.RunUUID,
				"tool_call_uuid": replay.ToolCallUUID, "tool_name": replay.ToolName, "route_id": replay.RouteID,
				"action": replay.Action, "method": replay.Method, "path": replay.Path, "target_uuid": replay.TargetUUID,
				"status": "in_progress", "runtime_generated": true, "confirmation_request_uuid": row.UUID,
			})
		}
		return nil
	})
	if err == nil && broadcastAnswered {
		service.broadcastThread(projectUUID, threadUUID, "chat:user_input_answered", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "request_uuid": requestUUID, "status": result.Status})
	}
	return result, err
}

func validateUserInputResponse(row userInputRow, input UserInputResponse) (map[string]any, map[string]any, error) {
	if row.SchemaVersion == userInputSchemaCodexQuestions {
		return validateCodexUserInputResponse(row, input)
	}
	return validateLegacyUserInputResponse(row, input)
}

func validateLegacyUserInputResponse(row userInputRow, input UserInputResponse) (map[string]any, map[string]any, error) {
	var request struct {
		InputType string            `json:"input_type"`
		Options   []UserInputOption `json:"options"`
	}
	if err := json.Unmarshal([]byte(row.RequestJSON), &request); err != nil {
		return nil, nil, domainError(CodeStateConflict, "用户输入选项损坏", "无法安全提交回答。", err)
	}
	allowed := map[string]UserInputOption{}
	for _, option := range request.Options {
		allowed[option.UUID] = option
	}
	selected := make([]UserInputOption, 0, len(input.SelectedOptionUUIDs))
	seen := map[string]struct{}{}
	for _, id := range input.SelectedOptionUUIDs {
		if !isUUIDv7(id) {
			return nil, nil, domainError(CodeValidation, "选项 UUID 无效", "selected_option_uuids 只能包含公开 UUIDv7。", nil)
		}
		option, ok := allowed[id]
		if !ok {
			return nil, nil, domainError(CodeValidation, "选项不存在", "只能提交请求中列出的选项。", nil)
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		selected = append(selected, option)
	}
	if request.InputType == "single_choice" && len(selected) > 1 {
		return nil, nil, domainError(CodeValidation, "只能选择一个选项", "single_choice 只接受一个选项。", nil)
	}
	other := strings.TrimSpace(input.OtherText)
	if len([]rune(other)) > 4000 || (len(selected) == 0 && other == "") {
		return nil, nil, domainError(CodeValidation, "回答无效", "请选择至少一个选项或填写其他说明，说明最多 4000 字符。", nil)
	}
	ids := make([]string, 0, len(selected))
	for _, option := range selected {
		ids = append(ids, option.UUID)
	}
	sort.Strings(ids)
	result := map[string]any{"selected_option_uuids": ids, "selected_options": selected, "other_text": other}
	return result, result, nil
}

func (service *Service) CancelUserInput(ctx context.Context, projectUUID, threadUUID, requestUUID string) (UserInputRequest, error) {
	requests, err := service.ListUserInputRequests(ctx, projectUUID, threadUUID)
	if err != nil {
		return UserInputRequest{}, err
	}
	for _, request := range requests {
		if request.UUID == requestUUID {
			if request.Status != "pending" {
				return UserInputRequest{}, domainError(CodeStateConflict, "请求无法取消", "只有 pending 请求可以取消。", nil)
			}
			_, err := service.Abort(ctx, projectUUID, threadUUID)
			if err != nil {
				return UserInputRequest{}, err
			}
			request.Status = "cancelled"
			now := service.now().UTC()
			request.CancelledAt, request.UpdatedAt = &now, now
			return request, nil
		}
	}
	return UserInputRequest{}, domainError(CodeNotFound, "用户输入请求不存在", "request 不属于当前 thread。", nil)
}

// keep database/sql imported for generated query adapters used above.
var _ = errors.Is
var _ *sql.Tx
var _ = fmt.Sprintf
