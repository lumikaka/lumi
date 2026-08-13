package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
	"lumi/internal/server"
	"lumi/internal/webui"
	webassets "lumi/web"

	"github.com/labstack/echo/v4"
)

const usage = "Usage: lumi_web\n"

type httpLifecycle interface {
	Start(address string) error
	Shutdown(context.Context) error
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		slog.Error("lumi_web stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	if len(args) > 0 {
		if len(args) == 1 && args[0] == "--help" {
			_, err := io.WriteString(output, usage)
			return err
		}
		return fmt.Errorf("lumi_web: unexpected arguments: %s\n%s", strings.Join(args, " "), usage)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))
	appStore, err := appstore.Open(cfg.AppDataDir, cfg.DatabaseDSN)
	if err != nil {
		return err
	}
	defer appStore.Close()

	projectManager := project.NewManager(appStore)
	application, err := server.New(cfg, appStore, projectManager)
	if err != nil {
		return err
	}
	defer application.Close()
	if err := mountFrontend(application.Echo, cfg); err != nil {
		return err
	}

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(shutdownSignal)
	return serve(application, cfg, shutdownSignal)
}

func serve(application httpLifecycle, cfg config.Config, shutdownSignal <-chan os.Signal) error {
	serverErrors := make(chan error, 1)
	go func() {
		var startErr error
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("HTTP server panic recovered", "panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
				startErr = fmt.Errorf("HTTP server panic: %v", recovered)
			}
			serverErrors <- startErr
		}()

		slog.Info("server started", "address", cfg.Address, "environment", cfg.Environment)
		startErr = application.Start(cfg.Address)
		if errors.Is(startErr, http.ErrServerClosed) {
			startErr = nil
		}
	}()

	select {
	case err := <-serverErrors:
		return err
	case sig := <-shutdownSignal:
		slog.Info("shutdown signal received", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := application.Shutdown(ctx); err != nil {
		return err
	}
	return <-serverErrors
}

func mountFrontend(application *echo.Echo, cfg config.Config) error {
	if cfg.IsProduction() {
		frontend, embedded := webassets.Embedded()
		if !embedded {
			return errors.New("lumi_web: production frontend is not embedded; rebuild with the embed_frontend tag")
		}
		return webui.MountEmbedded(application, frontend)
	}
	target, err := cfg.ParsedViteDevServerURL()
	if err != nil {
		return err
	}
	return webui.MountDevelopment(application, target)
}
