package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/realtime"
)

func TestStdioAppWatchForwardsWSHintsAndRejoinsWithoutPolling(t *testing.T) {
	opt := cliOptions(t)
	root := t.TempDir()
	client := mcpClient(t, opt, root)
	initial := callText(t, client, "project_init", map[string]any{"idea": "App stream test", "name": "Before"}, "")
	projectUUID := field(initial, "project")["uuid"].(string)
	open := func() map[string]any {
		return callText(t, client, "watch_app_changes", map[string]any{"action": "open", "project_uuid": projectUUID}, "")
	}
	opened := open()
	if opened["state"] != "joined" {
		t.Fatal(opened)
	}
	wait := func(cursor any) chan map[string]any {
		done := make(chan map[string]any, 1)
		go func() {
			done <- callText(t, client, "watch_app_changes", map[string]any{"action": "next", "project_uuid": projectUUID, "subscription_uuid": opened["subscription_uuid"], "cursor": cursor}, "")
		}()
		return done
	}
	await := func(ch chan map[string]any, event string) map[string]any {
		t.Helper()
		select {
		case data := <-ch:
			if data["state"] != "changed" || data["event"] != event || data["project_uuid"] != projectUUID {
				t.Fatal(data)
			}
			assertPublicFields(t, data)
			b, _ := json.Marshal(data)
			if strings.Contains(string(b), "app_ticket") || strings.Contains(string(b), "websocket_url") {
				t.Fatal("descriptor leaked")
			}
			return data
		case <-time.After(5 * time.Second):
			t.Fatal("missing WS event")
			return nil
		}
	}
	pending := wait(opened["cursor"])
	setup := callText(t, client, "project_setup", map[string]any{"action": "update", "expected_revision": field(initial, "setup")["revision"], "project_name": "After WS hint", "generation_language": "en", "overall_style": "Watercolor", "generation_brief": "Help a star home", "picture_book": map[string]any{"format": "classic_picture_book", "aspect_ratio": map[string]any{"mode": "landscape"}, "large_image_minimal_text": false}}, "")
	changed := await(pending, "project:setup_changed")
	pending = wait(changed["cursor"])
	callText(t, client, "project_setup", map[string]any{"action": "finalize", "expected_revision": setup["revision"]}, "")
	await(pending, "project:setup_changed")
	if data := callText(t, client, "review_book", map[string]any{}, ""); data["title"] != "After WS hint" {
		t.Fatal(data)
	}
	close := func() {
		callText(t, client, "watch_app_changes", map[string]any{"action": "close", "project_uuid": projectUUID, "subscription_uuid": opened["subscription_uuid"]}, "")
	}
	close()
	opened = open()
	if opened["state"] != "joined" {
		t.Fatal(opened)
	}
	callText(t, client, "review_book", map[string]any{}, "")
	close()
}

func TestAppTicketsAreSignedScopedExpiringAndInstanceBound(t *testing.T) {
	now := time.Now()
	identity := instance{Endpoint: "http://127.0.0.1:1234", Nonce: "instance-one"}
	projectUUID := "01989abc-def0-7000-8000-000000000001"
	connection := appConnection(identity, projectUUID, now).(map[string]any)
	u, _ := url.Parse(connection["websocket_url"].(string))
	ticket := u.Query().Get("app_ticket")
	got, expires, err := verifyAppTicket(identity.Nonce, ticket, now)
	if err != nil || got != projectUUID || !expires.After(now) {
		t.Fatal(got, expires, err)
	}
	for _, tc := range []struct {
		secret, ticket string
		now            time.Time
	}{{"changed-instance", ticket, now}, {identity.Nonce, ticket + "x", now}, {identity.Nonce, ticket, now.Add(time.Hour)}, {identity.Nonce, "bad", now}} {
		if _, _, err := verifyAppTicket(tc.secret, tc.ticket, tc.now); err == nil {
			t.Fatal("invalid ticket accepted")
		}
	}
}

