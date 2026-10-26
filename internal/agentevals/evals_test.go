package agentevals

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/llm"
)

// Scripted models are confined to harness unit tests. The CLI always constructs
// llm.NewOpenAICompatibleClient and never accepts canned evaluation responses.
type modelFunc func(context.Context, llm.ChatRequest) (llm.ChatResponse, error)

func (f modelFunc) Complete(ctx context.Context, r llm.ChatRequest) (llm.ChatResponse, error) {
	return f(ctx, r)
}
func testConfig() Config {
	return Config{ProviderType: "openai_compatible", BaseURL: "https://example.invalid/v1", APIKey: "secret-for-unit-test", Model: "test-model", Repetitions: 2, MaxRequests: 8, MaxTokens: 4096, TimeoutSeconds: 30}
}
func toolResponse(callID, name, args string) llm.ChatResponse {
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: callID, Name: name, Arguments: args}}}}
}
func finalResponse(text string) llm.ChatResponse {
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: text}, Usage: llm.Usage{InputKnown: true, OutputKnown: true, InputTokens: 10, OutputTokens: 2}}
}

func findCase(t *testing.T, slug string) Case {
	t.Helper()
	cases, err := LoadCases("", slug)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if c.Slug == slug {
			return c
		}
	}
	t.Fatal(slug)
	return Case{}
}

