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
	case "read_app_review", "read_app_resource", "review_book", "review_chapters", "review_chapter", "review_page", "review_settings", "review_candidates", "read_app_image", "read_app_connection":
		return true
	case "project_overview", "book_outline", "prompt_catalog", "render_prompt", "image_target":
		return true
	case "project_setup", "story_profile", "chapters", "settings", "pages":
		return action == "get" || action == "list"
	}
	return false
}