func TestStdioAppReviewImageAndScopedRealtimeReadback(t *testing.T) {
	opt := cliOptions(t)
	root := t.TempDir()
	client := mcpClient(t, opt, root)
	init := callText(t, client, "project_init", map[string]any{"idea": "Review all four views", "name": "App review"}, "")
	projectUUID := field(init, "project")["uuid"].(string)
	// Draft project can already be reviewed, with no renderer or images.
	book := callText(t, client, "review_book", map[string]any{}, "")
	if book["view"] != "book" || book["open_url"] == "" {
		t.Fatal(book)
	}
	openCommand := cliProcess(opt, "open")
	openCommand.Dir = root
	output, err := openCommand.Output()
	if err != nil {
		t.Fatal(err)
	}
	var opened toolEnvelope
	if json.Unmarshal(output, &opened) != nil || !opened.Success || opened.Data["url"] != book["open_url"] {
		t.Fatalf("open=%s", output)
	}
	candidates := callText(t, client, "review_candidates", map[string]any{"kind": "text", "candidates": []any{map[string]any{"title": "One", "text": "Full first candidate"}, map[string]any{"title": "Two", "text": "Full second candidate"}}}, "")
	if len(candidates["items"].([]any)) != 1 {
		t.Fatal(candidates)
	}
	callText(t, client, "read_app_image", map[string]any{"asset_uuid": "01989abc-def0-7000-8000-000000000099"}, "asset_not_found")
	status := cliRun(t, opt, "status", 0)
	connData := appConnection(status.instance, projectUUID, time.Now()).(map[string]any)
	wsURL := connData["websocket_url"].(string)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Origin": []string{"null"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	send := func(topic, ref string) realtime.Frame {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteJSON(realtime.Frame{Topic: topic, Event: realtime.EventJoin, Payload: map[string]any{}, Ref: &ref, JoinRef: &ref}); err != nil {
			t.Fatal(err)
		}
		var frame realtime.Frame
		if err := conn.ReadJSON(&frame); err != nil {
			t.Fatal(err)
		}
		return frame
	}
	if frame := send("system", "denied"); frame.Event != realtime.EventError {
		t.Fatalf("cross-topic=%+v", frame)
	}
	if frame := send(realtime.ProjectTopic("01989abc-def0-7000-8000-000000000099"), "denied2"); frame.Event != realtime.EventError {
		t.Fatalf("cross-project=%+v", frame)
	}
	if frame := send(realtime.ProjectTopic(projectUUID), "authoring-ui"); frame.Event != realtime.EventReply {
		t.Fatalf("join=%+v", frame)
	}
	setup := field(init, "setup")
	setup = callText(t, client, "project_setup", map[string]any{"action": "update", "expected_revision": setup["revision"], "project_name": "Updated in SQLite", "generation_language": "en", "overall_style": "Watercolor", "generation_brief": "Stars return home", "picture_book": map[string]any{"format": "classic_picture_book", "aspect_ratio": map[string]any{"mode": "landscape"}, "large_image_minimal_text": false}}, "")
	expectHint(t, conn, "project:setup_changed")
	callText(t, client, "project_setup", map[string]any{"action": "finalize", "expected_revision": setup["revision"]}, "")
	expectHint(t, conn, "project:setup_changed")
	reread := callText(t, client, "review_book", map[string]any{}, "")
	if reread["title"] != "Updated in SQLite" {
		t.Fatal(reread)
	}
	// Rejoin yields a new calibration point; no periodic MCP reads are involved.
	if frame := send(realtime.ProjectTopic(projectUUID), "rejoined"); frame.Event != realtime.EventReply {
		t.Fatal(frame)
	}
	callText(t, client, "review_book", map[string]any{}, "")
	resource, err := client.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: toolResourceURI(t, client)})
	if err != nil || !strings.Contains(resource.Contents[0].Text, "img-src data:") {
		t.Fatal(resource, err)
	}
	if strings.Contains(resource.Contents[0].Text, "app_ticket=") {
		t.Fatal("ticket embedded in shared HTML")
	}
	address := strings.Split(wsURL, "?")[0] + "?app_ticket=bad"
	bad, response, err := websocket.DefaultDialer.Dial(address, http.Header{"Origin": []string{"null"}})
	if bad != nil {
		bad.Close()
	}
	if err == nil || response.StatusCode != 403 {
		t.Fatal("forged ticket accepted")
	}
}
func toolResourceURI(t *testing.T, client *mcp.ClientSession) string {
	listed, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == "review_book" {
			b, _ := json.Marshal(tool.Meta)
			var meta struct {
				UI struct {
					URI string `json:"resourceUri"`
				} `json:"ui"`
			}
			_ = json.Unmarshal(b, &meta)
			return meta.UI.URI
		}
	}
	t.Fatal("missing resource association")
	return ""
}
