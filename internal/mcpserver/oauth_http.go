package mcpserver

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// LocalHTTP mounts only MCP and OAuth, independently of desktop cookies. The
// authorization decision is deliberately absent: it belongs to the desktop API.
func (s *Service) LocalHTTP(base, frontend, nonce string) http.Handler {
	resource := base + "/mcp"
	s.connection.Store("endpoint", resource)
	host, _ := url.Parse(base)
	protocol := s.protocol(resource)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Host != host.Host {
			http.Error(w, "invalid local host", 403)
			return
		}
		// Protect against browser drive-by requests and DNS rebinding. Navigating to
		// authorization is allowed; approval still requires the protected Lumi UI.
		if r.Header.Get("Origin") != "" && !(r.Method == "GET" && r.URL.Path == "/oauth/authorize") {
			http.Error(w, "origin forbidden", 403)
			return
		}
		if instance := r.Header.Get("X-Lumi-Instance"); instance != "" && instance != nonce {
			http.Error(w, "stale instance", 403)
			return
		}
		respond := func(status int, v any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(v)
		}
		fail := func(err error) {
			var e *OAuthError
			if !errors.As(err, &e) {
				e = &OAuthError{"server_error", "Local authorization failed."}
			}
			status := 400
			if e.Code == "server_error" {
				status = 500
			}
			respond(status, map[string]string{"error": e.Code, "error_description": e.Description})
		}
		switch {
		case r.URL.Path == "/mcp":
			protocol.ServeHTTP(w, r)
		case r.Method == "GET" && (r.URL.Path == "/.well-known/oauth-protected-resource" || r.URL.Path == "/.well-known/oauth-protected-resource/mcp"):
			respond(200, map[string]any{"resource": resource, "resource_name": "Lumi local projects", "authorization_servers": []string{base}, "scopes_supported": []string{"mcp:project"}, "bearer_methods_supported": []string{"header"}})
		case r.Method == "GET" && r.URL.Path == "/.well-known/oauth-authorization-server":
			respond(200, map[string]any{"issuer": base, "authorization_endpoint": base + "/oauth/authorize", "token_endpoint": base + "/oauth/token", "registration_endpoint": base + "/oauth/register", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "token_endpoint_auth_methods_supported": []string{"none"}, "code_challenge_methods_supported": []string{"S256"}, "scopes_supported": []string{"mcp:project", "offline_access"}, "authorization_response_iss_parameter_supported": true})
		case r.Method == "POST" && r.URL.Path == "/oauth/register":
			kind, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if kind != "application/json" {
				fail(oauthError("invalid_client_metadata", "Use application/json."))
				return
			}
			var input struct {
				ClientName      string   `json:"client_name"`
				RedirectURIs    []string `json:"redirect_uris"`
				AuthMethod      string   `json:"token_endpoint_auth_method"`
				GrantTypes      []string `json:"grant_types"`
				ResponseTypes   []string `json:"response_types"`
				ApplicationType string   `json:"application_type"`
			}
			d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
			if err := d.Decode(&input); err != nil {
				fail(oauthError("invalid_client_metadata", "Invalid JSON metadata."))
				return
			}
			if err := d.Decode(new(any)); err != io.EOF {
				fail(oauthError("invalid_client_metadata", "Expected one JSON object."))
				return
			}
			if input.AuthMethod != "none" || (input.ApplicationType != "" && input.ApplicationType != "native") {
				fail(oauthError("invalid_client_metadata", "Only public native clients (auth method none) are supported."))
				return
			}
			for _, grant := range input.GrantTypes {
				if grant != "authorization_code" && grant != "refresh_token" {
					fail(oauthError("invalid_client_metadata", "Unsupported grant type."))
					return
				}
			}
			for _, response := range input.ResponseTypes {
				if response != "code" {
					fail(oauthError("invalid_client_metadata", "Only code responses are supported."))
					return
				}
			}
			result, err := s.RegisterOAuthClient(r.Context(), input.ClientName, input.RedirectURIs)
			if err != nil {
				fail(err)
				return
			}
			respond(201, result)
		case r.Method == "GET" && r.URL.Path == "/oauth/authorize":
			values, err := url.ParseQuery(r.URL.RawQuery)
			if err != nil {
				fail(oauthError("invalid_request", "Invalid query."))
				return
			}
			id, err := s.BeginOAuth(r.Context(), values, resource)
			if err != nil {
				fail(err)
				return
			}
			http.Redirect(w, r, strings.TrimSuffix(frontend, "/")+"/settings/mcp/authorize?request_uuid="+url.QueryEscape(id), http.StatusSeeOther)
		case r.Method == "POST" && r.URL.Path == "/oauth/token":
			kind, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if kind != "application/x-www-form-urlencoded" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
				fail(oauthError("invalid_request", "Use a form body and public client_id."))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 16384)
			if err := r.ParseForm(); err != nil {
				fail(oauthError("invalid_request", "Invalid form body."))
				return
			}
			result, err := s.OAuthToken(r.Context(), r.PostForm, resource)
			if err != nil {
				fail(err)
				return
			}
			respond(200, result)
		default:
			http.NotFound(w, r)
		}
	})
}
func (s *Service) ConnectionInfo() map[string]any {
	endpoint, _ := s.connection.Load("endpoint")
	return map[string]any{"endpoint": endpoint, "transport": "streamable_http", "authorization": "oauth2.1", "local_only": true}
}
