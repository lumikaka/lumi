package textmcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
	"lumi/internal/project"
)

func jsonSchema(t *testing.T, value any) map[string]any {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestModelSchemaProjectionPreservesFieldsAndAuthority(t *testing.T) {
	for _, spec := range Specs() {
		t.Run(spec.Name, func(t *testing.T) {
			before := jsonSchema(t, spec.Schema)
			published, err := modelVisibleSpec(spec)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, jsonSchema(t, spec.Schema)) {
				t.Fatal("projection mutated the authoritative schema")
			}
			want := jsonSchema(t, spec.Schema)
			delete(want, "allOf")
			if !reflect.DeepEqual(want, published.Schema) {
				t.Fatal("projection lost fields, constraints, or base required")
			}
			// The result must not alias maps or arrays shared by input fields.
			published.Schema["properties"].(map[string]any)["example"] = enum("detached")
			if !reflect.DeepEqual(before, jsonSchema(t, spec.Schema)) {
				t.Fatal("published properties alias authoritative properties")
			}
		})
	}
	first, _ := json.Marshal(Specs())
	for range 20 {
		next, _ := json.Marshal(Specs())
		if string(first) != string(next) {
			t.Fatal("schema definition order is unstable")
		}
	}
}

func TestProtocolListsModelSchemasAndKeepsAppValidation(t *testing.T) {
	forwarded := 0
	server := Protocol(func(context.Context, string, json.RawMessage) httpapi.Envelope {
		forwarded++
		return httpapi.Envelope{Success: true}
	})
	a, b := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "schema-contract", Version: "1"}, nil).Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	listed, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	wants := map[string]Spec{}
	for _, spec := range Specs() {
		if spec.Name != "read_app_connection" {
			wants[spec.Name] = spec
		}
	}
	if len(listed.Tools) != len(wants) {
		t.Fatalf("listed %d tools, want %d", len(listed.Tools), len(wants))
	}
	for _, tool := range listed.Tools {
		original, exists := wants[tool.Name]
		if !exists {
			t.Fatalf("unexpected published tool %s", tool.Name)
		}
		want := jsonSchema(t, original.Schema)
		delete(want, "allOf")
		if !reflect.DeepEqual(want, jsonSchema(t, tool.InputSchema)) {
			t.Fatalf("%s: tools/list did not preserve the full object schema", tool.Name)
		}
		for name, clause := range map[string]string{
			"project_setup":     "action=finalize requires expected_revision",
			"story_profile":     "action=write requires expected_revision, story_md",
			"chapters":          "action=create requires chapter_code, expected_revision, title",
			"settings":          "action=create requires asset_type, expected_revision, summary, title",
			"pages":             "action=create requires chapter_uuid, description_md, expected_revision, storyboard_md, title",
			"render_prompt":     "target=page requires chapter_uuid, page_uuid",
			"review_candidates": "kind=text requires candidates",
			"watch_app_changes": "action=next requires cursor, project_uuid, subscription_uuid",
		} {
			if tool.Name == name && !strings.Contains(tool.Description, clause) {
				t.Fatalf("%s: missing conditional guidance: %s", name, tool.Description)
			}
		}
	}
	// This tool bypasses Service.Call. Its own authoritative validation must
	// still reject conditions omitted from the published schema, before I/O.
	for _, args := range []map[string]any{
		{"action": "open"},
		{"action": "next", "project_uuid": "01989abc-def0-7000-8000-000000000001"},
		{"action": "close"},
	} {
		result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "watch_app_changes", Arguments: args})
		if err != nil || result == nil || !result.IsError {
			t.Fatalf("accepted incomplete app watch: result=%+v err=%v", result, err)
		}
		if !strings.Contains(result.Content[0].(*mcp.TextContent).Text, `"code":"invalid_arguments"`) {
			t.Fatal(result)
		}
	}
	if forwarded != 0 {
		t.Fatalf("invalid watch arguments reached I/O: %d", forwarded)
	}
}

func TestModelProjectionRejectsUnrepresentedConditions(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"then constraint": func(s map[string]any) {
			s["allOf"].([]any)[0].(map[string]any)["then"].(map[string]any)["properties"] = map[string]any{}
		},
		"else branch": func(s map[string]any) { s["allOf"].([]any)[0].(map[string]any)["else"] = map[string]any{} },
		"extra predicate": func(s map[string]any) {
			s["allOf"].([]any)[0].(map[string]any)["if"].(map[string]any)["required"] = []string{"action"}
		},
		"not a condition": func(s map[string]any) { s["allOf"] = []any{map[string]any{"required": []string{"expected_revision"}}} },
		"undefined field": func(s map[string]any) {
			s["allOf"].([]any)[0].(map[string]any)["then"] = map[string]any{"required": []string{"undefined"}}
		},
		"optional selector": func(s map[string]any) { s["required"] = []string{} },
		"nested condition": func(s map[string]any) {
			s["properties"].(map[string]any)["nested"] = map[string]any{"type": "object", "if": map[string]any{}}
		},
		"invalid selector type": func(s map[string]any) {
			s["allOf"].([]any)[0].(map[string]any)["if"] = map[string]any{"properties": map[string]any{"action": map[string]any{"type": "number", "enum": []string{"write"}}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			schema := actionSchema(map[string]any{"action": enum("get", "write"), "expected_revision": revision("version")}, map[string][]string{"write": {"expected_revision"}})
			mutate(schema)
			before := jsonSchema(t, schema)
			if _, err := modelVisibleSpec(Spec{Name: "unsupported", Schema: schema}); err == nil {
				t.Fatal("silently published an unsupported constraint")
			}
			if !reflect.DeepEqual(before, jsonSchema(t, schema)) {
				t.Fatal("failed projection mutated input")
			}
		})
	}
}

