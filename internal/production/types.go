package production

import (
	"encoding/json"
	"time"

	"lumi/internal/files"
	"lumi/internal/project"
)

const (
	AssetCharacter = "character"
	AssetScene     = "scene"
	AssetProp      = "prop"
	AssetReference = "reference"
)

type Pagination struct {
	PerPage     int   `json:"per_page"`
	CurrentPage int   `json:"current_page"`
	LastPage    int   `json:"last_page"`
	Total       int64 `json:"total"`
}

type PremiseProfile struct {
	UUID                string         `json:"uuid"`
	DefaultStyle        string         `json:"default_style"`
	CurrentSource       *PremiseSource `json:"current_source"`
	CurrentSettingImage *SettingImage  `json:"current_setting_image"`
	Revision            int64          `json:"revision"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

type PremiseSource struct {
	UUID          string          `json:"uuid"`
	SourceType    string          `json:"source_type"`
	SourceText    string          `json:"source_text"`
	StyleSnapshot string          `json:"style_snapshot"`
	ProviderUUID  string          `json:"provider_uuid,omitempty"`
	Model         string          `json:"model,omitempty"`
	Parameters    json.RawMessage `json:"parameters"`
	IgnoredAt     *time.Time      `json:"ignored_at,omitempty"`
	Revision      int64           `json:"revision"`
	CreatedAt     time.Time       `json:"created_at"`
}

type SettingImage struct {
	UUID       string      `json:"uuid"`
	SourceUUID string      `json:"source_uuid,omitempty"`
	Origin     string      `json:"origin"`
	Prompt     string      `json:"prompt"`
	Asset      files.Asset `json:"asset"`
	CreatedAt  time.Time   `json:"created_at"`
}

type PremiseAsset struct {
	UUID           string          `json:"uuid"`
	AssetType      string          `json:"asset_type"`
	Title          string          `json:"title"`
	Summary        string          `json:"summary"`
	Tags           []string        `json:"tags"`
	Position       json.RawMessage `json:"position"`
	Crop           json.RawMessage `json:"crop"`
	CurrentVariant *AssetVariant   `json:"current_variant"`
	Revision       int64           `json:"revision"`
	DeletedAt      *time.Time      `json:"deleted_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type AssetVariant struct {
	UUID                   string          `json:"uuid"`
	VersionNo              int             `json:"version_no"`
	SourceType             string          `json:"source_type"`
	SourceSettingImageUUID string          `json:"source_setting_image_uuid,omitempty"`
	Crop                   json.RawMessage `json:"crop"`
	Asset                  files.Asset     `json:"asset"`
	CreatedAt              time.Time       `json:"created_at"`
}

