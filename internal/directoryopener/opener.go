package directoryopener

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var (
	ErrNotFound     = errors.New("directory does not exist")
	ErrNotDirectory = errors.New("path is not a directory")
	ErrUnavailable  = errors.New("directory opener is unavailable")
)

type Opener interface {
	Open(context.Context, string) error
}

type Native struct {
	openDirectory func(context.Context, string) error
}

func NewNative() *Native {
	return &Native{openDirectory: openNativeDirectory}
}

func (opener *Native) Open(ctx context.Context, rootPath string) error {
	resolved, err := resolveDirectory(rootPath)
	if err != nil {
		return err
	}
	if opener == nil || opener.openDirectory == nil {
		return ErrUnavailable
	}
	return opener.openDirectory(ctx, resolved)
}

func resolveDirectory(rootPath string) (string, error) {
	absolute, err := filepath.Abs(rootPath)
	if err != nil {
		return "", fmt.Errorf("resolve directory path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, absolute)
		}
		return "", fmt.Errorf("resolve directory symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, resolved)
		}
		return "", fmt.Errorf("inspect directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s", ErrNotDirectory, resolved)
	}
	return filepath.Clean(resolved), nil
}

func runNativeCommand(ctx context.Context, name string, args ...string) error {
	if _, err := exec.LookPath(name); err != nil {
		return ErrUnavailable
	}
	if err := exec.CommandContext(ctx, name, args...).Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("open directory with %s: %w", name, err)
	}
	return nil
}
