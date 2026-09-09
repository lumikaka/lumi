package appstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrGlobalModelSettingsConflict = errors.New("global model settings revision conflict")

// GlobalModelSelection stores a fixed provider type, like site settings. Public
// provider identities are resolved at the API boundary, never used as DB joins.
type GlobalModelSelection struct {
	ProviderType   string `json:"provider_type"`
	Model          string `json:"model"`
	EnableThinking *bool  `json:"enable_thinking,omitempty"`
	PromptExtend   *bool  `json:"prompt_extend,omitempty"`
}

type GlobalModelSettings struct {
	ID                   int64 `gorm:"primaryKey;autoIncrement" json:"-"`
	Singleton            int `json:"-"`
	Settings             map[string]*GlobalModelSelection `gorm:"serializer:json"`
	Revision             int
	CreatedAt, UpdatedAt time.Time
}

func (GlobalModelSettings) TableName() string { return "global_model_settings" }

func (store *Store) GlobalModelSettings(ctx context.Context) (GlobalModelSettings, error) {
	var row GlobalModelSettings
	err := store.db.WithContext(ctx).Where("singleton = ?", 1).First(&row).Error
	return row, err
}

func (store *Store) PatchGlobalModelSettings(ctx context.Context, expectedRevision int, changes map[string]*GlobalModelSelection) (GlobalModelSettings, error) {
	var row GlobalModelSettings
	err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Acquire SQLite's writer lock before reading and merging. The compare-
		// and-swap also makes concurrent callers return a conflict, not lose edits.
		result := tx.Model(&GlobalModelSettings{}).Where("singleton = ? AND revision = ?", 1, expectedRevision).Updates(map[string]any{
			"revision": gorm.Expr("revision + 1"), "updated_at": time.Now().UTC(),
		})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrGlobalModelSettingsConflict }
		if err := tx.Where("singleton = ?", 1).First(&row).Error; err != nil { return err }
		if row.Settings == nil { row.Settings = make(map[string]*GlobalModelSelection) }
		for key, selection := range changes {
			if selection == nil { delete(row.Settings, key) } else { row.Settings[key] = selection }
		}
		data, err := json.Marshal(row.Settings)
		if err != nil { return err }
		return tx.Model(&GlobalModelSettings{}).Where("id = ?", row.ID).Update("settings", string(data)).Error
	})
	return row, err
}
