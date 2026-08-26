package promptcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	agentprompts "lumi/internal/agent/prompts"
	"reflect"
	"strings"
	"testing"
)

func TestCatalogLanguagesHaveIdenticalKeysGroupsAndPlaceholders(t *testing.T) {
	chinese := Definitions(LanguageChinese)
	english := Definitions(LanguageEnglish)
	if len(chinese) != len(english) {
		t.Fatalf("catalog size drift: zh-Hans=%d en=%d", len(chinese), len(english))
	}
	for index := range chinese {
		zh := chinese[index]
		en := english[index]
		if zh.Group != en.Group || zh.Key != en.Key {
			t.Fatalf("catalog identity drift at %d: zh=%s/%s en=%s/%s", index, zh.Group, zh.Key, en.Group, en.Key)
		}
		if !reflect.DeepEqual(Placeholders(zh.DefaultValue), Placeholders(en.DefaultValue)) {
			t.Fatalf("placeholder drift for %s/%s: zh=%v en=%v", zh.Group, zh.Key, Placeholders(zh.DefaultValue), Placeholders(en.DefaultValue))
		}
		if strings.TrimSpace(zh.DefaultValue) == "" || strings.TrimSpace(en.DefaultValue) == "" {
			t.Fatalf("blank default for %s/%s", zh.Group, zh.Key)
		}
	}
}

func TestCatalogContainsCanonicalKeys(t *testing.T) {
	want := map[string][]string{
		GroupStory:        {"json_system", "story_profile", "story_chapter", "chapter_batch_plan", "next_story_chapter", "profile_from_chapters"},
		GroupChapter:      {"json_system", "comic_storyboard", "section_premise_selection", "before_image", "section_reference_present", "section_reference_absent", "section_additional_direction", "section_image"},
		GroupPremise:      {"setting_image", "asset_breakdown", "single_asset_generation"},
		GroupPremiseStyle: {"project_overall_style", "simple_cel_anime", "hong_kong_comic", "minimal_japanese_handdrawn"},
		GroupAgent:        {"base", "conversation_summary"},
		GroupRuntime:      {"project_language_instruction"},
	}
	if definitions := Definitions(LanguageChinese); len(definitions) != 24 {
		t.Fatalf("catalog size = %d, want 24", len(definitions))
	}
	for group, keys := range want {
		for _, key := range keys {
			if _, ok := Lookup(group, key, LanguageChinese); !ok {
				t.Errorf("missing zh-Hans %s/%s", group, key)
			}
			if _, ok := Lookup(group, key, LanguageEnglish); !ok {
				t.Errorf("missing en %s/%s", group, key)
			}
		}
	}
	before, _ := Lookup(GroupChapter, "before_image", LanguageChinese)
	section, _ := Lookup(GroupChapter, "section_image", LanguageChinese)
	preset, _ := Lookup(GroupPremiseStyle, "simple_cel_anime", LanguageChinese)
	if before.PromptType != PromptTypeFragment || preset.PromptType != PromptTypePreset || section.PromptType != PromptTypeTemplate || !strings.Contains(section.DefaultValue, "{{before_image_prompt}}") {
		t.Fatalf("prompt types or composition are invalid: before=%q preset=%q section=%q", before.PromptType, preset.PromptType, section.PromptType)
	}
}

