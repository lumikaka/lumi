package agentevals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type HistoricalSource struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Kind     string `json:"kind"`
	Snapshot string `json:"snapshot"`
	SHA256   string `json:"sha256"`
}
type HistoricalMapping struct {
	Slug              string   `json:"slug"`
	CaseSHA256        string   `json:"case_sha256"`
	EnvironmentSHA256 string   `json:"environment_sha256"`
	Equivalence       string   `json:"equivalence"`
	Differences       []string `json:"differences"`
}
type HistoricalObservation struct {
	ID                string              `json:"id"`
	SourceID          string              `json:"source_id"`
	Case              string              `json:"case"`
	Variant           string              `json:"variant"`
	Scope             string              `json:"scope"`
	Metric            string              `json:"metric"`
	Value             float64             `json:"value"`
	SampleCount       int                 `json:"sample_count"`
	Successes         *int                `json:"successes"`
	Rounded           bool                `json:"rounded"`
	ExecutionRevision *string             `json:"execution_revision"`
	Config            *Config             `json:"config"`
	Mappings          []HistoricalMapping `json:"mappings"`
	Notes             []string            `json:"notes"`
}
type HistoricalBaseline struct {
	SchemaVersion int                     `json:"schema_version"`
	ID            string                  `json:"id"`
	RetrievedOn   string                  `json:"retrieved_on"`
	Sources       []HistoricalSource      `json:"sources"`
	Observations  []HistoricalObservation `json:"observations"`
	Limitations   []string                `json:"limitations"`
}

func digestBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func CaseDigest(c Case) string    { b, _ := json.Marshal(normalized(c)); return digestBytes(b) }
func EnvironmentDigest(source map[string]string) string {
	selected := map[string]string{}
	for _, p := range []string{"internal/agentevals/environment.go", "internal/agentevals/reference_environment.go", "internal/agentevals/grade.go"} {
		v := source[p]
		if v == "" {
			return ""
		}
		selected[p] = v
	}
	b, _ := json.Marshal(selected)
	return digestBytes(b)
}
func validDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size
}

