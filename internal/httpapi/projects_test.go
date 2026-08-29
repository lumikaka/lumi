package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/files"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func projectAPIHarness(t *testing.T) (*echo.Echo, *project.Manager) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "app")
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(store)
	t.Cleanup(func() {
		_ = manager.Close()
		_ = store.Close()
	})
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	projects := NewProjectHandler(manager)
	story := NewStoryHandler(manager)
	defaults := NewProjectDefaultsHandler()
	open := NewOpenProjectHandler(manager)
	recent := NewRecentProjectHandler(manager)
	e.POST("/api/v1/projects", projects.Create)
	e.GET("/api/v1/projects/:project_uuid", story.ShowProject)
	e.GET("/api/v1/project-defaults", defaults.Show)
	e.GET("/api/v1/open-projects", open.Index)
	e.POST("/api/v1/open-projects", open.Create)
	e.PUT("/api/v1/open-projects/:project_uuid", open.Update)
	e.DELETE("/api/v1/open-projects/:project_uuid", open.Delete)
	e.GET("/api/v1/recent-projects", recent.Index)
	e.PATCH("/api/v1/recent-projects/:project_uuid", recent.Update)
	e.DELETE("/api/v1/recent-projects/:project_uuid", recent.Delete)
	e.GET("/media/recent-projects/:project_uuid/cover", recent.Cover)
	return e, manager
}

