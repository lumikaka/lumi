package jobqueue

import (
	"context"
	"encoding/json"
	"lumi/internal/agent"
	"lumi/internal/mcpserver"
	"lumi/internal/project"
	"lumi/internal/sitesettings"
	"testing"
)

func TestMCPGenerationPersistsWithoutChatAndRecoversAfterClose(t *testing.T) {
	h := newQueueHarness(t)
	chapter := h.createChapter(t, "vol01.ch01")
	api := agent.NewExternalProjectAPI(nil, h.queue)
	s, err := mcpserver.New(h.app, h.projects, api, nil)
	if err != nil {
		t.Fatal(err)
	}
	grant, _, err := s.CreateGrant(t.Context(), h.project.UUID, "generation integration", "edit")
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"method": "POST", "url": "/api/v1/projects/" + h.project.UUID + "/chapters/" + chapter.UUID + "/generations", "request_body": map[string]any{"prompt_key": "story_chapter", "prompt": "[restart] continue story"}, "response_filter": ".data | {uuid,status,kind}"}
	ctx, cancel := context.WithCancel(t.Context())
	result := s.Call(ctx, grant, args, "generation-1")
	cancel()
	data := result["data"].(map[string]any)
	encoded, _ := json.Marshal(data["call"])
	var call mcpserver.Call
	_ = json.Unmarshal(encoded, &call)
	business, ok := data["result"].(map[string]any)
	if !ok || business["success"] != true {
		t.Fatalf("%+v", result)
	}
	taskUUID := business["data"].(map[string]any)["uuid"].(string)
	waitFakeStarted(t, h.fakeModel, "restart")
	// Dropping the submitting context cannot cancel the accepted durable job.
	task, err := h.queue.GetTask(t.Context(), h.project.UUID, taskUUID)
	if err != nil || task.Status == StatusCancelled {
		t.Fatalf("%+v %v", task, err)
	}
	var n int64
	if err = h.runtime(t).store.DB().Raw("SELECT COUNT(*) FROM chat_threads").Scan(&n).Error; err != nil || n != 0 {
		t.Fatalf("fabricated chat threads: %d %v", n, err)
	}
	if task.IdempotencyKey != "mcp:"+call.UUID || task.ProviderUUID != h.provider.UUID || task.Model == "" {
		t.Fatalf("ownership/model: %+v", task)
	}
	if _, err = h.projects.CloseProject(t.Context(), h.project.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.projects.OpenRecent(t.Context(), h.project.UUID); err != nil {
		t.Fatal(err)
	}
	// Simulate interruption after the task commit but before saving the MCP result.
	if err = h.app.DB().Model(&mcpserver.Call{}).Where("uuid = ?", call.UUID).Updates(map[string]any{"status": "executing", "result": ""}).Error; err != nil {
		t.Fatal(err)
	}
	recovered, err := mcpserver.New(h.app, h.projects, api, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Accepted tasks retain their configuration even if the user later changes
	// provider verification. Recovery must not try to submit a second job.
	if err := h.app.DB().Exec("UPDATE site_settings SET value = 'false' WHERE key = ?", sitesettings.CloudflareVerifiedKey).Error; err != nil {
		t.Fatal(err)
	}
	again := recovered.Call(t.Context(), grant, args, "generation-1")
	got := again["data"].(map[string]any)["result"].(map[string]any)
	if got["success"] != true || got["data"].(map[string]any)["uuid"] != taskUUID {
		t.Fatalf("duplicate recovery: %+v", again)
	}
	waitTaskStatus(t, h.queue, h.project.UUID, taskUUID, StatusCompleted)
	if err = h.projects.WithStore(t.Context(), h.project.UUID, func(store *project.Store) error {
		return store.DB().Raw("SELECT COUNT(*) FROM task_runs WHERE idempotency_key = ?", "mcp:"+call.UUID).Scan(&n).Error
	}); err != nil || n != 1 {
		t.Fatalf("duplicate jobs: %d %v", n, err)
	}
}
