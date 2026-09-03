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

func TestPremiseMachineCuttingPromptsMatchRuntimeContract(t *testing.T) {
	tests := []struct {
		language              string
		settingRequired       []string
		settingForbidden      []string
		breakdownRequired     []string
		breakdownForbidden    []string
		previousSettingMark   string
		previousBreakdownMark string
	}{
		{
			language:              LanguageChinese,
			settingRequired:       []string{"1 到 6 个", "纯白 #FFFFFF", "3 列 × 2 行", "中央 76%", "四周各保留至少 12%", "禁止所有可见文字"},
			settingForbidden:      []string{"6 到 12 个", "最多可以到 16 个", "必须在主体附近放一个简短标题"},
			breakdownRequired:     []string{"最终可用框", "不会自动添加 padding", "四边各外扩", "5%", "confidence >= 0.92", "不要依赖图片中的 OCR", `"type": "character"`},
			breakdownForbidden:    []string{`"plan": {`, `"tool_options": {`, `"quality_checks":`, `"filename":`, "表示该设定项在整张图中的大致区域", "无法判断时给空对象"},
			previousSettingMark:   "漫画项目的设定图设计师",
			previousBreakdownMark: "漫画制作资产整理员",
		},
		{
			language:              LanguageEnglish,
			settingRequired:       []string{"1 to 6", "pure white #FFFFFF", "3-column × 2-row", "central 76%", "at least 12%", "No visible text"},
			settingForbidden:      []string{"between 6 and 12", "at most 16", "must have a short title near the subject"},
			breakdownRequired:     []string{"final usable box", "does not add padding", "Expand every side", "5%", "confidence >= 0.92", "Do not rely on OCR", `"type": "character"`},
			breakdownForbidden:    []string{`"plan": {`, `"tool_options": {`, `"quality_checks":`, `"filename":`, "represents the approximate region", "if impossible to judge, return an empty object"},
			previousSettingMark:   "setting image designer",
			previousBreakdownMark: "comic-production asset organizer",
		},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			setting, ok := Lookup(GroupPremise, "setting_image", test.language)
			if !ok {
				t.Fatal("missing setting_image definition")
			}
			breakdown, ok := Lookup(GroupPremise, "asset_breakdown", test.language)
			if !ok {
				t.Fatal("missing asset_breakdown definition")
			}
			if got, want := Placeholders(setting.DefaultValue), []string{"input_text", "style_prompt"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("setting_image placeholders=%v want=%v", got, want)
			}
			if got, want := Placeholders(breakdown.DefaultValue), []string{"image_info_json", "input_text", "style_name", "style_prompt"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("asset_breakdown placeholders=%v want=%v", got, want)
			}
			for _, pair := range []struct {
				name       string
				definition Definition
				mark       string
			}{
				{name: "setting_image", definition: setting, mark: test.previousSettingMark},
				{name: "asset_breakdown", definition: breakdown, mark: test.previousBreakdownMark},
			} {
				if len(pair.definition.PreviousDefaultValues) != 1 {
					t.Fatalf("%s previous defaults=%d want=1", pair.name, len(pair.definition.PreviousDefaultValues))
				}
				previous := pair.definition.PreviousDefaultValues[0]
				if previous == pair.definition.DefaultValue || !strings.Contains(previous, pair.mark) {
					t.Fatalf("%s previous default is not the expected legacy builtin", pair.name)
				}
				if !reflect.DeepEqual(Placeholders(previous), Placeholders(pair.definition.DefaultValue)) {
					t.Fatalf("%s previous placeholders=%v current=%v", pair.name, Placeholders(previous), Placeholders(pair.definition.DefaultValue))
				}
			}
			for _, required := range test.settingRequired {
				if !strings.Contains(setting.DefaultValue, required) {
					t.Errorf("setting_image missing %q", required)
				}
			}
			for _, forbidden := range test.settingForbidden {
				if strings.Contains(setting.DefaultValue, forbidden) {
					t.Errorf("setting_image retained obsolete rule %q", forbidden)
				}
			}
			for _, required := range test.breakdownRequired {
				if !strings.Contains(breakdown.DefaultValue, required) {
					t.Errorf("asset_breakdown missing %q", required)
				}
			}
			for _, forbidden := range test.breakdownForbidden {
				if strings.Contains(breakdown.DefaultValue, forbidden) {
					t.Errorf("asset_breakdown retained obsolete or unsupported field %q", forbidden)
				}
			}
		})
	}
}

