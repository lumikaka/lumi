package agentevals

// The pi adapter calls this process over JSONL. It shares fixtures, tools and
// grades with Lumi, but never supplies a model response on either engine's behalf.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"lumi/internal/agent"
	"lumi/internal/llm"
)

type SharedSpec struct {
	SchemaVersion int                             `json:"schema_version"`
	SystemPrompt  string                          `json:"system_prompt"`
	Cases         []Case                          `json:"cases"`
	Tools         map[string][]llm.ToolDefinition `json:"tools"`
}

func SharedCases() ([]Case, error) {
	cases, err := LoadCases("", "")
	if err != nil {
		return nil, err
	}
	var shared []Case
	for _, c := range cases {
		switch c.Slug {
		case "smoke", "audit-match", "audit-mismatch", "audit-inconclusive":
			shared = append(shared, c)
		}
	}
	if len(shared) != 4 {
		return nil, fmt.Errorf("shared cohort must contain four cases")
	}
	return shared, nil
}

func ExportSharedSpec(path string) error {
	cases, err := SharedCases()
	if err != nil {
		return err
	}
	spec := SharedSpec{SchemaVersion: 1, SystemPrompt: SystemPrompt, Cases: cases, Tools: map[string][]llm.ToolDefinition{}}
	for _, c := range cases {
		spec.Tools[c.Slug] = newEnvironment(c).Tools()
	}
	return WriteJSON(path, spec)
}

func ServeSharedCase(slug string, input io.Reader, output io.Writer) error {
	cases, err := SharedCases()
	if err != nil {
		return err
	}
	var selected *Case
	for i := range cases {
		if cases[i].Slug == slug {
			selected = &cases[i]
		}
	}
	if selected == nil {
		return fmt.Errorf("not a shared case: %s", slug)
	}
	c := *selected
	e := newEnvironment(c)
	decoder, encoder := json.NewDecoder(input), json.NewEncoder(output)
	for {
		var request struct {
			Op        string          `json:"op"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Final     string          `json:"final"`
		}
		if err := decoder.Decode(&request); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		var response any
		switch request.Op {
		case "describe":
			response = map[string]any{"case": c, "system_prompt": c.systemPrompt(), "tools": e.Tools()}
		case "call":
			args, callErr := e.Validate(request.Name, string(request.Arguments))
			var value any
			if callErr == nil {
				value, callErr = e.Execute(context.Background(), request.Name, args)
			}
			envelope := map[string]any{"success": true, "data": value}
			if callErr != nil {
				code, message := agent.CodeToolValidation, "工具调用失败。"
				var ae *agent.Error
				if errors.As(callErr, &ae) {
					code, message = ae.Code, ae.Message
				}
				envelope = map[string]any{"success": false, "data": nil, "error": map[string]any{"code": code, "message": message, "details": ""}}
			}
			content, err := json.Marshal(envelope)
			if err != nil {
				return err
			}
			response = map[string]any{"content": string(content), "is_error": callErr != nil}
		case "grade":
			response = map[string]any{"scores": grade(c, e, request.Final), "tools": e.events, "files": e.files}
		default:
			return fmt.Errorf("unknown bridge operation: %s", request.Op)
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
}
