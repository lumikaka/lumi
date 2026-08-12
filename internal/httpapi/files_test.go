package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"

	"github.com/labstack/echo/v4"
)

func assetAPIHarness(t *testing.T) (*echo.Echo, string) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(app)
	created, err := manager.Create(t.Context(), "Assets API", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewFilesHandler(manager, nil)
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/projects/:project_uuid/asset-uploads", handler.CreateUpload)
	e.GET("/api/v1/projects/:project_uuid/asset-uploads/:upload_uuid", handler.ShowUpload)
	e.POST("/api/v1/projects/:project_uuid/asset-uploads/:upload_uuid/completions", handler.FinalizeUpload)
	e.GET("/api/v1/projects/:project_uuid/assets", handler.ListAssets)
	e.GET("/api/v1/projects/:project_uuid/assets/:asset_uuid", handler.ShowAsset)
	e.GET("/media/projects/:project_uuid/assets/:asset_uuid/content", handler.Content)
	t.Cleanup(func() { _ = manager.Close(); _ = app.Close() })
	return e, created.UUID
}

func apiPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 80, G: uint8(x * 10), B: uint8(y * 20), A: 255})
		}
	}
	var content bytes.Buffer
	if err := png.Encode(&content, img); err != nil {
		t.Fatal(err)
	}
	return content.Bytes()
}

func TestAssetUploadAPIUsesUUIDEnvelopeAndMediaRange(t *testing.T) {
	e, projectUUID := assetAPIHarness(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("purpose", "premise_asset"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("display_name", "Range image"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "range.fake")
	if err != nil {
		t.Fatal(err)
	}
	content := apiPNG(t)
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectUUID+"/asset-uploads", &body)
	request.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	uploadData := envelopeData(t, recorder)
	uploadUUID := uploadData["uuid"].(string)
	if uploadData["mime_type"] != "image/png" || uploadData["state"] != "ready" {
		t.Fatalf("upload=%#v", uploadData)
	}
	if strings.Contains(recorder.Body.String(), "file_object_id") || strings.Contains(recorder.Body.String(), "key_path") || strings.Contains(recorder.Body.String(), ".lumi") || strings.Contains(recorder.Body.String(), `"id"`) {
		t.Fatalf("upload leaked internal storage: %s", recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectUUID+"/asset-uploads/"+uploadUUID+"/completions", strings.NewReader(`{}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("purpose-less finalize status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	finalizeBody := strings.NewReader(`{"purpose":"premise_asset"}`)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectUUID+"/asset-uploads/"+uploadUUID+"/completions", finalizeBody)
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("finalize status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assetData := envelopeData(t, recorder)
	assetUUID := assetData["uuid"].(string)
	expectedURL := "/media/projects/" + projectUUID + "/assets/" + assetUUID + "/content"
	if assetData["content_url"] != expectedURL {
		t.Fatalf("content_url=%v", assetData["content_url"])
	}
	if strings.Contains(recorder.Body.String(), "file_object_id") || strings.Contains(recorder.Body.String(), "key_path") || strings.Contains(recorder.Body.String(), `"id"`) {
		t.Fatalf("asset leaked internal storage: %s", recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, expectedURL, nil)
	request.Header.Set("Range", "bytes=0-9")
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("range status=%d headers=%v", recorder.Code, recorder.Header())
	}
	if recorder.Body.Len() != 10 {
		t.Fatalf("range length=%d", recorder.Body.Len())
	}
	for _, header := range []string{"Content-Type", "Content-Length", "ETag", "Last-Modified", "X-Content-Type-Options", "Cache-Control", "Content-Disposition", "Accept-Ranges", "Content-Range"} {
		if recorder.Header().Get(header) == "" {
			t.Fatalf("missing %s", header)
		}
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("nosniff=%s", recorder.Header().Get("X-Content-Type-Options"))
	}
}

func TestAssetAPIRejectsInvalidUUIDWithoutPathLookup(t *testing.T) {
	e, projectUUID := assetAPIHarness(t)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectUUID+"/assets/../../etc/passwd", nil))
	if recorder.Code == http.StatusOK {
		t.Fatal("path traversal was accepted")
	}
	var envelope Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err == nil && envelope.Success {
		t.Fatalf("response=%s", recorder.Body.String())
	}
}
