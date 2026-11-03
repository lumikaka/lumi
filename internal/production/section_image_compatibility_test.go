package production_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"lumi/internal/production"
	"lumi/internal/promptcatalog"
)

// Baselines were captured from Web code at 7d2a7d1e968c29cd5b209d2e34a7068cc3679288.
// They include custom language instructions and user direction, for every
// role/language/reference state and all supported durable snapshot versions.
func TestSectionImagePromptMatchesPreExtractionGolden(t *testing.T) {
	data, err := os.ReadFile("testdata/section_image_prompt_sha256.json")
	if err != nil {
		t.Fatal(err)
	}
	golden := map[string]string{}
	if err = json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"en", "zh-Hans"} {
		for _, role := range []string{"body", "front_cover", "back_cover"} {
			for _, version := range []int{1, 2, 3, 4, 5} {
				for _, count := range []int{0, 1, 4} {
					t.Run(fmt.Sprintf("%s/%s/v%d/refs%d", language, role, version, count), func(t *testing.T) {
						lookup := func(group, key string) string {
							d, ok := promptcatalog.Lookup(group, key, language)
							if !ok {
								t.Fatalf("unknown %s/%s", group, key)
							}
							return d.DefaultValue
						}
						key := "before_image"
						if role == "front_cover" {
							key = "cover_before_image"
						}
						if role == "back_cover" {
							key = "back_cover_before_image"
						}
						snapshot := production.GenerationSnapshot{Version: version, GenerationLanguage: language, ResourceUUID: "01989abc-def0-7000-8000-000000000001", Prompt: "  保留完整对白；右侧增加一盏灯。  ", StoryboardMD: "  已保存分镜：狐狸说：‘天亮了！’  ", StyleSnapshot: "CUSTOM PROJECT EFFECTIVE STYLE", LanguageInstruction: "CUSTOM LANGUAGE INSTRUCTION", PromptTemplate: strings.ReplaceAll(lookup("chapter", "section_image"), "{{before_image_prompt}}", lookup("chapter", key)), ReferencePresentPrompt: lookup("chapter", "section_reference_present"), ReferenceAbsentPrompt: lookup("chapter", "section_reference_absent"), AdditionalDirectionPrompt: lookup("chapter", "section_additional_direction")}
						refs := make([]production.PremiseAssetReference, count)
						for i := range refs {
							refs[i].Title = fmt.Sprintf("小狐狸 %d", i+1)
						}
						after, err := production.RenderSectionImagePrompt(snapshot, refs)
						if err != nil {
							t.Fatal(err)
						}
						key = fmt.Sprintf("%s/%s/v%d/refs%d", language, role, version, count)
						hash := fmt.Sprintf("%x", sha256.Sum256([]byte(after)))
						if hash != golden[key] {
							t.Fatalf("prompt bytes changed from pre-extraction Web baseline: got %s want %s", hash, golden[key])
						}
						if version >= 2 && (!strings.Contains(after, "CUSTOM LANGUAGE INSTRUCTION") || !strings.Contains(after, "保留完整对白")) {
							t.Fatal("missing language or guidance")
						}
					})
				}
			}
		}
	}
}
