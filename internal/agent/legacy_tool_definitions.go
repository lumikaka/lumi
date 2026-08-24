package agent

// legacyRecoveryToolDefinitions is the frozen schema surface used only when a
// persisted pre-phase-three Run explicitly carries legacy_typed_tools. It is
// intentionally separate from the active four-tool definition set.
func legacyRecoveryToolDefinitions() []map[string]any {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	stringField := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	integerField := func(description string) map[string]any {
		return map[string]any{"type": "integer", "description": description}
	}
	return []map[string]any{
		{"name": "get_story_profile", "description": "Read the current project STORY.md profile.", "parameters": object(map[string]any{})},
		{"name": "update_story_profile", "description": "Create a new current STORY.md profile version.", "parameters": object(map[string]any{"story_md": stringField("Complete STORY.md content"), "expected_revision": integerField("Current profile revision")}, "story_md", "expected_revision")},
		{"name": "list_chapters", "description": "List active story chapters with current story summaries.", "parameters": object(map[string]any{})},
		{"name": "get_chapter", "description": "Read one story chapter by UUID.", "parameters": object(map[string]any{"chapter_uuid": stringField("Public chapter UUIDv7")}, "chapter_uuid")},
		{"name": "update_chapter_story", "description": "Append a new current story version to a chapter.", "parameters": object(map[string]any{"chapter_uuid": stringField("Public chapter UUIDv7"), "content": stringField("Complete replacement chapter content"), "content_format": map[string]any{"type": "string", "enum": []string{"txt", "md"}}, "expected_revision": integerField("Current chapter revision")}, "chapter_uuid", "content", "content_format", "expected_revision")},
		{"name": "get_premise", "description": "Read the current premise profile.", "parameters": object(map[string]any{})},
		{"name": "list_premise_assets", "description": "List active premise assets.", "parameters": object(map[string]any{})},
		{"name": "get_premise_asset", "description": "Read one active premise asset by public UUID.", "parameters": object(map[string]any{"premise_asset_uuid": stringField("Public premise asset UUIDv7")}, "premise_asset_uuid")},
		{"name": "request_api", "description": "Recover a persisted project API call.", "parameters": object(map[string]any{
			"url": stringField("Canonical relative project path"), "method": map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE"}},
			"query": map[string]any{"type": "object", "additionalProperties": true}, "request_body": map[string]any{"type": "object", "additionalProperties": true}, "response_filter": stringField("Optional safe projection"),
		}, "url", "method")},
		{"name": "read_agent_doc", "description": "Recover a persisted Agent documentation read.", "parameters": object(map[string]any{"path": stringField("Registered Agent doc path")}, "path")},
		{"name": "request_current_project_api", "description": "Recover the frozen phase-two asset-reference API adapter.", "parameters": object(map[string]any{
			"url": stringField("Allowlisted current-project API path"), "method": map[string]any{"type": "string", "enum": []string{"GET", "POST", "PATCH", "DELETE"}},
			"request_body": object(map[string]any{"file_uuid": stringField("Public file UUIDv7"), "asset_type": map[string]any{"type": "string", "enum": []string{"character", "scene", "prop", "reference"}}, "title": stringField("Asset title"), "summary": stringField("Asset summary"), "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "expected_revision": integerField("Current asset revision")}),
		}, "url", "method")},
		{"name": "create_premise_asset", "description": "Recover persisted premise asset creation.", "parameters": object(map[string]any{"file_uuid": stringField("Public file UUIDv7"), "upload_uuid": stringField("Public upload UUIDv7"), "asset_type": map[string]any{"type": "string", "enum": []string{"character", "scene", "prop", "reference"}}, "title": stringField("Asset title"), "summary": stringField("Asset summary"), "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "asset_type", "title")},
		{"name": "update_premise_asset", "description": "Recover persisted premise asset update.", "parameters": object(map[string]any{"premise_asset_uuid": stringField("Public premise asset UUIDv7"), "expected_revision": integerField("Current asset revision"), "file_uuid": stringField("Optional public file UUIDv7"), "asset_type": map[string]any{"type": "string", "enum": []string{"character", "scene", "prop", "reference"}}, "title": stringField("Asset title"), "summary": stringField("Asset summary"), "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "premise_asset_uuid", "expected_revision")},
		{"name": "image_gen", "description": "Recover a persisted image generation call.", "parameters": object(map[string]any{"prompt": stringField("Image prompt"), "reference_file_uuids": map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{"type": "string"}}, "size": map[string]any{"type": "string", "enum": []string{"512x512", "1024x1024", "1024x1536", "1536x1024"}}, "quality": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}}, "filename": stringField("Output filename")}, "prompt")},
		{"name": "get_comic_section", "description": "Recover persisted comic section read.", "parameters": object(map[string]any{"chapter_uuid": stringField("Public chapter UUIDv7"), "section_uuid": stringField("Public section UUIDv7")}, "chapter_uuid", "section_uuid")},
		{"name": "update_comic_storyboard", "description": "Recover persisted storyboard update.", "parameters": object(map[string]any{"chapter_uuid": stringField("Public chapter UUIDv7"), "section_uuid": stringField("Public section UUIDv7"), "content_md": stringField("Complete storyboard Markdown"), "expected_revision": integerField("Current section revision")}, "chapter_uuid", "section_uuid", "content_md", "expected_revision")},
		{"name": "start_generation", "description": "Recover persisted domain generation.", "parameters": object(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"story_chapter_generation", "premise_setting_generation", "premise_asset_breakdown", "comic_image_generation"}}, "resource_uuid": stringField("Public target UUIDv7"), "chapter_uuid": stringField("Public chapter UUIDv7"), "model": stringField("Model"), "prompt": stringField("Generation prompt"), "premise_asset_uuids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "kind", "resource_uuid", "prompt")},
		{"name": "request_user_input", "description": "Recover a persisted bounded user-input request.", "parameters": object(map[string]any{
			"input_type": map[string]any{"type": "string", "enum": []string{"single_choice", "multiple_choice"}}, "question": stringField("Question"), "options": map[string]any{"type": "array", "minItems": 2, "maxItems": 8, "items": object(map[string]any{"label": stringField("Label"), "description": stringField("Description")}, "label")},
			"confirmation": object(map[string]any{"route": stringField("Route ID"), "project_uuid": stringField("Project UUIDv7"), "target_uuid": stringField("Target UUIDv7"), "expected_revision": integerField("Revision"), "request_fingerprint": stringField("Request fingerprint"), "confirm_option": integerField("Confirming option")}, "route", "project_uuid", "target_uuid", "expected_revision", "request_fingerprint", "confirm_option"),
		}, "input_type", "question", "options")},
	}
}
