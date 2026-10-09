package modeltest

import (
	"errors"
	"time"
)

var (
	ErrInvalid  = errors.New("model test selection is invalid")
	ErrNotFound = errors.New("model test not found")
	ErrConflict = errors.New("model test execution already has different selections")
	ErrClosed   = errors.New("model test service is closing")
)

const EventChanged = "model_test:changed"

type Selection struct {
	ProviderUUID string `json:"provider_uuid"`
	Model        string `json:"model"`
}

type Model struct {
	Selection    `gorm:"embedded"`
	ProviderType string `json:"provider_type"`
	ProviderName string `json:"provider_name"`
}

type ExecutionInput struct {
	TextModel  Selection `json:"text_model"`
	ImageModel Selection `json:"image_model"`
}

type Test struct {
	ID          int64      `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID        string     `json:"uuid"`
	Status      string     `json:"status"`
	TextModel   Model      `gorm:"embedded;embeddedPrefix:text_" json:"text_model"`
	ImageModel  Model      `gorm:"embedded;embeddedPrefix:image_" json:"image_model"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Steps       []Step     `gorm:"-" json:"steps,omitempty"`
}

func (Test) TableName() string { return "model_tests" }

type Step struct {
	ID                int64      `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID              string     `json:"uuid"`
	ModelTestID       int64      `json:"-"`
	Position          int        `json:"position"`
	Kind              string     `json:"kind"`
	Status            string     `json:"status"`
	StartedAt         *time.Time `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	DurationMS        int64      `json:"duration_ms"`
	OutputText        string     `json:"output_text,omitempty"`
	ErrorCode         string     `json:"error_code,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	HTTPStatus        int        `json:"http_status,omitempty"`
	ProviderErrorCode string     `json:"provider_error_code,omitempty"`
	ProviderRequestID string     `json:"provider_request_id,omitempty"`
	ImageMIMEType     string     `json:"image_mime_type,omitempty"`
	ImageWidth        int        `json:"image_width,omitempty"`
	ImageHeight       int        `json:"image_height,omitempty"`
	ContentURL        string     `gorm:"-" json:"content_url,omitempty"`
}

func (Step) TableName() string { return "model_test_steps" }

type Pagination struct {
	PerPage     int   `json:"per_page"`
	CurrentPage int   `json:"current_page"`
	LastPage    int   `json:"last_page"`
	Total       int64 `json:"total"`
}

type Page struct {
	Items      []Test     `json:"items"`
	Pagination Pagination `json:"pagination"`
}
