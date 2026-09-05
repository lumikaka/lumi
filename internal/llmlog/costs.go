package llmlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"lumi/internal/pricing"
	"lumi/internal/project"
)

type CostGroup struct {
	Currency    string `json:"currency"`
	Model       string `json:"model,omitempty"`
	Scenario    string `json:"scenario,omitempty"`
	RequestType string `json:"request_type,omitempty"`
	Count       int64  `json:"count"`
	Amount      string `json:"amount"`
	Nanos       int64  `json:"-"`
}
type CostSummary struct {
	Total      int64       `json:"total"`
	Calculated int64       `json:"calculated"`
	Unpriced   int64       `json:"unpriced"`
	Pending    int64       `json:"pending"`
	Totals     []CostGroup `json:"totals" gorm:"-"`
	ByModel    []CostGroup `json:"by_model" gorm:"-"`
	ByScenario []CostGroup `json:"by_scenario" gorm:"-"`
}

func (s *Service) costFilter(ctx context.Context, f Filter) (string, []any, error) {
	f = normalizeFilter(f)
	if err := validateFilter(f); err != nil {
		return "", nil, err
	}
	var id int64
	if err := s.store.DB().WithContext(ctx).Model(&project.Project{}).Where("uuid=?", s.store.ProjectUUID()).Pluck("id", &id).Error; err != nil {
		return "", nil, err
	}
	where, args := filterWhere(f)
	return "SELECT * FROM (" + unifiedLogsSQL + ")" + where, append([]any{id}, args...), nil
}
func (s *Service) CostSummary(ctx context.Context, f Filter) (CostSummary, error) {
	query, args, err := s.costFilter(ctx, f)
	if err != nil {
		return CostSummary{}, err
	}
	out := CostSummary{Totals: []CostGroup{}, ByModel: []CostGroup{}, ByScenario: []CostGroup{}}
	err = s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw("SELECT COUNT(*) AS total,COALESCE(SUM(cost_status='calculated'),0) AS calculated,COALESCE(SUM(cost_status='unpriced'),0) AS unpriced,COALESCE(SUM(cost_status='pending'),0) AS pending FROM ("+query+")", args...).Scan(&out).Error; err != nil {
			return err
		}
		for _, group := range []struct {
			columns string
			target  *[]CostGroup
		}{{"", &out.Totals}, {",model,request_type", &out.ByModel}, {",scenario", &out.ByScenario}} {
			*group.target = []CostGroup{}
			sql := "SELECT cost_currency AS currency" + group.columns + ",COUNT(*) AS count,SUM(cost_nanos) AS nanos FROM (" + query + ") WHERE cost_status='calculated' GROUP BY cost_currency" + group.columns + " ORDER BY cost_currency" + group.columns
			if err := tx.Raw(sql, args...).Scan(group.target).Error; err != nil {
				return err
			}
			for i := range *group.target {
				item := &(*group.target)[i]
				item.Amount = pricing.Amount(item.Nanos)
			}
		}
		return nil
	})
	return out, err
}

type frozenEstimate struct {
	Snapshot pricing.Snapshot `json:"snapshot"`
	Usage    pricing.Usage    `json:"usage"`
	Estimate pricing.Estimate `json:"estimate"`
}
type backfillRecord struct {
	ID        int64
	UUID      string
	ProjectID int64
	CreatedAt time.Time
}

func (backfillRecord) TableName() string { return "llm_cost_backfills" }

type backfillItem struct {
	ID           int64
	BackfillID   int64
	LLMLogID     int64  `gorm:"column:llm_log_id"`
	EstimateJSON string `gorm:"column:estimate_json"`
	Applied      bool
}

func (backfillItem) TableName() string { return "llm_cost_backfill_items" }

type Backfill struct {
	UUID       string      `json:"uuid"`
	CreatedAt  time.Time   `json:"created_at"`
	Total      int64       `json:"total"`
	Calculated int64       `json:"calculated"`
	Skipped    int64       `json:"skipped"`
	Processed  int64       `json:"processed"`
	HasMore    bool        `json:"has_more"`
	Totals     []CostGroup `json:"totals" gorm:"-"`
}
type legacyCostRow struct {
	ID                                                           int64
	UUID, ProviderUUID, ProviderType, Model, RequestType, Status string
	InputTokens, OutputTokens                                    int64
	CachedInputTokens                                            *int64
	RequestPayload, Response, BillingUsage, PriceSnapshot        sql.NullString
}

