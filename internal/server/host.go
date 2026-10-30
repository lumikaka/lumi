package server

import (
	"net"
	"net/http"

	"lumi/internal/httpapi"

	"github.com/labstack/echo/v4"
)

// Run before routing and protocol interceptors so GET, media and preflights
// receive the same DNS-rebinding protection. Never trust forwarded headers.
func trustedHost(address string) echo.MiddlewareFunc {
	_, configuredPort, _ := net.SplitHostPort(address)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			r := c.Request()
			expectedPort := configuredPort
			if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
				_, expectedPort, _ = net.SplitHostPort(local.String())
			}
			if expectedPort == "" || (r.Host != net.JoinHostPort("127.0.0.1", expectedPort) && r.Host != net.JoinHostPort("localhost", expectedPort) && r.Host != net.JoinHostPort("::1", expectedPort)) {
				return httpapi.NewError(http.StatusForbidden, "untrusted_host", "请求主机不受信任", "请使用本实例端口上的 127.0.0.1、localhost 或 [::1]。", nil)
			}
			return next(c)
		}
	}
}
