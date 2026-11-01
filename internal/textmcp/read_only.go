package textmcp

import (
	"context"
	"encoding/json"

	"lumi/internal/httpapi"
	"lumi/internal/project"
)

// CallReadOnly is an additive CLI transport mode. Ordinary MCP registrations,
// arguments and behavior are unchanged. Reject before opening or dispatching.
func (s *Service) CallReadOnly(ctx context.Context, root, name string, raw json.RawMessage) httpapi.Envelope {
	return s.Call(project.WithReadOnly(ctx), root, name, raw)
}

func readOnlyOperation(name, action string) bool {
	switch name {
	case "project_overview", "book_outline", "prompt_catalog", "render_prompt":
		return true
	case "project_setup", "story_profile", "chapters", "settings", "pages":
		return action == "get" || action == "list"
	}
	return false
}
