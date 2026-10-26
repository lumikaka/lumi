package agentevals

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// These synthetic bridge inputs test infrastructure, not model performance.
func TestSharedBridgeUsesActualEnvironmentAndGrading(t *testing.T) {
	requests := []map[string]any{
		{"op": "call", "name": "eval_read", "arguments": map[string]any{"path": "docs/names.md"}},
		{"op": "call", "name": "eval_read", "arguments": map[string]any{"path": "src/names.py"}},
		{"op": "call", "name": "eval_submit", "arguments": map[string]any{"verdict": "match", "explanation": "src/names.py format_name follows the documented claim"}},
		{"op": "grade", "final": "The docs match."},
	}
	var input, output bytes.Buffer
	for _, r := range requests {
		if err := json.NewEncoder(&input).Encode(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := ServeSharedCase("audit-match", &input, &output); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	for range 3 {
		var r struct {
			Content string `json:"content"`
			IsError bool   `json:"is_error"`
		}
		if err := decoder.Decode(&r); err != nil {
			t.Fatal(err)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(r.Content), &envelope); err != nil {
			t.Fatal(err)
		}
		if r.IsError || envelope["success"] != true {
			t.Fatalf("unexpected tool response: %+v", r)
		}
	}
	var result struct {
		Scores []Score     `json:"scores"`
		Tools  []ToolEvent `json:"tools"`
	}
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Scores) != 4 || len(result.Tools) != 3 {
		t.Fatalf("incomplete result: %+v", result)
	}
	for _, s := range result.Scores {
		if s.Value != 1 {
			t.Fatalf("unexpected score: %+v", s)
		}
	}
	// A second process/session cannot inherit the previous submission.
	output.Reset()
	if err := ServeSharedCase("audit-match", strings.NewReader("{\"op\":\"grade\",\"final\":\"success\"}\n"), &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for _, s := range result.Scores {
		if s.Value != 0 {
			t.Fatalf("leaked state: %+v", s)
		}
	}
}

func TestSharedBridgeRejectsUnsafeAndRetiredTools(t *testing.T) {
	for _, request := range []string{
		`{"op":"call","name":"eval_read","arguments":{"path":"../secret"}}`,
		`{"op":"call","name":"eval_reload","arguments":{}}`,
		`{"op":"call","name":"eval_read","arguments":{"path":null}}`,
	} {
		var output bytes.Buffer
		if err := ServeSharedCase("audit-match", strings.NewReader(request), &output); err != nil {
			t.Fatal(err)
		}
		var result struct {
			IsError bool `json:"is_error"`
		}
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatalf("unsafe bridge call accepted: %s", request)
		}
	}
	if err := ServeSharedCase("add-model", strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("retired case accepted")
	}
}
