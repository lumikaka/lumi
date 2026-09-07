package agent

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"time"

	"gorm.io/gorm"
)

// Task UUIDs are the existing workflow/task bridge. EXISTS keeps a shared
// task's requests unique even when several workflow steps reference it.
const workflowTrajectoryLogAssociation = `(
 (logs.source_type='workflow' AND logs.workflow_id=w.id) OR
 (logs.source_type='production' AND EXISTS (
   SELECT 1 FROM workflow_steps s
   JOIN production_task_runs task ON task.uuid=s.task_uuid AND task.project_id=w.project_id
   WHERE s.workflow_id=w.id AND task.id=logs.production_task_run_id
 )) OR
 (logs.source_type='story_generation' AND EXISTS (
   SELECT 1 FROM workflow_steps s
   JOIN task_runs task ON task.uuid=s.task_uuid AND task.project_id=w.project_id
   WHERE s.workflow_id=w.id AND task.id=logs.task_run_id
 ))
)`

const workflowTrajectoryLogScope = `logs.project_id = ? AND (
 (logs.source_type='project_chat' AND logs.chat_thread_id=?) OR
 EXISTS (SELECT 1 FROM workflows w WHERE w.project_id=logs.project_id AND w.thread_id=? AND ` + workflowTrajectoryLogAssociation + `)
)`

type workflowTrajectoryCall struct {
	SourceKind string    `json:"source_kind"`
	SourceUUID string    `json:"source_uuid"`
	CreatedAt  time.Time `json:"created_at"`
}

type workflowTrajectoryCursor struct {
	Version    int    `json:"version"`
	ThreadUUID string `json:"thread_uuid"`
	workflowTrajectoryCall
}

