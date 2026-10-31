package textmcp

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
	"lumi/internal/realtime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type appHint struct {
	ProjectUUID      string `json:"project_uuid"`
	SubscriptionUUID string `json:"subscription_uuid"`
	Cursor           string `json:"cursor"`
	State            string `json:"state"`
	Event            string `json:"event,omitempty"`
}

// The WS lives in stdio. next blocks until an actual WS change or ping, never
// querying business state. Normal tool results do not need progress forwarding.
type appWatches struct {
	forward Forward
	mu      sync.Mutex
	subs    map[string]*appWatch
	owners  map[*mcp.ServerSession]bool
	idle    time.Duration
}
type appWatch struct {
	mu                   sync.Mutex
	owner                *mcp.ServerSession
	project, uuid, event string
	seq, change          uint64
	closed, waiting      bool
	wake                 chan struct{}
	conn                 *websocket.Conn
	timer                *time.Timer
}

func newAppWatches(forward Forward) *appWatches {
	return &appWatches{forward: forward, subs: map[string]*appWatch{}, owners: map[*mcp.ServerSession]bool{}, idle: 2 * time.Minute}
}
func appWatchFailure(code, message string) (*mcp.CallToolResult, error) {
	return boundedResult(Failure(code, message, "可手动重连或在 Lumi 中打开；不会通过定时查询刷新业务状态。"))
}
func (m *appWatches) call(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := validate("watch_app_changes", r.Params.Arguments); err != nil {
		return appWatchFailure("invalid_arguments", "实时审阅参数无效")
	}
	var input struct {
		Action, Cursor string
		ProjectUUID    string `json:"project_uuid"`
		Subscription   string `json:"subscription_uuid"`
	}
	_ = json.Unmarshal(r.Params.Arguments, &input)
	if input.Action == "open" {
		return m.open(ctx, r.Session, input.ProjectUUID)
	}
	m.mu.Lock()
	sub := m.subs[input.Subscription]
	m.mu.Unlock()
	if sub == nil || sub.owner != r.Session || sub.project != input.ProjectUUID {
		return appWatchFailure("app_watch_expired", "实时订阅已结束或不属于当前会话")
	}
	if input.Action == "close" {
		m.remove(sub)
		return boundedResult(httpapi.Envelope{Success: true, Data: sub.hint("closed")})
	}
	after, err := strconv.ParseUint(input.Cursor, 10, 64)
	if err != nil {
		return appWatchFailure("invalid_arguments", "实时游标无效")
	}
	sub.mu.Lock()
	if sub.closed || sub.waiting || after > sub.seq {
		sub.mu.Unlock()
		return appWatchFailure("app_watch_invalid_cursor", "每个订阅只能等待一个有效游标")
	}
	sub.waiting = true
	if sub.timer != nil {
		sub.timer.Reset(m.idle)
	}
	sub.mu.Unlock()
	defer func() { sub.mu.Lock(); sub.waiting = false; sub.mu.Unlock() }()
	for {
		sub.mu.Lock()
		if sub.closed {
			sub.mu.Unlock()
			return appWatchFailure("app_watch_disconnected", "CLI 实时连接已中断")
		}
		if sub.seq > after {
			state := "heartbeat"
			// A later keepalive must not hide a business change between tool calls.
			if sub.change > after {
				state = "changed"
			}
			data := sub.hintLocked(state)
			sub.mu.Unlock()
			return boundedResult(httpapi.Envelope{Success: true, Data: data})
		}
		wake := sub.wake
		sub.mu.Unlock()
		select {
		case <-ctx.Done():
			m.remove(sub)
			return appWatchFailure("app_watch_cancelled", "实时等待已关闭")
		case <-wake:
		}
	}
}
func (s *appWatch) hintLocked(state string) appHint {
	h := appHint{ProjectUUID: s.project, SubscriptionUUID: s.uuid, Cursor: strconv.FormatUint(s.seq, 10), State: state}
	if state == "changed" {
		h.Event = s.event
	}
	return h
}
func (s *appWatch) hint(state string) appHint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hintLocked(state)
}
func (s *appWatch) publish(event string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.seq++
	if event != "" {
		s.change, s.event = s.seq, event
	}
	close(s.wake)
	s.wake = make(chan struct{})
}
func (m *appWatches) remove(s *appWatch) {
	m.mu.Lock()
	if m.subs[s.uuid] == s {
		delete(m.subs, s.uuid)
	}
	m.mu.Unlock()
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		if s.timer != nil {
			s.timer.Stop()
		}
		close(s.wake)
		_ = s.conn.Close()
	}
	s.mu.Unlock()
}
func (m *appWatches) open(ctx context.Context, owner *mcp.ServerSession, project string) (*mcp.CallToolResult, error) {
	connection := m.forward(ctx, "read_app_connection", json.RawMessage(`{}`))
	if !connection.Success {
		return boundedResult(connection)
	}
	b, err := json.Marshal(connection.Data)
	if err != nil {
		return nil, err
	}
	var descriptor struct {
		ProjectUUID string `json:"project_uuid"`
		URL         string `json:"websocket_url"`
	}
	if json.Unmarshal(b, &descriptor) != nil || descriptor.ProjectUUID != project {
		return appWatchFailure("app_project_mismatch", "实时审阅项目不匹配")
	}
	endpoint, err := url.Parse(descriptor.URL)
	if err != nil || endpoint.Scheme != "ws" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.Path != "/api/v1/ws" || endpoint.User != nil || endpoint.Fragment != "" || endpoint.Query().Get("app_ticket") == "" {
		return appWatchFailure("app_connection_invalid", "CLI 实时连接无效")
	}
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: nil}
	conn, response, err := dialer.DialContext(ctx, descriptor.URL, http.Header{"Origin": []string{"http://" + endpoint.Host}})
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		return appWatchFailure("app_watch_disconnected", "CLI 实时连接未建立")
	}
	keep := false
	defer func() {
		if !keep {
			_ = conn.Close()
		}
	}()
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()
	conn.SetReadLimit(64 << 10)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	ref := "lumi-app-watch"
	topic := realtime.ProjectTopic(project)
	if err := conn.WriteJSON(realtime.Frame{Topic: topic, Event: realtime.EventJoin, Payload: map[string]any{}, Ref: &ref, JoinRef: &ref}); err != nil {
		return appWatchFailure("app_watch_disconnected", "实时订阅未建立")
	}
	for {
		var frame realtime.Frame
		if err := conn.ReadJSON(&frame); err != nil {
			return appWatchFailure("app_watch_join_failed", "实时项目订阅未建立")
		}
		if frame.Topic != topic || frame.JoinRef == nil || *frame.JoinRef != ref || frame.Event != realtime.EventReply || frame.Ref == nil || *frame.Ref != ref {
			continue
		}
		payload, _ := json.Marshal(frame.Payload)
		var reply struct{ Status string }
		if json.Unmarshal(payload, &reply) != nil || reply.Status != "ok" {
			return appWatchFailure("app_watch_join_failed", "实时项目订阅被拒绝")
		}
		break
	}
	if ctx.Err() != nil {
		return appWatchFailure("app_watch_cancelled", "实时订阅已关闭")
	}
	_ = conn.SetReadDeadline(time.Time{})
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	sub := &appWatch{owner: owner, project: project, uuid: id.String(), wake: make(chan struct{}), conn: conn}
	m.mu.Lock()
	if len(m.subs) >= 16 {
		m.mu.Unlock()
		return appWatchFailure("app_watch_limit", "实时审阅窗口过多，请关闭部分窗口")
	}
	m.subs[sub.uuid] = sub
	firstOwner := !m.owners[owner]
	m.owners[owner] = true
	m.mu.Unlock()
	sub.mu.Lock()
	sub.timer = time.AfterFunc(m.idle, func() { m.remove(sub) })
	sub.mu.Unlock()
	if firstOwner {
		go func() {
			_ = owner.Wait()
			m.mu.Lock()
			delete(m.owners, owner)
			var owned []*appWatch
			for _, s := range m.subs {
				if s.owner == owner {
					owned = append(owned, s)
				}
			}
			m.mu.Unlock()
			for _, s := range owned {
				m.remove(s)
			}
		}()
	}
	keep = true
	// Capture the join cursor before reading subsequent WS hints.
	joined := sub.hint("joined")
	defaultPing := conn.PingHandler()
	conn.SetPingHandler(func(data string) error {
		if err := defaultPing(data); err != nil {
			return err
		}
		sub.publish("")
		return nil
	})
	go func() {
		defer m.remove(sub)
		for {
			var frame realtime.Frame
			if err := conn.ReadJSON(&frame); err != nil {
				return
			}
			if frame.Topic != topic || frame.JoinRef == nil || *frame.JoinRef != ref {
				continue
			}
			if frame.Event == realtime.EventClose || frame.Event == realtime.EventError {
				return
			}
			if frame.Event != "" && len(frame.Event) <= 128 && !strings.HasPrefix(frame.Event, "phx_") {
				sub.publish(frame.Event)
			}
		}
	}()
	return boundedResult(httpapi.Envelope{Success: true, Data: joined})
}
