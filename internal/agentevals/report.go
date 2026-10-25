package agentevals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Stats struct {
	N      int      `json:"n"`
	Mean   *float64 `json:"mean"`
	Min    *float64 `json:"min"`
	Max    *float64 `json:"max"`
	StdDev *float64 `json:"stddev"`
}

func statistics(values []float64) Stats {
	s := Stats{N: len(values)}
	if len(values) == 0 {
		return s
	}
	lo, hi, sum := values[0], values[0], 0.0
	for _, v := range values {
		lo = min(lo, v)
		hi = max(hi, v)
		sum += v
	}
	mean := sum / float64(len(values))
	variance := 0.0
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	sd := math.Sqrt(variance / float64(len(values)))
	s.Mean = &mean
	s.Min = &lo
	s.Max = &hi
	if len(values) > 1 {
		s.StdDev = &sd
	}
	return s
}

type CaseSummary struct {
	Slug              string           `json:"slug"`
	Planned           int              `json:"planned"`
	Scored            int              `json:"scored"`
	Complete          bool             `json:"complete"`
	ObservedVariation bool             `json:"observed_variation"`
	Scores            map[string]Stats `json:"scores"`
	DurationMS        Stats            `json:"duration_ms"`
	InputTokens       Stats            `json:"input_tokens"`
	OutputTokens      Stats            `json:"output_tokens"`
}
type Report struct {
	SchemaVersion  int           `json:"schema_version"`
	ProtocolDigest string        `json:"protocol_digest"`
	Config         Config        `json:"config"`
	Planned        int           `json:"planned"`
	Recorded       int           `json:"recorded"`
	Blocked        []string      `json:"blocked"`
	Results        []Result      `json:"results"`
	Cases          []CaseSummary `json:"cases"`
}

