package agentevals

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/llm"
)

// Historical comparisons use the case definitions and source hashes frozen in
// the real run, never today's executable catalog or implementation files.
func referenceProtocol(t *testing.T) Protocol {
	t.Helper()
	var p Protocol
	if err := readJSON("../../evals/records/2026-10-25-pi-reference-qwen3.7-plus/protocol.json", &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Cases) != 6 || p.Digest != protocolDigest(p) {
		t.Fatal("invalid archived reference protocol")
	}
	return p
}
func referenceCases(t *testing.T) []Case             { return referenceProtocol(t).Cases }
func referenceSource(t *testing.T) map[string]string { return referenceProtocol(t).Source }
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

func TestRetiredReferenceCannotRunButHistoricalReportRemainsReadable(t *testing.T) {
	if _, err := LoadCases("@pi-reference", ""); err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("retired suite accepted: %v", err)
	}
	cases := referenceCases(t)
	if _, err := Plan(testConfig(), cases, referenceSource(t)); err == nil {
		t.Fatal("retired tasks still executable")
	}
	for _, c := range cases {
		dir := t.TempDir()
		if err := WriteJSON(filepath.Join(dir, c.Slug+".json"), c); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadCases(dir, ""); err == nil {
			t.Fatal("custom directory re-enabled retired environment")
		}
	}
	dir := "../../evals/records/2026-10-25-pi-reference-qwen3.7-plus"
	before, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := ReadReport(dir)
	if err != nil || r.Recorded != 18 || len(r.Blocked) != 0 {
		t.Fatalf("lost historical evidence: %+v %v", r, err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil || string(before) != string(after) {
		t.Fatal("historical report was modified")
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
