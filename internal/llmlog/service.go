package llmlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"lumi/internal/project"
)

var (
	ErrNotFound      = errors.New("LLM log not found")
	ErrInvalidFilter = errors.New("invalid LLM log filter")
)

type Log struct {
	UUID                      string     `json:"uuid"`
	SourceType                string     `json:"source_type"`
	Scenario                  string     `json:"scenario"`
	Scope                     string     `json:"scope,omitempty"`
	ProviderUUID              string     `json:"provider_uuid"`
	ProviderType              string     `json:"provider_type"`
	Model                     string     `json:"model"`
	Status                    string     `json:"status"`
	InputSummary              string     `json:"input_summary"`
	OutputSummary             string     `json:"output_summary"`
	InputTokens               int        `json:"input_tokens"`
	CachedInputTokens         *int       `json:"cached_input_tokens"`
	OutputTokens              int        `json:"output_tokens"`
	InputCharacters           *int       `json:"input_characters"`
	OutputCharacters          *int       `json:"output_characters"`
	OutputTokensPerSecond     *float64   `json:"output_tokens_per_second"`
	OutputCharactersPerSecond *float64   `json:"output_characters_per_second"`
	DurationMS                int64      `json:"duration_ms"`
	FinishReason              string     `json:"finish_reason"`
	ErrorCode                 string     `json:"error_code"`
	ErrorMessage              string     `json:"error_message"`
	RequestType               string     `json:"request_type"`
	Attempt                   int        `json:"attempt"`
	HTTPStatus                int        `json:"http_status"`
	ProviderErrorCode         string     `json:"provider_error_code"`
	ProviderRequestID         string     `json:"provider_request_id"`
	TaskUUID                  string     `json:"task_uuid"`
	ThreadUUID                string     `json:"thread_uuid"`
	RunUUID                   string     `json:"run_uuid"`
	WorkflowUUID              string     `json:"workflow_uuid"`
	WorkflowStepUUID          string     `json:"workflow_step_uuid"`
	CreatedAt                 time.Time  `json:"created_at"`
	CompletedAt               *time.Time `json:"completed_at"`
}

type Detail struct {
	Log
	RequestPayload json.RawMessage `json:"request_payload"`
	Response       json.RawMessage `json:"response"`
}

type detailRow struct {
	Log
	RequestPayload sql.NullString `gorm:"column:request_payload"`
	Response       sql.NullString `gorm:"column:response"`
}

type Pagination struct {
	PerPage     int   `json:"per_page"`
	CurrentPage int   `json:"current_page"`
	LastPage    int   `json:"last_page"`
	Total       int64 `json:"total"`
}

type Filter struct {
	Scope        string
	ProviderUUID string
	ProviderType string
	Model        string
	Scenario     string
	Status       string
	RequestType  string
	Keyword      string
}

type ProviderFilterOption struct {
	UUID string `json:"uuid"`
	Type string `json:"type"`
}

type FilterGroups struct {
	Providers     []ProviderFilterOption `json:"providers"`
	ProviderTypes []string               `json:"provider_types"`
	Models        []string               `json:"models"`
	Scenarios     []string               `json:"scenarios"`
	Statuses      []string               `json:"statuses"`
	RequestTypes  []string               `json:"request_types"`
}

type Service struct {
	store *project.Store
}

func NewService(store *project.Store) *Service {
	return &Service{store: store}
}

