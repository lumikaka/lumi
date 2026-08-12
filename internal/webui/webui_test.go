package webui

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"lumi/internal/httpapi"

	"github.com/labstack/echo/v4"
)

func TestEmbeddedFrontendRoutes(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":        {Data: []byte("<main>user app</main>")},
		"admin.html":        {Data: []byte("<main>admin app</main>")},
		"assets/app-123.js": {Data: []byte("console.log('app')")},
	}
	e := echo.New()
	e.HTTPErrorHandler = httpapi.ErrorHandler
	e.GET("/api/v1/known", func(c echo.Context) error { return c.String(http.StatusOK, "api") })
	if err := MountEmbedded(e, assets); err != nil {
		t.Fatal(err)
	}

	for _, scenario := range []struct {
		name, method, path, accept, body, cache string
		status                                  int
	}{
		{name: "user root", method: http.MethodGet, path: "/", accept: "text/html", body: "user app", status: http.StatusOK},
		{name: "user navigation", method: http.MethodGet, path: "/about", accept: "text/html", body: "user app", status: http.StatusOK},
		{name: "admin root", method: http.MethodGet, path: "/admin", accept: "text/html", body: "admin app", status: http.StatusOK},
		{name: "admin navigation", method: http.MethodGet, path: "/admin/settings", accept: "text/html", body: "admin app", status: http.StatusOK},
		{name: "asset", method: http.MethodGet, path: "/assets/app-123.js", body: "console.log", cache: "public, max-age=31536000, immutable", status: http.StatusOK},
		{name: "unknown api", method: http.MethodGet, path: "/api/v1/missing", accept: "text/html", status: http.StatusNotFound},
		{name: "missing asset", method: http.MethodGet, path: "/assets/missing.js", accept: "text/html", status: http.StatusNotFound},
		{name: "json navigation", method: http.MethodGet, path: "/about", accept: "application/json", status: http.StatusNotFound},
		{name: "non get", method: http.MethodPost, path: "/about", accept: "text/html", status: http.StatusMethodNotAllowed},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(scenario.method, scenario.path, nil)
			if scenario.accept != "" {
				request.Header.Set(echo.HeaderAccept, scenario.accept)
			}
			e.ServeHTTP(recorder, request)
			if recorder.Code != scenario.status || (scenario.body != "" && !strings.Contains(recorder.Body.String(), scenario.body)) {
				t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
			}
			if scenario.cache != "" && recorder.Header().Get(echo.HeaderCacheControl) != scenario.cache {
				t.Fatalf("Cache-Control = %q", recorder.Header().Get(echo.HeaderCacheControl))
			}
		})
	}
}

func TestDevelopmentProxy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set(echo.HeaderContentType, echo.MIMETextPlain)
		_, _ = io.WriteString(writer, request.URL.RequestURI()+"|"+request.Header.Get("X-Forwarded-Host"))
	}))
	t.Cleanup(backend.Close)
	target, _ := url.Parse(backend.URL)

	e := echo.New()
	e.HTTPErrorHandler = httpapi.ErrorHandler
	if err := MountDevelopment(e, target); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/src/main.jsx?direct=1", nil)
	request.Host = "localhost:5801"
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "/src/main.jsx?direct=1|localhost:5801") {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/route-without-extension", nil)
	request.Header.Set(echo.HeaderAccept, echo.MIMEApplicationJSON)
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "/route-without-extension") {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestDevelopmentProxyIsolationAndFailure(t *testing.T) {
	target, _ := url.Parse("http://127.0.0.1:1")
	e := echo.New()
	e.HTTPErrorHandler = httpapi.ErrorHandler
	if err := MountDevelopment(e, target); err != nil {
		t.Fatal(err)
	}

	apiRecorder := httptest.NewRecorder()
	e.ServeHTTP(apiRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil))
	if apiRecorder.Code != http.StatusNotFound || !strings.Contains(apiRecorder.Body.String(), "not_found") {
		t.Fatalf("API status = %d, body = %q", apiRecorder.Code, apiRecorder.Body.String())
	}

	frontendRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(echo.HeaderAccept, echo.MIMETextHTML)
	e.ServeHTTP(frontendRecorder, request)
	if frontendRecorder.Code != http.StatusBadGateway || !strings.Contains(frontendRecorder.Body.String(), "Vite development server is unavailable") {
		t.Fatalf("frontend status = %d, body = %q", frontendRecorder.Code, frontendRecorder.Body.String())
	}
}

func TestDevelopmentProxyWebSocketUpgrade(t *testing.T) {
	forwardedHost := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.EqualFold(request.Header.Get(echo.HeaderUpgrade), "websocket") {
			http.Error(writer, "missing websocket upgrade", http.StatusBadRequest)
			return
		}
		forwardedHost <- request.Header.Get("X-Forwarded-Host")
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer connection.Close()
		_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		_ = buffered.Flush()
	}))
	t.Cleanup(backend.Close)
	target, _ := url.Parse(backend.URL)

	e := echo.New()
	if err := MountDevelopment(e, target); err != nil {
		t.Fatal(err)
	}
	frontend := httptest.NewServer(e)
	t.Cleanup(frontend.Close)
	frontendURL, _ := url.Parse(frontend.URL)

	connection, err := net.Dial("tcp", frontendURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	request, _ := http.NewRequest(http.MethodGet, frontend.URL+"/@vite/client", nil)
	request.Host = "localhost:5801"
	_, _ = fmt.Fprintf(connection, "GET /@vite/client HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGVzdC1sdW1pLWhtcg==\r\n\r\n", request.Host)
	response, err := http.ReadResponse(bufio.NewReader(connection), request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if got := <-forwardedHost; got != "localhost:5801" {
		t.Fatalf("X-Forwarded-Host = %q", got)
	}
}

func TestEmbeddedHeadHasNoBody(t *testing.T) {
	e := echo.New()
	if err := MountEmbedded(e, fstest.MapFS{
		"index.html": {Data: []byte("index")}, "admin.html": {Data: []byte("admin")},
	}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/admin/settings", nil))
	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}
