package project

import (
	"context"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"lumi/internal/dbmigrate"
)

func directorySnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := info.Mode().String()
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += stringHash(data)
		}
		result[relative] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func stringHash(data []byte) string { sum := sha256.Sum256(data); return string(sum[:]) }

func TestInitAtEmptyAndExistingUserFiles(t *testing.T) {
	for _, withFiles := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "git-and-user-files"}[withFiles], func(t *testing.T) {
			manager, app := testManager(t)
			root := t.TempDir()
			if withFiles {
				for _, directory := range []string{".git", ".codex"} {
					if err := os.Mkdir(filepath.Join(root, directory), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				for _, name := range []string{".git/config", ".codex/config.toml", "AGENTS.md", "notes.txt"} {
					if err := os.WriteFile(filepath.Join(root, name), []byte("user-owned"), 0o640); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := directorySnapshot(t, root)
			result, err := manager.InitAt(t.Context(), root, CreateInput{Name: "原地项目", GenerationLanguage: "en-US"})
			if err != nil || !result.Created || !result.Open || !isUUIDv7(result.UUID) || result.Name != "原地项目" {
				t.Fatalf("init=%+v err=%v", result, err)
			}
			canonical, _ := normalizeDirectory(root)
			if result.RootPath != canonical {
				t.Fatalf("root=%s", result.RootPath)
			}
			after := directorySnapshot(t, root)
			for name, value := range before {
				if after[name] != value {
					t.Fatalf("changed existing %s", name)
				}
			}
			store := openStoreForTest(t, manager, result.UUID)
			var projectCount, actorCount int64
			if err := store.DB().Model(&Project{}).Count(&projectCount).Error; err != nil {
				t.Fatal(err)
			}
			if err := store.DB().Model(&Actor{}).Where("kind='local_user'").Count(&actorCount).Error; err != nil {
				t.Fatal(err)
			}
			if projectCount != 1 || actorCount != 1 || store.project.GenerationLanguage != "en" || store.project.FormatVersion != SupportedFormatVersion {
				t.Fatalf("invalid initialized state: %+v %d %d", store.project, projectCount, actorCount)
			}
			recent, err := app.RecentProject(t.Context(), result.UUID)
			if err != nil || recent.RootPath != canonical {
				t.Fatalf("recent=%+v err=%v", recent, err)
			}
			if _, err := manager.CloseProject(t.Context(), result.UUID); err != nil {
				t.Fatal(err)
			}
			before = directorySnapshot(t, root)
			// Repeated init must not rename, change language, reopen, run hooks,
			// or update any project file or recent timestamp.
			manager.WithOpenHook(func(context.Context, *Store) error { t.Fatal("existing init ran hook"); return nil })
			again, err := manager.InitAt(t.Context(), root, CreateInput{Name: "ignored", GenerationLanguage: "invalid"})
			if err != nil || again.Created || again.Open || again.UUID != result.UUID || again.Name != result.Name {
				t.Fatalf("again=%+v err=%v", again, err)
			}
			if after := directorySnapshot(t, root); !reflect.DeepEqual(before, after) {
				for path, value := range after {
					if before[path] != value {
						t.Logf("changed: %s", path)
					}
				}
				t.Fatal("idempotent init mutated project")
			}
			recentAfter, err := app.RecentProject(t.Context(), result.UUID)
			if err != nil || !reflect.DeepEqual(recent, recentAfter) {
				t.Fatalf("recent changed: %v", err)
			}
		})
	}
}

func TestInitAtConflictsDoNotChangeDirectory(t *testing.T) {
	manager, _ := testManager(t)
	for _, name := range []string{"README.md", "STORY.md", "project.sqlite", "project.sqlite-wal", "project.sqlite-shm", "project.sqlite-journal", ".lumi", "assets"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, name), []byte("not a Lumi project"), 0o640); err != nil {
				t.Fatal(err)
			}
			before := directorySnapshot(t, root)
			_, err := manager.InitAt(t.Context(), root, CreateInput{})
			if errorCode(err) != CodeInitializationConflict || !strings.Contains(err.Error(), name) {
				t.Fatalf("error=%v", err)
			}
			if !reflect.DeepEqual(before, directorySnapshot(t, root)) {
				t.Fatal("conflict mutated directory")
			}
		})
	}
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "absent"), filepath.Join(root, "README.md")); err != nil {
		t.Skip(err)
	}
	if _, err := manager.InitAt(t.Context(), root, CreateInput{}); errorCode(err) != CodeInitializationConflict {
		t.Fatalf("symlink error=%v", err)
	}
}

