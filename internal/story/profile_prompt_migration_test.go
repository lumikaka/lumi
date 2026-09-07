package story

import (
	"context"
	"strings"
	"testing"

	"lumi/internal/project"
	"lumi/internal/promptcatalog"
)

func TestProfileFromChaptersPromptMigrationPreservesUserChoices(t *testing.T) {
	for _, language := range []string{promptcatalog.LanguageChinese, promptcatalog.LanguageEnglish} {
		for _, format := range []string{"vertical_strip", "classic_picture_book"} {
			for _, custom := range []bool{false, true} {
				name := language + "/" + format + "/builtin"
				if custom {
					name = language + "/" + format + "/custom"
				}
				t.Run(name, func(t *testing.T) {
					manager, _, service := storyHarness(t)
					ctx := context.Background()
					if format == "classic_picture_book" {
						created, err := manager.CreateWithInput(ctx, project.CreateInput{
							Name: "Picture Book Prompt Migration", GenerationLanguage: language,
							PictureBook: &project.PictureBookInput{Format: format, AspectRatio: &project.AspectRatioInput{Mode: project.AspectCustom, Width: 4, Height: 3}},
						}, project.ExplicitNewProjectParent(t.TempDir()))
						if err != nil {
							t.Fatal(err)
						}
						if err := manager.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
							service = NewService(store)
							return nil
						}); err != nil {
							t.Fatal(err)
						}
					}
					if err := service.store.DB().Exec("UPDATE projects SET generation_language=?", language).Error; err != nil {
						t.Fatal(err)
					}
					definition, old := replacePromptWithPreviousBuiltin(t, service, promptcatalog.GroupStory, "profile_from_chapters")
					previous := strings.ReplaceAll(strings.TrimSpace(definition.DefaultValue), "- 本任务只重建故事总纲，不规划或创建章节。chapter_plans 必须严格返回 []，不得填入已有章节或新增章节计划。", "- chapter_plans 可为空数组")
					previous = strings.ReplaceAll(previous, "- This task only reconstructs the story profile; it does not plan or create chapters. chapter_plans must be exactly [], with no existing chapters or new chapter plans.", "- chapter_plans may be an empty array")
					if old.Prompt != previous {
						t.Fatal("previous builtin does not preserve the project's picture-book directive")
					}
					if custom {
						if err := service.EnsurePromptCatalogVersions(ctx, "migration"); err != nil {
							t.Fatal(err)
						}
						if _, err := service.CreatePromptVersion(ctx, CreatePromptInput{PromptGroup: promptcatalog.GroupStory, PromptKey: "profile_from_chapters", Prompt: old.Prompt, ExpectedCurrentVersion: 2}); err != nil {
							t.Fatal(err)
						}
					}
					for range 2 {
						if err := service.EnsurePromptCatalogVersions(ctx, "migration"); err != nil {
							t.Fatal(err)
						}
					}
					versions, pagination, err := service.ListPromptVersions(ctx, promptcatalog.GroupStory, "profile_from_chapters", 1, 20)
					if err != nil {
						t.Fatal(err)
					}
					if custom {
						if pagination.Total != 3 || versions[0].Prompt != old.Prompt {
							t.Fatalf("changed user choice: %+v", versions)
						}
						return
					}
					if pagination.Total != 2 || versions[0].Prompt != strings.TrimSpace(definition.DefaultValue) || versions[1].Prompt != old.Prompt {
						t.Fatalf("migration did not preserve history or was duplicated: %+v", versions)
					}
					required := "chapter_plans 必须严格返回 []"
					if language == promptcatalog.LanguageEnglish {
						required = "chapter_plans must be exactly []"
					}
					if !strings.Contains(versions[0].Prompt, required) {
						t.Fatalf("missing strict output rule: %s", versions[0].Prompt)
					}
				})
			}
		}
	}
}
