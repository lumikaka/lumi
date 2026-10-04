package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/imagegen"
	"lumi/internal/jobqueue"
	"lumi/internal/modelsettings"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
	"lumi/internal/story"

	"github.com/labstack/echo/v4"
)

type modelSettingsEventFake struct {
	topic, event string
	payload      any
}

func (fake *modelSettingsEventFake) Broadcast(topic, event string, payload any) {
	fake.topic, fake.event, fake.payload = topic, event, payload
}

func TestModelSettingsHandlerExposesRevisionedPublicResource(t *testing.T) {
	for _, global := range []bool{false, true} {
		name := "project"
		if global {
			name = "global"
		}
		t.Run(name, func(t *testing.T) { testModelSettingsPublicResource(t, global) })
	}
}

func testModelSettingsPublicResource(t *testing.T, global bool) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	createdProvider, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "api/text", DefaultImageModel: "api/image", APIKey: "private-secret"})
	if err != nil {
		t.Fatal(err)
	}
	projects := project.NewManager(app)
	createdProject, err := projects.Create(ctx, "API model settings", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = projects.Close(); providers.Close(); _ = app.Close() })

	events := &modelSettingsEventFake{}
	handler := NewModelSettingsHandler(projects, modelsettings.NewResolver(providers), events)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.GET("/api/v1/projects/:project_uuid/model-settings", handler.Show)
	e.PATCH("/api/v1/projects/:project_uuid/model-settings", handler.Update)
	base := "/api/v1/projects/" + createdProject.UUID + "/model-settings"
	expectedTopic, expectedEvent, expectedSource := "project:"+createdProject.UUID, "project:model_settings_changed", "scenario_override"
	if global {
		e.GET("/api/v1/model-settings", handler.ShowGlobal)
		e.PATCH("/api/v1/model-settings", handler.UpdateGlobal)
		base = "/api/v1/model-settings"
		expectedTopic, expectedEvent, expectedSource = "system", "model_settings:changed", "global_scenario_default"
	}

	shown := requestJSON(t, e, "GET", base, nil)
	if shown.Code != 200 || strings.Contains(shown.Body.String(), "private-secret") || strings.Contains(shown.Body.String(), `"id"`) || !strings.Contains(shown.Body.String(), `"source":"global_provider_default"`) {
		t.Fatalf("show=%d %s", shown.Code, shown.Body.String())
	}
	updated := requestJSON(t, e, "PATCH", base, map[string]any{
		"expected_revision": 0,
		"overrides":         map[string]any{"story_text": map[string]any{"provider_uuid": createdProvider.UUID, "model": "api/text"}},
	})
	if updated.Code != 200 || !strings.Contains(updated.Body.String(), `"revision":1`) || !strings.Contains(updated.Body.String(), `"source":"`+expectedSource+`"`) {
		t.Fatalf("update=%d %s", updated.Code, updated.Body.String())
	}
	if events.topic != expectedTopic || events.event != expectedEvent || strings.Contains(strings.TrimSpace(updated.Body.String()), "private-secret") {
		t.Fatalf("event=%s %s %#v", events.topic, events.event, events.payload)
	}
	if global {
		payload, err := json.Marshal(events.payload)
		if err != nil || string(payload) != `{"revision":1}` {
			t.Fatalf("global event payload=%s err=%v", payload, err)
		}
		projectShown := requestJSON(t, e, "GET", "/api/v1/projects/"+createdProject.UUID+"/model-settings", nil)
		if projectShown.Code != 200 || !strings.Contains(projectShown.Body.String(), `"source":"global_scenario_default"`) {
			t.Fatalf("project inheritance=%s", projectShown.Body.String())
		}
	}
	conflict := requestJSON(t, e, "PATCH", base, map[string]any{"expected_revision": 0, "overrides": map[string]any{"story_text": nil}})
	if conflict.Code != 409 || !strings.Contains(conflict.Body.String(), `"code":"project_model_settings_conflict"`) {
		t.Fatalf("conflict=%d %s", conflict.Code, conflict.Body.String())
	}
	invalid := requestJSON(t, e, "PATCH", base, map[string]any{
		"expected_revision": 1,
		"overrides":         map[string]any{"project_image": map[string]any{"provider_uuid": createdProvider.UUID, "model": "api/text"}},
	})
	if invalid.Code != 422 || !strings.Contains(invalid.Body.String(), `"code":"project_model_settings_invalid"`) {
		t.Fatalf("invalid=%d %s", invalid.Code, invalid.Body.String())
	}
	cleared := requestJSON(t, e, "PATCH", base, map[string]any{"expected_revision": 1, "overrides": map[string]any{"story_text": nil}})
	if cleared.Code != 200 || !strings.Contains(cleared.Body.String(), `"revision":2`) || !strings.Contains(cleared.Body.String(), `"override":null`) {
		t.Fatalf("clear=%d %s", cleared.Code, cleared.Body.String())
	}
}

