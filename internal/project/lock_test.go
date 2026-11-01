package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProjectLockOwnerIsReadableWhileHeld(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".lumi"), 0o700); err != nil {
		t.Fatal(err)
	}
	projectUUID := "019a3e81-0000-7000-8000-000000000001"
	lock, err := acquireProjectLock(root, projectUUID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	reader, err := os.Open(filepath.Join(root, ".lumi", "project.lock"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := readLockMetadata(reader)
	_ = reader.Close()
	if metadata.PID != os.Getpid() || metadata.ProjectUUID != projectUUID {
		t.Fatalf("owner metadata unavailable while locked: %+v", metadata)
	}
	other, err := acquireProjectLock(root, projectUUID, time.Now())
	if other != nil {
		_ = other.Close()
		t.Fatal("second handle acquired an owned project lock")
	}
	var conflict *Error
	if !errors.As(err, &conflict) || conflict.Code != CodeLocked || !strings.Contains(conflict.Details, fmt.Sprint(os.Getpid())) {
		t.Fatalf("lock conflict did not report the owner: %v", err)
	}
}

func TestReadLegacyLockMetadata(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "project.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	want := lockMetadata{PID: 123, Hostname: "legacy-host"}
	if err := json.NewEncoder(file).Encode(want); err != nil {
		t.Fatal(err)
	}
	if got := readLockMetadata(file); got != want {
		t.Fatalf("legacy metadata = %+v, want %+v", got, want)
	}
}
