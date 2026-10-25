package agentevals

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/llm"
)

func referenceCases(t *testing.T) []Case {
	t.Helper()
	cases, err := LoadCases("@pi-reference", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 6 {
		t.Fatal("expected three scenarios in two documentation variants")
	}
	return cases
}
func referenceSource(t *testing.T) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, p := range []string{"internal/agentevals/environment.go", "internal/agentevals/reference_environment.go", "internal/agentevals/grade.go"} {
		b, err := os.ReadFile(filepath.Join("../..", p))
		if err != nil {
			t.Fatal(err)
		}
		result[p] = digestBytes(b)
	}
	return result
}
func TestHistoricalSourcesAndCaseMappings(t *testing.T) {
	b, err := LoadHistoricalBaseline("../../evals/baselines/pi/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Observations) != 8 {
		t.Fatal("expected six case rates and two aggregate rates")
	}
	cases := referenceCases(t)
	found := 0
	for _, o := range b.Observations {
		if o.Config != nil || o.ExecutionRevision != nil {
			t.Fatal("unknown upstream facts must stay unavailable")
		}
		if o.Scope == "cohort" && len(o.Mappings) != 0 {
			t.Fatal("aggregate must not be mapped to a case")
		}
		for _, m := range o.Mappings {
			for _, c := range cases {
				if m.Slug == c.Slug {
					found++
					if m.CaseSHA256 != CaseDigest(c) {
						t.Fatalf("case mapping changed: %s got %s", c.Slug, CaseDigest(c))
					}
					if m.EnvironmentSHA256 != EnvironmentDigest(referenceSource(t)) {
						t.Fatalf("environment mapping changed: got %s", EnvironmentDigest(referenceSource(t)))
					}
				}
			}
		}
	}
	if found != 6 {
		t.Fatal("incomplete mapping")
	}
}

