package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/modelsettings"
	"lumi/internal/project"
	"lumi/internal/provider"

	"github.com/labstack/echo/v4"
)

func TestImageGenerationPreflightValidatesBeforeProjectCreation(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	if _, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "openai/gpt-5", DefaultImageModel: "openai/gpt-image-1.5", APIKey: "secret"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		providers.Close()
		_ = app.Close()
	})
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/image-generation-preflights", NewImageGenerationPreflightHandler(providers).Create)

	supported := requestJSON(t, e, http.MethodPost, "/api/v1/image-generation-preflights", map[string]any{
		"picture_book": map[string]any{"format": "wordless_picture_book", "aspect_ratio": map[string]any{"mode": "square"}},
	})
	if supported.Code != http.StatusOK || !strings.Contains(supported.Body.String(), `"value":"1024x1024"`) || !strings.Contains(supported.Body.String(), `"format":"wordless_picture_book"`) || strings.Contains(supported.Body.String(), `"id"`) {
		t.Fatalf("supported preflight=%d %s", supported.Code, supported.Body.String())
	}
	unsupported := requestJSON(t, e, http.MethodPost, "/api/v1/image-generation-preflights", map[string]any{
		"picture_book": map[string]any{"format": "classic_picture_book", "aspect_ratio": map[string]any{"mode": "landscape"}},
	})
	if unsupported.Code != http.StatusUnprocessableEntity || !strings.Contains(unsupported.Body.String(), `"code":"image_aspect_ratio_unsupported"`) {
		t.Fatalf("unsupported preflight=%d %s", unsupported.Code, unsupported.Body.String())
	}
}

func TestProjectImageGenerationPreflightUsesImmutableProfileAndEffectiveModel(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	configured, err := providers.Create(ctx, provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "openai/gpt-5", DefaultImageModel: "openai/gpt-image-1.5", APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	projects := project.NewManager(app)
	created, err := projects.CreateWithInput(ctx, project.CreateInput{
		Name: "Square Book",
		PictureBook: &project.PictureBookInput{
			Format:      project.PictureBookWordless,
			AspectRatio: &project.AspectRatioInput{Mode: project.AspectSquare},
		},
	}, project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = projects.Close()
		providers.Close()
		_ = app.Close()
	})
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/projects/:project_uuid/image-generation-preflights", NewProjectImageGenerationPreflightHandler(projects, modelsettings.NewResolver(providers)).Create)
	response := requestJSON(t, e, http.MethodPost, "/api/v1/projects/"+created.UUID+"/image-generation-preflights", map[string]any{})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), configured.UUID) || !strings.Contains(response.Body.String(), `"model_source":"global_provider_default"`) || !strings.Contains(response.Body.String(), `"value":"1024x1024"`) || !strings.Contains(response.Body.String(), `"format":"wordless_picture_book"`) {
		t.Fatalf("project preflight=%d %s", response.Code, response.Body.String())
	}
}
