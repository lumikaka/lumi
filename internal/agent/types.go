package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

const (
	ThreadIdle               = "idle"
	ThreadBusy               = "busy"
	ThreadWaitingForInput    = "waiting_for_input"
	ThreadCompleted          = "completed"
	ThreadFailed             = "failed"
	ThreadCancelled          = "cancelled"
	ThreadInterrupted        = "interrupted"
	ThreadScopeProject       = "project"
	ThreadScopePremise       = "premise"
	ScenePremiseAsset        = "premise_asset_generation"
	SceneAssetReference      = "asset_reference"
	SceneStoryboardReference = "storyboard_reference"

	TurnQueued          = "queued"
	TurnInProgress      = "in_progress"
	TurnWaitingForInput = "waiting_for_input"
	TurnCompleted       = "completed"
	TurnFailed          = "failed"
	TurnCancelled       = "cancelled"
	TurnInterrupted     = "interrupted"

	WorkflowYolo                     = "yolo_project_initialization"
	WorkflowComicSectionImage        = "comic_section_image_generation"
	WorkflowComicStoryboard          = "comic_storyboard_generation"
	WorkflowStoryChapter             = "story_chapter_generation"
	WorkflowStoryChapterBatchPlan    = "story_chapter_batch_plan"
	WorkflowStepComicStoryboard      = "comic_storyboard"
	WorkflowStepStoryChapter         = "story_chapter"
	WorkflowStepChapterBatchPlan     = "chapter_batch_plan"
	WorkflowStepSelectReferences     = "select_reference_assets"
	WorkflowStepSaveSectionPremise   = "save_section_premise"
	WorkflowStepGenerateSectionImage = "generate_section_image"
	WorkflowStepSaveSectionImage     = "save_section_image"

	WorkflowQueued      = "queued"
	WorkflowRunning     = "running"
	WorkflowCompleted   = "completed"
	WorkflowFailed      = "failed"
	WorkflowCancelled   = "cancelled"
	WorkflowInterrupted = "interrupted"

	JobChatTurn      = "chat_turn"
	JobChatResume    = "chat_resume"
	JobWorkflowStep  = "workflow_step"
	DefaultMaxSteps  = 12
	MaxToolResult    = 64 << 10
	MaxContextBytes  = 512 << 10
	MaxSummaryBytes  = 24 << 10
	DefaultItemsPage = 100
)

var YoloStepKeys = []string{
	"project_initialization",
	"story",
	"story_profile",
	"premise",
	"comic_sections",
	"first_section_image",
}

var ComicSectionImageStepKeys = []string{
	WorkflowStepSelectReferences,
	WorkflowStepSaveSectionPremise,
	WorkflowStepGenerateSectionImage,
	WorkflowStepSaveSectionImage,
}

type Queue interface {
	EnqueueAgentTx(context.Context, string, *sql.Tx, JobSpec) (int64, error)
	CancelAgentJob(context.Context, string, int64) error
	CancelAgentWork(string, string)
	StartDomainTask(context.Context, string, DomainTaskRequest) (DomainTask, error)
	GetDomainTask(context.Context, string, string, string) (DomainTask, error)
	CancelDomainTask(context.Context, string, string, string) error
	RetryDomainTask(context.Context, string, string, string) (DomainTask, error)
}

type JobSpec struct {
	Version      int
	ProjectUUID  string
	JobKind      string
	ResourceUUID string
	ThreadUUID   string
}

type DomainTaskRequest struct {
	Kind                  string
	ResourceUUID          string
	ChapterUUID           string
	ProviderUUID          string
	Model                 string
	SelectionProviderUUID string
	SelectionModel        string
	Prompt                string
	IdempotencyKey        string
	PremiseAssetUUIDs     []string
	AssetOperation        string
	AssetType             string
	AssetTitle            string
	AssetSummary          string
	AssetTags             []string
}