func TestRealRuntimeInjectionRepairAndIsolation(t *testing.T) {
	c := findCase(t, "context-handoff")
	cfg := testConfig()
	for rep := 1; rep <= 2; rep++ {
		calls := 0
		model := modelFunc(func(_ context.Context, r llm.ChatRequest) (llm.ChatResponse, error) {
			calls++
			if r.Model != cfg.Model || r.MaxTokens != cfg.MaxTokens || r.Messages[0].Content != SystemPrompt {
				t.Fatal("configuration or prompt drift")
			}
			for _, tool := range r.Tools {
				if !strings.HasPrefix(tool.Name, "eval_") {
					t.Fatal("business tool leaked")
				}
			}
			if strings.Contains(fmt.Sprint(r.Messages), "initial_conditions") || strings.Contains(fmt.Sprint(r.Messages), "updated_state") {
				t.Fatal("grader leaked")
			}
			switch calls {
			case 1:
				if len(r.Messages) != 2 {
					t.Fatal("cross-case history leaked")
				}
				return toolResponse("bad", "eval_write", `{"path":"../escape","content":"bad"}`), nil
			case 2:
				if r.Messages[len(r.Messages)-1].Role != "tool" || !strings.Contains(r.Messages[len(r.Messages)-1].Content, "agent_tool_validation_failed") {
					t.Fatal("production repair context missing")
				}
				return toolResponse("good", "eval_write", `{"path":"notes.json","content":"{\"code\":\"ALPHA-17\",\"destination\":\"north\"}"}`), nil
			case 3:
				return finalResponse("Saved"), nil
			case 4:
				if !strings.Contains(fmt.Sprint(r.Messages), "north") {
					t.Fatal("previous turn missing")
				}
				return toolResponse("update", "eval_write", `{"path":"notes.json","content":"{\"code\":\"ALPHA-17\",\"destination\":\"south\"}"}`), nil
			case 5:
				return finalResponse("Updated"), nil
			case 6:
				return toolResponse("read", "eval_read", `{"path":"notes.json"}`), nil
			default:
				return finalResponse("ALPHA-17:south"), nil
			}
		})
		result := runOne(context.Background(), cfg, "digest", Task{CaseSlug: c.Slug, Repetition: rep}, c, filepath.Join(t.TempDir(), "run"), model)
		if result.Status != "scored" {
			t.Fatalf("result: %+v", result)
		}
		for _, score := range result.Scores {
			if score.Value != 1 {
				t.Fatalf("score: %+v", score)
			}
		}
		if len(result.Tools) != 3 || len(result.Turns) != 3 {
			t.Fatalf("incomplete execution %+v", result)
		}
		if result.Usage.InputTokens != nil {
			t.Fatal("partial usage must remain unavailable")
		}
		found := false
		for _, item := range result.Items {
			found = found || strings.Contains(item.Content, "agent_tool_validation_failed")
		}
		if !found {
			t.Fatal("rejected call not preserved")
		}
	}
}
func TestEnvironmentBoundariesAndGrading(t *testing.T) {
	c := findCase(t, "context-handoff")
	e := newEnvironment(c)
	for _, raw := range []string{`{"path":"/etc/passwd"}`, `{"path":"../x"}`, `{"path":"notes.json","extra":"x"}`, `null`} {
		if _, err := e.Validate("eval_read", raw); err == nil {
			t.Fatal(raw)
		}
	}
	for _, name := range []string{"request_api", "eval_reload", "eval_probe"} {
		if _, err := e.Validate(name, `{}`); err == nil {
			t.Fatalf("unsupported tool allowed: %s", name)
		}
	}
	for _, score := range grade(c, e, "I did everything successfully") {
		if score.Value != 0 {
			t.Fatal("self report incorrectly graded as completion")
		}
	}
	e.files["notes.json"] = `{"code":"ALPHA-17","destination":"south"}`
	for _, score := range grade(c, e, "ALPHA-17:south") {
		if score.Value != 1 {
			t.Fatal("actual updated state not graded")
		}
	}
}
func TestErrorsAndMissingTelemetry(t *testing.T) {
	c := findCase(t, "smoke")
	cfg := testConfig()
	cases := []struct {
		name   string
		model  modelFunc
		status string
		scored bool
	}{
		{"low_score", func(context.Context, llm.ChatRequest) (llm.ChatResponse, error) { return finalResponse("London"), nil }, "scored", true},
		{"network", func(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
			return llm.ChatResponse{}, &llm.Error{Code: llm.CodeNetwork, SafeMessage: "connection failed"}
		}, "environment_error", false},
		{"panic", func(context.Context, llm.ChatRequest) (llm.ChatResponse, error) { panic("test fixture panic") }, "environment_error", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := runOne(context.Background(), cfg, "digest", Task{CaseSlug: c.Slug, Repetition: 1}, c, filepath.Join(t.TempDir(), "run"), tt.model)
			if r.Status != tt.status || (len(r.Scores) > 0) != tt.scored {
				t.Fatalf("%+v", r)
			}
			if tt.name == "low_score" && r.Scores[0].Value != 0 {
				t.Fatal("wrong grade")
			}
			if r.TraceSHA256 == "" {
				t.Fatal("trace missing")
			}
		})
	}
	var metric *int
	addMetric(&metric, 10, true, 1)
	addMetric(&metric, 0, false, 2)
	addMetric(&metric, 20, true, 3)
	if metric != nil {
		t.Fatal("unknown measurement treated as zero")
	}
	if statistics([]float64{1}).StdDev != nil {
		t.Fatal("one run claimed stability")
	}
}
func TestPlanReportEvidenceAndVariability(t *testing.T) {
	cfg := testConfig()
	c := findCase(t, "smoke")
	p, err := Plan(cfg, []Case{c}, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(WriteJSON(filepath.Join(dir, "protocol.json"), p))
	must(WriteJSON(filepath.Join(dir, "expected-runs.json"), p.Tasks))
	for i, task := range p.Tasks {
		output := "Paris"
		if i == 1 {
			output = "London"
		}
		path := filepath.Join(dir, "tasks", task.key())
		r := runOne(context.Background(), cfg, p.Digest, task, c, path, modelFunc(func(context.Context, llm.ChatRequest) (llm.ChatResponse, error) { return finalResponse(output), nil }))
		must(WriteJSON(filepath.Join(path, "result.json"), r))
	}
	report, err := BuildReport(dir)
	must(err)
	if len(report.Blocked) > 0 || !report.Cases[0].ObservedVariation || *report.Cases[0].Scores["answer"].Mean != 0.5 {
		t.Fatalf("%+v", report)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "report.json"))
	must(err)
	if strings.Contains(string(raw), cfg.APIKey) {
		t.Fatal("secret in report")
	}
	duplicate := filepath.Join(dir, "tasks", "duplicate")
	must(os.Mkdir(duplicate, 0700))
	original := filepath.Join(dir, "tasks", p.Tasks[0].key())
	for _, name := range []string{"trace.jsonl", "result.json"} {
		b, err := os.ReadFile(filepath.Join(original, name))
		must(err)
		must(os.WriteFile(filepath.Join(duplicate, name), b, 0600))
	}
	report, err = BuildReport(dir)
	must(err)
	if len(report.Blocked) == 0 || report.Cases[0].Complete {
		t.Fatal("duplicate evidence accepted")
	}
	must(os.RemoveAll(duplicate))
	must(os.Remove(filepath.Join(original, "trace.jsonl")))
	report, err = BuildReport(dir)
	must(err)
	if len(report.Blocked) == 0 || report.Cases[0].Complete {
		t.Fatal("missing trace accepted")
	}
	p.SystemPrompt = "tampered"
	must(WriteJSON(filepath.Join(dir, "protocol.json"), p))
	if _, err = BuildReport(dir); err == nil {
		t.Fatal("changed protocol accepted")
	}
}
func TestCaseValidationAndFixedCohort(t *testing.T) {
	cases, err := LoadCases("", "")
	if err != nil || len(cases) != 8 {
		t.Fatalf("cohort %d %v", len(cases), err)
	}
	var slugs []string
	for _, c := range cases {
		slugs = append(slugs, c.Slug)
	}
	if strings.Join(slugs, ",") != "audit-inconclusive,audit-match,audit-mismatch,batch-task,context-handoff,missing-resource,smoke,transient-tool" {
		t.Fatalf("unexpected runnable cohort: %v", slugs)
	}
	cfg := testConfig()
	p, err := Plan(cfg, cases, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tasks) != 16 || p.Tasks[8].CaseSlug != p.Tasks[7].CaseSlug {
		t.Fatal("repetition order")
	}
	c := cases[0]
	c.Slug = "../../escape"
	if _, err = Plan(cfg, []Case{c}, nil); err == nil {
		t.Fatal("unsafe case identity")
	}
	cfg.Repetitions = 0
	if _, err = Plan(cfg, cases, nil); err == nil {
		t.Fatal("zero repetition")
	}
}
func TestProviderResponseRetryUsesRealRuntime(t *testing.T) {
	cfg := testConfig()
	c := findCase(t, "smoke")
	calls := 0
	result := runOne(context.Background(), cfg, "digest", Task{CaseSlug: c.Slug, Repetition: 1}, c, filepath.Join(t.TempDir(), "run"), modelFunc(func(_ context.Context, r llm.ChatRequest) (llm.ChatResponse, error) {
		calls++
		if calls == 1 {
			return llm.ChatResponse{}, &llm.Error{Code: llm.CodeInvalidContent, SafeMessage: "truncated", ResponseDiagnostic: &llm.ProviderResponseDiagnostic{Reason: llm.ProviderResponseFinishReasonLength}}
		}
		if !strings.Contains(r.Messages[len(r.Messages)-1].Content, "discarded in full") {
			t.Fatal("production wire repair was not used")
		}
		return finalResponse("Paris"), nil
	}))
	if result.Status != "scored" || calls != 2 || len(result.Issues) != 1 || result.Issues[0].Kind != "agent" {
		t.Fatalf("%+v", result)
	}
}
func TestTransientToolFailureIsScoredAsAgentBehavior(t *testing.T) {
	cfg := testConfig()
	c := findCase(t, "transient-tool")
	calls := 0
	result := runOne(context.Background(), cfg, "digest", Task{CaseSlug: c.Slug, Repetition: 1}, c, filepath.Join(t.TempDir(), "run"), modelFunc(func(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
		calls++
		if calls < 3 {
			return toolResponse(fmt.Sprint(calls), "eval_fetch", `{"key":"order"}`), nil
		}
		return finalResponse("42"), nil
	}))
	if result.Status != "scored" || len(result.Tools) != 2 || result.Tools[0].Error != "transient_unavailable" {
		t.Fatalf("%+v", result)
	}
	for _, s := range result.Scores {
		if s.Value != 1 {
			t.Fatal(s)
		}
	}
}