func TestInitAtRollbackPreservesUserFiles(t *testing.T) {
	for _, phase := range []string{"migration", "hook", "recent-index"} {
		t.Run(phase, func(t *testing.T) {
			manager, app := testManager(t)
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("keep"), 0o640); err != nil {
				t.Fatal(err)
			}
			before := directorySnapshot(t, root)
			ctx := t.Context()
			switch phase {
			case "migration":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "hook":
				manager.WithOpenHook(func(context.Context, *Store) error { return errors.New("forced hook failure") })
			case "recent-index":
				if err := app.DB().Exec(`CREATE TRIGGER reject_init BEFORE INSERT ON recent_projects BEGIN SELECT RAISE(FAIL, 'forced recent failure'); END`).Error; err != nil {
					t.Fatal(err)
				}
			}
			if _, err := manager.InitAt(ctx, root, CreateInput{}); err == nil {
				t.Fatal("expected failure")
			}
			if !reflect.DeepEqual(before, directorySnapshot(t, root)) {
				t.Fatalf("rollback left or removed files: before=%v after=%v", before, directorySnapshot(t, root))
			}
			if len(manager.OpenProjectUUIDs()) != 0 {
				t.Fatal("failed init still open")
			}
		})
	}
}

func TestInitializationLayoutRollbackPreservesConcurrentNodes(t *testing.T) {
	root := t.TempDir()
	layout := &initializationLayout{root: root}
	if err := layout.create("test"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "user.txt"), []byte("concurrent user file"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A concurrent writer can replace a node we created; rollback must not
	// unlink the replacement merely because its pathname matches our ledger.
	replacement := filepath.Join(root, "replacement")
	if err := os.WriteFile(replacement, []byte("user readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, filepath.Join(root, "README.md")); err != nil {
		t.Fatal(err)
	}
	_ = layout.cleanup() // Nonempty assets is deliberately retained.
	for _, name := range []string{"README.md", "assets/user.txt"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "project.sqlite")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("database remains: %v", err)
	}
}

func TestInitAtDesktopOpenValidatesIdentityLockAndMigration(t *testing.T) {
	manager, app := testManager(t)
	initialized, err := manager.InitAt(t.Context(), t.TempDir(), CreateInput{})
	if err != nil {
		t.Fatal(err)
	}
	desktop := NewManager(app)
	defer desktop.Close()
	if _, err := desktop.OpenSelected(t.Context(), ExplicitExistingDirectory(initialized.RootPath)); errorCode(err) != CodeLocked {
		t.Fatalf("lock error=%v", err)
	}
	if _, err := manager.CloseProject(t.Context(), initialized.UUID); err != nil {
		t.Fatal(err)
	}
	opened, err := desktop.OpenSelected(t.Context(), ExplicitExistingDirectory(initialized.RootPath))
	if err != nil || opened.UUID != initialized.UUID {
		t.Fatalf("desktop open=%+v err=%v", opened, err)
	}
	store := openStoreForTest(t, desktop, initialized.UUID)
	runner, err := dbmigrate.OpenProject(projectDSN(initialized.RootPath))
	if err != nil {
		t.Fatal(err)
	}
	needsUp, err := runner.NeedsUp()
	latest := runner.LatestVersion()
	_ = runner.Close()
	if err != nil || needsUp || store.project.SchemaVersion != int64(latest) {
		t.Fatalf("migration needsUp=%v schema=%d latest=%d err=%v", needsUp, store.project.SchemaVersion, latest, err)
	}
	assertSQLiteHealthy(t, store)
	if err := store.DB().Exec("UPDATE projects SET uuid='not-a-uuid'").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := desktop.CloseProject(t.Context(), initialized.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := desktop.OpenSelected(t.Context(), ExplicitExistingDirectory(initialized.RootPath)); errorCode(err) != CodeInvalidProject {
		t.Fatalf("identity error=%v", err)
	}
}

func TestInspectDirectoryReadsUncheckpointedWAL(t *testing.T) {
	const helperEnv = "LUMI_INIT_WAL_TEST_ROOT"
	if root := os.Getenv(helperEnv); root != "" {
		manager, _ := testManager(t)
		result, err := manager.InitAt(t.Context(), root, CreateInput{Name: "before"})
		if err != nil {
			t.Fatal(err)
		}
		store := openStoreForTest(t, manager, result.UUID)
		if err := store.DB().Exec("UPDATE projects SET name='WAL name'").Error; err != nil {
			t.Fatal(err)
		}
		os.Exit(0) // Simulate process exit without SQLite shutdown/checkpoint.
	}
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestInspectDirectoryReadsUncheckpointedWAL$")
	cmd.Env = append(os.Environ(), helperEnv+"="+root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture=%s err=%v", output, err)
	}
	before := directorySnapshot(t, root)
	info, err := InspectDirectory(t.Context(), root)
	if err != nil || info.Name != "WAL name" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	if !reflect.DeepEqual(before, directorySnapshot(t, root)) {
		t.Fatal("inspection changed crashed project")
	}
}