type DomainTask struct {
	UUID         string `json:"uuid"`
	Kind         string `json:"kind"`
	ResourceUUID string `json:"resource_uuid"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type Thread struct {
	UUID         string     `json:"uuid"`
	ProjectUUID  string     `json:"project_uuid"`
	Title        string     `json:"title"`
	Status       string     `json:"status"`
	Scope        string     `json:"scope"`
	Scene        string     `json:"scene,omitempty"`
	SubjectUUID  string     `json:"subject_uuid,omitempty"`
	ProviderUUID string     `json:"provider_uuid"`
	Model        string     `json:"model"`
	ModelSource  string     `json:"model_source"`
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Turn struct {
	UUID               string     `json:"uuid"`
	ThreadUUID         string     `json:"thread_uuid"`
	SourceType         string     `json:"source_type"`
	SourceFollowUpUUID string     `json:"source_follow_up_uuid,omitempty"`
	QueueSequence      int64      `json:"queue_sequence"`
	InputText          string     `json:"input_text"`
	Status             string     `json:"status"`
	ErrorCode          string     `json:"error_code,omitempty"`
	ErrorMessage       string     `json:"error_message,omitempty"`
	CancelRequestedAt  *time.Time `json:"cancel_requested_at,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type Run struct {
	UUID              string     `json:"uuid"`
	ThreadUUID        string     `json:"thread_uuid"`
	TurnUUID          string     `json:"turn_uuid"`
	TriggerType       string     `json:"trigger_type"`
	Status            string     `json:"status"`
	StepCount         int        `json:"step_count"`
	MaxSteps          int        `json:"max_steps"`
	ProviderUUID      string     `json:"provider_uuid"`
	Model             string     `json:"model"`
	ModelSource       string     `json:"model_source"`
	ContextBytes      int        `json:"context_bytes"`
	ErrorCode         string     `json:"error_code,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	CancelRequestedAt *time.Time `json:"cancel_requested_at,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Item struct {
	UUID            string           `json:"uuid"`
	ThreadUUID      string           `json:"thread_uuid"`
	TurnUUID        string           `json:"turn_uuid,omitempty"`
	RunUUID         string           `json:"run_uuid,omitempty"`
	Sequence        int64            `json:"sequence"`
	ItemType        string           `json:"item_type"`
	Role            string           `json:"role"`
	Content         string           `json:"content"`
	ContentFormat   string           `json:"content_format"`
	Status          string           `json:"status"`
	ToolCallUUID    string           `json:"tool_call_uuid,omitempty"`
	ToolName        string           `json:"tool_name,omitempty"`
	TargetUUID      string           `json:"target_uuid,omitempty"`
	Metadata        json.RawMessage  `json:"metadata"`
	ImageReferences []ImageReference `json:"image_references,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
}

type ImageReference struct {
	UploadUUID       string `json:"upload_uuid"`
	FileUUID         string `json:"file_uuid"`
	OriginalFilename string `json:"original_filename"`
	MIMEType         string `json:"mime_type"`
	ByteSize         int64  `json:"byte_size"`
	Width            *int   `json:"width"`
	Height           *int   `json:"height"`
	ContentURL       string `json:"content_url"`
}

type Event struct {
	UUID       string          `json:"uuid"`
	ThreadUUID string          `json:"thread_uuid"`
	RunUUID    string          `json:"run_uuid,omitempty"`
	Sequence   int64           `json:"sequence"`
	EventType  string          `json:"event_type"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
}

