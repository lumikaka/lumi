package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newDirectoryNamingDraft(t *testing.T, manager *Manager) Summary {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "Lumi")
	previous := resolveDefaultProjectParentDir
	resolveDefaultProjectParentDir = func() (string, error) { return parent, nil }
	t.Cleanup(func() { resolveDefaultProjectParentDir = previous })
	root, err := manager.PlanDraftProjectRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.CreateDraftAt(context.Background(), DraftCreateInput{
		ProjectUUID: setupTestUUID(t), SetupUUID: setupTestUUID(t), RootPath: root, InitialInput: "创建一个小火车的故事",
	})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func finalizeDirectoryNamingDraft(t *testing.T, manager *Manager, projectUUID string) {
	t.Helper()
	ctx := context.Background()
	store := openStoreForTest(t, manager, projectUUID)
	name, style := "勇敢的小火车", "水彩"
	state, err := store.UpdateProjectSetupDraft(ctx, SetupDraftPatchInput{
		ExpectedRevision: 1, ProjectName: &name, OverallStyle: &style, PictureBook: &PictureBookInput{Format: PictureBookClassic},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinalizeProjectSetup(ctx, state.Revision); err != nil {
		t.Fatal(err)
	}
	if err := manager.SyncProjectName(ctx, projectUUID); err != nil {
		t.Fatal(err)
	}
}

func TestDraftDirectoryNamingWaitsForWorkAndLeasesThenPreservesIdentity(t *testing.T) {
	ctx := context.Background()
	manager, app := testManager(t)
	created := newDirectoryNamingDraft(t, manager)
	other, err := manager.Create(ctx, "Other", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	otherStore := openStoreForTest(t, manager, other.UUID)
	finalizeDirectoryNamingDraft(t, manager, created.UUID)
	parent := filepath.Dir(created.RootPath)
	// Both files and directories occupy candidate names, including a conflict
	// that appeared after the preview was read.
	if err := os.WriteFile(filepath.Join(parent, "勇敢的小火车"), []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := manager.PreviewDirectoryName(ctx, created.UUID, "勇敢的小火车")
	if err != nil || preview.RootPath != filepath.Join(parent, "勇敢的小火车-2") {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	if err := os.Mkdir(preview.RootPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(created.RootPath, "keep.txt"), []byte("project file"), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := &lifecycleRuntime{busy: true}
	manager.WithRuntime(runtime)
	if err := manager.ApplyPendingDirectoryRenames(ctx); err != nil {
		t.Fatal(err)
	}
	if len(runtime.stoppedFor) != 0 {
		t.Fatal("stopped busy runtime")
	}
	runtime.busy = false
	if err := manager.WithStore(ctx, created.UUID, func(*Store) error { return manager.ApplyPendingDirectoryRenames(ctx) }); err != nil {
		t.Fatal(err)
	}
	if len(runtime.stoppedFor) != 0 {
		t.Fatal("stopped leased store")
	}
	release, err := manager.AcquirePresence(created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := manager.ApplyPendingDirectoryRenames(ctx); err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(parent, "勇敢的小火车-3")
	recent, err := app.RecentProject(ctx, created.UUID)
	if err != nil || recent.RootPath != wantRoot || recent.Name != "勇敢的小火车" || recent.PendingDirectoryName != "" || recent.AutoNameDirectory {
		t.Fatalf("recent=%+v err=%v", recent, err)
	}
	if _, err := os.Stat(created.RootPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old root remains: %v", err)
	}
	for path, expected := range map[string]string{filepath.Join(parent, "勇敢的小火车"): "untouched", filepath.Join(wantRoot, "keep.txt"): "project file"} {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != expected {
			t.Fatalf("file %s=%q err=%v", path, content, err)
		}
	}
	store := openStoreForTest(t, manager, created.UUID)
	if store.Root() != wantRoot || store.ProjectUUID() != created.UUID || store.SetupStatus() != SetupStatusReady {
		t.Fatal("reopened wrong project")
	}
	if otherStore != openStoreForTest(t, manager, other.UUID) || len(runtime.stoppedFor) != 1 || runtime.stoppedFor[0] != created.UUID {
		t.Fatal("changed another project")
	}
	activity, _ := manager.Activity(created.UUID)
	if activity.PresenceLeases != 1 {
		t.Fatalf("lost presence: %+v", activity)
	}
	if err := manager.SyncProjectName(ctx, created.UUID); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyPendingDirectoryRenames(ctx); err != nil {
		t.Fatal(err)
	}
	if len(runtime.stoppedFor) != 1 {
		t.Fatal("renamed twice")
	}
}

func TestDirectoryNamingLeavesHistoryAndImportedDraftsAlone(t *testing.T) {
	ctx := context.Background()
	manager, app := testManager(t)
	historical, err := manager.Create(ctx, "Lumi Draft", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SyncProjectName(ctx, historical.UUID); err != nil {
		t.Fatal(err)
	}
	created := newDirectoryNamingDraft(t, manager)
	if _, err := manager.CloseProject(ctx, created.UUID); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(t.TempDir(), "User chosen folder")
	if err := os.Rename(created.RootPath, moved); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.OpenSelected(ctx, ExplicitExistingDirectory(moved)); err != nil {
		t.Fatal(err)
	}
	moved, err = normalizeDirectory(moved)
	if err != nil {
		t.Fatal(err)
	}
	finalizeDirectoryNamingDraft(t, manager, created.UUID)
	if err := manager.ApplyPendingDirectoryRenames(ctx); err != nil {
		t.Fatal(err)
	}
	for projectUUID, root := range map[string]string{historical.UUID: historical.RootPath, created.UUID: moved} {
		recent, err := app.RecentProject(ctx, projectUUID)
		if err != nil || recent.RootPath != root || recent.AutoNameDirectory || recent.PendingDirectoryName != "" {
			t.Fatalf("unexpected naming %+v err=%v", recent, err)
		}
	}
}

func TestExplicitDirectoryRenameCanBeCancelledAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	manager, app := testManager(t)
	created, err := manager.Create(ctx, "Original", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetDirectoryRename(ctx, created.UUID, "New", true); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetDirectoryRename(ctx, created.UUID, "New", false); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyPendingDirectoryRenames(ctx); err != nil {
		t.Fatal(err)
	}
	if openStoreForTest(t, manager, created.UUID).Root() != created.RootPath {
		t.Fatal("renamed without option")
	}
	if err := manager.SetDirectoryRename(ctx, created.UUID, "New", true); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CloseProject(ctx, created.UUID); err != nil {
		t.Fatal(err)
	}
	restarted := NewManager(app)
	t.Cleanup(func() { _ = restarted.Close() })
	if _, err := restarted.OpenRecent(ctx, created.UUID); err != nil {
		t.Fatal(err)
	}
	if err := restarted.ApplyPendingDirectoryRenames(ctx); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(openStoreForTest(t, restarted, created.UUID).Root()) != "New" {
		t.Fatal("lost pending rename on restart")
	}
}

func TestDirectoryRenameRecoversFilesystemIndexGap(t *testing.T) {
	ctx := context.Background()
	manager, app := testManager(t)
	created, err := manager.Create(ctx, "Original", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetDirectoryRename(ctx, created.UUID, "New", true); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CloseProject(ctx, created.UUID); err != nil {
		t.Fatal(err)
	}
	newRoot, err := moveProjectDirectory(created.RootPath, "New")
	if err != nil {
		t.Fatal(err)
	}
	opened, err := manager.OpenRecent(ctx, created.UUID)
	if err != nil || opened.RootPath != newRoot {
		t.Fatalf("opened=%+v err=%v", opened, err)
	}
	recent, err := app.RecentProject(ctx, created.UUID)
	if err != nil || recent.RootPath != newRoot || recent.PendingDirectoryName != "" {
		t.Fatalf("recent=%+v err=%v", recent, err)
	}
}
