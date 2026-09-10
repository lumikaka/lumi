package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPHistoryAppearsInExistingThreadsAndRemainsReadOnly(t *testing.T) {
	h := newMCPHarness(t)
	request := func(method, path, body string, want int) map[string]any {
		t.Helper()
		r := httptest.NewRequest(method, "/api/v1/projects/"+h.p+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://localhost:5801")
		w := httptest.NewRecorder()
		h.a.Echo.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(w.Body.String(), `"id":`) {
			t.Fatal("internal ID exposed")
		}
		return envelope
	}
	empty := request(http.MethodGet, "/chat_threads", "", 200)["data"].(map[string]any)["items"].([]any)
	if len(empty) != 0 {
		t.Fatal("protocol connect created history")
	}
	h.tool(t, "read_agent_doc", map[string]any{"path": "/api/v1/agent-docs/overview.md"}, false)
	h.request(t, "GET", "", nil, "", false)
	edit := map[string]any{"name": "MCP history", "description": "private input is not history", "expected_revision": float64(1)}
	first := h.request(t, "PATCH", "", edit, "edit-history", false)
	h.request(t, "PATCH", "", edit, "stale-history", true)
	h.request(t, "PATCH", "", map[string]any{"name": "MCP history 2", "description": "private input is not history", "expected_revision": float64(2)}, "edit-history-2", false)
	threads := request(http.MethodGet, "/chat_threads", "", 200)["data"].(map[string]any)["items"].([]any)
	if len(threads) != 1 {
		t.Fatal(threads)
	}
	thread := threads[0].(map[string]any)
	if thread["thread_type"] != "mcp" || thread["status"] != "idle" || thread["model"] != "" {
		t.Fatal(thread)
	}
	u := thread["uuid"].(string)
	data := request(http.MethodGet, "/chat_threads/"+u+"/mcp_activity", "", 200)["data"].(map[string]any)
	items := data["items"].([]any)
	if len(items) != 4 || items[2].(map[string]any)["status"] != "failed" {
		t.Fatalf("history %+v", items)
	}
	encoded, _ := json.Marshal(data)
	if strings.Contains(string(encoded), "private input") || strings.Contains(string(encoded), "request_body") || strings.Contains(string(encoded), "expected_revision") {
		t.Fatal("full request leaked")
	}
	// Recovering/replaying a previous result does not duplicate its business effect.
	replay := h.request(t, "PATCH", "", edit, "edit-history", false)
	if callInfo(first)["uuid"] != callInfo(replay)["uuid"] {
		t.Fatal("idempotency changed")
	}
	h.tool(t, "get_call", map[string]any{"call_uuid": callInfo(first)["uuid"]}, false)
	data = request(http.MethodGet, "/chat_threads/"+u+"/mcp_activity", "", 200)["data"].(map[string]any)
	if len(data["items"].([]any)) != 5 {
		t.Fatalf("replay not summarized as a read: %+v", data)
	}
	request(http.MethodPost, "/chat_threads/"+u+"/turns", `{"input_text":"continue"}`, 422)
	request(http.MethodPost, "/chat_threads/"+u+"/follow_ups", `{"input_text":"continue"}`, 422)
	request(http.MethodGet, "/chat_threads/"+u+"/trajectory", "", 422)
	request(http.MethodGet, "/chat_threads/01970000-0000-7000-8000-000000000001/mcp_activity", "", 404)
}