func LoadHistoricalBaseline(file string) (HistoricalBaseline, error) {
	var b HistoricalBaseline
	f, err := os.Open(file)
	if err != nil {
		return b, err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err = d.Decode(&b); err != nil {
		return b, err
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return b, fmt.Errorf("baseline must contain one object")
	}
	if b.SchemaVersion != 1 || b.ID == "" || len(b.Observations) == 0 {
		return b, fmt.Errorf("invalid baseline header")
	}
	sources := map[string]bool{}
	for _, s := range b.Sources {
		u, err := url.Parse(s.URL)
		if s.ID == "" || sources[s.ID] || err != nil || u.Scheme != "https" || u.Host == "" || !validPath(s.Snapshot) || !strings.HasPrefix(s.Snapshot, "sources/") || !validDigest(s.SHA256) || (s.Kind != "pr_summary" && s.Kind != "run_artifact") {
			return b, fmt.Errorf("invalid baseline source %s", s.ID)
		}
		data, err := os.ReadFile(filepath.Join(filepath.Dir(file), filepath.FromSlash(s.Snapshot)))
		if err != nil || digestBytes(data) != s.SHA256 {
			return b, fmt.Errorf("missing or changed source snapshot %s", s.ID)
		}
		sources[s.ID] = true
	}
	seen := map[string]bool{}
	for _, o := range b.Observations {
		if o.ID == "" || seen[o.ID] || !sources[o.SourceID] || o.Case == "" || o.Metric != "pass_rate" || o.SampleCount < 1 || math.IsNaN(o.Value) || math.IsInf(o.Value, 0) || o.Value < 0 || o.Value > 1 || (o.Variant != "with_docs" && o.Variant != "without_docs") || (o.Scope != "case" && o.Scope != "cohort") {
			return b, fmt.Errorf("invalid observation %s", o.ID)
		}
		if o.Successes != nil && (*o.Successes < 0 || *o.Successes > o.SampleCount || math.Abs(float64(*o.Successes)/float64(o.SampleCount)-o.Value) > 0.000001) {
			return b, fmt.Errorf("inconsistent count in %s", o.ID)
		}
		if o.Scope == "cohort" && len(o.Mappings) > 0 {
			return b, fmt.Errorf("aggregate results cannot be assigned to individual cases")
		}
		mapped := map[string]bool{}
		for _, m := range o.Mappings {
			if !slugPattern.MatchString(m.Slug) || mapped[m.Slug] || !validDigest(m.CaseSHA256) || (m.Equivalence != "adapted" && m.Equivalence != "equivalent") || (m.EnvironmentSHA256 != "" && !validDigest(m.EnvironmentSHA256)) || (m.Equivalence == "adapted" && len(m.Differences) == 0) {
				return b, fmt.Errorf("invalid mapping in %s", o.ID)
			}
			mapped[m.Slug] = true
		}
		seen[o.ID] = true
	}
	return b, nil
}

type HistoricalComparisonRow struct {
	BaselineID            string   `json:"baseline_id"`
	Case                  string   `json:"case"`
	Variant               string   `json:"variant"`
	SourceURL             string   `json:"source_url"`
	Status                string   `json:"status"`
	HistoricalRate        *float64 `json:"historical_rate"`
	HistoricalN           int      `json:"historical_n"`
	LumiSlug              string   `json:"lumi_slug"`
	LumiRate              *float64 `json:"lumi_rate"`
	LumiN                 int      `json:"lumi_n"`
	LumiPlanned           int      `json:"lumi_planned"`
	DeltaPercentagePoints *float64 `json:"delta_percentage_points"`
	Reasons               []string `json:"reasons"`
}
type HistoricalComparison struct {
	SchemaVersion  int                       `json:"schema_version"`
	BaselineID     string                    `json:"baseline_id"`
	BaselineSHA256 string                    `json:"baseline_sha256"`
	ProtocolDigest string                    `json:"protocol_digest"`
	LumiReport     string                    `json:"lumi_report"`
	Config         Config                    `json:"config"`
	Blocked        []string                  `json:"blocked"`
	Rows           []HistoricalComparisonRow `json:"rows"`
}

func localPassRate(r Report, c Case) (*float64, int) {
	passed, count := 0, 0
	for _, run := range r.Results {
		if run.Task.CaseSlug != c.Slug || (run.Status != "scored" && run.Status != "agent_error") {
			continue
		}
		valid := len(run.Scores) == len(c.Checks)
		scores := map[string]float64{}
		for _, s := range run.Scores {
			if _, duplicate := scores[s.Name]; duplicate || s.Value < 0 || s.Value > 1 {
				valid = false
			}
			scores[s.Name] = s.Value
		}
		full := true
		for _, check := range c.Checks {
			v, found := scores[check.Name]
			if !found {
				valid = false
			}
			if v != 1 {
				full = false
			}
		}
		if !valid {
			continue
		}
		count++
		if full {
			passed++
		}
	}
	if count == 0 {
		return nil, 0
	}
	v := float64(passed) / float64(count)
	return &v, count
}

func compareHistorical(p Protocol, r Report, b HistoricalBaseline) HistoricalComparison {
	out := HistoricalComparison{SchemaVersion: 1, BaselineID: b.ID, ProtocolDigest: p.Digest, Config: p.Config, Blocked: append([]string{}, r.Blocked...), Rows: []HistoricalComparisonRow{}}
	sources := map[string]HistoricalSource{}
	for _, s := range b.Sources {
		sources[s.ID] = s
	}
	used := map[string]bool{}
	for _, observation := range b.Observations {
		source := sources[observation.SourceID]
		base := HistoricalComparisonRow{BaselineID: observation.ID, Case: observation.Case, Variant: observation.Variant, SourceURL: source.URL, HistoricalRate: &observation.Value, HistoricalN: observation.SampleCount, Reasons: append([]string{}, observation.Notes...)}
		matched := false
		for _, mapping := range observation.Mappings {
			for _, c := range p.Cases {
				if mapping.Slug != c.Slug {
					continue
				}
				matched = true
				used[c.Slug] = true
				row := base
				row.Reasons = append([]string{}, base.Reasons...)
				row.LumiSlug = c.Slug
				row.LumiPlanned = p.Config.Repetitions
				row.LumiRate, row.LumiN = localPassRate(r, c)
				row.Status = "direct"
				conditions := []string{}
				if mapping.Equivalence != "equivalent" {
					conditions = append(conditions, "用例环境为适配实现，尚未证明与 pi 等价")
				}
				conditions = append(conditions, mapping.Differences...)
				if source.Kind != "run_artifact" {
					conditions = append(conditions, "来源是作者报告的摘要，未取得逐次原始运行记录")
				}
				if observation.ExecutionRevision == nil || *observation.ExecutionRevision == "" {
					conditions = append(conditions, "pi 实际执行提交未报告；不能用 PR head 代替")
				}
				if observation.Config == nil {
					conditions = append(conditions, "pi 模型、提示词配置和执行预算不可用")
				} else {
					a, z := *observation.Config, p.Config
					a.Repetitions = 0
					z.Repetitions = 0
					if !equal(a, z) {
						conditions = append(conditions, "模型、服务商或执行配置不一致")
					}
				}
				if row.LumiN != row.LumiPlanned || len(r.Blocked) > 0 {
					conditions = append(conditions, "本次运行存在环境错误或证据缺口")
				}
				if len(conditions) > 0 {
					row.Status = "approximate"
				}
				if CaseDigest(c) != mapping.CaseSHA256 || mapping.EnvironmentSHA256 == "" || EnvironmentDigest(p.Source) != mapping.EnvironmentSHA256 {
					row.Status = "contract_changed"
					conditions = append(conditions, "本地用例或工具/评分实现与已审核映射不一致，需重新核对")
				}
				row.Reasons = append(row.Reasons, conditions...)
				if row.Status == "direct" && row.LumiRate != nil {
					v := 100 * (*row.LumiRate - observation.Value)
					row.DeltaPercentagePoints = &v
				}
				out.Rows = append(out.Rows, row)
			}
		}
		if !matched {
			base.Status = "no_local_match"
			base.Reasons = append(base.Reasons, "本次没有对应的本地用例；整体成绩不拆分给单个用例")
			out.Rows = append(out.Rows, base)
		}
	}
	for _, c := range p.Cases {
		if !used[c.Slug] {
			rate, n := localPassRate(r, c)
			out.Rows = append(out.Rows, HistoricalComparisonRow{Case: c.Slug, LumiSlug: c.Slug, LumiRate: rate, LumiN: n, LumiPlanned: p.Config.Repetitions, Status: "no_baseline", Reasons: []string{"未收集到此用例可对应的 pi 数值基线"}})
		}
	}
	return out
}

// WriteHistoricalComparison never invokes either Agent and preserves the source run.
func WriteHistoricalComparison(runDir, baselineFile, output string) (HistoricalComparison, error) {
	var empty HistoricalComparison
	b, err := LoadHistoricalBaseline(baselineFile)
	if err != nil {
		return empty, err
	}
	r, err := ReadReport(runDir)
	if err != nil {
		return empty, err
	}
	var p Protocol
	if err = readJSON(filepath.Join(runDir, "protocol.json"), &p); err != nil {
		return empty, err
	}
	report := compareHistorical(p, r, b)
	if relative, e := filepath.Rel(output, filepath.Join(runDir, "report.md")); e == nil {
		report.LumiReport = filepath.ToSlash(relative)
	}
	data, err := os.ReadFile(baselineFile)
	if err != nil {
		return empty, err
	}
	report.BaselineSHA256 = digestBytes(data)
	if err = os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return empty, err
	}
	if err = os.Mkdir(output, 0700); err != nil {
		return empty, fmt.Errorf("comparison output must be new: %w", err)
	}
	if err = WriteJSON(filepath.Join(output, "comparison.json"), report); err != nil {
		return report, err
	}
	if err = WriteJSON(filepath.Join(output, "baseline.json"), b); err != nil {
		return report, err
	}
	// Bundle the cited snapshots so the report remains reviewable offline.
	for _, source := range b.Sources {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(baselineFile), source.Snapshot))
		if err != nil {
			return report, err
		}
		target := filepath.Join(output, source.Snapshot)
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return report, err
		}
		if err = os.WriteFile(target, data, 0600); err != nil {
			return report, err
		}
	}
	err = os.WriteFile(filepath.Join(output, "comparison.md"), []byte(renderHistoricalComparison(report)), 0600)
	return report, err
}

