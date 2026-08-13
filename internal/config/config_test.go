package config

import (
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"lumi/internal/platformpath"
)

func TestLoadUsesDefaults(t *testing.T) {
	for _, key := range []string{"APP_ENV", "APP_ADDRESS", "FRONTEND_URL", "VITE_DEV_SERVER_URL", "LUMI_DATA_DIR", "DATABASE_DSN", logLevelEnv, desktopAccessTokenEnv} {
		t.Setenv(key, "")
	}
	dataDir, err := platformpath.DefaultAppDataDir(defaultEnvironment)
	if err != nil {
		t.Fatal(err)
	}

	want := Config{
		Environment:      defaultEnvironment,
		Address:          defaultAddress,
		FrontendURL:      defaultFrontendURL,
		ViteDevServerURL: defaultViteDevServerURL,
		AppDataDir:       dataDir,
		DatabaseDSN:      SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")),
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv(logLevelEnv, "")
	t.Setenv("APP_ADDRESS", ":15801")
	t.Setenv("FRONTEND_URL", "https://lumi.example")
	t.Setenv("VITE_DEV_SERVER_URL", "http://vite.example:5802")
	t.Setenv("LUMI_DATA_DIR", "/tmp/lumi-test-data")
	t.Setenv("DATABASE_DSN", "file:test.sqlite3")

	want := Config{
		Environment: "test", Address: ":15801", FrontendURL: "https://lumi.example",
		ViteDevServerURL: "http://vite.example:5802", AppDataDir: "/tmp/lumi-test-data", DatabaseDSN: "file:test.sqlite3",
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadUsesEnvironmentSpecificLogLevelDefaults(t *testing.T) {
	for _, scenario := range []struct {
		environment string
		want        slog.Level
	}{
		{environment: "development", want: slog.LevelInfo},
		{environment: "test", want: slog.LevelInfo},
		{environment: "production", want: slog.LevelWarn},
		{environment: "PRODUCTION", want: slog.LevelWarn},
	} {
		t.Run(scenario.environment, func(t *testing.T) {
			t.Setenv("APP_ENV", scenario.environment)
			t.Setenv("LUMI_DATA_DIR", t.TempDir())
			unsetEnv(t, logLevelEnv)

			got, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if got.LogLevel != scenario.want {
				t.Fatalf("LogLevel = %s, want %s", got.LogLevel, scenario.want)
			}
		})
	}
}

func TestLoadLogLevelOverride(t *testing.T) {
	for _, scenario := range []struct {
		value string
		want  slog.Level
	}{
		{value: "debug", want: slog.LevelDebug},
		{value: "INFO", want: slog.LevelInfo},
		{value: " Warn ", want: slog.LevelWarn},
		{value: "error", want: slog.LevelError},
	} {
		t.Run(scenario.value, func(t *testing.T) {
			t.Setenv("APP_ENV", "production")
			t.Setenv("LUMI_DATA_DIR", t.TempDir())
			t.Setenv(logLevelEnv, scenario.value)

			got, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if got.LogLevel != scenario.want {
				t.Fatalf("LogLevel = %s, want %s", got.LogLevel, scenario.want)
			}
		})
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	for _, value := range []string{"trace", "   "} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("APP_ENV", "production")
			t.Setenv("LUMI_DATA_DIR", t.TempDir())
			t.Setenv(logLevelEnv, value)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "debug, info, warn, or error") {
				t.Fatalf("Load() error = %v", err)
			}
		})
	}
}

func TestLoadReturnsDefaultAppDataResolutionFailure(t *testing.T) {
	for _, key := range []string{"APP_ENV", "LUMI_DATA_DIR", "DATABASE_DSN"} {
		t.Setenv(key, "")
	}
	t.Setenv(logLevelEnv, "")
	previous := resolveDefaultAppDataDir
	resolveDefaultAppDataDir = func(string) (string, error) { return "", errors.New("local data unavailable") }
	t.Cleanup(func() { resolveDefaultAppDataDir = previous })

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "LUMI_DATA_DIR") {
		t.Fatalf("Load() error = %v", err)
	}

	override := t.TempDir()
	t.Setenv("LUMI_DATA_DIR", override)
	got, err := Load()
	if err != nil || got.AppDataDir != override {
		t.Fatalf("explicit LUMI_DATA_DIR = %q, error = %v", got.AppDataDir, err)
	}
}

func TestProductionLoadIgnoresDotEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".env", []byte("LUMI_DATA_DIR=/tmp/lumi-from-dotenv\nDATABASE_DSN=file:from-dotenv.sqlite\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv(logLevelEnv, "")
	unsetEnv(t, "LUMI_DATA_DIR")
	unsetEnv(t, "DATABASE_DSN")

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.AppDataDir == "/tmp/lumi-from-dotenv" || got.DatabaseDSN == "file:from-dotenv.sqlite" {
		t.Fatalf("production loaded local .env values: %#v", got)
	}
	wantDataDir, err := platformpath.DefaultAppDataDir("production")
	if err != nil {
		t.Fatal(err)
	}
	if got.AppDataDir != wantDataDir {
		t.Fatalf("production AppDataDir = %q, want %q", got.AppDataDir, wantDataDir)
	}
}

func TestLoadConsumesDesktopAccessToken(t *testing.T) {
	const token = "runtime-desktop-token"
	t.Setenv(desktopAccessTokenEnv, token)
	t.Setenv(logLevelEnv, "")

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.DesktopAuth == nil || !got.DesktopAuth.MatchesToken(token) {
		t.Fatal("Load() did not configure desktop authentication")
	}
	if got.DesktopAuth.MatchesToken("different-token") {
		t.Fatal("desktop authentication accepted a different token")
	}
	if _, exists := os.LookupEnv(desktopAccessTokenEnv); exists {
		t.Fatalf("%s remained in the process environment", desktopAccessTokenEnv)
	}
}

func TestValidate(t *testing.T) {
	valid := Config{
		Environment: "development", Address: ":5801", FrontendURL: "http://localhost:5801",
		ViteDevServerURL: "http://127.0.0.1:5802", AppDataDir: t.TempDir(), DatabaseDSN: "file:test.sqlite3",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	invalid := valid
	invalid.ViteDevServerURL = "/relative"
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() succeeded with a relative Vite URL")
	}

	production := invalid
	production.Environment = "PRODUCTION"
	if err := production.Validate(); err != nil {
		t.Fatalf("production should ignore Vite URL: %v", err)
	}
}

func TestSQLiteDSNKeepsWindowsDriveInPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lumi data.sqlite")
	uri := strings.SplitN(SQLiteDSN(path), "?", 2)[0]
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "" {
		t.Fatalf("SQLite URI %q has authority %q", uri, parsed.Host)
	}
	uriPath := parsed.Path
	if uriPath == "" {
		uriPath = parsed.Opaque
	}
	decoded, err := url.PathUnescape(uriPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Clean(filepath.FromSlash(decoded)); got != filepath.Clean(path) {
		t.Fatalf("SQLite URI path = %q, want %q", got, path)
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	previous, found := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if found {
			_ = os.Setenv(key, previous)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
