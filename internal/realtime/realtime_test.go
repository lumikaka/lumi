package realtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

func TestNilHubBroadcastIsNoOp(t *testing.T) {
	var hub *Hub
	hub.Broadcast(SystemTopic, SystemStatusChanged, map[string]any{"status": "ok"})
}

func TestSystemChannelJoinBroadcastHeartbeatAndLeave(t *testing.T) {
	hub := NewHub(PublicTopics{})
	server := websocketTestServer(t, hub, "http://frontend.test")
	connection := dialWebSocket(t, server, "http://frontend.test")

	join := Frame{Topic: SystemTopic, Event: EventJoin, Payload: map[string]any{}, Ref: pointer("1"), JoinRef: pointer("join-1")}
	writeFrame(t, connection, join)
	assertReply(t, readFrame(t, connection), "ok", "1", "join-1")
	if count := hub.MembershipCount(SystemTopic); count != 1 {
		t.Fatalf("membership count = %d", count)
	}

	hub.Broadcast(SystemTopic, SystemStatusChanged, map[string]any{"status": "ok"})
	broadcast := readFrame(t, connection)
	if broadcast.Event != SystemStatusChanged || broadcast.JoinRef == nil || *broadcast.JoinRef != "join-1" {
		t.Fatalf("broadcast = %#v", broadcast)
	}

	heartbeat := Frame{Topic: PhoenixHeartbeatTopic, Event: EventHeartbeat, Payload: map[string]any{}, Ref: pointer("2")}
	writeFrame(t, connection, heartbeat)
	assertReply(t, readFrame(t, connection), "ok", "2", "")

	leave := Frame{Topic: SystemTopic, Event: EventLeave, Payload: map[string]any{}, Ref: pointer("3"), JoinRef: pointer("join-1")}
	writeFrame(t, connection, leave)
	assertReply(t, readFrame(t, connection), "ok", "3", "join-1")
	if closed := readFrame(t, connection); closed.Event != EventClose {
		t.Fatalf("close frame = %#v", closed)
	}
}

func TestOneConnectionJoinsMultipleProjectTopicsAndReleasesPresence(t *testing.T) {
	first := "01989abc-def0-7000-8000-000000000001"
	second := "01989abc-def0-7000-8000-000000000002"
	var leasesMu sync.Mutex
	leases := map[string]int{}
	hub := NewHub(ProjectTopics{AcquirePresence: func(projectUUID string) (func(), error) {
		leasesMu.Lock()
		leases[projectUUID]++
		leasesMu.Unlock()
		var once sync.Once
		return func() {
			once.Do(func() {
				leasesMu.Lock()
				leases[projectUUID]--
				leasesMu.Unlock()
			})
		}, nil
	}})
	server := websocketTestServer(t, hub, "http://frontend.test")
	connection := dialWebSocket(t, server, "http://frontend.test")
	joinConnection(t, connection, ProjectTopic(first), "first")
	joinConnection(t, connection, ProjectTopic(second), "second")
	if hub.MembershipCount(ProjectTopic(first)) != 1 || hub.MembershipCount(ProjectTopic(second)) != 1 {
		t.Fatalf("memberships = %d/%d", hub.MembershipCount(ProjectTopic(first)), hub.MembershipCount(ProjectTopic(second)))
	}

	hub.Broadcast(ProjectTopic(first), "task:progress", map[string]any{"project_uuid": first})
	firstEvent := readFrame(t, connection)
	if firstEvent.Topic != ProjectTopic(first) || firstEvent.Payload.(map[string]any)["project_uuid"] != first {
		t.Fatalf("first event = %#v", firstEvent)
	}
	hub.Broadcast(ProjectTopic(second), "task:progress", map[string]any{"project_uuid": second})
	secondEvent := readFrame(t, connection)
	if secondEvent.Topic != ProjectTopic(second) || secondEvent.Payload.(map[string]any)["project_uuid"] != second {
		t.Fatalf("second event = %#v", secondEvent)
	}

	writeFrame(t, connection, Frame{Topic: ProjectTopic(first), Event: EventLeave, Payload: map[string]any{}, Ref: pointer("leave-first"), JoinRef: pointer("first")})
	assertReply(t, readFrame(t, connection), "ok", "leave-first", "first")
	if closed := readFrame(t, connection); closed.Event != EventClose {
		t.Fatalf("close frame = %#v", closed)
	}
	eventually(t, func() bool {
		leasesMu.Lock()
		defer leasesMu.Unlock()
		return leases[first] == 0 && leases[second] == 1
	})
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		leasesMu.Lock()
		defer leasesMu.Unlock()
		return leases[first] == 0 && leases[second] == 0
	})
}

