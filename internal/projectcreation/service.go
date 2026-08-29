package projectcreation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/project"
	"lumi/internal/provider"

	"github.com/google/uuid"
)

const (
	StatusPending              = "pending"
	StatusCreatingProject      = "creating_project"
	StatusCreatingConversation = "creating_conversation"
	StatusActive               = "active"
	StatusFailed               = "failed"
	StatusCancelled            = "cancelled"

	CodeInvalidInput        = "project_creation_invalid"
	CodeIdempotencyConflict = "project_creation_idempotency_conflict"
	CodeNotFound            = "project_creation_session_not_found"
)

type Error struct {
	Code, Message, Details string
	Cause                  error
}

func (err *Error) Error() string {
	if err.Cause != nil {
		return fmt.Sprintf("%s: %v", err.Message, err.Cause)
	}
	return err.Message
}
func (err *Error) Unwrap() error { return err.Cause }

type Session struct {
	UUID         string     `json:"uuid"`
	Status       string     `json:"status"`
	ProjectUUID  string     `json:"project_uuid,omitempty"`
	ThreadUUID   string     `json:"thread_uuid,omitempty"`
	TurnUUID     string     `json:"turn_uuid,omitempty"`
	ErrorCode    string     `json:"error_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type Service struct {
	app      *appstore.Store
	projects projectCoordinator
	agents   agentBootstrapper
	now      func() time.Time
	mu       sync.Mutex
}

type projectCoordinator interface {
	PlanDraftProjectRoot(context.Context) (string, error)
	CreateDraftAt(context.Context, project.DraftCreateInput) (project.Summary, error)
	IsOpen(string) bool
	OpenRecent(context.Context, string) (project.Summary, error)
}

type agentBootstrapper interface {
	ValidateBootstrapTextModel(context.Context) error
	BootstrapConversation(context.Context, string, string, string) (agent.BootstrapConversationResult, error)
}

func NewService(app *appstore.Store, projects projectCoordinator, agents agentBootstrapper) *Service {
	return &Service{app: app, projects: projects, agents: agents, now: time.Now}
}

func newUUIDv7() (string, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return value.String(), nil
}

func validateCreateInput(inputText, idempotencyKey string) (string, string, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if strings.TrimSpace(inputText) == "" || !utf8.ValidString(inputText) || strings.ContainsRune(inputText, 0) || len(inputText) > 256<<10 {
		return "", "", &Error{Code: CodeInvalidInput, Message: "项目创建输入无效", Details: "input_text 必须是非空 UTF-8 文本且不超过 256 KiB。"}
	}
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 200 || !utf8.ValidString(idempotencyKey) || strings.ContainsRune(idempotencyKey, 0) {
		return "", "", &Error{Code: CodeInvalidInput, Message: "幂等键无效", Details: "idempotency_key 必须为 8 到 200 字节的有效文本。"}
	}
	return inputText, idempotencyKey, nil
}

func initialLanguage(input string) string {
	for _, value := range input {
		if unicode.Is(unicode.Han, value) {
			return project.GenerationLanguageSimplifiedChinese
		}
	}
	return project.GenerationLanguageEnglish
}

func publicSession(record appstore.ProjectCreationSession) Session {
	projectUUID := ""
	if record.RecentProjectID != nil {
		projectUUID = record.PlannedProjectUUID
	}
	return Session{
		UUID: record.UUID, Status: record.Status, ProjectUUID: projectUUID,
		ThreadUUID: record.ThreadUUID, TurnUUID: record.TurnUUID,
		ErrorCode: record.ErrorCode, ErrorMessage: record.ErrorMessage,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, CompletedAt: record.CompletedAt,
	}
}

func (service *Service) Create(ctx context.Context, inputText, idempotencyKey string) (Session, error) {
	inputText, idempotencyKey, err := validateCreateInput(inputText, idempotencyKey)
	if err != nil {
		return Session{}, err
	}
	sessionUUID, err := newUUIDv7()
	if err != nil {
		return Session{}, err
	}
	projectUUID, err := newUUIDv7()
	if err != nil {
		return Session{}, err
	}
	now := service.now().UTC()
	record, _, err := service.app.CreateOrGetProjectCreationSession(ctx, appstore.ProjectCreationSession{
		UUID: sessionUUID, IdempotencyKey: idempotencyKey, InputText: inputText,
		Status: StatusPending, PlannedProjectUUID: projectUUID, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return Session{}, err
	}
	if record.InputText != inputText {
		return Session{}, &Error{Code: CodeIdempotencyConflict, Message: "幂等键已用于另一条输入", Details: "请为不同的首页输入生成新的 idempotency_key。"}
	}
	return service.Resume(ctx, record.UUID)
}

func (service *Service) Get(ctx context.Context, sessionUUID string) (Session, error) {
	record, err := service.app.ProjectCreationSession(ctx, sessionUUID)
	if errors.Is(err, appstore.ErrProjectCreationSessionNotFound) {
		return Session{}, &Error{Code: CodeNotFound, Message: "项目创建会话不存在", Details: "该 UUID 不属于本机创建会话。", Cause: err}
	}
	if err != nil {
		return Session{}, err
	}
	return publicSession(record), nil
}

func (service *Service) Resume(ctx context.Context, sessionUUID string) (Session, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	record, err := service.app.ProjectCreationSession(ctx, sessionUUID)
	if errors.Is(err, appstore.ErrProjectCreationSessionNotFound) {
		return Session{}, &Error{Code: CodeNotFound, Message: "项目创建会话不存在", Details: "该 UUID 不属于本机创建会话。", Cause: err}
	}
	if err != nil {
		return Session{}, err
	}
	if record.Status == StatusActive || record.Status == StatusCancelled {
		return publicSession(record), nil
	}
	if err := service.agents.ValidateBootstrapTextModel(ctx); err != nil {
		return service.fail(ctx, record, err)
	}
	now := service.now().UTC()
	updates := map[string]any{"status": StatusCreatingProject, "attempt_count": record.AttemptCount + 1, "updated_at": now, "error_code": "", "error_message": "", "failed_at": nil}
	if record.PlannedRootPath == "" {
		root, err := service.projects.PlanDraftProjectRoot(ctx)
		if err != nil {
			return service.fail(ctx, record, err)
		}
		updates["planned_root_path"] = root
		record.PlannedRootPath = root
	}
	record, err = service.app.UpdateProjectCreationSession(ctx, record.UUID, updates)
	if err != nil {
		return Session{}, err
	}
	if record.RecentProjectID == nil {
		_, err = service.projects.CreateDraftAt(ctx, project.DraftCreateInput{
			ProjectUUID: record.PlannedProjectUUID, SetupUUID: record.UUID,
			RootPath: record.PlannedRootPath, InitialInput: record.InputText,
			GenerationLanguage: initialLanguage(record.InputText),
		})
		if err != nil {
			return service.fail(ctx, record, err)
		}
		recentID, err := service.app.RecentProjectID(ctx, record.PlannedProjectUUID)
		if err != nil {
			return service.fail(ctx, record, err)
		}
		record, err = service.app.UpdateProjectCreationSession(ctx, record.UUID, map[string]any{
			"recent_project_id": recentID, "status": StatusCreatingConversation, "updated_at": service.now().UTC(),
		})
		if err != nil {
			return Session{}, err
		}
	} else {
		if !service.projects.IsOpen(record.PlannedProjectUUID) {
			if _, err := service.projects.OpenRecent(ctx, record.PlannedProjectUUID); err != nil {
				return service.fail(ctx, record, err)
			}
		}
		record, err = service.app.UpdateProjectCreationSession(ctx, record.UUID, map[string]any{"status": StatusCreatingConversation, "updated_at": service.now().UTC()})
		if err != nil {
			return Session{}, err
		}
	}
	bootstrap, err := service.agents.BootstrapConversation(ctx, record.PlannedProjectUUID, record.UUID, record.InputText)
	if err != nil {
		return service.fail(ctx, record, err)
	}
	completed := service.now().UTC()
	record, err = service.app.UpdateProjectCreationSession(ctx, record.UUID, map[string]any{
		"status": StatusActive, "thread_uuid": bootstrap.Thread.UUID, "turn_uuid": bootstrap.Turn.UUID,
		"completed_at": completed, "updated_at": completed, "error_code": "", "error_message": "", "failed_at": nil,
	})
	if err != nil {
		return Session{}, err
	}
	return publicSession(record), nil
}

func (service *Service) fail(ctx context.Context, record appstore.ProjectCreationSession, cause error) (Session, error) {
	code, message := publicFailure(cause)
	now := service.now().UTC()
	failed, err := service.app.UpdateProjectCreationSession(context.WithoutCancel(ctx), record.UUID, map[string]any{
		"status": StatusFailed, "error_code": code, "error_message": message, "failed_at": now, "updated_at": now,
	})
	if err != nil {
		return Session{}, errors.Join(cause, err)
	}
	return publicSession(failed), nil
}

func publicFailure(err error) (string, string) {
	var projectErr *project.Error
	if errors.As(err, &projectErr) {
		return projectErr.Code, projectErr.Message
	}
	var agentErr *agent.Error
	if errors.As(err, &agentErr) {
		return agentErr.Code, agentErr.Message
	}
	var providerErr *provider.Error
	if errors.As(err, &providerErr) {
		return providerErr.Code, providerErr.Message
	}
	var creationErr *Error
	if errors.As(err, &creationErr) {
		return creationErr.Code, creationErr.Message
	}
	return "project_creation_failed", "项目创建暂时失败"
}

func (service *Service) Reconcile(ctx context.Context) error {
	records, err := service.app.ResumableProjectCreationSessions(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if _, resumeErr := service.Resume(ctx, record.UUID); resumeErr != nil && ctx.Err() == nil {
			return resumeErr
		}
	}
	return ctx.Err()
}
