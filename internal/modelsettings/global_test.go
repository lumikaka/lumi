package modelsettings

import (
	"encoding/json"
	"strings"
	"testing"

	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
)

func TestGlobalDefaultsAndProjectPrecedence(t *testing.T) {
	for _, scenario := range []string{ChatArea, StoryText, SectionPremiseSelection} {
		t.Run(scenario, func(t *testing.T) {
			h := newSettingsHarness(t)
			r := NewResolver(h.providers)
			initial, err := r.GetGlobal(h.ctx)
			if err != nil || initial.Revision != 0 || len(initial.Settings) != 5 || initial.Settings[scenario].Source != SourceGlobalDefault {
				t.Fatalf("initial=%+v err=%v", initial, err)
			}
			global, err := r.PatchGlobal(h.ctx, PatchInput{Changes: map[string]*Selection{
				ProjectText: {ProviderUUID: h.bailian.UUID, Model: provider.BailianTextModel},
				scenario:    {ProviderUUID: h.bailian.UUID, Model: provider.BailianTextModelQwen38Max},
			}})
			if err != nil {
				t.Fatal(err)
			}
			assertModel := func(model, source string) {
				t.Helper()
				resolved, err := r.Resolve(h.ctx, h.store, scenario, KindText, "", "")
				if err != nil || resolved.Model != model || resolved.Source != source {
					t.Fatalf("resolved=%+v err=%v; want %s %s", resolved, err, model, source)
				}
			}
			assertModel(provider.BailianTextModelQwen38Max, SourceGlobalScenario)
			// A project-wide default B must outrank the global scenario A.
			projectView, err := r.Patch(h.ctx, h.store, PatchInput{Changes: map[string]*Selection{
				ProjectText: {ProviderUUID: h.cloud.UUID, Model: "cloud/text"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			assertModel("cloud/text", SourceProjectTextOverride)
			projectView, err = r.Patch(h.ctx, h.store, PatchInput{ExpectedRevision: projectView.Revision, Changes: map[string]*Selection{
				scenario: {ProviderUUID: h.bailian.UUID, Model: provider.BailianTextModel},
			}})
			if err != nil {
				t.Fatal(err)
			}
			assertModel(provider.BailianTextModel, SourceScenarioOverride)
			explicit, err := r.Resolve(h.ctx, h.store, scenario, KindText, h.cloud.UUID, "cloud/text")
			if err != nil || explicit.Model != "cloud/text" || explicit.Source != SourceExplicitTask {
				t.Fatalf("explicit=%+v err=%v", explicit, err)
			}
			if _, err := r.Patch(h.ctx, h.store, PatchInput{ExpectedRevision: projectView.Revision, Changes: map[string]*Selection{scenario: nil, ProjectText: nil}}); err != nil {
				t.Fatal(err)
			}
			assertModel(provider.BailianTextModelQwen38Max, SourceGlobalScenario)
			global, err = r.PatchGlobal(h.ctx, PatchInput{ExpectedRevision: global.Revision, Changes: map[string]*Selection{scenario: nil}})
			if err != nil {
				t.Fatal(err)
			}
			assertModel(provider.BailianTextModel, SourceGlobalTextDefault)
			if _, err := r.PatchGlobal(h.ctx, PatchInput{ExpectedRevision: global.Revision, Changes: map[string]*Selection{ProjectText: nil}}); err != nil {
				t.Fatal(err)
			}
			assertModel("cloud/text", SourceGlobalDefault)
		})
	}
}

func TestGlobalDefaultsAreSharedWithNewProjectsAndPersistProviderTypes(t *testing.T) {
	h := newSettingsHarness(t)
	r := NewResolver(h.providers)
	global, err := r.PatchGlobal(h.ctx, PatchInput{Changes: map[string]*Selection{
		ProjectText: {ProviderUUID: h.bailian.UUID, Model: provider.BailianTextModelQwen38Max},
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := h.projects.Create(h.ctx, "Created after defaults", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := h.projects.WithCurrentStore(h.ctx, created.UUID, func(store *project.Store) error {
		for _, current := range []*project.Store{h.store, store} {
			view, err := NewResolver(h.providers).Get(h.ctx, current)
			if err != nil {
				return err
			}
			if view.Revision != 0 || view.Settings[ChatArea].Effective.Model != provider.BailianTextModelQwen38Max {
				t.Fatalf("project should inherit without a copied override: %+v", view)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	row, err := h.app.GlobalModelSettings(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(row.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if row.Revision != global.Revision || strings.Contains(string(data), h.bailian.UUID) || row.Settings[ProjectText].ProviderType != provider.TypeAliyunBailian {
		t.Fatalf("unexpected stored settings: %s", data)
	}
}

func TestInvalidGlobalAndProjectOverridesFallThrough(t *testing.T) {
	h := newSettingsHarness(t)
	r := NewResolver(h.providers)
	if _, err := r.PatchGlobal(h.ctx, PatchInput{Changes: map[string]*Selection{
		ProjectText: {ProviderUUID: h.bailian.UUID, Model: provider.BailianTextModelQwen38Max},
		StoryText:   {ProviderUUID: h.cloud.UUID, Model: "cloud/text"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Patch(h.ctx, h.store, PatchInput{Changes: map[string]*Selection{ProjectText: {ProviderUUID: h.cloud.UUID, Model: "cloud/text"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.providers.Activate(h.ctx, provider.TypeAliyunBailian); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.providers.Settings().UpdateSystem(h.ctx, map[string]any{sitesettings.CloudflareDefaultModelKey: "cloud/changed"}); err != nil {
		t.Fatal(err)
	}
	global, err := r.GetGlobal(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s := global.Settings[StoryText]; s.OverrideStatus != "invalid" || s.Override.Model != "cloud/text" || s.Source != SourceGlobalTextDefault {
		t.Fatalf("global fallback=%+v", s)
	}
	view, err := r.Get(h.ctx, h.store)
	if err != nil {
		t.Fatal(err)
	}
	if view.Settings[ProjectText].OverrideStatus != "invalid" || view.Settings[StoryText].Effective.Model != provider.BailianTextModelQwen38Max || view.Settings[StoryText].Source != SourceGlobalTextDefault {
		t.Fatalf("project fallback=%+v", view.Settings)
	}
	if _, err := r.Resolve(h.ctx, h.store, StoryText, KindText, h.cloud.UUID, "cloud/text"); errorCode(err) != CodeInvalid {
		t.Fatalf("explicit unavailable model accepted: %v", err)
	}
	if _, _, err := h.providers.Settings().Reset(h.ctx, []string{sitesettings.BailianAPIKeyKey}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(h.ctx, h.store, StoryText, KindText, "", ""); errorCode(err) != CodeNoModel {
		t.Fatalf("missing model error=%v", err)
	}
}

func TestGlobalValidationAndConcurrentUpdates(t *testing.T) {
	h := newSettingsHarness(t)
	r := NewResolver(h.providers)
	f := false
	for _, changes := range []map[string]*Selection{
		{"unknown": nil},
		{ProjectImage: {ProviderUUID: h.cloud.UUID, Model: "cloud/text"}},
		{ProjectText: {ProviderUUID: h.bailian.UUID, Model: provider.BailianTextModel, EnableThinking: &f}},
		{ProjectImage: {ProviderUUID: h.cloud.UUID, Model: "cloud/image", PromptExtend: &f}},
		{StoryText: {ProviderUUID: h.bailian.UUID, Model: "unknown"}},
	} {
		if _, err := r.PatchGlobal(h.ctx, PatchInput{Changes: changes}); errorCode(err) != CodeInvalid {
			t.Fatalf("accepted invalid changes %+v: %v", changes, err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, key := range []string{ChatArea, StoryText} {
		go func(key string) {
			<-start
			_, err := NewResolver(h.providers).PatchGlobal(h.ctx, PatchInput{Changes: map[string]*Selection{key: {ProviderUUID: h.cloud.UUID, Model: "cloud/text"}}})
			results <- err
		}(key)
	}
	close(start)
	success, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if errorCode(err) == CodeConflict {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflicts=%d", success, conflicts)
	}
	view, err := r.GetGlobal(h.ctx)
	if err != nil || view.Revision != 1 {
		t.Fatalf("view=%+v err=%v", view, err)
	}
}
