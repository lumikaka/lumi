package mcpserver

import (
	"context"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/project"
)

func seedMCPInitialization(t *testing.T, s *Service, g Grant) {
	t.Helper()
	err := s.projects.WithStore(t.Context(), g.ProjectUUID, func(store *project.Store) error {
		now := time.Now().UTC()
		return store.DB().Exec(`INSERT INTO workflows(uuid,project_id,kind,title,status,input_snapshot,idempotency_key,provider_uuid,model,created_at,updated_at)
			VALUES(?,(SELECT id FROM projects WHERE uuid=?),'yolo_project_initialization','初始化','running','{}','mcp-initialization-lock',?,'test',?,?)`, newUUID(), g.ProjectUUID, newUUID(), now, now).Error
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMCPInitializationBlocksNewWritesAndPreservesReadsAndReplay(t *testing.T) {
	s, g := activityHarness(t)
	dispatched := 0
	s.api = agent.NewExternalProjectAPI(func(_ context.Context, _ agent.ProjectAPIDispatchRequest) (agent.ProjectAPIDispatchResponse, error) {
		dispatched++
		return agent.ProjectAPIDispatchResponse{Status: 200, Body: []byte(`{"success":true,"data":{"uuid":"` + g.ProjectUUID + `","name":"项目","revision":1}}`)}, nil
	}, nil)
	read := map[string]any{"method": "GET", "url": "/api/v1/projects/" + g.ProjectUUID, "response_filter": ".data | {uuid,name,revision}"}
	previous := s.Call(t.Context(), g, read, "before-initialization")
	if previous["success"] != true || dispatched != 1 {
		t.Fatalf("read failed: %+v", previous)
	}
	seedMCPInitialization(t, s, g)
	for _, args := range []map[string]any{
		{"method": "POST", "url": "/api/v1/projects/" + g.ProjectUUID + "/chapters", "request_body": map[string]any{"chapter_code": "vol01.ch01", "title": "不应创建"}, "response_filter": ".data | {uuid}"},
		{"method": "POST", "url": "/api/v1/projects/" + g.ProjectUUID + "/chapters/" + newUUID() + "/generations", "request_body": map[string]any{"prompt_key": "story_chapter", "prompt": "不应生成"}, "response_filter": ".data | {uuid}"},
		{"method": "DELETE", "url": "/api/v1/projects/" + g.ProjectUUID + "/chapters/" + newUUID(), "request_body": map[string]any{"expected_revision": float64(1)}, "response_filter": ".data | {uuid}"},
	} {
		result := s.Call(t.Context(), g, args, newUUID())
		if result["success"] != false || result["error"].(map[string]any)["code"] != project.CodeProjectInitializing {
			t.Fatalf("MCP write escaped initialization guard: %+v", result)
		}
	}
	if dispatched != 1 {
		t.Fatal("blocked write reached dispatcher")
	}
	replay := s.Call(t.Context(), g, read, "before-initialization")
	if replay["success"] != true || dispatched != 1 {
		t.Fatalf("completed call replay was blocked or reexecuted: %+v", replay)
	}
	current := s.Call(t.Context(), g, read, "during-initialization")
	if current["success"] != true || dispatched != 2 {
		t.Fatalf("new read blocked during initialization: %+v", current)
	}
	var count int64
	if err := s.app.DB().Model(&Call{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("blocked writes created pending/executing calls: count=%d err=%v", count, err)
	}
}

func TestMCPConfirmationRechecksInitializationBeforeExecution(t *testing.T) {
	s, g := activityHarness(t)
	dispatched := false
	s.api = agent.NewExternalProjectAPI(func(context.Context, agent.ProjectAPIDispatchRequest) (agent.ProjectAPIDispatchResponse, error) {
		dispatched = true
		return agent.ProjectAPIDispatchResponse{}, nil
	}, nil)
	result := s.Call(t.Context(), g, map[string]any{"method": "DELETE", "url": "/api/v1/projects/" + g.ProjectUUID + "/chapters/" + newUUID(), "request_body": map[string]any{"expected_revision": float64(1)}, "response_filter": ".data | {uuid}"}, "pending-before-initialization")
	if result["success"] != true {
		t.Fatalf("create pending call: %+v", result)
	}
	call := result["data"].(map[string]any)["call"].(Call)
	seedMCPInitialization(t, s, g)
	result = s.Decide(t.Context(), g.ProjectUUID, call.UUID, call.Fingerprint, "approve")
	if result["success"] != true || dispatched {
		t.Fatalf("confirmation escaped guard: %+v dispatched=%v", result, dispatched)
	}
	data := result["data"].(map[string]any)
	if data["call"].(Call).Status != "failed" {
		t.Fatalf("blocked confirmation not persisted: %+v", result)
	}
	business := data["result"].(map[string]any)
	if business["error"].(map[string]any)["code"] != project.CodeProjectInitializing {
		t.Fatalf("lost initialization error: %+v", result)
	}
}
