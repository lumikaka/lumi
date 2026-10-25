package agentevals

// This is a bounded Go simulation of the three upstream scenario contracts.
// It does not load Pi, execute generated code, or access the host filesystem.
// Source tasks/judges and MIT notice: evals/baselines/pi/reference/.
import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"lumi/internal/llm"
)

func referenceToolCatalog() map[string]llm.ToolDefinition {
	str := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer", "minimum": 1}
	def := func(name, description string, props map[string]any, required ...string) llm.ToolDefinition {
		return llm.ToolDefinition{Name: name, Description: description, Parameters: map[string]any{"type": "object", "properties": props, "required": append([]string{}, required...), "additionalProperties": false}}
	}
	return map[string]llm.ToolDefinition{
		"read":  def("read", "Read a text file. Relative paths use /eval/workspace; ~ means /eval/home. Optional offset and limit count lines, starting at 1.", map[string]any{"path": str, "offset": integer, "limit": integer}, "path"),
		"write": def("write", "Write text to a file, creating parent directories. Accepts relative or absolute paths.", map[string]any{"path": str, "content": str}, "path", "content"),
		"edit":  def("edit", "Replace exactly one occurrence of oldText with newText in a text file.", map[string]any{"path": str, "oldText": str, "newText": str}, "path", "oldText", "newText"),
		"ls":    def("ls", "List files and directories. Default path is the working directory.", map[string]any{"path": str, "limit": integer}),
		"find":  def("find", "Find file paths matching a glob pattern below a directory.", map[string]any{"pattern": str, "path": str, "limit": integer}, "pattern"),
		"grep":  def("grep", "Search text files using a regular expression, returning path, line number and matching text.", map[string]any{"pattern": str, "path": str, "glob": str, "ignoreCase": map[string]any{"type": "boolean"}, "literal": map[string]any{"type": "boolean"}, "limit": integer}, "pattern"),
	}
}

func referencePath(raw string) (string, error) {
	if strings.ContainsAny(raw, "\\\x00") {
		return "", fmt.Errorf("invalid virtual path")
	}
	switch {
	case raw == "~":
		raw = "/eval/home"
	case strings.HasPrefix(raw, "~/"):
		raw = "/eval/home/" + raw[2:]
	case !strings.HasPrefix(raw, "/"):
		raw = "/eval/workspace/" + raw
	}
	p := path.Clean(raw)
	for _, root := range []string{"/eval/home", "/eval/workspace", "/eval/runtime"} {
		if p == root || strings.HasPrefix(p, root+"/") {
			return strings.TrimPrefix(p, "/eval/"), nil
		}
	}
	return "", fmt.Errorf("path is outside the virtual roots")
}

