package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/httpapi"
	"lumi/internal/project"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

func TestDesktopAuthenticationProtectsLocalServiceRoutes(t *testing.T) {
	const token = "desktop-runtime-token"
	e := echo.New()
	e.HTTPErrorHandler = httpapi.ErrorHandler
	e.Use(desktopAuthentication(config.NewDesktopAuthentication(token)))
	for _, path := range []string{"/api/v1/providers", "/api/v1/ws", "/api/v1/missing", "/media/projects/project/assets/asset/content"} {
		e.GET(path, func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	}
	e.GET("/api/v1/health", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.GET("/assets/app.js", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	for _, path := range []string{"/api/v1/providers", "/api/v1/ws", "/api/v1/missing", "/media/projects/project/assets/asset/content"} {
		recorder := serveDesktopRequest(e, http.MethodGet, path, "", nil)
		assertDesktopError(t, recorder, http.StatusUnauthorized, "desktop_authentication_required")
	}
	for _, path := range []string{"/api/v1/health", "/assets/app.js"} {
		recorder := serveDesktopRequest(e, http.MethodGet, path, "", nil)
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, body = %s", path, recorder.Code, recorder.Body.String())
		}
	}

	recorder := serveDesktopRequest(e, http.MethodGet, "/api/v1/providers", token, nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("authenticated status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	recorder = serveDesktopRequest(e, http.MethodOptions, "/api/v1/providers", "", nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDesktopSessionCreationSetsSessionCookie(t *testing.T) {
	const token = "desktop-runtime-token"
	e := echo.New()
	e.HTTPErrorHandler = httpapi.ErrorHandler
	e.POST("/api/v1/desktop-sessions", newDesktopSessionHandler(config.NewDesktopAuthentication(token)).Create)

	recorder := serveDesktopRequest(e, http.MethodPost, "/api/v1/desktop-sessions", "", strings.NewReader(`{"token":"`+token+`"}`))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var envelope httpapi.Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Success || envelope.Data != nil {
		t.Fatalf("response = %#v", envelope)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != desktopSessionCookie || cookie.Value != token || cookie.Path != "/" || !cookie.HttpOnly || cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != 0 {
		t.Fatalf("cookie = %#v", cookie)
	}
}

func TestDesktopSessionCreationRejectsInvalidInput(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = httpapi.ErrorHandler
	e.POST("/api/v1/desktop-sessions", newDesktopSessionHandler(config.NewDesktopAuthentication("expected-token")).Create)

	for _, body := range []string{"", `{}`, `{"token":"wrong-token"}`, `{"token":"expected-token"} trailing`} {
		recorder := serveDesktopRequest(e, http.MethodPost, "/api/v1/desktop-sessions", "", strings.NewReader(body))
		assertDesktopError(t, recorder, http.StatusUnauthorized, "desktop_authentication_failed")
		if len(recorder.Result().Cookies()) != 0 {
			t.Fatalf("invalid input %q set a cookie", body)
		}
	}
}

func TestDesktopAuthenticationRejectsPreviousRuntimeCookie(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = httpapi.ErrorHandler
	e.Use(desktopAuthentication(config.NewDesktopAuthentication("new-runtime-token")))
	e.GET("/api/v1/providers", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })

	recorder := serveDesktopRequest(e, http.MethodGet, "/api/v1/providers", "previous-runtime-token", nil)
	assertDesktopError(t, recorder, http.StatusUnauthorized, "desktop_authentication_required")
}

func TestDesktopAuthenticationDisabledPreservesExistingBehavior(t *testing.T) {
	e := echo.New()
	e.Use(desktopAuthentication(nil))
	e.GET("/api/v1/providers", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })

	recorder := serveDesktopRequest(e, http.MethodGet, "/api/v1/providers", "", nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDesktopAuthenticationIntegratesWithApplicationRoutes(t *testing.T) {
	const token = "desktop-runtime-token"
	const frontendURL = "http://localhost:5801"
	application := newDesktopIntegrationApplication(t, config.NewDesktopAuthentication(token), frontendURL)
	application.GET("/assets/app.js", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })

	for _, path := range []string{"/api/v1/providers", "/api/v1/missing", "/media/projects/project/assets/asset/content"} {
		recorder := serveDesktopRequest(application.Echo, http.MethodGet, path, "", nil)
		assertDesktopError(t, recorder, http.StatusUnauthorized, "desktop_authentication_required")
	}
	for _, path := range []string{"/api/v1/health", "/assets/app.js"} {
		recorder := serveDesktopRequest(application.Echo, http.MethodGet, path, "", nil)
		if recorder.Code < 200 || recorder.Code >= 300 {
			t.Fatalf("GET %s status = %d, body = %s", path, recorder.Code, recorder.Body.String())
		}
	}

	sessionRequest := httptest.NewRequest(http.MethodPost, "/api/v1/desktop-sessions", strings.NewReader(`{"token":"`+token+`"}`))
	sessionRequest.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	sessionRequest.Header.Set(echo.HeaderOrigin, frontendURL)
	sessionRecorder := httptest.NewRecorder()
	application.ServeHTTP(sessionRecorder, sessionRequest)
	if sessionRecorder.Code != http.StatusCreated {
		t.Fatalf("desktop session status = %d, body = %s", sessionRecorder.Code, sessionRecorder.Body.String())
	}
	cookies := sessionRecorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("desktop session cookies = %#v", cookies)
	}

	providersRequest := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	providersRequest.AddCookie(cookies[0])
	providersRecorder := httptest.NewRecorder()
	application.ServeHTTP(providersRecorder, providersRequest)
	if providersRecorder.Code != http.StatusOK {
		t.Fatalf("authenticated providers status = %d, body = %s", providersRecorder.Code, providersRecorder.Body.String())
	}

	writeRequest := httptest.NewRequest(http.MethodPost, "/api/v1/open-projects", strings.NewReader(`{"root_path":""}`))
	writeRequest.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	writeRequest.AddCookie(cookies[0])
	writeRecorder := httptest.NewRecorder()
	application.ServeHTTP(writeRecorder, writeRequest)
	if writeRecorder.Code != http.StatusForbidden {
		t.Fatalf("authenticated write without origin status = %d, body = %s", writeRecorder.Code, writeRecorder.Body.String())
	}

	httpServer := httptest.NewServer(application)
	t.Cleanup(httpServer.Close)
	websocketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/ws"
	headers := http.Header{echo.HeaderOrigin: []string{frontendURL}}
	connection, response, err := websocket.DefaultDialer.Dial(websocketURL, headers)
	if connection != nil {
		_ = connection.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated websocket err = %v, response = %#v", err, response)
	}
	_ = response.Body.Close()

	headers.Set(echo.HeaderCookie, cookies[0].Name+"="+cookies[0].Value)
	connection, response, err = websocket.DefaultDialer.Dial(websocketURL, headers)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
		}
		t.Fatalf("authenticated websocket failed: %v", err)
	}
	_ = connection.Close()
}

func TestDesktopSessionRouteIsAbsentWhenAuthenticationIsDisabled(t *testing.T) {
	const frontendURL = "http://localhost:5801"
	application := newDesktopIntegrationApplication(t, nil, frontendURL)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/desktop-sessions", strings.NewReader(`{"token":"anything"}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	request.Header.Set(echo.HeaderOrigin, frontendURL)
	recorder := httptest.NewRecorder()
	application.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	for _, route := range application.Routes() {
		if route.Method == http.MethodPost && route.Path == "/api/v1/desktop-sessions" {
			t.Fatal("desktop session route was registered without desktop authentication")
		}
	}
}

func newDesktopIntegrationApplication(t *testing.T, authentication *config.DesktopAuthentication, frontendURL string) *Application {
	t.Helper()
	dataDir := t.TempDir()
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := config.Config{
		Environment: "test", Address: ":0", FrontendURL: frontendURL,
		ViteDevServerURL: "http://127.0.0.1:5802", AppDataDir: dataDir,
		DatabaseDSN: config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")), DesktopAuth: authentication,
	}
	application, err := New(cfg, store, project.NewManager(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	return application
}

func serveDesktopRequest(e *echo.Echo, method, path, token string, body *strings.Reader) *httptest.ResponseRecorder {
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, body)
		request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	if token != "" {
		request.AddCookie(&http.Cookie{Name: desktopSessionCookie, Value: token})
	}
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	return recorder
}

func assertDesktopError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var envelope httpapi.Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Success || envelope.Data != nil || envelope.Error == nil || envelope.Error.Code != code {
		t.Fatalf("response = %#v", envelope)
	}
}