func TestConditionalValidationRejectsBeforeBusinessAccess(t *testing.T) {
	const uuid = "01989abc-def0-7000-8000-000000000001"
	for _, test := range []struct{ tool, input string }{
		{"project_setup", `{"action":"update","project_name":"x"}`},
		{"project_setup", `{"action":"finalize"}`},
		{"project_setup", `{"action":"update","expected_revision":1,"generation_scope":"project_only"}`},
		{"project_setup", `{"action":"update","expected_revision":1,"picture_book":{"format":"strip"}}`},
		{"project_setup", `{"action":"update","expected_revision":1,"picture_book":{"format":"comic_story","aspect_ratio":{"mode":"custom","width":2,"height":3,"extra":true}}}`},
		{"project_setup", `{"action":"update","expected_revision":1,"picture_book":{"format":"comic_story","aspect_ratio":"2:3"}}`},
		{"story_profile", `{"action":"write","expected_revision":0}`},
		{"story_profile", `{"action":"write","story_md":"x"}`},
		{"story_profile", `{"action":"write","story_md":"x","expected_revision":-1}`},
		{"chapters", `{"action":"create","chapter_code":"vol01.ch01","expected_revision":0}`},
		{"chapters", `{"action":"write","chapter_uuid":"` + uuid + `","content":"x"}`},
		{"settings", `{"action":"create","type":"character","description_md":"x","title":"x","expected_revision":0}`},
		{"settings", `{"action":"create","asset_type":"character","title":"x","expected_revision":0}`},
		{"settings", `{"action":"select_image","setting_uuid":"` + uuid + `","expected_revision":0}`},
		{"pages", `{"action":"create","chapter_uuid":"` + uuid + `","title":"x","description_md":"x","expected_revision":0}`},
		{"pages", `{"action":"create","chapter_uuid":"` + uuid + `","title":"x","description_md":"x","storyboard_md":"x"}`},
		{"pages", `{"action":"select_image","chapter_uuid":"` + uuid + `","page_uuid":"` + uuid + `","expected_revision":0}`},
		{"render_prompt", `{"prompt_group":"story","prompt_key":"story_profile","target":"story"}`},
		{"render_prompt", `{"prompt_group":"chapter","prompt_key":"comic_storyboard","target":"pages"}`},
		{"render_prompt", `{"prompt_group":"chapter","prompt_key":"before_image","target":"page","chapter_uuid":"` + uuid + `"}`},
		{"render_prompt", `{"prompt_group":"premise","prompt_key":"setting_image","target":"setting"}`},
		{"review_candidates", `{"kind":"page","page_uuid":"` + uuid + `"}`},
		{"review_candidates", `{"kind":"setting"}`},
		{"review_candidates", `{"kind":"text"}`},
		{"review_candidates", `{"kind":"text","candidates":[{"title":"a","text":"a"},{"title":"b"}]}`},
	} {
		t.Run(test.tool+"/"+test.input, func(t *testing.T) {
			root := t.TempDir()
			// A nil Manager would panic if validation let any request reach
			// project access. It must not initialize or modify business state.
			result := (&Service{}).Call(t.Context(), root, test.tool, json.RawMessage(test.input))
			if code(result) != "invalid_arguments" {
				t.Fatalf("invalid request accepted: %+v", result)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid request touched project: %v %v", entries, err)
			}
		})
	}
}

func TestBundledSkillExamplesMatchAuthoritativeContract(t *testing.T) {
	path := filepath.Join("..", "..", "codex-plugins", "lumi", "skills", "create-picture-book", "references", "mcp-usage.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```json\\n(.*?)\\n```").FindAllSubmatch(content, -1)
	if len(blocks) == 0 {
		t.Fatal("no executable documentation examples")
	}
	for _, block := range blocks {
		var example struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(block[1], &example); err != nil {
			t.Fatal(err)
		}
		if err := validate(example.Name, example.Arguments); err != nil {
			t.Fatalf("%s example: %v", example.Name, err)
		}
		var args arguments
		if err := json.Unmarshal(example.Arguments, &args); err != nil {
			t.Fatal(err)
		}
		if args.PictureBook != nil {
			if _, err := project.NormalizePictureBookInput(args.PictureBook); err != nil {
				t.Fatalf("invalid documented picture book: %v", err)
			}
		}
	}
}
