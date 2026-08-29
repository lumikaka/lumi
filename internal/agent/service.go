package agent

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/modelsettings"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/realtime"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	projects             *project.Manager
	providers            *provider.Service
	models               *modelsettings.Resolver
	model                llm.ToolClient
	image                imagegen.Client
	queue                Queue
	hub                  *realtime.Hub
	projectAPIRoutes     []agentAPIRoute
	projectAPIDispatcher ProjectAPIDispatcher
	turnBudget           turnBudgetLimits
	now                  func() time.Time
}

type turnBudgetLimits struct {
	MaxModelRequests    int
	MaxActiveDurationMS int64
	MaxTokenUnits       int64
	MaxNoProgressRounds int
}

var defaultTurnBudgetLimits = turnBudgetLimits{
	MaxModelRequests:    DefaultMaxModelRequests,
	MaxActiveDurationMS: DefaultMaxActiveDurationMS,
	MaxTokenUnits:       DefaultMaxTokenUnits,
	MaxNoProgressRounds: DefaultMaxNoProgressRounds,
}

func NewService(projects *project.Manager, providers *provider.Service, model llm.ToolClient, queue Queue, hub *realtime.Hub) *Service {
	return &Service{projects: projects, providers: providers, models: modelsettings.NewResolver(providers), model: model, queue: queue, hub: hub, turnBudget: defaultTurnBudgetLimits, now: time.Now}
}

func (service *Service) WithImageClient(client imagegen.Client) *Service {
	if client != nil {
		service.image = client
	}
	return service
}

func newUUIDv7() (string, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate UUIDv7: %w", err)
	}
	return value.String(), nil
}

func isUUIDv7(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.Version() == 7
}

func validateText(value string, maxBytes int, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return "", domainError(CodeValidation, field+"不能为空", "请输入有效 UTF-8 文本。", nil)
	}
	if len(value) > maxBytes {
		return "", domainError(CodeValidation, field+"过长", fmt.Sprintf("最多允许 %d 字节。", maxBytes), nil)
	}
	return value, nil
}

func (service *Service) withStore(ctx context.Context, projectUUID string, callback func(*project.Store) error) error {
	if service == nil || service.projects == nil {
		return domainError(CodeStateConflict, "Agent 服务不可用", "项目服务尚未初始化。", nil)
	}
	return service.projects.WithStore(ctx, projectUUID, callback)
}

func projectID(ctx context.Context, db *gorm.DB, projectUUID string) (int64, error) {
	var id int64
	if err := db.WithContext(ctx).Raw("SELECT id FROM projects WHERE uuid=?", projectUUID).Scan(&id).Error; err != nil || id == 0 {
		return 0, domainError(CodeNotFound, "项目不存在", "项目不是当前活动项目。", err)
	}
	return id, nil
}

func (service *Service) CreateThread(ctx context.Context, projectUUID string, input CreateThreadInput) (Thread, error) {
	title, err := validateText(input.Title, 160*4, "Thread 标题")
	if err != nil {
		return Thread{}, err
	}
	if len([]rune(title)) > 160 || (strings.TrimSpace(input.ProviderUUID) != "" && !isUUIDv7(strings.TrimSpace(input.ProviderUUID))) {
		return Thread{}, domainError(CodeValidation, "Thread 参数无效", "title 最多 160 个字符。", nil)
	}
	threadUUID, err := newUUIDv7()
	if err != nil {
		return Thread{}, err
	}
	now := service.now().UTC()
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		resolved, model, modelSource, err := service.resolveTextModel(ctx, store, modelsettings.ChatArea, input.ProviderUUID, input.Model)
		if err != nil {
			return err
		}
		record := threadRecord{UUID: threadUUID, ProjectID: pid, Title: title, Status: ThreadIdle, ThreadType: ThreadTypeConversation, ProviderUUID: resolved.UUID, Model: model, ModelSource: modelSource, NextTurnSequence: 1, NextItemSequence: 1, NextEventSequence: 1, CreatedAt: now, UpdatedAt: now}
		return store.DB().WithContext(ctx).Create(&record).Error
	})
	if err != nil {
		return Thread{}, err
	}
	return service.GetThread(ctx, projectUUID, threadUUID)
}

