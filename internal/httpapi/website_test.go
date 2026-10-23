package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/production"
	"lumi/internal/project"
)

func TestWebsiteAPIStatusAndPreviewAllowlist(t *testing.T) {
	e, manager, uuid, _ := productionAPIHarness(t)
	h := NewProductionHandler(manager, nil, nil)
	e.GET("/api/v1/projects/:project_uuid/website", h.Website)
	e.GET("/media/projects/:project_uuid/website/*", h.WebsiteContent)
	base := "/api/v1/projects/" + uuid + "/website"
	response := requestJSON(t, e, http.MethodGet, base, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%s", response.Body.String())
	}
	var envelope struct {
		Success bool                     `json:"success"`
		Data    production.WebsiteStatus `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || !envelope.Success || envelope.Data.OutputDirectory != "website/" || envelope.Data.Available {
		t.Fatalf("envelope=%+v %v", envelope, err)
	}
	now := time.Now().UTC()
	err := manager.WithCurrentStore(t.Context(), uuid, func(store *project.Store) error {
		root := filepath.Join(store.Root(), "website")
		if err := os.MkdirAll(root, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<!doctype html><title>Reader</title>"), 0644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, "private.txt"), []byte("PRIVATE"), 0644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, "rss.xml"), []byte(`<rss version="2.0"><channel/></rss>`), 0644); err != nil {
			return err
		}
		// Backend IDs are only relationships in SQLite, never part of the DTO.
		if err := store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,created_at,updated_at) SELECT '01900000-0000-7000-8000-000000000991',id,'website_generation',uuid,'{}','completed','api',?,? FROM projects`, now, now).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO website_generations(uuid,project_id,task_id,snapshot_json,snapshot_hash,manifest_json,created_at,completed_at) SELECT '01900000-0000-7000-8000-000000000992',project_id,id,'{}','hash','{"files":{"index.html":{"size":35},"rss.xml":{"size":34}}}',?,? FROM production_task_runs WHERE idempotency_key='api'`, now, now).Error; err != nil {
			return err
		}
		return store.DB().Exec(`INSERT INTO project_websites(project_id,generation_id) SELECT project_id,id FROM website_generations`).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	media := "/media/projects/" + uuid + "/website/"
	response = requestJSON(t, e, http.MethodGet, media+"index.html", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Reader") || response.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("preview=%d %s", response.Code, response.Body.String())
	}
	response = requestJSON(t, e, http.MethodGet, media+"rss.xml", nil)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/rss+xml; charset=utf-8" || !strings.Contains(response.Body.String(), `<rss version="2.0">`) {
		t.Fatalf("RSS preview=%d %s", response.Code, response.Body.String())
	}
	for _, relative := range []string{"private.txt", "project.sqlite", "../project.sqlite", "%2e%2e/project.sqlite", "static/../../project.sqlite"} {
		response = requestJSON(t, e, http.MethodGet, media+relative, nil)
		if response.Code == http.StatusOK || strings.Contains(response.Body.String(), "PRIVATE") {
			t.Fatalf("preview exposed %s", relative)
		}
	}
	response = requestJSON(t, e, http.MethodGet, base, nil)
	for _, forbidden := range []string{`"id":`, `"project_id":`, `"task_id":`, `"generation_id":`, `"snapshot_json":`, `"manifest_json":`, `"root_path":`} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("API leaked %s", forbidden)
		}
	}
}
