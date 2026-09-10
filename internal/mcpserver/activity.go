package mcpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"lumi/internal/agent"
	"lumi/internal/project"
)

const activitySessionGap = 30 * time.Minute

type activityThread struct {
	ID           int64
	UUID         string
	ThreadID     int64
	GrantUUID    string
	ClientName   string
	StartedAt    time.Time
	LastCallAt   time.Time
	NextSequence int64
	Revision     int64
}

func (activityThread) TableName() string { return "mcp_threads" }

type activityRecord struct {
	ID           int64
	UUID         string
	MCPThreadID  int64 `gorm:"column:mcp_thread_id"`
	Sequence     int64
	CallUUID     *string
	ToolName     string
	Action       string
	ResourceKey  string
	Label        string
	Status       string
	ErrorMessage string
	AdmittedAt   time.Time
}

func (activityRecord) TableName() string { return "mcp_thread_activities" }

type activitySpec struct{ action, resource, label string }

// Only reviewed route metadata is used here, never a request body or response.
func requestActivity(req agent.ExternalRequest) activitySpec {
	action, resource := req.ActivityIdentity()
	return activitySpec{action, resource, req.Action()}
}

func (s *Service) beginActivity(ctx context.Context, g Grant, tool string, spec activitySpec, callUUID *string) (activityRecord, error) {
	s.activityMu.Lock()
	defer s.activityMu.Unlock()
	now := s.now().UTC()
	row := activityRecord{UUID: newUUID(), ToolName: tool, Action: spec.action, ResourceKey: spec.resource, Label: spec.label, Status: "executing", AdmittedAt: now, CallUUID: callUUID}
	s.activityRunning.Store(row.UUID, true)
	err := s.projects.WithStore(ctx, g.ProjectUUID, func(store *project.Store) error {
		return store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			row.Label = activityLabel(tx, spec)
			var pid int64
			if err := tx.Raw("SELECT id FROM projects WHERE uuid=?", g.ProjectUUID).Scan(&pid).Error; err != nil {
				return err
			}
			var session activityThread
			err := tx.Table("mcp_threads AS m").Select("m.*").Joins("JOIN chat_threads t ON t.id=m.thread_id").Where("m.grant_uuid=? AND t.project_id=? AND t.archived_at IS NULL", g.UUID, pid).Order("m.id DESC").Take(&session).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if errors.Is(err, gorm.ErrRecordNotFound) || !now.Before(session.LastCallAt.Add(activitySessionGap)) {
				threadUUID := newUUID()
				if err := tx.Exec(`INSERT INTO chat_threads(uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,created_at,updated_at) VALUES(?,?,?,'idle','mcp','','','',?,?)`, threadUUID, pid, row.Label, now, now).Error; err != nil {
					return err
				}
				var tid int64
				if err := tx.Raw("SELECT id FROM chat_threads WHERE uuid=?", threadUUID).Scan(&tid).Error; err != nil {
					return err
				}
				session = activityThread{UUID: newUUID(), ThreadID: tid, GrantUUID: g.UUID, ClientName: safeActivityText(g.Name, 120), StartedAt: now, LastCallAt: now, NextSequence: 1}
				if err := tx.Create(&session).Error; err != nil {
					return err
				}
			}
			row.MCPThreadID, row.Sequence = session.ID, session.NextSequence
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			if err := tx.Model(&session).Updates(map[string]any{"last_call_at": now, "next_sequence": session.NextSequence + 1, "revision": gorm.Expr("revision+1")}).Error; err != nil {
				return err
			}
			if spec.action != "read" {
				return tx.Exec("UPDATE chat_threads SET title=?,updated_at=? WHERE id=?", row.Label, now, session.ThreadID).Error
			}
			return tx.Exec("UPDATE chat_threads SET updated_at=? WHERE id=?", now, session.ThreadID).Error
		})
	})
	if err != nil {
		s.activityRunning.Delete(row.UUID)
	} else {
		s.notify(g.ProjectUUID)
	}
	return row, err
}

// Resource titles are snapshots from local facts, not request payloads. Only
// explicitly supported resource tables can be read by this display helper.
func activityLabel(tx *gorm.DB, spec activitySpec) string {
	parts := strings.Split(strings.Trim(spec.resource, "/"), "/")
	for i := len(parts) - 2; i >= 0; i-- {
		if !validPublicUUID(parts[i+1]) {
			continue
		}
		table := ""
		switch parts[i] {
		case "chapters":
			table = "chapters"
		case "sections":
			table = "comic_sections"
		case "assets":
			if strings.Contains(spec.resource, "/premise/") {
				table = "premise_assets"
			}
		}
		if table == "" {
			continue
		}
		var title string
		if tx.Table(table).Select("title").Where("uuid=?", parts[i+1]).Scan(&title).Error == nil && strings.TrimSpace(title) != "" {
			return safeActivityText(spec.label, 60) + " · " + safeActivityText(title, 60)
		}
	}
	return safeActivityText(spec.label, 120)
}

