package pricing

import "testing"

func TestRelayAcceptsCustomPricingWithoutBuiltinPrice(t *testing.T) {
	r := textRule()
	r.ProviderType = "openai_relay"
	r.Model = "gpt-6"
	if err := Validate(r); err != nil {
		t.Fatal(err)
	}
	for _, rule := range builtinRules() {
		if rule.ProviderType == "openai_relay" {
			t.Fatal("relay must not inherit upstream pricing")
		}
	}
	e := Calculate(snapshot(r), Usage{InputTokens: Int(1000000), CachedInputTokens: Int(0), OutputTokens: Int(0)})
	if e.Status != "calculated" || e.Amount == nil || *e.Amount != "2.000000000" {
		t.Fatalf("estimate=%+v", e)
	}
}
