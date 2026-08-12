package llmlog

import (
	"encoding/json"
	"testing"
)

func TestSnapshotCharacterCountCountsUnicodeStringValuesOnly(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want int
	}{
		{name: "unicode runes", raw: json.RawMessage(`{"prompt":"月光🌙","max_tokens":1024,"stream":true}`), want: 3},
		{name: "nested text", raw: json.RawMessage(`{"messages":[{"role":"user","content":"你好"},{"role":"assistant","content":"ok"}]}`), want: 17},
		{name: "invalid JSON", raw: json.RawMessage(`{"prompt":`), want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := snapshotCharacterCount(test.raw); got != test.want {
				t.Fatalf("snapshotCharacterCount(%s) = %d, want %d", test.raw, got, test.want)
			}
		})
	}
}
