package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
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
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"

	"github.com/labstack/echo/v4"
)

func productionAPIHarness(t *testing.T) (*echo.Echo, *project.Manager, string, story.Chapter) {
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
	e.PATCH("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid", handler.UpdateSection)
	e.DELETE("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid", handler.DeleteSection)
	e.POST("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/restorations", handler.RestoreSection)
	e.PUT("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-section-order", handler.ReorderSections)
	e.PUT("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/premise-assets", handler.SetSectionPremiseAssets)
	e.GET("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic", handler.ShowComicState)
	e.GET("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-snapshots", handler.ListSnapshots)
	e.GET("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-snapshots/:snapshot_uuid", handler.ShowSnapshot)
	e.GET("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-sections", handler.ListSections)
	e.POST("/api/v1/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/storyboard-variants", handler.CreateStoryboard)
	e.GET("/api/v1/projects/:project_uuid/comic-exports/readiness", handler.ExportReadiness)
	e.GET("/api/v1/projects/:project_uuid/comic-exports", handler.ListExports)
	e.GET("/media/projects/:project_uuid/comic-exports/:export_uuid/content", handler.ExportContent)
	e.DELETE("/api/v1/projects/:project_uuid/premise-assets/trash", handler.EmptyPremiseAssetTrash)
	e.DELETE("/api/v1/projects/:project_uuid/premise-assets/:premise_asset_uuid/permanent", handler.PermanentlyDeletePremiseAsset)
	t.Cleanup(func() { _ = manager.Close(); _ = app.Close() })
	return e, manager, created.UUID, chapter
}

