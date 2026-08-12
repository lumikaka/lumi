//go:build darwin

package directorypicker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const chooseFolderAppleScript = `
on run argv
  if (count of argv) is greater than 0 then
    set selectedFolder to choose folder with prompt "选择 Lumi 项目文件夹 / Select a Lumi project folder" default location (POSIX file (item 1 of argv))
  else
    set selectedFolder to choose folder with prompt "选择 Lumi 项目文件夹 / Select a Lumi project folder"
  end if
  return POSIX path of selectedFolder
end run
`

func pickNativeDirectory(ctx context.Context, initialPath string) (string, error) {
	args := []string{"-e", chooseFolderAppleScript}
	if initialPath != "" {
		args = append(args, "--", initialPath)
	}
	output, err := exec.CommandContext(ctx, "osascript", args...).CombinedOutput()
	if err == nil {
		return string(output), nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	details := string(output)
	if strings.Contains(details, "(-128)") || strings.Contains(strings.ToLower(details), "user canceled") {
		return "", ErrCancelled
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "", ErrUnavailable
	}
	return "", fmt.Errorf("open macOS directory picker: %w", err)
}
