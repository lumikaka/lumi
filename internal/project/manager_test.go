package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/dbmigrate"

	"github.com/google/uuid"
)

func testManager(t *testing.T) (*Manager, *appstore.Store) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "app")
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	manager := NewManager(store)
	t.Cleanup(func() { _ = manager.Close() })
	return manager, store
}

func TestCreateCloseReopenAndRelocateProject(t *testing.T) {
	ctx := context.Background()
	manager, store := testManager(t)
	parent := t.TempDir()
	created, err := manager.Create(ctx, "月光书", ExplicitNewProjectParent(parent))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := uuid.Parse(created.UUID)
	if err != nil || parsed.Version() != 7 {
		t.Fatalf("project UUID = %q, error = %v", created.UUID, err)
	}
	for _, relative := range append([]string{"README.md", "STORY.md", "project.sqlite"}, projectDirectories...) {
		if _, err := os.Stat(filepath.Join(created.RootPath, relative)); err != nil {
			t.Fatalf("missing %s: %v", relative, err)
		}
	}
	projectStore := openStoreForTest(t, manager, created.UUID)
	var actorCount int64
	if err := projectStore.DB().Model(&Actor{}).Where("kind = ?", "local_user").Count(&actorCount).Error; err != nil || actorCount != 1 {
		t.Fatalf("actor count = %d, error = %v", actorCount, err)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}

	restarted := NewManager(store)
	t.Cleanup(func() { _ = restarted.Close() })
	opened, err := restarted.OpenRecent(ctx, created.UUID)
	if err != nil || opened.UUID != created.UUID {
		t.Fatalf("reopened = %+v, error = %v", opened, err)
	}
	if err := restarted.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	movedParent := t.TempDir()
	movedRoot := filepath.Join(movedParent, "moved-project")
	if err := os.Rename(created.RootPath, movedRoot); err != nil {
		t.Fatal(err)
	}
	items, err := restarted.RecentProjects(ctx)
	if err != nil || len(items) != 1 || !items[0].Available || items[0].Status != "recent" || items[0].StatusDetail != "" {
		t.Fatalf("missing recent item = %+v, error = %v", items, err)
	}
	if _, err := restarted.OpenRecent(ctx, created.UUID); errorCode(err) != CodeProjectNotFound {
		t.Fatalf("open missing recent error = %v", err)
	}
	relocated, err := restarted.Relocate(ctx, created.UUID, ExplicitExistingDirectory(movedRoot))
	canonicalMovedRoot, canonicalErr := normalizeDirectory(movedRoot)
	if err != nil || canonicalErr != nil || relocated.RootPath != canonicalMovedRoot || relocated.UUID != created.UUID {
		t.Fatalf("relocated = %+v, error = %v", relocated, err)
	}
	assertSQLiteHealthy(t, openStoreForTest(t, restarted, created.UUID))
}