func TestProductionAPIComicSectionPremiseAssetSelectionContract(t *testing.T) {
	e, manager, projectUUID, chapter := productionAPIHarness(t)
	base := "/api/v1/projects/" + projectUUID + "/chapters/" + chapter.UUID
	sectionResponse := requestJSON(t, e, http.MethodPost, base+"/comic-sections", map[string]any{
		"title": "Reference page", "storyboard_md": "Fox enters the forest", "page_role": production.PageRoleBody,
	})
	if sectionResponse.Code != http.StatusCreated {
		t.Fatalf("section=%d %s", sectionResponse.Code, sectionResponse.Body.String())
	}
	section := envelopeData(t, sectionResponse)
	assets := make([]production.PremiseAsset, 0, 2)
	if err := manager.WithCurrentStore(t.Context(), projectUUID, func(store *project.Store) error {
		service := production.NewService(store, nil)
		for index, assetType := range []string{production.AssetCharacter, production.AssetScene} {
			upload, err := service.Files().CreateUpload(t.Context(), files.CreateUploadInput{
				Purpose: "premise_asset", OriginalFilename: fmt.Sprintf("reference-%d.png", index+1), Reader: bytes.NewReader(productionAPIImage(t, uint8(80+index))),
			})
			if err != nil {
				return err
			}
			asset, err := service.ImportPremiseAsset(t.Context(), production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: assetType, Title: fmt.Sprintf("Reference %d", index+1)})
			if err != nil {
				return err
			}
			assets = append(assets, asset)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	path := base + "/comic-sections/" + section["uuid"].(string) + "/premise-assets"
	selectedResponse := requestJSON(t, e, http.MethodPut, path, map[string]any{
		"premise_asset_uuids": []string{assets[1].UUID, assets[0].UUID}, "expected_revision": section["revision"],
	})
	if selectedResponse.Code != http.StatusOK || strings.Contains(selectedResponse.Body.String(), `"id":`) {
		t.Fatalf("selection=%d %s", selectedResponse.Code, selectedResponse.Body.String())
	}
	selected := envelopeData(t, selectedResponse)
	references := selected["premise_assets"].([]any)
	if len(references) != 2 || references[0].(map[string]any)["asset_uuid"] != assets[1].UUID || references[1].(map[string]any)["asset_uuid"] != assets[0].UUID {
		t.Fatalf("ordered references=%#v", references)
	}
	for _, reference := range references {
		item := reference.(map[string]any)
		if !productionUUIDv7(item["asset_uuid"]) || !productionUUIDv7(item["variant_uuid"]) || !productionUUIDv7(item["file_uuid"]) {
			t.Fatalf("non-public reference=%#v", item)
		}
	}
	stale := requestJSON(t, e, http.MethodPut, path, map[string]any{
		"premise_asset_uuids": []string{assets[0].UUID}, "expected_revision": section["revision"],
	})
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `"code":"production_conflict"`) {
		t.Fatalf("stale selection=%d %s", stale.Code, stale.Body.String())
	}
	duplicate := requestJSON(t, e, http.MethodPut, path, map[string]any{
		"premise_asset_uuids": []string{assets[0].UUID, assets[0].UUID}, "expected_revision": selected["revision"],
	})
	if duplicate.Code != http.StatusUnprocessableEntity || !strings.Contains(duplicate.Body.String(), `"code":"production_validation_failed"`) {
		t.Fatalf("duplicate selection=%d %s", duplicate.Code, duplicate.Body.String())
	}
	listResponse := requestJSON(t, e, http.MethodGet, base+"/comic-sections", nil)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"asset_uuid":"`+assets[1].UUID+`"`) || strings.Contains(listResponse.Body.String(), `"id":`) {
		t.Fatalf("persisted selection=%d %s", listResponse.Code, listResponse.Body.String())
	}
}

func productionAPIImage(t *testing.T, seed uint8) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			value.Set(x, y, color.RGBA{R: seed + uint8(x), G: uint8(y * 12), B: 130, A: 255})
		}
	}
	var content bytes.Buffer
	if err := png.Encode(&content, value); err != nil {
		t.Fatal(err)
	}
	return content.Bytes()
}

func TestProductionAPIUsesEnvelopesAndPublicUUIDs(t *testing.T) {
	e, _, projectUUID, chapter := productionAPIHarness(t)
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
	if !productionUUIDv7(section["uuid"]) || section["chapter_uuid"] != chapter.UUID || section["page_role"] != production.PageRoleBody {
		t.Fatalf("section=%#v", section)
	}
	snapshotsResponse := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+projectUUID+"/chapters/"+chapter.UUID+"/comic-snapshots", nil)
	if snapshotsResponse.Code != http.StatusOK || !strings.Contains(snapshotsResponse.Body.String(), `"section_count":1`) || strings.Contains(snapshotsResponse.Body.String(), `"snapshot":`) || strings.Contains(snapshotsResponse.Body.String(), `"snapshot_hash"`) || strings.Contains(snapshotsResponse.Body.String(), `"id":`) {
		t.Fatalf("snapshot summaries status=%d body=%s", snapshotsResponse.Code, snapshotsResponse.Body.String())
	}
	snapshotItems := envelopeData(t, snapshotsResponse)["items"].([]any)
	snapshotUUID := snapshotItems[0].(map[string]any)["uuid"].(string)
	detailResponse := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+projectUUID+"/chapters/"+chapter.UUID+"/comic-snapshots/"+snapshotUUID, nil)
	if detailResponse.Code != http.StatusOK || !strings.Contains(detailResponse.Body.String(), `"chapter_code":"vol01.ch01"`) || !strings.Contains(detailResponse.Body.String(), `"storyboard_md":"# Wide shot"`) || !strings.Contains(detailResponse.Body.String(), `"page_role":"body"`) || strings.Contains(detailResponse.Body.String(), `"snapshot_json"`) || strings.Contains(detailResponse.Body.String(), `"key_path"`) || strings.Contains(detailResponse.Body.String(), `"id":`) {
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

func TestProductionAPIComicSectionPageRoleContract(t *testing.T) {
	e, _, projectUUID, chapter := productionAPIHarness(t)
	base := "/api/v1/projects/" + projectUUID + "/chapters/" + chapter.UUID
	emptyFrontResponse := requestJSON(t, e, http.MethodPost, base+"/comic-sections", map[string]any{
		"title": "Front", "storyboard_md": "Front board", "page_role": production.PageRoleFrontCover,
	})
	if emptyFrontResponse.Code != http.StatusUnprocessableEntity || !strings.Contains(emptyFrontResponse.Body.String(), `"code":"production_validation_failed"`) {
		t.Fatalf("empty front status=%d body=%s", emptyFrontResponse.Code, emptyFrontResponse.Body.String())
	}
	bodyResponse := requestJSON(t, e, http.MethodPost, base+"/comic-sections", map[string]any{
		"title": "Body", "storyboard_md": "Body board",
	})
	if bodyResponse.Code != http.StatusCreated {
		t.Fatalf("body status=%d body=%s", bodyResponse.Code, bodyResponse.Body.String())
	}
	body := envelopeData(t, bodyResponse)
	if body["page_role"] != production.PageRoleBody || body["section_no"] != float64(1) {
		t.Fatalf("body=%#v", body)
	}
	frontResponse := requestJSON(t, e, http.MethodPost, base+"/comic-sections", map[string]any{
		"title": "Front", "storyboard_md": "Front board", "page_role": production.PageRoleFrontCover,
	})
	if frontResponse.Code != http.StatusCreated {
		t.Fatalf("front status=%d body=%s", frontResponse.Code, frontResponse.Body.String())
	}
	front := envelopeData(t, frontResponse)
	if front["page_role"] != production.PageRoleFrontCover || front["section_no"] != float64(1) {
		t.Fatalf("front=%#v", front)
	}
	backResponse := requestJSON(t, e, http.MethodPatch, base+"/comic-sections/"+front["uuid"].(string), map[string]any{
		"page_role": production.PageRoleBackCover, "expected_revision": front["revision"],
	})
	if backResponse.Code != http.StatusOK {
		t.Fatalf("back update status=%d body=%s", backResponse.Code, backResponse.Body.String())
	}
	back := envelopeData(t, backResponse)
	if back["page_role"] != production.PageRoleBackCover || back["section_no"] != float64(2) {
		t.Fatalf("back=%#v", back)
	}
	reorderResponse := requestJSON(t, e, http.MethodPut, base+"/comic-section-order", map[string]any{
		"section_uuids": []string{body["uuid"].(string)},
	})
	if reorderResponse.Code != http.StatusOK || !strings.Contains(reorderResponse.Body.String(), `"page_role":"body"`) || !strings.Contains(reorderResponse.Body.String(), `"page_role":"back_cover"`) {
		t.Fatalf("reorder status=%d body=%s", reorderResponse.Code, reorderResponse.Body.String())
	}
}

func TestProductionAPIComicSectionTrashAndRestoreContract(t *testing.T) {
	e, _, projectUUID, chapter := productionAPIHarness(t)
	base := "/api/v1/projects/" + projectUUID + "/chapters/" + chapter.UUID
	firstResponse := requestJSON(t, e, http.MethodPost, base+"/comic-sections", map[string]any{"title": "Recover me"})
	secondResponse := requestJSON(t, e, http.MethodPost, base+"/comic-sections", map[string]any{"title": "Keep me"})
	if firstResponse.Code != http.StatusCreated || secondResponse.Code != http.StatusCreated {
		t.Fatalf("create statuses=%d/%d first=%s second=%s", firstResponse.Code, secondResponse.Code, firstResponse.Body.String(), secondResponse.Body.String())
	}
	first := envelopeData(t, firstResponse)
	sectionPath := base + "/comic-sections/" + first["uuid"].(string)
	deleted := requestJSON(t, e, http.MethodDelete, sectionPath+"?expected_revision="+fmt.Sprint(first["revision"]), nil)
	if deleted.Code != http.StatusOK || !strings.Contains(deleted.Body.String(), `"data":null`) {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	active := requestJSON(t, e, http.MethodGet, base+"/comic-sections", nil)
	if active.Code != http.StatusOK || strings.Contains(active.Body.String(), first["uuid"].(string)) {
		t.Fatalf("active status=%d body=%s", active.Code, active.Body.String())
	}
	trash := requestJSON(t, e, http.MethodGet, base+"/comic-sections?state=trashed", nil)
	if trash.Code != http.StatusOK || !strings.Contains(trash.Body.String(), `"deleted_at":`) || strings.Contains(trash.Body.String(), `"id":`) {
		t.Fatalf("trash status=%d body=%s", trash.Code, trash.Body.String())
	}
	trashItems := envelopeData(t, trash)["items"].([]any)
	trashedPage := trashItems[0].(map[string]any)
	missingRevision := requestJSON(t, e, http.MethodPost, sectionPath+"/restorations", map[string]any{})
	if missingRevision.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing revision status=%d body=%s", missingRevision.Code, missingRevision.Body.String())
	}
	restoredResponse := requestJSON(t, e, http.MethodPost, sectionPath+"/restorations", map[string]any{"expected_revision": trashedPage["revision"]})
	if restoredResponse.Code != http.StatusOK || strings.Contains(restoredResponse.Body.String(), `"deleted_at":`) || strings.Contains(restoredResponse.Body.String(), `"id":`) {
		t.Fatalf("restore status=%d body=%s", restoredResponse.Code, restoredResponse.Body.String())
	}
	restored := envelopeData(t, restoredResponse)
	if restored["uuid"] != first["uuid"] || restored["revision"].(float64) != trashedPage["revision"].(float64)+1 {
		t.Fatalf("restored=%#v trash=%#v", restored, trashedPage)
	}
	emptyTrash := requestJSON(t, e, http.MethodGet, base+"/comic-sections?state=trashed", nil)
	if emptyTrash.Code != http.StatusOK || !strings.Contains(emptyTrash.Body.String(), `"items":[]`) {
		t.Fatalf("empty trash status=%d body=%s", emptyTrash.Code, emptyTrash.Body.String())
	}
	invalidState := requestJSON(t, e, http.MethodGet, base+"/comic-sections?state=unknown", nil)
	if invalidState.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidState.Body.String(), `"code":"production_validation_failed"`) {
		t.Fatalf("invalid state status=%d body=%s", invalidState.Code, invalidState.Body.String())
	}
}

func TestComicExportContentSupportsRangeETagAndExpiry(t *testing.T) {
	e, manager, projectUUID, _ := productionAPIHarness(t)
	const (
		exportUUID   = "01900000-0000-7000-8000-000000000401"
		taskUUID     = "01900000-0000-7000-8000-000000000402"
		snapshotHash = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	)
	content := []byte("0123456789")
	digest := sha256.Sum256(content)
	expiresAt := time.Now().UTC().Add(time.Hour)
	if err := manager.WithCurrentStore(t.Context(), projectUUID, func(store *project.Store) error {
		relative := production.ExportRelativePath(exportUUID, "project", "", snapshotHash, production.ExportSnapshot{})
		target, err := store.ResolvePath(relative)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return err
		}
		return store.DB().Exec(`INSERT INTO comic_exports(uuid,project_id,task_uuid,scope,format,status,snapshot_json,snapshot_hash,relative_path,retention_days,expires_at,byte_size,content_sha256,created_at,completed_at) VALUES(?,(SELECT id FROM projects WHERE uuid=?),?,'project','zip','ready','{}',?,?,7,?,?,?,datetime('now'),datetime('now'))`, exportUUID, projectUUID, taskUUID, snapshotHash, relative, expiresAt, len(content), fmt.Sprintf("%x", digest[:])).Error
	}); err != nil {
		t.Fatal(err)
	}

	path := "/media/projects/" + projectUUID + "/comic-exports/" + exportUUID + "/content"
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Range", "bytes=2-5")
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusPartialContent || recorder.Body.String() != "2345" {
		t.Fatalf("range status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("ETag") != `"sha256-`+fmt.Sprintf("%x", digest[:])+`"` || recorder.Header().Get("Accept-Ranges") != "bytes" || !strings.Contains(recorder.Header().Get("Content-Disposition"), "filename*=UTF-8''") {
		t.Fatalf("download headers=%v", recorder.Header())
	}

	if err := manager.WithCurrentStore(t.Context(), projectUUID, func(store *project.Store) error {
		return store.DB().Exec(`UPDATE comic_exports SET expires_at=? WHERE uuid=?`, time.Now().UTC().Add(-time.Second), exportUUID).Error
	}); err != nil {
		t.Fatal(err)
	}
	expired := requestJSON(t, e, http.MethodGet, path, nil)
	if expired.Code != http.StatusGone || !strings.Contains(expired.Body.String(), `"code":"comic_export_expired"`) {
		t.Fatalf("expired status=%d body=%s", expired.Code, expired.Body.String())
	}
	if err := manager.WithCurrentStore(t.Context(), projectUUID, func(store *project.Store) error {
		return store.DB().Exec(`DELETE FROM comic_exports WHERE uuid=?`, exportUUID).Error
	}); err != nil {
		t.Fatal(err)
	}
	missing := requestJSON(t, e, http.MethodGet, path, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func TestComicPDFExportContentUsesPDFHeaders(t *testing.T) {
	e, manager, projectUUID, _ := productionAPIHarness(t)
	const (
		exportUUID   = "01900000-0000-7000-8000-000000000411"
		taskUUID     = "01900000-0000-7000-8000-000000000412"
		snapshotHash = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	)
	content := []byte("%PDF-1.7\n%%EOF\n")
	digest := sha256.Sum256(content)
	expiresAt := time.Now().UTC().Add(time.Hour)
	snapshot := production.ExportSnapshot{Version: 5, Format: production.ExportFormatPDF, ProjectUUID: projectUUID, ProjectTitle: "Lumi PDF", Scope: "project"}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.WithCurrentStore(t.Context(), projectUUID, func(store *project.Store) error {
		relative := production.ExportRelativePath(exportUUID, "project", "", snapshotHash, snapshot)
		target, err := store.ResolvePath(relative)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return err
		}
		return store.DB().Exec(`INSERT INTO comic_exports(uuid,project_id,task_uuid,scope,format,status,snapshot_json,snapshot_hash,relative_path,retention_days,expires_at,byte_size,content_sha256,created_at,completed_at) VALUES(?,(SELECT id FROM projects WHERE uuid=?),?,'project','pdf','ready',?,?,?,7,?,?,?,datetime('now'),datetime('now'))`, exportUUID, projectUUID, taskUUID, string(snapshotJSON), snapshotHash, relative, expiresAt, len(content), fmt.Sprintf("%x", digest[:])).Error
	}); err != nil {
		t.Fatal(err)
	}

	path := "/media/projects/" + projectUUID + "/comic-exports/" + exportUUID + "/content"
	recorder := requestJSON(t, e, http.MethodGet, path, nil)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "application/pdf" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(recorder.Header().Get("Content-Disposition"), "comic-export.pdf") || !strings.Contains(recorder.Header().Get("Content-Disposition"), "filename*=UTF-8''Lumi%20PDF.pdf") || recorder.Body.String() != string(content) {
		t.Fatalf("pdf status=%d headers=%v body=%q", recorder.Code, recorder.Header(), recorder.Body.String())
	}
}

func productionUUIDv7(value any) bool {
	text, ok := value.(string)
	return ok && len(text) == 36 && text[14] == '7'
}