func renderHistoricalComparison(r HistoricalComparison) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Lumi / pi 历史成绩对照\n\n基线：%s；模型：%s；协议：`%s`。\n\n", r.BaselineID, r.Config.Model, r.ProtocolDigest)
	if r.LumiReport != "" {
		fmt.Fprintf(&b, "[Lumi 完整执行报告](<%s>)。\n\n", r.LumiReport)
	}
	counts := map[string]int{}
	for _, row := range r.Rows {
		counts[row.Status]++
	}
	fmt.Fprintf(&b, "可直接比较：%d；仅近似参考：%d；映射已失效：%d；无本地对应：%d；缺少 pi 基线：%d。\n\n", counts["direct"], counts["approximate"], counts["contract_changed"], counts["no_local_match"], counts["no_baseline"])
	b.WriteString("pi 数值来自历史记录。本报告不执行 pi；近似参考不计算能力差值。N/A 表示指标缺失。不同任务或不同文档组不合并总分。\n\n")
	b.WriteString("| 用例 / 文档组 | pi 历史比例 / n | Lumi 满分比例 / 有效数 / 计划数 | 分类 | 差值（百分点） |\n|---|---:|---:|---|---:|\n")
	percent := func(p *float64) string {
		if p == nil {
			return "N/A"
		}
		return fmt.Sprintf("%.1f%%", *p*100)
	}
	clean := func(s string) string { return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(s) }
	labels := map[string]string{"direct": "可直接比较", "approximate": "仅近似参考", "contract_changed": "映射已失效", "no_local_match": "本地无对应用例", "no_baseline": "缺少 pi 基线"}
	for _, row := range r.Rows {
		fmt.Fprintf(&b, "| %s / %s | %s / %d | %s / %d / %d | %s | %s |\n", clean(row.Case), clean(row.Variant), percent(row.HistoricalRate), row.HistoricalN, percent(row.LumiRate), row.LumiN, row.LumiPlanned, labels[row.Status], number(row.DeltaPercentagePoints))
	}
	b.WriteString("\nLumi 满分比例：该次运行全部评分项为 1 的次数 / 有效评分次数。它是本对照的描述性指标，不是运行通过门槛。环境故障不计作模型零分，计划数保留。小样本和四舍五入的历史比例不能证明稳定性或统计显著性。\n\n")
	for _, row := range r.Rows {
		fmt.Fprintf(&b, "## %s / %s\n\n", clean(row.Case), clean(row.Variant))
		if row.SourceURL != "" {
			fmt.Fprintf(&b, "来源：[原始记录](%s)。\n\n", row.SourceURL)
		}
		for _, reason := range row.Reasons {
			fmt.Fprintf(&b, "- %s\n", clean(reason))
		}
		b.WriteByte('\n')
	}
	if len(r.Blocked) > 0 {
		b.WriteString("## 本地证据缺口\n\n")
		for _, issue := range r.Blocked {
			fmt.Fprintf(&b, "- %s\n", clean(issue))
		}
	}
	return b.String()
}
