//go:build !windows

package platformpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultAppDataDirKeepsHomeBasedLayout(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DefaultAppDataDir("development"); err != nil || got != filepath.Join(home, ".lumi_dev") {
		t.Fatalf("development data directory = %q, error = %v", got, err)
	}
	if got, err := DefaultAppDataDir("PRODUCTION"); err != nil || got != filepath.Join(home, ".lumi") {
		t.Fatalf("production data directory = %q, error = %v", got, err)
	}
}

func TestDefaultProjectParentDirKeepsDocumentsLayout(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Documents", "Lumi")
	if got, err := DefaultProjectParentDir(); err != nil || got != want {
		t.Fatalf("default project parent = %q, want %q, error = %v", got, want, err)
	}
}