func TestReferencePlanFreezesEachPrompt(t *testing.T) {
	cases := referenceCases(t)
	p, err := Plan(testConfig(), cases, referenceSource(t))
	if err != nil {
		t.Fatal(err)
	}
	if p.PromptMode != "per_case" || p.SystemPrompt != "" || p.ReferenceCommit != "cfee633b1ba9497b0c12462da97a08b418b5ebf8" {
		t.Fatal("ambiguous reference protocol metadata")
	}
	for _, c := range p.Cases {
		if c.SystemPrompt == "" {
			t.Fatal("missing actual prompt")
		}
	}
	regression, err := LoadCases("", "")
	if err != nil || len(regression) != 14 {
		t.Fatal("reference variants changed the regression cohort")
	}
}
func TestReferenceRuntimeUsesActualGeneratedTemplate(t *testing.T) {
	cases := referenceCases(t)
	for _, c := range cases {
		if !strings.Contains(c.Slug, "prompt-template-project") {
			continue
		}
		calls := 0
		model := modelFunc(func(_ context.Context, r llm.ChatRequest) (llm.ChatResponse, error) {
			calls++
			if r.Messages[0].Content != c.SystemPrompt {
				t.Fatal("wrong system prompt")
			}
			if calls == 1 {
				return toolResponse("write", "write", `{"path":".pi/prompts/current-time.md","content":"---\ndescription: Current time\n---\nTell me the current time and timezone."}`), nil
			}
			if calls == 2 {
				return finalResponse("Created"), nil
			}
			if r.Messages[len(r.Messages)-1].Content != "Tell me the current time and timezone." {
				t.Fatal("template did not expand into a real user turn")
			}
			return finalResponse("The environment does not provide a clock."), nil
		})
		result := runOne(context.Background(), testConfig(), "test", Task{CaseSlug: c.Slug, Repetition: 1}, c, filepath.Join(t.TempDir(), "run"), model)
		if result.Status != "scored" || len(result.Scores) != 1 || result.Scores[0].Value != 1 {
			t.Fatalf("%s: %s %#v", c.Slug, result.Status, result.Issues)
		}
		e := newEnvironment(c)
		e.files["workspace/.pi/prompts/current-time.md"] = "WRONG CONTENT"
		e.reload()
		if e.expandReferencePrompt("/current-time") != "WRONG CONTENT" {
			t.Fatal("environment substituted the expected answer")
		}
		if grade(c, e, "")[0].Value != 0 {
			t.Fatal("incorrect template passed")
		}
		delete(e.files, "workspace/.pi/prompts/current-time.md")
		e.reload()
		e.expandReferencePrompt("/current-time")
		if grade(c, e, "")[0].Value != 0 {
			t.Fatal("missing template passed")
		}
	}
}
func TestReferenceVirtualPathsAndReloadSemantics(t *testing.T) {
	c := referenceCases(t)[0]
	e := newEnvironment(c)
	for _, p := range []string{"/etc/passwd", "../../etc/passwd", "~/../../etc/passwd", "x\\y"} {
		raw, _ := json.Marshal(map[string]any{"path": p})
		if _, err := e.Validate("read", string(raw)); err == nil {
			t.Fatalf("escaped roots: %s", p)
		}
	}
	if _, err := e.executeReferenceTool("write", map[string]any{"path": "/eval/runtime/docs/x.md", "content": "changed"}); err == nil {
		t.Fatal("wrote runtime docs")
	}
	e.files["workspace/.pi/prompts/current-time.md"] = "Tell me the current time and timezone."
	if e.expandReferencePrompt("/current-time") != "/current-time" {
		t.Fatal("template active before reload")
	}
	e.reload()
	e.expandReferencePrompt("/current-time")
	if e.referenceObservation("project").(map[string]any)["expandedToTimeRequest"] != true {
		t.Fatal("loaded template not expanded")
	}
	e.files["home/.pi/agent/prompts/current-time.md"] = "Tell me the current time and timezone."
	e.reload()
	e.expandReferencePrompt("/current-time")
	ob := e.referenceObservation("project").(map[string]any)
	if ob["template"] != nil || ob["otherScopeTemplateExists"] != true {
		t.Fatal("duplicate or wrong-scope template accepted")
	}
}
func TestReferenceModelLoadsConfigurationAndChecksPreservation(t *testing.T) {
	var c Case
	for _, candidate := range referenceCases(t) {
		if candidate.Slug == "pi-add-model-with-docs" {
			c = candidate
		}
	}
	e := newEnvironment(c)
	expected := c.Checks[0].Expected.(map[string]any)["result"].(map[string]any)["model"].(map[string]any)
	config := map[string]any{"providers": map[string]any{"openai": map[string]any{"models": []any{expected}}}}
	data, _ := json.Marshal(config)
	e.files["home/.pi/agent/models.json"] = string(data)
	if grade(c, e, "")[0].Value != 0 {
		t.Fatal("configuration active before reload")
	}
	e.reload()
	if grade(c, e, "")[0].Value != 1 {
		t.Fatal("valid model did not load")
	}
	delete(e.active["model_registry"].(map[string]map[string]any), "builtin-fixture")
	if grade(c, e, "")[0].Value != 0 {
		t.Fatal("preservation is not observable")
	}
	e.files["home/.pi/agent/models.json"] = "broken"
	e.reload()
	if grade(c, e, "")[0].Value != 0 {
		t.Fatal("invalid configuration passed")
	}
}
func TestHistoricalComparisonNeverInventsDifferences(t *testing.T) {
	b, err := LoadHistoricalBaseline("../../evals/baselines/pi/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := referenceCases(t)
	cfg := testConfig()
	cfg.Repetitions = 1
	p := Protocol{Config: cfg, Cases: cases, Source: referenceSource(t)}
	r := Report{}
	for _, c := range cases {
		r.Results = append(r.Results, Result{Task: Task{CaseSlug: c.Slug, Repetition: 1}, Status: "scored", Scores: []Score{{Name: "upstream_contract", Value: 1}}})
	}
	comparison := compareHistorical(p, r, b)
	approximate, unmatched := 0, 0
	for _, row := range comparison.Rows {
		if row.DeltaPercentagePoints != nil {
			t.Fatal("unverified PR summary produced a capability delta")
		}
		switch row.Status {
		case "approximate":
			approximate++
		case "no_local_match":
			unmatched++
		default:
			t.Fatal(row.Status)
		}
	}
	if approximate != 6 || unmatched != 2 {
		t.Fatal("lost case or aggregate evidence")
	}
	p.Cases[0].Steps[0].Content += " modified"
	comparison = compareHistorical(p, r, b)
	found := false
	for _, row := range comparison.Rows {
		if row.LumiSlug == p.Cases[0].Slug && row.Status == "contract_changed" {
			found = true
		}
	}
	if !found {
		t.Fatal("same slug concealed a changed case")
	}
	// A missing environment run is unavailable, not a zero score.
	r.Results[1].Status = "environment_error"
	comparison = compareHistorical(p, r, b)
	for _, row := range comparison.Rows {
		if row.LumiSlug == cases[1].Slug && (row.LumiRate != nil || row.LumiN != 0) {
			t.Fatal("environment error counted as zero")
		}
	}
}
func TestHistoricalComparisonPreservesAndValidatesEvidence(t *testing.T) {
	c := findCase(t, "smoke")
	cfg := testConfig()
	cfg.Repetitions = 1
	p, err := Plan(cfg, []Case{c}, referenceSource(t))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = WriteJSON(filepath.Join(dir, "protocol.json"), p); err != nil {
		t.Fatal(err)
	}
	if err = WriteJSON(filepath.Join(dir, "expected-runs.json"), p.Tasks); err != nil {
		t.Fatal(err)
	}
	result := runOne(context.Background(), cfg, p.Digest, p.Tasks[0], c, filepath.Join(dir, "tasks", p.Tasks[0].key()), modelFunc(func(context.Context, llm.ChatRequest) (llm.ChatResponse, error) { return finalResponse("Paris"), nil }))
	if err = WriteJSON(filepath.Join(dir, "tasks", p.Tasks[0].key(), "result.json"), result); err != nil {
		t.Fatal(err)
	}
	if _, err = BuildReport(dir); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "report.json"))
	out := filepath.Join(t.TempDir(), "comparison")
	comp, err := WriteHistoricalComparison(dir, "../../evals/baselines/pi/catalog.json", out)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "report.json"))
	if string(before) != string(after) {
		t.Fatal("input report changed")
	}
	if comp.Rows[len(comp.Rows)-1].Status != "no_baseline" {
		t.Fatal("invented smoke score")
	}
	if _, err = LoadHistoricalBaseline(filepath.Join(out, "baseline.json")); err != nil {
		t.Fatal("snapshot bundle is not self-contained", err)
	}
	if _, err = WriteHistoricalComparison(dir, "../../evals/baselines/pi/catalog.json", out); err == nil {
		t.Fatal("overwrote comparison")
	}
	if err = os.WriteFile(filepath.Join(out, "sources/pr-10261.json"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadHistoricalBaseline(filepath.Join(out, "baseline.json")); err == nil {
		t.Fatal("changed snapshot accepted")
	}
}
