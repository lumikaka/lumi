package modelsettings

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
)

type settingsHarness struct {
	ctx       context.Context
	app       *appstore.Store
	projects  *project.Manager
	providers *provider.Service
	store     *project.Store
	cloud     provider.Provider
	bailian   provider.Provider
}

func newSettingsHarness(t *testing.T) *settingsHarness {
	t.Helper()
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	cloud, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "cloud/text", DefaultImageModel: "cloud/image", APIKey: "cloud-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := providers.Settings().Update(ctx, map[string]any{
		sitesettings.BailianWorkspaceKey: "workspace-1",
		sitesettings.BailianRegionKey:    "cn-beijing",
		sitesettings.BailianAPIKeyKey:    "bailian-secret",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := providers.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bailian, err := providers.MarkVerified(ctx, items[1].UUID)
	if err != nil {
		t.Fatal(err)
	}
	projects := project.NewManager(app)
	created, err := projects.Create(ctx, "Model settings", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var store *project.Store
	if err := projects.WithCurrentStore(ctx, created.UUID, func(current *project.Store) error { store = current; return nil }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = projects.Close(); providers.Close(); _ = app.Close() })
	return &settingsHarness{ctx: ctx, app: app, projects: projects, providers: providers, store: store, cloud: cloud, bailian: bailian}
}

func TestProjectAndScenarioModelInheritance(t *testing.T) {
	h := newSettingsHarness(t)
	resolver := NewResolver(h.providers)
	view, err := resolver.Get(h.ctx, h.store)
	if err != nil {
		t.Fatal(err)
	}
	if view.Revision != 0 || view.Settings[StoryText].Source != SourceGlobalDefault || view.Settings[StoryText].Effective.Model != "cloud/text" {
		t.Fatalf("initial view=%+v", view)
	}
	view, err = resolver.Patch(h.ctx, h.store, PatchInput{ExpectedRevision: 0, Changes: map[string]*Selection{
		ProjectText: {ProviderUUID: h.bailian.UUID, Model: provider.BailianTextModel},
		StoryText:   {ProviderUUID: h.cloud.UUID, Model: "cloud/text"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if view.Revision != 1 || view.Settings[ChatArea].Source != SourceProjectTextOverride || view.Settings[ChatArea].Effective.ProviderUUID != h.bailian.UUID || view.Settings[StoryText].Source != SourceScenarioOverride || view.Settings[StoryText].Effective.ProviderUUID != h.cloud.UUID {
		t.Fatalf("overridden view=%+v", view.Settings)
	}
	resolved, err := resolver.Resolve(h.ctx, h.store, StoryText, KindText, "", "")
	if err != nil || resolved.Source != SourceScenarioOverride || resolved.Model != "cloud/text" {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	cleared, err := resolver.Patch(h.ctx, h.store, PatchInput{ExpectedRevision: 1, Changes: map[string]*Selection{StoryText: nil}})
	if err != nil || cleared.Settings[StoryText].Source != SourceProjectTextOverride || cleared.Settings[StoryText].Effective.ProviderUUID != h.bailian.UUID {
		t.Fatalf("cleared=%+v err=%v", cleared.Settings[StoryText], err)
	}
	if _, err := resolver.Patch(h.ctx, h.store, PatchInput{ExpectedRevision: 1, Changes: map[string]*Selection{ChatArea: nil}}); errorCode(err) != CodeConflict {
		t.Fatalf("stale revision error=%v", err)
	}
}

func TestBailianQwen38MaxIsSelectable(t *testing.T) {
	h := newSettingsHarness(t)
	resolver := NewResolver(h.providers)
	view, err := resolver.Get(h.ctx, h.store)
	if err != nil {
		t.Fatal(err)
	}
	models := make([]string, 0, 2)
	for _, option := range view.Options.TextModels {
		if option.ProviderUUID == h.bailian.UUID {
			models = append(models, option.Model)
		}
	}
	if len(models) != 2 || models[0] != provider.BailianTextModel || models[1] != provider.BailianTextModelQwen38Max {
		t.Fatalf("Bailian text models=%v", models)
	}

	view, err = resolver.Patch(h.ctx, h.store, PatchInput{ExpectedRevision: 0, Changes: map[string]*Selection{
		ProjectText: {ProviderUUID: h.bailian.UUID, Model: provider.BailianTextModelQwen38Max},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if effective := view.Settings[ProjectText].Effective; effective == nil || effective.ProviderUUID != h.bailian.UUID || effective.Model != provider.BailianTextModelQwen38Max {
		t.Fatalf("effective project text model=%+v", effective)
	}
	resolved, err := resolver.Resolve(h.ctx, h.store, ProjectText, KindText, "", "")
	if err != nil || resolved.Model != provider.BailianTextModelQwen38Max {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
}

func TestUnavailableAndMismatchedModelsAreSafe(t *testing.T) {
	h := newSettingsHarness(t)
	resolver := NewResolver(h.providers)
	if _, err := resolver.Patch(h.ctx, h.store, PatchInput{ExpectedRevision: 0, Changes: map[string]*Selection{
		ProjectImage: {ProviderUUID: h.cloud.UUID, Model: "cloud/text"},
	}}); errorCode(err) != CodeInvalid {
		t.Fatalf("text model accepted for image setting: %v", err)
	}
	view, err := resolver.Patch(h.ctx, h.store, PatchInput{ExpectedRevision: 0, Changes: map[string]*Selection{
		ProjectText: {ProviderUUID: h.cloud.UUID, Model: "cloud/text"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.providers.Activate(h.ctx, provider.TypeAliyunBailian); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.providers.Settings().Update(h.ctx, map[string]any{sitesettings.CloudflareDefaultModelKey: "cloud/new-text"}); err != nil {
		t.Fatal(err)
	}
	view, err = resolver.Get(h.ctx, h.store)
	if err != nil {
		t.Fatal(err)
	}
	setting := view.Settings[ProjectText]
	if setting.OverrideStatus != "invalid" || setting.Effective == nil || setting.Effective.ProviderUUID != h.bailian.UUID || setting.Source != SourceGlobalDefault {
		t.Fatalf("invalid override did not fall back safely: %+v", setting)
	}
	if _, err := resolver.Resolve(h.ctx, h.store, ProjectImage, KindImage, "", "cloud/new-text"); errorCode(err) != CodeInvalid {
		t.Fatalf("explicit text model accepted for image task: %v", err)
	}
}

func errorCode(err error) string {
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Code
	}
	return ""
}
