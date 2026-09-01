package agent

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"lumi/internal/project"

	"gorm.io/gorm"
)

const DefaultTrajectoryPage = 80

type TrajectoryPage struct {
	Thread           Thread                   `json:"thread"`
	Turns            []TrajectoryTurn         `json:"turns"`
	Items            []TrajectorySourceItem   `json:"items"`
	Tools            []TrajectoryTool         `json:"tools"`
	ModelRequests    []TrajectoryModelRequest `json:"model_requests"`
	Compactions      []TrajectoryCompaction   `json:"compactions"`
	CursorPagination CursorPagination         `json:"cursor_pagination"`
	HistoryComplete  bool                     `json:"history_complete"`
	Overview         TrajectoryOverview       `json:"overview"`
}

type TrajectoryTurn struct {
	UUID               string     `json:"uuid"`
	QueueSequence      int64      `json:"queue_sequence"`
	SourceType         string     `json:"source_type"`
	SourceFollowUpUUID string     `json:"source_follow_up_uuid,omitempty"`
	Status             string     `json:"status"`
	ErrorCode          string     `json:"error_code,omitempty"`
	ErrorMessage       string     `json:"error_message,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type TrajectorySourceItem struct {
	UUID             string          `json:"uuid"`
	ThreadUUID       string          `json:"thread_uuid"`
	TurnUUID         string          `json:"turn_uuid,omitempty"`
	Sequence         int64           `json:"sequence"`
	EventSequence    *int64          `json:"event_sequence,omitempty"`
	ItemType         string          `json:"item_type"`
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	ContentFormat    string          `json:"content_format"`
	Status           string          `json:"status"`
	ToolCallUUID     string          `json:"tool_call_uuid,omitempty"`
	ToolName         string          `json:"tool_name,omitempty"`
	TargetUUID       string          `json:"target_uuid,omitempty"`
	RequestUUID      string          `json:"request_uuid,omitempty"`
	RequestOrdinal   int             `json:"request_ordinal,omitempty"`
	Metadata         json.RawMessage `json:"metadata"`
	OrderingAccuracy string          `json:"ordering_accuracy"`
	CreatedAt        time.Time       `json:"created_at"`
}

type TrajectoryTool struct {
	UUID               string          `json:"uuid"`
	ThreadUUID         string          `json:"thread_uuid"`
	TurnUUID           string          `json:"turn_uuid"`
	CallItemUUID       string          `json:"call_item_uuid"`
	ResultItemUUID     string          `json:"result_item_uuid,omitempty"`
	ToolCallUUID       string          `json:"tool_call_uuid"`
	ToolName           string          `json:"tool_name"`
	TargetUUID         string          `json:"target_uuid,omitempty"`
	RequestUUID        string          `json:"request_uuid,omitempty"`
	RequestOrdinal     int             `json:"request_ordinal,omitempty"`
	CallSequence       int64           `json:"call_sequence"`
	ResultSequence     *int64          `json:"result_sequence,omitempty"`
	StartEventSequence *int64          `json:"start_event_sequence,omitempty"`
	EndEventSequence   *int64          `json:"end_event_sequence,omitempty"`
	Status             string          `json:"status"`
	DerivedReason      string          `json:"derived_reason,omitempty"`
	Arguments          json.RawMessage `json:"arguments"`
	Result             json.RawMessage `json:"result,omitempty"`
	ErrorCode          string          `json:"error_code,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	StartedAt          *time.Time      `json:"started_at,omitempty"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
	DurationMS         *int64          `json:"duration_ms,omitempty"`
	OrderingAccuracy   string          `json:"ordering_accuracy"`
	CreatedAt          time.Time       `json:"created_at"`
}

type TrajectoryModelRequest struct {
	UUID               string          `json:"uuid"`
	ThreadUUID         string          `json:"thread_uuid"`
	TurnUUID           string          `json:"turn_uuid"`
	RunUUID            string          `json:"run_uuid"`
	RequestOrdinal     int             `json:"request_ordinal"`
	RequestType        string          `json:"request_type"`
	Scenario           string          `json:"scenario"`
	ProviderUUID       string          `json:"provider_uuid"`
	ProviderType       string          `json:"provider_type"`
	Model              string          `json:"model"`
	Status             string          `json:"status"`
	Options            json.RawMessage `json:"options"`
	InputSummary       string          `json:"input_summary,omitempty"`
	OutputSummary      string          `json:"output_summary,omitempty"`
	AssistantPreview   string          `json:"assistant_preview,omitempty"`
	HasToolCalls       bool            `json:"has_tool_calls"`
	InputTokens        *int            `json:"input_tokens,omitempty"`
	CachedInputTokens  *int            `json:"cached_input_tokens,omitempty"`
	OutputTokens       *int            `json:"output_tokens,omitempty"`
	DurationMS         *int64          `json:"duration_ms,omitempty"`
	FinishReason       string          `json:"finish_reason,omitempty"`
	ErrorCode          string          `json:"error_code,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	HTTPStatus         *int            `json:"http_status,omitempty"`
	ProviderErrorCode  string          `json:"provider_error_code,omitempty"`
	ProviderRequestID  string          `json:"provider_request_id,omitempty"`
	SystemPromptDigest string          `json:"system_prompt_digest,omitempty"`
	ToolCatalogDigest  string          `json:"tool_catalog_digest,omitempty"`
	HasRequestPayload  bool            `json:"has_request_payload"`
	HasResponse        bool            `json:"has_response"`
	StartEventSequence *int64          `json:"start_event_sequence,omitempty"`
	EndEventSequence   *int64          `json:"end_event_sequence,omitempty"`
	OrderingAccuracy   string          `json:"ordering_accuracy"`
	UsageAccuracy      string          `json:"usage_accuracy"`
	CreatedAt          time.Time       `json:"created_at"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
}

type TrajectoryCompaction struct {
	UUID                string    `json:"uuid"`
	ThreadUUID          string    `json:"thread_uuid"`
	TurnUUID            *string   `json:"turn_uuid"`
	ThroughItemSequence int64     `json:"through_item_sequence"`
	EventSequence       *int64    `json:"event_sequence,omitempty"`
	Summary             string    `json:"summary"`
	SourceBytes         int       `json:"source_bytes"`
	OrderingAccuracy    string    `json:"ordering_accuracy"`
	CreatedAt           time.Time `json:"created_at"`
}

