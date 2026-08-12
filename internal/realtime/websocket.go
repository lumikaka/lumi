package realtime

import (
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

type WebSocketHandler struct {
	hub      *Hub
	upgrader websocket.Upgrader
}

func NewWebSocketHandler(hub *Hub, frontendURL string) *WebSocketHandler {
	return &WebSocketHandler{
		hub: hub,
		upgrader: websocket.Upgrader{
			HandshakeTimeout: writeWait,
			CheckOrigin: func(request *http.Request) bool {
				origin := request.Header.Get("Origin")
				allowed := OriginAllowed(origin, frontendURL)
				if !allowed {
					slog.Warn("websocket origin rejected", "origin", origin)
				}
				return allowed
			},
		},
	}
}

func (handler *WebSocketHandler) Serve(c echo.Context) error {
	connection, err := handler.upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		// The upgrader already wrote the HTTP response.
		return nil
	}
	newClient(c.Request().Context(), handler.hub, connection).run()
	return nil
}

func OriginAllowed(origin, frontendURL string) bool {
	originURL, originErr := url.Parse(strings.TrimSpace(origin))
	if originErr != nil || originURL.Scheme == "" || originURL.Host == "" {
		return false
	}
	for _, allowed := range AllowedOrigins(frontendURL) {
		frontend, err := url.Parse(allowed)
		if err == nil && strings.EqualFold(originURL.Scheme, frontend.Scheme) && strings.EqualFold(originURL.Host, frontend.Host) {
			return true
		}
	}
	return false
}

func AllowedOrigins(frontendURL string) []string {
	result := make([]string, 0, 3)
	seen := make(map[string]struct{})
	add := func(origin string) {
		if _, exists := seen[origin]; exists {
			return
		}
		seen[origin] = struct{}{}
		result = append(result, origin)
	}
	for _, raw := range strings.Split(frontendURL, ",") {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		hostname := strings.ToLower(parsed.Hostname())
		add(originString(parsed.Scheme, hostname, parsed.Port()))
		if hostname == "localhost" || isLoopbackIP(hostname) {
			for _, alias := range []string{"localhost", "127.0.0.1", "::1"} {
				add(originString(parsed.Scheme, alias, parsed.Port()))
			}
		}
	}
	return result
}

func originString(scheme, hostname, port string) string {
	host := hostname
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return (&url.URL{Scheme: strings.ToLower(scheme), Host: host}).String()
}

func isLoopbackIP(hostname string) bool {
	address := net.ParseIP(hostname)
	return address != nil && address.IsLoopback()
}
