package httpapi

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
	"lumi/internal/story"

	"github.com/labstack/echo/v4"
)

func productionAPIHarness(t *testing.T) (*echo.Echo, string, story.Chapter) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(app).WithOpenHook(story.ReconcileOnOpen)
	created, err := manager.Create(t.Context(), "Production API", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var chapter story.Chapter
	if err := manager.WithCurrentStore(t.Context(), created.UUID, func(store *project.Store) error {
		var createErr error
		chapter, createErr = story.NewService(store).CreateChapter(t.Context(), story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Opening"})
		return createErr
	}); err != nil {
		t.Fatal(err)
	}
	handler := NewProductionHandler(manager, nil, nil)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.GET("/api/v1/projects/:project_uuid/premise", handler.ShowPremise)
	e.POST("/api/v1/projects/:project_uuid/premise-sources", handler.CreatePremiseSource)
	e.GET("/api/v1/projects/:project_uuid/premise-sources", handler.ListPremiseSources)
	e.GET("/api/v1/projects/:project_uuid/premise-setting-images", handler.ListSettingImages)
	e.PATCH("/api/v1/projects/:project_uuid/premise-sources/:source_uuid", handler.UpdatePremiseSource)
	e.POST("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-sections", handler.CreateSection)
	e.GET("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic", handler.ShowComicState)
	e.GET("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-snapshots", handler.ListSnapshots)
	e.GET("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-snapshots/:snapshot_uuid", handler.ShowSnapshot)
	e.GET("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-sections", handler.ListSections)
	e.POST("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/storyboard-variants", handler.CreateStoryboard)
	e.GET("/api/v1/projects/:project_uuid/comic-exports/readiness", handler.ExportReadiness)
	e.GET("/api/v1/projects/:project_uuid/comic-exports", handler.ListExports)
	e.DELETE("/api/v1/projects/:project_uuid/premise-assets/trash", handler.EmptyPremiseAssetTrash)
	e.DELETE("/api/v1/projects/:project_uuid/premise-assets/:premise_asset_uuid/permanent", handler.PermanentlyDeletePremiseAsset)
	t.Cleanup(func() { _ = manager.Close(); _ = app.Close() })
	return e, created.UUID, chapter
}

func TestProductionAPIUsesEnvelopesAndPublicUUIDs(t *testing.T) {
	e, projectUUID, chapter := productionAPIHarness(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectUUID+"/premise-sources", strings.NewReader(`{"source_text":"A lantern city","style_snapshot":"ink","source_type":"manual","parameters":{"temperature":0.2}}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("source status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	source := envelopeData(t, recorder)
	if !productionUUIDv7(source["uuid"]) || source["style_snapshot"] != "ink" {
		t.Fatalf("source=%#v", source)
	}
	sourceList := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+projectUUID+"/premise-sources?page=1&per_page=1", nil)
	if sourceList.Code != http.StatusOK || !strings.Contains(sourceList.Body.String(), `"pagination":{"per_page":1,"current_page":1,"last_page":1,"total":1}`) || strings.Contains(sourceList.Body.String(), `"id":`) {
		t.Fatalf("source pagination status=%d body=%s", sourceList.Code, sourceList.Body.String())
	}
	filteredSettings := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+projectUUID+"/premise-setting-images?source_uuid="+source["uuid"].(string), nil)
	if filteredSettings.Code != http.StatusOK || !strings.Contains(filteredSettings.Body.String(), `"items":[]`) {
		t.Fatalf("setting filter status=%d body=%s", filteredSettings.Code, filteredSettings.Body.String())
	}
	patched := requestJSON(t, e, http.MethodPatch, "/api/v1/projects/"+projectUUID+"/premise-sources/"+source["uuid"].(string), map[string]any{"ignored": false, "expected_revision": 0})
	if patched.Code != http.StatusOK || !strings.Contains(patched.Body.String(), `"revision":0`) || strings.Contains(patched.Body.String(), `"id":`) {
		t.Fatalf("source patch status=%d body=%s", patched.Code, patched.Body.String())
	}
	invalidPatch := requestJSON(t, e, http.MethodPatch, "/api/v1/projects/"+projectUUID+"/premise-sources/"+source["uuid"].(string), map[string]any{"ignored": true})
	if invalidPatch.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid source patch status=%d body=%s", invalidPatch.Code, invalidPatch.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectUUID+"/chapters/"+chapter.UUID+"/comic-sections", strings.NewReader(`{"title":"Frame one","description_md":"Opening","storyboard_md":"# Wide shot"}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("section status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	section := envelopeData(t, recorder)
	if !productionUUIDv7(section["uuid"]) || section["chapter_uuid"] != chapter.UUID {
		t.Fatalf("section=%#v", section)
	}
	snapshotsResponse := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+projectUUID+"/chapters/"+chapter.UUID+"/comic-snapshots", nil)
	if snapshotsResponse.Code != http.StatusOK || !strings.Contains(snapshotsResponse.Body.String(), `"section_count":1`) || strings.Contains(snapshotsResponse.Body.String(), `"snapshot":`) || strings.Contains(snapshotsResponse.Body.String(), `"snapshot_hash"`) || strings.Contains(snapshotsResponse.Body.String(), `"id":`) {
		t.Fatalf("snapshot summaries status=%d body=%s", snapshotsResponse.Code, snapshotsResponse.Body.String())
	}
	snapshotItems := envelopeData(t, snapshotsResponse)["items"].([]any)
	snapshotUUID := snapshotItems[0].(map[string]any)["uuid"].(string)
	detailResponse := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+projectUUID+"/chapters/"+chapter.UUID+"/comic-snapshots/"+snapshotUUID, nil)
	if detailResponse.Code != http.StatusOK || !strings.Contains(detailResponse.Body.String(), `"chapter_code":"vol01.ch01"`) || !strings.Contains(detailResponse.Body.String(), `"storyboard_md":"# Wide shot"`) || strings.Contains(detailResponse.Body.String(), `"snapshot_json"`) || strings.Contains(detailResponse.Body.String(), `"key_path"`) || strings.Contains(detailResponse.Body.String(), `"id":`) {
		t.Fatalf("snapshot detail status=%d body=%s", detailResponse.Code, detailResponse.Body.String())
	}
	for _, forbidden := range []string{`"id"`, "river_job_id", "file_object_id", "key_path", "project.sqlite", "/Users/", "/private/"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("production response leaked %q: %s", forbidden, recorder.Body.String())
		}
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectUUID+"/chapters/"+chapter.UUID+"/comic-sections", nil)
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"items"`) {
		t.Fatalf("sections status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectUUID+"/chapters/"+chapter.UUID+"/comic", nil)
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"has_premise_assets":false`) || !strings.Contains(recorder.Body.String(), `"premise_asset_count":0`) || strings.Contains(recorder.Body.String(), `"id":`) {
		t.Fatalf("comic readiness status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectUUID+"/comic-exports/readiness?scope=chapter&chapter_uuid="+chapter.UUID, nil)
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"active_chapter_count":1`) || !strings.Contains(recorder.Body.String(), `"missing_section_count":1`) || !strings.Contains(recorder.Body.String(), `"chapter_uuid":"`+chapter.UUID+`"`) || strings.Contains(recorder.Body.String(), `"id":`) {
		t.Fatalf("readiness status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	exportsResponse := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+projectUUID+"/comic-exports?page=1&per_page=10&scope=chapter&chapter_uuid="+chapter.UUID, nil)
	if exportsResponse.Code != http.StatusOK || !strings.Contains(exportsResponse.Body.String(), `"pagination":{"per_page":10,"current_page":1,"last_page":1,"total":0}`) || strings.Contains(exportsResponse.Body.String(), `"id":`) {
		t.Fatalf("export pagination status=%d body=%s", exportsResponse.Code, exportsResponse.Body.String())
	}
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+projectUUID+"/premise-assets/trash", nil)
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"deleted_count":0`) || !strings.Contains(recorder.Body.String(), `"blocked_items":[]`) || strings.Contains(recorder.Body.String(), `"id":`) {
		t.Fatalf("empty trash status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+projectUUID+"/premise-assets/019ff013-ffff-7000-8000-000000000000/permanent", nil)
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), `"success":false`) {
		t.Fatalf("permanent delete validation status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func productionUUIDv7(value any) bool {
	text, ok := value.(string)
	return ok && len(text) == 36 && text[14] == '7'
}