func TestGlobalModelRateLimitRulesAreRevisionedAndValidated(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	created, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "api/text", DefaultImageModel: "api/image", APIKey: "private-secret"})
	if err != nil {
		t.Fatal(err)
	}
	allProviders, err := providers.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bailianUUID := ""
	for _, item := range allProviders {
		if item.ProviderType == provider.TypeAliyunBailian {
			bailianUUID = item.UUID
		}
	}
	if bailianUUID == "" {
		t.Fatal("Bailian provider identity missing")
	}
	t.Cleanup(func() { providers.Close(); _ = app.Close() })
	resolver := modelsettings.NewResolver(providers)
	handler := NewModelSettingsHandler(nil, resolver, nil)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.GET("/api/v1/model-settings", handler.ShowGlobal)
	e.PATCH("/api/v1/model-settings", handler.UpdateGlobal)
	path := "/api/v1/model-settings"
	initial := requestJSON(t, e, "GET", path, nil)
	if initial.Code != 200 || !strings.Contains(initial.Body.String(), `"scope":"provider"`) || !strings.Contains(initial.Body.String(), `"rpm":200`) {
		t.Fatalf("initial=%d %s", initial.Code, initial.Body.String())
	}
	update := map[string]any{"provider_uuid": created.UUID, "model": "api/text", "rpm": 3, "rps": 1}
	createdRule := requestJSON(t, e, "PATCH", path, map[string]any{"expected_revision": 0, "rate_limit_updates": []any{update}})
	if createdRule.Code != 200 || !strings.Contains(createdRule.Body.String(), `"rpm":3`) || !strings.Contains(createdRule.Body.String(), `"rps":1`) || !strings.Contains(createdRule.Body.String(), `"revision":1`) {
		t.Fatalf("created rule=%d %s", createdRule.Code, createdRule.Body.String())
	}
	row, err := app.GlobalModelSettings(ctx)
	if err != nil || len(row.RateLimits[created.ProviderType]) != 1 {
		t.Fatalf("persisted limits=%+v err=%v", row.RateLimits, err)
	}
	for _, test := range []struct {
		name   string
		body   map[string]any
		status int
	}{
		{"stale revision", map[string]any{"expected_revision": 0, "rate_limit_updates": []any{update}}, 409},
		{"zero", map[string]any{"expected_revision": 1, "rate_limit_updates": []any{map[string]any{"provider_uuid": created.UUID, "model": "api/text", "rpm": 0}}}, 422},
		{"wrong model", map[string]any{"expected_revision": 1, "rate_limit_updates": []any{map[string]any{"provider_uuid": created.UUID, "model": "api/unknown", "rpm": 3}}}, 422},
		{"other provider model", map[string]any{"expected_revision": 1, "rate_limit_updates": []any{map[string]any{"provider_uuid": created.UUID, "model": provider.BailianTextModel, "rpm": 3}}}, 422},
		{"other provider identity", map[string]any{"expected_revision": 1, "rate_limit_updates": []any{map[string]any{"provider_uuid": bailianUUID, "model": "api/text", "rpm": 3}}}, 422},
		{"duplicate", map[string]any{"expected_revision": 1, "rate_limit_updates": []any{update, update}}, 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := requestJSON(t, e, "PATCH", path, test.body)
			if response.Code != test.status {
				t.Fatalf("response=%d %s", response.Code, response.Body.String())
			}
		})
	}
	cleared := requestJSON(t, e, "PATCH", path, map[string]any{"expected_revision": 1, "rate_limit_updates": []any{map[string]any{"provider_uuid": created.UUID, "model": "api/text", "rpm": nil, "rps": nil}}})
	if cleared.Code != 200 || strings.Contains(cleared.Body.String(), `"scope":"model"`) || !strings.Contains(cleared.Body.String(), `"revision":2`) {
		t.Fatalf("cleared=%d %s", cleared.Code, cleared.Body.String())
	}
	providerRule := map[string]any{"scope": "provider", "provider_uuid": created.UUID, "rpm": 20, "rps": 2}
	providerSaved := requestJSON(t, e, "PATCH", path, map[string]any{"expected_revision": 2, "rate_limit_updates": []any{providerRule}})
	if providerSaved.Code != 200 || !strings.Contains(providerSaved.Body.String(), `"scope":"provider"`) || !strings.Contains(providerSaved.Body.String(), `"rpm":20`) || !strings.Contains(providerSaved.Body.String(), `"revision":3`) {
		t.Fatalf("provider rule=%d %s", providerSaved.Code, providerSaved.Body.String())
	}
	row, err = app.GlobalModelSettings(ctx)
	if err != nil || row.ProviderRateLimits[created.ProviderType].RPM == nil || *row.ProviderRateLimits[created.ProviderType].RPM != 20 {
		t.Fatalf("persisted provider limits=%+v err=%v", row.ProviderRateLimits, err)
	}
	for _, invalid := range []map[string]any{
		{"scope": "provider", "provider_uuid": created.UUID, "model": "api/text", "rpm": 10},
		{"scope": "other", "provider_uuid": created.UUID, "rpm": 10},
		{"scope": "provider", "provider_uuid": created.UUID, "rpm": -1},
	} {
		response := requestJSON(t, e, "PATCH", path, map[string]any{"expected_revision": 3, "rate_limit_updates": []any{invalid}})
		if response.Code != 422 {
			t.Fatalf("invalid provider rule=%d %s", response.Code, response.Body.String())
		}
	}
	providerCleared := requestJSON(t, e, "PATCH", path, map[string]any{"expected_revision": 3, "rate_limit_updates": []any{map[string]any{"scope": "provider", "provider_uuid": created.UUID, "rpm": nil, "rps": nil}}})
	if providerCleared.Code != 200 || !strings.Contains(providerCleared.Body.String(), `"rate_limits":[]`) {
		t.Fatalf("cleared provider rule=%d %s", providerCleared.Code, providerCleared.Body.String())
	}
	combined := requestJSON(t, e, "PATCH", path, map[string]any{"expected_revision": 4, "rate_limit_updates": []any{
		map[string]any{"scope": "provider", "provider_uuid": created.UUID, "rpm": 20},
		map[string]any{"scope": "model", "provider_uuid": created.UUID, "model": "api/text", "rps": 1},
	}})
	if combined.Code != 200 || strings.Count(combined.Body.String(), `"scope":`) != 2 || !strings.Contains(combined.Body.String(), `"revision":5`) {
		t.Fatalf("combined rules=%d %s", combined.Code, combined.Body.String())
	}
}

