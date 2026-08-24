package agent

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"lumi/internal/project"

	"gorm.io/gorm"
)

var (
	diagnosticBearerPattern      = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]+`)
	diagnosticSecretPattern      = regexp.MustCompile(`(?i)((?:api[_ -]?key|authorization|access[_ -]?token|refresh[_ -]?token|cookie|password|secret)\s*[:=]\s*)[^\s,;]+`)
	diagnosticUnixPathPattern    = regexp.MustCompile(`(?m)(^|[\s"'\x60(])(?:file://)?/(?:Users|Volumes|home|root|private|var|tmp|opt|etc|mnt|srv|workspace)/[^\s"'\x60)]+`)
	diagnosticWindowsPathPattern = regexp.MustCompile(`(?i)(^|[\s"'\x60(])[a-z]:\\[^\s"'\x60)]+`)
)

func sanitizeDiagnosticText(value string) string {
	value = diagnosticBearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = diagnosticSecretPattern.ReplaceAllString(value, "${1}[REDACTED]")
	value = diagnosticUnixPathPattern.ReplaceAllString(value, "${1}[REDACTED_PATH]")
	value = diagnosticWindowsPathPattern.ReplaceAllString(value, "${1}[REDACTED_PATH]")
	if len(value) > 16<<10 {
		value = value[:16<<10] + "…"
	}
	return value
}

func sanitizeDiagnosticJSON(value string) json.RawMessage {
	var decoded any
	if json.Unmarshal([]byte(value), &decoded) != nil {
		return json.RawMessage("{}")
	}
	sanitized := sanitizeDiagnosticValue(decoded, 0)
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		return json.RawMessage("{}")
	}
	return encoded
}

func sanitizeDiagnosticValue(value any, depth int) any {
	if depth > 8 {
		return "[truncated]"
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := map[string]any{}
		for _, key := range keys {
			if len(result) >= 100 || diagnosticKeyBlocked(key) {
				continue
			}
			candidate := typed[key]
			canonicalKey := canonicalDiagnosticKey(key)
			if canonicalKey == "uuid" || strings.HasSuffix(canonicalKey, "_uuid") {
				text, ok := candidate.(string)
				if !ok || !isUUIDv7(text) {
					continue
				}
			}
			if canonicalKey == "uuids" || strings.HasSuffix(canonicalKey, "_uuids") {
				values, ok := candidate.([]any)
				if !ok {
					if stringsValue, stringsOK := candidate.([]string); stringsOK {
						values = make([]any, len(stringsValue))
						for index := range stringsValue {
							values[index] = stringsValue[index]
						}
					} else {
						continue
					}
				}
				valid := true
				for _, raw := range values {
					text, ok := raw.(string)
					valid = valid && ok && isUUIDv7(text)
				}
				if !valid {
					continue
				}
			}
			result[key] = sanitizeDiagnosticValue(candidate, depth+1)
		}
		return result
	case map[string]string:
		converted := make(map[string]any, len(typed))
		for key, item := range typed {
			converted[key] = item
		}
		return sanitizeDiagnosticValue(converted, depth)
	case []any:
		limit := len(typed)
		if limit > 100 {
			limit = 100
		}
		result := make([]any, 0, limit)
		for _, item := range typed[:limit] {
			result = append(result, sanitizeDiagnosticValue(item, depth+1))
		}
		return result
	case []string:
		result := make([]any, 0, len(typed))
		for index, item := range typed {
			if index >= 100 {
				break
			}
			result = append(result, sanitizeDiagnosticValue(item, depth+1))
		}
		return result
	case string:
		return sanitizeDiagnosticText(typed)
	case nil, bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return typed
	default:
		return nil
	}
}