func (service *Service) ListThreads(ctx context.Context, projectUUID string) ([]Thread, error) {
	page, err := service.ListThreadsPage(ctx, projectUUID, 1, 100)
	return page.Items, err
}

func (service *Service) ListThreadsPage(ctx context.Context, projectUUID string, currentPage, perPage int) (ThreadPage, error) {
	if currentPage < 1 {
		currentPage = 1
	}
	if perPage < 1 {
		perPage = 30
	}
	if perPage > 100 {
		perPage = 100
	}
	result := ThreadPage{Items: []Thread{}, Pagination: PagePagination{PerPage: perPage, CurrentPage: currentPage}}
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		query := store.DB().WithContext(ctx).Where("project_id=? AND archived_at IS NULL", pid)
		if err := query.Model(&threadRecord{}).Count(&result.Pagination.Total).Error; err != nil {
			return err
		}
		result.Pagination.LastPage = int((result.Pagination.Total + int64(perPage) - 1) / int64(perPage))
		if result.Pagination.LastPage < 1 {
			result.Pagination.LastPage = 1
		}
		var rows []threadRecord
		if err := query.Order("updated_at DESC, id DESC").Limit(perPage).Offset((currentPage - 1) * perPage).Find(&rows).Error; err != nil {
			return err
		}
		result.Items = make([]Thread, 0, len(rows))
		for _, row := range rows {
			result.Items = append(result.Items, threadDTO(row, projectUUID))
		}
		return nil
	})
	return result, err
}

func (service *Service) GetThread(ctx context.Context, projectUUID, threadUUID string) (Thread, error) {
	if !isUUIDv7(threadUUID) {
		return Thread{}, domainError(CodeValidation, "Thread UUID 无效", "thread_uuid 必须是 UUIDv7。", nil)
	}
	var result Thread
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		var row threadRecord
		if err := store.DB().WithContext(ctx).Where("project_id=? AND uuid=?", pid, threadUUID).First(&row).Error; err != nil {
			return notFound(err, "Chat thread 不存在")
		}
		result = threadDTO(row, projectUUID)
		return nil
	})
	return result, err
}

func threadDTO(row threadRecord, projectUUID string) Thread {
	return Thread{UUID: row.UUID, ProjectUUID: projectUUID, Title: row.Title, Status: row.Status, ThreadType: row.ThreadType, ProviderUUID: row.ProviderUUID, Model: row.Model, ModelSource: row.ModelSource, ArchivedAt: row.ArchivedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func notFound(err error, message string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		return domainError(CodeNotFound, message, "资源不存在或不属于当前项目。", err)
	}
	return err
}

func (service *Service) CreateTurn(ctx context.Context, projectUUID, threadUUID string, input CreateTurnInput) (Turn, error) {
	text, err := validateText(input.InputText, 256<<10, "用户消息")
	if err != nil {
		return Turn{}, err
	}
	if !isUUIDv7(threadUUID) {
		return Turn{}, domainError(CodeValidation, "Thread UUID 无效", "thread_uuid 必须是 UUIDv7。", nil)
	}
	var created Turn
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		var promptThread threadRecord
		if err := store.DB().WithContext(ctx).
			Where("project_id = (SELECT id FROM projects WHERE uuid = ?) AND uuid = ? AND archived_at IS NULL", projectUUID, threadUUID).
			First(&promptThread).Error; err != nil {
			return notFound(err, "Chat thread 不存在")
		}
		promptSnapshot, err := service.loadContextPrompts(ctx, store, promptThread)
		if err != nil {
			return err
		}
		resolved, model, modelSource, err := service.resolveTextModel(ctx, store, modelsettings.ChatArea, "", "")
		if err != nil {
			return err
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
		pid, err := projectIDSQL(ctx, tx, projectUUID)
		if err != nil {
			return err
		}
		thread, err := lockThreadSQL(ctx, tx, pid, threadUUID)
		if err != nil {
			return err
		}
		thread.ProviderUUID = resolved.UUID
		thread.Model = model
		thread.ModelSource = modelSource
		turn, _, err := service.createTurnTx(ctx, tx, projectUUID, &thread, text, "prompt", 0, promptSnapshot, references)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		created = turnDTO(turn, threadUUID, "")
		return nil
	})
	if err == nil {
		service.broadcastThread(projectUUID, threadUUID, "chat:turn_queued", map[string]any{"project_uuid": projectUUID, "thread_uuid": threadUUID, "turn_uuid": created.UUID, "status": created.Status})
	}
	return created, err
}

