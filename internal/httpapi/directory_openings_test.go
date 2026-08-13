package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"lumi/internal/directoryopener"

	"github.com/labstack/echo/v4"
)

type directoryOpenerFake struct {
	rootPath string
	err      error
}

func (opener *directoryOpenerFake) Open(_ context.Context, rootPath string) error {
	opener.rootPath = rootPath
	return opener.err
}

func TestDirectoryOpeningHandlerOpensTheRequestedPath(t *testing.T) {
	opener := &directoryOpenerFake{}
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/directory-openings", NewDirectoryOpeningHandler(opener).Create)

	response := requestJSON(t, e, http.MethodPost, "/api/v1/directory-openings", map[string]any{"root_path": "/books/moon"})
	if response.Code != http.StatusCreated || response.Body.String() != "{\"success\":true,\"data\":null}\n" {
		t.Fatalf("opening response = %d, %s", response.Code, response.Body.String())
	}
	if opener.rootPath != "/books/moon" {
		t.Fatalf("opened path = %q", opener.rootPath)
	}
}

func TestDirectoryOpeningHandlerMapsErrors(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "not found", err: directoryopener.ErrNotFound, status: http.StatusNotFound, code: "directory_not_found"},
		{name: "not a folder", err: directoryopener.ErrNotDirectory, status: http.StatusUnprocessableEntity, code: "directory_not_a_folder"},
		{name: "unavailable", err: directoryopener.ErrUnavailable, status: http.StatusNotImplemented, code: "directory_opener_unavailable"},
		{name: "failed", err: errors.New("boom"), status: http.StatusInternalServerError, code: "directory_open_failed"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			e := echo.New()
			e.HTTPErrorHandler = ErrorHandler
			e.POST("/api/v1/directory-openings", NewDirectoryOpeningHandler(&directoryOpenerFake{err: scenario.err}).Create)
			response := requestJSON(t, e, http.MethodPost, "/api/v1/directory-openings", map[string]any{"root_path": "/books/moon"})
			if response.Code != scenario.status || !strings.Contains(response.Body.String(), `"code":"`+scenario.code+`"`) {
				t.Fatalf("response = %d, %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestDirectoryOpeningHandlerRejectsAnEmptyPath(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	e.POST("/api/v1/directory-openings", NewDirectoryOpeningHandler(&directoryOpenerFake{}).Create)
	response := requestJSON(t, e, http.MethodPost, "/api/v1/directory-openings", map[string]any{"root_path": "  "})
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"code":"validation_failed"`) {
		t.Fatalf("response = %d, %s", response.Code, response.Body.String())
	}
}