type ComicState struct {
	UUID              string    `json:"uuid"`
	ChapterUUID       string    `json:"chapter_uuid"`
	Status            string    `json:"status"`
	HasPremiseAssets  bool      `json:"has_premise_assets"`
	PremiseAssetCount int       `json:"premise_asset_count"`
	Revision          int64     `json:"revision"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type ComicSection struct {
	UUID              string             `json:"uuid"`
	ChapterUUID       string             `json:"chapter_uuid"`
	SectionNo         int                `json:"section_no"`
	Title             string             `json:"title"`
	DescriptionMD     string             `json:"description_md"`
	CurrentStoryboard *StoryboardVariant `json:"current_storyboard"`
	CurrentImage      *ImageVariant      `json:"current_image"`
	Revision          int64              `json:"revision"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

type StoryboardVariant struct {
	UUID       string    `json:"uuid"`
	VersionNo  int       `json:"version_no"`
	ContentMD  string    `json:"content_md"`
	SourceType string    `json:"source_type"`
	CreatedAt  time.Time `json:"created_at"`
}

type ImageVariant struct {
	UUID           string                  `json:"uuid"`
	VersionNo      int                     `json:"version_no"`
	SourceType     string                  `json:"source_type"`
	GenerationUUID string                  `json:"generation_uuid,omitempty"`
	InputSnapshot  json.RawMessage         `json:"input_snapshot"`
	Generation     *ImageGenerationSummary `json:"generation,omitempty"`
	Asset          files.Asset             `json:"asset"`
	SectionPremise *SectionPremise         `json:"section_premise,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
}

type ImageGenerationSummary struct {
	UUID         string `json:"uuid"`
	Status       string `json:"status"`
	ProviderUUID string `json:"provider_uuid,omitempty"`
	ProviderType string `json:"provider_type,omitempty"`
	Model        string `json:"model,omitempty"`
	ModelSource  string `json:"model_source,omitempty"`
}

type SectionPremise struct {
	Asset           files.Asset             `json:"asset"`
	SelectedAssets  []PremiseAssetReference `json:"selected_assets"`
	SelectedTitles  []string                `json:"selected_titles"`
	SelectionReason string                  `json:"selection_reason"`
	ImageInfo       SectionPremiseImageInfo `json:"image_info"`
}

type SectionPremiseImageInfo struct {
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	MIMEType        string `json:"mime_type"`
	ComposerVersion string `json:"composer_version"`
}

type SectionPremiseMetadata struct {
	SelectedAssets  []PremiseAssetReference `json:"selected_assets"`
	SelectedTitles  []string                `json:"selected_titles"`
	SelectionReason string                  `json:"selection_reason"`
	ImageInfo       SectionPremiseImageInfo `json:"image_info"`
}

type ChapterSnapshot struct {
	UUID         string    `json:"uuid"`
	VersionNo    int       `json:"version_no"`
	Reason       string    `json:"reason"`
	Source       string    `json:"source"`
	SectionCount int       `json:"section_count"`
	CreatedAt    time.Time `json:"created_at"`
}

type ChapterSnapshotDetail struct {
	ChapterSnapshot
	SchemaVersion int                      `json:"schema_version"`
	Chapter       ChapterSnapshotChapter   `json:"chapter"`
	Sections      []ChapterSnapshotSection `json:"sections"`
}

type ChapterSnapshotChapter struct {
	UUID        string `json:"uuid"`
	ChapterCode string `json:"chapter_code"`
	Title       string `json:"title"`
}

type ChapterSnapshotSection struct {
	UUID             string               `json:"uuid"`
	SectionNo        int                  `json:"section_no"`
	Title            string               `json:"title"`
	StoryboardMD     string               `json:"storyboard_md"`
	CurrentImage     ChapterSnapshotMedia `json:"current_image"`
	PremiseReference ChapterSnapshotMedia `json:"premise_reference"`
}

type ChapterSnapshotMedia struct {
	Status      string       `json:"status"`
	VariantUUID string       `json:"variant_uuid,omitempty"`
	AssetUUID   string       `json:"asset_uuid,omitempty"`
	Asset       *files.Asset `json:"asset,omitempty"`
}

type Export struct {
	UUID          string          `json:"uuid"`
	TaskUUID      string          `json:"task_uuid"`
	Scope         string          `json:"scope"`
	ChapterUUID   string          `json:"chapter_uuid,omitempty"`
	Format        string          `json:"format"`
	Filename      string          `json:"filename"`
	Status        string          `json:"status"`
	Snapshot      json.RawMessage `json:"snapshot"`
	SnapshotHash  string          `json:"snapshot_hash"`
	DownloadURL   string          `json:"download_url"`
	ExpiresAt     *time.Time      `json:"expires_at"`
	RetentionDays int             `json:"retention_days"`
	ByteSize      int64           `json:"byte_size"`
	ContentSHA256 string          `json:"content_sha256"`
	// OutputAsset is only populated for exports created before short-lived exports stopped
	// being registered in the Asset Store. New clients use DownloadURL.
	OutputAsset  *files.Asset `json:"output_asset,omitempty"`
	RelativePath string       `json:"relative_path,omitempty"`
	ErrorCode    string       `json:"error_code,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	CompletedAt  *time.Time   `json:"completed_at,omitempty"`
}

type UpdatePremiseInput struct {
	DefaultStyle     string
	ExpectedRevision int64
}

type CreateSourceInput struct {
	SourceText    string
	StyleSnapshot string
	SourceType    string
	ProviderUUID  string
	Model         string
	Parameters    any
}

type CreateAssetInput struct {
	UploadUUID             string
	FileUUID               string
	ToolExecutionUUID      string
	ChatThreadUUID         string
	SourcePremiseAssetUUID string
	AssetType              string
	Title                  string
	Summary                string
	Tags                   []string
	Position               any
	Crop                   any
	SourceType             string
	SourceSettingImageUUID string
}

type UpdateAssetInput struct {
	FileUUID          string
	ToolExecutionUUID string
	ChatThreadUUID    string
	AssetType         *string
	Title             *string
	Summary           *string
	Tags              *[]string
	Position          any
	Crop              any
	ExpectedRevision  int64
}

type CreateSectionInput struct {
	Title         string
	DescriptionMD string
	StoryboardMD  string
}

type GeneratedComicSection struct {
	Title        string
	StoryboardMD string
}

type UpdateSectionInput struct {
	Title            *string
	DescriptionMD    *string
	ExpectedRevision int64
}

type GenerationSnapshot struct {
	Version                   int                         `json:"version"`
	Kind                      string                      `json:"kind"`
	ProjectUUID               string                      `json:"project_uuid"`
	GenerationLanguage        string                      `json:"generation_language,omitempty"`
	ResourceUUID              string                      `json:"resource_uuid"`
	SourceUUID                string                      `json:"source_uuid,omitempty"`
	SourceText                string                      `json:"source_text,omitempty"`
	ChapterUUID               string                      `json:"chapter_uuid,omitempty"`
	Prompt                    string                      `json:"prompt"`
	PromptTemplate            string                      `json:"prompt_template,omitempty"`
	LanguageInstruction       string                      `json:"language_instruction,omitempty"`
	ReferencePresentPrompt    string                      `json:"reference_present_prompt,omitempty"`
	ReferenceAbsentPrompt     string                      `json:"reference_absent_prompt,omitempty"`
	AdditionalDirectionPrompt string                      `json:"additional_direction_prompt,omitempty"`
	SelectionPrompt           string                      `json:"selection_prompt,omitempty"`
	SelectionProviderUUID     string                      `json:"selection_provider_uuid,omitempty"`
	SelectionBaseURL          string                      `json:"selection_base_url,omitempty"`
	SelectionModel            string                      `json:"selection_model,omitempty"`
	SelectionModelSource      string                      `json:"selection_model_source,omitempty"`
	StyleSnapshot             string                      `json:"style_snapshot"`
	StoryboardUUID            string                      `json:"storyboard_uuid,omitempty"`
	StoryboardMD              string                      `json:"storyboard_md,omitempty"`
	PremiseAssets             []PremiseAssetReference     `json:"premise_assets,omitempty"`
	PremiseCandidates         []PremiseAssetReference     `json:"premise_candidates,omitempty"`
	AssetOperation            string                      `json:"asset_operation,omitempty"`
	AssetType                 string                      `json:"asset_type,omitempty"`
	AssetTitle                string                      `json:"asset_title,omitempty"`
	AssetSummary              string                      `json:"asset_summary,omitempty"`
	AssetTags                 []string                    `json:"asset_tags,omitempty"`
	AssetRevision             int64                       `json:"asset_revision,omitempty"`
	ProviderUUID              string                      `json:"provider_uuid,omitempty"`
	ProviderType              string                      `json:"provider_type,omitempty"`
	ProviderBaseURL           string                      `json:"provider_base_url,omitempty"`
	Model                     string                      `json:"model,omitempty"`
	ModelSource               string                      `json:"model_source,omitempty"`
	PictureBook               *project.PictureBookProfile `json:"picture_book,omitempty"`
	OutputSize                string                      `json:"output_size,omitempty"`
	Parameters                json.RawMessage             `json:"parameters"`
}

type PremiseAssetReference struct {
	AssetUUID   string `json:"asset_uuid"`
	VariantUUID string `json:"variant_uuid"`
	FileUUID    string `json:"file_uuid,omitempty"`
	Title       string `json:"title,omitempty"`
}

type ExportSnapshot struct {
	Version              int                         `json:"version"`
	Format               string                      `json:"format,omitempty"`
	ProjectUUID          string                      `json:"project_uuid"`
	ProjectTitle         string                      `json:"project_title,omitempty"`
	Scope                string                      `json:"scope"`
	ChapterUUID          string                      `json:"chapter_uuid,omitempty"`
	AllowMissingImages   bool                        `json:"allow_missing_images"`
	ActiveChapterCount   int                         `json:"active_chapter_count"`
	SectionCount         int                         `json:"section_count"`
	ExportedSectionCount int                         `json:"exported_section_count"`
	MissingSectionCount  int                         `json:"missing_section_count"`
	MissingSectionUUIDs  []string                    `json:"missing_section_uuids"`
	Entries              []ExportEntry               `json:"entries"`
	PictureBook          *project.PictureBookProfile `json:"picture_book,omitempty"`
	PDFLayout            *ExportPDFLayout            `json:"pdf_layout,omitempty"`
}

type ExportEntry struct {
	ChapterUUID    string `json:"chapter_uuid,omitempty"`
	ChapterCode    string `json:"chapter_code"`
	ChapterTitle   string `json:"chapter_title"`
	SectionNo      int    `json:"section_no"`
	SectionUUID    string `json:"section_uuid"`
	ImageAssetUUID string `json:"image_asset_uuid"`
	Extension      string `json:"extension"`
	MIMEType       string `json:"mime_type,omitempty"`
	Width          int    `json:"width,omitempty"`
	Height         int    `json:"height,omitempty"`
}

type ExportPDFLayout struct {
	PageSize        string `json:"page_size"`
	Placement       string `json:"placement"`
	MarginMM        int    `json:"margin_mm"`
	GutterMM        int    `json:"gutter_mm"`
	RendererVersion int    `json:"renderer_version"`
}

type ExportMissingSection struct {
	UUID        string `json:"uuid"`
	SectionNo   int    `json:"section_no"`
	Title       string `json:"title"`
	ChapterUUID string `json:"chapter_uuid"`
}

type ExportReadiness struct {
	Scope               string                 `json:"scope"`
	ChapterUUID         string                 `json:"chapter_uuid,omitempty"`
	ActiveChapterCount  int                    `json:"active_chapter_count"`
	ActiveSectionCount  int                    `json:"active_section_count"`
	ImageSectionCount   int                    `json:"image_section_count"`
	MissingSectionCount int                    `json:"missing_section_count"`
	CanExport           bool                   `json:"can_export"`
	Complete            bool                   `json:"complete"`
	MissingSections     []ExportMissingSection `json:"missing_sections"`
}

type PremiseAssetDeleteBlocker struct {
	UUID   string `json:"uuid"`
	Reason string `json:"reason"`
}

type PremiseTrashDeleteResult struct {
	DeletedCount         int                         `json:"deleted_count"`
	FileSoftDeletedCount int                         `json:"file_soft_deleted_count"`
	RetainedFileCount    int                         `json:"retained_file_count"`
	BlockedItems         []PremiseAssetDeleteBlocker `json:"blocked_items"`
}
