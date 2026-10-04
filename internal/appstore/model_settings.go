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

type GlobalModelRateLimit struct {
	RPM *int `json:"rpm"`
	RPS *int `json:"rps"`
}

type GlobalModelSettings struct {
	ID                   int64                                      `gorm:"primaryKey;autoIncrement" json:"-"`
	Singleton            int                                        `json:"-"`
	Settings             map[string]*GlobalModelSelection           `gorm:"serializer:json"`
	RateLimits           map[string]map[string]GlobalModelRateLimit `gorm:"serializer:json"`
	// ProviderRateLimits contains only saved rules; the Cloudflare default is seeded once by migration.
	ProviderRateLimits   map[string]GlobalModelRateLimit            `gorm:"serializer:json"`
	Revision             int
	CreatedAt, UpdatedAt time.Time
}

func (GlobalModelSettings) TableName() string { return "global_model_settings" }

func (store *Store) GlobalModelSettings(ctx context.Context) (GlobalModelSettings, error) {
	var row GlobalModelSettings
	err := store.db.WithContext(ctx).Where("singleton = ?", 1).First(&row).Error
	return row, err
}

func (store *Store) PatchGlobalModelSettings(ctx context.Context, expectedRevision int, changes map[string]*GlobalModelSelection, limitChanges map[string]map[string]*GlobalModelRateLimit, providerLimitChanges map[string]*GlobalModelRateLimit) (GlobalModelSettings, error) {
	var row GlobalModelSettings
	err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Acquire SQLite's writer lock before reading and merging. The compare-
		// and-swap also makes concurrent callers return a conflict, not lose edits.
		result := tx.Model(&GlobalModelSettings{}).Where("singleton = ? AND revision = ?", 1, expectedRevision).Updates(map[string]any{
			"revision": gorm.Expr("revision + 1"), "updated_at": time.Now().UTC(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrGlobalModelSettingsConflict
		}
		if err := tx.Where("singleton = ?", 1).First(&row).Error; err != nil {
			return err
		}
		if row.Settings == nil {
			row.Settings = make(map[string]*GlobalModelSelection)
		}
		if row.RateLimits == nil {
			row.RateLimits = make(map[string]map[string]GlobalModelRateLimit)
		}
		if row.ProviderRateLimits == nil {
			row.ProviderRateLimits = make(map[string]GlobalModelRateLimit)
		}
		for key, selection := range changes {
			if selection == nil {
				delete(row.Settings, key)
			} else {
				row.Settings[key] = selection
			}
		}
		data, err := json.Marshal(row.Settings)
		if err != nil {
			return err
		}
		for providerType, models := range limitChanges {
			if row.RateLimits[providerType] == nil {
				row.RateLimits[providerType] = make(map[string]GlobalModelRateLimit)
			}
			for model, limit := range models {
				if limit == nil {
					delete(row.RateLimits[providerType], model)
				} else {
					row.RateLimits[providerType][model] = *limit
				}
			}
			if len(row.RateLimits[providerType]) == 0 {
				delete(row.RateLimits, providerType)
			}
		}
		limitsData, err := json.Marshal(row.RateLimits)
		if err != nil {
			return err
		}
		for providerType, limit := range providerLimitChanges {
			if limit == nil {
				delete(row.ProviderRateLimits, providerType)
			} else {
				row.ProviderRateLimits[providerType] = *limit
			}
		}
		providerLimitsData, err := json.Marshal(row.ProviderRateLimits)
		if err != nil {
			return err
		}
		return tx.Model(&GlobalModelSettings{}).Where("id = ?", row.ID).Updates(map[string]any{"settings": string(data), "rate_limits": string(limitsData), "provider_rate_limits": string(providerLimitsData)}).Error
	})
	return row, err
}
