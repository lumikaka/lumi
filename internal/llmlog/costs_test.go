package llmlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/pricing"
	"lumi/internal/project"
	"path/filepath"
	"testing"
	"time"
)

func withCostFixture(t *testing.T, fn func(*project.Store, *pricing.Service, StartInput, *appstore.Store)) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(app)
	t.Cleanup(func() { _ = manager.Close(); _ = app.Close() })
	header, err := manager.Create(ctx, "Costs", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	err = manager.WithCurrentStore(ctx, header.UUID, func(store *project.Store) error {
		var id, task int64
		store.DB().Model(&project.Project{}).Where("uuid=?", header.UUID).Pluck("id", &id)
		providerID := uuid.Must(uuid.NewV7()).String()
		taskUUID := uuid.Must(uuid.NewV7()).String()
		now := time.Now().UTC()
		if err := store.DB().Exec(`INSERT INTO task_runs(uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,retryable,provider_uuid,model,progress,attempt,max_attempts,error_code,error_message,created_at,updated_at) VALUES(?,?,'story_chapter_generation',?,1,'{}','running','cost-test',0,?,'meter/model',0,1,1,'','',?,?)`, taskUUID, id, uuid.Must(uuid.NewV7()).String(), providerID, now, now).Error; err != nil {
			return err
		}
		store.DB().Table("task_runs").Where("uuid=?", taskUUID).Pluck("id", &task)
		start := StartInput{ProjectID: id, TaskRunID: task, SourceType: SourceStoryGeneration, Scenario: "story_chapter_generation", RequestType: RequestText, Attempt: 1, ProviderUUID: providerID, ProviderType: "cloudflare_ai_gateway", Model: "meter/model", RequestPayload: json.RawMessage(`{"model":"meter/model","prompt":"hello"}`)}
		fn(store, pricing.NewService(app.DB()), start, app)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func costRule(start StartInput) pricing.Rule {
	return pricing.Rule{ProviderUUID: start.ProviderUUID, ProviderType: start.ProviderType, Model: start.Model, RequestType: "text", Mode: "tokens", Currency: "USD", Rates: []pricing.Rate{{Metric: "input_tokens", Price: "2"}, {Metric: "cached_input_tokens", Price: "0.5"}, {Metric: "output_tokens", Price: "8"}}}
}
func costUsage() pricing.Usage {
	return pricing.Usage{InputTokens: pricing.Int(1000000), CachedInputTokens: pricing.Int(250000), OutputTokens: pricing.Int(100000)}
}
func TestCostRecordingFreezesPriceAndRollsBackAtomically(t *testing.T) {
	withCostFixture(t, func(store *project.Store, prices *pricing.Service, start StartInput, app *appstore.Store) {
		ctx := context.Background()
		r := costRule(start)
		v1, err := prices.Create(ctx, r, "")
		if err != nil {
			t.Fatal(err)
		}
		start.Prices = prices
		events := &recorderEventPublisher{}
		h, err := Begin(ctx, store, events, start)
		if err != nil {
			t.Fatal(err)
		}
		r.Rates[0].Price = "10"
		if _, err := prices.Create(ctx, r, v1.UUID); err != nil {
			t.Fatal(err)
		}
		finish := FinishInput{BillingUsage: costUsage(), Err: context.Canceled}
		if err := Finish(ctx, store, events, h, finish); err != nil {
			t.Fatal(err)
		}
		detail, err := NewService(store).Get(ctx, h.UUID)
		if err != nil {
			t.Fatal(err)
		}
		if detail.CostAmount == nil || *detail.CostAmount != "2.425000000" || detail.Status != "cancelled" {
			t.Fatalf("%+v", detail)
		}
		if err := Finish(ctx, store, events, h, finish); err == nil {
			t.Fatal("duplicate finish succeeded")
		}
		h2, err := Begin(ctx, store, events, start)
		if err != nil {
			t.Fatal(err)
		}
		rollback := errors.New("rollback")
		if err := FinishAtomic(ctx, store, events, h2, finish, func(context.Context, *sql.Tx) error { return rollback }); !errors.Is(err, rollback) {
			t.Fatal(err)
		}
		pending, err := NewService(store).Get(ctx, h2.UUID)
		if err != nil {
			t.Fatal(err)
		}
		if pending.CostStatus != "pending" || pending.CostAmount != nil {
			t.Fatal(pending)
		}
		summary, err := NewService(store).CostSummary(ctx, Filter{})
		if err != nil {
			t.Fatal(err)
		}
		if summary.Total != 2 || summary.Calculated != 1 || summary.Pending != 1 || summary.Totals[0].Amount != "2.425000000" {
			t.Fatalf("%+v", summary)
		}
		if len(events.events) != 3 {
			t.Fatalf("uncommitted event: %d", len(events.events))
		}
		// Reading the project cost needs neither the source catalog nor provider keys.
		if err := app.Close(); err != nil {
			t.Fatal(err)
		}
		detail, err = NewService(store).Get(ctx, h.UUID)
		if err != nil || detail.CostAmount == nil || *detail.CostAmount != "2.425000000" {
			t.Fatalf("portable cost: %+v %v", detail, err)
		}
	})
}
func TestBackfillFreezesScopeAndPricesAndIsIdempotent(t *testing.T) {
	withCostFixture(t, func(store *project.Store, prices *pricing.Service, start StartInput, _ *appstore.Store) {
		ctx := context.Background()
		r := costRule(start)
		price, err := prices.Create(ctx, r, "")
		if err != nil {
			t.Fatal(err)
		}
		for range 201 {
			h, err := Begin(ctx, store, nil, start)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.DB().Exec("UPDATE llm_logs SET status='completed',cost_status='unpriced',input_tokens=1000000,cached_input_tokens=250000,output_tokens=100000,completed_at=? WHERE id=?", time.Now().UTC(), h.ID).Error; err != nil {
				t.Fatal(err)
			}
		}
		unknown, err := Begin(ctx, store, nil, start)
		if err != nil {
			t.Fatal(err)
		}
		if err := Finish(ctx, store, nil, unknown, FinishInput{Err: errors.New("no usage")}); err != nil {
			t.Fatal(err)
		}
		service := NewService(store)
		preview, err := service.PreviewBackfill(ctx, Filter{Model: start.Model}, []pricing.Price{price})
		if err != nil {
			t.Fatal(err)
		}
		if preview.Total != 202 || preview.Calculated != 201 || preview.Skipped != 1 || preview.Totals[0].Amount != "487.425000000" {
			t.Fatalf("%+v", preview)
		}
		r.Rates[0].Price = "100"
		if _, err := prices.Create(ctx, r, price.UUID); err != nil {
			t.Fatal(err)
		}
		later, err := Begin(ctx, store, nil, start)
		if err != nil {
			t.Fatal(err)
		}
		if err := Finish(ctx, store, nil, later, FinishInput{BillingUsage: costUsage(), Err: errors.New("failed after usage")}); err != nil {
			t.Fatal(err)
		}
		first, err := service.ApplyBackfill(ctx, preview.UUID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if first.Processed != 200 || !first.HasMore {
			t.Fatal(first)
		}
		done, err := service.ApplyBackfill(ctx, preview.UUID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if done.Processed != 202 || done.HasMore {
			t.Fatal(done)
		}
		if _, err := service.ApplyBackfill(ctx, preview.UUID, nil); err != nil {
			t.Fatal(err)
		}
		summary, err := service.CostSummary(ctx, Filter{Model: start.Model})
		if err != nil {
			t.Fatal(err)
		}
		if summary.Total != 203 || summary.Calculated != 201 || summary.Unpriced != 2 || summary.Totals[0].Amount != "487.425000000" {
			t.Fatalf("%+v", summary)
		}
		items, page, _, err := service.List(ctx, Filter{}, 1, 1)
		if err != nil || len(items) != 1 || page.Total != 203 {
			t.Fatalf("pagination: %+v %v", page, err)
		}
		raw, err := json.Marshal(summary)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]any
		_ = json.Unmarshal(raw, &wire)
		if _, ok := wire["Nanos"]; ok {
			t.Fatal("internal amount exposed")
		}
	})
}
func TestCostSummaryGroupsCurrenciesAndDates(t *testing.T) {
	withCostFixture(t, func(store *project.Store, prices *pricing.Service, start StartInput, _ *appstore.Store) {
		ctx := context.Background()
		start.Prices = prices
		for _, currency := range []string{"CNY", "USD"} {
			start.Model = "meter/" + currency
			r := costRule(start)
			r.Currency = currency
			if _, err := prices.Create(ctx, r, ""); err != nil {
				t.Fatal(err)
			}
			h, err := Begin(ctx, store, nil, start)
			if err != nil {
				t.Fatal(err)
			}
			if err := Finish(ctx, store, nil, h, FinishInput{BillingUsage: costUsage(), Err: errors.New("failed")}); err != nil {
				t.Fatal(err)
			}
		}
		s := NewService(store)
		summary, err := s.CostSummary(ctx, Filter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(summary.Totals) != 2 || len(summary.ByModel) != 2 {
			t.Fatal(summary)
		}
		empty, err := s.CostSummary(ctx, Filter{To: "2000-01-01T00:00:00Z"})
		if err != nil || empty.Total != 0 {
			t.Fatalf("%+v %v", empty, err)
		}
		if _, err := s.CostSummary(ctx, Filter{From: "invalid"}); !errors.Is(err, ErrInvalidFilter) {
			t.Fatal(err)
		}
	})
}

func TestRetriesEachKeepTheirOwnEstimateAndMissingPriceDoesNotFailCall(t *testing.T) {
	withCostFixture(t, func(store *project.Store, prices *pricing.Service, start StartInput, _ *appstore.Store) {
		ctx := context.Background()
		if _, err := prices.Create(ctx, costRule(start), ""); err != nil {
			t.Fatal(err)
		}
		start.Prices = prices
		for attempt := 1; attempt <= 3; attempt++ {
			start.Attempt = attempt
			h, err := Begin(ctx, store, nil, start)
			if err != nil {
				t.Fatal(err)
			}
			finish := FinishInput{BillingUsage: costUsage(), Response: json.RawMessage(`{"content":"done"}`)}
			if attempt < 3 {
				finish.Err = errors.New("retryable provider error after usage")
			}
			if err := Finish(ctx, store, nil, h, finish); err != nil {
				t.Fatal(err)
			}
		}
		s := NewService(store)
		summary, err := s.CostSummary(ctx, Filter{})
		if err != nil || summary.Total != 3 || summary.Calculated != 3 || len(summary.Totals) != 1 || summary.Totals[0].Amount != "7.275000000" {
			t.Fatalf("retry estimates: %+v %v", summary, err)
		}
		start.Model = "unconfigured/model"
		h, err := Begin(ctx, store, nil, start)
		if err != nil {
			t.Fatal(err)
		}
		if err := Finish(ctx, store, nil, h, FinishInput{BillingUsage: costUsage(), Response: json.RawMessage(`{"content":"done"}`)}); err != nil {
			t.Fatal(err)
		}
		detail, err := s.Get(ctx, h.UUID)
		if err != nil || detail.Status != "completed" || detail.CostStatus != "unpriced" || detail.CostAmount != nil || detail.CostReason != "missing_price" {
			t.Fatalf("unconfigured price changed result: %+v %v", detail, err)
		}
	})
}
