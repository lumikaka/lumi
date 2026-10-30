package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/httpapi"
	"lumi/internal/processlock"
	"lumi/internal/project"
	"lumi/internal/server"
	"lumi/internal/webui"
	webassets "lumi/web"
)

const cliProjectIdleGrace = 15 * time.Second

func runDaemon(opt options) error {
	lock, err := processlock.Try(opt.path(".lock"))
	if err != nil {
		return fmt.Errorf("CLI singleton: %w", err)
	}
	defer lock.Close()
	// CLI's environment is explicit, including when the invoking shell has
	// development/desktop configuration. Never consume a desktop session token.
	_ = os.Setenv("APP_ENV", opt.environment)
	_ = os.Setenv("LUMI_DATA_DIR", opt.dataDir)
	_ = os.Unsetenv("LUMI_DESKTOP_ACCESS_TOKEN")
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	_ = os.Unsetenv("LUMI_DESKTOP_ACCESS_TOKEN") // A development .env may contain it.
	cfg.DesktopAuth = nil
	cfg.ProjectIdleGrace = cliProjectIdleGrace
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	cfg.Address = listener.Addr().String()
	cfg.FrontendURL = "http://" + cfg.Address
	if err := cfg.Validate(); err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))
	store, err := appstore.Open(cfg.AppDataDir, cfg.DatabaseDSN)
	if err != nil {
		return err
	}
	defer store.Close()
	projects := project.NewManager(store)
	defer projects.Close()
	application, err := server.New(cfg, store, projects)
	if err != nil {
		return err
	}
	defer application.Close()
	if frontend, embedded := webassets.Embedded(); embedded {
		if err := webui.MountEmbedded(application.Echo, frontend); err != nil {
			return err
		}
	} else if !cfg.IsProduction() {
		target, err := cfg.ParsedViteDevServerURL()
		if err != nil {
			return err
		}
		if err := webui.MountDevelopment(application.Echo, target); err != nil {
			return err
		}
	} else {
		application.GET("/", func(c echo.Context) error {
			return c.String(503, "Frontend is not embedded. Run make build for the complete Lumi CLI.\n")
		})
	}
	nonce, err := newNonce()
	if err != nil {
		return err
	}
	value := instance{Endpoint: cfg.FrontendURL, PID: os.Getpid(), Environment: opt.environment, Nonce: nonce}
	registerInitialization(application.Echo, value, projects)
	shutdown := make(chan struct{}, 1)
	application.GET(instanceRoute, func(c echo.Context) error { return serveIdentity(c, value, projects) })
	application.DELETE(instanceRoute, func(c echo.Context) error {
		if c.Request().Header.Get("If-Match") != value.Nonce {
			return httpapi.NewError(409, "cli_instance_changed", "CLI 实例已更换", "请重新读取 CLI 发现信息。", nil)
		}
		if err := serveIdentity(c, value, projects); err != nil {
			return err
		}
		select {
		case shutdown <- struct{}{}:
		default:
		}
		return nil
	})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	// Serve Echo directly: CLI deliberately does not publish or replace the
	// desktop/legacy MCP discovery file owned by Application.Start.
	application.Echo.Server.Handler = application.Echo
	serveErr := make(chan error, 1)
	go func() { serveErr <- application.Echo.Server.Serve(listener) }()
	cleanup, err := writeInstance(opt, value)
	if err != nil {
		_ = application.Echo.Server.Close()
		<-serveErr
		return err
	}
	defer func() {
		// Keep discovery and the singleton lock until project shutdown finishes,
		// including unexpected listener failures.
		_ = application.Close()
		cleanup()
	}()
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-shutdown:
	case <-signals:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = application.Shutdown(ctx)
	if err != nil {
		_ = application.Echo.Server.Close()
	}
	return errors.Join(err, normalizeServeError(<-serveErr))
}

func normalizeServeError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func serveIdentity(c echo.Context, value instance, projects *project.Manager) error {
	items, err := projects.OpenProjects(c.Request().Context())
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return httpapi.Success(c, 200, instanceStatus{instance: value, Running: true, OpenProjects: items})
}