const unifiedLogsSQL = `
SELECT
  logs.uuid AS uuid,
  logs.source_type AS source_type,
  logs.scenario AS scenario,
	CASE
	    WHEN logs.source_type = 'project_chat' THEN 'project'
    WHEN logs.source_type = 'workflow' THEN 'project'
    WHEN logs.source_type = 'production' AND logs.scenario IN ('premise_setting_generation','premise_asset_breakdown','premise_asset_generation') THEN 'premise'
    WHEN logs.source_type = 'production' THEN 'project'
    ELSE ''
	END AS scope,
  logs.provider_uuid AS provider_uuid,
  logs.provider_type AS provider_type,
  logs.model AS model,
  logs.status AS status,
  logs.input_summary AS input_summary,
  logs.output_summary AS output_summary,
  logs.input_tokens AS input_tokens,
  logs.cached_input_tokens AS cached_input_tokens,
  logs.output_tokens AS output_tokens,
  logs.input_characters AS input_characters,
  logs.output_characters AS output_characters,
  CASE WHEN logs.request_type = 'text' AND logs.duration_ms > 0 AND logs.output_tokens > 0
       THEN (logs.output_tokens * 1000.0) / logs.duration_ms ELSE NULL END AS output_tokens_per_second,
  CASE WHEN logs.request_type = 'text' AND logs.duration_ms > 0 AND logs.output_characters > 0
       THEN (logs.output_characters * 1000.0) / logs.duration_ms ELSE NULL END AS output_characters_per_second,
  logs.duration_ms AS duration_ms,
  logs.finish_reason AS finish_reason,
  logs.error_code AS error_code,
  logs.error_message AS error_message,
  logs.request_type AS request_type,
  logs.attempt AS attempt,
  logs.http_status AS http_status,
  logs.provider_error_code AS provider_error_code,
  logs.provider_request_id AS provider_request_id,
  COALESCE(tasks.uuid, production_tasks.uuid, '') AS task_uuid,
  COALESCE(chat_threads.uuid, workflow_threads.uuid, story_threads.uuid, '') AS thread_uuid,
  COALESCE(chat_runs.uuid, story_runs.uuid, '') AS run_uuid,
  COALESCE(workflows.uuid, '') AS workflow_uuid,
  COALESCE(workflow_steps.uuid, '') AS workflow_step_uuid,
  logs.created_at AS created_at,
  logs.completed_at AS completed_at
FROM llm_logs AS logs
LEFT JOIN task_runs AS tasks ON tasks.id = logs.task_run_id
LEFT JOIN production_task_runs AS production_tasks ON production_tasks.id = logs.production_task_run_id
LEFT JOIN agent_threads AS story_threads ON story_threads.id = logs.agent_thread_id
LEFT JOIN agent_runs AS story_runs ON story_runs.id = logs.agent_run_id
LEFT JOIN chat_threads ON chat_threads.id = logs.chat_thread_id
LEFT JOIN chat_runs ON chat_runs.id = logs.chat_run_id
LEFT JOIN workflows ON workflows.id = logs.workflow_id
LEFT JOIN workflow_steps ON workflow_steps.id = logs.workflow_step_id
LEFT JOIN chat_threads AS workflow_threads ON workflow_threads.id = workflows.thread_id
WHERE logs.project_id = ?`