func historicalUsage(row legacyCostRow) pricing.Usage {
	var u pricing.Usage
	if row.BillingUsage.Valid {
		_ = json.Unmarshal([]byte(row.BillingUsage.String), &u)
		return u
	}
	if row.RequestType == "text" {
		// Old integer defaults did not distinguish zero from missing usage. Only
		// positive legacy counters (and the nullable cache counter) are evidence.
		if row.InputTokens > 0 {
			u.InputTokens = pricing.Int(row.InputTokens)
		}
		if row.OutputTokens > 0 {
			u.OutputTokens = pricing.Int(row.OutputTokens)
		}
		u.CachedInputTokens = row.CachedInputTokens
	} else {
		var request struct {
			Model, Size, Quality string
			Images               []json.RawMessage
		}
		var response struct {
			ByteSize int64 `json:"byte_size"`
		}
		if row.RequestPayload.Valid && json.Unmarshal([]byte(row.RequestPayload.String), &request) == nil && request.Model != "" {
			u.Size = request.Size
			u.Quality = request.Quality
			u.InputImages = pricing.Int(int64(len(request.Images)))
		}
		if row.Response.Valid && json.Unmarshal([]byte(row.Response.String), &response) == nil && response.ByteSize > 0 {
			u.OutputImages = pricing.Int(1)
		}
	}
	return u
}
func (s *Service) PreviewBackfill(ctx context.Context, f Filter, prices []pricing.Price) (Backfill, error) {
	if len(prices) == 0 || len(prices) > 100 {
		return Backfill{}, pricing.ErrInvalid
	}
	for i, a := range prices {
		if pricing.Validate(a.Rule) != nil {
			return Backfill{}, pricing.ErrInvalid
		}
		for _, b := range prices[:i] {
			if a.ProviderUUID == b.ProviderUUID && a.ProviderType == b.ProviderType && a.Model == b.Model && a.RequestType == b.RequestType {
				return Backfill{}, fmt.Errorf("%w: select one version per provider/model/request type", pricing.ErrInvalid)
			}
		}
	}
	query, args, err := s.costFilter(ctx, f)
	if err != nil {
		return Backfill{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Backfill{}, err
	}
	plan := backfillRecord{UUID: id.String(), CreatedAt: time.Now().UTC()}
	err = s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&project.Project{}).Where("uuid=?", s.store.ProjectUUID()).Pluck("id", &plan.ProjectID).Error; err != nil {
			return err
		}
		if err := tx.Create(&plan).Error; err != nil {
			return err
		} // write lock + stable preview
		var after int64
		for {
			rows := []legacyCostRow{}
			sql := "SELECT raw.* FROM llm_logs raw JOIN (" + query + ") filtered ON raw.uuid=filtered.uuid WHERE raw.cost_status='unpriced' AND raw.status<>'pending' AND raw.id>? ORDER BY raw.id LIMIT 200"
			params := append(append([]any{}, args...), after)
			if err := tx.Raw(sql, params...).Scan(&rows).Error; err != nil {
				return err
			}
			if len(rows) == 0 {
				break
			}
			items := []backfillItem{}
			for _, row := range rows {
				after = row.ID
				var chosen *pricing.Price
				var old pricing.Snapshot
				_ = json.Unmarshal([]byte(row.PriceSnapshot.String), &old)
				for i := range prices {
					p := &prices[i]
					if p.ProviderType != row.ProviderType || p.Model != row.Model || p.RequestType != row.RequestType || (p.ProviderUUID != "" && p.ProviderUUID != row.ProviderUUID) {
						continue
					}
					if old.Context.Region != "" && old.Context.Region != p.Region {
						continue
					}
					if chosen != nil {
						return pricing.ErrInvalid
					}
					chosen = p
				}
				c := pricing.Context{ProviderUUID: row.ProviderUUID, ProviderType: row.ProviderType, Model: row.Model, RequestType: row.RequestType}
				if chosen != nil {
					c.Region = chosen.Region
				}
				u := historicalUsage(row)
				c.Size = u.Size
				c.Quality = u.Quality
				snapshot := pricing.Snapshot{Context: c, Price: chosen}
				estimate := pricing.Calculate(snapshot, u)
				items = append(items, backfillItem{BackfillID: plan.ID, LLMLogID: row.ID, EstimateJSON: pricing.JSON(frozenEstimate{snapshot, u, estimate})})
			}
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Backfill{}, err
	}
	return s.GetBackfill(ctx, plan.UUID)
}
func (s *Service) GetBackfill(ctx context.Context, id string) (Backfill, error) {
	if !pricing.ValidUUID(id) {
		return Backfill{}, pricing.ErrInvalid
	}
	var plan backfillRecord
	if err := s.store.DB().WithContext(ctx).Where("uuid=?", id).First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Backfill{}, ErrNotFound
		}
		return Backfill{}, err
	}
	out := Backfill{UUID: plan.UUID, CreatedAt: plan.CreatedAt, Totals: []CostGroup{}}
	type itemRow struct {
		EstimateJSON string `gorm:"column:estimate_json"`
		Applied      bool
	}
	rows, err := s.store.DB().WithContext(ctx).Model(&backfillItem{}).Select("estimate_json,applied").Where("backfill_id=?", plan.ID).Rows()
	if err != nil {
		return out, err
	}
	defer rows.Close()
	sums := map[string]*big.Int{}
	counts := map[string]int64{}
	for rows.Next() {
		var r itemRow
		if err := s.store.DB().ScanRows(rows, &r); err != nil {
			return out, err
		}
		var e frozenEstimate
		if err := json.Unmarshal([]byte(r.EstimateJSON), &e); err != nil {
			return out, err
		}
		out.Total++
		if r.Applied {
			out.Processed++
		}
		if e.Estimate.Status == "calculated" {
			out.Calculated++
			n, err := pricing.ParseAmount(*e.Estimate.Amount)
			if err != nil {
				return out, err
			}
			c := e.Estimate.Currency
			if sums[c] == nil {
				sums[c] = new(big.Int)
			}
			sums[c].Add(sums[c], big.NewInt(n))
			counts[c]++
		} else {
			out.Skipped++
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for c, n := range sums {
		if !n.IsInt64() {
			return out, fmt.Errorf("cost summary overflow")
		}
		out.Totals = append(out.Totals, CostGroup{Currency: c, Count: counts[c], Amount: pricing.Amount(n.Int64())})
	}
	sort.Slice(out.Totals, func(i, j int) bool { return out.Totals[i].Currency < out.Totals[j].Currency })
	out.HasMore = out.Processed < out.Total
	return out, nil
}
func (s *Service) ListBackfills(ctx context.Context) ([]Backfill, error) {
	rows := []backfillRecord{}
	if err := s.store.DB().WithContext(ctx).Order("id DESC").Limit(5).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []Backfill{}
	for _, r := range rows {
		v, e := s.GetBackfill(ctx, r.UUID)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) ApplyBackfill(ctx context.Context, id string, events EventPublisher) (Backfill, error) {
	if !pricing.ValidUUID(id) {
		return Backfill{}, pricing.ErrInvalid
	}
	changed := false
	err := s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec("UPDATE llm_cost_backfills SET created_at=created_at WHERE uuid=?", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		var plan backfillRecord
		if err := tx.Where("uuid=?", id).First(&plan).Error; err != nil {
			return err
		}
		items := []backfillItem{}
		if err := tx.Where("backfill_id=? AND applied=0", plan.ID).Order("id").Limit(200).Find(&items).Error; err != nil {
			return err
		}
		for _, item := range items {
			var e frozenEstimate
			if err := json.Unmarshal([]byte(item.EstimateJSON), &e); err != nil {
				return err
			}
			if e.Estimate.Status == "calculated" {
				n, err := pricing.ParseAmount(*e.Estimate.Amount)
				if err != nil {
					return err
				}
				r := tx.Exec("UPDATE llm_logs SET price_snapshot=?,billing_usage=?,cost_status='calculated',cost_reason='',cost_origin='backfill',cost_currency=?,cost_nanos=?,cost_breakdown=? WHERE id=? AND cost_status='unpriced' AND status<>'pending'", pricing.JSON(e.Snapshot), pricing.JSON(e.Usage), e.Estimate.Currency, n, pricing.JSON(e.Estimate.Lines), item.LLMLogID)
				if r.Error != nil {
					return r.Error
				}
				changed = changed || r.RowsAffected > 0
			}
			if err := tx.Model(&backfillItem{}).Where("id=? AND applied=0", item.ID).Update("applied", true).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Backfill{}, err
	}
	if changed {
		emitChanged(events, s.store.ProjectUUID(), "", "calculated")
	}
	return s.GetBackfill(ctx, id)
}
