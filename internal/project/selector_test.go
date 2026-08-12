package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDefaultNewProjectParentCreatesDocumentsLumi(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "Documents", "Lumi")
	selected, err := ensureDefaultNewProjectParent(parent)
	if err != nil {
		t.Fatal(err)
	}
	canonicalWant, err := normalizeDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	if selected != canonicalWant {
		t.Fatalf("selected parent = %q, want %q", selected, canonicalWant)
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		t.Fatalf("default parent info = %v, error = %v", info, err)
	}
}

func TestExplicitNewProjectParentRecognizesResolvedDefault(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "Redirected Documents", "Lumi")
	previous := resolveDefaultProjectParentDir
	resolveDefaultProjectParentDir = func() (string, error) { return parent, nil }
	t.Cleanup(func() { resolveDefaultProjectParentDir = previous })

	selected, err := ExplicitNewProjectParent(parent).SelectNewProjectParent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	canonicalParent, err := normalizeDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	if selected != canonicalParent {
		t.Fatalf("selected parent = %q, want %q", selected, canonicalParent)
	}
}

func TestDefaultNewProjectParentReportsResolutionFailureWithoutBlockingOverrides(t *testing.T) {
	previous := resolveDefaultProjectParentDir
	resolveDefaultProjectParentDir = func() (string, error) { return "", errors.New("documents unavailable") }
	t.Cleanup(func() { resolveDefaultProjectParentDir = previous })

	if _, err := (DefaultNewProjectParent{}).SelectNewProjectParent(context.Background()); errorCode(err) != CodeDefaultProjectParentUnavailable {
		t.Fatalf("default resolution error = %v", err)
	}
	override := t.TempDir()
	selected, err := ExplicitNewProjectParent(override).SelectNewProjectParent(context.Background())
	if err != nil {
		t.Fatalf("explicit override failed with unavailable default: %v", err)
	}
	canonicalOverride, err := normalizeDirectory(override)
	if err != nil {
		t.Fatal(err)
	}
	if selected != canonicalOverride {
		t.Fatalf("selected override = %q, want %q", selected, canonicalOverride)
	}
}

func TestDefaultNewProjectParentAliases(t *testing.T) {
	for _, path := range []string{"", "~/Documents/Lumi", "~/Documents/Lumi/"} {
		if !isDefaultNewProjectParent(path) {
			t.Fatalf("path %q was not recognized as the default", path)
		}
	}
	if isDefaultNewProjectParent("/tmp/Lumi") {
		t.Fatal("absolute override was recognized as the default")
	}
}

func TestExplicitNewProjectParentKeepsAbsoluteOverrides(t *testing.T) {
	parent := t.TempDir()
	selected, err := ExplicitNewProjectParent(parent).SelectNewProjectParent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	canonicalParent, err := normalizeDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	if selected != canonicalParent {
		t.Fatalf("selected parent = %q, want %q", selected, canonicalParent)
	}
}
