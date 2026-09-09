package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/labstack/echo/v4"
	"lumi/internal/config"
	"lumi/internal/httpapi"
	"lumi/internal/mcpbridge"
	"lumi/internal/mcpserver"
	"net"
	"net/http"
	"os"
)

// Start publishes MCP on the actual business listener, including a desktop
// fallback port. stdio discovers this address without starting another runtime.
func (a *Application) Start(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	bound := listener.Addr().(*net.TCPAddr)
	if !bound.IP.Equal(net.ParseIP("127.0.0.1")) {
		return fmt.Errorf("local Lumi MCP requires APP_ADDRESS to bind 127.0.0.1")
	}
	nonce := make([]byte, 32)
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	base := "http://" + listener.Addr().String()
	instance := mcpbridge.Instance{Endpoint: base + "/mcp", Nonce: hex.EncodeToString(nonce), Environment: mcpEnvironment(a.mcpConfig)}
	a.mountMCP(base, instance.Nonce)
	cleanup, err := mcpbridge.Write(mcpbridge.Path(a.mcpConfig.AppDataDir, mcpEnvironment(a.mcpConfig)), instance)
	if err != nil {
		return err
	}
	defer cleanup()
	a.Echo.Listener = listener
	return a.Echo.Start(address)
}

// Protocol paths have independent authentication and standard response formats.
// Intercept before desktop Cookie, CORS, REST Origin and request-log middleware;
// all other paths (especially consent/grant management) retain that middleware.
func (a *Application) mountMCP(base, nonce string) {
	handler := a.mcpService.LocalHTTP(base, a.mcpConfig.FrontendURL, nonce)
	a.Echo.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			switch c.Request().URL.Path {
			case "/mcp", "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-authorization-server", "/oauth/register", "/oauth/authorize", "/oauth/token":
				handler.ServeHTTP(c.Response(), c.Request())
				return nil
			default:
				return next(c)
			}
		}
	})
}

func configureMCPManagement(api *echo.Group, s *mcpserver.Service, cfg config.Config) {
	cfg.Environment = mcpEnvironment(cfg)
	api.GET("/mcp", func(c echo.Context) error { return httpapi.Success(c, 200, s.ConnectionInfo()) })
	api.GET("/mcp-authorization-requests/:uuid", func(c echo.Context) error {
		value, err := s.OAuthRequest(c.Request().Context(), c.Param("uuid"))
		if err != nil {
			return httpapi.NewError(404, "mcp_authorization_not_found", "授权请求不存在或已过期，请重新连接客户端。", "", nil)
		}
		c.Response().Header().Set("Cache-Control", "no-store")
		return httpapi.Success(c, 200, value)
	})
	api.POST("/mcp-authorization-requests/:uuid/decisions", func(c echo.Context) error {
		var input struct {
			ProjectUUID string `json:"project_uuid"`
			Permission  string `json:"permission"`
			Decision    string `json:"decision"`
		}
		if err := decodeMCPManagement(c, &input); err != nil {
			return err
		}
		redirect, err := s.DecideOAuth(c.Request().Context(), c.Param("uuid"), input.ProjectUUID, input.Permission, input.Decision)
		if err != nil {
			return httpapi.NewError(409, "mcp_authorization_failed", "无法完成授权，请检查项目或重新连接客户端。", "", nil)
		}
		c.Response().Header().Set("Cache-Control", "no-store")
		return httpapi.Success(c, 200, map[string]any{"redirect_url": redirect})
	})
	base := "/projects/:project_uuid/mcp-grants"
	api.GET(base, func(c echo.Context) error {
		items, err := s.Grants(c.Request().Context(), c.Param("project_uuid"))
		if err != nil {
			return err
		}
		return httpapi.Success(c, 200, map[string]any{"items": items})
	})
	api.POST(base, func(c echo.Context) error {
		var input struct {
			Name       string `json:"name"`
			Permission string `json:"permission"`
		}
		if err := decodeMCPManagement(c, &input); err != nil {
			return err
		}
		g, token, err := s.CreateGrant(c.Request().Context(), c.Param("project_uuid"), input.Name, input.Permission)
		if err != nil {
			return httpapi.NewError(422, "mcp_invalid_grant", "无法创建授权", err.Error(), nil)
		}
		c.Response().Header().Set("Cache-Control", "no-store")
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		return httpapi.Success(c, 201, map[string]any{"grant": g, "token": token, "client_config": map[string]any{"mcpServers": map[string]any{"lumi": map[string]any{"command": executable, "args": []string{"--mcp", "--environment", cfg.Environment, "--data-dir", cfg.AppDataDir}, "env": map[string]string{"LUMI_MCP_TOKEN": token}}}}})
	})
	api.DELETE(base+"/:grant_uuid", func(c echo.Context) error {
		if err := s.Revoke(c.Request().Context(), c.Param("project_uuid"), c.Param("grant_uuid")); err != nil {
			return httpapi.NewError(404, "mcp_grant_not_found", "授权不存在", "", err)
		}
		return httpapi.Success(c, 200, nil)
	})
	calls := "/projects/:project_uuid/mcp-calls"
	api.GET(calls, func(c echo.Context) error {
		items, err := s.Calls(c.Request().Context(), c.Param("project_uuid"))
		if err != nil {
			return err
		}
		return httpapi.Success(c, 200, map[string]any{"items": items})
	})
	api.POST(calls+"/:call_uuid/decisions", func(c echo.Context) error {
		var input struct {
			Decision    string `json:"decision"`
			Fingerprint string `json:"fingerprint"`
		}
		if err := decodeMCPManagement(c, &input); err != nil {
			return err
		}
		result := s.Decide(context.WithoutCancel(c.Request().Context()), c.Param("project_uuid"), c.Param("call_uuid"), input.Fingerprint, input.Decision)
		status := 200
		if result["success"] == false {
			status = 409
		}
		return c.JSON(status, result)
	})
}
func decodeMCPManagement(c echo.Context, v any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil || !decoderAtEOF(decoder) {
		return httpapi.NewError(422, "validation_failed", "请求参数无效", "只允许文档列出的 JSON 字段。", errors.New("invalid MCP management request"))
	}
	return nil
}

func mcpEnvironment(cfg config.Config) string {
	if cfg.IsProduction() {
		return "production"
	}
	return "development"
}
