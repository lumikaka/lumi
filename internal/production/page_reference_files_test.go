package production

import (
	"bytes"
	"os"
	"sync"
	"testing"
)

func TestPageReferenceFileRecovery(t *testing.T) {
	files := new(PageReferenceFiles)
	t.Cleanup(files.Close)
	data := []byte("complete reference")
	save := func(project string) string {
		t.Helper()
		path, _, err := files.save(project, data)
		if err != nil {
			t.Fatal(err)
		}
		return path
	}

	path := save("project")
	if other := save("another project"); other == path {
		t.Fatal("two projects shared a reference file")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	recreated := save("project")
	if recreated == path {
		t.Fatal("recreated file reused the missing path")
	}
	if err := os.WriteFile(recreated, bytes.Repeat([]byte{'X'}, len(data)), 0600); err != nil {
		t.Fatal(err)
	}
	repaired := save("project")
	if repaired == recreated {
		t.Fatal("same-sized corrupt file reused its path")
	}
	got, err := os.ReadFile(repaired)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("incorrect repaired reference: %q, %v", got, err)
	}
	files.Close()
	if _, err := os.Stat(repaired); !os.IsNotExist(err) {
		t.Fatalf("temporary reference survived cleanup: %v", err)
	}
}

func TestConcurrentPageReferencesReturnOnePath(t *testing.T) {
	files := new(PageReferenceFiles)
	t.Cleanup(files.Close)
	var wg sync.WaitGroup
	paths := make([]string, 8)
	for i := range paths {
		wg.Go(func() {
			path, _, err := files.save("project", []byte("reference"))
			if err != nil {
				t.Error(err)
			}
			paths[i] = path
		})
	}
	wg.Wait()
	for _, path := range paths {
		if path != paths[0] {
			t.Fatal("concurrent preparations published multiple paths")
		}
	}
}
