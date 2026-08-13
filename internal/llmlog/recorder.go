package llmlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/project"
	"lumi/internal/providerdiag"

	"github.com/google/uuid"
)

const (
	SourceStoryGeneration = "story_generation"
	SourceProjectChat     = "project_chat"
	SourceProduction      = "production"
	SourceWorkflow        = "workflow"
	RequestText           = "text"
	RequestImage          = "image"
	EventChanged          = "llm_log:changed"
)

type EventPublisher interface {
	Broadcast(topic, event string, payload any)
}

type StartInput struct {
	ProjectID           int64
	TaskRunID           int64
	ProductionTaskRunID int64
	AgentThreadID       int64
	AgentRunID          int64
	ChatThreadID        int64
	ChatRunID           int64
	WorkflowID          int64
	WorkflowStepID      int64
	SourceType          string
	Scenario            string
	RequestType         string
	Attempt             int
	ProviderUUID        string
	ProviderType        string
	Model               string
	InputSummary        string
	RequestPayload      json.RawMessage
}

type Handle struct {
	ID          int64
	UUID        string
	RequestType string
	StartedAt   time.Time
}

type FinishInput struct {
	Status            string
	OutputSummary     string
	InputTokens       int
	CachedInputTokens *int
	OutputTokens      int
	FinishReason      string
	Response          json.RawMessage
	Err               error
}

