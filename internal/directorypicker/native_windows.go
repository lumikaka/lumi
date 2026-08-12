//go:build windows

package directorypicker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

const chooseFolderPowerShell = `$shell = New-Object -ComObject Shell.Application
$folder = $shell.BrowseForFolder(0, 'Select a Lumi project folder', 0, $env:LUMI_DIRECTORY_PICKER_INITIAL_PATH)
if ($null -ne $folder) {
  [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new()
  Write-Output $folder.Self.Path
}`

func pickNativeDirectory(ctx context.Context, initialPath string) (string, error) {
	command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-Command", chooseFolderPowerShell)
	command.Env = append(os.Environ(), "LUMI_DIRECTORY_PICKER_INITIAL_PATH="+initialPath)
	output, err := command.CombinedOutput()
	if err == nil {
		return string(output), nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "", ErrUnavailable
	}
	return "", fmt.Errorf("open Windows directory picker: %w", err)
}
