package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lumi/internal/agentevals"
)

func main() { os.Exit(run()) }
func run() int {
	flags := flag.NewFlagSet("lumi_evals", flag.ContinueOnError)
	list := flags.Bool("list", false, "list matching cases without a model request")
	suite := flags.String("suite", "regression", "regression or pi-reference")
	compareDir := flags.String("compare", "", "compare an existing run with a historical baseline; no model requests")
	baseline := flags.String("baseline", "evals/baselines/pi/catalog.json", "historical baseline catalog for --compare")
	reportDir := flags.String("report", "", "rebuild a saved report; no model requests")
	casesDir := flags.String("cases", "", "directory of case JSON files; default: bundled suite")
	filter := flags.String("filter", "", "case slug or suite substring")
	out := flags.String("out", "", "new output directory (default evals/results/<timestamp>)")
	lumiDir := flags.String("lumi-config-dir", "", "read only model settings from this Lumi app data directory")
	providerType := flags.String("provider", os.Getenv("LUMI_EVAL_PROVIDER"), "provider type")
	baseURL := flags.String("base-url", os.Getenv("LUMI_EVAL_BASE_URL"), "model API base URL")
	model := flags.String("model", os.Getenv("LUMI_EVAL_MODEL"), "one model for the whole run")
	repeats := flags.Int("repeat", 1, "repetitions per case (1..100)")
	requests := flags.Int("max-requests", 16, "maximum model requests per turn, including finalization")
	maxTokens := flags.Int("max-tokens", 4096, "maximum output tokens per model request")
	timeout := flags.Int("timeout", 180, "seconds per case")
	temp := flags.String("temperature", "", "optional temperature; omitted uses provider default")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		return 2
	}
	if *compareDir != "" {
		if *reportDir != "" || *list {
			fmt.Fprintln(os.Stderr, "--compare cannot be combined with --report or --list")
			return 2
		}
		if *out == "" {
			*out = filepath.Join("evals", "results", "comparison-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
		}
		r, err := agentevals.WriteHistoricalComparison(*compareDir, *baseline, *out)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Println("Comparison:", filepath.Join(*out, "comparison.md"))
		if len(r.Blocked) > 0 {
			return 1
		}
		return 0
	}
	if *reportDir != "" {
		report, err := agentevals.BuildReport(*reportDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Println(filepath.Join(*reportDir, "report.md"))
		if len(report.Blocked) > 0 {
			return 1
		}
		return 0
	}
	if *suite != "regression" && *suite != "pi-reference" {
		fmt.Fprintln(os.Stderr, "unknown suite")
		return 2
	}
	if *suite == "pi-reference" {
		if *casesDir != "" {
			fmt.Fprintln(os.Stderr, "--suite pi-reference cannot be combined with --cases")
			return 2
		}
		*casesDir = "@pi-reference"
	}
	cases, err := agentevals.LoadCases(*casesDir, *filter)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *list {
		for _, c := range cases {
			fmt.Printf("%s [%s]: %s\n", c.Slug, c.Suite, c.Goal)
		}
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cfg := agentevals.Config{ProviderType: *providerType, BaseURL: *baseURL, Model: *model, APIKey: os.Getenv("LUMI_EVAL_API_KEY"), Repetitions: *repeats, MaxRequests: *requests, MaxTokens: *maxTokens, TimeoutSeconds: *timeout}
	if *lumiDir != "" {
		if cfg.APIKey != "" || cfg.BaseURL != "" {
			fmt.Fprintln(os.Stderr, "do not combine Lumi settings with an explicit key or base URL")
			return 2
		}
		resolved, err := agentevals.ReadLumiProvider(ctx, *lumiDir, *providerType)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Cannot resolve Lumi model settings:", err)
			return 2
		}
		cfg.ProviderType, cfg.BaseURL, cfg.APIKey = resolved.ProviderType, resolved.BaseURL, resolved.APIKey
		if cfg.Model == "" {
			cfg.Model = resolved.DefaultModel
		}
	}
	if cfg.ProviderType == "" {
		cfg.ProviderType = "openai_compatible"
	}
	if *temp != "" {
		v, err := strconv.ParseFloat(*temp, 64)
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid temperature")
			return 2
		}
		cfg.Temperature = &v
	}
	if err = cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *out == "" {
		*out = filepath.Join("evals", "results", time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	if err = os.MkdirAll(filepath.Dir(*out), 0700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	report, err := agentevals.Run(ctx, *out, cfg, cases, sourceManifest(), os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println("Report:", filepath.Join(*out, "report.md"))
	if len(report.Blocked) > 0 {
		return 1
	}
	return 0
}
func sourceManifest() map[string]string {
	result := map[string]string{}
	if b, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		result["git_head"] = strings.TrimSpace(string(b))
	}
	if b, err := exec.Command("git", "status", "--porcelain").Output(); err == nil {
		result["git_status"] = string(b)
	}
	for _, root := range []string{"internal/agent", "internal/llm", "internal/agentevals", "cmd/lumi_evals"} {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			b, err := os.ReadFile(path)
			if err == nil {
				h := sha256.Sum256(b)
				result[filepath.ToSlash(path)] = hex.EncodeToString(h[:])
			}
			return nil
		})
	}
	if exe, err := os.Executable(); err == nil {
		if b, err := os.ReadFile(exe); err == nil {
			h := sha256.Sum256(b)
			result["binary_sha256"] = hex.EncodeToString(h[:])
		}
	}
	return result
}
