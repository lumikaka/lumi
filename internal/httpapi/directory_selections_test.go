package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"lumi/internal/directorypicker"

	"github.com/labstack/echo/v4"
)

type directoryPickerFake struct {
	selected string
	err      error
	initial  string
}

func (picker *directoryPickerFake) Pick(_ context.Context, initialPath string) (string, error) {
	picker.initial = initialPath
	return picker.selected, picker.err
}

func TestDirectorySelectionHandlerReturnsSelectedPath(t *testing.T) {
	picker := &directoryPickerFake{selected: "/books/moon"}
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/directory-selections", NewDirectorySelectionHandler(picker).Create)

	response := requestJSON(t, e, http.MethodPost, "/api/v1/directory-selections", map[string]any{"initial_path": "/books"})
	if response.Code != http.StatusCreated {
		t.Fatalf("selection status = %d, body = %s", response.Code, response.Body.String())
	}
	if picker.initial != "/books" {
		t.Fatalf("initial path = %q", picker.initial)
	}
	data := envelopeData(t, response)
	if data["root_path"] != "/books/moon" {
		t.Fatalf("selection data = %+v", data)
	}
}

func TestDirectorySelectionHandlerTreatsCancellationAsSuccess(t *testing.T) {
	picker := &directoryPickerFake{err: directorypicker.ErrCancelled}
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/directory-selections", NewDirectorySelectionHandler(picker).Create)

	response := requestJSON(t, e, http.MethodPost, "/api/v1/directory-selections", map[string]any{"initial_path": ""})
	if response.Code != http.StatusOK || response.Body.String() != "{\"success\":true,\"data\":null}\n" {
		t.Fatalf("cancel response = %d, %s", response.Code, response.Body.String())
	}
}

func TestDirectorySelectionHandlerMapsPickerErrors(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "busy", err: directorypicker.ErrBusy, status: http.StatusConflict, code: "directory_picker_busy"},
		{name: "unavailable", err: directorypicker.ErrUnavailable, status: http.StatusNotImplemented, code: "directory_picker_unavailable"},
		{name: "failed", err: errors.New("boom"), status: http.StatusInternalServerError, code: "directory_picker_failed"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			e := echo.New()
			e.HTTPErrorHandler = ErrorHandler
			e.POST("/api/v1/directory-selections", NewDirectorySelectionHandler(&directoryPickerFake{err: scenario.err}).Create)
			response := requestJSON(t, e, http.MethodPost, "/api/v1/directory-selections", map[string]any{"initial_path": ""})
			if response.Code != scenario.status || !strings.Contains(response.Body.String(), `"code":"`+scenario.code+`"`) {
				t.Fatalf("response = %d, %s", response.Code, response.Body.String())
			}
		})
	}
}
