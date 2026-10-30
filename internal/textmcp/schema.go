package textmcp

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

type Spec struct {
	Name, Description string
	Schema            map[string]any
}

func obj(properties map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func str(description string) map[string]any {
	return map[string]any{"type": "string", "maxLength": 120000, "description": description}
}
func enum(values ...string) map[string]any { return map[string]any{"type": "string", "enum": values} }
func revision(description string) map[string]any {
	return map[string]any{"type": "integer", "minimum": 0, "description": description}
}
func paged(p map[string]any) map[string]any {
	p["after"] = map[string]any{"type": "string", "pattern": "^[0-9]{1,10}$", "description": "next_cursor from the previous response."}
	p["before"] = map[string]any{"type": "string", "pattern": "^[0-9]{1,10}$", "description": "Return a page before this cursor; mutually exclusive with after."}
	p["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "List size, default 20; reduce if result_too_large."}
	return p
}
func actionSchema(p map[string]any, requirements map[string][]string) map[string]any {
	s := obj(p, "action")
	conditions := []any{}
	for action, fields := range requirements {
		conditions = append(conditions, map[string]any{"if": map[string]any{"properties": map[string]any{"action": map[string]any{"const": action}}}, "then": map[string]any{"required": fields}})
	}
	s["allOf"] = conditions
	return s
}

func Specs() []Spec {
	uuid := map[string]any{"type": "string", "pattern": "^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$", "description": "Public UUIDv7 from a previous read."}
	rev := revision("Exact revision from the last read or render_prompt; required for every mutation.")
	boolSchema := map[string]any{"type": "boolean"}
	uuidList := map[string]any{"type": "array", "items": uuid, "maxItems": 100, "description": "Limit prompt context to these resources; [] omits the collection. Default: all. The write target is always included separately."}
	renderSchema := obj(map[string]any{
		"prompt_group": enum("story", "chapter", "premise", "premise_style", "agent", "runtime"),
		"prompt_key":   str("Key from prompt_catalog."),
		"target":       enum("setup", "story_profile", "chapter", "pages", "page", "setting", "premise"),
		"chapter_uuid": uuid, "page_uuid": uuid, "setting_uuid": uuid,
		"context_chapter_uuids": uuidList, "context_setting_uuids": uuidList,
		"variables": map[string]any{"type": "object", "additionalProperties": str("Template task input; authoritative project context cannot be overridden.")},
	}, "prompt_group", "prompt_key", "target")
	renderSchema["allOf"] = []any{
		map[string]any{"if": map[string]any{"properties": map[string]any{"target": enum("chapter", "pages", "page")}}, "then": map[string]any{"required": []string{"chapter_uuid"}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"target": enum("page")}}, "then": map[string]any{"required": []string{"page_uuid"}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"target": enum("setting")}}, "then": map[string]any{"required": []string{"setting_uuid"}}},
	}
	pictureBook := obj(map[string]any{
		"format":                   enum("classic_picture_book", "wordless_picture_book", "interactive_picture_book", "comic_story", "vertical_strip"),
		"aspect_ratio":             obj(map[string]any{"mode": enum("landscape", "square", "portrait", "custom"), "width": map[string]any{"type": "integer", "minimum": 1}, "height": map[string]any{"type": "integer", "minimum": 1}}, "mode"),
		"large_image_minimal_text": boolSchema, "interaction_mode": enum("find_it", "make_a_choice", "guess", "follow_along"), "comic_layout": enum("four_panel", "page_comic"),
	}, "format")
	return []Spec{
		{"project_init", "Initialize only the process startup directory, preserving existing files. Creates a setup draft; idempotently opens existing projects.", obj(map[string]any{"name": str("Project name; defaults to directory name."), "generation_language": enum("zh-Hans", "en"), "idea": str("Original user idea, required for a new project.")}, "idea")},
		{"project_overview", "Read bound project identity, project revision, setup state and text counts.", obj(map[string]any{})},
		{"project_setup", "Read, edit, or finalize setup without providers. Update then finalize using the new revision. Final format is immutable.", actionSchema(map[string]any{"action": enum("get", "update", "finalize"), "expected_revision": rev, "project_name": str("Final project title."), "generation_language": enum("zh-Hans", "en"), "overall_style": str("Visual style direction."), "generation_brief": str("Creative brief."), "picture_book": pictureBook}, map[string][]string{"update": {"expected_revision"}, "finalize": {"expected_revision"}})},
		{"story_profile", "Read or write the canonical Story Profile and STORY.md projection.", actionSchema(map[string]any{"action": enum("get", "write"), "story_md": str("Complete STORY.md content."), "expected_revision": rev}, map[string][]string{"write": {"story_md", "expected_revision"}})},
		{"chapters", "List/get chapters, create with project revision, update title, write prose, or soft-delete with chapter revision. txt/md supported.", actionSchema(paged(map[string]any{"action": enum("list", "get", "create", "update", "write", "delete"), "chapter_uuid": uuid, "chapter_code": str("Unique code such as vol01.ch01."), "title": str("Chapter title."), "content": str("Complete chapter prose."), "content_format": enum("txt", "md"), "expected_revision": rev}), map[string][]string{"get": {"chapter_uuid"}, "create": {"chapter_code", "title", "expected_revision"}, "update": {"chapter_uuid", "title", "expected_revision"}, "write": {"chapter_uuid", "content", "expected_revision"}, "delete": {"chapter_uuid", "expected_revision"}})},
		{"settings", "List/get/create/update/soft-delete text settings (character, scene, prop, reference); artwork is optional. Create uses premise.revision from list; edits use item.revision.", actionSchema(paged(map[string]any{"action": enum("list", "get", "create", "update", "delete"), "setting_uuid": uuid, "asset_type": enum("character", "scene", "prop", "reference"), "title": str("Setting title."), "summary": str("Full text description, up to 12000 characters."), "tags": map[string]any{"type": "array", "items": str("Search tag."), "maxItems": 50}, "expected_revision": rev}), map[string][]string{"get": {"setting_uuid"}, "create": {"asset_type", "title", "summary", "expected_revision"}, "update": {"setting_uuid", "expected_revision"}, "delete": {"setting_uuid", "expected_revision"}})},
		{"pages", "List/get/create/update/soft-delete pages and their visual paragraphs. description_md stores page text; storyboard_md stores the complete scene description. Create uses state.revision from list; edits use page.revision.", actionSchema(paged(map[string]any{"action": enum("list", "get", "create", "update", "delete"), "chapter_uuid": uuid, "page_uuid": uuid, "title": str("Page title."), "description_md": str("Narrative, dialogue and captions."), "storyboard_md": str("Visual paragraphs: composition, action, expression, lighting and exact visible copy."), "page_role": enum("body", "front_cover", "back_cover"), "expected_revision": rev}), map[string][]string{"list": {"chapter_uuid"}, "get": {"chapter_uuid", "page_uuid"}, "create": {"chapter_uuid", "title", "description_md", "storyboard_md", "expected_revision"}, "update": {"chapter_uuid", "page_uuid", "expected_revision"}, "delete": {"chapter_uuid", "page_uuid", "expected_revision"}})},
		{"book_outline", "Paginated compact structure across the whole book, including chapter and page UUIDs/revisions, roles, and text/image presence. Does not return prose.", obj(paged(map[string]any{}))},
		{"prompt_catalog", "List effective project prompt metadata and required placeholders; templates remain in internal/promptcatalog, including project overrides.", obj(paged(map[string]any{"prompt_group": enum("story", "chapter", "premise", "premise_style", "agent", "runtime")}))},
		{"render_prompt", "Render an effective catalog template with project context and output requirements for Codex to execute. Returns a self-contained task, target UUID, expected_revision and write instructions. No provider is called. Supply missing task inputs in variables. Narrow context_chapter_uuids/context_setting_uuids if the result is too large.", renderSchema},
		{"export_book", "Export all text to markdown or ZIP (book.md + book.json + available page images), even before any artwork. PDF reuses existing image PDF export and requires at least one body image. Returns a local path inside this project.", obj(map[string]any{"format": enum("markdown", "zip", "pdf"), "allow_missing_images": boolSchema}, "format", "allow_missing_images")},
	}
}

var schemas = sync.OnceValue(func() map[string]*jsonschema.Resolved {
	result := map[string]*jsonschema.Resolved{}
	for _, spec := range Specs() {
		b, _ := json.Marshal(spec.Schema)
		var s jsonschema.Schema
		if err := json.Unmarshal(b, &s); err != nil {
			panic(err)
		}
		r, err := s.Resolve(nil)
		if err != nil {
			panic(err)
		}
		result[spec.Name] = r
	}
	return result
})

func validate(name string, raw json.RawMessage) error {
	s, ok := schemas()[name]
	if !ok {
		return fmt.Errorf("unknown tool: %s", name)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return s.Validate(value)
}
