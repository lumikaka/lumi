package jobqueue

import (
	"encoding/json"
	"time"

	"lumi/internal/agent"
	"lumi/internal/project"
)

const (
	RiverVersion                  = "v0.41.0"
	QueueStory                    = "story_generation"
	QueueAssetMaintenance         = "asset_maintenance"
	QueueProduction               = "production"
	QueueAgent                    = "agent"
	KindStoryChapterGeneration    = "story_chapter_generation"
	KindStoryProfileGeneration    = "story_profile_generation"
	KindStoryProfileFromChapters  = "story_profile_from_chapters"
	KindStoryChapterBatchPlan     = "story_chapter_batch_plan"
	KindComicStoryboardGeneration = "comic_storyboard_generation"
	KindAssetReconcile            = "asset_reconcile"
	KindAssetIntegrityScan        = "asset_integrity_scan"
	KindAssetThumbnailRebuild     = "asset_thumbnail_rebuild"
	KindAssetUploadCleanup        = "asset_upload_cleanup"
	KindAssetGCApply              = "asset_gc_apply"
	KindPremiseSettingGeneration  = "premise_setting_generation"
	KindPremiseAssetBreakdown     = "premise_asset_breakdown"
	KindPremiseAssetGeneration    = "premise_asset_generation"
	KindComicImageGeneration      = "comic_image_generation"
	KindComicExport               = "comic_export"
)

const (
	StatusQueued          = "queued"
	StatusRunning         = "running"
	StatusWaitingForInput = "waiting_for_input"
	StatusCompleted       = "completed"
	StatusFailed          = "failed"
	StatusCancelled       = "cancelled"
	StatusInterrupted     = "interrupted"
)

