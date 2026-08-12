package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/story"

	"github.com/labstack/echo/v4"
)

func storyAPIHarness(t *testing.T) (*echo.Echo, project.Summary) {
	t.Helper()
	dataDirectory := filepath.Join(t.TempDir(), "app")
	appStore, err := appstore.Open(dataDirectory, config.SQLiteDSN(filepath.Join(dataDirectory, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(appStore).WithOpenHook(story.ReconcileOnOpen)
	created, err := manager.Create(context.Background(), "API Story", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = manager.Close()
		_ = appStore.Close()
	})
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	handler := NewStoryHandler(manager)
	e.GET("/api/v1/projects/:project_uuid", handler.ShowProject)
	e.PATCH("/api/v1/projects/:project_uuid", handler.UpdateProject)
	e.GET("/api/v1/projects/:project_uuid/chapters", handler.ListChapters)
	e.POST("/api/v1/projects/:project_uuid/chapters", handler.CreateChapter)
	e.DELETE("/api/v1/projects/:project_uuid/chapters/trash", handler.EmptyChapterTrash)
	e.DELETE("/api/v1/projects/:project_uuid/chapters/:chapter_uuid", handler.TrashChapter)
	e.DELETE("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/permanent", handler.PermanentlyDeleteChapter)
	e.POST("/api/v1/projects/:project_uuid/chapter-imports", handler.ImportChapters)
	e.GET("/api/v1/projects/:project_uuid/chapters/:chapter_uuid", handler.ShowChapter)
	e.PUT("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/current-story", handler.UpdateCurrentStory)
	e.GET("/api/v1/projects/:project_uuid/story-profile", handler.ShowStoryProfile)
	e.POST("/api/v1/projects/:project_uuid/story-profile/imports", handler.ImportExternalStoryMD)
	e.GET("/api/v1/projects/:project_uuid/prompts", handler.ListPromptCatalog)
	e.PATCH("/api/v1/projects/:project_uuid/prompt-groups/:prompt_group", handler.UpdatePromptGroup)
	e.GET("/api/v1/projects/:project_uuid/prompt-versions", handler.ListPromptVersions)
	e.POST("/api/v1/projects/:project_uuid/prompt-versions", handler.CreatePromptVersion)
	e.POST("/api/v1/projects/:project_uuid/prompt-versions/:version_uuid/restorations", handler.RestorePromptVersion)
	return e, created
}

func TestProjectUpdateRejectsImmutablePictureBookProfile(t *testing.T) {
	e, projectSummary := storyAPIHarness(t)
	response := requestJSON(t, e, http.MethodPatch, "/api/v1/projects/"+projectSummary.UUID, map[string]any{
		"picture_book": map[string]any{"format": "vertical_strip"},
	})
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"code":"picture_book_profile_immutable"`) {
		t.Fatalf("immutable update=%d %s", response.Code, response.Body.String())
	}
}

func TestEmptyChapterTrashEndpointIsStaticIdempotentAndUsesPublicFields(t *testing.T) {
	e, projectSummary := storyAPIHarness(t)
	base := "/api/v1/projects/" + projectSummary.UUID
	created := requestJSON(t, e, http.MethodPost, base+"/chapters", map[string]any{"chapter_code": "vol01.ch90", "title": "Trash", "content": "", "content_format": "txt"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create chapter = %d %s", created.Code, created.Body.String())
	}
	chapter := envelopeData(t, created)
	trashed := requestJSON(t, e, http.MethodDelete, base+"/chapters/"+chapter["uuid"].(string)+"?expected_revision=0", nil)
	if trashed.Code != http.StatusOK {
		t.Fatalf("trash chapter = %d %s", trashed.Code, trashed.Body.String())
	}
	emptied := requestJSON(t, e, http.MethodDelete, base+"/chapters/trash", nil)
	if emptied.Code != http.StatusOK || !strings.Contains(emptied.Body.String(), `"deleted_count":1`) || !strings.Contains(emptied.Body.String(), `"blocked_items":[]`) || strings.Contains(emptied.Body.String(), `"id":`) {
		t.Fatalf("empty chapter trash = %d %s", emptied.Code, emptied.Body.String())
	}
	repeated := requestJSON(t, e, http.MethodDelete, base+"/chapters/trash", nil)
	if repeated.Code != http.StatusOK || !strings.Contains(repeated.Body.String(), `"deleted_count":0`) || !strings.Contains(repeated.Body.String(), `"blocked_items":[]`) {
		t.Fatalf("repeat empty chapter trash = %d %s", repeated.Code, repeated.Body.String())
	}
}

func TestPromptCatalogEndpointReturnsCanonicalDefaultsWithoutInternalIDs(t *testing.T) {
	e, projectSummary := storyAPIHarness(t)
	response := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+projectSummary.UUID+"/prompts?prompt_group=chapter", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"prompt_key":"comic_storyboard"`) || !strings.Contains(response.Body.String(), `"prompt_key":"section_image"`) || !strings.Contains(response.Body.String(), `"prompt_key":"section_reference_present"`) || !strings.Contains(response.Body.String(), `"prompt_type":"fragment"`) || !strings.Contains(response.Body.String(), `"default_value":`) {
		t.Fatalf("catalog response = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"id":`) {
		t.Fatalf("catalog leaked internal id: %s", response.Body.String())
	}
}

func TestPromptGroupEndpointSavesAtomicallyAndRestoresDefault(t *testing.T) {
	e, projectSummary := storyAPIHarness(t)
	base := "/api/v1/projects/" + projectSummary.UUID
	updated := requestJSON(t, e, http.MethodPatch, base+"/prompt-groups/story", map[string]any{
		"prompts": map[string]string{
			"json_system":   "Custom JSON contract",
			"story_profile": "Custom profile prompt",
		},
		"expected_current_versions": map[string]int{"json_system": 1, "story_profile": 1},
	})
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"prompt":"Custom JSON contract"`) || !strings.Contains(updated.Body.String(), `"version_no":2`) {
		t.Fatalf("group update = %d %s", updated.Code, updated.Body.String())
	}
	conflict := requestJSON(t, e, http.MethodPatch, base+"/prompt-groups/story", map[string]any{
		"prompts": map[string]string{
			"json_system":   "Must roll back",
			"story_profile": "Stale profile",
		},
		"expected_current_versions": map[string]int{"json_system": 2, "story_profile": 1},
	})
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"code":"prompt_revision_conflict"`) {
		t.Fatalf("group conflict = %d %s", conflict.Code, conflict.Body.String())
	}
	history := requestJSON(t, e, http.MethodGet, base+"/prompt-versions?prompt_group=story&prompt_key=json_system&page=1&per_page=20", nil)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"total":2`) || strings.Contains(history.Body.String(), "Must roll back") {
		t.Fatalf("atomic history = %d %s", history.Code, history.Body.String())
	}
	definition, _ := promptcatalog.LookupForPictureBook(promptcatalog.GroupStory, "json_system", promptcatalog.LanguageChinese, promptcatalog.PictureBookOptions{
		Format:       project.PictureBookClassic,
		AspectWidth:  4,
		AspectHeight: 3,
	})
	restored := requestJSON(t, e, http.MethodPatch, base+"/prompt-groups/story", map[string]any{
		"prompts":                   map[string]string{"json_system": definition.DefaultValue},
		"expected_current_versions": map[string]int{"json_system": 2},
	})
	if restored.Code != http.StatusOK || !strings.Contains(restored.Body.String(), `"source_type":"default_restore"`) || !strings.Contains(restored.Body.String(), `"is_custom":false`) {
		t.Fatalf("default restore = %d %s", restored.Code, restored.Body.String())
	}
	invalid := requestJSON(t, e, http.MethodPatch, base+"/prompt-groups/story", map[string]any{
		"prompts":                   map[string]string{"json_system": "Unknown {{not_allowed}}"},
		"expected_current_versions": map[string]int{"json_system": 3},
	})
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"code":"validation_failed"`) {
		t.Fatalf("invalid placeholder = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestProjectGenerationLanguageUsesRevisionedProjectResource(t *testing.T) {
	e, projectSummary := storyAPIHarness(t)
	base := "/api/v1/projects/" + projectSummary.UUID
	shown := requestJSON(t, e, http.MethodGet, base, nil)
	if shown.Code != http.StatusOK || !strings.Contains(shown.Body.String(), `"generation_language":"zh-Hans"`) {
		t.Fatalf("show project = %d %s", shown.Code, shown.Body.String())
	}
	updated := requestJSON(t, e, http.MethodPatch, base, map[string]any{"name": "API Story", "description": "English output", "generation_language": "en", "expected_revision": 1})
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"generation_language":"en"`) || !strings.Contains(updated.Body.String(), `"revision":2`) {
		t.Fatalf("update language = %d %s", updated.Code, updated.Body.String())
	}
	invalid := requestJSON(t, e, http.MethodPatch, base, map[string]any{"name": "API Story", "description": "Invalid", "generation_language": "fr", "expected_revision": 2})
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"code":"validation_failed"`) {
		t.Fatalf("invalid language = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestStoryHandlersUseUUIDEnvelopeAndRevisionConflict(t *testing.T) {
	e, projectSummary := storyAPIHarness(t)
	base := "/api/v1/projects/" + projectSummary.UUID
	createdResponse := requestJSON(t, e, http.MethodPost, base+"/chapters", map[string]any{"chapter_code": "vol01.ch01", "title": "First", "content": "Version one", "content_format": "md"})
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createdResponse.Code, createdResponse.Body.String())
	}
	created := envelopeData(t, createdResponse)
	chapterUUID := created["uuid"].(string)
	if strings.Contains(createdResponse.Body.String(), `"id":`) {
		t.Fatalf("chapter response leaked internal id: %s", createdResponse.Body.String())
	}
	updated := requestJSON(t, e, http.MethodPut, base+"/chapters/"+chapterUUID+"/current-story", map[string]any{"content": "Version two", "content_format": "md", "expected_revision": 1})
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"version_no":2`) {
		t.Fatalf("update status = %d, body = %s", updated.Code, updated.Body.String())
	}
	stale := requestJSON(t, e, http.MethodPut, base+"/chapters/"+chapterUUID+"/current-story", map[string]any{"content": "Stale", "content_format": "md", "expected_revision": 1})
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `"code":"chapter_revision_conflict"`) {
		t.Fatalf("stale status = %d, body = %s", stale.Code, stale.Body.String())
	}
	list := requestJSON(t, e, http.MethodGet, base+"/chapters", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"data":{"items":[`) {
		t.Fatalf("list status = %d, body = %s", list.Code, list.Body.String())
	}
	invalidProject := requestJSON(t, e, http.MethodGet, "/api/v1/projects/not-a-uuid/chapters", nil)
	if invalidProject.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidProject.Body.String(), `"code":"invalid_uuid"`) {
		t.Fatalf("invalid project status = %d, body = %s", invalidProject.Code, invalidProject.Body.String())
	}
}

