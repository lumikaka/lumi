package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"lumi/internal/database"

	"github.com/labstack/echo/v4"
)

func TestHealthHandler(t *testing.T) {
	db, err := database.Open("file:" + filepath.Join(t.TempDir(), "health.sqlite3") + "?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.GET("/health", NewHealthHandler(db).Show)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	var response Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || !response.Success {
		t.Fatalf("status = %d, response = %+v", recorder.Code, response)
	}
}

func TestHealthHandlerReportsClosedDatabase(t *testing.T) {
	db, err := database.Open("file:" + filepath.Join(t.TempDir(), "closed.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.GET("/health", NewHealthHandler(db).Show)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
