package agent

import (
	"context"
	"strings"
	"testing"
)

func TestExternalRegistryFailsClosedAndSanitizesResponses(t *testing.T) {
	const p = "01970000-0000-7000-8000-000000000001"
	api := NewExternalProjectAPI(func(context.Context, ProjectAPIDispatchRequest) (ProjectAPIDispatchResponse, error) {
		return ProjectAPIDispatchResponse{Status: 200, Body: []byte(`{"success":true,"data":{"uuid":"` + p + `","name":"safe","id":42,"root_path":"secret","revision":1}}`)}, nil
	}, nil)
	for _, args := range []map[string]any{
		{"method": "GET", "url": "/api/v1/projects/" + p + "/chat_threads", "response_filter": ".data"},
		{"method": "GET", "url": "/api/v1/projects/" + p, "response_filter": ".data | {id}"},
		{"method": "GET", "url": "/api/v1/projects/" + p, "response_filter": ".data | {uuid}", "headers": map[string]any{"confirmed": true}},
	} {
		if _, err := api.Prepare(p, args); err == nil {
			t.Fatalf("accepted %#v", args)
		}
	}
	r, err := api.Prepare(p, map[string]any{"method": "GET", "url": "/api/v1/projects/" + p, "response_filter": ".data | {uuid,name,revision}"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := api.Execute(t.Context(), r, p)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if len(m) != 3 || m["name"] != "safe" {
		t.Fatalf("%+v", m)
	}
	if err := validateAgentAPIResponse(map[string]any{"nested": map[string]any{"id": 1}}); err == nil {
		t.Fatal("internal ID accepted")
	}
	oversized := NewExternalProjectAPI(func(context.Context, ProjectAPIDispatchRequest) (ProjectAPIDispatchResponse, error) {
		return ProjectAPIDispatchResponse{Status: 200, Body: []byte(`{"success":true,"data":{"uuid":"` + p + `","name":"` + strings.Repeat("x", MaxToolResult) + `","revision":1}}`)}, nil
	}, nil)
	if _, err = oversized.Execute(t.Context(), r, p); err == nil {
		t.Fatal("oversized result accepted")
	}
}
