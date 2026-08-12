package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"lumi/internal/config"
	webassets "lumi/web"

	"github.com/labstack/echo/v4"
)

func TestHelpAndUnexpectedArguments(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != usage {
		t.Fatalf("help = %q, want %q", output.String(), usage)
	}
	if err := run([]string{"serve"}, &output); err == nil || !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("unexpected argument error = %v", err)
	}
}

func TestDevelopmentMountsViteProxy(t *testing.T) {
	e := echo.New()
	err := mountFrontend(e, config.Config{
		Environment: "development", ViteDevServerURL: "http://127.0.0.1:5802",
	})
	if err != nil || len(e.Routes()) != 2 {
		t.Fatalf("routes = %d, error = %v", len(e.Routes()), err)
	}
}

func TestProductionRequiresEmbeddedFrontend(t *testing.T) {
	_, embedded := webassets.Embedded()
	err := mountFrontend(echo.New(), config.Config{Environment: "production"})
	if embedded && err != nil {
		t.Fatalf("embedded production mount error = %v", err)
	}
	if !embedded && (err == nil || !strings.Contains(err.Error(), "embed_frontend")) {
		t.Fatalf("missing frontend error = %v", err)
	}
}

type fakeLifecycle struct {
	started      chan struct{}
	stopped      chan struct{}
	shutdownSeen bool
}

func (server *fakeLifecycle) Start(_ string) error {
	close(server.started)
	<-server.stopped
	return http.ErrServerClosed
}

func (server *fakeLifecycle) Shutdown(_ context.Context) error {
	server.shutdownSeen = true
	close(server.stopped)
	return nil
}

func TestServeShutsDownGracefully(t *testing.T) {
	server := &fakeLifecycle{started: make(chan struct{}), stopped: make(chan struct{})}
	signals := make(chan os.Signal, 1)
	result := make(chan error, 1)
	go func() {
		result <- serve(server, config.Config{Address: ":5801", Environment: "test"}, signals)
	}()

	<-server.started
	signals <- syscall.SIGTERM
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
		if !server.shutdownSeen {
			t.Fatal("Shutdown was not called")
		}
	case <-time.After(time.Second):
		t.Fatal("graceful shutdown timed out")
	}
}