func safeActivityText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	r := []rune(value)
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return value
}

// Public failures deliberately use a bounded code-to-copy map. Raw provider or
// dispatcher errors may include paths, secrets or content and are not history.
func activityError(result map[string]any) string {
	e, _ := result["error"].(map[string]any)
	code, _ := e["code"].(string)
	switch {
	case strings.Contains(code, "revision") || strings.Contains(code, "conflict"):
		return "内容版本或状态已变化"
	case strings.Contains(code, "not_found"):
		return "资源不存在"
	case strings.Contains(code, "unauthorized") || strings.Contains(code, "read_only"):
		return "没有操作权限"
	case strings.Contains(code, "validation") || strings.Contains(code, "invalid"):
		return "请求参数无效"
	case strings.Contains(code, "setup_incomplete"):
		return "项目设置尚未完成"
	default:
		return "操作未成功，请查看项目状态"
	}
}

func (s *Service) finishActivity(p string, row activityRecord, status, message string) {
	defer s.activityRunning.Delete(row.UUID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Failure to record history must never change or replay a business result.
	if err := s.projects.WithStore(ctx, p, func(store *project.Store) error {
		return updateActivity(store.DB().WithContext(ctx), row.UUID, status, message)
	}); err == nil {
		s.notify(p)
	}
}

func updateActivity(db *gorm.DB, u, status, message string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var row activityRecord
		if err := tx.Where("uuid=?", u).First(&row).Error; err != nil {
			return err
		}
		if row.Status == status && row.ErrorMessage == message {
			return nil
		}
		// Terminal call outcomes are immutable; a concurrent reconciliation
		// snapshot must not restore an older pending state over a final result.
		if row.Status != "executing" && row.Status != "pending_confirmation" && row.Status != "interrupted" {
			return nil
		}
		if err := tx.Model(&row).Updates(map[string]any{"status": status, "error_message": message}).Error; err != nil {
			return err
		}
		return tx.Model(&activityThread{}).Where("id=?", row.MCPThreadID).UpdateColumn("revision", gorm.Expr("revision+1")).Error
	})
}

// Reconcile only unfinished MCP *calls*. Never inspect or follow background
// tasks. On restart an unrecorded ordinary write remains interrupted.
func (s *Service) reconcileActivities(ctx context.Context, store *project.Store) error {
	var rows []activityRecord
	if err := store.DB().WithContext(ctx).Where("status IN ?", []string{"executing", "pending_confirmation"}).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if _, running := s.activityRunning.Load(row.UUID); running {
			continue
		}
		status, message := "interrupted", "调用结果不明"
		if row.CallUUID != nil {
			var call Call
			err := s.app.DB().WithContext(ctx).Where("uuid=?", *row.CallUUID).Take(&call).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				status = call.Status
				if status == "pending_confirmation" && !s.now().Before(call.ExpiresAt) {
					status = "expired"
				}
				if status == "executing" {
					if _, running := s.running.Load(call.UUID); running {
						continue
					}
					// Only explicit MCP recovery may resolve an unrecorded
					// submission. History never follows its background task.
					status = "interrupted"
				}
				message = ""
				if status == "failed" {
					var result map[string]any
					_ = json.Unmarshal([]byte(call.Result), &result)
					message = activityError(result)
				}
				if status == "interrupted" {
					message = "调用结果不明"
				}
			}
		}
		if err := updateActivity(store.DB().WithContext(ctx), row.UUID, status, message); err != nil {
			return err
		}
	}
	return nil
}

// syncCallActivity also covers confirmation decisions made outside tools/call.
func (s *Service) syncCallActivity(p string, call Call) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.projects.WithStore(ctx, p, func(store *project.Store) error {
		var row activityRecord
		if err := store.DB().Where("call_uuid=?", call.UUID).Take(&row).Error; err != nil {
			return err
		}
		message := ""
		if call.Status == "failed" {
			var result map[string]any
			_ = json.Unmarshal([]byte(call.Result), &result)
			message = activityError(result)
		}
		if call.Status == "interrupted" {
			message = "调用结果不明"
		}
		return updateActivity(store.DB().WithContext(ctx), row.UUID, call.Status, message)
	})
	s.notify(p)
}

type ActivitySummary struct {
	UUID       string    `json:"uuid"`
	Text       string    `json:"text"`
	Status     string    `json:"status"`
	AdmittedAt time.Time `json:"admitted_at"`
	labels     []string
}

type ActivityPage struct {
	ClientName       string                 `json:"client_name"`
	StartedAt        time.Time              `json:"started_at"`
	LastCallAt       time.Time              `json:"last_call_at"`
	Items            []ActivitySummary      `json:"items"`
	CursorPagination agent.CursorPagination `json:"cursor_pagination"`
}