type observedHTTPImageProvider struct {
	requests chan imagegen.Request
	content  []byte
}

func (client observedHTTPImageProvider) Generate(ctx context.Context, request imagegen.Request) (imagegen.Response, error) {
	select {
	case client.requests <- request:
		return imagegen.Response{Bytes: client.content, MIMEType: "image/png"}, nil
	case <-ctx.Done():
		return imagegen.Response{}, ctx.Err()
	}
}

func TestProjectImageProSelectionPreflightAndTaskFreeze(t *testing.T) {
	for _, global := range []bool{false, true} {
		name := "project"
		if global {
			name = "global"
		}
		t.Run(name, func(t *testing.T) { testImageProSelectionPreflightAndTaskFreeze(t, global) })
	}
}

func testImageProSelectionPreflightAndTaskFreeze(t *testing.T, global bool) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	t.Cleanup(providers.Close)
	if _, _, err := providers.Settings().Update(ctx, map[string]any{
		sitesettings.BailianWorkspaceKey: "image-pro-workspace",
		sitesettings.BailianRegionKey:    "cn-beijing",
		sitesettings.BailianAPIKeyKey:    "image-pro-secret",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := providers.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bailian provider.Provider
	for _, item := range items {
		if item.ProviderType == provider.TypeAliyunBailian {
			bailian, err = providers.MarkVerified(ctx, item.UUID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := providers.Activate(ctx, provider.TypeAliyunBailian); err != nil {
		t.Fatal(err)
	}
	imageClient := observedHTTPImageProvider{requests: make(chan imagegen.Request, 1), content: apiPNG(t)}
	queue := jobqueue.NewManager(providers, immediateModelFake{}, nil).WithImageClient(imageClient)
	projects := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen).WithRuntime(queue).WithOpenHook(queue.StartProject)
	t.Cleanup(func() { _ = projects.Close() })
	created, err := projects.CreateWithInput(ctx, project.CreateInput{
		Name:        "Pro image model",
		PictureBook: &project.PictureBookInput{Format: project.PictureBookClassic, AspectRatio: &project.AspectRatioInput{Mode: project.AspectLandscape}},
	}, project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var chapter story.Chapter
	var section production.ComicSection
	if err := projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		var err error
		chapter, err = story.NewService(store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Opening"})
		if err != nil {
			return err
		}
		section, err = production.NewService(store, nil).CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Landscape", StoryboardMD: "A quiet forest in a 4:3 frame."})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	models := modelsettings.NewResolver(providers)
	settingsHandler := NewModelSettingsHandler(projects, models, nil)
	e.GET("/api/v1/projects/:project_uuid/model-settings", settingsHandler.Show)
	e.PATCH("/api/v1/projects/:project_uuid/model-settings", settingsHandler.Update)
	e.GET("/api/v1/model-settings", settingsHandler.ShowGlobal)
	e.PATCH("/api/v1/model-settings", settingsHandler.UpdateGlobal)
	e.POST("/api/v1/projects/:project_uuid/image-generation-preflights", NewProjectImageGenerationPreflightHandler(projects, models).Create)
	e.POST("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/image-generations", NewProductionHandler(projects, queue, nil).GenerateSectionImage)
	base := "/api/v1/projects/" + created.UUID
	settingsPath, expectedSource := base+"/model-settings", modelsettings.SourceProjectImageOverride
	if global {
		settingsPath, expectedSource = "/api/v1/model-settings", modelsettings.SourceGlobalImageDefault
	}
	updated := requestJSON(t, e, http.MethodPatch, settingsPath, map[string]any{
		"expected_revision": 0,
		"overrides":         map[string]any{"project_image": map[string]any{"provider_uuid": bailian.UUID, "model": provider.BailianImageModelPro, "enable_thinking": false, "prompt_extend": false}},
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("save Pro=%d %s", updated.Code, updated.Body.String())
	}
	shown := requestJSON(t, e, http.MethodGet, settingsPath, nil)
	var settings struct {
		Data modelsettings.View `json:"data"`
	}
	if err := json.Unmarshal(shown.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	setting := settings.Data.Settings[modelsettings.ProjectImage]
	if shown.Code != http.StatusOK || settings.Data.Revision != 1 || setting.Effective == nil || setting.Effective.Model != provider.BailianImageModelPro || setting.Effective.EnableThinking == nil || *setting.Effective.EnableThinking || setting.Effective.PromptExtend == nil || *setting.Effective.PromptExtend || setting.Inherited == nil || setting.Inherited.Model != provider.BailianImageModel {
		t.Fatalf("persisted settings=%d %s", shown.Code, shown.Body.String())
	}
	preflight := requestJSON(t, e, http.MethodPost, base+"/image-generation-preflights", map[string]any{})
	if preflight.Code != http.StatusOK {
		t.Fatalf("Pro preflight=%d %s", preflight.Code, preflight.Body.String())
	}
	data := envelopeData(t, preflight)
	if data["model"] != provider.BailianImageModelPro || data["provider_uuid"] != bailian.UUID || data["model_source"] != expectedSource || data["output_size"].(map[string]any)["value"] != "1536x1152" {
		t.Fatalf("Pro preflight=%+v", data)
	}
	generated := requestJSON(t, e, http.MethodPost, base+"/chapters/"+chapter.UUID+"/comic-sections/"+section.UUID+"/image-generations", map[string]any{"idempotency_key": "pro-image-generation"})
	if generated.Code != http.StatusCreated {
		t.Fatalf("Pro task=%d %s", generated.Code, generated.Body.String())
	}
	taskUUID := envelopeData(t, generated)["uuid"].(string)
	cleared := requestJSON(t, e, http.MethodPatch, settingsPath, map[string]any{
		"expected_revision": 1, "overrides": map[string]any{"project_image": nil},
	})
	if cleared.Code != http.StatusOK {
		t.Fatalf("restore inheritance=%d %s", cleared.Code, cleared.Body.String())
	}
	task, err := queue.GetProductionTask(ctx, created.UUID, taskUUID)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot production.GenerationSnapshot
	if err := json.Unmarshal(task.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if task.ProviderUUID != bailian.UUID || task.Model != provider.BailianImageModelPro || task.ModelSource != expectedSource || snapshot.Model != provider.BailianImageModelPro || snapshot.EnableThinking == nil || *snapshot.EnableThinking || snapshot.PromptExtend == nil || *snapshot.PromptExtend || snapshot.OutputSize != "1536x1152" {
		t.Fatalf("frozen task model=%s source=%s snapshot=%+v", task.Model, task.ModelSource, snapshot)
	}
	select {
	case request := <-imageClient.requests:
		if request.ProviderType != provider.TypeAliyunBailian || request.Model != provider.BailianImageModelPro || request.EnableThinking == nil || *request.EnableThinking || request.EnablePromptExtend == nil || *request.EnablePromptExtend || request.Size != snapshot.OutputSize || request.BaseURL != bailian.ImageBaseURL {
			t.Fatalf("image request provider=%s model=%s size=%s url=%s", request.ProviderType, request.Model, request.Size, request.BaseURL)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Pro image request was not issued")
	}
}
