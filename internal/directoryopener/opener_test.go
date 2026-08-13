package directoryopener

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeOpenResolvesAnExistingDirectory(t *testing.T) {
	directory := t.TempDir()
	var opened string
	opener := &Native{openDirectory: func(_ context.Context, rootPath string) error {
		opened = rootPath
		return nil
	}}

	if err := opener.Open(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	if opened != want {
		t.Fatalf("opened path = %q, want %q", opened, want)
	}
}

func TestNativeOpenRejectsMissingPathsAndFiles(t *testing.T) {
	opener := &Native{openDirectory: func(context.Context, string) error { return nil }}
	if err := opener.Open(context.Background(), filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing path error = %v", err)
	}
	file := filepath.Join(t.TempDir(), "project.txt")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := opener.Open(context.Background(), file); !errors.Is(err, ErrNotDirectory) {
		t.Fatalf("file path error = %v", err)
	}
}