func Begin(ctx context.Context, store *project.Store, events EventPublisher, input StartInput) (Handle, error) {
	if store == nil || input.ProjectID <= 0 || input.Attempt <= 0 || strings.TrimSpace(input.SourceType) == "" || strings.TrimSpace(input.Scenario) == "" || strings.TrimSpace(input.RequestType) == "" || strings.TrimSpace(input.ProviderUUID) == "" || strings.TrimSpace(input.Model) == "" || len(input.RequestPayload) == 0 || !json.Valid(input.RequestPayload) {
		return Handle{}, fmt.Errorf("invalid AI call log input")
	}
	value, err := uuid.NewV7()
	if err != nil {
		return Handle{}, err
	}
	now := time.Now().UTC()
	sqlDB, err := store.DB().DB()
	if err != nil {
		return Handle{}, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Handle{}, err
	}
	defer tx.Rollback()
	var inputCharacters any
	if input.RequestType == RequestText {
		inputCharacters = snapshotCharacterCount(input.RequestPayload)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO llm_logs(
		uuid,project_id,task_run_id,production_task_run_id,agent_thread_id,agent_run_id,
		chat_thread_id,chat_run_id,workflow_id,workflow_step_id,
		source_type,scenario,request_type,attempt,provider_uuid,provider_type,model,status,input_summary,input_characters,request_payload,created_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'pending',?,?,?,?)`,
		value.String(), input.ProjectID, nullableID(input.TaskRunID), nullableID(input.ProductionTaskRunID), nullableID(input.AgentThreadID), nullableID(input.AgentRunID),
		nullableID(input.ChatThreadID), nullableID(input.ChatRunID), nullableID(input.WorkflowID), nullableID(input.WorkflowStepID),
		input.SourceType, input.Scenario, input.RequestType, input.Attempt, input.ProviderUUID, input.ProviderType, input.Model, Summarize(input.InputSummary, 1000), inputCharacters, string(input.RequestPayload), now)
	if err != nil {
		return Handle{}, err
	}
	logID, err := result.LastInsertId()
	if err != nil {
		return Handle{}, err
	}
	if err := tx.Commit(); err != nil {
		return Handle{}, err
	}
	handle := Handle{ID: logID, UUID: value.String(), RequestType: input.RequestType, StartedAt: time.Now()}
	emitChanged(events, store.ProjectUUID(), handle.UUID, "pending")
	return handle, nil
}

func Finish(ctx context.Context, store *project.Store, events EventPublisher, handle Handle, input FinishInput) error {
	if store == nil || handle.ID <= 0 {
		return fmt.Errorf("invalid AI call log handle")
	}
	status := strings.TrimSpace(input.Status)
	errorCode, errorMessage := "", ""
	diagnostic := providerdiag.Details{}
	if input.Err != nil {
		status, errorCode, errorMessage = classifyFailure(input.Err)
		diagnostic = providerdiag.FromError(input.Err)
		if diagnostic.Message != "" {
			errorMessage = diagnostic.Message
		}
	}
	if status == "" {
		status = "completed"
	}
	if len(input.Response) > 0 && !json.Valid(input.Response) {
		return fmt.Errorf("invalid AI call log response")
	}
	if status == "completed" && len(input.Response) == 0 {
		return fmt.Errorf("completed AI call log response is required")
	}
	var outputCharacters any
	if handle.RequestType == RequestText && len(input.Response) > 0 {
		outputCharacters = snapshotCharacterCount(input.Response)
	}
	result := store.DB().WithContext(ctx).Exec(`UPDATE llm_logs SET
		status=?,output_summary=?,input_tokens=?,cached_input_tokens=?,output_tokens=?,output_characters=?,duration_ms=?,finish_reason=?,
		error_code=?,error_message=?,http_status=?,provider_error_code=?,provider_request_id=?,response=?,completed_at=?
		WHERE id=? AND status='pending'`,
		status, Summarize(input.OutputSummary, 1000), input.InputTokens, nullableInt(input.CachedInputTokens), input.OutputTokens, outputCharacters, time.Since(handle.StartedAt).Milliseconds(), input.FinishReason,
		errorCode, Summarize(errorMessage, 2000), diagnostic.HTTPStatus, diagnostic.ProviderCode, diagnostic.RequestID, nullableJSON(input.Response), time.Now().UTC(), handle.ID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("AI call log %s is not pending", handle.UUID)
	}
	emitChanged(events, store.ProjectUUID(), handle.UUID, status)
	return nil
}

func emitChanged(events EventPublisher, projectUUID, logUUID, status string) {
	if events == nil {
		return
	}
	defer func() { _ = recover() }()
	events.Broadcast("project:"+projectUUID, EventChanged, map[string]any{
		"project_uuid": projectUUID,
		"log_uuid":     logUUID,
		"status":       status,
	})
}

func Summarize(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit < 1 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

func classifyFailure(err error) (status, code, message string) {
	status, code, message = "failed", "provider_call_failed", "Provider 请求失败。"
	if errors.Is(err, context.Canceled) {
		return "cancelled", llm.CodeCancelled, "请求已取消。"
	}
	var llmErr *llm.Error
	if errors.As(err, &llmErr) {
		status = "failed"
		if llmErr.Code == llm.CodeCancelled {
			status = "cancelled"
		}
		return status, llmErr.Code, llmErr.SafeMessage
	}
	var imageErr *imagegen.Error
	if errors.As(err, &imageErr) {
		status = "failed"
		if imageErr.Code == "image_cancelled" {
			status = "cancelled"
		}
		return status, imageErr.Code, imageErr.SafeMessage
	}
	return status, code, message
}

func nullableID(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

// snapshotCharacterCount counts Unicode code points contained in JSON string
// values. This matches the text actually sent to or returned by the provider
// without counting JSON punctuation or numeric usage metadata.
func snapshotCharacterCount(raw json.RawMessage) int {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return 0
	}
	return snapshotValueCharacterCount(value)
}

func snapshotValueCharacterCount(value any) int {
	switch current := value.(type) {
	case string:
		return utf8.RuneCountInString(current)
	case []any:
		total := 0
		for _, item := range current {
			total += snapshotValueCharacterCount(item)
		}
		return total
	case map[string]any:
		total := 0
		for _, item := range current {
			total += snapshotValueCharacterCount(item)
		}
		return total
	default:
		return 0
	}
}
