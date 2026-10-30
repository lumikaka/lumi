package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"lumi/internal/httpapi"
)

func TestTrustedHostCoversEveryPathAndActualListenerPort(t *testing.T) {
	for _, host := range []string{"127.0.0.1:8123", "localhost:8123", "[::1]:8123", "evil.example:8123", "127.0.0.1:9999", "localhost", "localhost.:8123", "127.0.0.2:8123", "[::ffff:127.0.0.1]:8123", "[localhost]:8123", "[127.0.0.1]:8123"} {
		for _, path := range []string{"/api/v1/providers", "/media/file", "/", "/mcp", "/oauth/token"} {
			e := echo.New()
			e.HTTPErrorHandler = httpapi.ErrorHandler
			e.Pre(trustedHost("127.0.0.1:0"))
			e.Any("/*", func(c echo.Context) error { return c.NoContent(204) })
			r := httptest.NewRequest("GET", "http://127.0.0.1:8123"+path, nil)
			r.Host = host
			r.Header.Set("X-Forwarded-Host", "localhost:8123")
			r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8123}))
			w := httptest.NewRecorder()
			e.ServeHTTP(w, r)
			want := 403
			if host == "127.0.0.1:8123" || host == "localhost:8123" || host == "[::1]:8123" {
				want = 204
			}
			if w.Code != want {
				t.Fatalf("Host=%q path=%s: %d, want %d", host, path, w.Code, want)
			}
		}
	}
}