func TestAgentPromptDefinitionsUseEmbeddedCurrentDefaults(t *testing.T) {
	keys := []string{"base", "conversation_summary"}
	values := map[string]string{
		"project_uuid":       "01900000-0000-7000-8000-000000000001",
		"subject_uuid":       "01900000-0000-7000-8000-000000000002",
		"asset_type":         "character",
		"asset_title":        "Courier",
		"asset_summary":      "A moonlit courier",
		"asset_tags":         `["courier"]`,
		"current_file_uuid":  "01900000-0000-7000-8000-000000000003",
		"asset_revision":     "2",
		"overall_style":      "Simple cel animation",
		"chapter_uuid":       "01900000-0000-7000-8000-000000000004",
		"section_uuid":       "01900000-0000-7000-8000-000000000005",
		"recommended_guides": "- /api/v1/agent-docs/guides/example.md",
		"summary":            "The current project facts.",
	}
	for _, language := range []string{LanguageChinese, LanguageEnglish} {
		for _, key := range keys {
			definition, ok := Lookup(GroupAgent, key, language)
			if !ok {
				t.Fatalf("missing %s Agent prompt %s", language, key)
			}
			if definition.DefaultValue != agentprompts.MustRead(key, language) {
				t.Fatalf("%s Agent prompt %s does not use the embedded default", language, key)
			}
			if key == "base" {
				if len(definition.PreviousDefaultValues) != 1 || definition.PreviousDefaultValues[0] == definition.DefaultValue || !strings.Contains(definition.PreviousDefaultValues[0], "workflow or source constraint is uncertain") && !strings.Contains(definition.PreviousDefaultValues[0], "流程或来源约束不确定") {
					t.Fatalf("%s Agent prompt %s previous defaults=%v", language, key, definition.PreviousDefaultValues)
				}
			} else if len(definition.PreviousDefaultValues) != 0 {
				t.Fatalf("%s Agent prompt %s unexpectedly has previous defaults=%v", language, key, definition.PreviousDefaultValues)
			}
			rendered, err := Render(definition.DefaultValue, values)
			if err != nil || strings.Contains(rendered, "{{") {
				t.Fatalf("render %s Agent prompt %s: rendered=%q err=%v", language, key, rendered, err)
			}
		}
	}
}

func TestRenderRejectsMissingPlaceholdersAndLeavesNone(t *testing.T) {
	if _, err := Render("hello {{name}} {{place}}", map[string]string{"name": "Lumi"}); err == nil || !strings.Contains(err.Error(), "place") {
		t.Fatalf("expected missing placeholder error, got %v", err)
	}
	rendered, err := Render("hello {{name}}", map[string]string{"name": "Lumi"})
	if err != nil {
		t.Fatal(err)
	}
	if rendered != "hello Lumi" || len(Placeholders(rendered)) != 0 {
		t.Fatalf("unexpected rendered prompt %q", rendered)
	}
}

func TestPlaceholdersIncludeUnsupportedMustacheNames(t *testing.T) {
	items := Placeholders("{{valid_name}} {{not-supported}} {{ spaced }}")
	if !reflect.DeepEqual(items, []string{" spaced ", "not-supported", "valid_name"}) {
		t.Fatalf("placeholders=%v", items)
	}
}

func TestLanguageInstructionMatchesGenerationContract(t *testing.T) {
	if !strings.Contains(LanguageInstruction(LanguageChinese), "项目语言：简体中文") || !strings.Contains(LanguageInstruction(LanguageChinese), "JSON key") {
		t.Fatal("Chinese language instruction lost required constraints")
	}
	if !strings.Contains(LanguageInstruction(LanguageEnglish), "Project language: English") || !strings.Contains(LanguageInstruction(LanguageEnglish), "JSON keys") {
		t.Fatal("English language instruction lost required constraints")
	}
}

func TestVerticalStripPictureBookSuiteRemainsByteForByteUnchanged(t *testing.T) {
	for _, language := range []string{LanguageChinese, LanguageEnglish} {
		protectedSuite := Definitions(language)
		strip := DefinitionsForPictureBook(language, PictureBookOptions{Format: "vertical_strip", AspectWidth: 1, AspectHeight: 3})
		if !reflect.DeepEqual(strip, protectedSuite) {
			t.Fatalf("%s vertical strip catalog changed", language)
		}
	}
}

