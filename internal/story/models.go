package story

import (
	"time"

	"lumi/internal/project"
)

type chapterRecord struct {
	ID             int64 `gorm:"primaryKey;autoIncrement"`
	UUID           string
	ProjectID      int64
	VolumeNo       int
	ChapterNo      int
	ChapterCode    string
	SortOrder      int
	Title          string
	CurrentStoryID *int64
	Revision       int64
	DeletedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (chapterRecord) TableName() string { return "chapters" }

type chapterStoryRecord struct {
	ID                int64 `gorm:"primaryKey;autoIncrement"`
	UUID              string
	ChapterID         int64
	ActorID           int64
	StorySourceID     int64
	StorySourceItemID int64
	VersionNo         int
	SourceType        string
	Content           string
	ContentFormat     string
	ContentHash       string
	CharCount         int
	CreatedAt         time.Time
}

func (chapterStoryRecord) TableName() string { return "chapter_stories" }

type storySourceRecord struct {
	ID          int64 `gorm:"primaryKey;autoIncrement"`
	UUID        string
	ProjectID   int64
	ActorID     int64
	SourceType  string
	RequestHash string
	ItemCount   int
	CreatedAt   time.Time
}

func (storySourceRecord) TableName() string { return "story_sources" }

type storySourceItemRecord struct {
	ID               int64 `gorm:"primaryKey;autoIncrement"`
	UUID             string
	SourceID         int64
	ChapterID        *int64
	Ordinal          int
	OriginalFilename *string
	ContentFormat    string
	ContentHash      string
	ByteSize         int64
	FileID           *int64
	CreatedAt        time.Time
}

func (storySourceItemRecord) TableName() string { return "story_source_items" }

type storyProfileRecord struct {
	ID               int64 `gorm:"primaryKey;autoIncrement"`
	UUID             string
	ProjectID        int64
	ActorID          int64
	VersionNo        int
	Revision         int64
	IsCurrent        bool
	StoryMD          string `gorm:"column:story_md"`
	ContentHash      string
	SourceType       string
	ProjectionState  string
	ExportedRevision int64
	ExportedHash     string
	ObservedFileHash string
	CreatedAt        time.Time
}

func (storyProfileRecord) TableName() string { return "project_story_profiles" }

type promptVersionRecord struct {
	ID                    int64 `gorm:"primaryKey;autoIncrement"`
	UUID                  string
	ProjectID             int64
	ActorID               int64
	RestoredFromVersionID *int64
	PromptGroup           string
	PromptKey             string
	VersionNo             int
	Prompt                string
	PromptHash            string
	SourceType            string
	CreatedAt             time.Time
}

func (promptVersionRecord) TableName() string { return "project_prompt_versions" }

type ProjectDetail struct {
	UUID               string                     `json:"uuid"`
	Name               string                     `json:"name"`
	Description        string                     `json:"description"`
	GenerationLanguage string                     `json:"generation_language"`
	Revision           int64                      `json:"revision"`
	ChapterCount       int64                      `json:"chapter_count"`
	TrashCount         int64                      `json:"trash_count"`
	UpdatedAt          time.Time                  `json:"updated_at"`
	PictureBook        project.PictureBookProfile `json:"picture_book"`
}

type ChapterStory struct {
	UUID           string    `json:"uuid"`
	VersionNo      int       `json:"version_no"`
	SourceType     string    `json:"source_type"`
	SourceUUID     string    `json:"source_uuid"`
	SourceItemUUID string    `json:"source_item_uuid"`
	Content        string    `json:"content"`
	ContentFormat  string    `json:"content_format"`
	ContentHash    string    `json:"content_hash"`
	CharCount      int       `json:"char_count"`
	CreatedAt      time.Time `json:"created_at"`
}

type Chapter struct {
	UUID         string        `json:"uuid"`
	ChapterCode  string        `json:"chapter_code"`
	VolumeNo     int           `json:"volume_no"`
	ChapterNo    int           `json:"chapter_no"`
	SortOrder    int           `json:"sort_order"`
	Title        string        `json:"title"`
	Revision     int64         `json:"revision"`
	TrashedAt    *time.Time    `json:"trashed_at"`
	CurrentStory *ChapterStory `json:"current_story"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type ChapterTrashBlockedItem struct {
	UUID        string `json:"uuid"`
	ChapterCode string `json:"chapter_code"`
	ErrorCode   string `json:"error_code"`
}

type EmptyChapterTrashResult struct {
	DeletedCount int                       `json:"deleted_count"`
	BlockedItems []ChapterTrashBlockedItem `json:"blocked_items"`
}

type StoryProfile struct {
	UUID             string    `json:"uuid"`
	VersionNo        int       `json:"version_no"`
	Revision         int64     `json:"revision"`
	StoryMD          string    `json:"story_md"`
	ContentHash      string    `json:"content_hash"`
	SourceType       string    `json:"source_type"`
	ProjectionState  string    `json:"projection_state"`
	ExportedRevision int64     `json:"exported_revision"`
	ExportedHash     string    `json:"exported_hash"`
	ObservedFileHash string    `json:"observed_file_hash"`
	CreatedAt        time.Time `json:"created_at"`
}

type PromptVersion struct {
	UUID             string    `json:"uuid"`
	PromptGroup      string    `json:"prompt_group"`
	PromptKey        string    `json:"prompt_key"`
	VersionNo        int       `json:"version_no"`
	Prompt           string    `json:"prompt"`
	PromptHash       string    `json:"prompt_hash"`
	SourceType       string    `json:"source_type"`
	RestoredFromUUID string    `json:"restored_from_uuid,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type PromptCatalogItem struct {
	PromptGroup    string         `json:"prompt_group"`
	PromptKey      string         `json:"prompt_key"`
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	PromptType     string         `json:"prompt_type"`
	DefaultValue   string         `json:"default_value"`
	EffectiveValue string         `json:"effective_value"`
	Placeholders   []string       `json:"placeholders"`
	LegacyKeys     []string       `json:"legacy_keys,omitempty"`
	IsCustom       bool           `json:"is_custom"`
	UsingLegacyKey string         `json:"using_legacy_key,omitempty"`
	CurrentVersion *PromptVersion `json:"current_version"`
}

type Pagination struct {
	PerPage     int   `json:"per_page"`
	CurrentPage int   `json:"current_page"`
	LastPage    int   `json:"last_page"`
	Total       int64 `json:"total"`
}
