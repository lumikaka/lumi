package projectcreation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/files"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/realtime"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	StatusPending              = "pending"
	StatusCreatingProject      = "creating_project"
	StatusAwaitingReferences   = "awaiting_references"
	StatusCreatingConversation = "creating_conversation"
	StatusActive               = "active"
	StatusFailed               = "failed"
	StatusCancelled            = "cancelled"

	CodeInvalidInput        = "project_creation_invalid"
	CodeIdempotencyConflict = "project_creation_idempotency_conflict"
	CodeNotFound            = "project_creation_session_not_found"
	CodeReferenceNotFound   = "project_creation_reference_not_found"
	CodeReferenceNotReady   = "project_creation_reference_not_ready"
	MaxReferenceFiles       = 16
	MaxReferenceFileBytes   = int64(32 << 20)
	ReferenceRoleAuto       = "auto"
	ReferenceRoleCharacter  = "character"
	ReferenceRoleScene      = "scene"
	ReferenceRoleProp       = "prop"
	ReferenceRoleStyle      = "style"
	PlanSourceSystemDefault = "system_default"
	PlanSourceUserConfirmed = "user_confirmed"
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
	UUID         string      `json:"uuid"`
	Status       string      `json:"status"`
	ProjectUUID  string      `json:"project_uuid,omitempty"`
	ThreadUUID   string      `json:"thread_uuid,omitempty"`
	TurnUUID     string      `json:"turn_uuid,omitempty"`
	ErrorCode    string      `json:"error_code,omitempty"`
	ErrorMessage string      `json:"error_message,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	CompletedAt  *time.Time  `json:"completed_at,omitempty"`
	References   []Reference `json:"references"`
}

type ReferenceFileInput struct {
	OriginalFilename string `json:"original_filename"`
	MIMEType         string `json:"mime_type"`
	ByteSize         int64  `json:"byte_size"`
	ReferenceRole    string `json:"reference_role,omitempty"`
	Title            string `json:"title,omitempty"`
	Instruction      string `json:"instruction,omitempty"`
	IncludeInYolo    *bool  `json:"include_in_yolo,omitempty"`
}

type Reference struct {
	UUID             string `json:"uuid"`
	Position         int    `json:"position"`
	OriginalFilename string `json:"original_filename"`
	MIMEType         string `json:"mime_type"`
	ByteSize         int64  `json:"byte_size"`
	ReferenceRole    string `json:"reference_role"`
	Title            string `json:"title"`
	Instruction      string `json:"instruction,omitempty"`
	IncludeInYolo    bool   `json:"include_in_yolo"`
	PlanSource       string `json:"plan_source"`
	Status           string `json:"status"`
	FileUUID         string `json:"file_uuid,omitempty"`
	ErrorCode        string `json:"error_code,omitempty"`
}

type EventPublisher interface {
	Broadcast(topic, event string, payload any)
}

type Service struct {
	app      *appstore.Store
	projects projectCoordinator
	agents   agentBootstrapper
	events   EventPublisher
	now      func() time.Time
	mu       sync.Mutex
}

type projectCoordinator interface {
	PlanDraftProjectRoot(context.Context) (string, error)
	CreateDraftAt(context.Context, project.DraftCreateInput) (project.Summary, error)
	IsOpen(string) bool
	OpenRecent(context.Context, string) (project.Summary, error)
	WithStore(context.Context, string, func(*project.Store) error) error
}

type agentBootstrapper interface {
	ValidateBootstrapTextModel(context.Context) error
	BootstrapConversation(context.Context, string, string, string, []agent.ReferenceInput) (agent.BootstrapConversationResult, error)
}

func NewService(app *appstore.Store, projects projectCoordinator, agents agentBootstrapper, publishers ...EventPublisher) *Service {
	service := &Service{app: app, projects: projects, agents: agents, now: time.Now}
	if len(publishers) > 0 {
		service.events = publishers[0]
	}
	return service
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

func validateReferenceFiles(values []ReferenceFileInput) ([]ReferenceFileInput, error) {
	if len(values) > MaxReferenceFiles {
		return nil, &Error{Code: CodeInvalidInput, Message: "参考图过多", Details: "首页首条消息最多携带 16 张参考图。"}
	}
	allowedMIME := map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}
	allowedRoles := map[string]bool{ReferenceRoleAuto: true, ReferenceRoleCharacter: true, ReferenceRoleScene: true, ReferenceRoleProp: true, ReferenceRoleStyle: true}
	result := make([]ReferenceFileInput, 0, len(values))
	for _, value := range values {
		value.OriginalFilename = strings.TrimSpace(value.OriginalFilename)
		value.MIMEType = strings.ToLower(strings.TrimSpace(value.MIMEType))
		if value.OriginalFilename == "" || !utf8.ValidString(value.OriginalFilename) || strings.ContainsRune(value.OriginalFilename, 0) || len([]rune(value.OriginalFilename)) > 255 {
			return nil, &Error{Code: CodeInvalidInput, Message: "参考图文件名无效", Details: "original_filename 必须是 1 到 255 个字符的有效文本。"}
		}
		if !allowedMIME[value.MIMEType] {
			return nil, &Error{Code: CodeInvalidInput, Message: "参考图格式无效", Details: "参考图只支持 PNG、JPEG 或 WebP。"}
		}
		if value.ByteSize <= 0 || value.ByteSize > MaxReferenceFileBytes {
			return nil, &Error{Code: CodeInvalidInput, Message: "参考图大小无效", Details: "每张参考图必须大于 0 字节且不超过 32 MiB。"}
		}
		defaultTitle := strings.TrimSpace(strings.TrimSuffix(filepath.Base(value.OriginalFilename), filepath.Ext(value.OriginalFilename)))
		if defaultTitle == "" {
			defaultTitle = value.OriginalFilename
		}
		value.ReferenceRole = strings.ToLower(strings.TrimSpace(value.ReferenceRole))
		if value.ReferenceRole == "" {
			value.ReferenceRole = ReferenceRoleAuto
		}
		if !allowedRoles[value.ReferenceRole] {
			return nil, &Error{Code: CodeInvalidInput, Message: "参考图用途无效", Details: "reference_role 只支持 auto、character、scene、prop 或 style。"}
		}
		value.Title = strings.TrimSpace(value.Title)
		if value.Title == "" {
			value.Title = defaultTitle
		}
		value.Instruction = strings.TrimSpace(value.Instruction)
		if !utf8.ValidString(value.Title) || strings.ContainsRune(value.Title, 0) || len([]rune(value.Title)) > 160 {
			return nil, &Error{Code: CodeInvalidInput, Message: "参考图标题无效", Details: "title 必须是 1 到 160 个有效字符。"}
		}
		if !utf8.ValidString(value.Instruction) || strings.ContainsRune(value.Instruction, 0) || len([]rune(value.Instruction)) > 2000 {
			return nil, &Error{Code: CodeInvalidInput, Message: "参考图说明无效", Details: "instruction 最多允许 2000 个有效字符。"}
		}
		include := true
		if value.IncludeInYolo != nil {
			include = *value.IncludeInYolo
		}
		value.IncludeInYolo = &include
		result = append(result, value)
	}
	return result, nil
}

func referencePlanSource(input ReferenceFileInput) string {
	defaultTitle := strings.TrimSpace(strings.TrimSuffix(filepath.Base(input.OriginalFilename), filepath.Ext(input.OriginalFilename)))
	if defaultTitle == "" {
		defaultTitle = input.OriginalFilename
	}
	if input.ReferenceRole == ReferenceRoleAuto && input.Title == defaultTitle && input.Instruction == "" && input.IncludeInYolo != nil && *input.IncludeInYolo {
		return PlanSourceSystemDefault
	}
	return PlanSourceUserConfirmed
}

func initialLanguage(input string) string {
	for _, value := range input {
		if unicode.Is(unicode.Han, value) {
			return project.GenerationLanguageSimplifiedChinese
		}
	}
	return project.GenerationLanguageEnglish
}

func referenceManifestMatches(records []appstore.ProjectCreationReference, inputs []ReferenceFileInput) bool {
	if len(records) != len(inputs) {
		return false
	}
	for index, record := range records {
		input := inputs[index]
		if record.Position != index+1 || record.OriginalFilename != input.OriginalFilename || record.DeclaredMIMEType != input.MIMEType || record.DeclaredByteSize != input.ByteSize || record.ReferenceRole != input.ReferenceRole || record.Title != input.Title || record.Instruction != input.Instruction || record.IncludeInYolo != (input.IncludeInYolo != nil && *input.IncludeInYolo) || record.PlanSource != referencePlanSource(input) {
			return false
		}
	}
	return true
}

func (service *Service) publicSession(ctx context.Context, record appstore.ProjectCreationSession) (Session, error) {
	projectUUID := ""
	if record.RecentProjectID != nil {
		projectUUID = record.PlannedProjectUUID
	}
	records, err := service.app.ProjectCreationReferences(ctx, record.ID)
	if err != nil {
		return Session{}, err
	}
	references := make([]Reference, 0, len(records))
	for _, item := range records {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = strings.TrimSpace(strings.TrimSuffix(filepath.Base(item.OriginalFilename), filepath.Ext(item.OriginalFilename)))
		}
		reference := Reference{UUID: item.UUID, Position: item.Position, OriginalFilename: item.OriginalFilename, MIMEType: item.DeclaredMIMEType, ByteSize: item.DeclaredByteSize, ReferenceRole: item.ReferenceRole, Title: title, Instruction: item.Instruction, IncludeInYolo: item.IncludeInYolo, PlanSource: item.PlanSource, Status: item.Status, ErrorCode: item.ErrorCode}
		if item.Status == "ready" {
			reference.FileUUID = item.FileUUID
		}
		references = append(references, reference)
	}
	return Session{
		UUID: record.UUID, Status: record.Status, ProjectUUID: projectUUID,
		ThreadUUID: record.ThreadUUID, TurnUUID: record.TurnUUID,
		ErrorCode: record.ErrorCode, ErrorMessage: record.ErrorMessage,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, CompletedAt: record.CompletedAt, References: references,
	}, nil
}

func (service *Service) publish(session Session) {
	if service.events == nil {
		return
	}
	// Realtime is only a refresh hint. A publisher failure must not unwind a
	// Session, File binding, or Chat bootstrap that is already durable.
	defer func() { _ = recover() }()
	payload := map[string]any{"session_uuid": session.UUID, "status": session.Status}
	if session.ProjectUUID != "" {
		payload["project_uuid"] = session.ProjectUUID
	}
	if session.ThreadUUID != "" {
		payload["thread_uuid"] = session.ThreadUUID
	}
	service.events.Broadcast(realtime.SystemTopic, realtime.ProjectCreationSessionChanged, payload)
}

func (service *Service) Create(ctx context.Context, inputText, idempotencyKey string) (Session, error) {
	return service.CreateWithReferences(ctx, inputText, idempotencyKey, nil)
}

func (service *Service) CreateWithReferences(ctx context.Context, inputText, idempotencyKey string, referenceFiles []ReferenceFileInput) (Session, error) {
	inputText, idempotencyKey, err := validateCreateInput(inputText, idempotencyKey)
	if err != nil {
		return Session{}, err
	}
	referenceFiles, err = validateReferenceFiles(referenceFiles)
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
	references := make([]appstore.ProjectCreationReference, 0, len(referenceFiles))
	for index, input := range referenceFiles {
		referenceUUID, uuidErr := newUUIDv7()
		if uuidErr != nil {
			return Session{}, uuidErr
		}
		uploadUUID, uuidErr := newUUIDv7()
		if uuidErr != nil {
			return Session{}, uuidErr
		}
		fileUUID, uuidErr := newUUIDv7()
		if uuidErr != nil {
			return Session{}, uuidErr
		}
		references = append(references, appstore.ProjectCreationReference{
			UUID: referenceUUID, Position: index + 1, UploadUUID: uploadUUID, FileUUID: fileUUID,
			OriginalFilename: input.OriginalFilename, DeclaredMIMEType: input.MIMEType, DeclaredByteSize: input.ByteSize,
			ReferenceRole: input.ReferenceRole, Title: input.Title, Instruction: input.Instruction, IncludeInYolo: input.IncludeInYolo != nil && *input.IncludeInYolo, PlanSource: referencePlanSource(input),
			Status: "pending", CreatedAt: now, UpdatedAt: now,
		})
	}
	record, _, err := service.app.CreateOrGetProjectCreationSession(ctx, appstore.ProjectCreationSession{
		UUID: sessionUUID, IdempotencyKey: idempotencyKey, InputText: inputText,
		Status: StatusPending, PlannedProjectUUID: projectUUID, CreatedAt: now, UpdatedAt: now,
	}, references)
	if err != nil {
		return Session{}, err
	}
	storedReferences, err := service.app.ProjectCreationReferences(ctx, record.ID)
	if err != nil {
		return Session{}, err
	}
	if record.InputText != inputText || !referenceManifestMatches(storedReferences, referenceFiles) {
		return Session{}, &Error{Code: CodeIdempotencyConflict, Message: "幂等键已用于另一份创建输入", Details: "请为不同的首页文字或参考图清单生成新的 idempotency_key。"}
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
	return service.publicSession(ctx, record)
}

type projectReferenceBinding struct {
	ReferenceUUID string
	FileUUID      string
}

func bindProjectReference(tx *gorm.DB, projectID int64, sessionUUID string, reference appstore.ProjectCreationReference, fileID int64, now time.Time) error {
	var existing struct {
		ProjectID           int64
		CreationSessionUUID string
		Position            int
		FileID              int64
	}
	result := tx.Table("project_creation_reference_files").Select("project_id,creation_session_uuid,position,file_id").Where("reference_uuid = ?", reference.UUID).Limit(1).Find(&existing)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		if existing.ProjectID != projectID || existing.CreationSessionUUID != sessionUUID || existing.Position != reference.Position || existing.FileID != fileID {
			return &Error{Code: CodeIdempotencyConflict, Message: "参考图绑定冲突", Details: "同一个 reference_uuid 已绑定到另一份创建事实。"}
		}
		return nil
	}
	bindingUUID, err := newUUIDv7()
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO project_creation_reference_files(uuid,project_id,creation_session_uuid,reference_uuid,position,file_id,reference_role,title,instruction,include_in_yolo,plan_source,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		bindingUUID, projectID, sessionUUID, reference.UUID, reference.Position, fileID, reference.ReferenceRole, reference.Title, reference.Instruction, reference.IncludeInYolo, reference.PlanSource, now, now).Error
}

func (service *Service) projectReferenceBindings(ctx context.Context, projectUUID, sessionUUID string) (map[string]string, error) {
	result := map[string]string{}
	err := service.projects.WithStore(ctx, projectUUID, func(store *project.Store) error {
		var rows []projectReferenceBinding
		if err := store.DB().WithContext(ctx).Table("project_creation_reference_files AS bindings").
			Select("bindings.reference_uuid,files.uuid AS file_uuid").
			Joins("JOIN files ON files.id=bindings.file_id").
			Where("bindings.creation_session_uuid = ?", sessionUUID).Order("bindings.position,bindings.id").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			result[row.ReferenceUUID] = row.FileUUID
		}
		return nil
	})
	return result, err
}

func (service *Service) finalizeReference(ctx context.Context, record appstore.ProjectCreationSession, reference appstore.ProjectCreationReference, reader io.Reader) error {
	return service.projects.WithStore(ctx, record.PlannedProjectUUID, func(store *project.Store) error {
		var projectID int64
		if err := store.DB().WithContext(ctx).Table("projects").Where("uuid = ?", record.PlannedProjectUUID).Pluck("id", &projectID).Error; err != nil {
			return err
		}
		fileService := files.NewService(store, service.events)
		if reader != nil {
			if _, err := fileService.CreateUploadWithIdentity(ctx, files.CreateUploadInput{
				Purpose: "project_chatbot_reference", OriginalFilename: reference.OriginalFilename, DisplayName: reference.OriginalFilename,
				Metadata: map[string]any{"source": "project_creation:" + reference.UUID}, Reader: reader,
			}, files.UploadIdentity{UploadUUID: reference.UploadUUID, FileUUID: reference.FileUUID}); err != nil {
				return err
			}
		}
		upload, err := fileService.GetUpload(ctx, reference.UploadUUID)
		if err != nil {
			return err
		}
		if upload.State != files.StateReady && upload.State != files.StateConsuming && upload.State != files.StateConsumed {
			return &Error{Code: CodeReferenceNotReady, Message: "参考图尚未上传完成", Details: "请重新选择该图片并重试上传。"}
		}
		asset, err := fileService.FinalizeUploadWithBind(ctx, reference.UploadUUID, "project_chatbot_reference", func(tx *gorm.DB, fileID int64) error {
			return bindProjectReference(tx, projectID, record.UUID, reference, fileID, service.now().UTC())
		})
		if err != nil {
			return err
		}
		if asset.UUID != reference.FileUUID {
			return &Error{Code: CodeIdempotencyConflict, Message: "参考图 File 身份冲突", Details: "Finalize 返回的 File UUID 与创建清单不一致。"}
		}
		return nil
	})
}

func (service *Service) reconcileReferences(ctx context.Context, record appstore.ProjectCreationSession, references []appstore.ProjectCreationReference) ([]appstore.ProjectCreationReference, error) {
	bindings, err := service.projectReferenceBindings(ctx, record.PlannedProjectUUID, record.UUID)
	if err != nil {
		return nil, err
	}
	for index := range references {
		reference := references[index]
		if fileUUID := bindings[reference.UUID]; fileUUID != "" {
			if fileUUID != reference.FileUUID {
				return nil, &Error{Code: CodeIdempotencyConflict, Message: "参考图恢复冲突", Details: "项目绑定的 File UUID 与创建清单不一致。"}
			}
			if reference.Status != "ready" || reference.ErrorCode != "" {
				reference, err = service.app.UpdateProjectCreationReference(ctx, reference.ID, map[string]any{"status": "ready", "error_code": "", "updated_at": service.now().UTC()})
				if err != nil {
					return nil, err
				}
				references[index] = reference
			}
			continue
		}
		if reference.Status == "ready" {
			return nil, &Error{Code: CodeIdempotencyConflict, Message: "参考图恢复记录缺失", Details: "应用库已标记 ready，但项目库不存在对应 File 绑定。"}
		}
		finalizeErr := service.finalizeReference(ctx, record, reference, nil)
		if finalizeErr == nil {
			reference, err = service.app.UpdateProjectCreationReference(ctx, reference.ID, map[string]any{"status": "ready", "error_code": "", "updated_at": service.now().UTC()})
			if err != nil {
				return nil, err
			}
			references[index] = reference
			continue
		}
		var fileErr *files.Error
		var creationErr *Error
		if (errors.As(finalizeErr, &fileErr) && (fileErr.Code == files.CodeUploadNotFound || fileErr.Code == files.CodeUploadNotReady || fileErr.Code == files.CodeUploadExpired)) || (errors.As(finalizeErr, &creationErr) && creationErr.Code == CodeReferenceNotReady) {
			continue
		}
		return nil, finalizeErr
	}
	return references, nil
}

func allReferencesReady(references []appstore.ProjectCreationReference) bool {
	for _, reference := range references {
		if reference.Status != "ready" {
			return false
		}
	}
	return true
}

func agentReferenceInputs(references []appstore.ProjectCreationReference) []agent.ReferenceInput {
	result := make([]agent.ReferenceInput, 0, len(references))
	for _, reference := range references {
		result = append(result, agent.ReferenceInput{ResourceType: agent.ReferenceTypeFile, ResourceUUID: reference.FileUUID})
	}
	return result
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
		return service.publicSession(ctx, record)
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
			"recent_project_id": recentID, "updated_at": service.now().UTC(),
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
	}
	references, err := service.app.ProjectCreationReferences(ctx, record.ID)
	if err != nil {
		return Session{}, err
	}
	if len(references) > 0 {
		record, err = service.app.UpdateProjectCreationSession(ctx, record.UUID, map[string]any{"status": StatusAwaitingReferences, "updated_at": service.now().UTC()})
		if err != nil {
			return Session{}, err
		}
		references, err = service.reconcileReferences(ctx, record, references)
		if err != nil {
			return service.fail(ctx, record, err)
		}
		if !allReferencesReady(references) {
			session, sessionErr := service.publicSession(ctx, record)
			if sessionErr == nil {
				service.publish(session)
			}
			return session, sessionErr
		}
	}
	record, err = service.app.UpdateProjectCreationSession(ctx, record.UUID, map[string]any{"status": StatusCreatingConversation, "updated_at": service.now().UTC()})
	if err != nil {
		return Session{}, err
	}
	bootstrap, err := service.agents.BootstrapConversation(ctx, record.PlannedProjectUUID, record.UUID, record.InputText, agentReferenceInputs(references))
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
	session, err := service.publicSession(ctx, record)
	if err == nil {
		service.publish(session)
	}
	return session, err
}

func (service *Service) UploadReference(ctx context.Context, sessionUUID, referenceUUID string, reader io.Reader) (Session, error) {
	service.mu.Lock()
	record, reference, err := service.app.ProjectCreationReference(ctx, sessionUUID, referenceUUID)
	if errors.Is(err, appstore.ErrProjectCreationSessionNotFound) {
		service.mu.Unlock()
		return Session{}, &Error{Code: CodeNotFound, Message: "项目创建会话不存在", Details: "该 UUID 不属于本机创建会话。", Cause: err}
	}
	if errors.Is(err, appstore.ErrProjectCreationReferenceNotFound) {
		service.mu.Unlock()
		return Session{}, &Error{Code: CodeReferenceNotFound, Message: "项目创建参考图不存在", Details: "该 reference_uuid 不属于当前创建会话。", Cause: err}
	}
	if err != nil {
		service.mu.Unlock()
		return Session{}, err
	}
	if record.Status == StatusCancelled {
		service.mu.Unlock()
		return Session{}, &Error{Code: CodeReferenceNotReady, Message: "项目创建会话已取消", Details: "已取消的创建会话不能继续上传参考图。"}
	}
	if record.Status == StatusActive {
		session, publicErr := service.publicSession(ctx, record)
		service.mu.Unlock()
		return session, publicErr
	}
	if record.RecentProjectID == nil {
		service.mu.Unlock()
		return Session{}, &Error{Code: CodeReferenceNotReady, Message: "草稿项目尚未创建", Details: "请先重试项目创建，再上传参考图。"}
	}
	now := service.now().UTC()
	reference, err = service.app.UpdateProjectCreationReference(ctx, reference.ID, map[string]any{"status": "uploading", "error_code": "", "updated_at": now})
	if err == nil {
		record, err = service.app.UpdateProjectCreationSession(ctx, record.UUID, map[string]any{"status": StatusAwaitingReferences, "updated_at": now})
	}
	if err == nil {
		err = service.finalizeReference(ctx, record, reference, reader)
	}
	if err != nil {
		code := "project_creation_reference_upload_failed"
		var fileErr *files.Error
		var creationErr *Error
		if errors.As(err, &fileErr) {
			code = fileErr.Code
		} else if errors.As(err, &creationErr) {
			code = creationErr.Code
		}
		_, _ = service.app.UpdateProjectCreationReference(context.WithoutCancel(ctx), reference.ID, map[string]any{"status": "failed", "error_code": code, "updated_at": service.now().UTC()})
		session, _ := service.publicSession(context.WithoutCancel(ctx), record)
		service.publish(session)
		service.mu.Unlock()
		return Session{}, err
	}
	_, err = service.app.UpdateProjectCreationReference(ctx, reference.ID, map[string]any{"status": "ready", "error_code": "", "updated_at": service.now().UTC()})
	service.mu.Unlock()
	if err != nil {
		return Session{}, err
	}
	return service.Resume(ctx, sessionUUID)
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
	session, publicErr := service.publicSession(context.WithoutCancel(ctx), failed)
	if publicErr != nil {
		return Session{}, publicErr
	}
	service.publish(session)
	return session, nil
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
	var fileErr *files.Error
	if errors.As(err, &fileErr) {
		return fileErr.Code, fileErr.Message
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