func TestManagerKeepsMultipleProjectsOpenAndClosesOnlyTheTarget(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	runtime := &lifecycleRuntime{}
	manager.WithRuntime(runtime)
	first, err := manager.Create(ctx, "First open project", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Create(ctx, "Second open project", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	items, err := manager.OpenProjects(ctx)
	if err != nil || len(items) != 2 || !items[0].Open || !items[1].Open {
		t.Fatalf("open projects = %+v, error = %v", items, err)
	}
	if manager.Current() != nil {
		t.Fatal("a multi-project manager exposed a global current project")
	}
	for _, expected := range []Summary{first, second} {
		if err := manager.WithStore(ctx, expected.UUID, func(store *Store) error {
			if store.ProjectUUID() != expected.UUID || store.Root() != expected.RootPath {
				t.Fatalf("store = %s %s, expected = %+v", store.ProjectUUID(), store.Root(), expected)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	closed, err := manager.CloseProject(ctx, first.UUID)
	if err != nil || !closed || manager.IsOpen(first.UUID) || !manager.IsOpen(second.UUID) {
		t.Fatalf("close first = %t, first open=%t second open=%t error=%v", closed, manager.IsOpen(first.UUID), manager.IsOpen(second.UUID), err)
	}
	if len(runtime.stoppedFor) != 1 || runtime.stoppedFor[0] != first.UUID {
		t.Fatalf("stopped runtimes = %v", runtime.stoppedFor)
	}
	if err := manager.WithStore(ctx, second.UUID, func(store *Store) error { return store.DB().Exec("SELECT 1").Error }); err != nil {
		t.Fatalf("second project was affected by first close: %v", err)
	}
}

func TestManagerMergesSameProjectOpenAndOpensDifferentProjectsConcurrently(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	first, err := manager.Create(ctx, "Concurrent first", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Create(ctx, "Concurrent second", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CloseProject(ctx, first.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CloseProject(ctx, second.UUID); err != nil {
		t.Fatal(err)
	}

	entered := make(chan string, 3)
	release := make(chan struct{})
	var countsMu sync.Mutex
	counts := make(map[string]int)
	manager.WithOpenHook(func(_ context.Context, store *Store) error {
		countsMu.Lock()
		counts[store.ProjectUUID()]++
		countsMu.Unlock()
		entered <- store.ProjectUUID()
		<-release
		return nil
	})

	type result struct {
		summary Summary
		err     error
	}
	results := make(chan result, 3)
	open := func(projectUUID string) {
		summary, err := manager.OpenRecent(ctx, projectUUID)
		results <- result{summary: summary, err: err}
	}
	go open(first.UUID)
	if opened := waitProjectOpenHook(t, entered); opened != first.UUID {
		t.Fatalf("first hook project = %s", opened)
	}
	go open(first.UUID)
	go open(second.UUID)
	if opened := waitProjectOpenHook(t, entered); opened != second.UUID {
		t.Fatalf("second concurrent hook project = %s", opened)
	}
	close(release)
	for range 3 {
		opened := <-results
		if opened.err != nil || (opened.summary.UUID != first.UUID && opened.summary.UUID != second.UUID) {
			t.Fatalf("concurrent open = %+v, error = %v", opened.summary, opened.err)
		}
	}
	countsMu.Lock()
	defer countsMu.Unlock()
	if counts[first.UUID] != 1 || counts[second.UUID] != 1 {
		t.Fatalf("open hook counts = %v", counts)
	}
}

func TestCloseProjectDrainsOnlyItsInFlightRequests(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.Create(ctx, "Lease drain", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	requestEntered := make(chan struct{})
	releaseRequest := make(chan struct{})
	requestDone := make(chan error, 1)
	go func() {
		requestDone <- manager.WithStore(ctx, created.UUID, func(*Store) error {
			close(requestEntered)
			<-releaseRequest
			return nil
		})
	}()
	<-requestEntered
	closeDone := make(chan error, 1)
	go func() {
		_, err := manager.CloseProject(ctx, created.UUID)
		closeDone <- err
	}()
	deadline := time.Now().Add(time.Second)
	for manager.IsOpen(created.UUID) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.IsOpen(created.UUID) {
		t.Fatal("project never entered draining state")
	}
	if err := manager.WithStore(ctx, created.UUID, func(*Store) error { return nil }); errorCode(err) != CodeProjectNotOpen {
		t.Fatalf("new request during drain error = %v", err)
	}
	select {
	case err := <-closeDone:
		t.Fatalf("close completed before request lease was released: %v", err)
	default:
	}
	close(releaseRequest)
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
}

type failFirstStopRuntime struct {
	mu    sync.Mutex
	calls int
}

func (runtime *failFirstStopRuntime) StopProject(context.Context, string) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.calls++
	if runtime.calls == 1 {
		return errors.New("runtime did not stop")
	}
	return nil
}

func (*failFirstStopRuntime) HasActiveWork(context.Context, string) (bool, error) {
	return false, nil
}

func TestFailedOpenRetainsStoreUntilRuntimeCanStop(t *testing.T) {
	ctx := context.Background()
	manager, app := testManager(t)
	runtime := &failFirstStopRuntime{}
	manager.WithRuntime(runtime).WithOpenHook(func(context.Context, *Store) error {
		return errors.New("later open hook failed")
	})
	parent := t.TempDir()
	if _, err := manager.Create(ctx, "Retained failure", ExplicitNewProjectParent(parent)); err == nil || !strings.Contains(err.Error(), "runtime did not stop") {
		t.Fatalf("create error = %v", err)
	}

	manager.mu.Lock()
	if len(manager.projects) != 1 {
		manager.mu.Unlock()
		t.Fatalf("retained entries = %d", len(manager.projects))
	}
	var retained *projectEntry
	for _, entry := range manager.projects {
		retained = entry
	}
	root := retained.root
	state := retained.state
	storeOpen := retained.store != nil && retained.store.db != nil
	manager.mu.Unlock()
	if state != projectUnavailable || !storeOpen {
		t.Fatalf("retained state=%v store_open=%t", state, storeOpen)
	}
	if _, err := os.Stat(filepath.Join(root, "project.sqlite")); err != nil {
		t.Fatalf("retained project was deleted: %v", err)
	}
	if err := manager.CloseAll(ctx); err != nil {
		t.Fatalf("retry close: %v", err)
	}
	if len(manager.projects) != 0 {
		t.Fatalf("entries after close = %d", len(manager.projects))
	}

	reopenedManager := NewManager(app)
	t.Cleanup(func() { _ = reopenedManager.Close() })
	opened, err := reopenedManager.OpenSelected(ctx, ExplicitExistingDirectory(root))
	if err != nil || opened.RootPath != root {
		t.Fatalf("reopen retained project = %+v, error = %v", opened, err)
	}
}

func waitProjectOpenHook(t *testing.T, entered <-chan string) string {
	t.Helper()
	select {
	case projectUUID := <-entered:
		return projectUUID
	case <-time.After(3 * time.Second):
		t.Fatal("project open hook did not run concurrently")
		return ""
	}
}

func TestCreateWithGenerationLanguagePersistsProjectSetting(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.CreateWithInput(ctx, CreateInput{Name: "English Story", GenerationLanguage: "en-US"}, ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var stored Project
	if err := manager.WithCurrentStore(ctx, created.UUID, func(store *Store) error {
		return store.DB().Where("uuid = ?", created.UUID).First(&stored).Error
	}); err != nil {
		t.Fatal(err)
	}
	if stored.GenerationLanguage != GenerationLanguageEnglish {
		t.Fatalf("generation language = %q", stored.GenerationLanguage)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.OpenRecent(ctx, created.UUID); err != nil {
		t.Fatal(err)
	}
	if err := manager.WithCurrentStore(ctx, created.UUID, func(store *Store) error {
		return store.DB().Where("uuid = ?", created.UUID).First(&stored).Error
	}); err != nil || stored.GenerationLanguage != GenerationLanguageEnglish {
		t.Fatalf("reopened language = %q, error = %v", stored.GenerationLanguage, err)
	}
	if _, err := manager.CreateWithInput(ctx, CreateInput{Name: "Invalid", GenerationLanguage: "fr"}, ExplicitNewProjectParent(t.TempDir())); errorCode(err) != CodeInvalidProject {
		t.Fatalf("invalid language error = %v", err)
	}
}

func TestCreateReservesFirstAvailableDirectoryWithoutChangingProjectName(t *testing.T) {
	for _, test := range []struct {
		name          string
		occupiedCount int
		expectedName  string
	}{
		{name: "base", occupiedCount: 0, expectedName: "小熊的月亮灯"},
		{name: "number two", occupiedCount: 1, expectedName: "小熊的月亮灯-2"},
		{name: "number three", occupiedCount: 2, expectedName: "小熊的月亮灯-3"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			manager, _ := testManager(t)
			parent := t.TempDir()
			canonicalParent, err := normalizeDirectory(parent)
			if err != nil {
				t.Fatal(err)
			}
			baseName := "小熊的月亮灯"
			for number := 1; number <= test.occupiedCount; number++ {
				path := projectDirectoryTestPath(parent, baseName, number)
				if err := os.Mkdir(path, 0o751); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o751); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "existing.txt"), []byte(fmt.Sprintf("existing-%d", number)), 0o640); err != nil {
					t.Fatal(err)
				}
			}

			created, err := manager.Create(ctx, baseName, ExplicitNewProjectParent(parent))
			if err != nil {
				t.Fatal(err)
			}
			if created.Name != baseName || created.RootPath != filepath.Join(canonicalParent, test.expectedName) {
				t.Fatalf("created = %+v", created)
			}
			if _, err := os.Stat(filepath.Join(parent, baseName+"-1")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unexpected -1 candidate: %v", err)
			}
			for number := 1; number <= test.occupiedCount; number++ {
				path := projectDirectoryTestPath(parent, baseName, number)
				content, err := os.ReadFile(filepath.Join(path, "existing.txt"))
				if err != nil || string(content) != fmt.Sprintf("existing-%d", number) {
					t.Fatalf("occupied candidate %s changed: content=%q error=%v", path, content, err)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatalf("stat occupied candidate %s: %v", path, err)
				}
				if runtime.GOOS != "windows" && info.Mode().Perm() != 0o751 {
					t.Fatalf("occupied candidate %s mode=%v", path, info.Mode().Perm())
				}
			}
		})
	}
}

func TestCreateUsesLastNumberedDirectoryCandidate(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	parent := t.TempDir()
	canonicalParent, err := normalizeDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	baseName := projectDirectoryName("Last Candidate")
	for number := 1; number < maxProjectDirectoryNumber; number++ {
		if err := os.Mkdir(projectDirectoryTestPath(parent, baseName, number), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	created, err := manager.Create(ctx, "Last Candidate", ExplicitNewProjectParent(parent))
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(canonicalParent, fmt.Sprintf("%s-%d", baseName, maxProjectDirectoryNumber))
	if created.RootPath != expected || created.Name != "Last Candidate" {
		t.Fatalf("created = %+v", created)
	}
}

func TestCreateSkipsExistingFileWithoutModifyingIt(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	parent := t.TempDir()
	canonicalParent, err := normalizeDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(canonicalParent, "File-Conflict")
	if err := os.WriteFile(base, []byte("keep this file"), 0o640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(base)
	if err != nil {
		t.Fatal(err)
	}

	created, err := manager.Create(ctx, "File Conflict", ExplicitNewProjectParent(parent))
	if err != nil {
		t.Fatal(err)
	}
	if created.RootPath != base+"-2" || created.Name != "File Conflict" {
		t.Fatalf("created = %+v", created)
	}
	content, err := os.ReadFile(base)
	if err != nil || string(content) != "keep this file" {
		t.Fatalf("existing file content=%q error=%v", content, err)
	}
	after, err := os.Stat(base)
	if err != nil {
		t.Fatalf("stat existing file after create: %v", err)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("existing file mode before=%v after=%v", before.Mode().Perm(), after.Mode().Perm())
	}
}

func TestFailedCreateRemovesOnlyReservedCandidate(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	parent := t.TempDir()
	baseName := "Cleanup Safety"
	for number := 1; number <= 2; number++ {
		path := projectDirectoryTestPath(parent, projectDirectoryName(baseName), number)
		if err := os.Mkdir(path, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "sentinel"), []byte(fmt.Sprintf("candidate-%d", number)), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	manager.WithOpenHook(func(context.Context, *Store) error {
		return errors.New("forced initialization failure")
	})

	if _, err := manager.Create(ctx, baseName, ExplicitNewProjectParent(parent)); err == nil {
		t.Fatal("expected create failure")
	}
	if _, err := os.Stat(projectDirectoryTestPath(parent, projectDirectoryName(baseName), 3)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reserved candidate was not removed: %v", err)
	}
	for number := 1; number <= 2; number++ {
		path := projectDirectoryTestPath(parent, projectDirectoryName(baseName), number)
		content, err := os.ReadFile(filepath.Join(path, "sentinel"))
		if err != nil || string(content) != fmt.Sprintf("candidate-%d", number) {
			t.Fatalf("existing candidate %s changed: content=%q error=%v", path, content, err)
		}
	}
}

func TestDirectoryNameExhaustionKeepsCurrentProjectState(t *testing.T) {
	ctx := context.Background()
	manager, store := testManager(t)
	runtime := &lifecycleRuntime{}
	var events []LifecycleEvent
	manager.WithRuntime(runtime).WithLifecycleHook(func(event LifecycleEvent) {
		events = append(events, event)
	})
	current, err := manager.Create(ctx, "Current", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	baseName := projectDirectoryName("Exhausted")
	for number := 1; number <= maxProjectDirectoryNumber; number++ {
		if err := os.Mkdir(projectDirectoryTestPath(parent, baseName, number), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := manager.Create(ctx, "Exhausted", ExplicitNewProjectParent(parent)); errorCode(err) != CodeProjectDirectoryNameExhausted {
		t.Fatalf("exhausted create error = %v", err)
	}
	assertFailedCreateKeptCurrentState(t, ctx, manager, store, runtime, events, current)
}

func TestUnwritableParentKeepsCurrentProjectState(t *testing.T) {
	ctx := context.Background()
	manager, store := testManager(t)
	runtime := &lifecycleRuntime{}
	var events []LifecycleEvent
	manager.WithRuntime(runtime).WithLifecycleHook(func(event LifecycleEvent) {
		events = append(events, event)
	})
	current, err := manager.Create(ctx, "Current", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Skipf("cannot restrict parent permissions: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	probe := filepath.Join(parent, "permission-probe")
	if err := os.Mkdir(probe, 0o755); err == nil {
		_ = os.Remove(probe)
		t.Skip("filesystem does not enforce the test directory permissions")
	} else if !errors.Is(err, os.ErrPermission) {
		t.Skipf("filesystem returned an unsupported permission error: %v", err)
	}

	if _, err := manager.Create(ctx, "Permission Failure", ExplicitNewProjectParent(parent)); errorCode(err) != CodePermissionDenied {
		t.Fatalf("permission create error = %v", err)
	}
	assertFailedCreateKeptCurrentState(t, ctx, manager, store, runtime, events, current)
}

func TestConcurrentCreatesReserveDistinctDirectories(t *testing.T) {
	const count = 6
	parent := t.TempDir()
	type createResult struct {
		summary Summary
		err     error
	}
	results := make(chan createResult, count)
	var wait sync.WaitGroup
	for range count {
		manager, _ := testManager(t)
		wait.Add(1)
		go func() {
			defer wait.Done()
			created, err := manager.Create(context.Background(), "Concurrent", ExplicitNewProjectParent(parent))
			results <- createResult{summary: created, err: err}
		}()
	}
	wait.Wait()
	close(results)

	roots := make(map[string]struct{}, count)
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent create error = %v", result.err)
		}
		if result.summary.Name != "Concurrent" {
			t.Fatalf("concurrent name = %q", result.summary.Name)
		}
		if _, exists := roots[result.summary.RootPath]; exists {
			t.Fatalf("duplicate root path = %s", result.summary.RootPath)
		}
		roots[result.summary.RootPath] = struct{}{}
		if _, err := os.Stat(filepath.Join(result.summary.RootPath, "project.sqlite")); err != nil {
			t.Fatalf("incomplete project at %s: %v", result.summary.RootPath, err)
		}
	}
	if len(roots) != count {
		t.Fatalf("root count = %d", len(roots))
	}
}

func projectDirectoryTestPath(parent, baseName string, number int) string {
	if number == 1 {
		return filepath.Join(parent, baseName)
	}
	return filepath.Join(parent, fmt.Sprintf("%s-%d", baseName, number))
}

func assertFailedCreateKeptCurrentState(t *testing.T, ctx context.Context, manager *Manager, store *appstore.Store, runtime *lifecycleRuntime, events []LifecycleEvent, current Summary) {
	t.Helper()
	active := manager.Current()
	if active == nil || active.UUID != current.UUID || active.RootPath != current.RootPath {
		t.Fatalf("current changed: before=%+v after=%+v", current, active)
	}
	if len(runtime.stoppedFor) != 0 {
		t.Fatalf("runtime stopped projects = %v", runtime.stoppedFor)
	}
	if len(events) != 1 || events[0] != (LifecycleEvent{ProjectUUID: current.UUID, Open: true}) {
		t.Fatalf("lifecycle events = %+v", events)
	}
	recents, err := store.RecentProjects(ctx)
	if err != nil || len(recents) != 1 || recents[0].UUID != current.UUID || recents[0].RootPath != current.RootPath {
		t.Fatalf("recent projects = %+v error=%v", recents, err)
	}
}

func TestProjectLockIsExclusiveAndRecoversAfterClose(t *testing.T) {
	ctx := context.Background()
	first, store := testManager(t)
	created, err := first.Create(ctx, "Locked", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	second := NewManager(store)
	if _, err := second.OpenRecent(ctx, created.UUID); errorCode(err) != CodeLocked {
		t.Fatalf("second open error = %v", err)
	}
	if err := first.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	opened, err := second.OpenRecent(ctx, created.UUID)
	if err != nil || opened.UUID != created.UUID {
		t.Fatalf("open after release = %+v, error = %v", opened, err)
	}
	_ = second.Close()
}

func TestRelocateVerifiesDatabaseIdentityWithoutChangingRecord(t *testing.T) {
	ctx := context.Background()
	manager, store := testManager(t)
	first, err := manager.Create(ctx, "First", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := manager.Create(ctx, "Second", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Relocate(ctx, first.UUID, ExplicitExistingDirectory(second.RootPath)); errorCode(err) != CodeIdentityMismatch {
		t.Fatalf("identity mismatch error = %v", err)
	}
	recent, err := store.RecentProject(ctx, first.UUID)
	if err != nil || recent.RootPath != first.RootPath {
		t.Fatalf("first recent = %+v, error = %v", recent, err)
	}
}

func TestFormatTooNewRefusesToOpen(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.Create(ctx, "Future", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	store := openStoreForTest(t, manager, created.UUID)
	if err := store.DB().Model(&Project{}).Where("uuid = ?", created.UUID).Update("format_version", SupportedFormatVersion+1).Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.OpenRecent(ctx, created.UUID); errorCode(err) != CodeFormatTooNew {
		t.Fatalf("future format error = %v", err)
	}
}

func TestResolveRelativePathRejectsTraversalAndSymlinks(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveRelativePath(root, "../../outside"); errorCode(err) != CodeInvalidPath {
		t.Fatalf("traversal error = %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ResolveRelativePath(root, "linked/file.txt"); errorCode(err) != CodeInvalidPath {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestOpenBackfillsNewManagedDirectoryAfterLock(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.Create(ctx, "Legacy layout", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	quarantine := filepath.Join(created.RootPath, ".lumi", "quarantine")
	if err := os.Remove(quarantine); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.OpenRecent(ctx, created.UUID); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(quarantine)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("quarantine info=%v error=%v", info, err)
	}
}

func TestFailedMigrationRestoresConsistentBackup(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.Create(ctx, "Restore", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := openStoreForTest(t, manager, created.UUID).DB().Exec("CREATE TABLE preserved (value TEXT); INSERT INTO preserved VALUES ('safe')").Error; err != nil {
		t.Fatal(err)
	}
	header, err := readHeader(ctx, created.RootPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	brokenFS := fstest.MapFS{
		"20260829000033_break.up.sql":   {Data: []byte("CREATE TABLE half_written (id INTEGER); THIS IS NOT SQL;")},
		"20260829000033_break.down.sql": {Data: []byte("DROP TABLE half_written;")},
	}
	_, err = migrateProjectWith(ctx, created.RootPath, &header, manager.now(), func(dsn string) (migrationRunner, error) {
		return dbmigrate.OpenWithFS(dsn, brokenFS, ".")
	})
	if errorCode(err) != CodeMigrationFailed {
		t.Fatalf("migration error = %v", err)
	}
	db, err := sql.Open("sqlite", projectReadOnlyDSN(created.RootPath))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRow("SELECT value FROM preserved").Scan(&value); err != nil || value != "safe" {
		t.Fatalf("preserved value = %q, error = %v", value, err)
	}
	var halfWritten int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 'half_written'").Scan(&halfWritten); err != nil || halfWritten != 0 {
		t.Fatalf("half_written count = %d, error = %v", halfWritten, err)
	}
	backups, err := filepath.Glob(filepath.Join(created.RootPath, ".lumi", "backups", "*.sqlite"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, error = %v", backups, err)
	}
}

func assertSQLiteHealthy(t *testing.T, store *Store) {
	t.Helper()
	for _, pragma := range []string{"integrity_check", "foreign_key_check"} {
		rows, err := store.DB().Raw("PRAGMA " + pragma).Rows()
		if err != nil {
			t.Fatal(err)
		}
		var values []string
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				t.Fatal(err)
			}
			values = append(values, value)
		}
		_ = rows.Close()
		if pragma == "integrity_check" && (len(values) != 1 || values[0] != "ok") {
			t.Fatalf("integrity_check = %v", values)
		}
		if pragma == "foreign_key_check" && len(values) != 0 {
			t.Fatalf("foreign_key_check = %v", values)
		}
	}
}

func openStoreForTest(t *testing.T, manager *Manager, projectUUID string) *Store {
	t.Helper()
	var result *Store
	if err := manager.WithStore(context.Background(), projectUUID, func(store *Store) error {
		result = store
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func errorCode(err error) string {
	var projectErr *Error
	if errors.As(err, &projectErr) {
		return projectErr.Code
	}
	return ""
}
