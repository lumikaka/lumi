package webui

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

const (
	indexDocument = "index.html"
	adminDocument = "admin.html"
)

type embeddedHandler struct {
	assets    fs.FS
	static    http.Handler
	indexHTML []byte
	adminHTML []byte
}

type developmentHandler struct {
	proxy *httputil.ReverseProxy
}

func MountEmbedded(e *echo.Echo, assets fs.FS) error {
	if e == nil {
		return errors.New("webui: echo instance is required")
	}
	handler, err := newEmbeddedHandler(assets)
	if err != nil {
		return err
	}
	mount(e, handler.handle)
	return nil
}

func MountDevelopment(e *echo.Echo, target *url.URL) error {
	if e == nil {
		return errors.New("webui: echo instance is required")
	}
	if target == nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return errors.New("webui: absolute Vite HTTP(S) URL is required")
	}
	handler := &developmentHandler{proxy: &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.SetXForwarded()
		},
		FlushInterval: -1 * time.Millisecond,
		ErrorHandler: func(writer http.ResponseWriter, _ *http.Request, err error) {
			writer.Header().Set(echo.HeaderContentType, echo.MIMETextPlainCharsetUTF8)
			writer.WriteHeader(http.StatusBadGateway)
			_, _ = fmt.Fprintf(writer, "Vite development server is unavailable: %v\n", err)
		},
	}}
	mount(e, handler.handle)
	return nil
}

func mount(e *echo.Echo, handler echo.HandlerFunc) {
	e.GET("/*", handler)
	e.HEAD("/*", handler)
}

func newEmbeddedHandler(assets fs.FS) (*embeddedHandler, error) {
	if assets == nil {
		return nil, errors.New("webui: frontend filesystem is required")
	}
	indexHTML, err := fs.ReadFile(assets, indexDocument)
	if err != nil {
		return nil, fmt.Errorf("webui: read %s: %w", indexDocument, err)
	}
	adminHTML, err := fs.ReadFile(assets, adminDocument)
	if err != nil {
		return nil, fmt.Errorf("webui: read %s: %w", adminDocument, err)
	}
	return &embeddedHandler{
		assets: assets, static: http.FileServer(http.FS(assets)), indexHTML: indexHTML, adminHTML: adminHTML,
	}, nil
}

func (handler *embeddedHandler) handle(c echo.Context) error {
	requestPath := c.Request().URL.Path
	if isBackendPath(requestPath) {
		return echo.ErrNotFound
	}
	if assetPath, ok := validAssetPath(requestPath); ok {
		if info, err := fs.Stat(handler.assets, assetPath); err == nil && !info.IsDir() {
			if strings.HasPrefix(assetPath, "assets/") {
				c.Response().Header().Set(echo.HeaderCacheControl, "public, max-age=31536000, immutable")
			} else {
				c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
			}
			handler.static.ServeHTTP(c.Response(), c.Request())
			return nil
		}
	}
	if !isHTMLNavigation(c.Request()) {
		return echo.ErrNotFound
	}
	if isAdminPath(requestPath) {
		return writeHTML(c, handler.adminHTML)
	}
	return writeHTML(c, handler.indexHTML)
}

func (handler *developmentHandler) handle(c echo.Context) error {
	request := c.Request()
	if isBackendPath(request.URL.Path) {
		return echo.ErrNotFound
	}
	handler.proxy.ServeHTTP(c.Response(), request)
	return nil
}

func writeHTML(c echo.Context, content []byte) error {
	response := c.Response()
	response.Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	response.Header().Set(echo.HeaderCacheControl, "no-cache")
	response.Header().Set(echo.HeaderContentLength, strconv.Itoa(len(content)))
	response.WriteHeader(http.StatusOK)
	if c.Request().Method == http.MethodHead {
		return nil
	}
	_, err := bytes.NewReader(content).WriteTo(response)
	return err
}

func isBackendPath(requestPath string) bool {
	return requestPath == "/api" || strings.HasPrefix(requestPath, "/api/")
}

func isAdminPath(requestPath string) bool {
	return requestPath == "/admin" || strings.HasPrefix(requestPath, "/admin/")
}

func validAssetPath(requestPath string) (string, bool) {
	trimmed := strings.TrimPrefix(requestPath, "/")
	if trimmed == "" {
		return "", false
	}
	cleaned := path.Clean(trimmed)
	if cleaned != trimmed || !fs.ValidPath(cleaned) {
		return "", false
	}
	return cleaned, true
}

func isHTMLNavigation(request *http.Request) bool {
	if strings.Contains(path.Base(request.URL.Path), ".") {
		return false
	}
	accept := request.Header.Get(echo.HeaderAccept)
	if accept == "" {
		return true
	}
	for _, value := range strings.Split(accept, ",") {
		mediaType := strings.TrimSpace(strings.SplitN(value, ";", 2)[0])
		if mediaType == echo.MIMETextHTML || mediaType == "*/*" {
			return true
		}
	}
	return false
}
