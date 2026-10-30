package textmcp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/httpapi"
	"lumi/internal/project"
	"lumi/internal/story"
)

func harness(t *testing.T) (*Service, string, string) {
	t.Helper()
	data := t.TempDir()
	app, err := appstore.Open(data, config.SQLiteDSN(filepath.Join(data, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen)
	t.Cleanup(func() { _ = manager.Close(); _ = app.Close() })
	root := t.TempDir()
	p, err := manager.InitAt(t.Context(), root, project.CreateInput{Name: "Text test", GenerationLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	return &Service{Projects: manager}, root, p.UUID
}
func invoke(t *testing.T, s *Service, root, name string, args map[string]any) httpapi.Envelope {
	t.Helper()
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return s.Call(t.Context(), root, name, b)
}
func object(t *testing.T, result httpapi.Envelope) map[string]any {
	t.Helper()
	if !result.Success {
		t.Fatalf("result=%+v error=%+v", result, result.Error)
	}
	b, _ := json.Marshal(result.Data)
	var data map[string]any
	_ = json.Unmarshal(b, &data)
	return data
}
func code(result httpapi.Envelope) string {
	if result.Error != nil {
		return result.Error.Code
	}
	return ""
}

func TestSchemasRejectMissingRevisionsUnknownPathsAndMalformedValues(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"pages", `{"action":"create","chapter_uuid":"01989abc-def0-7000-8000-000000000001","title":"page","description_md":"text","storyboard_md":"visual"}`},
		{"chapters", `{"action":"write","chapter_uuid":"01989abc-def0-7000-8000-000000000001","content":"text"}`},
		{"project_init", `{"idea":"idea","root_path":"/other"}`},
		{"project_overview", `{"path":"/other"}`},
		{"settings", `{"action":"get","setting_uuid":"1"}`},
		{"book_outline", `{"limit":101}`},
		{"story_profile", `{"action":"write","story_md":"x","expected_revision":null}`},
	} {
		if err := validate(tc.name, json.RawMessage(tc.raw)); err == nil {
			t.Errorf("accepted %s %s", tc.name, tc.raw)
		}
	}
}

func TestMCPSizeLimitIncludesTextAndStructuredContent(t *testing.T) {
	result, err := boundedResult(httpapi.Envelope{Success: true, Data: map[string]any{"content": strings.Repeat("x", MaxBytes/2)}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("combined MCP content exceeded limit")
	}
	wire, err := json.Marshal(result)
	if err != nil || len(wire) > MaxBytes {
		t.Fatalf("unbounded failure: bytes=%d err=%v", len(wire), err)
	}
}

func TestSettingsAndChapterCreationUseParentRevisions(t *testing.T) {
	s, root, _ := harness(t)
	list := object(t, invoke(t, s, root, "settings", map[string]any{"action": "list"}))
	rev := list["premise"].(map[string]any)["revision"]
	input := map[string]any{"action": "create", "asset_type": "scene", "title": "Forest", "summary": "Blue trees and silver rain.", "expected_revision": rev}
	created := object(t, invoke(t, s, root, "settings", input))
	if created["current_variant"] != nil {
		t.Fatal("text-only setting has image")
	}
	if got := code(invoke(t, s, root, "settings", input)); got != "production_conflict" {
		t.Fatalf("stale create=%s", got)
	}
	updated := object(t, invoke(t, s, root, "settings", map[string]any{"action": "update", "setting_uuid": created["uuid"], "summary": "Green trees.", "expected_revision": created["revision"]}))
	if got := code(invoke(t, s, root, "settings", map[string]any{"action": "delete", "setting_uuid": created["uuid"], "expected_revision": created["revision"]})); got != "production_conflict" {
		t.Fatalf("stale delete=%s", got)
	}
	object(t, invoke(t, s, root, "settings", map[string]any{"action": "delete", "setting_uuid": created["uuid"], "expected_revision": updated["revision"]}))
	list = object(t, invoke(t, s, root, "settings", map[string]any{"action": "list"}))
	if len(list["items"].([]any)) != 0 {
		t.Fatalf("deleted setting still listed: %v", list)
	}
	p := object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)
	ch := object(t, invoke(t, s, root, "chapters", map[string]any{"action": "create", "chapter_code": "vol01.ch01", "title": "one", "expected_revision": p["revision"]}))
	if got := code(invoke(t, s, root, "chapters", map[string]any{"action": "create", "chapter_code": "vol01.ch02", "title": "two", "expected_revision": p["revision"]})); got != "project_revision_conflict" {
		t.Fatalf("stale chapter plan=%s", got)
	}
	object(t, invoke(t, s, root, "chapters", map[string]any{"action": "delete", "chapter_uuid": ch["uuid"], "expected_revision": ch["revision"]}))
}

func TestRenderedPromptsUseEffectiveCatalogAndPreserveAuthoritativeFacts(t *testing.T) {
	s, root, uuid := harness(t)
	profile := object(t, invoke(t, s, root, "story_profile", map[string]any{"action": "get"}))
	object(t, invoke(t, s, root, "story_profile", map[string]any{"action": "write", "story_md": "# A real project story", "expected_revision": profile["revision"]}))
	err := s.Projects.WithStore(t.Context(), uuid, func(store *project.Store) error {
		_, err := story.NewService(store).CreatePromptVersion(t.Context(), story.CreatePromptInput{PromptGroup: "story", PromptKey: "story_profile", Prompt: "CUSTOM CATALOG TASK {{input_prompt}}; count {{chapter_count}}. Return story_md and chapter_plans.", ExpectedCurrentVersion: 1})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"prompt_group": "story", "prompt_key": "story_profile", "target": "story_profile", "variables": map[string]string{"input_prompt": "Find the moon", "chapter_count": "2", "story_md": "forged story"}}
	rendered := object(t, invoke(t, s, root, "render_prompt", input))
	prompt := rendered["prompt"].(string)
	for _, text := range []string{"CUSTOM CATALOG TASK", "Find the moon", "Return story_md and chapter_plans", "A real project story", uuid, rendered["target_uuid"].(string), "expected_revision"} {
		if !strings.Contains(prompt, text) {
			t.Errorf("missing %q", text)
		}
	}
	delete(input["variables"].(map[string]string), "chapter_count")
	if got := code(invoke(t, s, root, "render_prompt", input)); got != "prompt_inputs_missing" {
		t.Fatalf("missing placeholder=%s", got)
	}
}

func TestBoundedResultsAndSmallerReadRecovery(t *testing.T) {
	s, root, _ := harness(t)
	p := object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)
	for i, chCode := range []string{"vol01.ch01", "vol01.ch02", "vol01.ch03"} {
		if i > 0 {
			p = object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)
		}
		object(t, invoke(t, s, root, "chapters", map[string]any{"action": "create", "chapter_code": chCode, "title": "Long chapter", "content": strings.Repeat("x", 100000), "expected_revision": p["revision"]}))
	}
	if got := code(invoke(t, s, root, "chapters", map[string]any{"action": "list"})); got != "result_too_large" {
		t.Fatalf("unbounded result=%s", got)
	}
	page := object(t, invoke(t, s, root, "chapters", map[string]any{"action": "list", "limit": 1}))
	if len(page["items"].([]any)) != 1 {
		t.Fatalf("page=%v", page)
	}
	renderInput := map[string]any{"prompt_group": "story", "prompt_key": "story_profile", "target": "story_profile", "variables": map[string]string{"chapter_count": "1"}}
	if got := code(invoke(t, s, root, "render_prompt", renderInput)); got != "result_too_large" {
		t.Fatalf("unbounded prompt=%s", got)
	}
	renderInput["context_chapter_uuids"] = []string{}
	rendered := object(t, invoke(t, s, root, "render_prompt", renderInput))
	if len(rendered["prompt"].(string)) > MaxBytes {
		t.Fatal("narrowed prompt still too large")
	}
	if got := code(s.Call(t.Context(), root, "project_overview", json.RawMessage(strings.Repeat("x", MaxBytes+1)))); got != "arguments_too_large" {
		t.Fatalf("oversized input=%s", got)
	}
	if got := code(invoke(t, s, root, "book_outline", map[string]any{"after": "1", "before": "2"})); got != "invalid_arguments" {
		t.Fatalf("ambiguous cursor=%s", got)
	}
}