type FollowUp struct {
	UUID             string           `json:"uuid"`
	ThreadUUID       string           `json:"thread_uuid"`
	InputText        string           `json:"input_text"`
	Position         int              `json:"position"`
	Status           string           `json:"status"`
	PromotedTurnUUID string           `json:"promoted_turn_uuid,omitempty"`
	ImageReferences  []ImageReference `json:"image_references,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	DeletedAt        *time.Time       `json:"deleted_at,omitempty"`
}

type FollowUpDelivery struct {
	DeliveryMode string    `json:"delivery_mode"`
	Item         *Item     `json:"item,omitempty"`
	FollowUp     *FollowUp `json:"follow_up,omitempty"`
}

type UserInputOption struct {
	UUID        string `json:"uuid"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type UserInputRequest struct {
	UUID         string            `json:"uuid"`
	ThreadUUID   string            `json:"thread_uuid"`
	RunUUID      string            `json:"run_uuid"`
	TurnUUID     string            `json:"turn_uuid"`
	ItemUUID     string            `json:"item_uuid"`
	ToolCallUUID string            `json:"tool_call_uuid"`
	InputType    string            `json:"input_type"`
	Question     string            `json:"question"`
	Options      []UserInputOption `json:"options"`
	Response     json.RawMessage   `json:"response,omitempty"`
	Status       string            `json:"status"`
	AnsweredAt   *time.Time        `json:"answered_at,omitempty"`
	ResumedAt    *time.Time        `json:"resumed_at,omitempty"`
	CancelledAt  *time.Time        `json:"cancelled_at,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type CursorPage[T any] struct {
	Items            []T              `json:"items"`
	CursorPagination CursorPagination `json:"cursor_pagination"`
}

type CursorPagination struct {
	PerPage    int    `json:"per_page"`
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

type PagePagination struct {
	PerPage     int   `json:"per_page"`
	CurrentPage int   `json:"current_page"`
	LastPage    int   `json:"last_page"`
	Total       int64 `json:"total"`
}

type ThreadPage struct {
	Items      []Thread       `json:"items"`
	Pagination PagePagination `json:"pagination"`
}

type Workflow struct {
	UUID              string          `json:"uuid"`
	ProjectUUID       string          `json:"project_uuid"`
	ThreadUUID        string          `json:"thread_uuid,omitempty"`
	Kind              string          `json:"kind"`
	Title             string          `json:"title"`
	Status            string          `json:"status"`
	InputVersion      int             `json:"input_version"`
	InputSnapshot     json.RawMessage `json:"input_snapshot"`
	IdempotencyKey    string          `json:"idempotency_key"`
	ProviderUUID      string          `json:"provider_uuid"`
	Model             string          `json:"model"`
	ModelSource       string          `json:"model_source"`
	CurrentStepKey    string          `json:"current_step_key,omitempty"`
	ErrorCode         string          `json:"error_code,omitempty"`
	ErrorMessage      string          `json:"error_message,omitempty"`
	CancelRequestedAt *time.Time      `json:"cancel_requested_at,omitempty"`
	StartedAt         *time.Time      `json:"started_at,omitempty"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	Steps             []WorkflowStep  `json:"steps,omitempty"`
}

type WorkflowStep struct {
	UUID         string          `json:"uuid"`
	StepKey      string          `json:"step_key"`
	Position     int             `json:"position"`
	Status       string          `json:"status"`
	Progress     int             `json:"progress"`
	TaskUUID     string          `json:"task_uuid,omitempty"`
	ResourceUUID string          `json:"resource_uuid,omitempty"`
	Input        json.RawMessage `json:"input"`
	Output       json.RawMessage `json:"output"`
	ErrorCode    string          `json:"error_code,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type WorkflowDiagnosticRun struct {
	UUID         string     `json:"uuid"`
	WorkflowUUID string     `json:"workflow_uuid"`
	StepUUID     string     `json:"step_uuid"`
	StepKey      string     `json:"step_key"`
	Attempt      int        `json:"attempt"`
	Status       string     `json:"status"`
	Progress     int        `json:"progress"`
	TaskUUID     string     `json:"task_uuid,omitempty"`
	ResourceUUID string     `json:"resource_uuid,omitempty"`
	ErrorCode    string     `json:"error_code,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type WorkflowDiagnosticEvent struct {
	UUID         string          `json:"uuid"`
	WorkflowUUID string          `json:"workflow_uuid"`
	StepUUID     string          `json:"step_uuid,omitempty"`
	Sequence     int64           `json:"sequence"`
	EventType    string          `json:"event_type"`
	Payload      json.RawMessage `json:"payload"`
	CreatedAt    time.Time       `json:"created_at"`
}

type WorkflowLLMLog struct {
	UUID             string     `json:"uuid"`
	WorkflowUUID     string     `json:"workflow_uuid"`
	WorkflowStepUUID string     `json:"workflow_step_uuid"`
	Scenario         string     `json:"scenario"`
	RequestType      string     `json:"request_type"`
	Attempt          int        `json:"attempt"`
	Model            string     `json:"model"`
	Status           string     `json:"status"`
	InputTokens      int        `json:"input_tokens"`
	OutputTokens     int        `json:"output_tokens"`
	DurationMS       int64      `json:"duration_ms"`
	ErrorCode        string     `json:"error_code,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

type WorkflowLLMLogPage struct {
	Items      []WorkflowLLMLog `json:"items"`
	Pagination PagePagination   `json:"pagination"`
}

type CreateThreadInput struct {
	Title        string `json:"title"`
	Scope        string `json:"scope,omitempty"`
	Scene        string `json:"scene,omitempty"`
	SubjectUUID  string `json:"subject_uuid,omitempty"`
	ProviderUUID string `json:"-"`
	Model        string `json:"model"`
}

type CreateTurnInput struct {
	InputText   string   `json:"input_text"`
	MaxSteps    int      `json:"max_steps,omitempty"`
	UploadUUIDs []string `json:"upload_uuids,omitempty"`
}

type CreateFollowUpInput struct {
	InputText   string   `json:"input_text"`
	UploadUUIDs []string `json:"upload_uuids,omitempty"`
}

type SteeringInput struct {
	InputText   string   `json:"input_text"`
	UploadUUIDs []string `json:"upload_uuids,omitempty"`
}

type UserInputResponse struct {
	SelectedOptionUUIDs []string `json:"selected_option_uuids"`
	OtherText           string   `json:"other_text,omitempty"`
}

type CreateYoloInput struct {
	Title          string `json:"title"`
	StoryPrompt    string `json:"story_prompt"`
	ProviderUUID   string `json:"-"`
	Model          string `json:"model"`
	IdempotencyKey string `json:"idempotency_key"`
}

type threadRecord struct {
	ID                                                    int64 `gorm:"primaryKey"`
	UUID, Title, Status, Scope, Scene, SubjectUUID        string
	ProviderUUID, Model, ModelSource                      string
	ProjectID                                             int64
	NextTurnSequence, NextItemSequence, NextEventSequence int64
	ArchivedAt                                            *time.Time
	CreatedAt, UpdatedAt                                  time.Time
}

func (threadRecord) TableName() string { return "chat_threads" }

type turnRecord struct {
	ID, ThreadID, QueueSequence, SourceFollowUpID int64
	UUID, SourceType, InputText, Status           string
	RiverJobID                                    *int64
	ErrorCode, ErrorMessage                       string
	CancelRequestedAt, StartedAt, CompletedAt     *time.Time
	CreatedAt, UpdatedAt                          time.Time
}

func (turnRecord) TableName() string { return "chat_turns" }

type runRecord struct {
	ID, ThreadID, TurnID                                      int64
	UUID, TriggerType, Status                                 string
	StepCount, MaxSteps, ContextBytes                         int
	ProviderUUID, Model, ModelSource, ErrorCode, ErrorMessage string
	CancelRequestedAt, StartedAt, CompletedAt                 *time.Time
	CreatedAt, UpdatedAt                                      time.Time
}

func (runRecord) TableName() string { return "chat_runs" }

type itemRecord struct {
	ID, ThreadID, Sequence                               int64
	TurnID, RunID                                        *int64
	UUID, ItemType, Role, Content, ContentFormat, Status string
	RemoteItemUUID, ToolName, TargetUUID, MetadataJSON   string
	CreatedAt                                            time.Time
}

func (itemRecord) TableName() string { return "chat_items" }

func (toolExecutionRecord) TableName() string { return "agent_tool_executions" }

type followUpRecord struct {
	ID, ThreadID            int64
	UUID, InputText, Status string
	Position                int
	PromotedTurnID          *int64
	CreatedAt, UpdatedAt    time.Time
	DeletedAt               *time.Time
}

func (followUpRecord) TableName() string { return "chat_follow_ups" }

type workflowRecord struct {
	ID, ProjectID                                                             int64
	ThreadID                                                                  *int64
	UUID, Kind, Title, Status, InputSnapshot, IdempotencyKey                  string
	ProviderUUID, Model, ModelSource, CurrentStepKey, ErrorCode, ErrorMessage string
	InputVersion                                                              int
	CancelRequestedAt, StartedAt, CompletedAt                                 *time.Time
	CreatedAt, UpdatedAt                                                      time.Time
}

func (workflowRecord) TableName() string { return "workflows" }

type workflowStepRecord struct {
	ID, WorkflowID                                                int64
	UUID, StepKey, Status, IdempotencyKey, TaskUUID, ResourceUUID string
	RiverJobID                                                    *int64
	Position                                                      int
	InputJSON, OutputJSON, ErrorCode, ErrorMessage                string
	StartedAt, CompletedAt                                        *time.Time
	CreatedAt, UpdatedAt                                          time.Time
}

func (workflowStepRecord) TableName() string { return "workflow_steps" }
