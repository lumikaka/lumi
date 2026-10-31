package textmcp

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
	"lumi/internal/realtime"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAppWatchWaitsForWSWithoutStatePollingAndClosesLeases(t *testing.T) {
	const project = "01989abc-def0-7000-8000-000000000001"
	ready := make(chan *websocket.Conn, 8)
	closed := make(chan struct{}, 8)
	var sockets, reads atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := u.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		sockets.Add(1)
		defer conn.Close()
		if r.URL.Path != "/api/v1/ws" || r.Header.Get("Origin") == "" {
			t.Error("invalid CLI WS route")
		}
		var join realtime.Frame
		if conn.ReadJSON(&join) != nil {
			return
		}
		if join.Topic != realtime.ProjectTopic(project) || join.Event != realtime.EventJoin || join.Ref == nil || join.JoinRef == nil {
			t.Error("invalid envelope")
			return
		}
		_ = conn.WriteJSON(realtime.Frame{Topic: join.Topic, Event: realtime.EventReply, Payload: map[string]any{"status": "ok", "response": map[string]any{}}, Ref: join.Ref, JoinRef: join.JoinRef})
		ready <- conn
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				closed <- struct{}{}
				return
			}
		}
	}))
	defer backend.Close()
	forward := func(_ context.Context, name string, _ json.RawMessage) httpapi.Envelope {
		reads.Add(1)
		if name == "read_app_connection" {
			return httpapi.Envelope{Success: true, Data: map[string]any{"project_uuid": project, "websocket_url": "ws" + strings.TrimPrefix(backend.URL, "http") + "/api/v1/ws?app_ticket=unit"}}
		}
		return httpapi.Envelope{Success: true, Data: map[string]any{"project_uuid": project}}
	}
	manager := newAppWatches(forward)
	server := mcp.NewServer(&mcp.Implementation{Name: "wait-contract", Version: "1"}, nil)
	for _, spec := range Specs() {
		if spec.Name == "watch_app_changes" {
			server.AddTool(&mcp.Tool{Name: spec.Name, InputSchema: spec.Schema}, manager.call)
		}
		if spec.Name == "project_overview" {
			server.AddTool(&mcp.Tool{Name: spec.Name, InputSchema: spec.Schema}, func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return boundedResult(forward(ctx, "project_overview", r.Params.Arguments))
			})
		}
	}
	connect := func() *mcp.ClientSession {
		a, b := mcp.NewInMemoryTransports()
		ss, err := server.Connect(t.Context(), a, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ss.Close() })
		client, err := mcp.NewClient(&mcp.Implementation{Name: "wait-contract", Version: "1"}, nil).Connect(t.Context(), b, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })
		return client
	}
	client, other := connect(), connect()
	call := func(c *mcp.ClientSession, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		r, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: "watch_app_changes", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	decode := func(r *mcp.CallToolResult) appHint {
		t.Helper()
		b, _ := json.Marshal(r.StructuredContent)
		var e struct {
			Success bool
			Data    appHint
		}
		if json.Unmarshal(b, &e) != nil || !e.Success || r.IsError {
			t.Fatalf("result=%s", b)
		}
		if strings.Contains(string(b), `"id"`) || strings.Contains(string(b), "app_ticket") {
			t.Fatal("private fields leaked")
		}
		return e.Data
	}
	bad := call(client, map[string]any{"action": "open", "project_uuid": "01989abc-def0-7000-8000-000000000099"})
	if !bad.IsError || sockets.Load() != 0 {
		t.Fatal("cross project opened socket")
	}
	opened := decode(call(client, map[string]any{"action": "open", "project_uuid": project}))
	conn := <-ready
	if opened.State != "joined" || opened.Cursor != "0" {
		t.Fatal(opened)
	}
	args := func(action, cursor string) map[string]any {
		return map[string]any{"action": action, "project_uuid": project, "subscription_uuid": opened.SubscriptionUUID, "cursor": cursor}
	}
	awaitWaiting := func() {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			manager.mu.Lock()
			sub := manager.subs[opened.SubscriptionUUID]
			manager.mu.Unlock()
			if sub != nil {
				sub.mu.Lock()
				waiting := sub.waiting
				sub.mu.Unlock()
				if waiting {
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("server did not enter the blocking wait")
	}
	if !call(other, args("next", "0")).IsError {
		t.Fatal("subscription crossed MCP session")
	}
	if !call(client, args("next", "99")).IsError {
		t.Fatal("future cursor accepted")
	}
	type outcome struct {
		result *mcp.CallToolResult
		err    error
	}
	wait := func(ctx context.Context, cursor string) chan outcome {
		ch := make(chan outcome, 1)
		go func() {
			r, e := client.CallTool(ctx, &mcp.CallToolParams{Name: "watch_app_changes", Arguments: args("next", cursor)})
			ch <- outcome{r, e}
		}()
		return ch
	}
	before := reads.Load()
	pending := wait(t.Context(), "0")
	awaitWaiting()
	select {
	case r := <-pending:
		t.Fatalf("wait returned without WS event: %v", r)
	case <-time.After(30 * time.Millisecond):
	}
	if reads.Load() != before {
		t.Fatal("next queried REST")
	}
	if !call(client, args("next", "0")).IsError {
		t.Fatal("duplicate waiter accepted")
	}
	ctx, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()
	if r, e := client.CallTool(ctx, &mcp.CallToolParams{Name: "project_overview", Arguments: map[string]any{}}); e != nil || r.IsError {
		t.Fatal("wait blocked ordinary tools", r, e)
	}
	ref := "lumi-app-watch"
	_ = conn.WriteJSON(realtime.Frame{Topic: realtime.ProjectTopic(project), Event: "comic:section_changed", Payload: map[string]any{"id": 99}, JoinRef: &ref})
	await := func(ch chan outcome) appHint {
		t.Helper()
		select {
		case r := <-ch:
			if r.err != nil {
				t.Fatal(r.err)
			}
			return decode(r.result)
		case <-time.After(time.Second):
			t.Fatal("missing WS hint")
			return appHint{}
		}
	}
	changed := await(pending)
	if changed.State != "changed" || changed.Event != "comic:section_changed" {
		t.Fatal(changed)
	}
	// A WS event received between tool calls remains available via the cursor.
	_ = conn.WriteJSON(realtime.Frame{Topic: realtime.ProjectTopic(project), Event: "premise:asset_changed", Payload: map[string]any{}, JoinRef: &ref})
	changed = await(wait(t.Context(), changed.Cursor))
	if changed.State != "changed" || changed.Event != "premise:asset_changed" {
		t.Fatal(changed)
	}
	pending = wait(t.Context(), changed.Cursor)
	_ = conn.WriteControl(websocket.PingMessage, []byte("keepalive"), time.Now().Add(time.Second))
	beat := await(pending)
	if beat.State != "heartbeat" || reads.Load() != before+1 {
		t.Fatal("heartbeat read business state", beat, reads.Load())
	}
	cancelCtx, cancel := context.WithCancel(t.Context())
	pending = wait(cancelCtx, beat.Cursor)
	awaitWaiting()
	cancel()
	select {
	case <-pending:
	case <-time.After(time.Second):
		t.Fatal("wait not cancelled")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("cancellation leaked WS")
	}
	opened = decode(call(client, map[string]any{"action": "open", "project_uuid": project}))
	<-ready
	if decode(call(client, args("close", "0"))).State != "closed" {
		t.Fatal("close failed")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("explicit close leaked WS")
	}
	// Keepalive coalescing must retain an earlier business hint.
	s := &appWatch{project: project, wake: make(chan struct{})}
	s.publish("comic:section_changed")
	s.publish("")
	if s.change != 1 || s.seq != 2 || s.hint("changed").Event != "comic:section_changed" {
		t.Fatal("heartbeat hid change")
	}
	// Exercise idle cleanup using the same production handler with a short lease.
	leaseManager := newAppWatches(forward)
	leaseManager.idle = 40 * time.Millisecond
	leaseServer := mcp.NewServer(&mcp.Implementation{Name: "lease-contract", Version: "1"}, nil)
	for _, spec := range Specs() {
		if spec.Name == "watch_app_changes" {
			leaseServer.AddTool(&mcp.Tool{Name: spec.Name, InputSchema: spec.Schema}, leaseManager.call)
		}
	}
	a, b := mcp.NewInMemoryTransports()
	ss, _ := leaseServer.Connect(t.Context(), a, nil)
	defer ss.Close()
	lc, e := mcp.NewClient(&mcp.Implementation{Name: "lease", Version: "1"}, nil).Connect(t.Context(), b, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer lc.Close()
	decode(call(lc, map[string]any{"action": "open", "project_uuid": project}))
	<-ready
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("idle lease leaked WS")
	}
	leaseManager.mu.Lock()
	remaining := len(leaseManager.subs)
	leaseManager.mu.Unlock()
	if remaining != 0 {
		t.Fatal("idle lease retained")
	}
	decode(call(lc, map[string]any{"action": "open", "project_uuid": project}))
	<-ready
	_ = lc.Close()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("MCP session close leaked WS")
	}
}