type TrajectoryOverview struct {
	TurnCount               int64                     `json:"turn_count"`
	ItemCount               int64                     `json:"item_count"`
	ModelRequestCount       int64                     `json:"model_request_count"`
	ToolCount               int64                     `json:"tool_count"`
	CompactionCount         int64                     `json:"compaction_count"`
	ActiveTurnCount         int64                     `json:"active_turn_count"`
	ActiveRequestCount      int64                     `json:"active_request_count"`
	ActiveToolCount         int64                     `json:"active_tool_count"`
	LLMDurationMS           *int64                    `json:"llm_duration_ms,omitempty"`
	ToolDurationMS          *int64                    `json:"tool_duration_ms,omitempty"`
	ToolExecutionDurationMS *int64                    `json:"tool_execution_duration_ms,omitempty"`
	UserWaitDurationMS      *int64                    `json:"user_wait_duration_ms,omitempty"`
	AverageTTFTMS           *int64                    `json:"average_ttft_ms,omitempty"`
	OutputTokensPerSec      *float64                  `json:"output_tokens_per_second,omitempty"`
	InputTokens             *int64                    `json:"input_tokens,omitempty"`
	CachedInputTokens       *int64                    `json:"cached_input_tokens,omitempty"`
	OutputTokens            *int64                    `json:"output_tokens,omitempty"`
	CacheHitPercent         *int                      `json:"cache_hit_percent,omitempty"`
	Timeline                []TrajectoryTimelineEntry `json:"timeline"`
}

