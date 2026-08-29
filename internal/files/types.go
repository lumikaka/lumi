package files

import (
	"encoding/json"
	"io"
	"os"
	"time"

	"gorm.io/gorm"
)

const (
	StateReceiving = "receiving"
	StateReady     = "ready"
	StateConsuming = "consuming"
	StateConsumed  = "consumed"
	StateFailed    = "failed"
	StateExpired   = "expired"

	ObjectPending     = "pending"
	ObjectReady       = "ready"
	ObjectMissing     = "missing"
	ObjectCorrupt     = "corrupt"
	ObjectQuarantined = "quarantined"
)

type Upload struct {
	UUID             string     `json:"uuid"`
	Purpose          string     `json:"purpose"`
	OriginalFilename string     `json:"original_filename"`
	DisplayName      string     `json:"display_name,omitempty"`
	MIMEType         string     `json:"mime_type,omitempty"`
	ByteSize         *int64     `json:"byte_size,omitempty"`
	SHA256           string     `json:"sha256,omitempty"`
	Width            *int       `json:"width,omitempty"`
	Height           *int       `json:"height,omitempty"`
	DurationMS       *int64     `json:"duration_ms,omitempty"`
	State            string     `json:"state"`
	ErrorCode        string     `json:"error_code,omitempty"`
	AssetUUID        string     `json:"asset_uuid,omitempty"`
	ExpiresAt        time.Time  `json:"expires_at"`
	ConsumedAt       *time.Time `json:"consumed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type Asset struct {
	UUID             string     `json:"uuid"`
	Kind             string     `json:"kind"`
	Purpose          string     `json:"purpose"`
	OriginalFilename string     `json:"original_filename,omitempty"`
	DisplayName      string     `json:"display_name,omitempty"`
	SourceType       string     `json:"source_type"`
	SourceAssetUUID  string     `json:"source_asset_uuid,omitempty"`
	MIMEType         string     `json:"mime_type"`
	ByteSize         int64      `json:"byte_size"`
	Width            *int       `json:"width,omitempty"`
	Height           *int       `json:"height,omitempty"`
	DurationMS       *int64     `json:"duration_ms,omitempty"`
	Status           string     `json:"status"`
	Metadata         any        `json:"metadata"`
	ContentURL       string     `json:"content_url"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type CreateUploadInput struct {
	Purpose          string
	OriginalFilename string
	DisplayName      string
	Metadata         map[string]any
	Reader           io.Reader
}

// UploadIdentity is reserved for trusted domain workflows that must replay an
// upload across process boundaries without creating another logical File.
type UploadIdentity struct {
	UploadUUID string
	FileUUID   string
}

type CommitInput struct {
	Purpose          string
	OriginalFilename string
	DisplayName      string
	SourceType       string
	SourceAssetUUID  string
	Metadata         map[string]any
	Reader           io.Reader
	Bind             func(*gorm.DB, int64) error
}

type UpdateAssetInput struct {
	DisplayName *string
	Metadata    map[string]any
}

type AssetFilter struct {
	Purpose        string
	Kind           string
	IncludeTrashed bool
	TrashedOnly    bool
	Limit          int
}

type Content struct {
	File         *os.File
	Asset        Asset
	ETag         string
	LastModified time.Time
	Filename     string
}

type Finding struct {
	UUID       string     `json:"uuid"`
	Kind       string     `json:"kind"`
	Severity   string     `json:"severity"`
	SafePath   string     `json:"safe_path,omitempty"`
	Summary    string     `json:"summary"`
	Resolution string     `json:"resolution"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

type IntegrityScan struct {
	UUID           string     `json:"uuid"`
	Mode           string     `json:"mode"`
	Status         string     `json:"status"`
	Progress       int        `json:"progress"`
	CheckedObjects int        `json:"checked_objects"`
	FindingCount   int        `json:"finding_count"`
	Summary        any        `json:"summary"`
	Findings       []Finding  `json:"findings,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type GCEntry struct {
	ObjectUUID       string         `json:"object_uuid"`
	SHA256           string         `json:"sha256"`
	SafeKeyPath      string         `json:"safe_key_path"`
	ByteSize         int64          `json:"byte_size"`
	ReferenceSummary map[string]any `json:"reference_summary"`
}

type GCPlan struct {
	UUID           string     `json:"uuid"`
	SnapshotHash   string     `json:"snapshot_hash"`
	Status         string     `json:"status"`
	EstimatedBytes int64      `json:"estimated_bytes"`
	Entries        []GCEntry  `json:"entries"`
	CreatedAt      time.Time  `json:"created_at"`
	AppliedAt      *time.Time `json:"applied_at,omitempty"`
}

type uploadRecord struct {
	ID               int64 `gorm:"primaryKey;autoIncrement"`
	UUID             string
	ProjectID        int64
	ActorID          int64
	ReservedFileUUID string
	Purpose          string
	OriginalFilename string
	DisplayName      string
	MIMEType         *string
	CanonicalExt     *string
	ByteSize         *int64
	SHA256           *string
	Width            *int
	Height           *int
	DurationMS       *int64
	MetadataJSON     string
	State            string
	FileObjectID     *int64
	FinalizedFileID  *int64
	ErrorCode        string
	ExpiresAt        time.Time
	ConsumedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (uploadRecord) TableName() string { return "upload_stashed" }

type objectRecord struct {
	ID           int64 `gorm:"primaryKey;autoIncrement"`
	UUID         string
	ProjectID    int64
	SHA256       string
	KeyPath      string
	MIMEType     string
	CanonicalExt string
	ByteSize     int64
	Width        *int
	Height       *int
	DurationMS   *int64
	State        string
	CreatedAt    time.Time
	VerifiedAt   *time.Time
}

func (objectRecord) TableName() string { return "file_objects" }

type fileRecord struct {
	ID               int64 `gorm:"primaryKey;autoIncrement"`
	UUID             string
	ProjectID        int64
	FileObjectID     int64
	Kind             string
	Purpose          string
	OriginalFilename *string
	DisplayName      *string
	SourceType       string
	SourceFileID     *int64
	MetadataJSON     string
	ActorID          *int64
	CreatedAt        time.Time
	DeletedAt        *time.Time
}

func (fileRecord) TableName() string { return "files" }

type assetRow struct {
	ID               int64
	UUID             string
	ProjectID        int64
	FileObjectID     int64
	Kind             string
	Purpose          string
	OriginalFilename *string
	DisplayName      *string
	SourceType       string
	SourceFileID     *int64
	MetadataJSON     string
	ActorID          *int64
	CreatedAt        time.Time
	DeletedAt        *time.Time
	ObjectUUID       string `gorm:"column:object_uuid"`
	SHA256           string
	KeyPath          string
	MIMEType         string
	CanonicalExt     string
	ByteSize         int64
	Width            *int
	Height           *int
	DurationMS       *int64
	ObjectState      string `gorm:"column:object_state"`
	VerifiedAt       *time.Time
	SourceAssetUUID  string `gorm:"column:source_asset_uuid"`
}

func decodeMetadata(raw string) map[string]any {
	value := map[string]any{}
	_ = json.Unmarshal([]byte(raw), &value)
	return value
}
