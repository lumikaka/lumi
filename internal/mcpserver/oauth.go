package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// OAuth persistence belongs to the application database, never a project copy.
// Client IDs and request UUIDs are public UUIDv7; all relations use internal IDs.
type oauthClient struct {
	ID           int64
	UUID         string
	Name         string
	RedirectURIs string
	CreatedAt    time.Time
}

func (oauthClient) TableName() string { return "mcp_oauth_clients" }

type oauthAuthorization struct {
	ID              int64
	UUID            string
	ClientID        int64
	Resource        string
	RedirectURI     string
	Challenge       string
	State           string
	Scope           string
	RecentProjectID *int64
	Permission      string
	Status          string
	CodeHash        *string
	ExpiresAt       time.Time
	CreatedAt       time.Time
}

func (oauthAuthorization) TableName() string { return "mcp_oauth_authorizations" }

type oauthRefresh struct {
	ID        int64
	UUID      string
	GrantID   int64
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func (oauthRefresh) TableName() string { return "mcp_oauth_refresh_tokens" }

type OAuthError struct {
	Code        string
	Description string
}

func (e *OAuthError) Error() string             { return e.Description }
func oauthError(code, description string) error { return &OAuthError{code, description} }
func secret(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}
func loopbackRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "http" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	if u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" && u.Hostname() != "localhost" {
		return false
	}
	if port, err := strconv.Atoi(u.Port()); err != nil || port < 1 || port > 65535 {
		return false
	}
	for _, key := range []string{"code", "state", "iss", "error", "error_description"} {
		if u.Query().Has(key) {
			return false
		}
	}
	return true
}
func redirectMatches(registered, requested string) bool {
	// RFC 8252 permits native apps to choose a new loopback callback port.
	if !loopbackRedirect(requested) {
		return false
	}
	a, _ := url.Parse(registered)
	b, _ := url.Parse(requested)
	a.Host = a.Hostname()
	b.Host = b.Hostname()
	return a.String() == b.String()
}