func TestPromptPaginationAndRestoreReturnNewResource(t *testing.T) {
	e, projectSummary := storyAPIHarness(t)
	base := "/api/v1/projects/" + projectSummary.UUID
	firstResponse := requestJSON(t, e, http.MethodPost, base+"/prompt-versions", map[string]any{"prompt_group": "story", "prompt_key": "outline", "prompt": "First", "expected_current_version": 0})
	first := envelopeData(t, firstResponse)
	requestJSON(t, e, http.MethodPost, base+"/prompt-versions", map[string]any{"prompt_group": "story", "prompt_key": "outline", "prompt": "Second", "expected_current_version": 1})
	restoredResponse := requestJSON(t, e, http.MethodPost, base+"/prompt-versions/"+first["uuid"].(string)+"/restorations", map[string]any{"expected_current_version": 2})
	if restoredResponse.Code != http.StatusCreated || !strings.Contains(restoredResponse.Body.String(), `"version_no":3`) || !strings.Contains(restoredResponse.Body.String(), `"source_type":"version_restore"`) {
		t.Fatalf("restore status = %d, body = %s", restoredResponse.Code, restoredResponse.Body.String())
	}
	list := requestJSON(t, e, http.MethodGet, base+"/prompt-versions?prompt_group=story&prompt_key=outline&page=1&per_page=2", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"pagination":{"per_page":2,"current_page":1,"last_page":2,"total":3}`) {
		t.Fatalf("prompt list status = %d, body = %s", list.Code, list.Body.String())
	}
}

func TestStoryProfileExternalConflictAndMultipartImport(t *testing.T) {
	e, projectSummary := storyAPIHarness(t)
	base := "/api/v1/projects/" + projectSummary.UUID
	profileResponse := requestJSON(t, e, http.MethodGet, base+"/story-profile", nil)
	profile := envelopeData(t, profileResponse)
	if err := os.WriteFile(filepath.Join(projectSummary.RootPath, "STORY.md"), []byte("# External\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	conflict := requestJSON(t, e, http.MethodGet, base+"/story-profile", nil)
	if conflict.Code != http.StatusOK || !strings.Contains(conflict.Body.String(), `"projection_state":"conflict"`) {
		t.Fatalf("conflict status = %d, body = %s", conflict.Code, conflict.Body.String())
	}
	imported := requestJSON(t, e, http.MethodPost, base+"/story-profile/imports", map[string]any{"expected_revision": int64(profile["revision"].(float64))})
	if imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), `"source_type":"external_import"`) {
		t.Fatalf("external import status = %d, body = %s", imported.Code, imported.Body.String())
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("mode", "single")
	_ = writer.WriteField("chapter_code", "vol01.ch05")
	_ = writer.WriteField("title", "Imported")
	part, err := writer.CreateFormFile("file", "chapter.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("Imported body"))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, base+"/chapter-imports", &body)
	request.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), `"chapter_code":"vol01.ch05"`) || strings.Contains(recorder.Body.String(), `"id":`) {
		t.Fatalf("multipart import status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var envelope Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil || !envelope.Success {
		t.Fatalf("multipart envelope = %+v, error = %v", envelope, err)
	}
}
