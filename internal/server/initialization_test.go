package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/httpapi"
	"lumi/internal/project"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func TestInitializationGateCoversHTTPAndAgentDispatch(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	projects := project.NewManager(app)
	t.Cleanup(func() { _ = projects.Close() })
	created, err := projects.Create(ctx, "独占初始化", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	workflowID, _ := uuid.NewV7()
	providerID, _ := uuid.NewV7()
	now := time.Now().UTC()
	err = projects.WithStore(ctx, created.UUID, func(store *project.Store) error {
		return store.DB().Exec(`INSERT INTO workflows(uuid,project_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,created_at,updated_at)
			SELECT ?,id,'yolo_project_initialization','初始化','running',1,'{}',?,?,'test',?,? FROM projects WHERE uuid=?`, workflowID.String(), workflowID.String(), providerID.String(), now, now, created.UUID).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.HTTPErrorHandler = httpapi.ErrorHandler
	api := e.Group("/api/v1", projectRequestLease(projects))
	called := 0
	writeHandler := func(c echo.Context) error {
		called++
		return httpapi.Success(c, http.StatusOK, nil)
	}
	api.PATCH("/projects/:project_uuid", writeHandler)
	api.POST("/projects/:project_uuid/chat_threads", writeHandler)
	api.POST("/projects/:project_uuid/chat_threads/:thread_uuid/turns", writeHandler)
	api.POST("/projects/:project_uuid/workflows/:workflow_uuid/cancellations", writeHandler)
	api.GET("/projects/:project_uuid/chapters", func(c echo.Context) error { return httpapi.Success(c, http.StatusOK, nil) })
	initialization := httpapi.NewInitializationHandler(projects, nil)
	api.GET("/projects/:project_uuid/initialization", initialization.Show)
	api.POST("/projects/:project_uuid/initialization/handoffs", initialization.TakeOver)
	base := "/api/v1/projects/" + created.UUID
	for _, input := range []struct{ method, path string }{
		{http.MethodPatch, base}, {http.MethodPost, base + "/chat_threads"},
		{http.MethodPost, base + "/chat_threads/anything/turns"},
		{http.MethodPost, base + "/workflows/other/cancellations"},
	} {
		recorder := httptest.NewRecorder()
		e.ServeHTTP(recorder, httptest.NewRequest(input.method, input.path, nil))
		var body httpapi.Envelope
		_ = json.Unmarshal(recorder.Body.Bytes(), &body)
		if recorder.Code != http.StatusConflict || body.Success || body.Data != nil || body.Error == nil || body.Error.Code != project.CodeProjectInitializing {
			t.Fatalf("%s %s response=%d %s", input.method, input.path, recorder.Code, recorder.Body.String())
		}
	}
	dispatched, err := echoProjectAPIDispatcher(e)(ctx, agent.ProjectAPIDispatchRequest{Method: http.MethodPatch, Path: base, Body: map[string]any{"name": "越过界面"}, HasBody: true})
	if err != nil || dispatched.Status != http.StatusConflict || called != 0 {
		t.Fatalf("dispatch bypassed lock: %+v err=%v called=%d", dispatched, err, called)
	}
	for _, input := range []struct{ method, path string }{
		{http.MethodGet, base + "/chapters"}, {http.MethodGet, base + "/initialization"},
		{http.MethodPost, base + "/workflows/" + workflowID.String() + "/cancellations"},
	} {
		recorder := httptest.NewRecorder()
		e.ServeHTTP(recorder, httptest.NewRequest(input.method, input.path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected allowed %s %s: %s", input.method, input.path, recorder.Body.String())
		}
	}
	err = projects.WithStore(ctx, created.UUID, func(store *project.Store) error {
		return store.DB().Exec("UPDATE workflows SET status='cancelled' WHERE uuid=?", workflowID.String()).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, base+"/initialization/handoffs", strings.NewReader(`{"workflow_uuid":"`+workflowID.String()+`"}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"handed_off"`) {
		t.Fatalf("handoff response=%d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, base, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("handoff did not restore writes: %s", recorder.Body.String())
	}
}