func projectIDSQL(ctx context.Context, tx *sql.Tx, projectUUID string) (int64, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM projects WHERE uuid=?", projectUUID).Scan(&id); err != nil {
		return 0, notFound(err, "项目不存在")
	}
	return id, nil
}

func lockThreadSQL(ctx context.Context, tx *sql.Tx, projectID int64, threadUUID string) (threadRecord, error) {
	var row threadRecord
	err := tx.QueryRowContext(ctx, `SELECT id,uuid,project_id,title,status,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,archived_at,created_at,updated_at FROM chat_threads WHERE project_id=? AND uuid=? AND archived_at IS NULL`, projectID, threadUUID).
		Scan(&row.ID, &row.UUID, &row.ProjectID, &row.Title, &row.Status, &row.ProviderUUID, &row.Model, &row.ModelSource, &row.NextTurnSequence, &row.NextItemSequence, &row.NextEventSequence, &row.ArchivedAt, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		return row, notFound(err, "Chat thread 不存在")
	}
	return row, nil
}

func (service *Service) createTurnTx(ctx context.Context, tx *sql.Tx, projectUUID string, thread *threadRecord, text, sourceType string, followUpID int64, promptSnapshot contextPromptSet, references []storedContextReference) (turnRecord, runRecord, error) {
	now := service.now().UTC()
	queueSequence := thread.NextTurnSequence
	turnUUID, err := newUUIDv7()
	if err != nil {
		return turnRecord{}, runRecord{}, err
	}
	runUUID, err := newUUIDv7()
	if err != nil {
		return turnRecord{}, runRecord{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO chat_turns(uuid,thread_id,source_type,source_follow_up_id,queue_sequence,input_text,status,created_at,updated_at) VALUES(?,?,?,?,?,?,'queued',?,?)`, turnUUID, thread.ID, sourceType, nullableInt64(followUpID), queueSequence, text, now, now)
	if err != nil {
		return turnRecord{}, runRecord{}, err
	}
	turnID, err := result.LastInsertId()
	if err != nil {
		return turnRecord{}, runRecord{}, err
	}
	limits := service.turnBudget
	result, err = tx.ExecContext(ctx, `INSERT INTO chat_runs(uuid,thread_id,turn_id,trigger_type,status,provider_uuid,model,model_source,max_model_requests,max_active_duration_ms,max_token_units,max_no_progress_rounds,created_at,updated_at) VALUES(?,?,?,?,'queued',?,?,?,?,?,?,?,?,?)`, runUUID, thread.ID, turnID, sourceType, thread.ProviderUUID, thread.Model, thread.ModelSource, limits.MaxModelRequests, limits.MaxActiveDurationMS, limits.MaxTokenUnits, limits.MaxNoProgressRounds, now, now)
	if err != nil {
		return turnRecord{}, runRecord{}, err
	}
	runID, err := result.LastInsertId()
	if err != nil {
		return turnRecord{}, runRecord{}, err
	}
	userItem, err := appendItemTx(ctx, tx, thread, &turnID, &runID, "user_message", "user", text, "text", "completed", "", "", "", map[string]any{"source_type": sourceType, "prompt_snapshot": promptSnapshot}, now)
	if err != nil {
		return turnRecord{}, runRecord{}, err
	}
	if err := attachItemReferencesTx(ctx, tx, userItem.ID, references, now); err != nil {
		return turnRecord{}, runRecord{}, err
	}
	if _, err := appendEventTx(ctx, tx, thread, &runID, "turn_queued", map[string]any{"project_uuid": projectUUID, "thread_uuid": thread.UUID, "turn_uuid": turnUUID, "run_uuid": runUUID, "status": TurnQueued}, now); err != nil {
		return turnRecord{}, runRecord{}, err
	}
	jobID, err := service.queue.EnqueueAgentTx(ctx, projectUUID, tx, JobSpec{Version: 1, ProjectUUID: projectUUID, JobKind: JobChatTurn, ResourceUUID: turnUUID, ThreadUUID: thread.UUID})
	if err != nil {
		return turnRecord{}, runRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET river_job_id=? WHERE id=?`, jobID, turnID); err != nil {
		return turnRecord{}, runRecord{}, err
	}
	thread.NextTurnSequence++
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET provider_uuid=?,model=?,model_source=?,next_turn_sequence=?,next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.ProviderUUID, thread.Model, thread.ModelSource, thread.NextTurnSequence, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
		return turnRecord{}, runRecord{}, err
	}
	if _, err := RecomputeThreadStatusTx(ctx, tx, thread.ID, now); err != nil {
		return turnRecord{}, runRecord{}, err
	}
	turn := turnRecord{ID: turnID, ThreadID: thread.ID, UUID: turnUUID, SourceType: sourceType, SourceFollowUpID: followUpID, QueueSequence: queueSequence, InputText: text, Status: TurnQueued, RiverJobID: &jobID, CreatedAt: now, UpdatedAt: now}
	run := runRecord{ID: runID, ThreadID: thread.ID, TurnID: turnID, UUID: runUUID, TriggerType: sourceType, Status: TurnQueued, MaxModelRequests: limits.MaxModelRequests, MaxActiveDurationMS: limits.MaxActiveDurationMS, MaxTokenUnits: limits.MaxTokenUnits, MaxNoProgressRounds: limits.MaxNoProgressRounds, ProviderUUID: thread.ProviderUUID, Model: thread.Model, ModelSource: thread.ModelSource, CreatedAt: now, UpdatedAt: now}
	return turn, run, nil
}