func populateWorkflowTrajectory(tx *gorm.DB, thread threadRecord, page *TrajectoryPage, before, after, selectedUUID string, limit int) error {
	workflows, err := queryTrajectoryWorkflows(tx, thread)
	if err != nil {
		return err
	}
	page.Workflows = workflows
	// Workflows are thread context, repeated on each call page. Selecting one
	// keeps the normal call window and does not consume a Request/Tool cursor.
	for _, workflow := range workflows {
		if workflow.UUID == selectedUUID {
			selectedUUID = ""
			break
		}
	}
	var modelRows []trajectoryModelRow
	err = tx.Table("llm_logs AS logs").
		Select(`logs.uuid,COALESCE(turns.uuid,'') AS turn_uuid,COALESCE(runs.uuid,'') AS run_uuid,logs.source_type,logs.request_type,logs.scenario,logs.provider_uuid,logs.provider_type,logs.model,logs.status,logs.input_summary,logs.output_summary,logs.attempt,logs.input_tokens,logs.cached_input_tokens,logs.output_tokens,logs.duration_ms,logs.finish_reason,logs.error_code,logs.error_message,logs.http_status,logs.provider_error_code,logs.provider_request_id,logs.request_payload,logs.response,logs.created_at,logs.completed_at`).
		Joins("LEFT JOIN chat_runs runs ON runs.id=logs.chat_run_id AND runs.thread_id=logs.chat_thread_id").
		Joins("LEFT JOIN chat_turns turns ON turns.id=runs.turn_id AND turns.thread_id=runs.thread_id").
		Where(workflowTrajectoryLogScope, thread.ProjectID, thread.ID, thread.ID).
		Order("logs.created_at,logs.uuid").Scan(&modelRows).Error
	if err != nil {
		return err
	}
	turns, err := queryTrajectoryTurns(tx, thread.ID)
	if err != nil {
		return err
	}
	page.Turns = turns
	page.Overview.TurnCount = int64(len(turns))
	turnStatuses := make(map[string]string, len(turns))
	for _, turn := range turns {
		turnStatuses[turn.UUID] = turn.Status
		if activeTurnStatus(turn.Status) {
			page.Overview.ActiveTurnCount++
		}
	}
	var eventRows []trajectoryEventLink
	if err := tx.Table("chat_events").Select("uuid,event_type,payload_json,sequence,created_at").Where("thread_id=?", thread.ID).Order("sequence,id").Scan(&eventRows).Error; err != nil {
		return err
	}
	events := indexTrajectoryEvents(eventRows)
	requests := projectTrajectoryModelRequests(modelRows, thread.UUID, events)
	origins, err := queryTrajectoryWorkflowOrigins(tx, thread)
	if err != nil {
		return err
	}
	requestByUUID := make(map[string]TrajectoryModelRequest, len(requests))
	calls := make([]workflowTrajectoryCall, 0, len(requests))
	for index := range requests {
		request := &requests[index]
		request.RequestOrdinal = index + 1
		request.WorkflowOrigins = origins[request.UUID]
		requestByUUID[request.UUID] = *request
		calls = append(calls, workflowTrajectoryCall{"model_request", request.UUID, request.CreatedAt})
		if request.Status == "pending" {
			page.Overview.ActiveRequestCount++
		}
	}
	toolRows, err := queryTrajectoryTools(tx, thread.ID)
	if err != nil {
		return err
	}
	tools := projectTrajectoryTools(toolRows, thread.UUID, turnStatuses, events)
	aliases := make(map[string]string, len(tools)*3)
	for index := range tools {
		tool := &tools[index]
		if request, found := requestByUUID[tool.RequestUUID]; found {
			tool.RequestOrdinal = request.RequestOrdinal
		} else {
			// A metadata UUID alone must not create a cross-thread request link.
			tool.RequestUUID, tool.RequestOrdinal = "", 0
		}
		calls = append(calls, workflowTrajectoryCall{"tool", tool.ToolCallUUID, tool.CreatedAt})
		for _, alias := range []string{tool.UUID, tool.CallItemUUID, tool.ResultItemUUID} {
			if alias != "" {
				aliases[alias] = tool.ToolCallUUID
			}
		}
		if tool.Status == "pending" || tool.Status == "running" {
			page.Overview.ActiveToolCount++
		}
	}
	sort.Slice(calls, func(left, right int) bool { return compareWorkflowTrajectoryCalls(calls[left], calls[right]) < 0 })
	if alias := aliases[selectedUUID]; alias != "" {
		selectedUUID = alias
	}
	start, end, hasMore, err := workflowTrajectoryPageBounds(calls, thread.UUID, before, after, selectedUUID, limit)
	if err != nil {
		return err
	}
	page.CursorPagination.HasMore = hasMore
	page.HistoryComplete = start == 0
	selectedRequests, selectedTools := make(map[string]bool), make(map[string]bool)
	for _, call := range calls[start:end] {
		if call.SourceKind == "model_request" {
			selectedRequests[call.SourceUUID] = true
		} else {
			selectedTools[call.SourceUUID] = true
		}
	}
	if start < end {
		page.CursorPagination.PrevCursor = encodeWorkflowTrajectoryCursor(thread.UUID, calls[start])
		page.CursorPagination.NextCursor = encodeWorkflowTrajectoryCursor(thread.UUID, calls[end-1])
	}
	for _, tool := range tools {
		if selectedTools[tool.ToolCallUUID] {
			page.Tools = append(page.Tools, tool)
			selectedRequests[tool.RequestUUID] = true
		}
	}
	for _, request := range requests {
		if selectedRequests[request.UUID] {
			page.ModelRequests = append(page.ModelRequests, request)
		}
	}
	page.Overview.ModelRequestCount = int64(len(requests))
	page.Overview.ToolCount = int64(len(tools))
	populateTrajectoryOverviewStats(&page.Overview, requests, tools)
	page.Overview.Timeline = buildTrajectoryTimeline(nil, requests, tools, nil, events)
	return nil
}