func collapseActivities(rows []activityRecord) []ActivitySummary {
	result := []ActivitySummary{}
	var previous *activityRecord
	for i := range rows {
		r := &rows[i]
		merge := previous != nil && previous.Sequence+1 == r.Sequence && previous.Status == "succeeded" && r.Status == "succeeded" &&
			((previous.Action == "read" && r.Action == "read") || (strings.HasPrefix(r.Action, "modify:") && previous.Action == r.Action && previous.ResourceKey == r.ResourceKey && r.ResourceKey != ""))
		if r.Status == "executing" {
			previous = r
			continue
		}
		if merge && len(result) > 0 {
			last := &result[len(result)-1]
			if r.Action == "read" {
				found := false
				for _, label := range last.labels {
					if label == r.Label {
						found = true
					}
				}
				if !found && len(last.labels) < 3 {
					last.labels = append(last.labels, r.Label)
				}
				last.Text = "已" + strings.Join(last.labels, "、")
			} else {
				last.Text = "已" + r.Label
			}
		} else {
			text := "已" + r.Label
			switch r.Status {
			case "pending_confirmation":
				text = r.Label + "：等待确认"
			case "failed":
				text = r.Label + "失败：" + r.ErrorMessage
			case "rejected":
				text = r.Label + "：已拒绝"
			case "expired":
				text = r.Label + "：确认已过期"
			case "interrupted":
				text = r.Label + "：调用结果不明"
			default:
				if r.Action == "submit" {
					text = "已提交：" + r.Label
				}
			}
			result = append(result, ActivitySummary{UUID: r.UUID, Text: text, Status: r.Status, AdmittedAt: r.AdmittedAt, labels: []string{r.Label}})
		}
		previous = r
	}
	return result
}

type activityCursor struct {
	Thread   string `json:"thread_uuid"`
	Entry    string `json:"entry_uuid"`
	Revision int64  `json:"revision"`
}

func encodeActivityCursor(thread, entry string, revision int64) string {
	b, _ := json.Marshal(activityCursor{thread, entry, revision})
	return base64.RawURLEncoding.EncodeToString(b)
}
func decodeActivityCursor(raw, thread string, revision int64) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	var c activityCursor
	if err != nil || len(b) > 512 || json.Unmarshal(b, &c) != nil || c.Thread != thread || !validPublicUUID(c.Entry) {
		return "", fmt.Errorf("invalid activity cursor")
	}
	if c.Revision != revision {
		return "", ErrActivityChanged
	}
	return c.Entry, nil
}

var ErrActivityChanged = errors.New("mcp_activity_changed")
var ErrActivityNotFound = errors.New("mcp_activity_not_found")

func validPublicUUID(s string) bool { u, err := uuid.Parse(s); return err == nil && u.Version() == 7 }

func (s *Service) Activity(ctx context.Context, p, thread, before, after string, limit int) (ActivityPage, error) {
	page := ActivityPage{Items: []ActivitySummary{}}
	if !validPublicUUID(thread) || !validPublicUUID(p) {
		return page, ErrActivityNotFound
	}
	if before != "" && after != "" {
		return page, fmt.Errorf("before and after are mutually exclusive")
	}
	if limit < 1 || limit > 200 {
		return page, fmt.Errorf("limit must be 1–200")
	}
	err := s.projects.WithStore(ctx, p, func(store *project.Store) error {
		if err := s.reconcileActivities(ctx, store); err != nil {
			return err
		}
		return store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var session activityThread
			err := tx.Table("mcp_threads m").Select("m.*").Joins("JOIN chat_threads t ON t.id=m.thread_id JOIN projects p ON p.id=t.project_id").Where("t.uuid=? AND p.uuid=? AND t.thread_type='mcp'", thread, p).Take(&session).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrActivityNotFound
			}
			if err != nil {
				return err
			}
			var rows []activityRecord
			if err := tx.Where("mcp_thread_id=?", session.ID).Order("sequence").Find(&rows).Error; err != nil {
				return err
			}
			summaries := collapseActivities(rows)
			start, end := 0, len(summaries)
			cursor := after
			if before != "" {
				cursor = before
			}
			if cursor != "" {
				anchor, err := decodeActivityCursor(cursor, thread, session.Revision)
				if err != nil {
					return err
				}
				found := -1
				for i, r := range summaries {
					if r.UUID == anchor {
						found = i
						break
					}
				}
				if found < 0 {
					return fmt.Errorf("activity cursor anchor missing")
				}
				if after != "" {
					start = found + 1
				} else {
					end = found
					start = max(0, end-limit)
				}
			}
			if end > start+limit {
				end = start + limit
			}
			page.ClientName, page.StartedAt, page.LastCallAt = session.ClientName, session.StartedAt, session.LastCallAt
			page.Items = summaries[start:end]
			page.CursorPagination = agent.CursorPagination{PerPage: limit, HasMore: end < len(summaries)}
			if len(page.Items) > 0 {
				if end < len(summaries) {
					page.CursorPagination.NextCursor = encodeActivityCursor(thread, page.Items[len(page.Items)-1].UUID, session.Revision)
				}
				if start > 0 {
					page.CursorPagination.PrevCursor = encodeActivityCursor(thread, page.Items[0].UUID, session.Revision)
				}
			}
			return nil
		})
	})
	return page, err
}
