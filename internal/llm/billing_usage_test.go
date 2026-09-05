package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBillingUsageDistinguishesAbsentAndZero(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		known bool
		cache bool
	}{{`{}`, false, false}, {`{"prompt_tokens":0,"completion_tokens":0,"prompt_tokens_details":{"cached_tokens":0}}`, true, true}, {`{"prompt_tokens":8,"completion_tokens":2,"prompt_tokens_details":{}}`, true, false}} {
		var wire wireUsage
		if err := json.Unmarshal([]byte(tc.raw), &wire); err != nil {
			t.Fatal(err)
		}
		u := wire.normalized().BillingUsage()
		if (u.InputTokens != nil) != tc.known || (u.OutputTokens != nil) != tc.known || (u.CachedInputTokens != nil) != tc.cache {
			t.Fatalf("%s: %+v", tc.raw, u)
		}
	}
}
func TestInterruptedStreamPreservesReceivedUsage(t *testing.T) {
	raw := "data: {\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"prompt_tokens_details\":{\"cached_tokens\":4}}}\n" + "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n"
	result, err := readStream(context.Background(), strings.NewReader(raw), func(string) error { return context.Canceled })
	if !errors.Is(err, context.Canceled) || result.BillingUsage().InputTokens == nil || *result.BillingUsage().InputTokens != 10 {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestInvalidJSONContentPreservesReceivedUsage(t *testing.T) {
	result, err := readJSON(strings.NewReader(`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":0,"prompt_tokens_details":{"cached_tokens":0}}}`), nil)
	if err == nil || result.BillingUsage().OutputTokens == nil || *result.BillingUsage().OutputTokens != 0 {
		t.Fatalf("%+v %v", result, err)
	}
}