type Task struct {
	UUID              string          `json:"uuid"`
	Kind              string          `json:"kind"`
	ResourceUUID      string          `json:"resource_uuid"`
	InputVersion      int             `json:"input_version"`
	InputSnapshot     json.RawMessage `json:"input_snapshot"`
	Status            string          `json:"status"`
	IdempotencyKey    string          `json:"idempotency_key"`
	Retryable         bool            `json:"retryable"`
	ProviderUUID      string          `json:"provider_uuid"`
	Model             string          `json:"model"`
	ModelSource       string          `json:"model_source"`
	Progress          int             `json:"progress"`
	Attempt           int             `json:"attempt"`
	MaxAttempts       int             `json:"max_attempts"`
	ErrorCode         string          `json:"error_code"`
	ErrorMessage      string          `json:"error_message"`
	CancelRequestedAt *time.Time      `json:"cancel_requested_at"`
	StartedAt         *time.Time      `json:"started_at"`
	CompletedAt       *time.Time      `json:"completed_at"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type TaskEvent struct {
	UUID      string          `json:"uuid"`
	Sequence  int64           `json:"sequence"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type MaintenanceTask struct {
	UUID              string          `json:"uuid"`
	Kind              string          `json:"kind"`
	ResourceUUID      string          `json:"resource_uuid"`
	InputVersion      int             `json:"input_version"`
	InputSnapshot     json.RawMessage `json:"input_snapshot"`
	Status            string          `json:"status"`
	Progress          int             `json:"progress"`
	Attempt           int             `json:"attempt"`
	MaxAttempts       int             `json:"max_attempts"`
	ErrorCode         string          `json:"error_code"`
	ErrorMessage      string          `json:"error_message"`
	CancelRequestedAt *time.Time      `json:"cancel_requested_at"`
	StartedAt         *time.Time      `json:"started_at"`
	CompletedAt       *time.Time      `json:"completed_at"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type CreateMaintenanceInput struct {
	Kind       string `json:"kind"`
	PlanUUID   string `json:"plan_uuid,omitempty"`
	GraceHours int    `json:"grace_hours,omitempty"`
}

type CursorPagination struct {
	PerPage    int     `json:"per_page"`
	NextCursor *string `json:"next_cursor"`
	PrevCursor *string `json:"prev_cursor"`
	HasMore    bool    `json:"has_more"`
}

type GenerationParameters struct {
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
}

type CreateGenerationInput struct {
	ChapterUUID    string                        `json:"chapter_uuid"`
	ProviderUUID   string                        `json:"-"`
	Model          string                        `json:"model"`
	PromptKey      string                        `json:"prompt_key,omitempty"`
	Prompt         string                        `json:"prompt"`
	Parameters     GenerationParameters          `json:"parameters"`
	IdempotencyKey string                        `json:"idempotency_key"`
	Invocation     agent.DomainInvocationContext `json:"-"`
}

// CreateStoryWorkflowInput is used by the non-chapter Story prompt steps. The
// HTTP handlers choose the task kind; clients cannot submit an arbitrary kind.
type CreateStoryWorkflowInput struct {
	ProviderUUID    string                        `json:"-"`
	Model           string                        `json:"model"`
	Prompt          string                        `json:"prompt,omitempty"`
	ChapterCount    int                           `json:"chapter_count,omitempty"`
	MaxSectionCount *int                          `json:"max_section_count,omitempty"`
	Parameters      GenerationParameters          `json:"parameters"`
	IdempotencyKey  string                        `json:"idempotency_key"`
	Invocation      agent.DomainInvocationContext `json:"-"`
}

const (
	ComicStoryboardConflictOverwrite    = "overwrite"
	ComicStoryboardConflictKeepExisting = "keep_existing"
)

type ResolveComicStoryboardConflictInput struct {
	Action                     string `json:"action"`
	ExpectedComicStateRevision *int64 `json:"expected_comic_state_revision"`
}

type WorkflowConflictResolution struct {
	WorkflowUUID string `json:"workflow_uuid"`
	ThreadUUID   string `json:"thread_uuid"`
	TaskUUID     string `json:"task_uuid"`
	Action       string `json:"action"`
	Status       string `json:"status"`
}

type storyGenerationSnapshot struct {
	Version            int                         `json:"version"`
	ProjectUUID        string                      `json:"project_uuid"`
	GenerationLanguage string                      `json:"generation_language"`
	ChapterUUID        string                      `json:"chapter_uuid"`
	ChapterCode        string                      `json:"chapter_code"`
	ChapterStoryUUID   string                      `json:"chapter_story_uuid,omitempty"`
	ChapterRevision    int64                       `json:"chapter_revision"`
	InputContent       string                      `json:"input_content,omitempty"`
	PromptKey          string                      `json:"prompt_key"`
	PromptTemplate     string                      `json:"prompt_template"`
	SystemPrompt       string                      `json:"system_prompt"`
	Prompt             string                      `json:"prompt"`
	ProviderUUID       string                      `json:"provider_uuid"`
	ProviderType       string                      `json:"provider_type"`
	ProviderBaseURL    string                      `json:"provider_base_url"`
	Model              string                      `json:"model"`
	ModelSource        string                      `json:"model_source,omitempty"`
	Parameters         GenerationParameters        `json:"parameters"`
	WorkflowKind       string                      `json:"workflow_kind,omitempty"`
	ResourceRevision   int64                       `json:"resource_revision,omitempty"`
	ChapterCount       int                         `json:"chapter_count,omitempty"`
	TargetChapterCodes []string                    `json:"target_chapter_codes,omitempty"`
	MaxSectionCount    int                         `json:"max_section_count,omitempty"`
	MomentCountPlan    []int                       `json:"moment_count_plan,omitempty"`
	PictureBook        *project.PictureBookProfile `json:"picture_book,omitempty"`
}

type taskRecord struct {
	ID                int64 `gorm:"primaryKey;autoIncrement"`
	UUID              string
	ProjectID         int64
	RiverJobID        *int64
	Kind              string
	ResourceUUID      string
	InputVersion      int
	InputSnapshot     string
	Status            string
	IdempotencyKey    string
	Retryable         bool
	ProviderUUID      string
	Model             string
	ModelSource       string
	Progress          int
	Attempt           int
	MaxAttempts       int
	ErrorCode         string
	ErrorMessage      string
	CancelRequestedAt *time.Time
	StartedAt         *time.Time
	CompletedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (taskRecord) TableName() string { return "task_runs" }

func (record taskRecord) DTO() Task {
	return Task{UUID: record.UUID, Kind: record.Kind, ResourceUUID: record.ResourceUUID, InputVersion: record.InputVersion,
		InputSnapshot: json.RawMessage(record.InputSnapshot), Status: record.Status, IdempotencyKey: record.IdempotencyKey,
		Retryable: record.Retryable, ProviderUUID: record.ProviderUUID, Model: record.Model, ModelSource: record.ModelSource, Progress: record.Progress,
		Attempt: record.Attempt, MaxAttempts: record.MaxAttempts, ErrorCode: record.ErrorCode, ErrorMessage: record.ErrorMessage,
		CancelRequestedAt: record.CancelRequestedAt, StartedAt: record.StartedAt, CompletedAt: record.CompletedAt,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

type taskEventRecord struct {
	ID        int64 `gorm:"primaryKey;autoIncrement"`
	UUID      string
	TaskRunID int64
	Sequence  int64
	EventType string
	Payload   string
	CreatedAt time.Time
}

func (taskEventRecord) TableName() string { return "task_events" }

type riverArgs struct {
	Version      int    `json:"version"`
	ProjectUUID  string `json:"project_uuid" river:"unique"`
	TaskUUID     string `json:"task_uuid"`
	TaskKind     string `json:"task_kind" river:"unique"`
	ResourceUUID string `json:"resource_uuid" river:"unique"`
}

func (riverArgs) Kind() string { return "lumi_story_generation_v1" }

type maintenanceArgs struct {
	Version         int    `json:"version"`
	ProjectUUID     string `json:"project_uuid" river:"unique"`
	TaskUUID        string `json:"task_uuid"`
	MaintenanceKind string `json:"maintenance_kind" river:"unique"`
	ResourceUUID    string `json:"resource_uuid,omitempty" river:"unique"`
}

func (maintenanceArgs) Kind() string { return "lumi_asset_maintenance_v1" }

type productionArgs struct {
	Version      int    `json:"version"`
	ProjectUUID  string `json:"project_uuid" river:"unique"`
	TaskUUID     string `json:"task_uuid" river:"unique"`
	TaskKind     string `json:"task_kind" river:"unique"`
	ResourceUUID string `json:"resource_uuid" river:"unique"`
}

func (productionArgs) Kind() string { return "lumi_production_v1" }

type exportCleanupArgs struct {
	Version     int    `json:"version"`
	ProjectUUID string `json:"project_uuid" river:"unique"`
}

func (exportCleanupArgs) Kind() string { return "lumi_comic_export_cleanup_v1" }

type ProductionTask struct {
	UUID              string          `json:"uuid"`
	Kind              string          `json:"kind"`
	ResourceUUID      string          `json:"resource_uuid"`
	InputSnapshot     json.RawMessage `json:"input_snapshot"`
	Status            string          `json:"status"`
	IdempotencyKey    string          `json:"idempotency_key"`
	ProviderUUID      string          `json:"provider_uuid,omitempty"`
	Model             string          `json:"model,omitempty"`
	ModelSource       string          `json:"model_source,omitempty"`
	Progress          int             `json:"progress"`
	Attempt           int             `json:"attempt"`
	MaxAttempts       int             `json:"max_attempts"`
	ErrorCode         string          `json:"error_code,omitempty"`
	ErrorMessage      string          `json:"error_message,omitempty"`
	CancelRequestedAt *time.Time      `json:"cancel_requested_at,omitempty"`
	StartedAt         *time.Time      `json:"started_at,omitempty"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type CreateProductionGenerationInput struct {
	ProviderUUID          string               `json:"-"`
	Model                 string               `json:"model"`
	SelectionProviderUUID string               `json:"-"`
	SelectionModel        string               `json:"-"`
	Prompt                string               `json:"prompt"`
	Parameters            GenerationParameters `json:"parameters"`
	PremiseAssetUUIDs     []string             `json:"premise_asset_uuids,omitempty"`
	AssetOperation        string               `json:"asset_operation,omitempty"`
	AssetType             string               `json:"asset_type,omitempty"`
	AssetTitle            string               `json:"asset_title,omitempty"`
	AssetSummary          string               `json:"asset_summary,omitempty"`
	AssetTags             []string             `json:"asset_tags,omitempty"`
	IdempotencyKey        string               `json:"idempotency_key"`
}

type CreateComicImageGenerationBatchInput struct {
	SectionUUIDs   []string `json:"section_uuids"`
	IdempotencyKey string   `json:"idempotency_key"`
}

type ComicImageGenerationBatchTask struct {
	UUID         string `json:"uuid"`
	Kind         string `json:"kind"`
	ResourceUUID string `json:"resource_uuid"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type ComicImageGenerationBatch struct {
	ChapterUUID    string                          `json:"chapter_uuid"`
	RequestedCount int                             `json:"requested_count"`
	AcceptedCount  int                             `json:"accepted_count"`
	Tasks          []ComicImageGenerationBatchTask `json:"tasks"`
}

type CreateExportInput struct {
	Scope              string `json:"scope"`
	ChapterUUID        string `json:"chapter_uuid,omitempty"`
	Format             string `json:"format,omitempty"`
	AllowMissingImages bool   `json:"allow_missing_images"`
	IdempotencyKey     string `json:"idempotency_key"`
}

type productionTaskRecord struct {
	ID                                                                                          int64 `gorm:"primaryKey"`
	UUID                                                                                        string
	ProjectID                                                                                   int64
	RiverJobID                                                                                  *int64
	Kind, ResourceUUID, InputSnapshot, Status, IdempotencyKey, ProviderUUID, Model, ModelSource string
	Progress, Attempt, MaxAttempts                                                              int
	ErrorCode, ErrorMessage                                                                     string
	CancelRequestedAt, StartedAt, CompletedAt                                                   *time.Time
	CreatedAt, UpdatedAt                                                                        time.Time
}

func (productionTaskRecord) TableName() string { return "production_task_runs" }
func (r productionTaskRecord) DTO() ProductionTask {
	return ProductionTask{UUID: r.UUID, Kind: r.Kind, ResourceUUID: r.ResourceUUID, InputSnapshot: json.RawMessage(r.InputSnapshot), Status: r.Status, IdempotencyKey: r.IdempotencyKey, ProviderUUID: r.ProviderUUID, Model: r.Model, ModelSource: r.ModelSource, Progress: r.Progress, Attempt: r.Attempt, MaxAttempts: r.MaxAttempts, ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage, CancelRequestedAt: r.CancelRequestedAt, StartedAt: r.StartedAt, CompletedAt: r.CompletedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

type productionTaskEventRecord struct {
	ID                  int64 `gorm:"primaryKey"`
	UUID                string
	ProductionTaskRunID int64
	Sequence            int64
	EventType           string
	Payload             string
	CreatedAt           time.Time
}

func (productionTaskEventRecord) TableName() string { return "production_task_events" }

type maintenanceRecord struct {
	ID                int64 `gorm:"primaryKey;autoIncrement"`
	UUID              string
	ProjectID         int64
	RiverJobID        *int64
	Kind              string
	ResourceUUID      string
	InputVersion      int
	InputSnapshot     string
	Status            string
	Progress          int
	Attempt           int
	MaxAttempts       int
	ErrorCode         string
	ErrorMessage      string
	CancelRequestedAt *time.Time
	StartedAt         *time.Time
	CompletedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type maintenanceEventRecord struct {
	ID               int64 `gorm:"primaryKey;autoIncrement"`
	UUID             string
	MaintenanceRunID int64
	Sequence         int64
	EventType        string
	Payload          string
	CreatedAt        time.Time
}

func (maintenanceEventRecord) TableName() string { return "asset_maintenance_events" }

func (maintenanceRecord) TableName() string { return "asset_maintenance_runs" }
func (record maintenanceRecord) DTO() MaintenanceTask {
	return MaintenanceTask{UUID: record.UUID, Kind: record.Kind, ResourceUUID: record.ResourceUUID, InputVersion: record.InputVersion,
		InputSnapshot: json.RawMessage(record.InputSnapshot), Status: record.Status, Progress: record.Progress, Attempt: record.Attempt,
		MaxAttempts: record.MaxAttempts, ErrorCode: record.ErrorCode, ErrorMessage: record.ErrorMessage,
		CancelRequestedAt: record.CancelRequestedAt, StartedAt: record.StartedAt, CompletedAt: record.CompletedAt,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}
