//go:build linux

package directorypicker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
)

func pickNativeDirectory(ctx context.Context, initialPath string) (string, error) {
	if _, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--directory", "--title=Select a Lumi project folder"}
		if initialPath != "" {
			args = append(args, "--filename="+filepath.Clean(initialPath)+string(filepath.Separator))
		}
		return runLinuxPicker(ctx, "zenity", args...)
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		args := []string{"--getexistingdirectory", initialPath, "--title", "Select a Lumi project folder"}
		return runLinuxPicker(ctx, "kdialog", args...)
	}
	return "", ErrUnavailable
}

func runLinuxPicker(ctx context.Context, name string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err == nil {
		return string(output), nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", ErrCancelled
	}
	return "", fmt.Errorf("open Linux directory picker: %w", err)
}
