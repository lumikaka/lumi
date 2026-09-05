package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
)

func TestDirectoryRenameReopensRealProjectRuntime(t *testing.T) {
	dataDir := t.TempDir()
	dsn := config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite"))
	appStore, err := appstore.Open(dataDir, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = appStore.Close() })
	manager := project.NewManager(appStore)
	application, err := New(config.Config{
		Environment: "test", Address: ":0", FrontendURL: "http://localhost:5801",
		ViteDevServerURL: "http://127.0.0.1:5802", AppDataDir: dataDir, DatabaseDSN: dsn,
	}, appStore, manager)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	application.lifecycleCancel()
	<-application.lifecycleDone
	created, err := manager.Create(t.Context(), "Original", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	releasePresence, err := manager.AcquirePresence(created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	defer releasePresence()
	body, _ := json.Marshal(map[string]any{"name": "勇敢的小火车", "description": "", "expected_revision": 1, "rename_directory": true})
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/"+created.UUID, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:5801")
	response := httptest.NewRecorder()
	application.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", response.Code, response.Body.String())
	}
	controller := newProjectLifecycleController(manager, projectIdleGrace)
	if err := controller.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	recent, err := appStore.RecentProject(t.Context(), created.UUID)
	if err != nil || filepath.Base(recent.RootPath) != "勇敢的小火车" || recent.PendingDirectoryName != "" {
		t.Fatalf("renamed=%+v err=%v", recent, err)
	}
	for _, suffix := range []string{"", "/chat_threads", "/tasks"} {
		response := httptest.NewRecorder()
		application.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+created.UUID+suffix, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("reopened %s: %d %s", suffix, response.Code, response.Body.String())
		}
	}
	activity, open := manager.Activity(created.UUID)
	if !open || activity.PresenceLeases != 1 {
		t.Fatalf("presence=%+v open=%v", activity, open)
	}
}