type TrajectoryTimelineEntry struct {
	UUID             string     `json:"uuid"`
	SourceKind       string     `json:"source_kind"`
	Kind             string     `json:"kind"`
	TurnUUID         string     `json:"turn_uuid,omitempty"`
	ItemSequence     *int64     `json:"item_sequence,omitempty"`
	EventSequence    *int64     `json:"event_sequence,omitempty"`
	RequestUUID      string     `json:"request_uuid,omitempty"`
	RequestOrdinal   int        `json:"request_ordinal,omitempty"`
	ToolCallUUID     string     `json:"tool_call_uuid,omitempty"`
	Status           string     `json:"status"`
	Preview          string     `json:"preview"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	DurationMS       *int64     `json:"duration_ms,omitempty"`
	OrderingAccuracy string     `json:"ordering_accuracy"`
}

type trajectoryEventLink struct {
	UUID, EventType, PayloadJSON string
	Sequence                     int64
	CreatedAt                    time.Time
}

type trajectoryEventIndex struct {
	item           map[string]int64
	turnStart      map[string]int64
	toolStart      map[string]int64
	toolEnd        map[string]int64
	requestStart   map[string]int64
	requestEnd     map[string]int64
	compaction     map[string]int64
	compactionTurn map[string]string
}

type trajectoryItemRow struct {
	ID, Sequence, TurnID, RunID                                             int64
	UUID, TurnUUID, RunUUID, ItemType, Role, Content, ContentFormat, Status string
	RemoteItemUUID, ToolName, TargetUUID, MetadataJSON                      string
	CreatedAt                                                               time.Time
}

type trajectoryToolRow struct {
	ID, CallSequence                                                                 int64
	ResultSequence                                                                   sql.NullInt64
	UUID, TurnUUID, CallItemUUID, ResultItemUUID, ToolCallUUID, ToolName, TargetUUID string
	ArgumentsJSON, State                                                             string
	ResultJSON                                                                       sql.NullString
	ErrorCode, ErrorMessage, CallMetadata                                            string
	StartedAt, CompletedAt                                                           *time.Time
	CreatedAt                                                                        time.Time
}

type trajectoryModelRow struct {
	UUID, TurnUUID, RunUUID, ProviderUUID, ProviderType, Model, Status string
	RequestType, Scenario                                              string
	InputSummary, OutputSummary, FinishReason, ErrorCode, ErrorMessage string
	ProviderErrorCode, ProviderRequestID                               string
	Attempt, InputTokens, OutputTokens, HTTPStatus                     int
	CachedInputTokens                                                  *int
	DurationMS                                                         int64
	RequestPayload, Response                                           sql.NullString
	CreatedAt                                                          time.Time
	CompletedAt                                                        *time.Time
}

type trajectoryCompactionRow struct {
	UUID                string
	ThroughItemSequence int64
	Summary             string
	SourceBytes         int
	CreatedAt           time.Time
}

type trajectoryCompactItemRow struct {
	UUID, TurnUUID, ItemType, Status, Content string
	Sequence                                  int64
	CreatedAt                                 time.Time
}

func (service *Service) ListTrajectory(ctx context.Context, projectUUID, threadUUID, before, after, selectedItemUUID string, limit int) (TrajectoryPage, error) {
	if !isUUIDv7(threadUUID) {
		return TrajectoryPage{}, domainError(CodeValidation, "Thread UUID 无效", "thread_uuid 必须是 UUIDv7。", nil)
	}
	if limit <= 0 {
		limit = DefaultTrajectoryPage
	}
	if limit > 200 {
		limit = 200
	}
	beforeSequence, err := decodeCursor(before)
	if err != nil {
		return TrajectoryPage{}, err
	}
	afterSequence, err := decodeCursor(after)
	if err != nil {
		return TrajectoryPage{}, err
	}
	selectedItemUUID = strings.TrimSpace(selectedItemUUID)
	if beforeSequence > 0 && afterSequence > 0 || selectedItemUUID != "" && (beforeSequence > 0 || afterSequence > 0) {
		return TrajectoryPage{}, domainError(CodeValidation, "Trajectory cursor 参数冲突", "before、after 与 item_uuid 只能使用一种定位方式。", nil)
	}
	if selectedItemUUID != "" && !isUUIDv7(selectedItemUUID) {
		return TrajectoryPage{}, domainError(CodeValidation, "Trajectory Item UUID 无效", "item_uuid 必须是公开 UUIDv7。", nil)
	}

	page := TrajectoryPage{
		Turns: []TrajectoryTurn{}, Items: []TrajectorySourceItem{}, Tools: []TrajectoryTool{},
		ModelRequests: []TrajectoryModelRequest{}, Compactions: []TrajectoryCompaction{},
		CursorPagination: CursorPagination{PerPage: limit},
		Overview:         TrajectoryOverview{Timeline: []TrajectoryTimelineEntry{}},
	}
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		return store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			pid, err := projectID(ctx, tx, projectUUID)
			if err != nil {
				return err
			}
			var thread threadRecord
			if err := tx.Where("project_id=? AND uuid=?", pid, threadUUID).First(&thread).Error; err != nil {
				return notFound(err, "Chat thread 不存在")
			}
			page.Thread = threadDTO(thread, projectUUID)
			page.Thread.Title = sanitizeDiagnosticText(page.Thread.Title)
			page.Thread.Model = sanitizeDiagnosticText(page.Thread.Model)

			anchorSequence := int64(0)
			if selectedItemUUID != "" {
				anchorSequence, err = resolveTrajectoryAnchor(ctx, tx, thread.ID, selectedItemUUID)
				if err != nil {
					return err
				}
			}

			var stats struct{ ItemCount, MinSequence, MaxSequence int64 }
			if err := tx.Table("chat_items").Select("COUNT(*) AS item_count,COALESCE(MIN(sequence),0) AS min_sequence,COALESCE(MAX(sequence),0) AS max_sequence").Where("thread_id=?", thread.ID).Scan(&stats).Error; err != nil {
				return err
			}
			page.Overview.ItemCount = stats.ItemCount

			itemRows, hasMore, descending, err := queryTrajectoryItems(tx, thread.ID, beforeSequence, afterSequence, anchorSequence, limit)
			if err != nil {
				return err
			}
			page.CursorPagination.HasMore = hasMore
			if len(itemRows) > 0 {
				page.CursorPagination.PrevCursor = encodeCursor(itemRows[0].Sequence)
				page.CursorPagination.NextCursor = encodeCursor(itemRows[len(itemRows)-1].Sequence)
				page.HistoryComplete = itemRows[0].Sequence <= stats.MinSequence
			} else {
				page.HistoryComplete = stats.ItemCount == 0 || afterSequence == 0 && beforeSequence == 0
			}
			_ = descending

			turnRows, err := queryTrajectoryTurns(tx, thread.ID)
			if err != nil {
				return err
			}
			page.Turns = turnRows
			page.Overview.TurnCount = int64(len(turnRows))
			turnStatuses := make(map[string]string, len(turnRows))
			for _, turn := range turnRows {
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

			var modelRows []trajectoryModelRow
			if err := tx.Table("llm_logs AS logs").Select(`logs.uuid,turns.uuid AS turn_uuid,runs.uuid AS run_uuid,logs.request_type,logs.scenario,logs.provider_uuid,logs.provider_type,logs.model,logs.status,logs.input_summary,logs.output_summary,logs.attempt,logs.input_tokens,logs.cached_input_tokens,logs.output_tokens,logs.duration_ms,logs.finish_reason,logs.error_code,logs.error_message,logs.http_status,logs.provider_error_code,logs.provider_request_id,logs.request_payload,logs.response,logs.created_at,logs.completed_at`).Joins("JOIN chat_runs runs ON runs.id=logs.chat_run_id").Joins("JOIN chat_turns turns ON turns.id=runs.turn_id").Where("logs.chat_thread_id=? AND logs.source_type='project_chat'", thread.ID).Order("logs.created_at,logs.id").Scan(&modelRows).Error; err != nil {
				return err
			}

			var toolRows []trajectoryToolRow
			if err := tx.Table("agent_tool_executions AS executions").Select(`executions.id,executions.uuid,turns.uuid AS turn_uuid,calls.uuid AS call_item_uuid,COALESCE(results.uuid,'') AS result_item_uuid,executions.tool_call_uuid,executions.tool_name,executions.target_uuid,calls.sequence AS call_sequence,results.sequence AS result_sequence,executions.arguments_json,executions.state,executions.result_json,executions.error_code,executions.error_message,calls.metadata_json AS call_metadata,executions.started_at,executions.completed_at,executions.created_at`).Joins("JOIN chat_turns turns ON turns.id=executions.turn_id").Joins("JOIN chat_items calls ON calls.id=executions.item_id").Joins(`LEFT JOIN chat_items results ON results.id=(SELECT result_item.id FROM chat_items result_item WHERE result_item.thread_id=executions.thread_id AND result_item.remote_item_uuid=executions.tool_call_uuid AND result_item.item_type='tool_result' ORDER BY result_item.sequence DESC,result_item.id DESC LIMIT 1)`).Where("executions.thread_id=?", thread.ID).Order("calls.sequence,executions.id").Scan(&toolRows).Error; err != nil {
				return err
			}

			var compactionRows []trajectoryCompactionRow
			if err := tx.Table("agent_context_summaries").Select("uuid,through_item_sequence,summary,source_bytes,created_at").Where("thread_id=?", thread.ID).Order("through_item_sequence,id").Scan(&compactionRows).Error; err != nil {
				return err
			}

			var compactItems []trajectoryCompactItemRow
			if err := tx.Table("chat_items AS items").Select("items.uuid,COALESCE(turns.uuid,'') AS turn_uuid,items.sequence,items.item_type,items.status,substr(items.content,1,240) AS content,items.created_at").Joins("LEFT JOIN chat_turns turns ON turns.id=items.turn_id").Where("items.thread_id=?", thread.ID).Order("items.sequence,items.id").Scan(&compactItems).Error; err != nil {
				return err
			}

			low, high := trajectoryPageBounds(itemRows)
			page.Items = projectTrajectoryItems(itemRows, thread.UUID, events)
			allRequests := projectTrajectoryModelRequests(modelRows, thread.UUID, events)
			page.Overview.ModelRequestCount = int64(len(allRequests))
			for _, request := range allRequests {
				if request.Status == "pending" {
					page.Overview.ActiveRequestCount++
				}
				if trajectoryTurnOnPage(request.TurnUUID, itemRows) || request.UUID == selectedItemUUID {
					page.ModelRequests = append(page.ModelRequests, request)
				}
			}
			allTools := projectTrajectoryTools(toolRows, thread.UUID, turnStatuses, events)
			page.Overview.ToolCount = int64(len(allTools))
			for _, tool := range allTools {
				if tool.Status == "pending" || tool.Status == "running" {
					page.Overview.ActiveToolCount++
				}
				if trajectorySequenceOnPage(tool.CallSequence, low, high) || tool.ResultSequence != nil && trajectorySequenceOnPage(*tool.ResultSequence, low, high) || tool.ToolCallUUID == selectedItemUUID || tool.UUID == selectedItemUUID {
					page.Tools = append(page.Tools, tool)
				}
			}
			populateTrajectoryOverviewStats(&page.Overview, allRequests, allTools)
			allCompactions := projectTrajectoryCompactions(compactionRows, thread.UUID, events)
			page.Overview.CompactionCount = int64(len(allCompactions))
			for _, compaction := range allCompactions {
				if trajectorySequenceOnPage(compaction.ThroughItemSequence, low, high) || compaction.UUID == selectedItemUUID {
					page.Compactions = append(page.Compactions, compaction)
				}
			}
			page.Overview.Timeline = buildTrajectoryTimeline(compactItems, allRequests, allTools, allCompactions, events)
			return nil
		})
	})
	return page, err
}

func populateTrajectoryOverviewStats(overview *TrajectoryOverview, requests []TrajectoryModelRequest, tools []TrajectoryTool) {
	if overview == nil {
		return
	}
	if len(requests) > 0 {
		var llmDuration, inputTokens, cachedInputTokens, outputTokens int64
		durationRecorded, usageRecorded, cacheRecorded := true, true, true
		for _, request := range requests {
			if request.DurationMS == nil {
				durationRecorded = false
			} else {
				llmDuration += *request.DurationMS
			}
			if request.UsageAccuracy != "recorded" || request.InputTokens == nil || request.OutputTokens == nil {
				usageRecorded = false
			} else {
				inputTokens += int64(*request.InputTokens)
				outputTokens += int64(*request.OutputTokens)
			}
			if request.CachedInputTokens == nil {
				cacheRecorded = false
			} else {
				cachedInputTokens += int64(*request.CachedInputTokens)
			}
		}
		if durationRecorded {
			overview.LLMDurationMS = int64Pointer(llmDuration)
		}
		if usageRecorded {
			overview.InputTokens = int64Pointer(inputTokens)
			overview.OutputTokens = int64Pointer(outputTokens)
			if cacheRecorded {
				overview.CachedInputTokens = int64Pointer(cachedInputTokens)
				if inputTokens > 0 {
					overview.CacheHitPercent = intPointer(int(math.Round(float64(cachedInputTokens) / float64(inputTokens) * 100)))
				}
			}
		}
	}
	if len(tools) > 0 {
		var toolDuration, toolExecutionDuration, userWaitDuration int64
		durationRecorded, toolExecutionRecorded, userWaitRecorded := true, true, true
		var toolExecutionCount, userWaitCount int
		for _, tool := range tools {
			isUserWait := tool.ToolName == "request_user_input"
			if isUserWait {
				userWaitCount++
			} else {
				toolExecutionCount++
			}
			if tool.DurationMS == nil {
				durationRecorded = false
				if isUserWait {
					userWaitRecorded = false
				} else {
					toolExecutionRecorded = false
				}
				continue
			}
			toolDuration += *tool.DurationMS
			if isUserWait {
				userWaitDuration += *tool.DurationMS
			} else {
				toolExecutionDuration += *tool.DurationMS
			}
		}
		if durationRecorded {
			overview.ToolDurationMS = int64Pointer(toolDuration)
		}
		if toolExecutionCount > 0 && toolExecutionRecorded {
			overview.ToolExecutionDurationMS = int64Pointer(toolExecutionDuration)
		}
		if userWaitCount > 0 && userWaitRecorded {
			overview.UserWaitDurationMS = int64Pointer(userWaitDuration)
		}
	}
}

func resolveTrajectoryAnchor(ctx context.Context, tx *gorm.DB, threadID int64, itemUUID string) (int64, error) {
	var anchor sql.NullInt64
	query := `SELECT COALESCE(
  (SELECT sequence FROM chat_items WHERE thread_id=? AND uuid=? LIMIT 1),
  (SELECT MIN(sequence) FROM chat_items WHERE thread_id=? AND remote_item_uuid=?),
  (SELECT MAX(sequence) FROM chat_items WHERE thread_id=? AND json_extract(metadata_json,'$.request_uuid')=?),
  (SELECT MAX(items.sequence) FROM llm_logs logs JOIN chat_items items ON items.run_id=logs.chat_run_id WHERE logs.chat_thread_id=? AND logs.uuid=?),
  (SELECT through_item_sequence FROM agent_context_summaries WHERE thread_id=? AND uuid=? LIMIT 1)
) AS sequence`
	if err := tx.WithContext(ctx).Raw(query, threadID, itemUUID, threadID, itemUUID, threadID, itemUUID, threadID, itemUUID, threadID, itemUUID).Scan(&anchor).Error; err != nil {
		return 0, err
	}
	if !anchor.Valid || anchor.Int64 <= 0 {
		return 0, domainError(CodeNotFound, "Trajectory Item 不存在", "item_uuid 不属于当前 Thread。", nil)
	}
	return anchor.Int64, nil
}

func queryTrajectoryItems(tx *gorm.DB, threadID, before, after, anchor int64, limit int) ([]trajectoryItemRow, bool, bool, error) {
	query := tx.Table("chat_items AS items").Select(`items.id,items.uuid,items.sequence,COALESCE(items.turn_id,0) AS turn_id,COALESCE(items.run_id,0) AS run_id,COALESCE(turns.uuid,'') AS turn_uuid,COALESCE(runs.uuid,'') AS run_uuid,items.item_type,items.role,items.content,items.content_format,items.status,items.remote_item_uuid,items.tool_name,items.target_uuid,items.metadata_json,items.created_at`).Joins("LEFT JOIN chat_turns turns ON turns.id=items.turn_id").Joins("LEFT JOIN chat_runs runs ON runs.id=items.run_id").Where("items.thread_id=?", threadID)
	descending := before > 0 || after == 0
	if anchor > 0 {
		query = query.Where("items.sequence<=?", anchor)
	} else if before > 0 {
		query = query.Where("items.sequence<?", before)
	} else if after > 0 {
		query = query.Where("items.sequence>?", after)
	}
	if descending {
		query = query.Order("items.sequence DESC,items.id DESC")
	} else {
		query = query.Order("items.sequence,items.id")
	}
	var rows []trajectoryItemRow
	if err := query.Limit(limit + 1).Scan(&rows).Error; err != nil {
		return nil, false, descending, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	if descending {
		for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
			rows[left], rows[right] = rows[right], rows[left]
		}
	}
	return rows, hasMore, descending, nil
}

func queryTrajectoryTurns(tx *gorm.DB, threadID int64) ([]TrajectoryTurn, error) {
	var rows []struct {
		UUID, SourceType, SourceFollowUpUUID, Status, ErrorCode, ErrorMessage string
		QueueSequence                                                         int64
		StartedAt, CompletedAt                                                *time.Time
		CreatedAt, UpdatedAt                                                  time.Time
	}
	err := tx.Table("chat_turns AS turns").Select(`turns.uuid,turns.queue_sequence,turns.source_type,COALESCE(follow_ups.uuid,'') AS source_follow_up_uuid,turns.status,turns.error_code,turns.error_message,turns.started_at,turns.completed_at,turns.created_at,turns.updated_at`).Joins("LEFT JOIN chat_follow_ups follow_ups ON follow_ups.id=turns.source_follow_up_id").Where("turns.thread_id=?", threadID).Order("turns.queue_sequence,turns.id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]TrajectoryTurn, 0, len(rows))
	for _, row := range rows {
		result = append(result, TrajectoryTurn{UUID: row.UUID, QueueSequence: row.QueueSequence, SourceType: row.SourceType, SourceFollowUpUUID: row.SourceFollowUpUUID, Status: row.Status, ErrorCode: row.ErrorCode, ErrorMessage: publicDiagnosticErrorMessage(row.ErrorCode), StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return result, nil
}

func indexTrajectoryEvents(rows []trajectoryEventLink) trajectoryEventIndex {
	index := trajectoryEventIndex{item: map[string]int64{}, turnStart: map[string]int64{}, toolStart: map[string]int64{}, toolEnd: map[string]int64{}, requestStart: map[string]int64{}, requestEnd: map[string]int64{}, compaction: map[string]int64{}, compactionTurn: map[string]string{}}
	for _, row := range rows {
		var payload map[string]any
		if json.Unmarshal([]byte(row.PayloadJSON), &payload) != nil {
			continue
		}
		itemUUID, _ := payload["item_uuid"].(string)
		toolCallUUID, _ := payload["tool_call_uuid"].(string)
		requestUUID, _ := payload["request_uuid"].(string)
		summaryUUID, _ := payload["summary_uuid"].(string)
		turnUUID, _ := payload["turn_uuid"].(string)
		if isUUIDv7(itemUUID) {
			index.item[itemUUID] = row.Sequence
		}
		switch row.EventType {
		case "turn_queued":
			if isUUIDv7(turnUUID) {
				index.turnStart[turnUUID] = row.Sequence
			}
		case "tool_intent":
			if isUUIDv7(toolCallUUID) {
				index.toolStart[toolCallUUID] = row.Sequence
			}
		case "tool_result", "user_input_answered":
			if isUUIDv7(toolCallUUID) {
				index.toolEnd[toolCallUUID] = row.Sequence
			}
		case "model_request_started":
			if isUUIDv7(requestUUID) {
				index.requestStart[requestUUID] = row.Sequence
			}
		case "model_request_completed":
			if isUUIDv7(requestUUID) {
				index.requestEnd[requestUUID] = row.Sequence
			}
		case "compaction_created":
			if isUUIDv7(summaryUUID) {
				index.compaction[summaryUUID] = row.Sequence
				if isUUIDv7(turnUUID) {
					index.compactionTurn[summaryUUID] = turnUUID
				}
			}
		}
	}
	return index
}

func projectTrajectoryItems(rows []trajectoryItemRow, threadUUID string, events trajectoryEventIndex) []TrajectorySourceItem {
	items := make([]TrajectorySourceItem, 0, len(rows))
	for _, row := range rows {
		metadata := trajectoryPublicMetadata(row.MetadataJSON)
		requestUUID := metadataString(row.MetadataJSON, "request_uuid")
		requestOrdinal := metadataInt(row.MetadataJSON, "request_ordinal")
		var eventSequence *int64
		if sequence := events.item[row.UUID]; sequence > 0 {
			eventSequence = int64Pointer(sequence)
		} else if row.ItemType == "user_message" && events.turnStart[row.TurnUUID] > 0 {
			eventSequence = int64Pointer(events.turnStart[row.TurnUUID])
		} else if row.RemoteItemUUID != "" {
			sequence := events.toolStart[row.RemoteItemUUID]
			if row.ItemType == "tool_result" {
				sequence = events.toolEnd[row.RemoteItemUUID]
			}
			if sequence > 0 {
				eventSequence = int64Pointer(sequence)
			}
		}
		accuracy := "approximate"
		if eventSequence != nil {
			accuracy = "exact"
		}
		content := sanitizeDiagnosticText(row.Content)
		if row.ContentFormat == "json" && json.Valid([]byte(row.Content)) {
			content = string(sanitizeDiagnosticJSON(row.Content))
		}
		items = append(items, TrajectorySourceItem{UUID: row.UUID, ThreadUUID: threadUUID, TurnUUID: row.TurnUUID, Sequence: row.Sequence, EventSequence: eventSequence, ItemType: row.ItemType, Role: row.Role, Content: content, ContentFormat: row.ContentFormat, Status: row.Status, ToolCallUUID: publicUUIDOrEmpty(row.RemoteItemUUID), ToolName: row.ToolName, TargetUUID: publicUUIDOrEmpty(row.TargetUUID), RequestUUID: publicUUIDOrEmpty(requestUUID), RequestOrdinal: requestOrdinal, Metadata: metadata, OrderingAccuracy: accuracy, CreatedAt: row.CreatedAt})
	}
	return items
}

func trajectoryPublicMetadata(raw string) json.RawMessage {
	var value map[string]any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return json.RawMessage("{}")
	}
	delete(value, "prompt_snapshot")
	delete(value, "provider_call_id")
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("{}")
	}
	return sanitizeDiagnosticJSON(string(encoded))
}

func projectTrajectoryModelRequests(rows []trajectoryModelRow, threadUUID string, events trajectoryEventIndex) []TrajectoryModelRequest {
	result := make([]TrajectoryModelRequest, 0, len(rows))
	for _, row := range rows {
		options, systemDigest, toolsDigest, hasPayload := trajectoryRequestMetadata(row.RequestPayload)
		hasToolCalls, assistantPreview, hasResponse := trajectoryResponseMetadata(row.Response, row.OutputSummary)
		request := TrajectoryModelRequest{UUID: row.UUID, ThreadUUID: threadUUID, TurnUUID: row.TurnUUID, RunUUID: row.RunUUID, RequestOrdinal: row.Attempt, RequestType: sanitizeDiagnosticText(row.RequestType), Scenario: sanitizeDiagnosticText(row.Scenario), ProviderUUID: publicUUIDOrEmpty(row.ProviderUUID), ProviderType: sanitizeDiagnosticText(row.ProviderType), Model: sanitizeDiagnosticText(row.Model), Status: row.Status, Options: options, InputSummary: sanitizeDiagnosticText(row.InputSummary), OutputSummary: sanitizeDiagnosticText(row.OutputSummary), AssistantPreview: sanitizeDiagnosticText(assistantPreview), HasToolCalls: hasToolCalls, CachedInputTokens: row.CachedInputTokens, FinishReason: sanitizeDiagnosticText(row.FinishReason), ErrorCode: sanitizeDiagnosticText(row.ErrorCode), ErrorMessage: publicDiagnosticErrorMessage(row.ErrorCode), ProviderErrorCode: sanitizeDiagnosticText(row.ProviderErrorCode), ProviderRequestID: sanitizeDiagnosticText(row.ProviderRequestID), SystemPromptDigest: systemDigest, ToolCatalogDigest: toolsDigest, HasRequestPayload: hasPayload, HasResponse: hasResponse, OrderingAccuracy: "legacy_unlinked", UsageAccuracy: "recorded", CreatedAt: row.CreatedAt, CompletedAt: row.CompletedAt}
		if row.RequestType != "text" {
			request.OrderingAccuracy = "approximate"
		}
		if row.InputTokens > 0 {
			request.InputTokens = intPointer(row.InputTokens)
		}
		if row.OutputTokens > 0 {
			request.OutputTokens = intPointer(row.OutputTokens)
		}
		if row.DurationMS > 0 && row.CompletedAt != nil {
			request.DurationMS = int64Pointer(row.DurationMS)
		}
		if row.HTTPStatus > 0 {
			request.HTTPStatus = intPointer(row.HTTPStatus)
		}
		if row.InputTokens == 0 || row.OutputTokens == 0 {
			request.UsageAccuracy = "legacy_unknown"
		}
		if sequence := events.requestStart[row.UUID]; sequence > 0 {
			request.StartEventSequence = int64Pointer(sequence)
			request.OrderingAccuracy = "exact"
		}
		if sequence := events.requestEnd[row.UUID]; sequence > 0 {
			request.EndEventSequence = int64Pointer(sequence)
		}
		result = append(result, request)
	}
	return result
}

func trajectoryRequestMetadata(raw sql.NullString) (json.RawMessage, string, string, bool) {
	options := map[string]any{}
	if !raw.Valid || !json.Valid([]byte(raw.String)) {
		encoded, _ := json.Marshal(options)
		return encoded, "", "", false
	}
	var fields map[string]json.RawMessage
	var snapshot struct {
		Messages    []struct{ Role, Content string } `json:"messages"`
		Tools       json.RawMessage                  `json:"tools"`
		Temperature *float64                         `json:"temperature"`
		MaxTokens   int                              `json:"max_tokens"`
		Stream      bool                             `json:"stream"`
		Size        string                           `json:"size"`
		Quality     string                           `json:"quality"`
	}
	if json.Unmarshal([]byte(raw.String), &fields) != nil || json.Unmarshal([]byte(raw.String), &snapshot) != nil {
		encoded, _ := json.Marshal(options)
		return encoded, "", "", false
	}
	if snapshot.Temperature != nil {
		options["temperature"] = *snapshot.Temperature
	}
	if snapshot.MaxTokens > 0 {
		options["max_tokens"] = snapshot.MaxTokens
	}
	if _, exists := fields["stream"]; exists {
		options["stream"] = snapshot.Stream
	}
	if snapshot.Size != "" {
		options["size"] = sanitizeDiagnosticText(snapshot.Size)
	}
	if snapshot.Quality != "" {
		options["quality"] = sanitizeDiagnosticText(snapshot.Quality)
	}
	systemMessages := []string{}
	for _, message := range snapshot.Messages {
		if message.Role == "system" {
			systemMessages = append(systemMessages, message.Content)
		}
	}
	systemDigest, toolsDigest := "", ""
	if _, exists := fields["messages"]; exists {
		systemEncoded, _ := json.Marshal(systemMessages)
		systemDigest = digestBytes(systemEncoded)
	}
	if tools, exists := fields["tools"]; exists {
		toolsDigest = digestBytes(tools)
	}
	encodedOptions, _ := json.Marshal(options)
	return encodedOptions, systemDigest, toolsDigest, true
}

func trajectoryResponseMetadata(raw sql.NullString, fallback string) (bool, string, bool) {
	if !raw.Valid || !json.Valid([]byte(raw.String)) {
		return false, fallback, false
	}
	var snapshot struct {
		Message struct {
			Content   string            `json:"content"`
			ToolCalls []json.RawMessage `json:"tool_calls"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(raw.String), &snapshot) != nil {
		return false, fallback, false
	}
	preview := snapshot.Message.Content
	if strings.TrimSpace(preview) == "" && len(snapshot.Message.ToolCalls) > 0 {
		preview = "Tool calls requested"
	} else if strings.TrimSpace(preview) == "" {
		preview = fallback
	}
	return len(snapshot.Message.ToolCalls) > 0, preview, true
}

