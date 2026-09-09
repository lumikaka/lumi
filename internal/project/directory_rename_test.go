package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRenameDirectoryNoReplacePreservesExistingTargets(t *testing.T) {
	for _, targetType := range []string{"file", "directory"} {
		t.Run(targetType, func(t *testing.T) {
			parent := t.TempDir()
			source := filepath.Join(parent, "source")
			target := filepath.Join(parent, "target")
			if err := os.Mkdir(source, 0700); err != nil {
				t.Fatal(err)
			}
			sourceFile := filepath.Join(source, "keep.txt")
			if err := os.WriteFile(sourceFile, []byte("source contents"), 0600); err != nil {
				t.Fatal(err)
			}
			targetFile := target
			if targetType == "directory" {
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				targetFile = filepath.Join(target, "keep.txt")
			}
			if err := os.WriteFile(targetFile, []byte("target contents"), 0600); err != nil {
				t.Fatal(err)
			}

			if err := renameDirectoryNoReplace(source, target); !errors.Is(err, os.ErrExist) {
				t.Fatalf("rename over existing %s: got %v, want os.ErrExist", targetType, err)
			}
			for path, want := range map[string]string{sourceFile: "source contents", targetFile: "target contents"} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Fatalf("read %s: got %q, err=%v, want %q", path, got, err, want)
				}
			}
		})
	}
}
