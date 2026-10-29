package project

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"lumi/internal/taskexecution"
)

// InitializationState projects the existing Workflow facts. Handoff records
// acknowledge one specific unsuccessful initialization, never a future run.
type InitializationState struct {
	Status       string `json:"status"`
	Locked       bool   `json:"locked"`
	WorkflowUUID string `json:"workflow_uuid,omitempty"`
	ThreadUUID   string `json:"thread_uuid,omitempty"`
	CanTakeOver  bool   `json:"can_take_over"`
}

type initializationQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readInitializationState(ctx context.Context, db initializationQueryer, projectID int64) (InitializationState, error) {
	state := InitializationState{Status: "idle"}
	var workflowID int64
	var handedOff bool
	err := db.QueryRowContext(ctx, `SELECT w.id,w.uuid,COALESCE(t.uuid,''),w.status,
		EXISTS(SELECT 1 FROM project_initialization_handoffs h WHERE h.workflow_id=w.id)
		FROM workflows w LEFT JOIN chat_threads t ON t.id=w.thread_id
		WHERE w.project_id=? AND w.kind='yolo_project_initialization'
		ORDER BY CASE WHEN w.status IN ('queued','running') THEN 0 ELSE 1 END,w.id DESC LIMIT 1`, projectID).
		Scan(&workflowID, &state.WorkflowUUID, &state.ThreadUUID, &state.Status, &handedOff)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return InitializationState{}, err
	}
	switch state.Status {
	case "queued", "running":
		state.Locked = true
	case "failed", "cancelled", "interrupted":
		if handedOff {
			state.Status = "handed_off"
			return state, nil
		}
		state.Locked = true
		active, err := initializationHasActiveTasks(ctx, db, workflowID)
		if err != nil {
			return InitializationState{}, err
		}
		state.CanTakeOver = !active
	}
	return state, nil
}

// A cancelled parent may still have a child settling. Do not release editing
// until those persisted children have stopped producing resources.
func initializationHasActiveTasks(ctx context.Context, db initializationQueryer, workflowID int64) (bool, error) {
	var active bool
	err := db.QueryRowContext(ctx, `WITH parent_steps AS (
		SELECT s.*,w.project_id FROM workflow_steps s JOIN workflows w ON w.id=s.workflow_id WHERE w.id=?
	), related(uuid) AS (
		SELECT task_uuid FROM parent_steps WHERE task_uuid<>''
		UNION SELECT j.value FROM parent_steps s,json_each(s.output_json) j WHERE j.type='text'
		UNION SELECT j.value FROM parent_steps s,json_each(s.output_json,'$.task_uuids') j WHERE j.type='text'
		UNION SELECT t.uuid FROM task_runs t JOIN parent_steps s
			ON t.project_id=s.project_id AND t.idempotency_key=s.idempotency_key||':chapter'
		UNION SELECT t.uuid FROM production_task_runs t JOIN parent_steps s
			ON t.project_id=s.project_id AND t.idempotency_key IN (s.idempotency_key||':setting',s.idempotency_key||':breakdown',s.idempotency_key||':image')
		UNION SELECT bs.task_uuid FROM workflows b JOIN parent_steps s
			ON b.project_id=s.project_id AND b.kind='comic_image_generation_batch'
			AND json_extract(b.input_snapshot,'$.idempotency_key')=s.idempotency_key||':images'
			JOIN workflow_steps bs ON bs.workflow_id=b.id
	)
	SELECT EXISTS(SELECT 1 FROM task_runs WHERE uuid IN (SELECT uuid FROM related) AND status IN ('queued','running','waiting_for_input'))
		OR EXISTS(SELECT 1 FROM production_task_runs WHERE uuid IN (SELECT uuid FROM related) AND status IN ('queued','running'))`, workflowID).Scan(&active)
	return active, err
}

func (store *Store) InitializationState(ctx context.Context) (InitializationState, error) {
	db, err := store.db.DB()
	if err != nil {
		return InitializationState{}, err
	}
	return readInitializationState(ctx, db, store.project.ID)
}

func InitializationStateTx(ctx context.Context, tx *sql.Tx, projectID int64) (InitializationState, error) {
	return readInitializationState(ctx, tx, projectID)
}

func InitializationLockedError() error {
	return projectError(CodeProjectInitializing, "绘本初始化尚未结束", "请等待初始化完成，或停止后选择重试或手动接管。", nil)
}

func (store *Store) RequireInitializationAvailable(ctx context.Context) error {
	state, err := store.InitializationState(ctx)
	if err != nil {
		return err
	}
	if state.Locked {
		return InitializationLockedError()
	}
	return nil
}

func RequireInitializationAvailableTx(ctx context.Context, tx *sql.Tx, projectID int64) error {
	state, err := InitializationStateTx(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if state.Locked {
		return InitializationLockedError()
	}
	return nil
}

func (store *Store) TakeOverInitialization(ctx context.Context, workflowUUID string) (InitializationState, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(workflowUUID))
	if err != nil || parsed.Version() != 7 || parsed.String() != workflowUUID {
		return InitializationState{}, projectError(CodeInvalidUUID, "初始化流程 UUID 无效", "workflow_uuid 必须是规范 UUIDv7。", err)
	}
	db, err := store.db.DB()
	if err != nil {
		return InitializationState{}, err
	}
	tx, err := taskexecution.Begin(ctx, db)
	if err != nil {
		return InitializationState{}, err
	}
	defer tx.Rollback()
	state, err := InitializationStateTx(ctx, tx, store.project.ID)
	if err != nil {
		return InitializationState{}, err
	}
	if state.WorkflowUUID != workflowUUID || (state.Status != "handed_off" && !state.CanTakeOver) {
		return InitializationState{}, projectError(CodeInitializationHandoffConflict, "当前无法手动接管", "仅可接管已失败、取消或中断且子任务已结束的当前初始化。", nil)
	}
	handoffUUID, err := uuid.NewV7()
	if err != nil {
		return InitializationState{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_initialization_handoffs(uuid,workflow_id,created_at)
		SELECT ?,id,? FROM workflows WHERE uuid=? AND project_id=?
		ON CONFLICT(workflow_id) DO NOTHING`, handoffUUID.String(), time.Now().UTC(), workflowUUID, store.project.ID); err != nil {
		return InitializationState{}, err
	}
	state, err = InitializationStateTx(ctx, tx, store.project.ID)
	if err != nil {
		return InitializationState{}, err
	}
	return state, tx.Commit()
}