func projectTrajectoryTools(rows []trajectoryToolRow, threadUUID string, turnStatuses map[string]string, events trajectoryEventIndex) []TrajectoryTool {
	result := make([]TrajectoryTool, 0, len(rows))
	for _, row := range rows {
		arguments, requestUUID, requestOrdinal := trajectoryToolArguments(row.ArgumentsJSON, row.CallMetadata)
		var rawResult json.RawMessage
		// executing result_json may contain a private side-effect checkpoint;
		// only completed Tool Results are part of the public trajectory.
		if row.State == "completed" && row.ResultJSON.Valid {
			rawResult = sanitizeDiagnosticJSON(row.ResultJSON.String)
		}
		status := "pending"
		switch row.State {
		case "executing":
			status = "running"
		case "completed":
			status = "completed"
		case "failed":
			status = "error"
		}
		if trajectoryResultFailed(rawResult) {
			status = "error"
		}
		derivedReason := ""
		if (row.State == "intent" || row.State == "executing") && terminalTurnStatus(turnStatuses[row.TurnUUID]) {
			status = "interrupted"
			derivedReason = "Turn reached a terminal state before the persisted Tool lifecycle completed."
		}
		tool := TrajectoryTool{UUID: row.UUID, ThreadUUID: threadUUID, TurnUUID: row.TurnUUID, CallItemUUID: row.CallItemUUID, ResultItemUUID: row.ResultItemUUID, ToolCallUUID: publicUUIDOrEmpty(row.ToolCallUUID), ToolName: sanitizeDiagnosticText(row.ToolName), TargetUUID: publicUUIDOrEmpty(row.TargetUUID), RequestUUID: publicUUIDOrEmpty(requestUUID), RequestOrdinal: requestOrdinal, CallSequence: row.CallSequence, Status: status, DerivedReason: derivedReason, Arguments: arguments, Result: rawResult, ErrorCode: sanitizeDiagnosticText(row.ErrorCode), ErrorMessage: publicDiagnosticErrorMessage(row.ErrorCode), StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, OrderingAccuracy: "legacy_unlinked", CreatedAt: row.CreatedAt}
		if row.ResultSequence.Valid {
			tool.ResultSequence = int64Pointer(row.ResultSequence.Int64)
		}
		if sequence := events.toolStart[row.ToolCallUUID]; sequence > 0 {
			tool.StartEventSequence = int64Pointer(sequence)
			tool.OrderingAccuracy = "exact"
		}
		endSequence := events.toolEnd[row.ToolCallUUID]
		if endSequence == 0 && row.ResultItemUUID != "" {
			endSequence = events.item[row.ResultItemUUID]
		}
		if endSequence > 0 {
			tool.EndEventSequence = int64Pointer(endSequence)
		}
		if row.StartedAt != nil && row.CompletedAt != nil && !row.CompletedAt.Before(*row.StartedAt) {
			tool.DurationMS = int64Pointer(row.CompletedAt.Sub(*row.StartedAt).Milliseconds())
		}
		result = append(result, tool)
	}
	return result
}

