package story

import (
	"context"
	"strings"
	"testing"

	"lumi/internal/project"
	"lumi/internal/promptcatalog"
)

func TestCoverArtworkPromptMigratesBuiltinAndPreservesUserChoice(t *testing.T) {
	for _, language := range []string{promptcatalog.LanguageChinese, promptcatalog.LanguageEnglish} {
		t.Run(language, func(t *testing.T) {
			manager, _, service := storyHarness(t)
			ctx := context.Background()
			created, err := manager.CreateWithInput(ctx, project.CreateInput{
				Name: "Cover Prompt Migration", GenerationLanguage: language,
				PictureBook: &project.PictureBookInput{Format: project.PictureBookClassic},
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
			const key = "cover_before_image"
			definition, old := replacePromptWithPreviousBuiltin(t, service, promptcatalog.GroupChapter, key)
			for range 2 {
				if err := service.EnsurePromptCatalogVersions(ctx, "migration"); err != nil {
					t.Fatal(err)
				}
			}
			versions, pagination, err := service.ListPromptVersions(ctx, promptcatalog.GroupChapter, key, 1, 20)
			if err != nil || pagination.Total != 2 || len(versions) != 2 {
				t.Fatalf("migration history=%+v error=%v", versions, err)
			}
			if versions[0].SourceType != "migration" || versions[0].Prompt != strings.TrimSpace(definition.DefaultValue) || versions[1].Prompt != old.Prompt {
				t.Fatalf("migration did not upgrade the builtin while preserving history: %+v", versions)
			}
			// Explicitly choosing the older text must opt out of automatic upgrades.
			if _, err := service.CreatePromptVersion(ctx, CreatePromptInput{
				PromptGroup: promptcatalog.GroupChapter, PromptKey: key,
				Prompt: old.Prompt, ExpectedCurrentVersion: 2,
			}); err != nil {
				t.Fatal(err)
			}
			if err := service.EnsurePromptCatalogVersions(ctx, "migration"); err != nil {
				t.Fatal(err)
			}
			versions, pagination, err = service.ListPromptVersions(ctx, promptcatalog.GroupChapter, key, 1, 20)
			if err != nil || pagination.Total != 3 || len(versions) != 3 || versions[0].Prompt != old.Prompt {
				t.Fatalf("user choice was overwritten: %+v error=%v", versions, err)
			}
		})
	}
}
