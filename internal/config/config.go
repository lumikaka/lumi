package config

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"lumi/internal/platformpath"

	"github.com/joho/godotenv"
)

const (
	defaultEnvironment      = "development"
	defaultAddress          = "127.0.0.1:5801"
	defaultFrontendURL      = "http://localhost:5801"
	defaultViteDevServerURL = "http://127.0.0.1:5802"
	desktopAccessTokenEnv   = "LUMI_DESKTOP_ACCESS_TOKEN"
	logLevelEnv             = "LOG_LEVEL"
)

var resolveDefaultAppDataDir = platformpath.DefaultAppDataDir

// DesktopAuthentication holds only the digest of the runtime token supplied by
// the desktop launcher. The raw token must never become part of application
// configuration or be persisted.
type DesktopAuthentication struct {
	tokenHash [sha256.Size]byte
}

// NewDesktopAuthentication returns nil when token authentication is disabled.
func NewDesktopAuthentication(token string) *DesktopAuthentication {
	if token == "" {
		return nil
	}
	return &DesktopAuthentication{tokenHash: sha256.Sum256([]byte(token))}
}

// MatchesToken compares a candidate token without exposing the stored digest.
func (authentication *DesktopAuthentication) MatchesToken(token string) bool {
	if authentication == nil || token == "" {
		return false
	}
	candidateHash := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(authentication.tokenHash[:], candidateHash[:]) == 1
}

type Config struct {
	Environment      string
	LogLevel         slog.Level
	Address          string
	FrontendURL      string
	ViteDevServerURL string
	AppDataDir       string
	DatabaseDSN      string
	DesktopAuth      *DesktopAuthentication
}

func Load() (Config, error) {
	// Local development values never override variables supplied by the process.
	// Packaged production apps run from the user's home directory and must not
	// accidentally consume an unrelated ~/.env file.
	environment := getEnv("APP_ENV", defaultEnvironment)
	if !strings.EqualFold(strings.TrimSpace(environment), "production") {
		_ = godotenv.Load()
		environment = getEnv("APP_ENV", defaultEnvironment)
	}
	logLevel, err := loadLogLevel(environment)
	if err != nil {
		return Config{}, err
	}
	dataDirectory := strings.TrimSpace(os.Getenv("LUMI_DATA_DIR"))
	if dataDirectory == "" {
		var err error
		dataDirectory, err = resolveDefaultAppDataDir(environment)
		if err != nil {
			return Config{}, fmt.Errorf("resolve default LUMI_DATA_DIR: %w", err)
		}
	}
	databaseDSN := strings.TrimSpace(os.Getenv("DATABASE_DSN"))
	if databaseDSN == "" && dataDirectory != "" {
		databaseDSN = SQLiteDSN(filepath.Join(dataDirectory, "lumi.sqlite"))
	}
	desktopAuthentication := NewDesktopAuthentication(os.Getenv(desktopAccessTokenEnv))
	if desktopAuthentication != nil {
		_ = os.Unsetenv(desktopAccessTokenEnv)
	}

	return Config{
		Environment:      environment,
		LogLevel:         logLevel,
		Address:          getEnv("APP_ADDRESS", defaultAddress),
		FrontendURL:      getEnv("FRONTEND_URL", defaultFrontendURL),
		ViteDevServerURL: getEnv("VITE_DEV_SERVER_URL", defaultViteDevServerURL),
		AppDataDir:       dataDirectory,
		DatabaseDSN:      databaseDSN,
		DesktopAuth:      desktopAuthentication,
	}, nil
}

func (cfg Config) Validate() error {
	if strings.TrimSpace(cfg.Environment) == "" {
		return fmt.Errorf("APP_ENV cannot be empty")
	}
	if !validLogLevel(cfg.LogLevel) {
		return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, or error")
	}
	if strings.TrimSpace(cfg.Address) == "" {
		return fmt.Errorf("APP_ADDRESS cannot be empty")
	}
	if _, _, err := net.SplitHostPort(strings.TrimSpace(cfg.Address)); err != nil {
		return fmt.Errorf("APP_ADDRESS must be a host:port address: %w", err)
	}
	if strings.TrimSpace(cfg.AppDataDir) == "" {
		return fmt.Errorf("LUMI_DATA_DIR cannot be empty")
	}
	if !filepath.IsAbs(cfg.AppDataDir) {
		return fmt.Errorf("LUMI_DATA_DIR must be an absolute path")
	}
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		return fmt.Errorf("DATABASE_DSN cannot be empty")
	}
	if _, err := absoluteHTTPURL("FRONTEND_URL", cfg.FrontendURL); err != nil {
		return err
	}
	if !cfg.IsProduction() {
		if _, err := absoluteHTTPURL("VITE_DEV_SERVER_URL", cfg.ViteDevServerURL); err != nil {
			return err
		}
	}
	return nil
}

func loadLogLevel(environment string) (slog.Level, error) {
	raw, exists := os.LookupEnv(logLevelEnv)
	if !exists || raw == "" {
		if strings.EqualFold(strings.TrimSpace(environment), "production") {
			return slog.LevelWarn, nil
		}
		return slog.LevelInfo, nil
	}

	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, or error")
	}
}

func validLogLevel(level slog.Level) bool {
	return level == slog.LevelDebug || level == slog.LevelInfo || level == slog.LevelWarn || level == slog.LevelError
}

// SQLiteFileURI encodes a local path without letting a Windows drive letter
// become a URI authority.
func SQLiteFileURI(path string) string {
	// A Windows drive path such as C:/data/lumi.sqlite must remain part of the
	// URI path. The default URL rendering would produce file://C:/..., which
	// treats C: as an authority and is rejected by SQLite.
	return (&url.URL{Scheme: "file", OmitHost: true, Path: filepath.ToSlash(path)}).String()
}

// SQLiteDSN applies the connection-level safety settings shared by both stores.
func SQLiteDSN(path string) string {
	return SQLiteFileURI(path) + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
}

func (cfg Config) IsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(cfg.Environment), "production")
}

func (cfg Config) ParsedViteDevServerURL() (*url.URL, error) {
	return absoluteHTTPURL("VITE_DEV_SERVER_URL", cfg.ViteDevServerURL)
}

func absoluteHTTPURL(name, raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("%s must be an absolute HTTP(S) URL", name)
	}
	return parsed, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