func trajectoryToolArguments(raw, metadataRaw string) (json.RawMessage, string, int) {
	var value map[string]any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return json.RawMessage("{}"), "", 0
	}
	requestUUID, _ := value["__request_uuid"].(string)
	requestOrdinal := anyInt(value["__request_ordinal"])
	if requestUUID == "" {
		requestUUID = metadataString(metadataRaw, "request_uuid")
	}
	if requestOrdinal == 0 {
		requestOrdinal = metadataInt(metadataRaw, "request_ordinal")
	}
	for key := range value {
		if strings.HasPrefix(key, "__") {
			delete(value, key)
		}
	}
	safeAPIPath := ""
	if path, ok := value["path"].(string); ok && strings.HasPrefix(path, "/api/v1/") {
		safeAPIPath = path
	}
	delete(value, "path")
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("{}"), requestUUID, requestOrdinal
	}
	sanitized := sanitizeDiagnosticJSON(string(encoded))
	if safeAPIPath != "" {
		var restored map[string]any
		if json.Unmarshal(sanitized, &restored) == nil {
			restored["path"] = safeAPIPath
			if restoredJSON, restoreErr := json.Marshal(restored); restoreErr == nil {
				sanitized = restoredJSON
			}
		}
	}
	return sanitized, requestUUID, requestOrdinal
}

