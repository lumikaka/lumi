package mcpserver

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
)

type oauthFixture struct {
	s      *Service
	p      string
	client string
	values url.Values
	now    *time.Time
}

func oauthSetup(t *testing.T) oauthFixture {
	t.Helper()
	dir := t.TempDir()
	app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	projects := project.NewManager(app)
	t.Cleanup(func() { projects.Close() })
	p, err := projects.Create(t.Context(), "OAuth", project.ExplicitNewProjectParent(dir))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(app, projects, agent.NewExternalProjectAPI(nil, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	c, err := s.RegisterOAuthClient(t.Context(), "Local test", []string{"http://127.0.0.1:41000/callback"})
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(strings.Repeat("a", 43)))
	v := url.Values{"client_id": {c["client_id"].(string)}, "response_type": {"code"}, "resource": {"http://127.0.0.1:5801/mcp"}, "redirect_uri": {"http://127.0.0.1:42000/callback"}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(h[:])}, "state": {"original-state"}, "scope": {"mcp:project offline_access"}}
	return oauthFixture{s, p.UUID, c["client_id"].(string), v, &now}
}
func (f oauthFixture) approved(t *testing.T) url.Values {
	t.Helper()
	id, err := f.s.BeginOAuth(t.Context(), f.values, f.values.Get("resource"))
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := f.s.DecideOAuth(t.Context(), id, f.p, "edit", "approve")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(redirect)
	if u.Query().Get("state") != "original-state" || u.Query().Get("iss") != "http://127.0.0.1:5801" {
		t.Fatal("state or issuer lost")
	}
	return url.Values{"client_id": {f.client}, "grant_type": {"authorization_code"}, "resource": {f.values.Get("resource")}, "redirect_uri": {f.values.Get("redirect_uri")}, "code": {u.Query().Get("code")}, "code_verifier": {strings.Repeat("a", 43)}}
}
func TestOAuthCodeBindingExpiryAndRefreshReplay(t *testing.T) {
	f := oauthSetup(t)
	v := f.approved(t)
	resource := v.Get("resource")
	for key, bad := range map[string]string{"client_id": newUUID(), "redirect_uri": "http://127.0.0.1:42000/changed", "resource": "http://127.0.0.1:32323/mcp", "code_verifier": strings.Repeat("b", 43)} {
		original := v.Get(key)
		v.Set(key, bad)
		if _, err := f.s.OAuthToken(t.Context(), v, resource); err == nil {
			t.Fatalf("accepted tampered %s", key)
		}
		v.Set(key, original)
	}
	result, err := f.s.OAuthToken(t.Context(), v, resource)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.OAuthToken(t.Context(), v, resource); err == nil {
		t.Fatal("code replay")
	}
	token := result["access_token"].(string)
	g, err := f.s.Authenticate(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(g)
	if strings.Contains(string(public), `"id"`) || strings.Contains(string(public), "token_hash") {
		t.Fatal("internal ID/hash leaked")
	}
	*f.now = f.now.Add(61 * time.Minute)
	if _, err := f.s.Authenticate(t.Context(), token); err == nil {
		t.Fatal("expired access accepted")
	}
	refresh := url.Values{"grant_type": {"refresh_token"}, "client_id": {f.client}, "refresh_token": {result["refresh_token"].(string)}}
	next, err := f.s.OAuthToken(t.Context(), refresh, resource)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Authenticate(t.Context(), next["access_token"].(string)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.OAuthToken(t.Context(), refresh, resource); err == nil {
		t.Fatal("refresh replay accepted")
	}
	if _, err := f.s.Authenticate(t.Context(), next["access_token"].(string)); err == nil {
		t.Fatal("replay did not revoke family")
	}
}
func TestOAuthRejectExpireCloseAndRestart(t *testing.T) {
	f := oauthSetup(t)
	resource := f.values.Get("resource")
	id, err := f.s.BeginOAuth(t.Context(), f.values, resource)
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := f.s.DecideOAuth(t.Context(), id, "", "", "reject")
	if err != nil || !strings.Contains(redirect, "access_denied") {
		t.Fatalf("reject: %s %v", redirect, err)
	}
	if _, err := f.s.DecideOAuth(t.Context(), id, f.p, "edit", "approve"); err == nil {
		t.Fatal("rejected request approved")
	}
	id, err = f.s.BeginOAuth(t.Context(), f.values, resource)
	if err != nil {
		t.Fatal(err)
	}
	*f.now = f.now.Add(6 * time.Minute)
	view, err := f.s.OAuthRequest(t.Context(), id)
	if err != nil || view["status"] != "expired" {
		t.Fatalf("expiry %+v %v", view, err)
	}
	if _, err := f.s.DecideOAuth(t.Context(), id, f.p, "read", "approve"); err == nil {
		t.Fatal("expired approval")
	}
	v := f.approved(t)
	if closed, err := f.s.projects.CloseProject(t.Context(), f.p); err != nil || !closed {
		t.Fatalf("close: %v %v", closed, err)
	}
	pending, err := f.s.BeginOAuth(t.Context(), f.values, resource)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DecideOAuth(t.Context(), pending, f.p, "read", "approve"); err == nil {
		t.Fatal("closed project authorized")
	}
	// Service reconstruction preserves the pending code and all binding fields.
	restarted, err := New(f.s.app, f.s.projects, f.s.api, nil)
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = f.s.now
	tokens, err := restarted.OAuthToken(t.Context(), v, resource)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := restarted.Authenticate(t.Context(), tokens["access_token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	result := restarted.Call(t.Context(), grant, map[string]any{"method": "GET", "url": "/api/v1/projects/" + f.p, "response_filter": ".data | {uuid,name,revision}"}, "")
	if result["success"] != false || result["error"].(map[string]any)["code"] != "project_not_open" {
		t.Fatalf("closed project: %+v", result)
	}
	if err := restarted.Revoke(t.Context(), f.p, grant.UUID); err != nil {
		t.Fatal(err)
	}
	refresh := url.Values{"grant_type": {"refresh_token"}, "client_id": {f.client}, "resource": {resource}, "refresh_token": {tokens["refresh_token"].(string)}}
	if _, err := restarted.OAuthToken(t.Context(), refresh, resource); err == nil {
		t.Fatal("revoked refresh accepted")
	}
}
func TestOAuthValidationAndLocalHTTPBoundary(t *testing.T) {
	f := oauthSetup(t)
	resource := f.values.Get("resource")
	for _, raw := range []string{"https://example.com/callback", "http://127.0.0.1.evil.test:123/cb", "http://user@127.0.0.1:123/cb", "http://127.0.0.1:123/cb#fragment", "http://127.0.0.1:123/cb?code=x", "file:///tmp/cb"} {
		if _, err := f.s.RegisterOAuthClient(t.Context(), "bad", []string{raw}); err == nil {
			t.Fatalf("redirect accepted: %s", raw)
		}
	}
	for key, value := range map[string]string{"code_challenge_method": "plain", "response_type": "token", "scope": "admin", "redirect_uri": "http://127.0.0.1:42000/other", "resource": "https://example.com/mcp"} {
		original := f.values.Get(key)
		f.values.Set(key, value)
		if _, err := f.s.BeginOAuth(t.Context(), f.values, resource); err == nil {
			t.Fatalf("accepted %s", key)
		}
		f.values.Set(key, original)
	}
	f.values.Add("state", "duplicate")
	if _, err := f.s.BeginOAuth(t.Context(), f.values, resource); err == nil {
		t.Fatal("duplicate accepted")
	}
	f.values.Set("state", "original-state")
	handler := f.s.LocalHTTP("http://127.0.0.1:5801", "http://127.0.0.1:5801", "nonce")
	for _, tc := range []struct {
		path, host, origin, nonce string
		status                    int
	}{
		{"/mcp", "127.0.0.1:5801", "", "", 401},
		{"/.well-known/oauth-authorization-server", "evil.test:5801", "", "", 403},
		{"/mcp", "127.0.0.1:5801", "http://evil.test", "", 403},
		{"/mcp", "127.0.0.1:5801", "", "stale", 403},
		{"/api/v1/projects", "127.0.0.1:5801", "", "", 404},
	} {
		r := httptest.NewRequest("GET", "http://"+tc.host+tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-Lumi-Instance", tc.nonce)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
	v := f.approved(t)
	tokens, err := f.s.OAuthToken(t.Context(), v, resource)
	if err != nil {
		t.Fatal(err)
	}
	other := f.s.LocalHTTP("http://127.0.0.1:32323", "http://127.0.0.1:5801", "nonce")
	r := httptest.NewRequest("POST", "http://127.0.0.1:32323/mcp", nil)
	r.Header.Set("Authorization", "Bearer "+tokens["access_token"].(string))
	w := httptest.NewRecorder()
	other.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatal("wrong audience accepted")
	}
}

func TestOAuthPluginRegistrationIsStableAndNarrow(t *testing.T) {
	f := oauthSetup(t)
	for _, clientID := range []string{ProductionPluginClientUUID, DevelopmentPluginClientUUID} {
		var before oauthClient
		if err := f.s.app.DB().Where("uuid = ?", clientID).First(&before).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.s.ensurePluginClients(); err != nil {
			t.Fatal(err)
		}
		var after oauthClient
		if err := f.s.app.DB().Where("uuid = ?", clientID).First(&after).Error; err != nil || after.ID != before.ID {
			t.Fatalf("pre-registration changed identity: %v", err)
		}
		f.values.Set("client_id", clientID)
		for _, redirect := range []string{"http://127.0.0.1:48555/callback", "http://127.0.0.1:48556/callback"} {
			f.values.Set("redirect_uri", redirect)
			if _, err := f.s.BeginOAuth(t.Context(), f.values, f.values.Get("resource")); err != nil {
				t.Fatal(err)
			}
		}
		for _, redirect := range []string{"http://localhost:48555/callback", "http://127.0.0.1:48555/other", "http://127.0.0.1:48555/callback/unknown", "http://127.0.0.1:48555/callback?next=elsewhere", "http://127.0.0.1/callback", "https://evil.example/callback"} {
			f.values.Set("redirect_uri", redirect)
			if _, err := f.s.BeginOAuth(t.Context(), f.values, f.values.Get("resource")); err == nil {
				t.Fatalf("accepted unregistered redirect %s", redirect)
			}
		}
	}
	f.values.Set("client_id", "unregistered-plugin")
	f.values.Set("redirect_uri", "http://127.0.0.1:48555/callback")
	if _, err := f.s.BeginOAuth(t.Context(), f.values, f.values.Get("resource")); err == nil {
		t.Fatal("unknown fixed client accepted")
	}
}

func TestOAuthCodexDynamicCallbackAndRepeatedResource(t *testing.T) {
	f := oauthSetup(t)
	resource := f.values.Get("resource")
	callback := "http://127.0.0.1:50132/callback/VJL--Uj9wWIv"
	client, err := f.s.RegisterOAuthClient(t.Context(), "Codex", []string{callback})
	if err != nil {
		t.Fatal(err)
	}
	f.client = client["client_id"].(string)
	f.values.Set("client_id", f.client)
	f.values.Set("redirect_uri", callback)
	f.values.Add("resource", resource)
	for _, values := range [][]string{{resource, "http://127.0.0.1:32323/mcp"}, {"http://127.0.0.1:32323/mcp", resource}, {resource, ""}} {
		f.values["resource"] = values
		if _, err := f.s.BeginOAuth(t.Context(), f.values, resource); err == nil {
			t.Fatal("accepted mixed resource indicators")
		}
	}
	f.values["resource"] = []string{resource, resource}
	f.values.Set("redirect_uri", "http://127.0.0.1:50132/callback/another-server")
	if _, err := f.s.BeginOAuth(t.Context(), f.values, resource); err == nil {
		t.Fatal("accepted a callback not registered by this client")
	}
	f.values.Set("redirect_uri", callback)
	v := f.approved(t)
	for _, key := range []string{"client_id", "redirect_uri", "code", "code_verifier", "grant_type"} {
		original := v.Get(key)
		v.Add(key, original)
		if _, err := f.s.OAuthToken(t.Context(), v, resource); err == nil {
			t.Fatalf("accepted duplicated %s", key)
		}
		v.Set(key, original)
	}
	v.Add("resource", "http://127.0.0.1:32323/mcp")
	if _, err := f.s.OAuthToken(t.Context(), v, resource); err == nil {
		t.Fatal("accepted mixed token resource indicators")
	}
	v["resource"] = []string{resource, resource}
	tokens, err := f.s.OAuthToken(t.Context(), v, resource)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := f.s.Authenticate(t.Context(), tokens["access_token"].(string))
	if err != nil || grant.ProjectUUID != f.p || grant.Permission != "edit" {
		t.Fatalf("authorization lost its project/permission binding: %v", err)
	}
	refresh := url.Values{"grant_type": {"refresh_token"}, "client_id": {f.client}, "refresh_token": {tokens["refresh_token"].(string)}, "resource": {resource, resource}}
	if _, err := f.s.OAuthToken(t.Context(), refresh, resource); err != nil {
		t.Fatal(err)
	}
}
