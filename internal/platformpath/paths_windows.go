//go:build windows

package platformpath

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

var knownFolderPath = windows.KnownFolderPath

func DefaultAppDataDir(environment string) (string, error) {
	base, err := knownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return "", fmt.Errorf("resolve Windows LocalAppData known folder: %w", err)
	}
	return windowsAppDataDir(base, environment), nil
}

func DefaultProjectParentDir() (string, error) {
	documents, err := knownFolderPath(windows.FOLDERID_Documents, 0)
	if err != nil {
		return "", fmt.Errorf("resolve Windows Documents known folder: %w", err)
	}
	return defaultProjectParentDir(documents), nil
}

func CJKFontPaths() []string {
	directories := make([]string, 0, 2)
	if systemFonts, err := knownFolderPath(windows.FOLDERID_Fonts, 0); err == nil {
		directories = append(directories, systemFonts)
	}
	if localAppData, err := knownFolderPath(windows.FOLDERID_LocalAppData, 0); err == nil {
		directories = append(directories, filepath.Join(localAppData, "Microsoft", "Windows", "Fonts"))
	}
	return fontPaths(directories, windowsCJKFontNames)
}
