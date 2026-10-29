package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"lumi/internal/agent"
	"lumi/internal/taskexecution"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type executionIdentity struct {
	Version         int    `json:"version"`
	ProjectUUID     string `json:"project_uuid"`
	ExecutionUUID   string `json:"execution_uuid"`
	TaskUUID        string `json:"task_uuid"`
	TaskKind        string `json:"task_kind"`
	MaintenanceKind string `json:"maintenance_kind"`
	JobKind         string `json:"job_kind"`
	ResourceUUID    string `json:"resource_uuid"`
	ThreadUUID      string `json:"thread_uuid"`
	WakeupUUID      string `json:"wakeup_uuid"`
}

func executionIdentityFor(job *rivertype.JobRow) (executionIdentity, error) {
	var identity executionIdentity
	if job == nil {
		return identity, taskexecution.ErrSuperseded
	}
	if err := json.Unmarshal(job.EncodedArgs, &identity); err != nil {
		return identity, err
	}
	if !isUUIDv7(identity.ProjectUUID) || (identity.Version != 1 && identity.Version != 2) ||
		(identity.Version == 2 && !isUUIDv7(identity.ExecutionUUID)) || (identity.Version == 1 && identity.ExecutionUUID != "") {
		return identity, taskexecution.ErrSuperseded
	}
	return identity, nil
}

func executionBindingFor(job *rivertype.JobRow) taskexecution.Binding {
	var identity executionIdentity
	_ = json.Unmarshal(job.EncodedArgs, &identity)
	return taskexecution.Binding{JobID: job.ID, ExecutionUUID: identity.ExecutionUUID}
}

type executionRecord struct {
	Table                                                                 string
	ID                                                                    int64
	UUID, ProjectUUID, Kind, ResourceUUID, ThreadUUID, WakeupUUID, Status string
	JobID                                                                 sql.NullInt64
	ExecutionUUID                                                         sql.NullString
}

func (record executionRecord) binding() taskexecution.Binding {
	return taskexecution.Binding{JobID: record.JobID.Int64, ExecutionUUID: record.ExecutionUUID.String}
}

