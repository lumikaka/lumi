package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

type record struct {
	ID                                                                   int64
	UUID, ProviderUUID, ProviderType, Model, Region, RequestType, Source string
	CatalogKey                                                           *string
	RuleJSON                                                             string `gorm:"column:rule_json"`
	Active                                                               bool
	CreatedAt                                                            time.Time
}

func (record) TableName() string { return "model_prices" }
func public(row record) (Price, error) {
	var r Rule
	err := json.Unmarshal([]byte(row.RuleJSON), &r)
	return Price{UUID: row.UUID, Rule: r, Source: row.Source, Active: row.Active, CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339Nano)}, err
}
func ValidUUID(s string) bool { u, e := uuid.Parse(s); return e == nil && u.Version() == 7 }
func (s *Service) List(ctx context.Context) ([]Price, error) {
	rows := []record{}
	if err := s.db.WithContext(ctx).Order("active DESC,provider_type,model,region,created_at DESC,id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := []Price{}
	for _, row := range rows {
		p, err := public(row)
		if err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, nil
}
func (s *Service) Get(ctx context.Context, id string) (Price, error) {
	if !ValidUUID(id) {
		return Price{}, ErrInvalid
	}
	var row record
	err := s.db.WithContext(ctx).Where("uuid=?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Price{}, ErrNotFound
	}
	if err != nil {
		return Price{}, err
	}
	return public(row)
}
func scope(db *gorm.DB, r Rule, source string) *gorm.DB {
	return db.Where("provider_uuid=? AND provider_type=? AND model=? AND region=? AND request_type=? AND source=? AND active=1", r.ProviderUUID, r.ProviderType, r.Model, r.Region, r.RequestType, source)
}
func (s *Service) Create(ctx context.Context, r Rule, expectedUUID string) (Price, error) {
	if Validate(r) != nil || !ValidUUID(r.ProviderUUID) || (expectedUUID != "" && !ValidUUID(expectedUUID)) {
		return Price{}, ErrInvalid
	}
	now := time.Now().UTC()
	id, e := uuid.NewV7()
	if e != nil {
		return Price{}, e
	}
	row := record{UUID: id.String(), ProviderUUID: r.ProviderUUID, ProviderType: r.ProviderType, Model: r.Model, Region: r.Region, RequestType: r.RequestType, Source: "user", RuleJSON: JSON(r), Active: true, CreatedAt: now}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Acquire the SQLite write lock before checking the current version.
		if e := tx.Exec("UPDATE model_prices SET active=active WHERE active=1").Error; e != nil {
			return e
		}
		var old record
		e := scope(tx, r, "user").First(&old).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if (e == nil && old.UUID != expectedUUID) || (errors.Is(e, gorm.ErrRecordNotFound) && expectedUUID != "") {
			return ErrConflict
		}
		if e == nil {
			if e = tx.Model(&record{}).Where("id=?", old.ID).Update("active", false).Error; e != nil {
				return e
			}
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return Price{}, err
	}
	return public(row)
}
func (s *Service) Delete(ctx context.Context, id string) error {
	if !ValidUUID(id) {
		return ErrInvalid
	}
	var row record
	if err := s.db.WithContext(ctx).Where("uuid=? AND source='user'", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return s.db.WithContext(ctx).Model(&record{}).Where("id=?", row.ID).Update("active", false).Error
}
func (s *Service) Freeze(ctx context.Context, c Context) Snapshot {
	result := Snapshot{Context: c, Reason: "missing_price"}
	if s == nil || s.db == nil {
		return result
	}
	var row record
	err := s.db.WithContext(ctx).Where("active=1 AND provider_type=? AND model=? AND region=? AND request_type=? AND (provider_uuid=? OR (source='builtin' AND provider_uuid=''))", c.ProviderType, c.Model, c.Region, c.RequestType, c.ProviderUUID).Order("CASE WHEN source='user' THEN 0 ELSE 1 END").First(&row).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			result.Reason = "price_unavailable"
		}
		return result
	}
	p, err := public(row)
	if err != nil {
		result.Reason = "invalid_price"
		return result
	}
	result.Price = &p
	result.Reason = ""
	return result
}

// Seed appends catalog revisions once. Downgrades do not reactivate an older
// catalog row, and user overrides are never touched.
func (s *Service) Seed(ctx context.Context) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, r := range builtinRules() {
			if err := Validate(r); err != nil {
				return err
			}
			key := "2026-09-05/" + r.ProviderType + "/" + r.Model + "/" + r.Region + "/" + r.RequestType
			var count int64
			if err := tx.Model(&record{}).Where("catalog_key=?", key).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			if err := scope(tx.Model(&record{}), r, "builtin").Update("active", false).Error; err != nil {
				return err
			}
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			row := record{UUID: id.String(), ProviderType: r.ProviderType, Model: r.Model, Region: r.Region, RequestType: r.RequestType, Source: "builtin", CatalogKey: &key, RuleJSON: JSON(r), Active: true, CreatedAt: time.Now().UTC()}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