func normalizePagination(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 12
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

func (service *Service) List(ctx context.Context, filter Filter, page, perPage int) ([]Log, Pagination, FilterGroups, error) {
	filter = normalizeFilter(filter)
	if err := validateFilter(filter); err != nil {
		return nil, Pagination{}, FilterGroups{}, err
	}
	page, perPage = normalizePagination(page, perPage)
	var projectID int64
	if err := service.store.DB().WithContext(ctx).Model(&project.Project{}).Where("uuid = ?", service.store.ProjectUUID()).Pluck("id", &projectID).Error; err != nil {
		return nil, Pagination{}, FilterGroups{}, err
	}
	filterGroups, err := service.filterGroups(ctx, projectID, filter.Scope)
	if err != nil {
		return nil, Pagination{}, FilterGroups{}, err
	}
	whereSQL, whereArgs := filterWhere(filter)
	filteredSQL := `SELECT * FROM (` + unifiedLogsSQL + `) AS all_project_llm_logs` + whereSQL
	baseArgs := append([]any{projectID}, whereArgs...)
	var total int64
	if err := service.store.DB().WithContext(ctx).Raw(`SELECT COUNT(*) FROM (`+filteredSQL+`)`, baseArgs...).Scan(&total).Error; err != nil {
		return nil, Pagination{}, FilterGroups{}, err
	}
	items := make([]Log, 0, perPage)
	query := filteredSQL + ` ORDER BY created_at DESC, uuid DESC LIMIT ? OFFSET ?`
	queryArgs := append(append([]any{}, baseArgs...), perPage, (page-1)*perPage)
	if err := service.store.DB().WithContext(ctx).Raw(query, queryArgs...).Scan(&items).Error; err != nil {
		return nil, Pagination{}, FilterGroups{}, err
	}
	lastPage := int((total + int64(perPage) - 1) / int64(perPage))
	if lastPage < 1 {
		lastPage = 1
	}
	return items, Pagination{PerPage: perPage, CurrentPage: page, LastPage: lastPage, Total: total}, filterGroups, nil
}

func normalizeFilter(filter Filter) Filter {
	filter.Scope = strings.ToLower(strings.TrimSpace(filter.Scope))
	filter.ProviderUUID = strings.TrimSpace(filter.ProviderUUID)
	filter.ProviderType = strings.ToLower(strings.TrimSpace(filter.ProviderType))
	filter.Model = strings.TrimSpace(filter.Model)
	filter.Scenario = strings.TrimSpace(filter.Scenario)
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	filter.RequestType = strings.ToLower(strings.TrimSpace(filter.RequestType))
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	return filter
}

func validateFilter(filter Filter) error {
	if filter.Scope != "" && filter.Scope != "project" && filter.Scope != "premise" {
		return fmt.Errorf("%w: scope must be project or premise", ErrInvalidFilter)
	}
	if filter.Status != "" && filter.Status != "pending" && filter.Status != "completed" && filter.Status != "failed" && filter.Status != "cancelled" {
		return fmt.Errorf("%w: status is invalid", ErrInvalidFilter)
	}
	if filter.RequestType != "" && filter.RequestType != RequestText && filter.RequestType != RequestImage {
		return fmt.Errorf("%w: request_type is invalid", ErrInvalidFilter)
	}
	for name, value := range map[string]string{"provider_uuid": filter.ProviderUUID, "provider_type": filter.ProviderType, "model": filter.Model, "scenario": filter.Scenario, "keyword": filter.Keyword} {
		if len([]rune(value)) > 160 {
			return fmt.Errorf("%w: %s is too long", ErrInvalidFilter, name)
		}
	}
	return nil
}

func filterWhere(filter Filter) (string, []any) {
	clauses := []string{}
	args := []any{}
	add := func(column, value string) {
		if value == "" {
			return
		}
		clauses = append(clauses, column+" = ?")
		args = append(args, value)
	}
	add("scope", filter.Scope)
	add("provider_uuid", filter.ProviderUUID)
	add("provider_type", filter.ProviderType)
	add("model", filter.Model)
	add("scenario", filter.Scenario)
	add("status", filter.Status)
	add("request_type", filter.RequestType)
	if filter.Keyword != "" {
		needle := "%" + escapeLike(strings.ToLower(filter.Keyword)) + "%"
		clauses = append(clauses, `(lower(input_summary) LIKE ? ESCAPE '\' OR lower(output_summary) LIKE ? ESCAPE '\' OR lower(model) LIKE ? ESCAPE '\' OR lower(scenario) LIKE ? ESCAPE '\' OR lower(error_code) LIKE ? ESCAPE '\' OR lower(provider_request_id) LIKE ? ESCAPE '\')`)
		for range 6 {
			args = append(args, needle)
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func (service *Service) filterGroups(ctx context.Context, projectID int64, scope string) (FilterGroups, error) {
	groups := FilterGroups{Providers: []ProviderFilterOption{}, ProviderTypes: []string{}, Models: []string{}, Scenarios: []string{}, Statuses: []string{}, RequestTypes: []string{}}
	base := `SELECT * FROM (` + unifiedLogsSQL + `) AS scoped_project_llm_logs`
	args := []any{projectID}
	if scope != "" {
		base += ` WHERE scope = ?`
		args = append(args, scope)
	}
	if err := service.store.DB().WithContext(ctx).Raw(`SELECT DISTINCT provider_uuid AS uuid, provider_type AS type FROM (`+base+`) WHERE provider_uuid <> '' ORDER BY type, uuid`, args...).Scan(&groups.Providers).Error; err != nil {
		return FilterGroups{}, err
	}
	queries := []struct {
		column string
		target any
	}{
		{"provider_type", &groups.ProviderTypes},
		{"model", &groups.Models},
		{"scenario", &groups.Scenarios},
		{"status", &groups.Statuses},
		{"request_type", &groups.RequestTypes},
	}
	for _, query := range queries {
		sql := `SELECT DISTINCT ` + query.column + ` FROM (` + base + `) WHERE ` + query.column + ` <> '' ORDER BY ` + query.column
		if err := service.store.DB().WithContext(ctx).Raw(sql, args...).Scan(query.target).Error; err != nil {
			return FilterGroups{}, err
		}
	}
	return groups, nil
}

func (service *Service) Get(ctx context.Context, logUUID string) (Detail, error) {
	logUUID = strings.TrimSpace(logUUID)
	if logUUID == "" {
		return Detail{}, ErrNotFound
	}
	var projectID int64
	if err := service.store.DB().WithContext(ctx).Model(&project.Project{}).Where("uuid = ?", service.store.ProjectUUID()).Pluck("id", &projectID).Error; err != nil {
		return Detail{}, err
	}
	query := `SELECT summary.*, raw.request_payload, raw.response
FROM (` + unifiedLogsSQL + `) AS summary
JOIN llm_logs AS raw ON raw.uuid = summary.uuid
WHERE summary.uuid = ?
LIMIT 1`
	var row detailRow
	result := service.store.DB().WithContext(ctx).Raw(query, projectID, logUUID).Scan(&row)
	if result.Error != nil {
		return Detail{}, result.Error
	}
	if result.RowsAffected == 0 {
		return Detail{}, ErrNotFound
	}
	requestPayload, err := decodeNullableJSON(row.RequestPayload)
	if err != nil {
		return Detail{}, fmt.Errorf("decode LLM log request payload: %w", err)
	}
	response, err := decodeNullableJSON(row.Response)
	if err != nil {
		return Detail{}, fmt.Errorf("decode LLM log response: %w", err)
	}
	return Detail{Log: row.Log, RequestPayload: requestPayload, Response: response}, nil
}

func decodeNullableJSON(value sql.NullString) (json.RawMessage, error) {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil, nil
	}
	raw := json.RawMessage(value.String)
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid persisted JSON")
	}
	return raw, nil
}