func (service *Service) resolveProvider(ctx context.Context, providerUUID string) (provider.Resolved, error) {
	providerUUID = strings.TrimSpace(providerUUID)
	if providerUUID == "" {
		return service.providers.Active(ctx)
	}
	return service.providers.Resolve(ctx, providerUUID)
}

func (service *Service) resolveTextModel(ctx context.Context, store *project.Store, settingKey, providerUUID, model string) (provider.Resolved, string, string, error) {
	return service.resolveProjectModel(ctx, store, settingKey, modelsettings.KindText, providerUUID, model)
}

func (service *Service) resolveProjectModel(ctx context.Context, store *project.Store, settingKey, kind, providerUUID, model string) (provider.Resolved, string, string, error) {
	providerUUID = strings.TrimSpace(providerUUID)
	model = strings.TrimSpace(model)
	if providerUUID != "" {
		resolved, err := service.resolveProvider(ctx, providerUUID)
		if err != nil {
			return provider.Resolved{}, "", "", err
		}
		if model == "" {
			model = strings.TrimSpace(resolved.DefaultModel)
			if kind == modelsettings.KindImage {
				model = strings.TrimSpace(resolved.DefaultImageModel)
			}
		}
		if model == "" || len([]rune(model)) > 512 {
			return provider.Resolved{}, "", "", domainError(CodeValidation, "模型无效", "model 不能为空且最多 512 个字符。", nil)
		}
		return resolved, model, modelsettings.SourceExplicitTask, nil
	}
	choice, err := service.models.Resolve(ctx, store, settingKey, kind, "", model)
	if err != nil {
		var settingsErr *modelsettings.Error
		if errors.As(err, &settingsErr) {
			return provider.Resolved{}, "", "", domainError(CodeValidation, settingsErr.Message, settingsErr.Details, err)
		}
		return provider.Resolved{}, "", "", err
	}
	return choice.Provider, choice.Model, choice.Source, nil
}

