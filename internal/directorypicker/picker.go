package directorypicker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrBusy        = errors.New("directory picker is already open")
	ErrCancelled   = errors.New("directory selection was cancelled")
	ErrUnavailable = errors.New("directory picker is unavailable")
)

type Native struct {
	active chan struct{}
}

func NewNative() *Native {
	return &Native{active: make(chan struct{}, 1)}
}

func (picker *Native) Pick(ctx context.Context, initialPath string) (string, error) {
	select {
	case picker.active <- struct{}{}:
		defer func() { <-picker.active }()
	default:
		return "", ErrBusy
	}

	selected, err := pickNativeDirectory(ctx, usableInitialPath(initialPath))
	if err != nil {
		return "", err
	}
	return cleanSelection(selected)
}

func usableInitialPath(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	absolute, err := filepath.Abs(raw)
	if err != nil {
		return ""
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return ""
	}
	return absolute
}

func cleanSelection(raw string) (string, error) {
	path := strings.TrimSuffix(raw, "\r\n")
	path = strings.TrimSuffix(path, "\n")
	if path == "" {
		return "", ErrCancelled
	}
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("selected directory contains a null byte")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve selected directory: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve selected directory symlinks: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect selected directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("selected path is not a directory")
	}
	return filepath.Clean(canonical), nil
}
