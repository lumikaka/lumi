package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestSuccessEnvelope(t *testing.T) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), recorder)
	if err := Success(ctx, http.StatusOK, map[string]string{"status": "ok"}); err != nil {
		t.Fatal(err)
	}

	var response Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success || response.Data == nil || response.Error != nil {
		t.Fatalf("response = %+v", response)
	}
}

func TestErrorEnvelope(t *testing.T) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), recorder)
	ErrorHandler(NewError(http.StatusConflict, "conflict", "Conflict", "Already exists", errors.New("duplicate")), ctx)

	var response Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || response.Success || response.Data != nil || response.Error == nil || response.Error.Code != "conflict" {
		t.Fatalf("status = %d, response = %+v", recorder.Code, response)
	}
}

func TestEchoErrorsUseSnakeCaseCodes(t *testing.T) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/missing", nil), recorder)
	ErrorHandler(echo.ErrNotFound, ctx)

	var response Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != "not_found" || response.Data != nil {
		t.Fatalf("response = %+v", response)
	}
}
