package agent

import (
	"context"
	"database/sql"
	"strings"

	"lumi/internal/modelsettings"
	"lumi/internal/project"
)

type BootstrapConversationResult struct {
	Thread Thread `json:"thread"`
	Turn   Turn   `json:"turn"`
}

func (service *Service) ValidateBootstrapTextModel(ctx context.Context) error {
	if service == nil || service.providers == nil {
		return domainError(CodeProvider, "ChatArea 文本模型不可用", "Provider 服务尚未初始化。", nil)
	}
	resolved, err := service.providers.Active(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(resolved.DefaultModel) == "" {
		return domainError(CodeProvider, "ChatArea 文本模型不可用", "当前 Provider 没有配置默认文本模型。", nil)
	}
	return nil
}

func bootstrapThreadTitle(input string) string {
	input = strings.TrimSpace(input)
	if index := strings.IndexByte(input, '\n'); index >= 0 {
		input = strings.TrimSpace(input[:index])
	}
	runes := []rune(input)
	if len(runes) > 36 {
		input = string(runes[:36]) + "…"
	}
	if input == "" {
		return "项目初始化"
	}
	return input
}

func (service *Service) BootstrapConversation(ctx context.Context, projectUUID, creationSessionUUID, inputText string, referenceInputs []ReferenceInput) (BootstrapConversationResult, error) {
	_, err := validateText(inputText, 256<<10, "用户消息")
	if err != nil {
		return BootstrapConversationResult{}, err
	}
	text := inputText
	if !isUUIDv7(creationSessionUUID) {
		return BootstrapConversationResult{}, domainError(CodeValidation, "创建会话 UUID 无效", "creation_session_uuid 必须是 UUIDv7。", nil)
	}
	var result BootstrapConversationResult
	err = service.withStore(ctx, projectUUID, func(store *project.Store) error {
		pid, err := projectID(ctx, store.DB(), projectUUID)
		if err != nil {
			return err
		}
		if existing, found, err := loadBootstrapConversation(ctx, store, pid, projectUUID, creationSessionUUID); err != nil {
			return err
		} else if found {
			result = existing
			return nil
		}
		if store.SetupStatus() != project.SetupStatusDraft {
			return domainError(project.CodeProjectSetupIncomplete, "项目不处于草稿设置阶段", "只能为 draft 项目创建首页 bootstrap 对话。", nil)
		}
		resolved, model, modelSource, err := service.resolveTextModel(ctx, store, modelsettings.ChatArea, "", "")
		if err != nil {
			return err
		}
		promptSnapshot, err := service.loadContextPrompts(ctx, store, threadRecord{ThreadType: ThreadTypeConversation})
		if err != nil {
			return err
		}
		references, err := service.resolveContextReferences(ctx, store, pid, referenceInputs)
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
		// Recheck inside the same project transaction so a retry cannot create a
		// second Thread even when the application checkpoint write was lost.
		if existing, found, err := loadBootstrapConversationSQL(ctx, tx, pid, projectUUID, creationSessionUUID); err != nil {
			return err
		} else if found {
			result = existing
			return tx.Commit()
		}
		threadUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		now := service.now().UTC()
		insert, err := tx.ExecContext(ctx, `INSERT INTO chat_threads(
			uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,
			next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at
		) VALUES(?,?,?,'idle','conversation',?,?,?,1,1,1,?,?)`,
			threadUUID, pid, bootstrapThreadTitle(text), resolved.UUID, model, modelSource, now, now)
		if err != nil {
			return err
		}
		threadID, err := insert.LastInsertId()
		if err != nil {
			return err
		}
		thread, err := lockThreadSQL(ctx, tx, pid, threadUUID)
		if err != nil {
			return err
		}
		thread.ID, thread.ThreadType = threadID, ThreadTypeConversation
		turn, _, err := service.createTurnTx(ctx, tx, projectUUID, &thread, text, "prompt", 0, promptSnapshot, references)
		if err != nil {
			return err
		}
		bootstrapUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_creation_bootstraps(
			uuid,project_id,creation_session_uuid,thread_id,turn_id,created_at
		) VALUES(?,?,?,?,?,?)`, bootstrapUUID, pid, creationSessionUUID, threadID, turn.ID, now); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		result = BootstrapConversationResult{
			Thread: Thread{UUID: threadUUID, ProjectUUID: projectUUID, Title: bootstrapThreadTitle(text), Status: ThreadBusy, ThreadType: ThreadTypeConversation, ProviderUUID: resolved.UUID, Model: model, ModelSource: modelSource, CreatedAt: now, UpdatedAt: now},
			Turn:   turnDTO(turn, threadUUID, ""),
		}
		return nil
	})
	if err == nil {
		service.broadcastThread(projectUUID, result.Thread.UUID, "chat:turn_queued", map[string]any{"project_uuid": projectUUID, "thread_uuid": result.Thread.UUID, "turn_uuid": result.Turn.UUID, "status": result.Turn.Status})
	}
	return result, err
}

func loadBootstrapConversation(ctx context.Context, store *project.Store, projectID int64, projectUUID, sessionUUID string) (BootstrapConversationResult, bool, error) {
	sqlDB, err := store.DB().DB()
	if err != nil {
		return BootstrapConversationResult{}, false, err
	}
	return loadBootstrapConversationQuery(ctx, sqlDB, projectID, projectUUID, sessionUUID)
}

func loadBootstrapConversationSQL(ctx context.Context, tx *sql.Tx, projectID int64, projectUUID, sessionUUID string) (BootstrapConversationResult, bool, error) {
	return loadBootstrapConversationQuery(ctx, tx, projectID, projectUUID, sessionUUID)
}

type bootstrapQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadBootstrapConversationQuery(ctx context.Context, queryer bootstrapQueryer, projectID int64, projectUUID, sessionUUID string) (BootstrapConversationResult, bool, error) {
	var thread threadRecord
	var turn turnRecord
	err := queryer.QueryRowContext(ctx, `SELECT
		th.id,th.uuid,th.project_id,th.title,th.status,th.thread_type,th.provider_uuid,th.model,th.model_source,th.created_at,th.updated_at,
		t.id,t.uuid,t.thread_id,t.source_type,t.queue_sequence,t.input_text,t.status,t.error_code,t.error_message,
		t.cancel_requested_at,t.started_at,t.completed_at,t.created_at,t.updated_at
		FROM project_creation_bootstraps b
		JOIN chat_threads th ON th.id=b.thread_id
		JOIN chat_turns t ON t.id=b.turn_id
		WHERE b.project_id=? AND b.creation_session_uuid=?`, projectID, sessionUUID).Scan(
		&thread.ID, &thread.UUID, &thread.ProjectID, &thread.Title, &thread.Status, &thread.ThreadType,
		&thread.ProviderUUID, &thread.Model, &thread.ModelSource, &thread.CreatedAt, &thread.UpdatedAt,
		&turn.ID, &turn.UUID, &turn.ThreadID, &turn.SourceType, &turn.QueueSequence, &turn.InputText, &turn.Status,
		&turn.ErrorCode, &turn.ErrorMessage, &turn.CancelRequestedAt, &turn.StartedAt, &turn.CompletedAt, &turn.CreatedAt, &turn.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return BootstrapConversationResult{}, false, nil
	}
	if err != nil {
		return BootstrapConversationResult{}, false, err
	}
	return BootstrapConversationResult{Thread: threadDTO(thread, projectUUID), Turn: turnDTO(turn, thread.UUID, "")}, true, nil
}
