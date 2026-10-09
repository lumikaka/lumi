package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"io"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/provider"
	"lumi/internal/providerlimit"
	"lumi/internal/sitesettings"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelayChecksBothGroupsAndRejectsStaleConfiguration(t *testing.T) {
	for _, mode := range []string{"responses", "images"} {
		for _, scenario := range []string{"success", "text failure", "image failure", "empty image", "image rate limit", "changed during check", "mode changed during check"} {
			t.Run(mode+"/"+scenario, func(t *testing.T) {
				ctx := context.Background()
				imageModel := "gpt-6.1-sol"
				if mode == "images" {
					imageModel = "gpt-image-2"
				}
				dir := t.TempDir()
				app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
				if err != nil {
					t.Fatal(err)
				}
				defer app.Close()
				service := provider.NewService(app, provider.NewMemorySecretStore())
				defer service.Close()
				if _, _, err := service.Settings().Update(ctx, map[string]any{sitesettings.RelayBaseURLKey: "https://text.example/v1", sitesettings.RelayAPIKeyKey: "relay-text-private", sitesettings.RelayImageBaseURLKey: "https://image.example/api/v1", sitesettings.RelayImageAPIKeyKey: "relay-image-private", sitesettings.RelayImageAPIModeKey: mode, sitesettings.RelayDefaultImageModelKey: imageModel}); err != nil {
					t.Fatal(err)
				}
				items, err := service.List(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var relay provider.Provider
				for _, item := range items {
					if item.ProviderType == provider.TypeOpenAIRelay {
						relay = item
					}
				}
				calls := 0
				transport := &http.Client{Transport: providerCheckTransportFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					wantURL, wantKey, wantModel := "https://text.example/v1/responses", "relay-text-private", "gpt-6.1-sol"
					if calls == 2 {
						wantURL, wantKey, wantModel = "https://image.example/api/v1/responses", "relay-image-private", imageModel
						if mode == "images" {
							wantURL = "https://image.example/api/v1/images/generations"
						}
					}
					if req.URL.String() != wantURL || req.Header.Get("Authorization") != "Bearer "+wantKey || req.Header.Get("cf-aig-max-attempts") != "" || req.Header.Get("cf-aig-gateway-id") != "" {
						t.Fatal("wrong endpoint, credentials, or headers")
					}
					var payload map[string]any
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					if payload["model"] != wantModel || (calls == 1 && (payload["tools"] != nil || payload["stream"] != false)) {
						t.Fatalf("probe=%v", payload)
					}
					status, body := 200, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}]}`
					if calls == 2 {
						picture := base64.StdEncoding.EncodeToString(apiPNG(t))
						if mode == "images" {
							if payload["tools"] != nil || payload["prompt"] == nil {
								t.Fatal("incorrect Images probe")
							}
							body = fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, picture)
						} else {
							if payload["tools"] == nil {
								t.Fatal("image tool missing from probe")
							}
							body = fmt.Sprintf(`{"output":[{"type":"image_generation_call","result":%q}]}`, picture)
						}
					}
					if (scenario == "text failure" && calls == 1) || (scenario == "image failure" && calls == 2) {
						status, body = 401, `{"error":{"message":"denied `+wantKey+`"}}`
					}
					if calls == 2 && scenario == "empty image" {
						body = `{}`
					}
					if calls == 2 && scenario == "image rate limit" {
						status, body = 429, `{"error":{"message":"try later"}}`
					}
					if calls == 2 && scenario == "mode changed during check" {
						nextMode, nextModel := "images", "gpt-image-2"
						if mode == "images" {
							nextMode, nextModel = "responses", "gpt-6.1-sol"
						}
						if _, _, err := service.Settings().Update(ctx, map[string]any{sitesettings.RelayImageAPIModeKey: nextMode, sitesettings.RelayDefaultImageModelKey: nextModel}); err != nil {
							t.Fatal(err)
						}
					}
					if scenario == "changed during check" && calls == 2 {
						if _, _, err := service.Settings().Update(ctx, map[string]any{sitesettings.RelayImageAPIKeyKey: "new-image-key"}); err != nil {
							t.Fatal(err)
						}
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
				})}
				client := llm.NewOpenAICompatibleClient(transport)
				limiter := &providerLimitFake{}
				e := echo.New()
				e.HTTPErrorHandler = ErrorHandler
				e.POST("/providers/:provider_uuid/check", NewProviderHandler(service, client).WithImageClient(imagegen.NewOpenAICompatibleClient(transport)).WithProviderLimiter(limiter).Check)
				checked := requestJSON(t, e, "POST", "/providers/"+relay.UUID+"/check", nil)
				current, err := service.Get(ctx, relay.UUID)
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "success" {
					if checked.Code != 200 || !current.Ready {
						t.Fatalf("check=%s provider=%+v", checked.Body, current)
					}
				} else {
					if checked.Code == 200 || current.Ready {
						t.Fatal("failed or stale check accepted")
					}
					if scenario == "text failure" && !strings.Contains(checked.Body.String(), "provider_text_check_failed") {
						t.Fatal(checked.Body.String())
					}
					if (scenario == "image failure" || scenario == "empty image" || scenario == "image rate limit") && !strings.Contains(checked.Body.String(), "provider_image_check_failed") {
						t.Fatal(checked.Body.String())
					}
					if (scenario == "changed during check" || scenario == "mode changed during check") && checked.Code != 409 {
						t.Fatal(checked.Body.String())
					}
				}
				wantCalls := 2
				if scenario == "text failure" {
					wantCalls = 1
				}
				if calls == 2 && (limiter.class != providerlimit.RequestImage || !limiter.finished || limiter.baseURL != "https://image.example/api/v1") {
					t.Fatalf("wrong image limiter: %+v", limiter)
				}
				if scenario == "image rate limit" && checked.Code != 429 {
					t.Fatalf("status=%d", checked.Code)
				}
				if calls != wantCalls {
					t.Fatalf("calls=%d", calls)
				}
				if strings.Contains(checked.Body.String(), "relay-text-private") || strings.Contains(checked.Body.String(), "relay-image-private") {
					t.Fatal("secret leaked")
				}
			})
		}
	}
}
