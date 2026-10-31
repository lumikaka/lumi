package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lumi/internal/platformpath"
)

const usage = `Usage: lumi <serve|status|stop|init|open|project info|session import|mcp> [--environment production|development] [--data-dir PATH] [--json] [--timeout 90s]

serve   Ensure a detached CLI server is running and print its address.
status  Print the CLI instance and its open projects (exit 3 if stopped).
stop    Gracefully stop the CLI instance and release its projects.
init    Initialize cwd in place [--name NAME] [--language zh-Hans|en].
open    Open cwd project in the CLI instance's complete web UI (--json prints URL).
project info  Inspect cwd project identity and Codex write-protection status.
session import --rollout PATH  Manually save user/assistant text and Lumi tool names as read-only history.
                              Tool arguments, outputs and images are omitted. Automatic hooks are unavailable.
mcp     Run a cwd-bound stdio MCP server; auto-start the CLI instance.
`

type options struct {
	environment string
	dataDir     string
	json        bool
	timeout     time.Duration
	projectName string
	language    string
	rollout     string
}

func main() { os.Exit(command(os.Args[1:], os.Stdout, os.Stderr)) }

func command(args []string, out, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "help" || args[0] == "-h") {
		fmt.Fprint(out, usage)
		return 0
	}
	opt := options{}
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	name := args[0]
	if name == "project" && len(args) > 1 && args[1] == "info" {
		name = "project info"
		args = append([]string{name}, args[2:]...)
	}
	if name == "session" && len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		name = "session " + args[1]
		args = append([]string{name}, args[2:]...)
	}
	flags := flag.NewFlagSet("lumi "+name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opt.environment, "environment", "production", "production or development (ignores APP_ENV)")
	flags.StringVar(&opt.dataDir, "data-dir", os.Getenv("LUMI_DATA_DIR"), "application data directory")
	flags.BoolVar(&opt.json, "json", false, "output a JSON envelope")
	flags.DurationVar(&opt.timeout, "timeout", 90*time.Second, "maximum startup or shutdown wait")
	if name == "init" {
		flags.StringVar(&opt.projectName, "name", "", "project name (defaults to directory name)")
		flags.StringVar(&opt.language, "language", "zh-Hans", "project generation language")
	}
	if name == "session import" {
		flags.StringVar(&opt.rollout, "rollout", "", "Codex Desktop rollout JSONL path (explicit manual import only)")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(out, usage)
			return 0
		}
		// Parse may fail before reaching --json; keep error output predictable.
		for _, arg := range args[1:] {
			if arg == "--json" || arg == "--json=true" {
				opt.json = true
			}
		}
		return printError(opt, out, stderr, "invalid_arguments", err, 2)
	}
	if name == "session enable" || name == "session disable" {
		return printError(opt, out, stderr, "session_hooks_unavailable", errors.New("automatic import is unavailable and no Lumi session hook is installed; use lumi session import --rollout PATH (Codex App hook trust and lifecycle remain unverified)"), 2)
	}
	if flags.NArg() != 0 || (name != "serve" && name != "status" && name != "stop" && name != "__serve" && name != "init" && name != "open" && name != "project info" && name != "session import" && name != "mcp") || (opt.environment != "production" && opt.environment != "development") || opt.timeout <= 0 {
		return printError(opt, out, stderr, "invalid_arguments", errors.New("invalid command, environment, arguments or timeout"), 2)
	}
	var err error
	if strings.TrimSpace(opt.dataDir) == "" {
		opt.dataDir, err = platformpath.DefaultAppDataDir(opt.environment)
	}
	if err == nil {
		opt.dataDir, err = filepath.Abs(opt.dataDir)
	}
	if err != nil {
		return printError(opt, out, stderr, "configuration_failed", err, 1)
	}
	if name == "__serve" {
		if err := runDaemon(opt); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if name == "mcp" {
		if err := runMCP(opt); err != nil {
			fmt.Fprintln(stderr, "lumi mcp:", err)
			return 1
		}
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), opt.timeout)
	defer cancel()
	if name == "session import" {
		return importSessionCommand(ctx, opt, out, stderr)
	}
	if name == "open" {
		return openProjectCommand(ctx, opt, out, stderr)
	}
	if name == "init" || name == "project info" {
		return projectCommand(ctx, name, opt, out, stderr)
	}
	var status instanceStatus
	switch name {
	case "serve":
		status, err = ensureInstance(ctx, opt)
	case "status":
		status, err = inspectInstance(ctx, opt)
	case "stop":
		status, err = stopInstance(ctx, opt)
	}
	if err != nil && !errors.Is(err, errNotRunning) {
		return printError(opt, out, stderr, "instance_unavailable", err, 1)
	}
	if errors.Is(err, errNotRunning) {
		status = stoppedStatus(opt)
	}
	if opt.json {
		if err := json.NewEncoder(out).Encode(map[string]any{"success": true, "data": status}); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else if status.Running {
		fmt.Fprintf(out, "Lumi CLI: %s\nPID: %d\nEnvironment: %s\nOpen projects: %d\n", status.Endpoint, status.PID, status.Environment, len(status.OpenProjects))
		for _, item := range status.OpenProjects {
			fmt.Fprintf(out, "  %s  %s  %s\n", item.UUID, item.Name, item.RootPath)
		}
	} else {
		fmt.Fprintf(out, "Lumi CLI is stopped (%s).\n", opt.environment)
	}
	if name == "status" && !status.Running {
		return 3
	}
	return 0
}

func printError(opt options, out, stderr io.Writer, code string, err error, exitCode int) int {
	if opt.json {
		_ = json.NewEncoder(out).Encode(map[string]any{"success": false, "data": nil, "error": map[string]string{"code": code, "message": err.Error(), "details": ""}})
	} else {
		fmt.Fprintln(stderr, "lumi:", err)
	}
	return exitCode
}