func queryTrajectoryWorkflows(tx *gorm.DB, thread threadRecord) ([]TrajectoryWorkflow, error) {
	var rows []workflowRecord
	if err := tx.Select("uuid,kind,title,status,current_step_key,error_code,started_at,completed_at,created_at,updated_at").
		Where("project_id=? AND thread_id=?", thread.ProjectID, thread.ID).
		Order("created_at,uuid").Find(&rows).Error; err != nil {
		return nil, err
	}
	workflows := make([]TrajectoryWorkflow, 0, len(rows))
	for _, row := range rows {
		workflow := TrajectoryWorkflow{
			UUID: row.UUID, ThreadUUID: thread.UUID, Kind: sanitizeDiagnosticText(row.Kind), Title: sanitizeDiagnosticText(row.Title), Status: row.Status,
			CurrentStepKey: sanitizeDiagnosticText(row.CurrentStepKey), ErrorCode: sanitizeDiagnosticText(row.ErrorCode), ErrorMessage: publicDiagnosticErrorMessage(row.ErrorCode),
			StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		if row.StartedAt != nil && row.CompletedAt != nil && !row.CompletedAt.Before(*row.StartedAt) {
			duration := row.CompletedAt.Sub(*row.StartedAt).Milliseconds()
			workflow.DurationMS = &duration
		}
		workflows = append(workflows, workflow)
	}
	return workflows, nil
}

func queryTrajectoryWorkflowOrigins(tx *gorm.DB, thread threadRecord) (map[string][]TrajectoryWorkflowOrigin, error) {
	var rows []struct{ RequestUUID, UUID, Kind, Title string }
	err := tx.Table("llm_logs AS logs").
		Select("logs.uuid AS request_uuid,w.uuid,w.kind,w.title").
		Joins("JOIN workflows w ON w.project_id=logs.project_id").
		Where("w.project_id=? AND w.thread_id=?", thread.ProjectID, thread.ID).
		Where(workflowTrajectoryLogAssociation).
		Order("logs.created_at,logs.uuid,w.created_at,w.uuid").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string][]TrajectoryWorkflowOrigin)
	for _, row := range rows {
		result[row.RequestUUID] = append(result[row.RequestUUID], TrajectoryWorkflowOrigin{
			UUID: publicUUIDOrEmpty(row.UUID), Kind: sanitizeDiagnosticText(row.Kind), Title: sanitizeDiagnosticText(row.Title),
		})
	}
	return result, nil
}

func compareWorkflowTrajectoryCalls(left, right workflowTrajectoryCall) int {
	if !left.CreatedAt.Equal(right.CreatedAt) {
		if left.CreatedAt.Before(right.CreatedAt) {
			return -1
		}
		return 1
	}
	if left.SourceKind < right.SourceKind || left.SourceKind == right.SourceKind && left.SourceUUID < right.SourceUUID {
		return -1
	}
	if left.SourceKind == right.SourceKind && left.SourceUUID == right.SourceUUID {
		return 0
	}
	return 1
}

func encodeWorkflowTrajectoryCursor(threadUUID string, call workflowTrajectoryCall) string {
	encoded, _ := json.Marshal(workflowTrajectoryCursor{Version: 1, ThreadUUID: threadUUID, workflowTrajectoryCall: call})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func workflowTrajectoryCursorIndex(calls []workflowTrajectoryCall, threadUUID, encoded string) (int, error) {
	invalid := func() (int, error) {
		return 0, domainError(CodeValidation, "Trajectory cursor 无效", "cursor 必须属于当前 Workflow Thread 的调用。", nil)
	}
	if len(encoded) > 2048 {
		return invalid()
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return invalid()
	}
	var cursor workflowTrajectoryCursor
	if json.Unmarshal(data, &cursor) != nil || cursor.Version != 1 || cursor.ThreadUUID != threadUUID || !isUUIDv7(cursor.SourceUUID) || cursor.CreatedAt.IsZero() || (cursor.SourceKind != "model_request" && cursor.SourceKind != "tool") {
		return invalid()
	}
	index := sort.Search(len(calls), func(index int) bool {
		return compareWorkflowTrajectoryCalls(calls[index], cursor.workflowTrajectoryCall) >= 0
	})
	if index == len(calls) || compareWorkflowTrajectoryCalls(calls[index], cursor.workflowTrajectoryCall) != 0 {
		return invalid()
	}
	return index, nil
}

func workflowTrajectoryPageBounds(calls []workflowTrajectoryCall, threadUUID, before, after, selectedUUID string, limit int) (int, int, bool, error) {
	start, end := 0, len(calls)
	if before != "" {
		index, err := workflowTrajectoryCursorIndex(calls, threadUUID, before)
		if err != nil {
			return 0, 0, false, err
		}
		end = index
	} else if after != "" {
		index, err := workflowTrajectoryCursorIndex(calls, threadUUID, after)
		if err != nil {
			return 0, 0, false, err
		}
		start = index + 1
	} else if selectedUUID != "" {
		found := false
		for index, call := range calls {
			if call.SourceUUID == selectedUUID {
				end, found = index+1, true
				break
			}
		}
		if !found {
			return 0, 0, false, domainError(CodeNotFound, "Trajectory Item 不存在", "item_uuid 不属于当前 Thread。", nil)
		}
	}
	hasMore := end-start > limit
	if hasMore {
		if after != "" {
			end = start + limit
		} else {
			start = end - limit
		}
	}
	return start, end, hasMore, nil
}