func diagnosticKeyBlocked(key string) bool {
	canonical := canonicalDiagnosticKey(key)
	if canonical == "id" || strings.HasSuffix(canonical, "_id") || canonical == "path" || strings.HasSuffix(canonical, "_path") || canonical == "root" || canonical == "directory" || canonical == "dir" || strings.HasSuffix(canonical, "_directory") || strings.HasSuffix(canonical, "_dir") {
		return true
	}
	if canonical == "token" || strings.HasSuffix(canonical, "_token") {
		return true
	}
	for _, fragment := range []string{"authorization", "api_key", "apikey", "secret", "credential", "password", "cookie", "access_token", "refresh_token", "bearer"} {
		if strings.Contains(canonical, fragment) {
			return true
		}
	}
	return false
}

func canonicalDiagnosticKey(key string) string {
	characters := []rune(strings.TrimSpace(key))
	var builder strings.Builder
	lastUnderscore := false
	for index, char := range characters {
		if unicode.IsUpper(char) {
			previousLowerOrDigit := index > 0 && (unicode.IsLower(characters[index-1]) || unicode.IsDigit(characters[index-1]))
			nextLower := index+1 < len(characters) && unicode.IsLower(characters[index+1])
			if builder.Len() > 0 && !lastUnderscore && (previousLowerOrDigit || nextLower) {
				builder.WriteByte('_')
			}
			builder.WriteRune(unicode.ToLower(char))
			lastUnderscore = false
			continue
		}
		if char == '-' || char == '.' || char == ' ' {
			if builder.Len() > 0 && !lastUnderscore {
				builder.WriteByte('_')
				lastUnderscore = true
			}
			continue
		}
		builder.WriteRune(unicode.ToLower(char))
		lastUnderscore = char == '_'
	}
	return strings.Trim(builder.String(), "_")
}

func publicDiagnosticErrorMessage(code string) string {
	switch code {
	case "":
		return ""
	case CodeCancelled:
		return "用户已取消。"
	case CodeInterrupted:
		return "任务已中断，可以安全重试。"
	default:
		return "步骤未完成，请根据错误码查看关联调用日志。"
	}
}

func (service *Service) diagnosticWorkflow(ctx context.Context, store *project.Store, projectUUID, workflowUUID string) (workflowRecord, error) {
	if !isUUIDv7(workflowUUID) {
		return workflowRecord{}, domainError(CodeValidation, "Workflow UUID 无效", "workflow_uuid 必须是 UUIDv7。", nil)
	}
	pid, err := projectID(ctx, store.DB(), projectUUID)
	if err != nil {
		return workflowRecord{}, err
	}
	var workflow workflowRecord
	if err := store.DB().WithContext(ctx).Where("project_id=? AND uuid=?", pid, workflowUUID).First(&workflow).Error; err != nil {
		return workflowRecord{}, notFound(err, "Workflow 不存在")
	}
	return workflow, nil
}

