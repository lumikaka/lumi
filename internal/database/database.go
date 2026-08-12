package database

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/libtnb/sqlite"
	"gorm.io/gorm"
)

func Open(dsn string) (*gorm.DB, error) {
	if err := EnsureParentDirectory(dsn); err != nil {
		return nil, err
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get database handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

func EnsureParentDirectory(dsn string) error {
	path, ok, err := databaseFilePath(dsn)
	if err != nil {
		return fmt.Errorf("parse database path: %w", err)
	}
	if !ok {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	return nil
}

func databaseFilePath(dsn string) (string, bool, error) {
	raw := strings.TrimSpace(strings.SplitN(dsn, "?", 2)[0])
	if raw == "" || raw == ":memory:" || strings.Contains(raw, "mode=memory") {
		return "", false, nil
	}
	if strings.HasPrefix(raw, "file:") {
		raw = strings.TrimPrefix(raw, "file:")
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", false, err
	}
	if decoded == "" || decoded == ":memory:" {
		return "", false, nil
	}
	return decoded, true, nil
}