func readJSON(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
func BuildReport(dir string) (Report, error) {
	return buildReport(dir, true)
}

// ReadReport validates the original evidence without overwriting any artifacts.
func ReadReport(dir string) (Report, error) {
	return buildReport(dir, false)
}

func buildReport(dir string, persist bool) (Report, error) {
	var p Protocol
	if err := readJSON(filepath.Join(dir, "protocol.json"), &p); err != nil {
		return Report{}, err
	}
	if p.SchemaVersion != 1 || p.Digest != protocolDigest(p) {
		return Report{}, fmt.Errorf("protocol is invalid or its digest changed")
	}
	var expected []Task
	if err := readJSON(filepath.Join(dir, "expected-runs.json"), &expected); err != nil {
		return Report{}, err
	}
	if !equal(expected, p.Tasks) {
		return Report{}, fmt.Errorf("expected cohort does not match protocol")
	}
	r := Report{SchemaVersion: 1, ProtocolDigest: p.Digest, Config: p.Config, Planned: len(p.Tasks), Blocked: []string{}, Results: []Result{}}
	known := map[string]bool{}
	for _, t := range p.Tasks {
		if known[t.key()] {
			return r, fmt.Errorf("duplicate planned task")
		}
		known[t.key()] = true
	}
	observations := map[string][]Result{}
	err := filepath.WalkDir(filepath.Join(dir, "tasks"), func(path string, d os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "result.json" {
			return nil
		}
		var result Result
		if err := readJSON(path, &result); err != nil {
			r.Blocked = append(r.Blocked, "invalid result: "+path)
			return nil
		}
		key := result.Task.key()
		if !known[key] {
			r.Blocked = append(r.Blocked, "unexpected result: "+key)
			return nil
		}
		if result.ProtocolDigest != p.Digest {
			r.Blocked = append(r.Blocked, "protocol mismatch: "+key)
			return nil
		}
		trace, err := os.ReadFile(filepath.Join(filepath.Dir(path), "trace.jsonl"))
		h := sha256.Sum256(trace)
		if err != nil || result.TraceSHA256 == "" || result.TraceSHA256 != hex.EncodeToString(h[:]) {
			r.Blocked = append(r.Blocked, "missing or changed trace: "+key)
			return nil
		}
		observations[key] = append(observations[key], result)
		return nil
	})
	if err != nil {
		return r, err
	}
	byCase := map[string][]Result{}
	for _, t := range p.Tasks {
		rows := observations[t.key()]
		if len(rows) != 1 {
			r.Blocked = append(r.Blocked, fmt.Sprintf("%s: expected one result, found %d", t.key(), len(rows)))
			continue
		}
		v := rows[0]
		r.Recorded++
		r.Results = append(r.Results, v)
		if v.Status == "environment_error" {
			r.Blocked = append(r.Blocked, t.key()+": environment_error")
		} else if v.Status != "scored" && v.Status != "agent_error" {
			r.Blocked = append(r.Blocked, t.key()+": unknown status")
			continue
		}
		byCase[t.CaseSlug] = append(byCase[t.CaseSlug], v)
	}
	for _, c := range p.Cases {
		summary := CaseSummary{Slug: c.Slug, Planned: p.Config.Repetitions, Scores: map[string]Stats{}}
		dimensions := map[string][]float64{}
		var durations, inputs, outputs []float64
		for _, v := range byCase[c.Slug] {
			durations = append(durations, float64(v.DurationMS))
			if v.Usage.InputTokens != nil {
				inputs = append(inputs, float64(*v.Usage.InputTokens))
			}
			if v.Usage.OutputTokens != nil {
				outputs = append(outputs, float64(*v.Usage.OutputTokens))
			}
			if v.Status == "environment_error" {
				continue
			}
			seen := map[string]bool{}
			valid := len(v.Scores) == len(c.Checks)
			checks := map[string]bool{}
			for _, check := range c.Checks {
				checks[check.Name] = true
			}
			for _, s := range v.Scores {
				if seen[s.Name] || !checks[s.Name] || math.IsNaN(s.Value) || math.IsInf(s.Value, 0) || s.Value < 0 || s.Value > 1 {
					valid = false
				}
				seen[s.Name] = true
			}
			if !valid {
				r.Blocked = append(r.Blocked, v.Task.key()+": missing, duplicate or invalid scores")
				continue
			}
			summary.Scored++
			for _, s := range v.Scores {
				dimensions[s.Name] = append(dimensions[s.Name], s.Value)
			}
		}
		for _, ch := range c.Checks {
			stats := statistics(dimensions[ch.Name])
			summary.Scores[ch.Name] = stats
			if stats.Min != nil && *stats.Min != *stats.Max {
				summary.ObservedVariation = true
			}
		}
		summary.Complete = summary.Scored == summary.Planned
		summary.DurationMS = statistics(durations)
		summary.InputTokens = statistics(inputs)
		summary.OutputTokens = statistics(outputs)
		r.Cases = append(r.Cases, summary)
	}
	sort.Strings(r.Blocked)
	if !persist {
		return r, nil
	}
	if err = WriteJSON(filepath.Join(dir, "report.json"), r); err != nil {
		return r, err
	}
	if err = os.WriteFile(filepath.Join(dir, "report.md"), []byte(renderReport(r)), 0600); err != nil {
		return r, err
	}
	var observationsText strings.Builder
	for _, v := range r.Results {
		b, _ := json.Marshal(v)
		observationsText.Write(b)
		observationsText.WriteByte('\n')
	}
	if err = os.WriteFile(filepath.Join(dir, "observations.jsonl"), []byte(observationsText.String()), 0600); err != nil {
		return r, err
	}
	return r, nil
}
func number(v *float64) string {
	if v == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.3f", *v)
}
func tokens(v *int) string {
	if v == nil {
		return "N/A"
	}
	return fmt.Sprint(*v)
}
func cell(s string) string { return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(s) }
func renderReport(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Lumi Agent evaluation report\n\nModel: `%s` (%s). Planned: %d; recorded: %d. Repetitions: %d.\n\nProtocol: `%s`\n\n", r.Config.Model, r.Config.ProviderType, r.Planned, r.Recorded, r.Config.Repetitions, r.ProtocolDigest)
	b.WriteString("Scores describe each criterion on [0,1]; there is no global acceptance threshold. Agent errors retain observable scores. Environment failures are unscored and excluded from score means. Incomplete cohorts cannot support a complete-run claim. N/A means unavailable, never zero. One repetition cannot establish stability; standard deviation is population SD and is N/A for fewer than two observations. Cost is N/A because this harness has no frozen pricing source.\n\n")
	if len(r.Blocked) > 0 {
		b.WriteString("## Environment / evidence gaps\n\n")
		for _, s := range r.Blocked {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		b.WriteByte('\n')
	}
	b.WriteString("## Repetition statistics\n\n| Case | Recorded scores / planned | Criterion | Mean | Min | Max | SD | Variation |\n|---|---:|---|---:|---:|---:|---:|---|\n")
	for _, c := range r.Cases {
		names := []string{}
		for n := range c.Scores {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			s := c.Scores[n]
			fmt.Fprintf(&b, "| %s | %d/%d | %s | %s | %s | %s | %s | %t |\n", c.Slug, c.Scored, c.Planned, n, number(s.Mean), number(s.Min), number(s.Max), number(s.StdDev), c.ObservedVariation)
		}
	}
	b.WriteString("\n## Individual executions\n\n| Task | Status | Duration (s) | Requests | Input tokens | Output tokens | Cached input | Evidence |\n|---|---|---:|---:|---:|---:|---:|---|\n")
	for _, v := range r.Results {
		fmt.Fprintf(&b, "| %s | %s | %.2f | %d | %s | %s | %s | [result](tasks/%s/result.json), [trace](tasks/%s/trace.jsonl) |\n", v.Task.key(), v.Status, float64(v.DurationMS)/1000, v.Usage.Requests, tokens(v.Usage.InputTokens), tokens(v.Usage.OutputTokens), tokens(v.Usage.CachedInputTokens), v.Task.key(), v.Task.key())
	}
	for _, v := range r.Results {
		fmt.Fprintf(&b, "\n### %s\n\nFinal response:\n\n", v.Task.key())
		for _, line := range strings.Split(v.Final, "\n") {
			fmt.Fprintf(&b, "> %s\n", line)
		}
		if len(v.Issues) > 0 {
			b.WriteString("\nDiagnostics:\n\n")
			for _, i := range v.Issues {
				fmt.Fprintf(&b, "- %s / %s: %s\n", i.Kind, i.Code, i.Message)
			}
		}
		b.WriteString("\n| Criterion | Score | Basis | Observed |\n|---|---:|---|---|\n")
		for _, s := range v.Scores {
			obs, _ := json.Marshal(s.Observed)
			fmt.Fprintf(&b, "| %s | %.3f | %s | %s |\n", s.Name, s.Value, cell(s.Basis), cell(string(obs)))
		}
		if len(v.Tools) > 0 {
			b.WriteString("\nTool execution sequence (full results in evidence):\n\n")
			for n, t := range v.Tools {
				args, _ := json.Marshal(t.Arguments)
				fmt.Fprintf(&b, "%d. `%s` %s — %d ms", n+1, t.Name, string(args), t.DurationMS)
				if t.Error != "" {
					fmt.Fprintf(&b, "; error: %s", t.Error)
				}
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}
