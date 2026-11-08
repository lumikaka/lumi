package textmcp

import (
	"encoding/json"
	"fmt"
	"lumi/internal/production"
	"maps"
	"slices"
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
	for _, action := range slices.Sorted(maps.Keys(requirements)) {
		fields := requirements[action]
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
		"target":       enum("setup", "story_profile", "chapter", "pages", "page", "setting", "premise", "setting_board", "setting_breakdown"),
		"chapter_uuid": uuid, "page_uuid": uuid, "setting_uuid": uuid, "board_uuid": uuid,
		"context_chapter_uuids": uuidList, "context_setting_uuids": uuidList,
		"variables": map[string]any{"type": "object", "additionalProperties": str("Template task input; authoritative project context cannot be overridden.")},
	}, "prompt_group", "prompt_key", "target")
	renderSchema["allOf"] = []any{
		map[string]any{"if": map[string]any{"properties": map[string]any{"target": enum("chapter", "pages", "page")}}, "then": map[string]any{"required": []string{"chapter_uuid"}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"target": enum("page")}}, "then": map[string]any{"required": []string{"page_uuid"}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"target": enum("setting")}}, "then": map[string]any{"required": []string{"setting_uuid"}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"target": enum("setting_breakdown")}}, "then": map[string]any{"required": []string{"board_uuid"}}},
	}
	pictureBook := obj(map[string]any{
		"format":                   enum("classic_picture_book", "wordless_picture_book", "interactive_picture_book", "comic_story", "vertical_strip"),
		"aspect_ratio":             obj(map[string]any{"mode": enum("landscape", "square", "portrait", "custom"), "width": map[string]any{"type": "integer", "minimum": 1}, "height": map[string]any{"type": "integer", "minimum": 1}}, "mode"),
		"large_image_minimal_text": boolSchema, "interaction_mode": enum("find_it", "make_a_choice", "guess", "follow_along"), "comic_layout": enum("four_panel", "page_comic"),
	}, "format")
	shortText := func(max int) map[string]any { return map[string]any{"type": "string", "maxLength": max} }
	key := map[string]any{"type": "string", "minLength": 1, "maxLength": 160, "description": "Stable key for this exact request. Replay original input after interruption; use a new key for a different operation."}
	unit := map[string]any{"type": "number", "minimum": 0, "maximum": 1}
	crop := obj(map[string]any{"type": enum("character", "scene", "prop", "reference"), "title": shortText(160), "summary": shortText(12000), "tags": map[string]any{"type": "array", "minItems": 1, "maxItems": 3, "items": shortText(64)}, "crop_box": obj(map[string]any{"x": unit, "y": unit, "width": unit, "height": unit}, "x", "y", "width", "height"), "confidence": map[string]any{"type": "number", "minimum": 0.92, "maximum": 1}, "setting_uuid": uuid, "expected_revision": rev}, "type", "title", "summary", "tags", "crop_box", "confidence")

	selectedSettings := map[string]any{"type": "array", "items": uuid, "maxItems": production.MaxSectionPremiseAssets, "uniqueItems": true, "description": "Explicit current-project active settings in collage label order. [] means a real no-reference generation; omitted/null is invalid."}
	digest := map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"}
	reviewEvidence := map[string]any{"type": "string", "minLength": 1, "maxLength": 2000}
	settingReviews := map[string]any{"type": "array", "maxItems": production.MaxSectionPremiseAssets, "description": "Host visual assessments of the selected current images. Read image_target for style_digest and image versions. Compare originals and explicit user requirements; record concrete differences or agreement, not just 'checked'. No-reference selection uses [] or omits this field.", "items": obj(map[string]any{
		"setting_uuid": uuid, "variant_uuid": uuid, "expected_revision": rev, "style_digest": digest,
		"verdict": enum("passed", "needs_revision"), "reference_basis": reviewEvidence, "comparison": reviewEvidence,
	}, "setting_uuid", "variant_uuid", "expected_revision", "style_digest", "verdict", "reference_basis", "comparison")}
	preparation := obj(map[string]any{"project_uuid": uuid, "page_uuid": uuid, "expected_revision": rev, "setting_uuids": selectedSettings, "additional_direction": shortText(12000), "setting_reviews": settingReviews, "digest": digest, "signature": digest}, "project_uuid", "page_uuid", "expected_revision", "setting_uuids", "additional_direction", "digest", "signature")
	importSchema := obj(map[string]any{"path": str("Actual absolute local file path from the host image tool result."), "target_uuid": uuid, "slot": enum("page_image", "setting_image"), "expected_revision": rev, "fit_mode": enum("keep", "crop", "pad"), "description": shortText(12000), "prompt": shortText(12000), "preparation": preparation}, "path", "target_uuid", "slot", "expected_revision")
	importSchema["allOf"] = []any{map[string]any{"if": map[string]any{"properties": map[string]any{"slot": map[string]any{"const": "page_image"}}}, "then": map[string]any{"required": []string{"prompt", "preparation"}}}}

	specs := []Spec{
		{"project_init", "Initialize only the process startup directory, preserving existing files. Creates a setup draft; idempotently opens existing projects.", obj(map[string]any{"name": str("Project name; defaults to directory name."), "generation_language": enum("zh-Hans", "en"), "idea": str("Original user idea, required for a new project.")}, "idea")},
		{"project_overview", "Read bound project identity, project revision, setup state and text counts.", obj(map[string]any{})},
		{"project_setup", "Read, edit, or finalize setup without providers. Update then finalize using the new revision. Final format is immutable.", actionSchema(map[string]any{"action": enum("get", "update", "finalize"), "expected_revision": rev, "project_name": str("Final project title."), "generation_language": enum("zh-Hans", "en"), "overall_style": str("Visual style direction."), "generation_brief": str("Creative brief."), "picture_book": pictureBook}, map[string][]string{"update": {"expected_revision"}, "finalize": {"expected_revision"}})},
		{"story_profile", "Read or write the canonical Story Profile and STORY.md projection.", actionSchema(map[string]any{"action": enum("get", "write"), "story_md": str("Complete STORY.md content."), "expected_revision": rev}, map[string][]string{"write": {"story_md", "expected_revision"}})},
		{"chapters", "List/get chapters, create with project revision, update title, write prose, or soft-delete with chapter revision. txt/md supported.", actionSchema(paged(map[string]any{"action": enum("list", "get", "create", "update", "write", "delete"), "chapter_uuid": uuid, "chapter_code": str("Unique code such as vol01.ch01."), "title": str("Chapter title."), "content": str("Complete chapter prose."), "content_format": enum("txt", "md"), "expected_revision": rev}), map[string][]string{"get": {"chapter_uuid"}, "create": {"chapter_code", "title", "expected_revision"}, "update": {"chapter_uuid", "title", "expected_revision"}, "write": {"chapter_uuid", "content", "expected_revision"}, "delete": {"chapter_uuid", "expected_revision"}})},
		{"settings", "List/get/create/update/soft-delete text settings (character, scene, prop, reference); artwork is optional. Create uses premise.revision from list; edits use item.revision.", actionSchema(paged(map[string]any{"action": enum("list", "get", "create", "update", "delete"), "setting_uuid": uuid, "include_versions": boolSchema, "asset_type": enum("character", "scene", "prop", "reference"), "title": str("Setting title."), "summary": str("Full text description, up to 12000 characters."), "tags": map[string]any{"type": "array", "items": str("Search tag."), "maxItems": 50}, "expected_revision": rev}), map[string][]string{"get": {"setting_uuid"}, "create": {"asset_type", "title", "summary", "expected_revision"}, "update": {"setting_uuid", "expected_revision"}, "delete": {"setting_uuid", "expected_revision"}})},
		{"pages", "Use list with summary_only=true to resume; get with summary_only=true reads the complete review. After visual inspection record_image_review binds verdict and observation to the adopted version. List/get/create/update/soft-delete pages and their visual paragraphs. description_md stores page text; storyboard_md stores the complete scene description. Create uses state.revision from list; edits use page.revision.", actionSchema(paged(map[string]any{"action": enum("list", "get", "create", "update", "delete", "record_image_review"), "summary_only": boolSchema, "expected_review_revision": revision("For corrections to an existing review, use review_revision from summary; omit for first review."), "verdict": enum("passed", "needs_revision"), "observation": shortText(2000), "chapter_uuid": uuid, "page_uuid": uuid, "include_versions": boolSchema, "title": str("Page title."), "description_md": str("Narrative, dialogue and captions."), "storyboard_md": str("Visual paragraphs: composition, action, expression, lighting and exact visible copy."), "page_role": enum("body", "front_cover", "back_cover"), "expected_revision": rev}), map[string][]string{"record_image_review": {"chapter_uuid", "page_uuid", "variant_uuid", "expected_revision", "verdict", "observation"}, "list": {"chapter_uuid"}, "get": {"chapter_uuid", "page_uuid"}, "create": {"chapter_uuid", "title", "description_md", "storyboard_md", "expected_revision"}, "update": {"chapter_uuid", "page_uuid", "expected_revision"}, "delete": {"chapter_uuid", "page_uuid", "expected_revision"}})},
		{"book_outline", "Paginated compact structure across the whole book, including chapter and page UUIDs/revisions, roles, and text/image presence. Pass chapter_uuid to read only its pages and their current review status.", obj(paged(map[string]any{"chapter_uuid": uuid}))},
		{"prompt_catalog", "List effective project prompt metadata and required placeholders; templates remain in internal/promptcatalog, including project overrides.", obj(paged(map[string]any{"prompt_group": enum("story", "chapter", "premise", "premise_style", "agent", "runtime")}))},
		{"render_prompt", "Render an effective catalog template with project context and output requirements for Codex to execute. Returns a self-contained task, target UUID, expected_revision and write instructions. Fragments also return fragment_prompt. For page image generation use prepare_page_image instead: it resolves role/style/language/references and signs the complete prompt. render_prompt remains a template/task inspection tool. No provider is called. Supply missing task inputs in variables. Narrow context_chapter_uuids/context_setting_uuids if the result is too large.", renderSchema},
		{"setting_boards", "List/get host-generated setting boards, or import an actual host image path as one resumable batch. Import preserves the original pixels and records source text/style/prompt without a provider. expected_revision is settings list premise.revision. Get returns source, setting_image.asset (read_image reference), board revision and latest committed breakdown. Replay an import with its original key and input even if the host file expired.", actionSchema(paged(map[string]any{"action": enum("list", "get", "import"), "board_uuid": uuid, "path": str("Actual absolute file path returned by host image generation."), "source_text": shortText(12000), "style_snapshot": shortText(12000), "prompt": shortText(12000), "expected_revision": rev, "idempotency_key": key}), map[string][]string{"get": {"board_uuid"}, "import": {"path", "source_text", "prompt", "expected_revision", "idempotency_key"}})},
		{"apply_setting_breakdown", "Apply YOUR visual analysis to a saved board: deterministic crops and all setting/version bindings commit atomically, with no provider call. Use normalized non-overlapping crop_box coordinates from the actual image. Existing settings require setting_uuid and that setting's expected_revision; omitted UUID creates only and rejects existing titles. expected_revision is board.revision; expected_premise_revision is settings list premise.revision. Replay exactly the same request/key to recover; reread after conflict. Returns immutable output UUIDs, not a new generation.", obj(map[string]any{"board_uuid": uuid, "expected_revision": rev, "expected_premise_revision": rev, "idempotency_key": key, "assets": map[string]any{"type": "array", "minItems": 1, "maxItems": 6, "items": crop}}, "board_uuid", "expected_revision", "expected_premise_revision", "idempotency_key", "assets")},

		{"image_target", "Before host image generation, read the target ratio, fallback host ratio for constrained hosts, composition guidance, suggested crop/pad and current revision. For page_image, setting_readiness reports reusable and missing setting images. Omit setting_uuids to inventory candidates, then select only settings needed by this page; missing_images means prepare them first. review_required means images exist but must be visually compared with user requirements and originals; read style_prompt/style_digest and each item variant_uuid/revision for setting_reviews. Empty selection is not readiness approval. First-page limits do not skip setting images. With selected settings, pass review_source_page_uuid to retrieve matching evidence from a previously imported page. Confirm the reference basis still applies before reusing previous_review. For page_image, next call prepare_page_image with the explicit selection; it returns a temporary collage path, the full prompt and signed preparation for import. The common host ratio does not override the project canvas or layout. Settings use a square reference canvas; scenes follow the book ratio. No generation or provider call.", obj(map[string]any{"target_uuid": uuid, "slot": enum("page_image", "setting_image"), "review_source_page_uuid": uuid, "setting_uuids": map[string]any{"type": "array", "items": uuid, "maxItems": 100, "uniqueItems": true, "description": "page_image only. Active setting UUIDs needed for this page, in reference order. Omit to inventory project candidates; [] explicitly selects none, not ready. Only an explicit user instruction can waive missing setting images."}}, "target_uuid", "slot")},
		{"prepare_page_image", "Prepare host page artwork without a provider: explicitly select up to 12 active setting_uuids in label order ([] means no references). Every selected setting requires a passed setting_reviews entry bound to its current variant/revision and style_digest, with reference_basis and concrete comparison. Missing/failed/stale reviews are rejected; the server verifies the record, not visual similarity. Each page gets its own prompt, write-back and signed preparation. Identical final PNGs share a temporary path within this project and service lifetime. Returns reference_image_path and reference_image_sha256 for collage, and ordered versioned sources. " + pageReferenceViewing + " Use referenced_image_paths=[that same path] for collage; omit it for none. Do not read source assets, rewrite references, append to the prompt, or invoke a provider pipeline. Immediately import the host output with the EXACT generation_prompt and unchanged preparation. Changed page/settings/prompts require preparing and generating again.", obj(map[string]any{"page_uuid": uuid, "setting_uuids": selectedSettings, "setting_reviews": settingReviews, "additional_direction": shortText(12000)}, "page_uuid", "setting_uuids")},
		{"import_image", "Import the actual absolute host output path immediately. For page_image, prompt and the complete unchanged preparation (including digest/signature) from prepare_page_image are mandatory. Server reassembles current facts and checks the digest/prompt before writing and inside the final transaction. On stale input prepare/view/generate again; never patch revision/digest. Never guess paths or scan directories. PNG/JPEG/WebP up to 32 MiB/40M pixels; default keep, optional centered crop/pad. No provider call.", importSchema},
		{"read_image", "Read a project Asset UUID as a bounded MCP image for review or host generation reference. Use settings/pages get to obtain current_variant.asset.uuid or current_image.asset.uuid. For page generation use prepare_page_image and its collage viewing rules. This tool remains for setting artwork and review.", obj(map[string]any{"asset_uuid": uuid, "max_edge": map[string]any{"type": "integer", "minimum": 256, "maximum": 1024, "description": "Preview longest edge, default 1024; may shrink further to bound content size."}}, "asset_uuid")},
		{"export_book", "Export the requested chapter (chapter_uuid), or the full book when omitted, to PDF, markdown or ZIP (book.md + book.json + available page images), even before any artwork. PDF reuses existing image PDF export and requires at least one body image. Returns a local path inside this project.", obj(map[string]any{"format": enum("markdown", "zip", "pdf"), "chapter_uuid": uuid, "allow_missing_images": boolSchema}, "format", "allow_missing_images")},
	}
	for i := range specs {
		if specs[i].Name != "pages" && specs[i].Name != "settings" {
			continue
		}
		p := specs[i].Schema["properties"].(map[string]any)
		p["action"].(map[string]any)["enum"] = append(p["action"].(map[string]any)["enum"].([]string), "select_image")
		p["variant_uuid"] = uuid
		fields := []string{"setting_uuid", "variant_uuid", "expected_revision"}
		if specs[i].Name == "pages" {
			fields[0] = "page_uuid"
			fields = append(fields, "chapter_uuid")
		}
		conditions := specs[i].Schema["allOf"].([]any)
		specs[i].Schema["allOf"] = append(conditions, map[string]any{"if": map[string]any{"properties": map[string]any{"action": enum("select_image")}}, "then": map[string]any{"required": fields}})
		specs[i].Description += " select_image selects an existing variant_uuid using the target's current expected_revision; For a conversation candidate number, get with include_versions=true to read a paged identity-only image_versions manifest, match version_no, then select with the returned revision."
	}
	candidateSchema := obj(map[string]any{
		"kind": enum("page", "setting", "text"), "chapter_uuid": uuid, "page_uuid": uuid, "setting_uuid": uuid,
		"candidates": map[string]any{"type": "array", "minItems": 2, "maxItems": 12, "items": obj(map[string]any{"title": str("Candidate title"), "text": str("Complete candidate text")}, "title", "text")},
	}, "kind")
	candidateRequirements := map[string][]string{"page": {"chapter_uuid", "page_uuid"}, "setting": {"setting_uuid"}, "text": {"candidates"}}
	for _, kind := range slices.Sorted(maps.Keys(candidateRequirements)) {
		fields := candidateRequirements[kind]
		conditions, _ := candidateSchema["allOf"].([]any)
		candidateSchema["allOf"] = append(conditions, map[string]any{"if": map[string]any{"properties": map[string]any{"kind": enum(kind)}}, "then": map[string]any{"required": fields}})
	}
	watchSchema := actionSchema(map[string]any{"action": enum("open", "next", "close"), "project_uuid": uuid, "subscription_uuid": uuid, "cursor": map[string]any{"type": "string", "pattern": "^(0|[1-9][0-9]{0,19})$"}}, map[string][]string{"open": {"project_uuid"}, "next": {"project_uuid", "subscription_uuid", "cursor"}, "close": {"project_uuid", "subscription_uuid"}})
	return append(specs,
		Spec{"read_app_review", "App-only legacy review details/candidates. Payload is UI-only _meta; use read_app_resource for normal workspace pages.", obj(paged(map[string]any{"view": enum("book", "chapters", "chapter", "page", "settings", "candidates"), "chapter_uuid": uuid, "page_uuid": uuid, "setting_uuid": uuid, "kind": enum("page", "setting", "text"), "candidates": map[string]any{"type": "array", "minItems": 2, "maxItems": 12, "items": obj(map[string]any{"title": str("Candidate title"), "text": str("Candidate prose")}, "title", "text")}}), "view")},
		Spec{"read_app_resource", "App-only on-demand read of an allowlisted resource in the bound project. Business data is delivered in _meta, not model-visible content. No project/path/URL switching.", obj(map[string]any{"resource": str("Allowlisted project-relative resource, or project for the work itself."), "query": map[string]any{"type": "object", "additionalProperties": str("Validated REST query value.")}}, "resource")},
		Spec{"write_app_resource", "App-only user-submitted operation using the existing project REST contract, permissions and revisions. Never invoke automatically on mount, navigation or local version preview. File, export and AI generation require a supported host flow.", obj(map[string]any{"resource": str("Allowlisted project-relative resource."), "method": enum("POST", "PUT", "PATCH", "DELETE"), "query": map[string]any{"type": "object", "additionalProperties": str("Validated REST query value.")}, "body": map[string]any{"type": "object", "additionalProperties": true}}, "resource", "method")},
		Spec{"read_app_image_detail", "App-only enlarged JPEG for an opened artwork dialog. Image bytes are delivered in UI metadata.", obj(map[string]any{"asset_uuid": uuid, "max_edge": map[string]any{"type": "integer", "minimum": 256, "maximum": 2048}}, "asset_uuid", "max_edge")},
		Spec{"read_app_image", "App-only bounded JPEG preview. No filesystem paths; use read_image for model references.", obj(map[string]any{"asset_uuid": uuid, "max_edge": map[string]any{"type": "integer", "minimum": 256, "maximum": 1024}}, "asset_uuid")},
		Spec{"read_app_connection", "Internal CLI connection descriptor for the bound project's hint relay; not registered as an MCP tool.", obj(map[string]any{})},
		Spec{"watch_app_changes", "App-only WS event subscription. open joins the bound project, next blocks for a real WS change or keepalive, close releases it. Cursor prevents missed changes between calls. No timed business-state queries or writes.", watchSchema},
		Spec{"review_book", "Launch the normal shared Web work overview with description, format, progress and chapter/settings entries. Returns a compact identity/summary; UI details load on demand. The launch itself is read-only; explicit user submissions use normal workspace permissions. After requested creation/export, open this overview for a whole work, review_chapter for one chapter or review_page for one page; never wait for UI interaction.", obj(paged(map[string]any{}))},
		Spec{"review_chapters", "Launch the shared chapter list in business order with codes, summaries, page and adopted-image counts. Returns a compact identity/summary; UI loads details and ordinary user operations through the host. Navigate inside the App to a chapter.", obj(paged(map[string]any{}))},
		Spec{"review_chapter", "Open the chapter workspace at its chapter URL with ordered page navigation and adopted images. The UI reads details on demand; this tool returns only identity and a short summary.", obj(paged(map[string]any{"chapter_uuid": uuid}), "chapter_uuid")},
		Spec{"review_page", "Open the shared page workspace. The UI loads complete text, scene and versions on demand and supports user-submitted edits. Local version preview never adopts a version automatically.", obj(map[string]any{"chapter_uuid": uuid, "page_uuid": uuid}, "chapter_uuid", "page_uuid")},
		Spec{"review_settings", "Launch shared setting cards/detail. Returns compact identity and summary; the UI loads complete text, images and versions on demand. Optionally focus one setting.", obj(paged(map[string]any{"setting_uuid": uuid}))},
		Spec{"review_candidates", "Open shared page/setting candidate versions or supplied text candidates. Preview never writes; explicit UI adoption uses the existing revision flow. For conversation number choices, reread the target and use the indicated authoring selection with expected_revision.", candidateSchema},
	)
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
	// Accept historical inputs without advertising retired navigation options.
	if args, ok := value.(map[string]any); ok {
		if name == "review_book" {
			if mode, exists := args["mode"]; exists {
				if mode != "overview" && mode != "preview" {
					return fmt.Errorf("invalid legacy review mode")
				}
				delete(args, "mode")
			}
			if _, exists := args["chapter_uuid"]; exists {
				s = schemas()["review_chapter"]
			}
		}
		if name == "read_app_review" && args["view"] == "reader" {
			args["view"] = "book"
		}
	}
	return s.Validate(value)
}
