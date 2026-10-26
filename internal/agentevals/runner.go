package agentevals

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"lumi/internal/agent"
	"lumi/internal/llm"
	"lumi/internal/llmlog"
	"lumi/internal/providerdiag"
)

type Task struct {
	CaseSlug   string `json:"case_slug"`
	Repetition int    `json:"repetition"`
}

func (t Task) key() string { return fmt.Sprintf("%s/run-%03d", t.CaseSlug, t.Repetition) }

type Protocol struct {
	SchemaVersion   int               `json:"schema_version"`
	UUID            string            `json:"uuid"`
	CreatedAt       time.Time         `json:"created_at"`
	ReferenceCommit string            `json:"reference_commit"`
	Config          Config            `json:"config"`
	SystemPrompt    string            `json:"system_prompt"`
	PromptMode      string            `json:"prompt_mode,omitempty"`
	Cases           []Case            `json:"cases"`
	Tasks           []Task            `json:"tasks"`
	Source          map[string]string `json:"source"`
	Digest          string            `json:"digest"`
}
type Usage struct {
	InputTokens       *int     `json:"input_tokens"`
	OutputTokens      *int     `json:"output_tokens"`
	CachedInputTokens *int     `json:"cached_input_tokens"`
	CostUSD           *float64 `json:"cost_usd"`
	Requests          int      `json:"requests"`
}
type Issue struct {
	Kind    string `json:"kind"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Result struct {
	Task                Task              `json:"task"`
	ProtocolDigest      string            `json:"protocol_digest"`
	Status              string            `json:"status"`
	Final               string            `json:"final"`
	Scores              []Score           `json:"scores"`
	Issues              []Issue           `json:"issues"`
	StartedAt           time.Time         `json:"started_at"`
	DurationMS          int64             `json:"duration_ms"`
	Usage               Usage             `json:"usage"`
	Turns               []agent.Turn      `json:"turns"`
	Items               []agent.Item      `json:"items"`
	Tools               []ToolEvent       `json:"tools"`
	Files               map[string]string `json:"files"`
	ActiveConfiguration map[string]any    `json:"active_configuration"`
	ReloadErrors        []string          `json:"reload_errors"`
	TraceSHA256         string            `json:"trace_sha256"`
}
type traceWriter struct {
	file   *os.File
	secret string
	err    error
}

func (t *traceWriter) event(kind string, data any) {
	if t.err != nil {
		return
	}
	b, err := json.Marshal(map[string]any{"time": time.Now().UTC(), "type": kind, "data": data})
	if err != nil {
		t.err = err
		return
	}
	if t.secret != "" {
		b = []byte(strings.ReplaceAll(string(b), t.secret, "[REDACTED]"))
	}
	_, t.err = t.file.Write(append(b, '\n'))
	if t.err == nil {
		t.err = t.file.Sync()
	}
}

type recordingClient struct {
	inner  llm.ToolClient
	trace  *traceWriter
	usage  Usage
	issues []Issue
}

func (c *recordingClient) Complete(ctx context.Context, r llm.ChatRequest) (llm.ChatResponse, error) {
	payload, err := llmlog.EncodeChatRequest(r)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	c.trace.event("model_request", payload)
	if c.trace.err != nil {
		return llm.ChatResponse{}, c.trace.err
	}
	start := time.Now()
	response, err := c.inner.Complete(ctx, r)
	c.usage.Requests++
	if err != nil {
		var le *llm.Error
		if errors.As(err, &le) && le.PartialChatResponse() != nil {
			response = *le.PartialChatResponse()
		}
	}
	addMetric(&c.usage.InputTokens, response.Usage.InputTokens, response.Usage.InputKnown, c.usage.Requests)
	addMetric(&c.usage.OutputTokens, response.Usage.OutputTokens, response.Usage.OutputKnown, c.usage.Requests)
	if response.Usage.CachedInputTokens == nil {
		c.usage.CachedInputTokens = nil
	} else {
		addMetric(&c.usage.CachedInputTokens, *response.Usage.CachedInputTokens, true, c.usage.Requests)
	}
	encoded, encodeErr := llmlog.EncodeChatResponse(response, r.APIKey)
	event := map[string]any{"response": encoded, "duration_ms": time.Since(start).Milliseconds()}
	if err != nil {
		issue := classifyModelError(err)
		c.issues = append(c.issues, issue)
		event["error"] = issue
		var le *llm.Error
		if errors.As(err, &le) {
			d := le.Diagnostic
			event["provider_diagnostic"] = map[string]any{"http_status": d.HTTPStatus, "provider_code": d.ProviderCode, "message": d.Message, "request_id": d.RequestID, "retry_after_ms": d.RetryAfter.Milliseconds()}
		}
		event["error_detail"] = providerdiag.RedactPreview(err.Error(), r.APIKey, 4096)
		if errors.As(err, &le) && le.InvalidProviderResponse() != nil {
			diagnostic, e := llmlog.EncodeProviderResponseDiagnostic(*le.InvalidProviderResponse(), r.APIKey)
			if e != nil {
				encodeErr = e
			} else {
				event["diagnostic"] = diagnostic
			}
		}
	}
	c.trace.event("model_response", event)
	if encodeErr != nil {
		return response, encodeErr
	}
	if c.trace.err != nil {
		return response, c.trace.err
	}
	return response, err
}
func addMetric(dst **int, value int, known bool, request int) {
	if !known || request > 1 && *dst == nil {
		*dst = nil
		return
	}
	if *dst == nil {
		v := 0
		*dst = &v
	}
	**dst += value
}
func classifyModelError(err error) Issue {
	issue := Issue{Kind: "environment", Code: "provider_error", Message: "Model request failed"}
	var le *llm.Error
	if errors.As(err, &le) {
		issue.Code = le.Code
		issue.Message = le.SafeMessage
		if d := le.InvalidProviderResponse(); d != nil {
			issue.Kind = "agent"
			issue.Code = string(d.Reason)
			issue.Message = "Provider returned unusable model output"
			switch d.Reason {
			case llm.ProviderResponseStreamIncomplete, llm.ProviderResponseBodyReadError, llm.ProviderResponseInvalidStatus:
				issue.Kind = "environment"
			}
		}
	}
	if errors.Is(err, context.Canceled) {
		issue.Code = "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		issue.Code = "timeout"
	}
	return issue
}

// Unknown runtime/storage failures must not become a model zero. Attribute a
// failure to Agent behavior only with a known contract/budget code or evidence
// from an invalid model response.
func runtimeFailureKind(code string, issues []Issue) string {
	for _, issue := range issues {
		if issue.Kind == "environment" {
			return "environment"
		}
	}
	if len(issues) > 0 {
		return "agent"
	}
	switch code {
	case agent.CodeTurnBudget, agent.CodeMaxSteps, agent.CodeToolValidation, agent.CodeToolNotAllowed, agent.CodeStateConflict, agent.CodeContextTooLarge:
		return "agent"
	default:
		return "environment"
	}
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func protocolDigest(p Protocol) string { p.Digest = ""; return digest(p) }
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".writing-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(append(b, '\n')); err == nil {
		err = tmp.Sync()
	}
	err = errors.Join(err, tmp.Close())
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func Plan(cfg Config, cases []Case, source map[string]string) (Protocol, error) {
	if err := cfg.Validate(); err != nil {
		return Protocol{}, err
	}
	if len(cases) == 0 {
		return Protocol{}, fmt.Errorf("empty cohort")
	}
	p := Protocol{SchemaVersion: 1, CreatedAt: time.Now().UTC(), ReferenceCommit: ReferenceCommit, Config: cfg, SystemPrompt: SystemPrompt, Cases: cases, Source: source}
	u, err := uuid.NewV7()
	if err != nil {
		return p, err
	}
	p.UUID = u.String()
	p.Config.APIKey = ""
	seen := map[string]bool{}
	for _, c := range cases {
		if err = c.Validate(); err != nil {
			return p, err
		}
		if seen[c.Slug] {
			return p, fmt.Errorf("duplicate case")
		}
		seen[c.Slug] = true
		if c.SystemPrompt != "" {
			p.PromptMode = "per_case"
		}
	}
	if p.PromptMode == "per_case" {
		p.SystemPrompt = ""
	}
	// Reverse case order on even repetitions to reduce simple ordering bias.
	for rep := 1; rep <= cfg.Repetitions; rep++ {
		for index := range cases {
			i := index
			if rep%2 == 0 {
				i = len(cases) - 1 - index
			}
			p.Tasks = append(p.Tasks, Task{CaseSlug: cases[i].Slug, Repetition: rep})
		}
	}
	p.Digest = protocolDigest(p)
	return p, nil
}

func Run(ctx context.Context, output string, cfg Config, cases []Case, source map[string]string, progress io.Writer) (Report, error) {
	p, err := Plan(cfg, cases, source)
	if err != nil {
		return Report{}, err
	}
	if err = os.Mkdir(output, 0700); err != nil {
		return Report{}, fmt.Errorf("output must be a new directory: %w", err)
	}
	if err = WriteJSON(filepath.Join(output, "protocol.json"), p); err != nil {
		return Report{}, err
	}
	if err = WriteJSON(filepath.Join(output, "expected-runs.json"), p.Tasks); err != nil {
		return Report{}, err
	}
	bySlug := map[string]Case{}
	for _, c := range cases {
		bySlug[c.Slug] = c
	}
	for _, task := range p.Tasks {
		if ctx.Err() != nil {
			break
		}
		if progress != nil {
			fmt.Fprintf(progress, "Running %s\n", task.key())
		}
		result := runOne(ctx, cfg, p.Digest, task, bySlug[task.CaseSlug], filepath.Join(output, "tasks", task.key()), llm.NewOpenAICompatibleClient(nil))
		if err = WriteJSON(filepath.Join(output, "tasks", task.key(), "result.json"), result); err != nil {
			return Report{}, err
		}
		if progress != nil {
			fmt.Fprintf(progress, "  %s; %d requests; %.1fs\n", result.Status, result.Usage.Requests, float64(result.DurationMS)/1000)
		}
		// Refresh the report after every task so interruptions leave reviewable progress.
		if _, err = BuildReport(output); err != nil {
			return Report{}, err
		}
	}
	return BuildReport(output)
}
func runOne(parent context.Context, cfg Config, protocol string, task Task, c Case, dir string, model llm.ToolClient) (result Result) {
	result = Result{Task: task, ProtocolDigest: protocol, Status: "scored", StartedAt: time.Now().UTC(), Scores: []Score{}, Issues: []Issue{}}
	e := newEnvironment(c)
	var session *agent.IsolatedSession
	var trace *traceWriter
	var recorder *recordingClient
	defer func() {
		if recovered := recover(); recovered != nil {
			result.Status = "environment_error"
			result.Issues = append(result.Issues, Issue{Kind: "environment", Code: "harness_panic", Message: fmt.Sprint(recovered)})
			if trace != nil {
				trace.event("panic", string(debug.Stack()))
			}
		}
		if session != nil {
			items, err := session.Items(context.Background())
			result.Items = items
			if err != nil {
				result.Status = "environment_error"
				result.Issues = append(result.Issues, Issue{Kind: "environment", Code: "transcript_read", Message: err.Error()})
			}
			if err = session.Close(); err != nil {
				result.Status = "environment_error"
				result.Issues = append(result.Issues, Issue{Kind: "environment", Code: "cleanup", Message: err.Error()})
			}
		}
		result.Tools = e.events
		result.Files = e.files
		if recorder != nil {
			result.Usage = recorder.usage
			result.Issues = append(result.Issues, recorder.issues...)
		}
		if trace != nil {
			trace.event("task_finished", map[string]any{"status": result.Status, "files": e.files, "tools": e.events})
			closeErr := trace.file.Close()
			if err := errors.Join(trace.err, closeErr); err != nil {
				result.Status = "environment_error"
				result.Issues = append(result.Issues, Issue{Kind: "environment", Code: "trace_write", Message: err.Error()})
			}
			b, err := os.ReadFile(filepath.Join(dir, "trace.jsonl"))
			if err == nil {
				h := sha256.Sum256(b)
				result.TraceSHA256 = hex.EncodeToString(h[:])
			}
		}
		if result.Status != "environment_error" {
			result.Scores = grade(c, e, result.Final)
		} else {
			result.Scores = nil
		}
		result.DurationMS = time.Since(result.StartedAt).Milliseconds()
		// Secrets are excluded by construction; redact any provider-originated echo too.
		if cfg.APIKey != "" {
			b, _ := json.Marshal(result)
			b = []byte(strings.ReplaceAll(string(b), cfg.APIKey, "[REDACTED]"))
			_ = json.Unmarshal(b, &result)
		}
	}()
	fail := func(code string, err error) {
		result.Status = "environment_error"
		result.Issues = append(result.Issues, Issue{Kind: "environment", Code: code, Message: providerdiag.RedactPreview(err.Error(), cfg.APIKey, 4096)})
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		fail("artifact_directory", err)
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "trace.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		fail("trace_open", err)
		return
	}
	trace = &traceWriter{file: f, secret: cfg.APIKey}
	e.onEvent = func(event ToolEvent) { trace.event("tool_execution", event) }
	trace.event("task_started", map[string]any{"task": task, "protocol_digest": protocol})
	recorder = &recordingClient{inner: model, trace: trace}
	ctx, cancel := context.WithTimeout(parent, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	session, err = agent.NewIsolatedSession(ctx, agent.IsolatedConfig{Request: cfg.request(), SystemPrompt: c.systemPrompt(), MaxRequests: cfg.MaxRequests, Environment: e}, recorder)
	if err != nil {
		fail("session_setup", err)
		return
	}
	for _, step := range c.Steps {
		trace.event("step", step)
		before := len(recorder.issues)
		result.Final = ""
		prompt := step.Content
		turn, runErr := session.Prompt(ctx, prompt)
		result.Turns = append(result.Turns, turn)
		trace.event("turn", turn)
		items, readErr := session.Items(context.Background())
		if readErr != nil {
			fail("transcript_read", readErr)
			return
		}
		for _, item := range items {
			if item.ItemType == "assistant_message" && item.TurnUUID == turn.UUID {
				result.Final = item.Content
				var metadata map[string]any
				_ = json.Unmarshal(item.Metadata, &metadata)
				if reason, _ := metadata["completion_reason"].(string); reason == "budget_limit" || reason == "unexecuted_tool_markup" {
					result.Status = "agent_error"
					result.Issues = append(result.Issues, Issue{Kind: "agent", Code: reason, Message: fmt.Sprintf("Runtime ended this turn: %v", metadata)})
				}
			}
		}
		if runErr != nil || turn.Status != "completed" {
			result.Status = runtimeFailureKind(turn.ErrorCode, recorder.issues[before:]) + "_error"
			if ctx.Err() != nil {
				fail("case_timeout_or_cancelled", ctx.Err())
			} else if runErr != nil && len(recorder.issues) == before && turn.ErrorCode == "" {
				fail("runtime_error", runErr)
			} else if len(recorder.issues) == before {
				result.Issues = append(result.Issues, Issue{Kind: strings.TrimSuffix(result.Status, "_error"), Code: turn.ErrorCode, Message: turn.ErrorMessage})
			}
			break
		}
		if result.Status == "agent_error" {
			break
		}
	}
	return
}