func TestPictureBookSuitesKeepCanonicalKeysAndPlaceholders(t *testing.T) {
	options := []PictureBookOptions{
		{Format: "classic_picture_book", AspectWidth: 4, AspectHeight: 3},
		{Format: "classic_picture_book", AspectWidth: 3, AspectHeight: 4, LargeImageMinimalText: true},
		{Format: "wordless_picture_book", AspectWidth: 1, AspectHeight: 1},
		{Format: "interactive_picture_book", AspectWidth: 4, AspectHeight: 3, InteractionMode: "guess"},
		{Format: "comic_story", AspectWidth: 4, AspectHeight: 3, ComicLayout: "four_panel"},
		{Format: "comic_story", AspectWidth: 3, AspectHeight: 4, ComicLayout: "page_comic"},
	}
	for _, language := range []string{LanguageChinese, LanguageEnglish} {
		baseline := Definitions(language)
		for _, option := range options {
			definitions := DefinitionsForPictureBook(language, option)
			if len(definitions) != len(baseline) {
				t.Fatalf("%s %s definitions=%d", language, option.Format, len(definitions))
			}
			for index := range baseline {
				if definitions[index].Group != baseline[index].Group || definitions[index].Key != baseline[index].Key {
					t.Fatalf("%s %s identity drift at %d", language, option.Format, index)
				}
				if !reflect.DeepEqual(Placeholders(definitions[index].DefaultValue), Placeholders(baseline[index].DefaultValue)) {
					t.Fatalf("%s %s placeholder drift for %s/%s: got=%v want=%v", language, option.Format, baseline[index].Group, baseline[index].Key, Placeholders(definitions[index].DefaultValue), Placeholders(baseline[index].DefaultValue))
				}
			}
		}
	}
}

func TestPictureBookPromptOptionsAffectTheResolvedSuite(t *testing.T) {
	classic, _ := LookupForPictureBook(GroupChapter, "before_image", LanguageChinese, PictureBookOptions{Format: "classic_picture_book", AspectWidth: 4, AspectHeight: 3})
	minimal, _ := LookupForPictureBook(GroupChapter, "before_image", LanguageChinese, PictureBookOptions{Format: "classic_picture_book", AspectWidth: 4, AspectHeight: 3, LargeImageMinimalText: true})
	wordless, _ := LookupForPictureBook(GroupChapter, "before_image", LanguageChinese, PictureBookOptions{Format: "wordless_picture_book", AspectWidth: 1, AspectHeight: 1})
	interactive, _ := LookupForPictureBook(GroupChapter, "before_image", LanguageChinese, PictureBookOptions{Format: "interactive_picture_book", AspectWidth: 4, AspectHeight: 3, InteractionMode: "follow_along"})
	fourPanel, _ := LookupForPictureBook(GroupChapter, "comic_storyboard", LanguageEnglish, PictureBookOptions{Format: "comic_story", AspectWidth: 3, AspectHeight: 4, ComicLayout: "four_panel"})
	if !strings.Contains(classic.DefaultValue, "1–3 句") || !strings.Contains(minimal.DefaultValue, "0–1 句") {
		t.Fatal("classic minimal-text option did not change the page prompt")
	}
	if !strings.Contains(wordless.DefaultValue, "绝对禁止任何可读文字") {
		t.Fatal("wordless prompt lost its no-text contract")
	}
	if !strings.Contains(interactive.DefaultValue, "跟着做") || !strings.Contains(interactive.DefaultValue, "一个简单、安全") {
		t.Fatal("interactive mode did not affect its page prompt")
	}
	if !strings.Contains(fourPanel.DefaultValue, "exactly four sequential comic panels") || !strings.Contains(fourPanel.DefaultValue, "one section is one page") {
		t.Fatal("four-panel prompt lost panel or page planning rules")
	}
}

func TestVerticalStripPromptSuiteSHA256Canary(t *testing.T) {
	expected := map[string]string{
		LanguageChinese: "b31fe57d1b824c27bfa384ddf729d2cd68eefa11d6e56f604f9917d225238bee",
		LanguageEnglish: "e9ebd37f170be6b6470f35b915d0770ee0e88a7cc9ce0677d3c229b64bb61aba",
	}
	for _, language := range []string{LanguageChinese, LanguageEnglish} {
		hasher := sha256.New()
		stripDefinitions := DefinitionsForPictureBook(language, PictureBookOptions{Format: "vertical_strip", AspectWidth: 1, AspectHeight: 3})
		for _, definition := range stripDefinitions {
			_, _ = fmt.Fprintf(hasher, "%s\x00%s\x00%s\x00%s\x00", language, definition.Group, definition.Key, definition.DefaultValue)
		}
		got := hex.EncodeToString(hasher.Sum(nil))
		if got != expected[language] {
			t.Fatalf("vertical strip %s prompt suite SHA-256=%s", language, got)
		}
	}
}
