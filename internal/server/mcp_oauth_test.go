package server

import (
	"bytes"
	"context"
	"encoding/json"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"lumi/internal/config"
)

// This exercises SDK discovery, dynamic registration, browser redirect, the
// protected desktop consent API, PKCE token exchange and a real MCP session.
func TestMCPOAuthOfficialSDK(t *testing.T) {
	for _, scenario := range []struct{ mode, permission string }{{"dynamic", "read"}, {"dynamic", "edit"}, {"prod", "read"}, {"prod", "edit"}, {"dev", "read"}, {"dev", "edit"}} {
		permission := scenario.permission
		t.Run(scenario.mode+"/"+permission, func(t *testing.T) {
			h := newMCPHarness(t)
			backend := httptest.NewUnstartedServer(h.a.Echo)
			base := "http://" + backend.Listener.Addr().String()
			h.a.mountMCP(base, "test-instance")
			backend.Start()
			t.Cleanup(backend.Close)
			var capturedToken string
			clientName := "SDK OAuth"
			oauthConfig := &auth.AuthorizationCodeHandlerConfig{
				DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{ClientName: "SDK OAuth", RedirectURIs: []string{"http://127.0.0.1:48123/callback"}, TokenEndpointAuthMethod: "none", GrantTypes: []string{"authorization_code", "refresh_token"}}},
				RequestRefreshToken:             true,
				NewTokenSource: func(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
					token.Expiry = time.Now().Add(-time.Minute)
					return cfg.TokenSource(ctx, token), nil
				},
				AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
					browser := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
					authorizeURL, err := url.Parse(args.URL)
					if err != nil {
						return nil, err
					}
					if scenario.mode != "dynamic" {
						// Reproduce Codex sending the discovered resource twice.
						query := authorizeURL.Query()
						query.Add("resource", base+"/mcp")
						authorizeURL.RawQuery = query.Encode()
					}
					resp, err := browser.Get(authorizeURL.String())
					if err != nil {
						return nil, err
					}
					defer resp.Body.Close()
					if resp.StatusCode != 303 {
						b, _ := io.ReadAll(resp.Body)
						t.Fatalf("authorize %d: %s", resp.StatusCode, b)
					}
					location, err := url.Parse(resp.Header.Get("Location"))
					if err != nil {
						return nil, err
					}
					id := location.Query().Get("request_uuid")
					request := httptest.NewRequest("GET", "/api/v1/mcp-authorization-requests/"+id, nil)
					rec := httptest.NewRecorder()
					h.a.Echo.ServeHTTP(rec, request)
					if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"client_name":"`+clientName+`"`) {
						t.Fatalf("consent: %s", rec.Body.String())
					}
					body, _ := json.Marshal(map[string]string{"project_uuid": h.p, "permission": permission, "decision": "approve"})
					request = httptest.NewRequest("POST", "/api/v1/mcp-authorization-requests/"+id+"/decisions", bytes.NewReader(body))
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set("Origin", "http://localhost:5801")
					rec = httptest.NewRecorder()
					h.a.Echo.ServeHTTP(rec, request)
					var decision struct {
						Data struct {
							Redirect string `json:"redirect_url"`
						} `json:"data"`
					}
					if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &decision) != nil {
						t.Fatalf("approve %d: %s", rec.Code, rec.Body.String())
					}
					callback, err := url.Parse(decision.Data.Redirect)
					if err != nil {
						return nil, err
					}
					requestedCallback, err := url.Parse(authorizeURL.Query().Get("redirect_uri"))
					if err != nil || callback.Host != requestedCallback.Host || callback.Path != requestedCallback.Path {
						t.Fatalf("callback changed: %s", decision.Data.Redirect)
					}
					return &auth.AuthorizationResult{Code: callback.Query().Get("code"), State: callback.Query().Get("state"), Iss: callback.Query().Get("iss")}, nil
				},
			}
			if scenario.mode != "dynamic" {
				serverName := "lumi-" + scenario.mode + "-plugin"
				raw, err := os.ReadFile(filepath.Join("..", "..", "codex-plugins", serverName, ".mcp.json"))
				if err != nil {
					t.Fatal(err)
				}
				var plugin struct {
					Servers map[string]struct {
						Type     string
						URL      string
						Resource json.RawMessage `json:"oauth_resource"`
						OAuth    json.RawMessage `json:"oauth"`
					} `json:"mcpServers"`
				}
				if err := json.Unmarshal(raw, &plugin); err != nil {
					t.Fatal(err)
				}
				spec, ok := plugin.Servers[serverName]
				if !ok {
					t.Fatal("plugin MCP name mismatch")
				}
				environment := "development"
				clientName = "Lumi development plugin"
				if scenario.mode == "prod" {
					environment = "production"
					clientName = "Lumi desktop plugin"
				}
				expectedURL := "http://127.0.0.1:5801/mcp"
				if environment == "production" {
					expectedURL = "http://127.0.0.1:32323/mcp"
				}
				if spec.Type != "http" || spec.URL != expectedURL || len(spec.Resource) != 0 || len(spec.OAuth) != 0 {
					t.Fatalf("plugin connection drift: %+v", spec)
				}
				// A URL-only plugin discovers OAuth metadata and registers the exact
				// callback selected by Codex, including its server-specific suffix.
				oauthConfig.DynamicClientRegistrationConfig.Metadata.ClientName = clientName
				oauthConfig.DynamicClientRegistrationConfig.Metadata.RedirectURIs = []string{"http://127.0.0.1:50132/callback/VJL--Uj9wWIv"}
			}
			handler, err := auth.NewAuthorizationCodeHandler(oauthConfig)
			if err != nil {
				t.Fatal(err)
			}
			session, err := mcp.NewClient(&mcp.Implementation{Name: "oauth integration", Version: "1"}, nil).Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: base + "/mcp", OAuthHandler: handler}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { session.Close() })
			list, err := session.ListTools(t.Context(), nil)
			if err != nil || len(list.Tools) != 4 {
				t.Fatalf("list: %+v %v", list, err)
			}
			docs, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "read_agent_doc", Arguments: map[string]any{"path": "/api/v1/agent-docs/overview.md"}})
			if err != nil || docs.IsError {
				t.Fatalf("docs: %+v %v", docs, err)
			}
			source, err := handler.TokenSource(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			token, err := source.Token()
			if err != nil {
				t.Fatal(err)
			}
			capturedToken = token.AccessToken
			if token.RefreshToken == "" {
				t.Fatal("missing refresh token")
			}
			g, err := h.a.mcpService.Authenticate(t.Context(), capturedToken)
			if err != nil || g.Permission != permission || g.ProjectUUID != h.p {
				t.Fatalf("grant: %+v %v", g, err)
			}
			h.client = session
			h.request(t, "GET", "", nil, "", false)
			h.request(t, "PATCH", "", map[string]any{"name": "OAuth edit", "description": "", "expected_revision": float64(1)}, "oauth-edit", permission == "read")
			if err := h.a.mcpService.Revoke(t.Context(), h.p, g.UUID); err != nil {
				t.Fatal(err)
			}
			request, _ := http.NewRequest("POST", base+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
			request.Header.Set("Authorization", "Bearer "+capturedToken)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 401 || !strings.Contains(response.Header.Get("WWW-Authenticate"), "resource_metadata") {
				t.Fatalf("revocation: %d", response.StatusCode)
			}
		})
	}
}

func TestMCPOAuthConsentRequiresDesktopSessionAndOrigin(t *testing.T) {
	h := newMCPHarness(t)
	h.a.Echo.Use(desktopAuthentication(config.NewDesktopAuthentication("desktop-test-secret")))
	for _, tc := range []struct {
		cookie, origin, body string
		status               int
	}{
		{"", "http://localhost:5801", `{"decision":"approve"}`, 401},
		{"desktop-test-secret", "http://evil.test", `{"decision":"approve"}`, 403},
		{"desktop-test-secret", "http://localhost:5801", `{"decision":"approve","confirmed":true}`, 422},
		{"desktop-test-secret", "http://localhost:5801", `{"decision":"approve"}`, 409},
	} {
		request := httptest.NewRequest("POST", "/api/v1/mcp-authorization-requests/01970000-0000-7000-8000-000000000001/decisions", strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", tc.origin)
		if tc.cookie != "" {
			request.AddCookie(&http.Cookie{Name: desktopSessionCookie, Value: tc.cookie})
		}
		rec := httptest.NewRecorder()
		h.a.Echo.ServeHTTP(rec, request)
		if rec.Code != tc.status {
			t.Fatalf("consent boundary: %d want %d: %s", rec.Code, tc.status, rec.Body.String())
		}
	}
}

func TestMCPSharedListenerAuthenticationIsolation(t *testing.T) {
	h := newMCPHarness(t)
	h.a.Echo.Use(desktopAuthentication(config.NewDesktopAuthentication("desktop-shared-test")))
	h.a.mountMCP("http://127.0.0.1:5801", "current-instance")
	for _, tc := range []struct {
		method, path, cookie, bearer, origin string
		status                               int
	}{
		{"GET", "/.well-known/oauth-authorization-server", "", "", "", 200},
		{"POST", "/mcp", "desktop-shared-test", "", "", 401},
		{"GET", "/api/v1/providers", "", h.token, "", 401},
		{"GET", "/api/v1/mcp", "", h.token, "", 401},
		{"GET", "/api/v1/mcp", "desktop-shared-test", "", "", 200},
		{"POST", "/api/v1/mcp-authorization-requests/01970000-0000-7000-8000-000000000001/decisions", "", h.token, "http://localhost:5801", 401},
		{"POST", "/mcp", "", h.token, "http://localhost:5801", 403},
		{"POST", "/api/v1/mcp-authorization-requests/01970000-0000-7000-8000-000000000001/decisions", "desktop-shared-test", "", "http://evil.test", 403},
	} {
		request := httptest.NewRequest(tc.method, "http://127.0.0.1:5801"+tc.path, strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", tc.origin)
		if tc.bearer != "" {
			request.Header.Set("Authorization", "Bearer "+tc.bearer)
		}
		if tc.cookie != "" {
			request.AddCookie(&http.Cookie{Name: desktopSessionCookie, Value: tc.cookie})
		}
		rec := httptest.NewRecorder()
		h.a.Echo.ServeHTTP(rec, request)
		if rec.Code != tc.status {
			t.Fatalf("%s %s: status %d want %d: %s", tc.method, tc.path, rec.Code, tc.status, rec.Body.String())
		}
	}
}
