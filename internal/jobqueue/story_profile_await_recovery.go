package jobqueue

import (
	"context"
	"database/sql"
	"time"

	"lumi/internal/agent"
)

// Older versions suspended Chat after creating a profile task without creating
// its Workflow/await. Reattach only an existing task with the exact persisted
// tool idempotency key and an active owner; never replay the generation itself.
// The caller reconciles task terminal states before Agent reconciliation wakes
// the repaired await. All writes share that reconciliation transaction.
func repairMissingStoryProfileAwaitsTx(ctx context.Context, tx *sql.Tx, projectID int64, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT p.uuid,t.kind,t.resource_uuid,t.uuid,t.provider_uuid,t.model,t.model_source,t.input_snapshot,
		th.uuid,turns.uuid,r.uuid,x.uuid
		FROM task_runs t
		JOIN projects p ON p.id=t.project_id
		JOIN agent_tool_executions x ON x.idempotency_key=t.idempotency_key
		JOIN chat_threads th ON th.id=x.thread_id AND th.project_id=t.project_id
		JOIN chat_turns turns ON turns.id=x.turn_id AND turns.thread_id=th.id
		JOIN chat_runs r ON r.id=x.run_id AND r.turn_id=turns.id AND r.thread_id=th.id
		WHERE t.project_id=? AND t.kind IN (?,?) AND th.thread_type='conversation'
		AND x.tool_name='request_api' AND x.state='executing'
		AND r.status IN ('in_progress','queued') AND turns.status IN ('in_progress','queued')
		AND r.cancel_requested_at IS NULL AND turns.cancel_requested_at IS NULL
		AND NOT EXISTS(SELECT 1 FROM workflow_steps s WHERE s.task_uuid=t.uuid)
		AND NOT EXISTS(SELECT 1 FROM workflow_awaits a WHERE a.tool_execution_id=x.id)`,
		projectID, KindStoryProfileGeneration, KindStoryProfileFromChapters)
	if err != nil {
		return err
	}
	type missingAwait struct {
		projectUUID, kind, resourceUUID, taskUUID, providerUUID, model, modelSource, snapshot string
		invocation                                                                            agent.DomainInvocationContext
	}
	var missing []missingAwait
	for rows.Next() {
		var item missingAwait
		item.invocation = agent.DomainInvocationContext{Source: agent.InvocationChatTool, PresentationMode: agent.PresentationInline, AwaitCompletion: true}
		if err := rows.Scan(&item.projectUUID, &item.kind, &item.resourceUUID, &item.taskUUID, &item.providerUUID, &item.model, &item.modelSource, &item.snapshot,
			&item.invocation.ThreadUUID, &item.invocation.TurnUUID, &item.invocation.RunUUID, &item.invocation.ToolExecutionUUID); err != nil {
			rows.Close()
			return err
		}
		missing = append(missing, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range missing {
		if err := createStoryTaskWorkflowTx(ctx, tx, projectID, item.projectUUID, item.kind, item.resourceUUID, item.taskUUID,
			item.providerUUID, item.model, item.modelSource, []byte(item.snapshot), item.invocation, now); err != nil {
			return err
		}
	}
	return nil
}

func (manager *Manager) awaitStoryDomainTask(ctx context.Context, projectUUID string, task Task, invocation agent.DomainInvocationContext) (agent.DomainTask, error) {
	return manager.awaitDomainTask(ctx, projectUUID, storyDomainTask(task), invocation)
}