// RegisterOAuthClient supports native public clients only. No remote client
// metadata is fetched, so registration cannot become an SSRF proxy.
func (s *Service) RegisterOAuthClient(ctx context.Context, name string, redirects []string) (map[string]any, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 || len(redirects) == 0 || len(redirects) > 10 {
		return nil, oauthError("invalid_client_metadata", "Provide a client_name and 1–10 loopback redirect_uris.")
	}
	for _, raw := range redirects {
		if !loopbackRedirect(raw) {
			return nil, oauthError("invalid_redirect_uri", "Only HTTP loopback callbacks with a port are supported.")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int64
	if err := s.app.DB().WithContext(ctx).Model(&oauthClient{}).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= 1024 {
		return nil, oauthError("temporarily_unavailable", "Local client registration limit reached.")
	}
	b, _ := json.Marshal(redirects)
	c := oauthClient{UUID: newUUID(), Name: name, RedirectURIs: string(b), CreatedAt: s.now().UTC()}
	if err := s.app.DB().WithContext(ctx).Create(&c).Error; err != nil {
		return nil, err
	}
	return map[string]any{"client_id": c.UUID, "client_name": c.Name, "redirect_uris": redirects, "client_id_issued_at": c.CreatedAt.Unix(), "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}}, nil
}
func validOAuthValues(v url.Values, resource string) bool {
	for key, items := range v {
		// OAuth resource indicators may be repeated. This server has only one
		// audience: every occurrence must identify this exact MCP endpoint.
		if key == "resource" {
			if len(items) == 0 {
				return false
			}
			for _, item := range items {
				if item != resource {
					return false
				}
			}
			continue
		}
		if len(items) != 1 {
			return false
		}
	}
	return true
}
func (s *Service) BeginOAuth(ctx context.Context, v url.Values, resource string) (string, error) {
	if !validOAuthValues(v, resource) || len(v.Encode()) > 8192 || v.Get("response_type") != "code" || v.Get("resource") != resource || v.Get("code_challenge_method") != "S256" {
		return "", oauthError("invalid_request", "Use response_type=code, this MCP resource, and PKCE S256.")
	}
	challenge, err := base64.RawURLEncoding.DecodeString(v.Get("code_challenge"))
	if err != nil || len(challenge) != 32 || len(v.Get("code_challenge")) != 43 {
		return "", oauthError("invalid_request", "Invalid PKCE challenge.")
	}
	scope := strings.Fields(v.Get("scope"))
	offline := false
	for _, item := range scope {
		if item == "offline_access" {
			offline = true
		}
		if item != "mcp:project" && item != "offline_access" {
			return "", oauthError("invalid_scope", "Supported scopes: mcp:project offline_access.")
		}
	}
	scope = []string{"mcp:project"}
	if offline {
		scope = append(scope, "offline_access")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	db := s.app.DB().WithContext(ctx)
	var c oauthClient
	if db.Where("uuid = ?", v.Get("client_id")).First(&c).Error != nil {
		return "", oauthError("invalid_client", "Register a local client first.")
	}
	var redirects []string
	_ = json.Unmarshal([]byte(c.RedirectURIs), &redirects)
	match := false
	for _, redirect := range redirects {
		match = match || redirectMatches(redirect, v.Get("redirect_uri"))
	}
	if !match {
		return "", oauthError("invalid_request", "redirect_uri does not match registration.")
	}
	now := s.now().UTC()
	if err := db.Where("expires_at < ?", now).Delete(&oauthAuthorization{}).Error; err != nil {
		return "", err
	}
	var count int64
	if err := db.Model(&oauthAuthorization{}).Count(&count).Error; err != nil {
		return "", err
	}
	if count >= 2048 {
		return "", oauthError("temporarily_unavailable", "Too many pending authorizations.")
	}
	a := oauthAuthorization{UUID: newUUID(), ClientID: c.ID, Resource: resource, RedirectURI: v.Get("redirect_uri"), Challenge: v.Get("code_challenge"), State: v.Get("state"), Scope: strings.Join(scope, " "), Status: "pending", CreatedAt: now, ExpiresAt: now.Add(5 * time.Minute)}
	if err := db.Create(&a).Error; err != nil {
		return "", err
	}
	return a.UUID, nil
}
func (s *Service) OAuthRequest(ctx context.Context, id string) (map[string]any, error) {
	var a oauthAuthorization
	if err := s.app.DB().WithContext(ctx).Where("uuid = ?", id).First(&a).Error; err != nil {
		return nil, err
	}
	var c oauthClient
	if err := s.app.DB().WithContext(ctx).First(&c, a.ClientID).Error; err != nil {
		return nil, err
	}
	status := a.Status
	if !a.ExpiresAt.After(s.now()) {
		status = "expired"
	}
	return map[string]any{"uuid": a.UUID, "client_name": c.Name, "client_uuid": c.UUID, "redirect_uri": a.RedirectURI, "resource": a.Resource, "status": status, "expires_at": a.ExpiresAt, "offline_access": strings.Contains(a.Scope, "offline_access")}, nil
}
func (s *Service) DecideOAuth(ctx context.Context, id, projectUUID, permission, decision string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	db := s.app.DB().WithContext(ctx)
	var a oauthAuthorization
	if err := db.Where("uuid = ?", id).First(&a).Error; err != nil {
		return "", err
	}
	if a.Status != "pending" || !a.ExpiresAt.After(s.now()) {
		return "", oauthError("invalid_request", "Authorization already handled or expired. Reconnect the client.")
	}
	callback, _ := url.Parse(a.RedirectURI)
	q := callback.Query()
	q.Set("state", a.State)
	q.Set("iss", strings.TrimSuffix(a.Resource, "/mcp"))
	switch decision {
	case "reject":
		a.Status = "denied"
		q.Set("error", "access_denied")
	case "approve":
		if permission != "read" && permission != "edit" {
			return "", oauthError("invalid_request", "Choose read or edit permission.")
		}
		if !s.projects.IsOpen(projectUUID) {
			return "", oauthError("invalid_request", "请先在 Lumi 打开所选项目。")
		}
		p, err := s.app.RecentProject(ctx, projectUUID)
		if err != nil {
			return "", err
		}
		code, err := secret("lumi_code_")
		if err != nil {
			return "", err
		}
		hash := digest([]byte(code))
		a.CodeHash = &hash
		a.RecentProjectID = &p.ID
		a.Permission = permission
		a.Status = "approved"
		q.Set("code", code)
	default:
		return "", oauthError("invalid_request", "Unknown decision.")
	}
	if err := db.Save(&a).Error; err != nil {
		return "", err
	}
	callback.RawQuery = q.Encode()
	return callback.String(), nil
}
func validVerifier(v string) bool {
	if len(v) < 43 || len(v) > 128 {
		return false
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", c)) {
			return false
		}
	}
	return true
}
func (s *Service) OAuthToken(ctx context.Context, v url.Values, resource string) (map[string]any, error) {
	// A refresh token already identifies exactly one resource. Native OAuth
	// libraries may omit resource on refresh; an explicit different target is
	// always rejected, and the persisted grant is still checked below.
	target := v.Get("resource")
	if v.Get("grant_type") == "refresh_token" && target == "" {
		target = resource
	}
	if !validOAuthValues(v, resource) || target != resource {
		return nil, oauthError("invalid_target", "resource must identify this MCP endpoint.")
	}
	if v.Get("grant_type") != "authorization_code" && v.Get("grant_type") != "refresh_token" {
		return nil, oauthError("unsupported_grant_type", "Use authorization_code or refresh_token.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	var result map[string]any
	var replay bool
	var projectUUID string
	err := s.app.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var client oauthClient
		if tx.Where("uuid = ?", v.Get("client_id")).First(&client).Error != nil {
			return oauthError("invalid_client", "Unknown client.")
		}
		var g Grant
		scope := "mcp:project offline_access"
		refreshExpiry := now.Add(7 * 24 * time.Hour)
		issueRefresh := false
		if v.Get("grant_type") == "authorization_code" {
			var a oauthAuthorization
			if tx.Where("code_hash = ? AND client_id = ? AND resource = ? AND status = ? AND expires_at > ?", digest([]byte(v.Get("code"))), client.ID, resource, "approved", now).First(&a).Error != nil || a.RedirectURI != v.Get("redirect_uri") || !validVerifier(v.Get("code_verifier")) {
				return oauthError("invalid_grant", "Invalid or expired authorization code.")
			}
			h := sha256.Sum256([]byte(v.Get("code_verifier")))
			if subtle.ConstantTimeCompare([]byte(a.Challenge), []byte(base64.RawURLEncoding.EncodeToString(h[:]))) != 1 {
				return oauthError("invalid_grant", "PKCE verification failed.")
			}
			if a.RecentProjectID == nil {
				return errors.New("authorization missing project")
			}
			g = Grant{UUID: newUUID(), RecentProjectID: *a.RecentProjectID, Name: client.Name, Permission: a.Permission, OAuthClientID: &client.ID, OAuthResource: resource, CreatedAt: now}
			scope = a.Scope
			issueRefresh = strings.Contains(scope, "offline_access")
			if err := tx.Model(&a).Update("status", "used").Error; err != nil {
				return err
			}
		} else {
			var old oauthRefresh
			if tx.Where("token_hash = ?", digest([]byte(v.Get("refresh_token")))).First(&old).Error != nil {
				return oauthError("invalid_grant", "Invalid refresh token.")
			}
			if tx.Where("id = ? AND oauth_client_id = ? AND oauth_resource = ? AND revoked_at IS NULL", old.GrantID, client.ID, resource).First(&g).Error != nil {
				return oauthError("invalid_grant", "Authorization revoked or invalid.")
			}
			if old.UsedAt != nil {
				replay = true
				var p struct{ UUID string }
				if err := tx.Table("recent_projects").Select("uuid").Where("id = ?", g.RecentProjectID).Take(&p).Error; err != nil {
					return err
				}
				projectUUID = p.UUID
				return tx.Model(&g).Update("revoked_at", now).Error // commit family revocation before returning the error
			}
			if !old.ExpiresAt.After(now) {
				return oauthError("invalid_grant", "Refresh token expired; reconnect the client.")
			}
			refreshExpiry = old.ExpiresAt
			issueRefresh = true
			if err := tx.Model(&old).Update("used_at", now).Error; err != nil {
				return err
			}
		}
		// Reading the application record does not open the project or create a second runtime.
		var p struct{ UUID string }
		if err := tx.Table("recent_projects").Select("uuid").Where("id = ?", g.RecentProjectID).Take(&p).Error; err != nil {
			return err
		}
		projectUUID = p.UUID
		token, err := secret("lumi_mcp_")
		if err != nil {
			return err
		}
		expiry := now.Add(time.Hour)
		g.TokenHash = digest([]byte(token))
		g.TokenPrefix = token[:17]
		g.ExpiresAt = &expiry
		if err := tx.Save(&g).Error; err != nil {
			return err
		}
		result = map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": 3600, "scope": scope}
		if issueRefresh {
			refresh, err := secret("lumi_refresh_")
			if err != nil {
				return err
			}
			row := oauthRefresh{UUID: newUUID(), GrantID: g.ID, TokenHash: digest([]byte(refresh)), ExpiresAt: refreshExpiry}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			result["refresh_token"] = refresh
		}
		return tx.Where("expires_at < ?", now).Delete(&oauthRefresh{}).Error
	})
	if err != nil {
		return nil, err
	}
	s.notify(projectUUID)
	if replay {
		return nil, oauthError("invalid_grant", "Refresh token reuse detected; authorization revoked.")
	}
	return result, nil
}
