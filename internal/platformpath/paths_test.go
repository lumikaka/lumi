package platformpath

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestWindowsAppDataDirSeparatesEnvironments(t *testing.T) {
	base := filepath.Join("root", "local-app-data")
	if got := windowsAppDataDir(base, "production"); got != filepath.Join(base, windowsProductionDataDirectory) {
		t.Fatalf("production data directory = %q", got)
	}
	if got := windowsAppDataDir(base, "TEST"); got != filepath.Join(base, windowsDevelopmentDataDirectory) {
		t.Fatalf("development data directory = %q", got)
	}
}

func TestDefaultProjectParentDirAppendsLumi(t *testing.T) {
	documents := filepath.Join("home", "Documents")
	if got := defaultProjectParentDir(documents); got != filepath.Join(documents, "Lumi") {
		t.Fatalf("default project parent = %q", got)
	}
}

func TestFontPathsPreserveDirectoryAndCandidateOrder(t *testing.T) {
	directories := []string{filepath.Join("system", "fonts"), filepath.Join("user", "fonts")}
	names := []string{"first.ttc", "second.ttf"}
	want := []string{
		filepath.Join(directories[0], names[0]),
		filepath.Join(directories[0], names[1]),
		filepath.Join(directories[1], names[0]),
		filepath.Join(directories[1], names[1]),
	}
	if got := fontPaths(directories, names); !reflect.DeepEqual(got, want) {
		t.Fatalf("font paths = %#v, want %#v", got, want)
	}
}
