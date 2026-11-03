package project

import (
	"context"
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

func TestLockedDetailsGuideByOwner(t *testing.T) {
	cases := map[string]string{
		LockOwnerCLI:     "lumi stop",
		LockOwnerDesktop: "桌面版中关闭该项目",
		LockOwnerWeb:     "Web 服务",
		"":               "执行 lumi stop",
	}
	for owner, want := range cases {
		details := lockedDetails(lockMetadata{PID: 42, Hostname: "host", Owner: owner})
		if !strings.Contains(details, want) || !strings.Contains(details, "42") {
			t.Fatalf("owner %q details = %q, want %q", owner, details, want)
		}
	}
}

func TestLockStatusReportsOwnerKind(t *testing.T) {
	t.Cleanup(func() { SetLockOwner("") })
	ctx := context.Background()
	SetLockOwner(LockOwnerCLI)
	first, store := testManager(t)
	created, err := first.Create(ctx, "Owned", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	local, err := first.LockStatus(ctx, created.UUID)
	if err != nil || !local.Locked || !local.HeldByCurrentInstance || local.Owner.Kind != LockOwnerCLI {
		t.Fatalf("owner instance status = %+v error=%v", local, err)
	}

	SetLockOwner(LockOwnerDesktop)
	second := NewManager(store)
	t.Cleanup(func() { _ = second.Close() })
	_, err = second.OpenRecent(ctx, created.UUID)
	var conflict *Error
	if !errors.As(err, &conflict) || conflict.Code != CodeLocked || !strings.Contains(conflict.Details, "Lumi CLI") {
		t.Fatalf("open conflict = %v", err)
	}
	remote, err := second.LockStatus(ctx, created.UUID)
	if err != nil || !remote.Locked || remote.HeldByCurrentInstance || remote.Owner == nil ||
		remote.Owner.Kind != LockOwnerCLI || remote.Owner.PID != os.Getpid() || remote.Owner.AcquiredAt == nil ||
		!strings.Contains(remote.ReleaseDetails, "lumi stop") {
		t.Fatalf("remote status = %+v owner=%+v error=%v", remote, remote.Owner, err)
	}

	if _, err := first.CloseProject(ctx, created.UUID); err != nil {
		t.Fatal(err)
	}
	released, err := second.LockStatus(ctx, created.UUID)
	if err != nil || released.Locked || released.Owner != nil {
		t.Fatalf("released status = %+v error=%v", released, err)
	}
	// The probe must not leave the lock held.
	if _, err := second.OpenRecent(ctx, created.UUID); err != nil {
		t.Fatalf("open after probe: %v", err)
	}
	if _, err := second.LockStatus(ctx, "not-a-uuid"); errorCode(err) != CodeInvalidUUID {
		t.Fatalf("invalid uuid error = %v", err)
	}
}