func TestRealtimeRejectsUnknownTopicAndBadOrigin(t *testing.T) {
	hub := NewHub(PublicTopics{})
	server := websocketTestServer(t, hub, "http://frontend.test")
	connection := dialWebSocket(t, server, "http://frontend.test")
	writeFrame(t, connection, Frame{
		Topic: "unknown", Event: EventJoin, Payload: map[string]any{}, Ref: pointer("1"), JoinRef: pointer("1"),
	})
	reply := readFrame(t, connection)
	payload := reply.Payload.(map[string]any)
	response := payload["response"].(map[string]any)
	if payload["status"] != "error" || response["reason"] != "invalid_topic" {
		t.Fatalf("reply = %#v", reply)
	}

	badHeader := http.Header{"Origin": []string{"http://evil.test"}}
	badConnection, httpResponse, err := websocket.DefaultDialer.Dial(websocketURL(server.URL), badHeader)
	if badConnection != nil {
		_ = badConnection.Close()
	}
	if err == nil || httpResponse == nil || httpResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("bad origin response = %v, error = %v", httpResponse, err)
	}
}

func TestOriginAliasesAndGracefulHubShutdown(t *testing.T) {
	if !OriginAllowed("http://127.0.0.1:5801", "http://localhost:5801") ||
		!OriginAllowed("http://[::1]:5801", "http://localhost:5801") ||
		OriginAllowed("http://evil.test", "http://localhost:5801") {
		t.Fatal("unexpected origin validation result")
	}

	hub := NewHub(PublicTopics{})
	server := websocketTestServer(t, hub, "http://frontend.test")
	connection := dialWebSocket(t, server, "http://frontend.test")
	joinConnection(t, connection, SystemTopic, "shutdown")
	if err := hub.Close(); err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := connection.ReadMessage(); err == nil {
		t.Fatal("connection remained open")
	} else if closeError, ok := err.(*websocket.CloseError); !ok || closeError.Code != websocket.CloseNormalClosure {
		t.Fatalf("close error = %v", err)
	}
	if hub.ConnectionCount() != 0 || hub.MembershipCount(SystemTopic) != 0 {
		t.Fatal("hub shutdown leaked realtime state")
	}
}

func TestSlowClientDoesNotBlockBroadcast(t *testing.T) {
	hub := NewHub(PublicTopics{})
	client := newClient(t.Context(), hub, nil)
	if !hub.register(client) {
		t.Fatal("register slow client")
	}
	hub.join(client, SystemTopic, "slow", func() {})
	started := time.Now()
	for index := 0; index < defaultSendQueueSize+1; index++ {
		hub.Broadcast(SystemTopic, SystemStatusChanged, map[string]any{"sequence": index})
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("broadcast blocked for %s", elapsed)
	}
	eventually(t, func() bool { return hub.MembershipCount(SystemTopic) == 0 })
}

func websocketTestServer(t *testing.T, hub *Hub, frontendURL string) *httptest.Server {
	t.Helper()
	e := echo.New()
	e.GET("/api/v1/ws", NewWebSocketHandler(hub, frontendURL).Serve)
	server := httptest.NewServer(e)
	t.Cleanup(func() {
		_ = hub.Close()
		server.Close()
	})
	return server
}

func dialWebSocket(t *testing.T, server *httptest.Server, origin string) *websocket.Conn {
	t.Helper()
	header := http.Header{"Origin": []string{origin}}
	connection, response, err := websocket.DefaultDialer.Dial(websocketURL(server.URL), header)
	if err != nil {
		t.Fatalf("dial websocket: response=%v, error=%v", response, err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

func websocketURL(serverURL string) string {
	return "ws" + strings.TrimPrefix(serverURL, "http") + "/api/v1/ws"
}

func joinConnection(t *testing.T, connection *websocket.Conn, topic, ref string) {
	t.Helper()
	writeFrame(t, connection, Frame{Topic: topic, Event: EventJoin, Payload: map[string]any{}, Ref: &ref, JoinRef: &ref})
	assertReply(t, readFrame(t, connection), "ok", ref, ref)
}

func writeFrame(t *testing.T, connection *websocket.Conn, frame Frame) {
	t.Helper()
	if err := connection.WriteJSON(frame); err != nil {
		t.Fatal(err)
	}
}

func readFrame(t *testing.T, connection *websocket.Conn) Frame {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	_, message, err := connection.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var frame Frame
	if err := json.Unmarshal(message, &frame); err != nil {
		t.Fatal(err)
	}
	return frame
}

func assertReply(t *testing.T, frame Frame, status, ref, joinRef string) {
	t.Helper()
	if frame.Event != EventReply || frame.Ref == nil || *frame.Ref != ref {
		t.Fatalf("reply = %#v", frame)
	}
	if joinRef == "" {
		if frame.JoinRef != nil {
			t.Fatalf("join_ref = %v", frame.JoinRef)
		}
	} else if frame.JoinRef == nil || *frame.JoinRef != joinRef {
		t.Fatalf("join_ref = %v", frame.JoinRef)
	}
	payload := frame.Payload.(map[string]any)
	if payload["status"] != status {
		t.Fatalf("status = %v", payload["status"])
	}
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not satisfied")
}

func pointer(value string) *string { return &value }