func TestRecentProjectsExposeFirstPictureBookImageAsCover(t *testing.T) {
	e, manager := projectAPIHarness(t)
	created := requestJSON(t, e, http.MethodPost, "/api/v1/projects", map[string]any{"name": "Cover Book", "parent_path": t.TempDir()})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	projectUUID := envelopeData(t, created)["uuid"].(string)
	content := apiPNG(t)

	if err := manager.WithStore(t.Context(), projectUUID, func(store *project.Store) error {
		service := files.NewService(store, nil)
		upload, err := service.CreateUpload(t.Context(), files.CreateUploadInput{
			Purpose:          "comic_section_image",
			OriginalFilename: "first-page.png",
			DisplayName:      "First page",
			Reader:           bytes.NewReader(content),
		})
		if err != nil {
			return err
		}
		asset, err := service.FinalizeUpload(t.Context(), upload.UUID, "comic_section_image")
		if err != nil {
			return err
		}

		var projectID, actorID, fileID int64
		if err := store.DB().Table("projects").Where("uuid = ?", projectUUID).Pluck("id", &projectID).Error; err != nil {
			return err
		}
		if err := store.DB().Table("actors").Where("kind = ?", "local_user").Order("id").Limit(1).Pluck("id", &actorID).Error; err != nil {
			return err
		}
		if err := store.DB().Table("files").Where("uuid = ?", asset.UUID).Pluck("id", &fileID).Error; err != nil {
			return err
		}

		now := time.Now().UTC()
		chapterUUID := projectAPITestUUIDv7(t)
		stateUUID := projectAPITestUUIDv7(t)
		sectionUUID := projectAPITestUUIDv7(t)
		variantUUID := projectAPITestUUIDv7(t)
		if err := store.DB().Exec(`INSERT INTO chapters(uuid,project_id,volume_no,chapter_no,chapter_code,sort_order,title,revision,created_at,updated_at) VALUES(?,?,1,1,'chapter-001',1,'Opening',1,?,?)`, chapterUUID, projectID, now, now).Error; err != nil {
			return err
		}
		var chapterID int64
		if err := store.DB().Table("chapters").Where("uuid = ?", chapterUUID).Pluck("id", &chapterID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO chapter_comic_states(uuid,chapter_id,status,revision,created_at,updated_at) VALUES(?,?,'ready',1,?,?)`, stateUUID, chapterID, now, now).Error; err != nil {
			return err
		}
		var stateID int64
		if err := store.DB().Table("chapter_comic_states").Where("uuid = ?", stateUUID).Pluck("id", &stateID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO comic_sections(uuid,chapter_comic_state_id,actor_id,section_no,title,description_md,revision,created_at,updated_at) VALUES(?,?,?,1,'First page','The first illustrated page.',1,?,?)`, sectionUUID, stateID, actorID, now, now).Error; err != nil {
			return err
		}
		var sectionID int64
		if err := store.DB().Table("comic_sections").Where("uuid = ?", sectionUUID).Pluck("id", &sectionID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO comic_image_variants(uuid,comic_section_id,file_id,actor_id,version_no,source_type,input_snapshot,created_at) VALUES(?,?,?,?,1,'manual','{}',?)`, variantUUID, sectionID, fileID, actorID, now).Error; err != nil {
			return err
		}
		var variantID int64
		if err := store.DB().Table("comic_image_variants").Where("uuid = ?", variantUUID).Pluck("id", &variantID).Error; err != nil {
			return err
		}
		return store.DB().Table("comic_sections").Where("id = ?", sectionID).Update("current_image_variant_id", variantID).Error
	}); err != nil {
		t.Fatal(err)
	}

	if response := requestJSON(t, e, http.MethodDelete, "/api/v1/open-projects/"+projectUUID, nil); response.Code != http.StatusOK {
		t.Fatalf("close status=%d body=%s", response.Code, response.Body.String())
	}
	recent := requestJSON(t, e, http.MethodGet, "/api/v1/recent-projects", nil)
	contentSHA256 := fmt.Sprintf("%x", sha256.Sum256(content))
	expectedURL := "/media/recent-projects/" + projectUUID + "/cover?v=" + contentSHA256
	if recent.Code != http.StatusOK || !strings.Contains(recent.Body.String(), `"cover_image_url":"`+expectedURL+`"`) || strings.Contains(recent.Body.String(), `"id"`) {
		t.Fatalf("recent status=%d body=%s", recent.Code, recent.Body.String())
	}

	cover := requestJSON(t, e, http.MethodGet, expectedURL, nil)
	if cover.Code != http.StatusOK || !bytes.Equal(cover.Body.Bytes(), content) {
		t.Fatalf("cover status=%d body_length=%d", cover.Code, cover.Body.Len())
	}
	for _, header := range []string{"Content-Type", "Content-Length", "ETag", "Last-Modified", "X-Content-Type-Options", "Cache-Control", "Content-Disposition"} {
		if cover.Header().Get(header) == "" {
			t.Fatalf("missing %s", header)
		}
	}
	if cover.Header().Get("Content-Type") != "image/png" || cover.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("cover headers=%v", cover.Header())
	}
}

func projectAPITestUUIDv7(t *testing.T) string {
	t.Helper()
	value, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return value.String()
}

func TestOpenProjectsAreIndependentAndClosedProjectsReturnProjectNotOpen(t *testing.T) {
	e, _ := projectAPIHarness(t)
	firstResponse := requestJSON(t, e, http.MethodPost, "/api/v1/projects", map[string]any{"name": "API First", "parent_path": t.TempDir()})
	secondResponse := requestJSON(t, e, http.MethodPost, "/api/v1/projects", map[string]any{"name": "API Second", "parent_path": t.TempDir()})
	if firstResponse.Code != http.StatusCreated || secondResponse.Code != http.StatusCreated {
		t.Fatalf("create statuses = %d/%d, bodies = %s / %s", firstResponse.Code, secondResponse.Code, firstResponse.Body.String(), secondResponse.Body.String())
	}
	firstUUID := envelopeData(t, firstResponse)["uuid"].(string)
	secondUUID := envelopeData(t, secondResponse)["uuid"].(string)
	openResponse := requestJSON(t, e, http.MethodGet, "/api/v1/open-projects", nil)
	if openResponse.Code != http.StatusOK || !strings.Contains(openResponse.Body.String(), firstUUID) || !strings.Contains(openResponse.Body.String(), secondUUID) {
		t.Fatalf("open projects = %d %s", openResponse.Code, openResponse.Body.String())
	}
	for _, projectUUID := range []string{firstUUID, secondUUID} {
		response := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+projectUUID, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("project %s status = %d body = %s", projectUUID, response.Code, response.Body.String())
		}
	}
	if response := requestJSON(t, e, http.MethodDelete, "/api/v1/open-projects/"+firstUUID, nil); response.Code != http.StatusOK {
		t.Fatalf("close first = %d %s", response.Code, response.Body.String())
	}
	closedProject := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+firstUUID, nil)
	if closedProject.Code != http.StatusConflict || !strings.Contains(closedProject.Body.String(), `"code":"project_not_open"`) {
		t.Fatalf("closed project = %d %s", closedProject.Code, closedProject.Body.String())
	}
	remainingProject := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+secondUUID, nil)
	if remainingProject.Code != http.StatusOK {
		t.Fatalf("remaining project = %d %s", remainingProject.Code, remainingProject.Body.String())
	}
	unknownProject := requestJSON(t, e, http.MethodGet, "/api/v1/projects/01989abc-def0-7000-8000-000000000099", nil)
	if unknownProject.Code != http.StatusNotFound || !strings.Contains(unknownProject.Body.String(), `"code":"project_not_found"`) {
		t.Fatalf("unknown project = %d %s", unknownProject.Code, unknownProject.Body.String())
	}
}

func TestProjectDefaultsHandlerReturnsResolvedParentAndErrors(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "Documents", "Lumi")
	handler := NewProjectDefaultsHandler()
	handler.resolveParentPath = func() (string, error) { return parent, nil }
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.GET("/api/v1/project-defaults", handler.Show)

	response := requestJSON(t, e, http.MethodGet, "/api/v1/project-defaults", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("defaults status = %d, body = %s", response.Code, response.Body.String())
	}
	data := envelopeData(t, response)
	styles := data["default_overall_styles"].(map[string]any)
	if data["parent_path"] != parent || styles["zh-Hans"] != promptcatalog.DefaultProjectStyle("zh-Hans") || styles["en"] != promptcatalog.DefaultProjectStyle("en") || strings.Contains(response.Body.String(), `"id"`) {
		t.Fatalf("defaults body = %s", response.Body.String())
	}

	handler.resolveParentPath = func() (string, error) {
		return "", &project.Error{
			Code: project.CodeDefaultProjectParentUnavailable, Message: "无法确定默认项目目录",
			Details: "请检查系统 Documents 目录是否可用。", Err: errors.New("known folder unavailable"),
		}
	}
	failed := requestJSON(t, e, http.MethodGet, "/api/v1/project-defaults", nil)
	if failed.Code != http.StatusInternalServerError || !strings.Contains(failed.Body.String(), `"code":"default_project_parent_unavailable"`) || !strings.Contains(failed.Body.String(), `"data":null`) {
		t.Fatalf("failed defaults status = %d, body = %s", failed.Code, failed.Body.String())
	}
}

func TestProjectHandlersCreateOpenMissingAndRelocate(t *testing.T) {
	e, _ := projectAPIHarness(t)
	parent := t.TempDir()
	createResponse := requestJSON(t, e, http.MethodPost, "/api/v1/projects", map[string]any{"name": "API Book", "parent_path": parent})
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	created := envelopeData(t, createResponse)
	projectUUID := created["uuid"].(string)
	rootPath := created["root_path"].(string)
	if strings.Contains(createResponse.Body.String(), `"id"`) {
		t.Fatalf("create response leaked internal id: %s", createResponse.Body.String())
	}

	closed := requestJSON(t, e, http.MethodDelete, "/api/v1/open-projects/"+projectUUID, nil)
	if closed.Code != http.StatusOK {
		t.Fatalf("close status = %d, body = %s", closed.Code, closed.Body.String())
	}
	movedRoot := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(rootPath, movedRoot); err != nil {
		t.Fatal(err)
	}
	list := requestJSON(t, e, http.MethodGet, "/api/v1/recent-projects", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"status":"recent"`) || !strings.Contains(list.Body.String(), `"available":true`) || strings.Contains(list.Body.String(), project.CodeProjectNotFound) {
		t.Fatalf("missing list status = %d, body = %s", list.Code, list.Body.String())
	}
	openMissing := requestJSON(t, e, http.MethodPut, "/api/v1/open-projects/"+projectUUID, nil)
	if openMissing.Code != http.StatusNotFound || !strings.Contains(openMissing.Body.String(), project.CodeProjectNotFound) {
		t.Fatalf("open missing status = %d, body = %s", openMissing.Code, openMissing.Body.String())
	}

	relocated := requestJSON(t, e, http.MethodPatch, "/api/v1/recent-projects/"+projectUUID, map[string]any{"root_path": movedRoot})
	if relocated.Code != http.StatusOK || !strings.Contains(relocated.Body.String(), projectUUID) {
		t.Fatalf("relocate status = %d, body = %s", relocated.Code, relocated.Body.String())
	}
	forgotten := requestJSON(t, e, http.MethodDelete, "/api/v1/recent-projects/"+projectUUID, nil)
	if forgotten.Code != http.StatusOK {
		t.Fatalf("forget status = %d, body = %s", forgotten.Code, forgotten.Body.String())
	}
	if _, err := os.Stat(movedRoot); err != nil {
		t.Fatalf("forget deleted project directory: %v", err)
	}
}

func TestProjectCreateNormalizesPictureBookProfileAndRejectsIrrelevantFields(t *testing.T) {
	e, manager := projectAPIHarness(t)
	parent := t.TempDir()
	customStyle := "纸雕拼贴、可见纤维、柔和侧光"
	created := requestJSON(t, e, http.MethodPost, "/api/v1/projects", map[string]any{
		"name":          "Interactive Book",
		"parent_path":   parent,
		"overall_style": customStyle,
		"picture_book": map[string]any{
			"format":           "interactive_picture_book",
			"interaction_mode": "guess",
		},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	data := envelopeData(t, created)
	pictureBook := data["picture_book"].(map[string]any)
	ratio := pictureBook["aspect_ratio"].(map[string]any)
	if pictureBook["format"] != "interactive_picture_book" || pictureBook["interaction_mode"] != "guess" || pictureBook["large_image_minimal_text"] != nil || pictureBook["comic_layout"] != nil || ratio["mode"] != "landscape" || ratio["width"] != float64(4) || ratio["height"] != float64(3) {
		t.Fatalf("normalized picture_book=%+v", pictureBook)
	}
	if err := manager.WithStore(context.Background(), data["uuid"].(string), func(store *project.Store) error {
		var premiseStyle string
		var promptRecord struct {
			Prompt     string
			SourceType string
		}
		if err := store.DB().Table("premise_profiles").Select("default_style").Scan(&premiseStyle).Error; err != nil {
			return err
		}
		if err := store.DB().Table("project_prompt_versions").Select("prompt, source_type").Where("prompt_group = ? AND prompt_key = ?", "premise_style", "project_overall_style").Scan(&promptRecord).Error; err != nil {
			return err
		}
		if premiseStyle != customStyle || promptRecord.Prompt != customStyle || promptRecord.SourceType != "project_created" {
			t.Fatalf("persisted styles=%q/%q source=%q", premiseStyle, promptRecord.Prompt, promptRecord.SourceType)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	open := requestJSON(t, e, http.MethodGet, "/api/v1/open-projects", nil)
	if open.Code != http.StatusOK || !strings.Contains(open.Body.String(), `"status":"open"`) || !strings.Contains(open.Body.String(), `"open":true`) || strings.Contains(open.Body.String(), `"id"`) {
		t.Fatalf("open projects=%d %s", open.Code, open.Body.String())
	}

	invalidParent := t.TempDir()
	invalid := requestJSON(t, e, http.MethodPost, "/api/v1/projects", map[string]any{
		"name":        "Invalid Book",
		"parent_path": invalidParent,
		"picture_book": map[string]any{
			"format":           "wordless_picture_book",
			"interaction_mode": "find_it",
		},
	})
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"code":"invalid_picture_book_profile"`) {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	entries, err := os.ReadDir(invalidParent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid create wrote project directory: entries=%v err=%v", entries, err)
	}

	tooLongParent := t.TempDir()
	tooLong := requestJSON(t, e, http.MethodPost, "/api/v1/projects", map[string]any{
		"name":          "Too Much Style",
		"parent_path":   tooLongParent,
		"overall_style": strings.Repeat("画", 12001),
	})
	if tooLong.Code != http.StatusUnprocessableEntity || !strings.Contains(tooLong.Body.String(), `"code":"invalid_overall_style"`) {
		t.Fatalf("long style status=%d body=%s", tooLong.Code, tooLong.Body.String())
	}
	entries, err = os.ReadDir(tooLongParent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("long style create wrote project directory: entries=%v err=%v", entries, err)
	}
}

func TestProjectHandlersReturnErrorEnvelope(t *testing.T) {
	e, _ := projectAPIHarness(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/open-projects", strings.NewReader(`{"unknown":true}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid request status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var envelope Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Success || envelope.Data != nil || envelope.Error == nil || envelope.Error.Code != "invalid_json" {
		t.Fatalf("error envelope = %+v", envelope)
	}

	notFound := requestJSON(t, e, http.MethodPut, "/api/v1/open-projects/01989abc-def0-7000-8000-000000000001", nil)
	if notFound.Code != http.StatusNotFound || !strings.Contains(notFound.Body.String(), `"data":null`) {
		t.Fatalf("not found status = %d, body = %s", notFound.Code, notFound.Body.String())
	}
	invalidUUID := requestJSON(t, e, http.MethodPut, "/api/v1/open-projects/42", nil)
	if invalidUUID.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidUUID.Body.String(), `"code":"invalid_uuid"`) {
		t.Fatalf("invalid UUID status = %d, body = %s", invalidUUID.Code, invalidUUID.Body.String())
	}
}

func TestProjectHandlerReturnsOriginalNameAndSuffixedRootPath(t *testing.T) {
	e, _ := projectAPIHarness(t)
	parent := t.TempDir()
	canonicalParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(parent, "API-Book")
	if err := os.Mkdir(existing, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existing, "sentinel"), []byte("existing"), 0o640); err != nil {
		t.Fatal(err)
	}

	response := requestJSON(t, e, http.MethodPost, "/api/v1/projects", map[string]any{"name": "API Book", "parent_path": parent})
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	created := envelopeData(t, response)
	if created["name"] != "API Book" || created["root_path"] != filepath.Join(canonicalParent, "API-Book-2") {
		t.Fatalf("created = %+v", created)
	}
	content, err := os.ReadFile(filepath.Join(existing, "sentinel"))
	if err != nil || string(content) != "existing" {
		t.Fatalf("existing candidate content=%q error=%v", content, err)
	}
}

func TestProjectHandlerReturnsConflictWhenDirectoryNamesAreExhausted(t *testing.T) {
	e, _ := projectAPIHarness(t)
	parent := t.TempDir()
	for number := 1; number <= 1000; number++ {
		name := "Exhausted"
		if number > 1 {
			name += fmt.Sprintf("-%d", number)
		}
		if err := os.Mkdir(filepath.Join(parent, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	response := requestJSON(t, e, http.MethodPost, "/api/v1/projects", map[string]any{"name": "Exhausted", "parent_path": parent})
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"data":null`) || !strings.Contains(response.Body.String(), `"code":"project_directory_name_exhausted"`) {
		t.Fatalf("exhausted status = %d, body = %s", response.Code, response.Body.String())
	}
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		content, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = bytes.NewReader(content)
	}
	request := httptest.NewRequest(method, path, requestBody)
	if body != nil {
		request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func envelopeData(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Success {
		t.Fatalf("response = %s", recorder.Body.String())
	}
	return envelope.Data
}