func trajectoryResultFailed(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var envelope struct {
		Success *bool `json:"success"`
	}
	return json.Unmarshal(raw, &envelope) == nil && envelope.Success != nil && !*envelope.Success
}

func projectTrajectoryCompactions(rows []trajectoryCompactionRow, threadUUID string, events trajectoryEventIndex) []TrajectoryCompaction {
	result := make([]TrajectoryCompaction, 0, len(rows))
	for _, row := range rows {
		compaction := TrajectoryCompaction{UUID: row.UUID, ThreadUUID: threadUUID, ThroughItemSequence: row.ThroughItemSequence, Summary: sanitizeDiagnosticText(row.Summary), SourceBytes: row.SourceBytes, OrderingAccuracy: "approximate", CreatedAt: row.CreatedAt}
		if sequence := events.compaction[row.UUID]; sequence > 0 {
			compaction.EventSequence = int64Pointer(sequence)
			compaction.OrderingAccuracy = "exact"
			if turnUUID := events.compactionTurn[row.UUID]; isUUIDv7(turnUUID) {
				compaction.TurnUUID = stringPointer(turnUUID)
			}
		}
		result = append(result, compaction)
	}
	return result
}

func buildTrajectoryTimeline(items []trajectoryCompactItemRow, requests []TrajectoryModelRequest, tools []TrajectoryTool, compactions []TrajectoryCompaction, events trajectoryEventIndex) []TrajectoryTimelineEntry {
	result := make([]TrajectoryTimelineEntry, 0, len(items)+len(requests)+len(tools)+len(compactions))
	for _, item := range items {
		if item.ItemType == "tool_call" || item.ItemType == "tool_result" || item.ItemType == "user_input_request" {
			continue
		}
		sequence := item.Sequence
		entry := TrajectoryTimelineEntry{UUID: item.UUID, SourceKind: "chat_item", Kind: trajectoryKindForItem(item.ItemType), TurnUUID: item.TurnUUID, ItemSequence: &sequence, Status: trajectoryStatus(item.Status), Preview: trajectoryPreview(item.Content), StartedAt: timePointer(item.CreatedAt), OrderingAccuracy: "approximate"}
		if eventSequence := events.item[item.UUID]; eventSequence > 0 {
			entry.EventSequence = int64Pointer(eventSequence)
			entry.OrderingAccuracy = "exact"
		} else if item.ItemType == "user_message" && events.turnStart[item.TurnUUID] > 0 {
			entry.EventSequence = int64Pointer(events.turnStart[item.TurnUUID])
			entry.OrderingAccuracy = "exact"
		}
		result = append(result, entry)
	}
	for _, request := range requests {
		result = append(result, TrajectoryTimelineEntry{UUID: request.UUID, SourceKind: "model_request", Kind: "model_request", TurnUUID: request.TurnUUID, EventSequence: request.StartEventSequence, RequestUUID: request.UUID, RequestOrdinal: request.RequestOrdinal, Status: request.Status, Preview: request.Model, StartedAt: timePointer(request.CreatedAt), CompletedAt: request.CompletedAt, DurationMS: request.DurationMS, OrderingAccuracy: request.OrderingAccuracy})
	}
	for _, tool := range tools {
		sequence := tool.CallSequence
		result = append(result, TrajectoryTimelineEntry{UUID: tool.ToolCallUUID, SourceKind: "tool", Kind: "tool", TurnUUID: tool.TurnUUID, ItemSequence: &sequence, EventSequence: tool.StartEventSequence, RequestUUID: tool.RequestUUID, RequestOrdinal: tool.RequestOrdinal, ToolCallUUID: tool.ToolCallUUID, Status: tool.Status, Preview: tool.ToolName, StartedAt: firstTime(tool.StartedAt, timePointer(tool.CreatedAt)), CompletedAt: tool.CompletedAt, DurationMS: tool.DurationMS, OrderingAccuracy: tool.OrderingAccuracy})
	}
	for _, compaction := range compactions {
		sequence := compaction.ThroughItemSequence
		turnUUID := ""
		if compaction.TurnUUID != nil {
			turnUUID = *compaction.TurnUUID
		}
		result = append(result, TrajectoryTimelineEntry{UUID: compaction.UUID, SourceKind: "compaction", Kind: "compaction", TurnUUID: turnUUID, ItemSequence: &sequence, EventSequence: compaction.EventSequence, Status: "completed", Preview: trajectoryPreview(compaction.Summary), StartedAt: timePointer(compaction.CreatedAt), OrderingAccuracy: compaction.OrderingAccuracy})
	}
	sort.SliceStable(result, func(left, right int) bool {
		leftTime, rightTime := result[left].StartedAt, result[right].StartedAt
		if leftTime != nil && rightTime != nil && !leftTime.Equal(*rightTime) {
			return leftTime.Before(*rightTime)
		}
		return result[left].UUID < result[right].UUID
	})
	return result
}