func TestCatalogContainsCanonicalKeys(t *testing.T) {
	want := map[string][]string{
		GroupStory:        {"json_system", "story_profile", "story_chapter", "chapter_batch_plan", "next_story_chapter", "profile_from_chapters"},
		GroupChapter:      {"json_system", "comic_storyboard", "cover_storyboard", "section_premise_selection", "before_image", "cover_before_image", "back_cover_before_image", "section_reference_present", "section_reference_absent", "section_additional_direction", "section_image"},
		GroupPremise:      {"setting_image", "setting_reference_usage", "asset_breakdown", "single_asset_generation"},
		GroupPremiseStyle: {"project_overall_style", "simple_cel_anime", "hong_kong_comic", "minimal_japanese_handdrawn"},
		GroupAgent:        {"base", "conversation_summary"},
		GroupRuntime:      {"project_language_instruction"},
	}
	if definitions := Definitions(LanguageChinese); len(definitions) != 28 {
		t.Fatalf("catalog size = %d, want 28", len(definitions))
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
	coverBefore, _ := Lookup(GroupChapter, "cover_before_image", LanguageChinese)
	backCoverBefore, _ := Lookup(GroupChapter, "back_cover_before_image", LanguageChinese)
	section, _ := Lookup(GroupChapter, "section_image", LanguageChinese)
	cover, _ := Lookup(GroupChapter, "cover_storyboard", LanguageChinese)
	preset, _ := Lookup(GroupPremiseStyle, "simple_cel_anime", LanguageChinese)
	if before.PromptType != PromptTypeFragment || coverBefore.PromptType != PromptTypeFragment || backCoverBefore.PromptType != PromptTypeFragment || preset.PromptType != PromptTypePreset || section.PromptType != PromptTypeTemplate || cover.PromptType != PromptTypeTemplate || !strings.Contains(section.DefaultValue, "{{before_image_prompt}}") || !strings.Contains(cover.DefaultValue, "{{first_body_storyboard}}") {
		t.Fatalf("prompt types or composition are invalid: before=%q cover_before=%q back_cover_before=%q preset=%q section=%q cover=%q", before.PromptType, coverBefore.PromptType, backCoverBefore.PromptType, preset.PromptType, section.PromptType, cover.PromptType)
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
				if len(definition.PreviousDefaultValues) != 8 ||
					definition.PreviousDefaultValues[0] == definition.DefaultValue ||
					(!strings.Contains(definition.PreviousDefaultValues[0], "controlled YOLO flow") && !strings.Contains(definition.PreviousDefaultValues[0], "受控 YOLO")) ||
					(!strings.Contains(definition.PreviousDefaultValues[1], "top-level request_user_input field") && !strings.Contains(definition.PreviousDefaultValues[1], "顶层字段，与 questions 同级")) ||
					strings.Contains(definition.PreviousDefaultValues[1], "agent_tool_confirmation_required") ||
					strings.Contains(definition.PreviousDefaultValues[2], "top-level request_user_input field") || strings.Contains(definition.PreviousDefaultValues[2], "顶层字段，与 questions 同级") ||
					(!strings.Contains(definition.PreviousDefaultValues[3], "end the current Turn immediately") && !strings.Contains(definition.PreviousDefaultValues[3], "立即结束当前 Turn")) ||
					strings.Contains(definition.PreviousDefaultValues[4], "bootstrap first Turn") || strings.Contains(definition.PreviousDefaultValues[4], "bootstrap 首个 Turn") ||
					!strings.Contains(definition.PreviousDefaultValues[4], "ui_ref") || strings.Contains(definition.PreviousDefaultValues[5], "ui_ref") ||
					(!strings.Contains(definition.PreviousDefaultValues[6], "confirming-option index") && !strings.Contains(definition.PreviousDefaultValues[6], "确认选项索引")) ||
					(!strings.Contains(definition.PreviousDefaultValues[7], "workflow or source constraint is uncertain") && !strings.Contains(definition.PreviousDefaultValues[7], "流程或来源约束不确定")) {
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
	wordlessCover, _ := LookupForPictureBook(GroupChapter, "cover_storyboard", LanguageChinese, PictureBookOptions{Format: "wordless_picture_book", AspectWidth: 1, AspectHeight: 1})
	interactiveBackCover, _ := LookupForPictureBook(GroupChapter, "back_cover_before_image", LanguageChinese, PictureBookOptions{Format: "interactive_picture_book", AspectWidth: 4, AspectHeight: 3, InteractionMode: "follow_along"})
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
	if !strings.Contains(wordlessCover.DefaultValue, "正文页的无字") || !strings.Contains(wordlessCover.DefaultValue, "逐字标题") {
		t.Fatal("wordless cover prompt did not separate cover copy from body-page no-text rules")
	}
	if !strings.Contains(interactiveBackCover.DefaultValue, "封底") || !strings.Contains(interactiveBackCover.DefaultValue, "互动提问") || strings.Contains(interactiveBackCover.DefaultValue, "必须展示一个简单、安全") {
		t.Fatal("interactive back-cover prompt did not separate special-page composition from body interaction rules")
	}
}

func TestVerticalStripPromptSuiteSHA256Canary(t *testing.T) {
	expected := map[string]string{
		LanguageChinese: "96236e74e4454ecc735b0111ffabc45940a654e4bfa39f5f5a554f22f2097151",
		LanguageEnglish: "ca16e647af19f844b9cf2ed566fc24aabfd8f93c8d2f7cc9ddd3a82143590f8f",
	}
	for _, language := range []string{LanguageChinese, LanguageEnglish} {
		hasher := sha256.New()
		stripDefinitions := DefinitionsForPictureBook(language, PictureBookOptions{Format: "vertical_strip", AspectWidth: 1, AspectHeight: 3})
		for _, definition := range stripDefinitions {
			// The special-page prompt keys are new picture-book-only capabilities. Keep the
			// canary pinned to the pre-existing vertical-strip suite so adding the
			// unused key does not require blessing changes to protected prompts.
			if (definition.Group == GroupChapter && (definition.Key == "cover_storyboard" || definition.Key == "cover_before_image" || definition.Key == "back_cover_before_image")) || (definition.Group == GroupPremise && definition.Key == "setting_reference_usage") {
				continue
			}
			_, _ = fmt.Fprintf(hasher, "%s\x00%s\x00%s\x00%s\x00", language, definition.Group, definition.Key, definition.DefaultValue)
		}
		got := hex.EncodeToString(hasher.Sum(nil))
		if got != expected[language] {
			t.Fatalf("vertical strip %s prompt suite SHA-256=%s", language, got)
		}
	}
}