func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func appendItemTx(ctx context.Context, tx *sql.Tx, thread *threadRecord, turnID, runID *int64, itemType, role, content, format, status, remoteUUID, toolName, targetUUID string, metadata any, now time.Time) (itemRecord, error) {
	uuid, err := newUUIDv7()
	if err != nil {
		return itemRecord{}, err
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return itemRecord{}, err
	}
	if !json.Valid(encoded) {
		return itemRecord{}, domainError(CodeValidation, "Item metadata 无效", "metadata 必须是有效 JSON。", nil)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO chat_items(uuid,thread_id,turn_id,run_id,sequence,item_type,role,content,content_format,status,remote_item_uuid,tool_name,target_uuid,metadata_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, uuid, thread.ID, turnID, runID, thread.NextItemSequence, itemType, role, content, format, status, remoteUUID, toolName, targetUUID, string(encoded), now)
	if err != nil {
		return itemRecord{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return itemRecord{}, err
	}
	row := itemRecord{ID: id, ThreadID: thread.ID, TurnID: turnID, RunID: runID, Sequence: thread.NextItemSequence, UUID: uuid, ItemType: itemType, Role: role, Content: content, ContentFormat: format, Status: status, RemoteItemUUID: remoteUUID, ToolName: toolName, TargetUUID: targetUUID, MetadataJSON: string(encoded), CreatedAt: now}
	thread.NextItemSequence++
	return row, nil
}

func appendEventTx(ctx context.Context, tx *sql.Tx, thread *threadRecord, runID *int64, eventType string, payload any, now time.Time) (Event, error) {
	uuid, err := newUUIDv7()
	if err != nil {
		return Event{}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO chat_events(uuid,thread_id,run_id,sequence,event_type,payload_json,created_at) VALUES(?,?,?,?,?,?,?)`, uuid, thread.ID, runID, thread.NextEventSequence, eventType, string(encoded), now); err != nil {
		return Event{}, err
	}
	event := Event{UUID: uuid, ThreadUUID: thread.UUID, Sequence: thread.NextEventSequence, EventType: eventType, Payload: encoded, CreatedAt: now}
	thread.NextEventSequence++
	return event, nil
}

func turnDTO(row turnRecord, threadUUID, followUpUUID string) Turn {
	return Turn{UUID: row.UUID, ThreadUUID: threadUUID, SourceType: row.SourceType, SourceFollowUpUUID: followUpUUID, QueueSequence: row.QueueSequence, InputText: row.InputText, Status: row.Status, ErrorCode: row.ErrorCode, ErrorMessage: row.ErrorMessage, CancelRequestedAt: row.CancelRequestedAt, StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func (service *Service) ListTurns(ctx context.Context, projectUUID, threadUUID string) ([]Turn, error) {
	var result []Turn
	err := service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		var thread threadRecord
		if err := store.DB().WithContext(ctx).Where("project_id=? AND uuid=?", pid, threadUUID).First(&thread).Error; err != nil {
			return notFound(err, "Chat thread 不存在")
		}
		var rows []turnRecord
		if err := store.DB().WithContext(ctx).Where("thread_id=?", thread.ID).Order("queue_sequence,id").Find(&rows).Error; err != nil {
			return err
		}
		var waitingRows []struct{ ChatTurnID int64 }
		if err := store.DB().WithContext(ctx).Table("workflow_awaits").Select("chat_turn_id").Where("chat_thread_id=? AND status='waiting'", thread.ID).Scan(&waitingRows).Error; err != nil {
			return err
		}
		waitingTurnIDs := make(map[int64]struct{}, len(waitingRows))
		for _, waiting := range waitingRows {
			waitingTurnIDs[waiting.ChatTurnID] = struct{}{}
		}
		result = make([]Turn, 0, len(rows))
		for _, row := range rows {
			var followUpUUID string
			if row.SourceFollowUpID > 0 {
				_ = store.DB().WithContext(ctx).Raw("SELECT uuid FROM chat_follow_ups WHERE id=?", row.SourceFollowUpID).Scan(&followUpUUID).Error
			}
			dto := turnDTO(row, threadUUID, followUpUUID)
			if _, waiting := waitingTurnIDs[row.ID]; waiting && row.Status == TurnInProgress {
				dto.Status = TurnWaitingForWorkflow
			}
			result = append(result, dto)
		}
		return nil
	})
	return result, err
}

func encodeCursor(sequence int64) string {
	if sequence <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", sequence)))
}

func decodeCursor(value string) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, domainError(CodeValidation, "Cursor 无效", "cursor 无法解析。", err)
	}
	var sequence int64
	if _, err := fmt.Sscanf(string(decoded), "%d", &sequence); err != nil || sequence <= 0 {
		return 0, domainError(CodeValidation, "Cursor 无效", "cursor 无法解析。", err)
	}
	return sequence, nil
}

func (service *Service) ListItems(ctx context.Context, projectUUID, threadUUID, before, after string, limit int) (CursorPage[Item], error) {
	if limit <= 0 {
		limit = DefaultItemsPage
	}
	if limit > 200 {
		limit = 200
	}
	beforeSequence, err := decodeCursor(before)
	if err != nil {
		return CursorPage[Item]{}, err
	}
	afterSequence, err := decodeCursor(after)
	if err != nil {
		return CursorPage[Item]{}, err
	}
	if beforeSequence > 0 && afterSequence > 0 {
		return CursorPage[Item]{}, domainError(CodeValidation, "Cursor 参数冲突", "before 与 after 不能同时提供。", nil)
	}
	page := CursorPage[Item]{Items: []Item{}, CursorPagination: CursorPagination{PerPage: limit}}
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		var thread threadRecord
		if err := store.DB().WithContext(ctx).Where("project_id=? AND uuid=?", pid, threadUUID).First(&thread).Error; err != nil {
			return notFound(err, "Chat thread 不存在")
		}
		query := store.DB().WithContext(ctx).Where("thread_id=?", thread.ID)
		directionDesc := beforeSequence > 0 || (beforeSequence == 0 && afterSequence == 0)
		if beforeSequence > 0 {
			query = query.Where("sequence < ?", beforeSequence)
		}
		if afterSequence > 0 {
			query = query.Where("sequence > ?", afterSequence)
		}
		if directionDesc {
			query = query.Order("sequence DESC")
		} else {
			query = query.Order("sequence ASC")
		}
		var rows []itemRecord
		if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		page.CursorPagination.HasMore = len(rows) > limit
		if len(rows) > limit {
			rows = rows[:limit]
		}
		if directionDesc {
			for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
				rows[left], rows[right] = rows[right], rows[left]
			}
		}
		for _, row := range rows {
			var turnUUID, runUUID string
			if row.TurnID != nil {
				_ = store.DB().WithContext(ctx).Raw("SELECT uuid FROM chat_turns WHERE id=?", *row.TurnID).Scan(&turnUUID).Error
			}
			if row.RunID != nil {
				_ = store.DB().WithContext(ctx).Raw("SELECT uuid FROM chat_runs WHERE id=?", *row.RunID).Scan(&runUUID).Error
			}
			item := itemDTO(row, threadUUID, turnUUID, runUUID)
			item.References, err = service.itemReferences(ctx, store, row.ID)
			if err != nil {
				return err
			}
			page.Items = append(page.Items, item)
		}
		if len(page.Items) > 0 {
			page.CursorPagination.PrevCursor = encodeCursor(page.Items[0].Sequence)
			page.CursorPagination.NextCursor = encodeCursor(page.Items[len(page.Items)-1].Sequence)
		}
		return nil
	})
	return page, err
}

func itemDTO(row itemRecord, threadUUID, turnUUID, runUUID string) Item {
	metadata := json.RawMessage(row.MetadataJSON)
	if !json.Valid(metadata) {
		metadata = json.RawMessage("{}")
	} else {
		var publicMetadata map[string]json.RawMessage
		if json.Unmarshal(metadata, &publicMetadata) == nil {
			delete(publicMetadata, "prompt_snapshot")
			if encoded, err := json.Marshal(publicMetadata); err == nil {
				metadata = sanitizeDiagnosticJSON(string(encoded))
			}
		}
	}
	return Item{UUID: row.UUID, ThreadUUID: threadUUID, TurnUUID: turnUUID, RunUUID: runUUID, Sequence: row.Sequence, ItemType: row.ItemType, Role: row.Role, Content: row.Content, ContentFormat: row.ContentFormat, Status: row.Status, ToolCallUUID: row.RemoteItemUUID, ToolName: row.ToolName, TargetUUID: row.TargetUUID, Metadata: metadata, CreatedAt: row.CreatedAt}
}

func (service *Service) ListEvents(ctx context.Context, projectUUID, threadUUID, after string, limit int) (CursorPage[Event], error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	sequence, err := decodeCursor(after)
	if err != nil {
		return CursorPage[Event]{}, err
	}
	page := CursorPage[Event]{Items: []Event{}, CursorPagination: CursorPagination{PerPage: limit}}
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		var rows []struct {
			UUID, ThreadUUID, RunUUID, EventType, PayloadJSON string
			Sequence                                          int64
			CreatedAt                                         time.Time
		}
		if err := store.DB().WithContext(ctx).Table("chat_events AS e").Select("e.uuid,th.uuid AS thread_uuid,COALESCE(r.uuid,'') AS run_uuid,e.sequence,e.event_type,e.payload_json,e.created_at").Joins("JOIN chat_threads th ON th.id=e.thread_id").Joins("LEFT JOIN chat_runs r ON r.id=e.run_id").Where("th.project_id=? AND th.uuid=? AND e.sequence>?", pid, threadUUID, sequence).Order("e.sequence").Limit(limit + 1).Scan(&rows).Error; err != nil {
			return err
		}
		page.CursorPagination.HasMore = len(rows) > limit
		if len(rows) > limit {
			rows = rows[:limit]
		}
		for _, row := range rows {
			page.Items = append(page.Items, Event{UUID: row.UUID, ThreadUUID: row.ThreadUUID, RunUUID: row.RunUUID, Sequence: row.Sequence, EventType: row.EventType, Payload: sanitizeDiagnosticJSON(row.PayloadJSON), CreatedAt: row.CreatedAt})
		}
		if len(page.Items) > 0 {
			page.CursorPagination.NextCursor = encodeCursor(page.Items[len(page.Items)-1].Sequence)
		}
		return nil
	})
	return page, err
}

func (service *Service) broadcastThread(projectUUID, threadUUID, event string, payload map[string]any) {
	if service.hub == nil {
		return
	}
	payload = publicRealtimePayload(payload)
	service.hub.Broadcast(realtime.ProjectTopic(projectUUID), event, payload)
	service.hub.Broadcast("thread:"+threadUUID, event, payload)
}

func publicRealtimePayload(payload map[string]any) map[string]any {
	result, ok := sanitizeDiagnosticValue(payload, 0).(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return result
}