func (service *Service) ListWorkflowRuns(ctx context.Context, projectUUID, workflowUUID, before string, limit int) (CursorPage[WorkflowDiagnosticRun], error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	beforePosition, err := decodeCursor(before)
	if err != nil {
		return CursorPage[WorkflowDiagnosticRun]{}, err
	}
	page := CursorPage[WorkflowDiagnosticRun]{Items: []WorkflowDiagnosticRun{}, CursorPagination: CursorPagination{PerPage: limit}}
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		workflow, err := service.diagnosticWorkflow(ctx, store, projectUUID, workflowUUID)
		if err != nil {
			return err
		}
		query := store.DB().WithContext(ctx).Where("workflow_id=?", workflow.ID)
		if beforePosition > 0 {
			query = query.Where("position<?", beforePosition)
		}
		var rows []workflowStepRecord
		if err := query.Order("position DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		page.CursorPagination.HasMore = len(rows) > limit
		if len(rows) > limit {
			rows = rows[:limit]
		}
		for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
			rows[left], rows[right] = rows[right], rows[left]
		}
		attemptByStep := make(map[int64]int, len(rows))
		stepIDs := make([]int64, 0, len(rows))
		for _, row := range rows {
			stepIDs = append(stepIDs, row.ID)
		}
		if len(stepIDs) > 0 {
			var attempts []struct {
				StepID  int64
				Attempt int
			}
			if err := store.DB().WithContext(ctx).Table("workflow_steps AS steps").
				Select(`steps.id AS step_id,CASE WHEN COALESCE(MAX(logs.attempt),0)>0 THEN MAX(logs.attempt) ELSE 1 END AS attempt`).
				Joins("LEFT JOIN task_runs tasks ON tasks.project_id=? AND tasks.uuid=steps.task_uuid", workflow.ProjectID).
				Joins("LEFT JOIN llm_logs logs ON logs.workflow_step_id=steps.id OR (logs.source_type='story_generation' AND logs.task_run_id=tasks.id)").
				Where("steps.id IN ?", stepIDs).Group("steps.id").Scan(&attempts).Error; err != nil {
				return err
			}
			for _, attempt := range attempts {
				attemptByStep[attempt.StepID] = attempt.Attempt
			}
		}
		progressByTask := make(map[string]int, len(rows))
		taskUUIDs := make([]string, 0, len(rows))
		for _, row := range rows {
			if row.TaskUUID != "" {
				taskUUIDs = append(taskUUIDs, row.TaskUUID)
			}
		}
		if len(taskUUIDs) > 0 {
			var tasks []struct {
				UUID     string
				Progress int
			}
			if err := store.DB().WithContext(ctx).Table("task_runs").Select("uuid,progress").Where("project_id=? AND uuid IN ?", workflow.ProjectID, taskUUIDs).Scan(&tasks).Error; err != nil {
				return err
			}
			for _, task := range tasks {
				progressByTask[task.UUID] = task.Progress
			}
		}
		for _, row := range rows {
			progress, found := progressByTask[row.TaskUUID]
			if !found && row.Status == WorkflowCompleted {
				progress = 100
			}
			page.Items = append(page.Items, WorkflowDiagnosticRun{UUID: row.UUID, WorkflowUUID: workflow.UUID, StepUUID: row.UUID, StepKey: row.StepKey, Attempt: attemptByStep[row.ID], Status: row.Status, Progress: progress, TaskUUID: row.TaskUUID, ResourceUUID: row.ResourceUUID, ErrorCode: row.ErrorCode, StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
		}
		if len(rows) > 0 {
			page.CursorPagination.PrevCursor = encodeCursor(int64(rows[0].Position))
			page.CursorPagination.NextCursor = encodeCursor(int64(rows[len(rows)-1].Position))
		}
		return nil
	})
	return page, err
}

func (service *Service) ListWorkflowEvents(ctx context.Context, projectUUID, workflowUUID, before, after string, limit int) (CursorPage[WorkflowDiagnosticEvent], error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	beforeSequence, err := decodeCursor(before)
	if err != nil {
		return CursorPage[WorkflowDiagnosticEvent]{}, err
	}
	afterSequence, err := decodeCursor(after)
	if err != nil {
		return CursorPage[WorkflowDiagnosticEvent]{}, err
	}
	if beforeSequence > 0 && afterSequence > 0 {
		return CursorPage[WorkflowDiagnosticEvent]{}, domainError(CodeValidation, "Cursor 参数冲突", "before 与 after 不能同时提供。", nil)
	}
	page := CursorPage[WorkflowDiagnosticEvent]{Items: []WorkflowDiagnosticEvent{}, CursorPagination: CursorPagination{PerPage: limit}}
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		workflow, err := service.diagnosticWorkflow(ctx, store, projectUUID, workflowUUID)
		if err != nil {
			return err
		}
		var rows []struct {
			UUID, StepUUID, EventType, PayloadJSON string
			Sequence                               int64
			CreatedAt                              time.Time
		}
		query := store.DB().WithContext(ctx).Table("workflow_events AS events").Select("events.uuid,COALESCE(steps.uuid,'') AS step_uuid,events.sequence,events.event_type,events.payload_json,events.created_at").Joins("LEFT JOIN workflow_steps steps ON steps.id=events.step_id").Where("events.workflow_id=?", workflow.ID)
		descending := beforeSequence > 0 || (beforeSequence == 0 && afterSequence == 0)
		if beforeSequence > 0 {
			query = query.Where("events.sequence<?", beforeSequence)
		}
		if afterSequence > 0 {
			query = query.Where("events.sequence>?", afterSequence)
		}
		if descending {
			query = query.Order("events.sequence DESC")
		} else {
			query = query.Order("events.sequence ASC")
		}
		if err := query.Limit(limit + 1).Scan(&rows).Error; err != nil {
			return err
		}
		page.CursorPagination.HasMore = len(rows) > limit
		if len(rows) > limit {
			rows = rows[:limit]
		}
		if descending {
			for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
				rows[left], rows[right] = rows[right], rows[left]
			}
		}
		for _, row := range rows {
			page.Items = append(page.Items, WorkflowDiagnosticEvent{UUID: row.UUID, WorkflowUUID: workflow.UUID, StepUUID: row.StepUUID, Sequence: row.Sequence, EventType: row.EventType, Payload: sanitizeDiagnosticJSON(row.PayloadJSON), CreatedAt: row.CreatedAt})
		}
		if len(rows) > 0 {
			page.CursorPagination.PrevCursor = encodeCursor(rows[0].Sequence)
			page.CursorPagination.NextCursor = encodeCursor(rows[len(rows)-1].Sequence)
		}
		return nil
	})
	return page, err
}

