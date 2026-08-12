package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"lumi/internal/config"
	"lumi/internal/httpapi"

	"github.com/labstack/echo/v4"
)

const desktopSessionCookie = "lumi_desktop_session"

type desktopSessionHandler struct {
	authentication *config.DesktopAuthentication
}

type desktopSessionInput struct {
	Token string `json:"token"`
}

func newDesktopSessionHandler(authentication *config.DesktopAuthentication) *desktopSessionHandler {
	return &desktopSessionHandler{authentication: authentication}
}

func (handler *desktopSessionHandler) Create(c echo.Context) error {
	var input desktopSessionInput
	body := http.MaxBytesReader(c.Response(), c.Request().Body, 4096)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || !decoderAtEOF(decoder) || !handler.authentication.MatchesToken(input.Token) {
		return desktopAuthenticationFailed()
	}
	c.SetCookie(&http.Cookie{
		Name:     desktopSessionCookie,
		Value:    input.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	return httpapi.Success(c, http.StatusCreated, nil)
}

func decoderAtEOF(decoder *json.Decoder) bool {
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

func desktopAuthentication(authentication *config.DesktopAuthentication) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if authentication == nil || desktopAuthenticationPublicRequest(c.Request()) || !desktopAuthenticationProtectedPath(c.Request().URL.Path) {
				return next(c)
			}
			cookie, err := c.Cookie(desktopSessionCookie)
			if err != nil || !authentication.MatchesToken(cookie.Value) {
				return httpapi.NewError(
					http.StatusUnauthorized,
					"desktop_authentication_required",
					"桌面会话未认证",
					"请从 Lumi 菜单栏重新打开应用。",
					nil,
				)
			}
			return next(c)
		}
	}
}

func desktopAuthenticationPublicRequest(request *http.Request) bool {
	if request.Method == http.MethodOptions {
		return true
	}
	return request.URL.Path == "/api/v1/health" || request.URL.Path == "/api/v1/desktop-sessions"
}

func desktopAuthenticationProtectedPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/") || path == "/media" || strings.HasPrefix(path, "/media/")
}

func desktopAuthenticationFailed() error {
	return httpapi.NewError(
		http.StatusUnauthorized,
		"desktop_authentication_failed",
		"桌面认证失败",
		"访问令牌无效，请从 Lumi 菜单栏重新打开应用。",
		nil,
	)
}