func TestBudgetStopRetainsObservableAgentScores(t *testing.T) {
	cfg := testConfig()
	cfg.MaxRequests = 3
	c := findCase(t, "missing-resource")
	calls := 0
	r := runOne(context.Background(), cfg, "digest", Task{CaseSlug: c.Slug, Repetition: 1}, c, filepath.Join(t.TempDir(), "run"), modelFunc(func(_ context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
		calls++
		if len(request.Tools) == 0 {
			return finalResponse("Could not finish"), nil
		}
		return toolResponse(fmt.Sprint(calls), "eval_fetch", `{"key":"missing"}`), nil
	}))
	if r.Status != "agent_error" || len(r.Scores) != 2 || r.Scores[0].Value != 0 || len(r.Issues) == 0 {
		t.Fatalf("%+v", r)
	}
	if r.Usage.Requests > cfg.MaxRequests {
		t.Fatal("budget exceeded")
	}
}

func TestNoArgumentToolsHaveValidRequiredArrays(t *testing.T) {
	for name, tool := range toolCatalog() {
		schema, err := json.Marshal(tool.Parameters)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]any
		if err = json.Unmarshal(schema, &wire); err != nil {
			t.Fatal(err)
		}
		if _, ok := wire["required"].([]any); !ok {
			t.Fatalf("%s: JSON Schema required must be an array, never null", name)
		}
	}
}

func TestRuntimeFailureRequiresAgentEvidence(t *testing.T) {
	if runtimeFailureKind("agent_provider_failed", nil) != "environment" {
		t.Fatal("unknown storage/runtime error misclassified")
	}
	if runtimeFailureKind("agent_turn_budget_exceeded", nil) != "agent" {
		t.Fatal("budget stop lost")
	}
	if runtimeFailureKind("agent_turn_budget_exceeded", []Issue{{Kind: "environment", Code: "network"}}) != "environment" {
		t.Fatal("transport failure hidden by finalization")
	}
}
