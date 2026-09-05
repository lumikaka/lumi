package pricing

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"math"
	"path/filepath"
	"testing"
)

func textRule() Rule {
	return Rule{ProviderUUID: uuid.Must(uuid.NewV7()).String(), ProviderType: "cloudflare_ai_gateway", Model: "test/model", RequestType: "text", Mode: "tokens", Currency: "USD", Rates: []Rate{{Metric: "input_tokens", Price: "2"}, {Metric: "cached_input_tokens", Price: "0.5"}, {Metric: "output_tokens", Price: "8"}}}
}
func snapshot(r Rule) Snapshot { return Snapshot{Price: &Price{Rule: r}} }
func TestCalculateExactMoneyAndMissingUsage(t *testing.T) {
	r := textRule()
	usage := Usage{InputTokens: Int(1000000), CachedInputTokens: Int(250000), OutputTokens: Int(100000)}
	e := Calculate(snapshot(r), usage)
	if e.Status != "calculated" || *e.Amount != "2.425000000" || *e.Nanos != 2425000000 {
		t.Fatalf("estimate=%+v", e)
	}
	tests := []struct {
		name   string
		u      Usage
		reason string
	}{{"missing cache", Usage{InputTokens: Int(100), OutputTokens: Int(10)}, "missing_usage"}, {"cache exceeds input", Usage{InputTokens: Int(1), CachedInputTokens: Int(2), OutputTokens: Int(1)}, "invalid_usage"}, {"missing output", Usage{InputTokens: Int(1), CachedInputTokens: Int(0)}, "missing_usage"}, {"negative", Usage{InputTokens: Int(-1), CachedInputTokens: Int(0), OutputTokens: Int(0)}, "invalid_usage"}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := Calculate(snapshot(r), tc.u)
			if e.Reason != tc.reason || e.Amount != nil {
				t.Fatalf("%+v", e)
			}
		})
	}
	e = Calculate(snapshot(r), Usage{InputTokens: Int(0), CachedInputTokens: Int(0), OutputTokens: Int(0)})
	if e.Amount == nil || *e.Amount != "0.000000000" {
		t.Fatal(e)
	}
	if e := Calculate(Snapshot{}, usage); e.Reason != "missing_price" || e.Nanos != nil {
		t.Fatal(e)
	}
}
func TestTierBoundariesAndRounding(t *testing.T) {
	r := builtinRules()[0]
	for _, tc := range []struct {
		tokens int64
		rate   string
	}{{0, "2"}, {256000, "2"}, {256001, "6"}, {1000000, "6"}} {
		e := Calculate(snapshot(r), Usage{InputTokens: Int(tc.tokens), CachedInputTokens: Int(0), OutputTokens: Int(0)})
		if e.Status != "calculated" || e.Lines[0].Price != tc.rate {
			t.Fatalf("%d: %+v", tc.tokens, e)
		}
	}
	if e := Calculate(snapshot(r), Usage{InputTokens: Int(1000001), CachedInputTokens: Int(0), OutputTokens: Int(0)}); e.Reason != "missing_rate" {
		t.Fatal(e)
	}
	r = textRule()
	r.Rates[0].Price = "0.0005"
	e := Calculate(snapshot(r), Usage{InputTokens: Int(1), CachedInputTokens: Int(0), OutputTokens: Int(0)})
	if e.Nanos == nil || *e.Nanos != 1 {
		t.Fatalf("half nano rounding: %+v", e)
	}
	r.Rates[0].Price = "9000000000"
	if e := Calculate(snapshot(r), Usage{InputTokens: Int(math.MaxInt64), CachedInputTokens: Int(0), OutputTokens: Int(0)}); e.Reason != "amount_overflow" {
		t.Fatal(e)
	}
	for _, n := range []int64{0, 1, 999999999, 9007199254740993, math.MaxInt64} {
		v, err := ParseAmount(Amount(n))
		if err != nil || v != n {
			t.Fatalf("round trip %d: %d %v", n, v, err)
		}
	}
}
func TestImagesAndDisjointComponents(t *testing.T) {
	r := builtinRules()[2]
	e := Calculate(snapshot(r), Usage{InputImages: Int(2), OutputImages: Int(1), Resolution: "2k"})
	if e.Amount == nil || *e.Amount != "0.540000000" {
		t.Fatal(e)
	}
	if e := Calculate(snapshot(r), Usage{InputImages: Int(2), OutputImages: Int(1)}); e.Reason != "missing_rate" {
		t.Fatal(e)
	}
	r = textRule()
	r.Mode = "responses"
	r.RequestType = "image"
	r.ImageModel = "image/tool"
	r.Rates = append(r.Rates, Rate{Metric: "tool_input_tokens", Price: "1"}, Rate{Metric: "cached_tool_input_tokens", Price: "0.1"}, Rate{Metric: "image_input_tokens", Price: "10"}, Rate{Metric: "cached_image_input_tokens", Price: "2"}, Rate{Metric: "image_output_tokens", Price: "20"})
	u := Usage{InputTokens: Int(100), CachedInputTokens: Int(20), OutputTokens: Int(10), ImageInputTokens: Int(1000), CachedImageInputTokens: Int(200), ImageOutputTokens: Int(1000), ImageModel: "image/tool"}
	if e := Calculate(snapshot(r), u); e.Reason != "incomplete_components" {
		t.Fatal(e)
	}
	u.Disjoint = true
	u.ToolInputTokens = Int(0)
	u.CachedToolInputTokens = Int(0)
	e = Calculate(snapshot(r), u)
	if e.Amount == nil || *e.Amount != "0.028650000" {
		t.Fatal(e)
	}
}
func TestValidationRejectsAmbiguousOrInvalidRates(t *testing.T) {
	for _, value := range []string{"-1", "1e3", "NaN", "0.0000000001", "9223372037"} {
		r := textRule()
		r.Rates[0].Price = value
		if Validate(r) == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	r := textRule()
	r.Rates = append(r.Rates, r.Rates[0])
	if Validate(r) == nil {
		t.Fatal("overlapping rates accepted")
	}
	for _, r := range builtinRules() {
		if e := Validate(r); e != nil {
			t.Fatalf("builtin %s: %v", r.Model, e)
		}
	}
}
func TestCatalogVersionsFreezeAndRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	svc := NewService(app.DB())
	if err := svc.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	items, _ := svc.List(ctx)
	if err := svc.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	again, _ := svc.List(ctx)
	if len(items) != len(again) {
		t.Fatal("seed duplicated")
	}
	r := builtinRules()[0]
	r.ProviderUUID = uuid.Must(uuid.NewV7()).String()
	c := Context{ProviderUUID: r.ProviderUUID, ProviderType: r.ProviderType, Model: r.Model, Region: r.Region, RequestType: r.RequestType}
	built := svc.Freeze(ctx, c)
	if built.Price == nil || built.Price.Source != "builtin" {
		t.Fatal(built)
	}
	a, err := svc.Create(ctx, r, "")
	if err != nil {
		t.Fatal(err)
	}
	frozen := svc.Freeze(ctx, c)
	r.Rates[0].Price = "9"
	b, err := svc.Create(ctx, r, a.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Price.UUID != a.UUID || svc.Freeze(ctx, c).Price.UUID != b.UUID {
		t.Fatal("price snapshot changed")
	}
	if _, err := svc.Create(ctx, r, a.UUID); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	if err := svc.Delete(ctx, b.UUID); err != nil {
		t.Fatal(err)
	}
	if svc.Freeze(ctx, c).Price.Source != "builtin" {
		t.Fatal("override did not restore builtin")
	}
	c.Model += "-unknown"
	if svc.Freeze(ctx, c).Price != nil {
		t.Fatal("fuzzy model match")
	}
}
