package directorypicker

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCleanSelectionCanonicalizesDirectory(t *testing.T) {
	directory := t.TempDir()
	selected, err := cleanSelection(directory + string(filepath.Separator) + "\n")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	if selected != want {
		t.Fatalf("selected = %q, want %q", selected, want)
	}
}

func TestCleanSelectionRejectsCancellationAndFiles(t *testing.T) {
	if _, err := cleanSelection(""); !errors.Is(err, ErrCancelled) {
		t.Fatalf("empty selection error = %v", err)
	}
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanSelection(file); err == nil {
		t.Fatal("file selection unexpectedly succeeded")
	}
}

func TestCleanSelectionPreservesPathWhitespace(t *testing.T) {
	directoryName := " spaced directory "
	if runtime.GOOS == "windows" {
		// Win32 normalizes trailing spaces in ordinary filesystem paths.
		directoryName = " spaced directory"
	}
	directory := filepath.Join(t.TempDir(), directoryName)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	selected, err := cleanSelection(directory + "\n")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	if selected != want {
		t.Fatalf("selected = %q, want %q", selected, want)
	}
	if initial := usableInitialPath(directory); initial != directory {
		t.Fatalf("initial path = %q, want %q", initial, directory)
	}
}
