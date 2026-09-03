package agent

import (
	"context"
	"database/sql"
	"errors"
)

type yoloInlineOwner struct {
	ThreadID, TurnID, RunID, ToolExecutionID, ToolItemID      int64
	ThreadUUID, TurnUUID, RunUUID, ToolCallUUID, ToolItemUUID string
}

func normalizeYoloInvocation(invocation DomainInvocationContext) (DomainInvocationContext, error) {
	if invocation.Source == "" {
		invocation = DirectUIInvocationContext()
	}
	switch invocation.Source {
	case InvocationDirectUI:
		if invocation.PresentationMode != PresentationDedicatedThread || invocation.AwaitCompletion || invocation.ThreadUUID != "" || invocation.TurnUUID != "" || invocation.RunUUID != "" || invocation.ToolExecutionUUID != "" {
			return invocation, domainError(CodeValidation, "直接调用上下文无效", "direct_ui 只能创建不等待的独立 Workflow Thread。", nil)
		}
	case InvocationChatTool:
		if invocation.PresentationMode != PresentationInline || !invocation.AwaitCompletion {
			return invocation, domainError(CodeValidation, "Chat Tool 调用上下文无效", "chat_tool 必须以内联方式等待 Workflow。", nil)
		}
		for _, value := range []string{invocation.ThreadUUID, invocation.TurnUUID, invocation.RunUUID, invocation.ToolExecutionUUID} {
			if !isUUIDv7(value) {
				return invocation, domainError(CodeValidation, "Chat Tool 调用 UUID 无效", "内部 invocation 只能引用公开 UUIDv7。", nil)
			}
		}
	default:
		return invocation, domainError(CodeValidation, "自动生成调用来源无效", "自动生成流程只能由 direct_ui 或 chat_tool 创建。", nil)
	}
	return invocation, nil
}

func loadYoloInlineOwnerTx(ctx context.Context, tx *sql.Tx, projectID int64, invocation DomainInvocationContext) (yoloInlineOwner, error) {
	var owner yoloInlineOwner
	err := tx.QueryRowContext(ctx, `SELECT th.id,t.id,r.id,x.id,x.item_id,th.uuid,t.uuid,r.uuid,x.tool_call_uuid,i.uuid
		FROM chat_threads th
		JOIN chat_turns t ON t.thread_id=th.id
		JOIN chat_runs r ON r.thread_id=th.id AND r.turn_id=t.id
		JOIN agent_tool_executions x ON x.thread_id=th.id AND x.turn_id=t.id AND x.run_id=r.id
		JOIN chat_items i ON i.id=x.item_id
		WHERE th.project_id=? AND th.thread_type='conversation' AND th.uuid=? AND t.uuid=? AND r.uuid=? AND x.uuid=?
		  AND t.status='in_progress' AND r.status='in_progress' AND x.state IN ('intent','executing')`, projectID, invocation.ThreadUUID, invocation.TurnUUID, invocation.RunUUID, invocation.ToolExecutionUUID).
		Scan(&owner.ThreadID, &owner.TurnID, &owner.RunID, &owner.ToolExecutionID, &owner.ToolItemID, &owner.ThreadUUID, &owner.TurnUUID, &owner.RunUUID, &owner.ToolCallUUID, &owner.ToolItemUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return owner, domainError(CodeStateConflict, "Chat Tool 调用归属无效", "Thread、Turn、Run 与 Tool Execution 必须属于同一活动 Chat Run。", nil)
	}
	return owner, err
}
