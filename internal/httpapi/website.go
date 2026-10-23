package httpapi

import (
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v4"
	"lumi/internal/directoryopener"
	"lumi/internal/jobqueue"
	"lumi/internal/production"
	"lumi/internal/project"
)

func (h *ProductionHandler) Website(c echo.Context) error {
	var status production.WebsiteStatus
	err := h.withService(c, func(s *production.Service) error {
		var err error
		status, err = s.WebsiteStatus(c.Request().Context())
		return err
	})
	if err != nil {
		return err
	}
	return Success(c, http.StatusOK, status)
}
func (h *ProductionHandler) CreateWebsiteGeneration(c echo.Context) error {
	var input jobqueue.CreateWebsiteInput
	if err := decodeJSON(c, &input); err != nil {
		return err
	}
	task, err := h.tasks.CreateWebsiteGeneration(c.Request().Context(), c.Param("project_uuid"), input)
	if err != nil {
		return productionAPIError(err)
	}
	return Success(c, http.StatusAccepted, map[string]any{"task_uuid": task.UUID, "status": task.Status})
}
func (h *ProductionHandler) OpenWebsiteDirectory(c echo.Context) error {
	err := h.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		directory, err := store.ResolvePath("website")
		if err != nil {
			return err
		}
		return directoryopener.NewNative().Open(c.Request().Context(), directory)
	})
	if err != nil {
		return productionAPIError(err)
	}
	return Success(c, http.StatusCreated, nil)
}
func (h *ProductionHandler) WebsiteContent(c echo.Context) error {
	relative := c.Param("*")
	if relative == "" {
		relative = "index.html"
	}
	if path.Clean(relative) != relative || strings.ContainsAny(relative, "\\\x00") || strings.HasPrefix(relative, "/") {
		return NewError(http.StatusNotFound, "not_found", "网站文件不存在", "", nil)
	}
	err := h.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		f, err := production.NewService(store, h.events).OpenWebsiteContent(c.Request().Context(), relative)
		if err != nil {
			return err
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil {
			return err
		}
		c.Response().Header().Set("Cache-Control", "no-store")
		c.Response().Header().Set("X-Content-Type-Options", "nosniff")
		if relative == "rss.xml" {
			c.Response().Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		}
		c.Response().Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'")
		http.ServeContent(c.Response(), c.Request(), relative, stat.ModTime(), f)
		return nil
	})
	if err != nil {
		return productionAPIError(err)
	}
	return nil
}
