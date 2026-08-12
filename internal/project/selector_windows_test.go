//go:build windows

package project

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvedDefaultComparisonIgnoresWindowsPathCase(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "Documents", "Lumi")
	previous := resolveDefaultProjectParentDir
	resolveDefaultProjectParentDir = func() (string, error) { return parent, nil }
	t.Cleanup(func() { resolveDefaultProjectParentDir = previous })

	selected, err := ExplicitNewProjectParent(strings.ToUpper(parent)).SelectNewProjectParent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := normalizeDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(selected, canonical) {
		t.Fatalf("selected parent = %q, want %q", selected, canonical)
	}
}