func trajectoryPageBounds(rows []trajectoryItemRow) (int64, int64) {
	if len(rows) == 0 {
		return 0, 0
	}
	return rows[0].Sequence, rows[len(rows)-1].Sequence
}

func trajectoryTurnOnPage(turnUUID string, rows []trajectoryItemRow) bool {
	for _, row := range rows {
		if row.TurnUUID == turnUUID {
			return true
		}
	}
	return false
}

func trajectorySequenceOnPage(sequence, low, high int64) bool {
	return sequence > 0 && low > 0 && sequence >= low && sequence <= high
}
func activeTurnStatus(status string) bool {
	return status == TurnQueued || status == TurnInProgress || status == TurnWaitingForInput
}
func terminalTurnStatus(status string) bool {
	return status == TurnCompleted || status == TurnFailed || status == TurnCancelled || status == TurnInterrupted
}

func trajectoryKindForItem(itemType string) string {
	switch itemType {
	case "user_message":
		return "user"
	case "assistant_message":
		return "assistant"
	case "tool_call", "tool_result", "user_input_request":
		return "tool"
	case "context_summary":
		return "context"
	case "error":
		return "error"
	default:
		return "system"
	}
}

func trajectoryStatus(status string) string {
	switch status {
	case "failed":
		return "error"
	case "cancelled":
		return "interrupted"
	case "in_progress":
		return "running"
	default:
		return status
	}
}

func trajectoryPreview(value string) string {
	value = strings.Join(strings.Fields(sanitizeDiagnosticText(value)), " ")
	if len([]rune(value)) > 180 {
		return string([]rune(value)[:179]) + "…"
	}
	return value
}

func metadataInt(raw, key string) int {
	var value map[string]any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return 0
	}
	return anyInt(value[key])
}

func anyInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case json.Number:
		result, _ := typed.Int64()
		return int(result)
	default:
		return 0
	}
}

func publicUUIDOrEmpty(value string) string {
	if isUUIDv7(value) {
		return value
	}
	return ""
}
func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
func intPointer(value int) *int              { return &value }
func int64Pointer(value int64) *int64        { return &value }
func stringPointer(value string) *string     { return &value }
func timePointer(value time.Time) *time.Time { return &value }
func firstTime(values ...*time.Time) *time.Time {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
