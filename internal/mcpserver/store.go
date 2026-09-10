// Package mcpserver owns local project capabilities and durable external calls.
// It never opens a project by path; project stores belong to Project Manager.
package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/project"
)

type Grant struct {
	ID              int64      `json:"-"`
	UUID            string     `json:"uuid"`
	RecentProjectID int64      `json:"-"`
	Name            string     `json:"name"`
	Permission      string     `json:"permission"`
	TokenHash       string     `json:"-"`
	TokenPrefix     string     `json:"token_prefix"`
	CreatedAt       time.Time  `json:"created_at"`
	RevokedAt       *time.Time `json:"revoked_at"`
	LastUsedAt      *time.Time `json:"last_used_at"`
	ProjectUUID     string     `gorm:"-" json:"project_uuid"`
	OAuthClientID   *int64     `gorm:"column:oauth_client_id" json:"-"`
	OAuthResource   string     `gorm:"column:oauth_resource" json:"oauth_resource,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at"`
}

func (Grant) TableName() string { return "mcp_grants" }

type Call struct {
	ID             int64     `json:"-"`
	UUID           string    `json:"uuid"`
	GrantID        int64     `json:"-"`
	IdempotencyKey string    `json:"idempotency_key"`
	Fingerprint    string    `json:"fingerprint"`
	Arguments      string    `json:"-"`
	Action         string    `json:"action"`
	Status         string    `json:"status"`
	Async          bool      `json:"async"`
	Result         string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

func (Call) TableName() string { return "mcp_calls" }

type Service struct {
	app             *appstore.Store
	projects        *project.Manager
	api             *agent.ExternalProjectAPI
	mu              sync.Mutex // serializes admission/confirmation with revoke in this runtime
	changed         func(string)
	now             func() time.Time
	running         sync.Map
	connection      sync.Map
	activityMu      sync.Mutex
	activityRunning sync.Map
}

func New(app *appstore.Store, projects *project.Manager, api *agent.ExternalProjectAPI, changed func(string)) (*Service, error) {
	s := &Service{app: app, projects: projects, api: api, changed: changed, now: time.Now}
	if err := s.ensurePluginClients(); err != nil {
		return nil, err
	}
	// A process crash cannot establish whether a normal write committed. Never
	// automatically repeat it. Async submissions can be recovered by durable key.
	err := app.DB().Model(&Call{}).Where("status = ? AND async = ?", "executing", false).Update("status", "interrupted").Error
	if err == nil {
		projects.WithOpenHook(func(ctx context.Context, store *project.Store) error {
			return s.reconcileActivities(ctx, store)
		})
	}
	return s, err
}
func newUUID() string        { return uuid.Must(uuid.NewV7()).String() }
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func failure(code, message string) map[string]any {
	return map[string]any{"success": false, "data": nil, "error": map[string]any{"code": code, "message": message, "details": ""}}
}
func success(data any) map[string]any { return map[string]any{"success": true, "data": data} }
func publicError(err error) map[string]any {
	var e *agent.Error
	if errors.As(err, &e) {
		return failure(e.Code, e.Message+" "+e.Details)
	}
	var p *project.Error
	if errors.As(err, &p) {
		return failure(p.Code, p.Message+" "+p.Details)
	}
	return failure("mcp_operation_failed", "操作失败，请在 Lumi 查看项目状态。")
}

var ErrUnauthorized = errors.New("mcp_unauthorized")

func (s *Service) Authenticate(ctx context.Context, token string) (Grant, error) {
	if !strings.HasPrefix(token, "lumi_mcp_") || len(token) != 73 {
		return Grant{}, ErrUnauthorized
	}
	var g Grant
	err := s.app.DB().WithContext(ctx).Where("token_hash = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", digest([]byte(token)), s.now().UTC()).First(&g).Error
	if err != nil {
		return Grant{}, ErrUnauthorized
	}
	var p appstore.RecentProject
	if err = s.app.DB().WithContext(ctx).First(&p, g.RecentProjectID).Error; err != nil {
		return Grant{}, ErrUnauthorized
	}
	g.ProjectUUID = p.UUID
	if err = s.app.DB().WithContext(ctx).Model(&g).Update("last_used_at", s.now().UTC()).Error; err != nil {
		return Grant{}, err
	}
	return g, nil
}
func (s *Service) valid(ctx context.Context, g Grant) bool {
	var n int64
	return s.app.DB().WithContext(ctx).Model(&Grant{}).Where("id = ? AND token_hash = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", g.ID, g.TokenHash, s.now().UTC()).Count(&n).Error == nil && n == 1
}
func (s *Service) CreateGrant(ctx context.Context, projectUUID, name, permission string) (Grant, string, error) {
	if !s.projects.IsOpen(projectUUID) {
		return Grant{}, "", errors.New("请先在 Lumi 打开项目")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 || (permission != "read" && permission != "edit") {
		return Grant{}, "", errors.New("名称必须为 1–120 字节，权限必须为 read 或 edit")
	}
	p, err := s.app.RecentProject(ctx, projectUUID)
	if err != nil {
		return Grant{}, "", err
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return Grant{}, "", err
	}
	token := "lumi_mcp_" + hex.EncodeToString(b)
	g := Grant{UUID: newUUID(), RecentProjectID: p.ID, ProjectUUID: projectUUID, Name: name, Permission: permission, TokenHash: digest([]byte(token)), TokenPrefix: token[:17], CreatedAt: s.now().UTC()}
	err = s.app.DB().WithContext(ctx).Create(&g).Error
	s.notify(projectUUID)
	return g, token, err
}
func (s *Service) Grants(ctx context.Context, p string) ([]Grant, error) {
	items := []Grant{}
	err := s.app.DB().WithContext(ctx).Where("recent_project_id IN (SELECT id FROM recent_projects WHERE uuid = ?)", p).Order("id DESC").Find(&items).Error
	for i := range items {
		items[i].ProjectUUID = p
	}
	return items, err
}
func (s *Service) Revoke(ctx context.Context, p, u string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.app.DB().WithContext(ctx).Model(&Grant{}).Where("uuid = ? AND recent_project_id IN (SELECT id FROM recent_projects WHERE uuid = ?)", u, p).Update("revoked_at", s.now().UTC())
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	s.notify(p)
	return nil
}
func (s *Service) notify(p string) {
	if s.changed != nil {
		s.changed(p)
	}
}
func (s *Service) Call(ctx context.Context, g Grant, args map[string]any, key string) (result map[string]any) {
	recorded := false
	defer func() {
		if recorded || !s.projects.IsOpen(g.ProjectUUID) || !s.valid(ctx, g) {
			return
		}
		row, err := s.beginActivity(ctx, g, "request_api", activitySpec{action: "operation:invalid", label: "调用项目接口"}, nil)
		if err == nil {
			s.finishActivity(g.ProjectUUID, row, "failed", activityError(result))
		}
	}()
	if len(key) > 128 {
		return failure("mcp_invalid_key", "idempotency_key 最多 128 字节。")
	}
	req, err := s.api.Prepare(g.ProjectUUID, args)
	if err != nil {
		return publicError(err)
	}
	if !req.ReadOnly() && g.Permission != "edit" {
		return failure("mcp_read_only", "该授权只允许读取。")
	}
	if !req.ReadOnly() && strings.TrimSpace(key) == "" {
		return failure("mcp_idempotency_required", "写操作必须提供稳定的 idempotency_key；重试使用同一 key。")
	}
	if key == "" {
		key = newUUID()
	}
	s.mu.Lock()
	if !s.valid(ctx, g) {
		s.mu.Unlock()
		return failure("mcp_unauthorized", "授权已撤销。")
	}
	encoded, _ := json.Marshal(args)
	fp := digest(encoded)
	var c Call
	err = s.app.DB().WithContext(ctx).Where("grant_id = ? AND idempotency_key = ?", g.ID, key).First(&c).Error
	if err == nil {
		s.mu.Unlock()
		if c.Fingerprint != fp {
			return failure("mcp_idempotency_conflict", "同一幂等键不能用于不同请求。")
		}
		recorded = true
		// A replay is a read of an existing operation, not another execution.
		replay, recordErr := s.beginActivity(ctx, g, "get_call", activitySpec{action: "read", label: "读取调用结果"}, nil)
		if recordErr == nil {
			s.finishActivity(g.ProjectUUID, replay, "succeeded", "")
		}
		if c.Status == "executing" && c.Async {
			return s.execute(g, c, req)
		}
		return s.callResult(c)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		s.mu.Unlock()
		return publicError(err)
	}
	if !s.projects.IsOpen(g.ProjectUUID) {
		s.mu.Unlock()
		return failure(project.CodeProjectNotOpen, "项目尚未打开；请先在 Lumi 打开已授权项目，再重试。")
	}
	now := s.now().UTC()
	status := "executing"
	if req.Dangerous() {
		status = "pending_confirmation"
	}
	c = Call{UUID: newUUID(), GrantID: g.ID, IdempotencyKey: key, Fingerprint: fp, Arguments: string(encoded), Action: req.Action(), Status: status, Async: req.Async(), CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(15 * time.Minute)}
	recorded = true
	activity, activityErr := s.beginActivity(ctx, g, "request_api", requestActivity(req), &c.UUID)
	if activityErr != nil {
		s.mu.Unlock()
		return publicError(activityErr)
	}
	defer s.activityRunning.Delete(activity.UUID)
	err = s.app.DB().WithContext(ctx).Create(&c).Error
	s.mu.Unlock()
	if err != nil {
		s.finishActivity(g.ProjectUUID, activity, "failed", "操作未受理")
		return publicError(err)
	}
	s.syncCallActivity(g.ProjectUUID, c)
	if c.Status == "pending_confirmation" {
		return s.callResult(c)
	}
	return s.execute(g, c, req)
}
func (s *Service) execute(g Grant, c Call, req agent.ExternalRequest) map[string]any {
	if _, loaded := s.running.LoadOrStore(c.UUID, true); loaded {
		return s.callResult(c)
	}
	defer s.running.Delete(c.UUID)
	// Once admitted, an operation survives client cancellation, with a bounded
	// request lease. Background jobs belong to the persisted runtime thereafter.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var value any
	err := s.projects.WithStore(ctx, g.ProjectUUID, func(store *project.Store) error {
		if store.SetupStatus() != project.SetupStatusReady && !req.ReadOnly() {
			return &agent.Error{Code: project.CodeProjectSetupIncomplete, Message: "请先在 Lumi 完成项目设置。"}
		}
		var err error
		value, err = s.api.Execute(ctx, req, c.UUID)
		return err
	})
	result := success(value)
	c.Status = "succeeded"
	if err != nil {
		result = publicError(err)
		c.Status = "failed"
	}
	encoded, _ := json.Marshal(result)
	c.Result = string(encoded)
	c.UpdatedAt = s.now().UTC()
	if err = s.app.DB().Model(&Call{}).Where("id = ?", c.ID).Updates(map[string]any{"status": c.Status, "result": c.Result, "updated_at": c.UpdatedAt}).Error; err != nil {
		return failure("mcp_result_unavailable", "操作结果未能保存，请检查项目状态，不要使用新幂等键重放。")
	}
	s.syncCallActivity(g.ProjectUUID, c)
	return s.callResult(c)
}
func (s *Service) callResult(c Call) map[string]any {
	var result any
	if c.Result != "" {
		_ = json.Unmarshal([]byte(c.Result), &result)
	}
	if c.Status == "pending_confirmation" && !s.now().Before(c.ExpiresAt) {
		s.app.DB().Model(&c).Where("status = ?", "pending_confirmation").Update("status", "expired")
		c.Status = "expired"
	}
	var g Grant
	_ = s.app.DB().First(&g, c.GrantID).Error
	var p appstore.RecentProject
	_ = s.app.DB().First(&p, g.RecentProjectID).Error
	if c.Status == "expired" {
		s.syncCallActivity(p.UUID, c)
	}
	return success(map[string]any{"call": c, "result": result, "source": "external_mcp", "grant_uuid": g.UUID, "project_uuid": p.UUID, "status_url": "mcp://calls/" + c.UUID})
}
func (s *Service) GetCall(ctx context.Context, g Grant, u string) map[string]any {
	if !s.valid(ctx, g) {
		return failure("mcp_unauthorized", "授权已撤销。")
	}
	var c Call
	if err := s.app.DB().WithContext(ctx).Where("grant_id = ? AND uuid = ?", g.ID, u).First(&c).Error; err != nil {
		return failure("mcp_call_not_found", "调用不存在或不属于该授权。")
	}
	return s.callResult(c)
}
func (s *Service) Calls(ctx context.Context, p string) ([]map[string]any, error) {
	s.app.DB().Model(&Call{}).Where("status = ? AND expires_at <= ?", "pending_confirmation", s.now().UTC()).Update("status", "expired")
	var calls []Call
	err := s.app.DB().WithContext(ctx).Where("grant_id IN (SELECT g.id FROM mcp_grants g JOIN recent_projects p ON p.id=g.recent_project_id WHERE p.uuid = ?)", p).Order("CASE WHEN status = 'pending_confirmation' THEN 0 ELSE 1 END, id DESC").Limit(100).Find(&calls).Error
	items := []map[string]any{}
	for _, c := range calls {
		var args any
		_ = json.Unmarshal([]byte(c.Arguments), &args)
		var result any
		if c.Result != "" {
			_ = json.Unmarshal([]byte(c.Result), &result)
		}
		var grant Grant
		_ = s.app.DB().First(&grant, c.GrantID).Error
		items = append(items, map[string]any{"call": c, "request": args, "result": result, "grant_uuid": grant.UUID, "client_name": grant.Name})
	}
	return items, err
}
func (s *Service) Decide(ctx context.Context, p, u, fingerprint, decision string) map[string]any {
	if decision != "approve" && decision != "reject" {
		return failure("mcp_invalid_decision", "decision 必须为 approve 或 reject。")
	}
	s.mu.Lock()
	var c Call
	err := s.app.DB().Where("uuid = ? AND grant_id IN (SELECT g.id FROM mcp_grants g JOIN recent_projects p ON p.id=g.recent_project_id WHERE p.uuid = ?)", u, p).First(&c).Error
	if err != nil {
		s.mu.Unlock()
		return failure("mcp_call_not_found", "调用不存在。")
	}
	if c.Status != "pending_confirmation" {
		s.mu.Unlock()
		return s.callResult(c)
	}
	if c.Fingerprint != fingerprint || digest([]byte(c.Arguments)) != c.Fingerprint {
		s.mu.Unlock()
		return failure("mcp_fingerprint_mismatch", "请求指纹不匹配。")
	}
	var g Grant
	if s.app.DB().First(&g, c.GrantID).Error != nil || g.RevokedAt != nil {
		s.mu.Unlock()
		return failure("mcp_unauthorized", "原授权已撤销。")
	}
	g.ProjectUUID = p
	var args map[string]any
	_ = json.Unmarshal([]byte(c.Arguments), &args)
	req, err := s.api.Prepare(p, args)
	if err != nil {
		s.mu.Unlock()
		return publicError(err)
	}
	c.Status = "executing"
	if decision == "reject" {
		c.Status = "rejected"
	}
	if !s.now().Before(c.ExpiresAt) {
		c.Status = "expired"
	}
	r := s.app.DB().Model(&Call{}).Where("id = ? AND status = ?", c.ID, "pending_confirmation").Update("status", c.Status)
	s.mu.Unlock()
	if r.Error != nil {
		return publicError(r.Error)
	}
	if r.RowsAffected != 1 {
		return failure("mcp_state_conflict", "请求状态已变化。")
	}
	s.syncCallActivity(p, c)
	if c.Status != "executing" {
		return s.callResult(c)
	}
	return s.execute(g, c, req)
}