func (service *Service) ListWorkflowLLMLogs(ctx context.Context, projectUUID, workflowUUID, workflowStepUUID string, currentPage, perPage int) (WorkflowLLMLogPage, error) {
	if currentPage < 1 {
		currentPage = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	page := WorkflowLLMLogPage{Items: []WorkflowLLMLog{}, Pagination: PagePagination{PerPage: perPage, CurrentPage: currentPage}}
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		workflow, err := service.diagnosticWorkflow(ctx, store, projectUUID, workflowUUID)
		if err != nil {
			return err
		}
		workflowStepUUID = strings.TrimSpace(workflowStepUUID)
		if workflowStepUUID != "" {
			if !isUUIDv7(workflowStepUUID) {
				return domainError(CodeValidation, "Workflow step UUID 无效", "workflow_step_uuid 必须是 UUIDv7。", nil)
			}
			var count int64
			if err := store.DB().WithContext(ctx).Model(&workflowStepRecord{}).Where("workflow_id=? AND uuid=?", workflow.ID, workflowStepUUID).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return domainError(CodeNotFound, "Workflow step 不存在", "workflow_step_uuid 不属于当前 workflow。", nil)
			}
		}
		queryLogs := func() *gorm.DB {
			query := store.DB().WithContext(ctx).Table("workflow_steps AS steps").
				Joins("LEFT JOIN task_runs tasks ON tasks.project_id=? AND tasks.uuid=steps.task_uuid", workflow.ProjectID).
				Joins("JOIN llm_logs logs ON logs.workflow_step_id=steps.id OR (logs.source_type='story_generation' AND logs.task_run_id=tasks.id)").
				Where("steps.workflow_id=?", workflow.ID)
			if workflowStepUUID != "" {
				query = query.Where("steps.uuid=?", workflowStepUUID)
			}
			return query
		}
		if err := queryLogs().Count(&page.Pagination.Total).Error; err != nil {
			return err
		}
		page.Pagination.LastPage = int((page.Pagination.Total + int64(perPage) - 1) / int64(perPage))
		if page.Pagination.LastPage < 1 {
			page.Pagination.LastPage = 1
		}
		return queryLogs().Select(`logs.uuid,? AS workflow_uuid,steps.uuid AS workflow_step_uuid,logs.scenario,logs.request_type,logs.attempt,logs.model,logs.status,logs.input_tokens,logs.output_tokens,logs.duration_ms,logs.error_code,logs.created_at,logs.completed_at`, workflow.UUID).Order("logs.created_at DESC,logs.id DESC").Limit(perPage).Offset((currentPage - 1) * perPage).Scan(&page.Items).Error
	})
	return page, err
}