// Read only Lumi-owned tables. Identity and state of a queue row are always
// obtained through River's API, including during startup and ID-reuse repair.
func (runtime *projectRuntime) executionRecordsTx(ctx context.Context, tx *sql.Tx, jobID *int64, owners ...taskexecution.Owner) ([]executionRecord, error) {
	queries := []string{
		`SELECT 'task_runs',t.id,t.uuid,p.uuid,t.kind,t.resource_uuid,'','',t.status,t.river_job_id,t.execution_uuid FROM task_runs t JOIN projects p ON p.id=t.project_id`,
		`SELECT 'production_task_runs',t.id,t.uuid,p.uuid,t.kind,t.resource_uuid,'','',t.status,t.river_job_id,t.execution_uuid FROM production_task_runs t JOIN projects p ON p.id=t.project_id`,
		`SELECT 'asset_maintenance_runs',t.id,t.uuid,p.uuid,t.kind,t.resource_uuid,'','',t.status,t.river_job_id,t.execution_uuid FROM asset_maintenance_runs t JOIN projects p ON p.id=t.project_id`,
		`SELECT 'chat_turns',t.id,t.uuid,p.uuid,'',t.uuid,th.uuid,'',t.status,t.river_job_id,t.execution_uuid FROM chat_turns t JOIN chat_threads th ON th.id=t.thread_id JOIN projects p ON p.id=th.project_id`,
		`SELECT 'workflow_steps',t.id,t.uuid,p.uuid,'workflow_step',t.uuid,COALESCE(th.uuid,''),COALESCE(
			(SELECT a.uuid FROM workflow_step_waits a WHERE a.workflow_step_id=t.id AND a.river_job_id=t.river_job_id AND COALESCE(a.execution_uuid,'')=COALESCE(t.execution_uuid,'') ORDER BY a.generation DESC LIMIT 1),
			(SELECT a.uuid FROM workflow_step_waits a WHERE a.id=t.current_wait_id AND a.status IN ('ready','consumed')),''),t.status,t.river_job_id,t.execution_uuid FROM workflow_steps t JOIN workflows w ON w.id=t.workflow_id JOIN projects p ON p.id=w.project_id LEFT JOIN chat_threads th ON th.id=w.thread_id`,
		`SELECT 'workflow_awaits',t.id,t.uuid,p.uuid,'chat_resume',ct.uuid,th.uuid,t.uuid,t.status,t.river_job_id,t.execution_uuid FROM workflow_awaits t JOIN chat_turns ct ON ct.id=t.chat_turn_id JOIN chat_threads th ON th.id=t.chat_thread_id JOIN projects p ON p.id=th.project_id`,
	}
	var records []executionRecord
	for _, query := range queries {
		if len(owners) > 0 && !strings.HasPrefix(query, "SELECT '"+owners[0].Table+"',") {
			continue
		}
		args := []any{runtime.projectUUID}
		query += ` WHERE p.uuid=?`
		if len(owners) > 0 {
			query += ` AND t.uuid=?`
			args = append(args, owners[0].UUID)
		} else if jobID == nil {
			// Historical owners with no execution need no startup work.
			query += ` AND (t.river_job_id IS NOT NULL OR t.execution_uuid IS NOT NULL OR t.status IN ('queued','running','in_progress','waiting','waiting_for_input'))`
		}
		if jobID != nil {
			query += ` AND t.river_job_id=?`
			args = append(args, *jobID)
		}
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var record executionRecord
			if err := rows.Scan(&record.Table, &record.ID, &record.UUID, &record.ProjectUUID, &record.Kind, &record.ResourceUUID, &record.ThreadUUID, &record.WakeupUUID, &record.Status, &record.JobID, &record.ExecutionUUID); err != nil {
				rows.Close()
				return nil, err
			}
			records = append(records, record)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func (record executionRecord) owns(job *rivertype.JobRow, checkBinding bool) bool {
	identity, err := executionIdentityFor(job)
	if err != nil || identity.ProjectUUID != record.ProjectUUID {
		return false
	}
	if checkBinding && (!record.JobID.Valid || record.JobID.Int64 != job.ID || record.ExecutionUUID.String != identity.ExecutionUUID) {
		return false
	}
	switch record.Table {
	case "task_runs":
		return job.Kind == (riverArgs{}).Kind() && identity.TaskUUID == record.UUID && identity.TaskKind == record.Kind && identity.ResourceUUID == record.ResourceUUID
	case "production_task_runs":
		return job.Kind == (productionArgs{}).Kind() && identity.TaskUUID == record.UUID && identity.TaskKind == record.Kind && identity.ResourceUUID == record.ResourceUUID
	case "asset_maintenance_runs":
		// Maintenance can replace resource_uuid with its completed scan UUID.
		return job.Kind == (maintenanceArgs{}).Kind() && identity.TaskUUID == record.UUID && identity.MaintenanceKind == record.Kind
	case "chat_turns":
		return job.Kind == (agentArgs{}).Kind() && (identity.JobKind == agent.JobChatTurn || identity.JobKind == agent.JobChatResume) && identity.ResourceUUID == record.UUID && identity.ThreadUUID == record.ThreadUUID
	case "workflow_steps":
		return job.Kind == (agentArgs{}).Kind() && identity.JobKind == agent.JobWorkflowStep && identity.ResourceUUID == record.UUID && identity.ThreadUUID == record.ThreadUUID && identity.WakeupUUID == record.WakeupUUID
	case "workflow_awaits":
		return job.Kind == (agentArgs{}).Kind() && identity.JobKind == agent.JobChatResume && identity.ResourceUUID == record.ResourceUUID && identity.ThreadUUID == record.ThreadUUID && identity.WakeupUUID == record.UUID
	}
	return false
}

func riverTerminal(job *rivertype.JobRow) bool {
	return job.State == rivertype.JobStateCompleted || job.State == rivertype.JobStateCancelled || job.State == rivertype.JobStateDiscarded
}

func clearExecutionRecordTx(ctx context.Context, tx *sql.Tx, record executionRecord) error {
	if !taskexecution.ValidTable(record.Table) {
		return taskexecution.ErrSuperseded
	}
	_, err := tx.ExecContext(ctx, `UPDATE `+record.Table+` SET river_job_id=NULL,execution_uuid=NULL WHERE id=? AND river_job_id IS ? AND execution_uuid IS ?`, record.ID, record.JobID, record.ExecutionUUID)
	return err
}

func (runtime *projectRuntime) clearReusedBindingsTx(ctx context.Context, tx *sql.Tx, job *rivertype.JobRow) error {
	records, err := runtime.executionRecordsTx(ctx, tx, &job.ID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.owns(job, true) {
			continue
		}
		if err := clearExecutionRecordTx(ctx, tx, record); err != nil {
			return err
		}
		slog.InfoContext(ctx, "releasing stale queue binding in transaction", "project_uuid", runtime.projectUUID, "task_uuid", record.UUID, "owner_type", record.Table, "reason", "job_id_reused")
	}
	return nil
}

// insertExecutionTx is the only insertion path for business-owned executions.
// River's duplicate result is authoritative: never bind our proposed UUID to
// an existing job carrying a different execution UUID.
func (runtime *projectRuntime) insertExecutionTx(ctx context.Context, tx *sql.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	executionUUID, err := newUUIDv7()
	if err != nil {
		return nil, err
	}
	switch value := args.(type) {
	case riverArgs:
		value.Version = 2
		value.ExecutionUUID = executionUUID
		args = value
	case productionArgs:
		value.Version = 2
		value.ExecutionUUID = executionUUID
		args = value
	case maintenanceArgs:
		value.Version = 2
		value.ExecutionUUID = executionUUID
		args = value
	case agentArgs:
		value.Version = 2
		value.ExecutionUUID = executionUUID
		args = value
	default:
		return nil, fmt.Errorf("unsupported business execution args %T", args)
	}
	result, err := runtime.client.InsertTx(ctx, tx, args, opts)
	if err != nil {
		return nil, err
	}
	if result.Job == nil {
		return nil, taskexecution.ErrSuperseded
	}
	if !result.UniqueSkippedAsDuplicate {
		if err := runtime.clearReusedBindingsTx(ctx, tx, result.Job); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (runtime *projectRuntime) executionContext(ctx context.Context, job *rivertype.JobRow) (context.Context, error) {
	identity, err := executionIdentityFor(job)
	if err != nil || identity.ProjectUUID != runtime.projectUUID {
		return ctx, taskexecution.ErrSuperseded
	}
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return ctx, err
	}
	defer tx.Rollback()
	records, err := runtime.executionRecordsTx(ctx, tx, &job.ID)
	if err != nil {
		return ctx, err
	}
	for _, record := range records {
		if record.Table == "workflow_awaits" || !record.owns(job, true) {
			continue
		}
		return taskexecution.WithFence(ctx, taskexecution.Fence{Owner: taskexecution.Owner{Table: record.Table, UUID: record.UUID, ProjectUUID: record.ProjectUUID}, Binding: record.binding()}), nil
	}
	return ctx, taskexecution.ErrSuperseded
}

func executionWorkKey(uuid string, binding taskexecution.Binding) string {
	return fmt.Sprintf("%s/%d/%s", uuid, binding.JobID, binding.ExecutionUUID)
}

func (runtime *projectRuntime) registerExecutionWork(ctx context.Context, cancel context.CancelFunc) func() {
	fence, _ := taskexecution.FromContext(ctx)
	key := executionWorkKey(fence.UUID, fence.Binding)
	runtime.registerWork(key, cancel)
	return func() { runtime.unregisterWork(key) }
}

func (runtime *projectRuntime) cancelExecutionWork(uuid string, binding taskexecution.Binding) {
	runtime.cancelWork(executionWorkKey(uuid, binding))
}

func activeProductStatus(status string) bool {
	return status == StatusQueued || status == StatusRunning || status == StatusWaitingForInput
}

func (runtime *projectRuntime) jobForBindingTx(ctx context.Context, tx *sql.Tx, record executionRecord) (*rivertype.JobRow, error) {
	if !record.JobID.Valid {
		return nil, nil
	}
	job, err := runtime.client.JobGetTx(ctx, tx, record.JobID.Int64)
	if errors.Is(err, rivertype.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !record.owns(job, true) {
		return nil, nil
	}
	return job, nil
}

func (runtime *projectRuntime) findExecutionRecordTx(ctx context.Context, tx *sql.Tx, table, uuid string) (executionRecord, error) {
	records, err := runtime.executionRecordsTx(ctx, tx, nil, taskexecution.Owner{Table: table, UUID: uuid})
	if err != nil {
		return executionRecord{}, err
	}
	for _, record := range records {
		if record.Table == table && record.UUID == uuid {
			return record, nil
		}
	}
	return executionRecord{}, sql.ErrNoRows
}

func (runtime *projectRuntime) cancelOwnedJobTx(ctx context.Context, tx *sql.Tx, table, uuid string) error {
	record, err := runtime.findExecutionRecordTx(ctx, tx, table, uuid)
	if err != nil {
		return err
	}
	job, err := runtime.jobForBindingTx(ctx, tx, record)
	if err != nil {
		return err
	}
	if job == nil {
		return clearExecutionRecordTx(ctx, tx, record)
	}
	runtime.cancelExecutionWork(uuid, record.binding())
	if riverTerminal(job) {
		if err := runtime.repairTerminalProductTx(ctx, tx, record, job); err != nil {
			return err
		}
		return clearExecutionRecordTx(ctx, tx, record)
	} else {
		cancelled, err := runtime.client.JobCancelTx(ctx, tx, job.ID)
		if err != nil {
			return err
		}
		if riverTerminal(cancelled) {
			// The caller persists its cancellation projection in this transaction.
			// A queued job can become terminal here, even before subscriptions start.
			return clearExecutionRecordTx(ctx, tx, record)
		}
	}
	return nil
}

func (runtime *projectRuntime) releaseTerminalExecution(ctx context.Context, job *rivertype.JobRow) error {
	if !riverTerminal(job) {
		return nil
	}
	tx, err := runtime.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	records, err := runtime.executionRecordsTx(ctx, tx, &job.ID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if !record.owns(job, true) {
			continue
		}
		// Validate current River state, not just a possibly delayed event.
		current, err := runtime.jobForBindingTx(ctx, tx, record)
		if err != nil {
			return err
		}
		if current != nil && !riverTerminal(current) {
			continue
		}
		if err := runtime.repairTerminalProductTx(ctx, tx, record, job); err != nil {
			return err
		}
		if err := clearExecutionRecordTx(ctx, tx, record); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (runtime *projectRuntime) handleExecutionEvent(ctx context.Context, event *river.Event) error {
	if event == nil || event.Job == nil || !isBusinessJob(event.Job.Kind) {
		return nil
	}
	current, err := runtime.client.JobGet(ctx, event.Job.ID)
	if err != nil && !errors.Is(err, rivertype.ErrNotFound) {
		return err
	}
	if current != nil && (executionBindingFor(current) != executionBindingFor(event.Job) || current.Attempt != event.Job.Attempt || current.State != event.Job.State) {
		return nil
	}
	guardedCtx, err := runtime.executionContext(ctx, event.Job)
	if errors.Is(err, taskexecution.ErrSuperseded) {
		// A resume may already have replaced the canonical Turn binding while
		// an Await still holds the previous execution. Release only matching
		// references; never project the previous execution onto its successor.
		return runtime.releaseTerminalExecution(ctx, event.Job)
	}
	if err != nil {
		return err
	}
	if err := runtime.projectRiverEvent(guardedCtx, event); err != nil {
		if errors.Is(err, taskexecution.ErrSuperseded) {
			return nil
		}
		return err
	}
	return runtime.releaseTerminalExecution(ctx, event.Job)
}

func isBusinessJob(kind string) bool {
	return strings.HasPrefix(kind, "lumi_") && (kind == (riverArgs{}).Kind() || kind == (productionArgs{}).Kind() || kind == (maintenanceArgs{}).Kind() || kind == (agentArgs{}).Kind())
}