func (e *Environment) validateReferenceTool(name, raw string) (map[string]any, error) {
	if !e.allowed[name] {
		return nil, fmt.Errorf("tool is unavailable")
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil || args == nil {
		return nil, fmt.Errorf("expected an argument object")
	}
	d := referenceToolCatalog()[name]
	props := d.Parameters["properties"].(map[string]any)
	for k, value := range args {
		p, exists := props[k]
		if !exists {
			return nil, fmt.Errorf("unknown field %s", k)
		}
		switch p.(map[string]any)["type"] {
		case "string":
			if _, ok := value.(string); !ok {
				return nil, fmt.Errorf("%s must be a string", k)
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return nil, fmt.Errorf("%s must be boolean", k)
			}
		case "integer":
			n, ok := value.(float64)
			if !ok || n < 1 || n > 100000 || n != float64(int(n)) {
				return nil, fmt.Errorf("invalid %s", k)
			}
		}
	}
	for _, k := range d.Parameters["required"].([]string) {
		if _, ok := args[k]; !ok {
			return nil, fmt.Errorf("missing %s", k)
		}
	}
	if p, ok := args["path"].(string); ok {
		if _, err := referencePath(p); err != nil {
			return nil, err
		}
	}
	return args, nil
}

func (e *Environment) executeReferenceTool(name string, args map[string]any) (any, error) {
	raw, _ := args["path"].(string)
	p, err := referencePath(raw)
	if err != nil {
		return nil, err
	}
	text, exists := e.files[p]
	limit := 1000
	if n, ok := args["limit"].(float64); ok {
		limit = int(n)
	}
	switch name {
	case "read":
		if !exists {
			return nil, toolFault("not_found", "File does not exist")
		}
		start := 0
		if n, ok := args["offset"].(float64); ok {
			start = int(n) - 1
		}
		lines := strings.Split(text, "\n")
		if start >= len(lines) {
			return nil, toolFault("invalid_offset", "Offset exceeds file length")
		}
		if _, ok := args["limit"]; !ok {
			limit = len(lines)
		}
		return strings.Join(lines[start:min(start+limit, len(lines))], "\n"), nil
	case "write", "edit":
		if strings.HasPrefix(p, "runtime/") || p == "runtime" {
			return nil, toolFault("read_only", "Runtime documentation is read-only")
		}
		content, _ := args["content"].(string)
		if name == "edit" {
			old := args["oldText"].(string)
			if !exists || old == "" || strings.Count(text, old) != 1 {
				return nil, toolFault("edit_mismatch", "oldText must match exactly once")
			}
			content = strings.Replace(text, old, args["newText"].(string), 1)
		}
		if p == "home" || p == "workspace" {
			return nil, toolFault("is_directory", "Cannot replace a root directory")
		}
		e.files[p] = content
		return "Successfully wrote " + raw, nil
	case "ls", "find", "grep":
		var pattern *regexp.Regexp
		if name == "grep" {
			s := args["pattern"].(string)
			if args["literal"] == true {
				s = regexp.QuoteMeta(s)
			}
			if args["ignoreCase"] == true {
				s = "(?i)" + s
			}
			pattern, err = regexp.Compile(s)
			if err != nil {
				return nil, toolFault("invalid_pattern", err.Error())
			}
		}
		found := map[string]bool{}
		for file, contents := range e.files {
			if file != p && !strings.HasPrefix(file, p+"/") {
				continue
			}
			rel := strings.TrimPrefix(file, p+"/")
			switch name {
			case "ls":
				part := strings.SplitN(rel, "/", 2)
				entry := part[0]
				if len(part) > 1 {
					entry += "/"
				}
				found[entry] = true
			case "find":
				glob := args["pattern"].(string)
				match, _ := path.Match(glob, rel)
				base, _ := path.Match(glob, path.Base(file))
				if match || base || glob == "**/*" {
					found["/eval/"+file] = true
				}
			case "grep":
				if glob, ok := args["glob"].(string); ok {
					match, _ := path.Match(glob, path.Base(file))
					if !match {
						continue
					}
				}
				for i, line := range strings.Split(contents, "\n") {
					if pattern.MatchString(line) {
						found[fmt.Sprintf("/eval/%s:%d:%s", file, i+1, line)] = true
					}
				}
			}
		}
		out := make([]string, 0, len(found))
		for s := range found {
			out = append(out, s)
		}
		sort.Strings(out)
		if len(out) > limit {
			out = out[:limit]
		}
		return strings.Join(out, "\n"), nil
	}
	return nil, fmt.Errorf("unknown reference tool")
}

func templateBody(raw string) (string, error) {
	s := strings.TrimPrefix(raw, "\ufeff")
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	if strings.HasPrefix(s, "---") {
		if i := strings.Index(s[3:], "\n---"); i >= 0 {
			i += 3
			if i >= 4 {
				var metadata any
				if err := yaml.Unmarshal([]byte(s[4:i]), &metadata); err != nil {
					return "", err
				}
			}
			return strings.TrimSpace(s[i+4:]), nil
		}
	}
	return s, nil
}

func (e *Environment) reloadReference() any {
	e.active = map[string]any{}
	e.reloadErrors = []string{}
	templates := []map[string]any{}
	paths := make([]string, 0, len(e.files))
	for p := range e.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, root := range []string{"home/.pi/agent/prompts", "workspace/.pi/prompts"} {
		for _, p := range paths {
			if path.Dir(p) != root || !strings.HasSuffix(p, ".md") {
				continue
			}
			body, err := templateBody(e.files[p])
			if err != nil {
				e.reloadErrors = append(e.reloadErrors, p+": invalid frontmatter")
				continue
			}
			scope := "project"
			if strings.HasPrefix(p, "home/") {
				scope = "user"
			}
			templates = append(templates, map[string]any{"name": strings.TrimSuffix(path.Base(p), ".md"), "scope": scope, "path": p, "body": body})
		}
	}
	e.active["templates"] = templates
	registry := map[string]map[string]any{"builtin-fixture": {"id": "builtin-fixture", "name": "Built-in fixture", "provider": "openai"}}
	e.active["model_registry"] = registry
	if raw, ok := e.files["home/.pi/agent/models.json"]; ok {
		var configuration map[string]any
		if err := json.Unmarshal([]byte(raw), &configuration); err != nil || configuration == nil {
			e.reloadErrors = append(e.reloadErrors, "models.json: invalid JSON object")
		} else {
			e.active["models"] = configuration
			providers, _ := configuration["providers"].(map[string]any)
			provider, _ := providers["openai"].(map[string]any)
			models, _ := provider["models"].([]any)
			for _, value := range models {
				model, ok := value.(map[string]any)
				if !ok {
					e.reloadErrors = append(e.reloadErrors, "invalid model")
					continue
				}
				id, ok := model["id"].(string)
				if !ok || id == "" {
					e.reloadErrors = append(e.reloadErrors, "invalid model id")
					continue
				}
				registry[id] = model
			}
		}
	}
	for _, kind := range []string{"extensions", "skills"} {
		items := []string{}
		for _, p := range paths {
			if strings.HasPrefix(p, "home/.pi/agent/"+kind+"/") || strings.HasPrefix(p, "workspace/.pi/"+kind+"/") {
				items = append(items, p)
			}
		}
		e.active[kind] = items
	}
	return map[string]any{"errors": e.reloadErrors, "loaded_templates": len(templates)}
}

var emptyTemplateArgs = regexp.MustCompile(`\$\{(\d+|ARGUMENTS|@):-([^}]*)\}|\$\{@:\d+(?::\d+)?\}|\$(ARGUMENTS|@|\d+)`)

func (e *Environment) expandReferencePrompt(prompt string) string {
	// These cases invoke /current-time without arguments. No command executes.
	if prompt == "/current-time" {
		if templates, ok := e.active["templates"].([]map[string]any); ok {
			for _, t := range templates {
				if t["name"] == "current-time" {
					prompt = emptyTemplateArgs.ReplaceAllStringFunc(t["body"].(string), func(s string) string { return emptyTemplateArgs.FindStringSubmatch(s)[2] })
					break
				}
			}
		}
	}
	e.expanded = prompt
	return prompt
}

func (e *Environment) referenceObservation(scope string) any {
	if scope == "model" {
		return e.referenceModelObservation()
	}
	expectedPath, otherPath := "workspace/.pi/prompts/current-time.md", "home/.pi/agent/prompts/current-time.md"
	if scope == "user" {
		expectedPath, otherPath = otherPath, expectedPath
	}
	var template any
	matches := []map[string]any{}
	if templates, ok := e.active["templates"].([]map[string]any); ok {
		for _, t := range templates {
			if t["name"] == "current-time" {
				matches = append(matches, t)
			}
		}
	}
	if len(matches) == 1 {
		template = map[string]any{"scope": matches[0]["scope"], "atExpectedPath": matches[0]["path"] == expectedPath}
	}
	_, otherExists := e.files[otherPath]
	extensions, _ := e.active["extensions"].([]string)
	if extensions == nil {
		extensions = []string{}
	}
	skills, _ := e.active["skills"].([]string)
	if skills == nil {
		skills = []string{}
	}
	diagnostics := append([]string{}, e.reloadErrors...)
	return map[string]any{"template": template, "promptDiagnostics": diagnostics, "extensions": extensions, "extensionErrors": []string{}, "skills": skills, "otherScopeTemplateExists": otherExists, "expandedToTimeRequest": strings.TrimSpace(e.expanded) == "Tell me the current time and timezone."}
}

func (e *Environment) referenceModelObservation() any {
	failure := func(s string) any { return map[string]any{"result": map[string]any{"error": s}} }
	if len(e.reloadErrors) > 0 {
		return failure("configuration error")
	}
	registry, _ := e.active["model_registry"].(map[string]map[string]any)
	model := registry["fixture-chat"]
	if model == nil {
		return failure("Model openai/fixture-chat is unavailable after reload.")
	}
	fields := map[string]any{"id": "fixture-chat", "name": "fixture-chat", "provider": "openai", "reasoning": false, "input": []string{"text"}, "cost": map[string]any{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0}, "contextWindow": 0, "maxTokens": 0}
	for key := range fields {
		if key == "provider" || key == "id" {
			continue
		}
		if v, ok := model[key]; ok {
			fields[key] = v
		}
	}
	// Additive merge preserves the simulated built-in catalog. Full Pi registry
	// semantics, extension providers and generated code are explicitly unverified.
	_, preserved := registry["builtin-fixture"]
	return map[string]any{"result": map[string]any{"model": fields, "existingModelsPreserved": preserved}}
}
