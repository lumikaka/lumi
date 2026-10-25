package agentevals

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"
	"time"

	"lumi/internal/agent"
	"lumi/internal/llm"
)

type ToolEvent struct {
	Name       string         `json:"name"`
	Arguments  map[string]any `json:"arguments"`
	Result     any            `json:"result"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	DurationMS int64          `json:"duration_ms"`
}
type Environment struct {
	files        map[string]string
	active       map[string]any
	allowed      map[string]bool
	faults       map[string]int
	events       []ToolEvent
	reloadErrors []string
	onEvent      func(ToolEvent)
	reference    bool
	expanded     string
}

func newEnvironment(c Case) *Environment {
	e := &Environment{files: map[string]string{}, active: map[string]any{}, allowed: map[string]bool{}, faults: map[string]int{}}
	e.reference = c.Environment == "pi-reference-v1"
	for k, v := range c.Files {
		e.files[k] = v
	}
	for _, t := range c.Tools {
		e.allowed[t] = true
	}
	for k, v := range c.Faults {
		e.faults[k] = v
	}
	return e
}
func validPath(p string) bool {
	return p != "" && p != "." && !strings.Contains(p, "\\") && !strings.HasPrefix(p, "/") && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}
func toolCatalog() map[string]llm.ToolDefinition {
	object := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": append([]string{}, required...), "additionalProperties": false}
	}
	str := map[string]any{"type": "string"}
	return map[string]llm.ToolDefinition{
		"eval_read":   {Name: "eval_read", Description: "Read the complete text of a fixture/workspace file. Paths are relative to the virtual workspace.", Parameters: object(map[string]any{"path": str}, "path")},
		"eval_write":  {Name: "eval_write", Description: "Replace a virtual workspace file with text. Runtime configuration takes effect only after eval_reload or a harness reload step.", Parameters: object(map[string]any{"path": str, "content": str}, "path", "content")},
		"eval_list":   {Name: "eval_list", Description: "List all paths in the virtual workspace.", Parameters: object(map[string]any{})},
		"eval_search": {Name: "eval_search", Description: "Find literal text in virtual files. Returns matching paths and lines.", Parameters: object(map[string]any{"query": str}, "query")},
		"eval_reload": {Name: "eval_reload", Description: "Reload JSON configurations under config/. Returns configuration errors, never executes generated code.", Parameters: object(map[string]any{})},
		"eval_probe":  {Name: "eval_probe", Description: "Inspect or exercise reloaded simulated runtime. Targets: models, provider, stream, hello, footer. Use input for greeting name or provider probe text.", Parameters: object(map[string]any{"target": str, "input": str}, "target", "input")},
		"eval_submit": {Name: "eval_submit", Description: "Submit an audit verdict with implementation evidence. Verdict is match, mismatch or inconclusive. This records a result; then give a concise final answer.", Parameters: object(map[string]any{"verdict": map[string]any{"type": "string", "enum": []string{"match", "mismatch", "inconclusive"}}, "explanation": str}, "verdict", "explanation")},
		"eval_fetch":  {Name: "eval_fetch", Description: "Fetch a fixture record by key. Errors include transient_unavailable (retry) and not_found (do not invent data).", Parameters: object(map[string]any{"key": str}, "key")},
	}
}
func (e *Environment) Tools() []llm.ToolDefinition {
	var out []llm.ToolDefinition
	catalog := toolCatalog()
	if e.reference {
		catalog = referenceToolCatalog()
	}
	for k := range e.allowed {
		out = append(out, catalog[k])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (e *Environment) Validate(name, raw string) (map[string]any, error) {
	if e.reference {
		return e.validateReferenceTool(name, raw)
	}
	if !e.allowed[name] {
		return nil, fmt.Errorf("tool %s is not available", name)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil || args == nil {
		return nil, fmt.Errorf("arguments must be one JSON object")
	}
	def := toolCatalog()[name]
	props := def.Parameters["properties"].(map[string]any)
	for k, v := range args {
		if _, ok := props[k]; !ok {
			return nil, fmt.Errorf("unknown field %s", k)
		}
		if _, ok := v.(string); !ok {
			return nil, fmt.Errorf("%s must be a string", k)
		}
	}
	for _, k := range def.Parameters["required"].([]string) {
		if _, ok := args[k]; !ok {
			return nil, fmt.Errorf("missing field %s", k)
		}
	}
	if p, ok := args["path"].(string); ok && !validPath(p) {
		return nil, fmt.Errorf("path must remain inside virtual workspace")
	}
	if name == "eval_submit" {
		v := args["verdict"]
		if v != "match" && v != "mismatch" && v != "inconclusive" {
			return nil, fmt.Errorf("invalid verdict")
		}
		if strings.TrimSpace(args["explanation"].(string)) == "" {
			return nil, fmt.Errorf("explanation is required")
		}
	}
	return args, nil
}
func toolFault(code, msg string) error { return &agent.Error{Code: code, Message: msg, Details: msg} }
func (e *Environment) Execute(ctx context.Context, name string, args map[string]any) (value any, err error) {
	start := time.Now().UTC()
	defer func() {
		ev := ToolEvent{Name: name, Arguments: args, Result: value, StartedAt: start, DurationMS: time.Since(start).Milliseconds()}
		if err != nil {
			ev.Error = err.Error()
		}
		e.events = append(e.events, ev)
		if e.onEvent != nil {
			e.onEvent(ev)
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if !e.allowed[name] {
		return nil, toolFault("not_allowed", "Unknown evaluation tool")
	}
	if e.reference {
		return e.executeReferenceTool(name, args)
	}
	if e.faults[name] > 0 {
		e.faults[name]--
		return nil, toolFault("transient_unavailable", "Fixture service is temporarily unavailable. Retry the same operation.")
	}
	switch name {
	case "eval_read":
		v, ok := e.files[args["path"].(string)]
		if !ok {
			return nil, toolFault("not_found", "File does not exist")
		}
		return v, nil
	case "eval_write":
		e.files[args["path"].(string)] = args["content"].(string)
		return map[string]any{"written": true}, nil
	case "eval_list":
		paths := []string{}
		for p := range e.files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		return map[string]any{"items": paths}, nil
	case "eval_search":
		matches := []string{}
		for p, s := range e.files {
			for i, line := range strings.Split(s, "\n") {
				if strings.Contains(line, args["query"].(string)) {
					matches = append(matches, fmt.Sprintf("%s:%d:%s", p, i+1, line))
				}
			}
		}
		sort.Strings(matches)
		return map[string]any{"items": matches}, nil
	case "eval_reload":
		return e.reload(), nil
	case "eval_probe":
		return e.probe(args["target"].(string), args["input"].(string))
	case "eval_submit":
		return args, nil
	case "eval_fetch":
		key := args["key"].(string)
		v, ok := e.files["records/"+key+".json"]
		if !ok {
			return nil, toolFault("not_found", "Record does not exist; report unavailable")
		}
		var data any
		if json.Unmarshal([]byte(v), &data) != nil {
			return nil, toolFault("invalid_record", "Record is malformed")
		}
		return data, nil
	}
	return nil, toolFault("not_allowed", "Unknown tool")
}
func (e *Environment) reload() any {
	if e.reference {
		return e.reloadReference()
	}
	e.active = map[string]any{}
	e.reloadErrors = []string{}
	keys := []string{}
	for p := range e.files {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	for _, p := range keys {
		if strings.HasPrefix(p, "config/") {
			var v any
			if err := json.Unmarshal([]byte(e.files[p]), &v); err != nil {
				e.reloadErrors = append(e.reloadErrors, p+": invalid JSON")
			} else {
				e.active[p] = v
			}
		}
	}
	return map[string]any{"errors": e.reloadErrors, "loaded": len(e.active)}
}
func (e *Environment) probe(target, input string) (any, error) {
	if len(e.reloadErrors) > 0 {
		return nil, toolFault("reload_failed", "Fix invalid JSON configuration")
	}
	switch target {
	case "models":
		if v, ok := e.active["config/models.json"]; ok {
			return v, nil
		}
	case "hello":
		if c, ok := e.active["config/hello.json"].(map[string]any); ok {
			tpl, _ := c["template"].(string)
			if strings.Contains(tpl, "{name}") {
				return strings.ReplaceAll(tpl, "{name}", input), nil
			}
		}
	case "provider", "stream":
		c, ok := e.active["config/"+target+".json"].(map[string]any)
		if !ok {
			break
		}
		protocol := "chat_completions"
		if target == "stream" {
			protocol = "event_stream"
		}
		if c["protocol"] != protocol || c["endpoint"] != "fixture://acme" || c["credential_env"] != "ACME_KEY" || c["model"] != "acme-chat" {
			return nil, toolFault("invalid_configuration", "Provider configuration does not match fixture API")
		}
		if input != "ping" {
			return nil, toolFault("invalid_request", "Fixture API accepts ping")
		}
		if target == "stream" && (c["text_field"] != "delta.text" || c["done_event"] != "complete") {
			return nil, toolFault("invalid_stream_mapping", "Use the event fields from the API documentation")
		}
		return map[string]any{"valid_request_received": true, "text": "pong", "input_tokens": 4, "output_tokens": 3, "model": "acme-chat", "protocol": protocol}, nil
	case "footer":
		return e.footer(), nil
	}
	return nil, toolFault("not_configured", "Requested runtime resource is not loaded")
}
func (e *Environment) footer() []string {
	c, _ := e.active["config/footer.json"].(map[string]any)
	var lines []string
	for _, pct := range []float64{42.2, 65, 120} {
		prefix, _ := c["prefix"].(string)
		suffix, _ := c["suffix"].(string)
		width, _ := c["width"].(float64)
		if width < 1 || width > 30 {
			lines = append(lines, "")
			continue
		}
		n := pct
		if c["clamp"] == true {
			n = math.Max(0, math.Min(100, n))
		}
		filled := int(math.Round(width * n / 100))
		filled = max(0, min(int(width), filled))
		lines = append(lines, fmt.Sprintf("%s%s%s %.0f%%%s", prefix, strings.Repeat("█", filled), strings.Repeat("░", int(width)-filled), math.Round(n), suffix))
	}
	return lines
}
