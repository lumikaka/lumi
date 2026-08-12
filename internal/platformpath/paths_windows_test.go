//go:build windows

package platformpath

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsKnownFoldersDriveApplicationProjectAndFontPaths(t *testing.T) {
	previous := knownFolderPath
	t.Cleanup(func() { knownFolderPath = previous })
	localAppData := `D:\Profiles\Lumi\AppData\Local`
	documents := `E:\OneDrive\Documents`
	systemFonts := `F:\Windows\Fonts`
	knownFolderPath = func(folderID *windows.KNOWNFOLDERID, _ uint32) (string, error) {
		switch folderID {
		case windows.FOLDERID_LocalAppData:
			return localAppData, nil
		case windows.FOLDERID_Documents:
			return documents, nil
		case windows.FOLDERID_Fonts:
			return systemFonts, nil
		default:
			return "", errors.New("unexpected known folder")
		}
	}

	production, err := DefaultAppDataDir("production")
	if err != nil || production != filepath.Join(localAppData, windowsProductionDataDirectory) {
		t.Fatalf("production data directory = %q, error = %v", production, err)
	}
	development, err := DefaultAppDataDir("development")
	if err != nil || development != filepath.Join(localAppData, windowsDevelopmentDataDirectory) {
		t.Fatalf("development data directory = %q, error = %v", development, err)
	}
	parent, err := DefaultProjectParentDir()
	if err != nil || parent != filepath.Join(documents, "Lumi") {
		t.Fatalf("project parent = %q, error = %v", parent, err)
	}

	wantFonts := fontPaths([]string{systemFonts, filepath.Join(localAppData, "Microsoft", "Windows", "Fonts")}, windowsCJKFontNames)
	if got := CJKFontPaths(); !reflect.DeepEqual(got, wantFonts) {
		t.Fatalf("font paths = %#v, want %#v", got, wantFonts)
	}
}

func TestWindowsKnownFolderFailuresAreExplicit(t *testing.T) {
	previous := knownFolderPath
	t.Cleanup(func() { knownFolderPath = previous })
	knownFolderPath = func(*windows.KNOWNFOLDERID, uint32) (string, error) {
		return "", errors.New("known folder unavailable")
	}

	if _, err := DefaultAppDataDir("production"); err == nil || !strings.Contains(err.Error(), "LocalAppData") {
		t.Fatalf("app data error = %v", err)
	}
	if _, err := DefaultProjectParentDir(); err == nil || !strings.Contains(err.Error(), "Documents") {
		t.Fatalf("project parent error = %v", err)
	}
	if got := CJKFontPaths(); len(got) != 0 {
		t.Fatalf("font paths after known-folder failure = %#v", got)
	}
}
