package project

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/promptcatalog"
)

type Runtime interface {
	StopProject(context.Context, string) error
	HasActiveWork(context.Context, string) (bool, error)
}

type OpenHook func(context.Context, *Store) error

type openHookContextKey struct{}

// IsProjectCreation reports whether an open hook is running as part of the
// initial project creation rather than opening an existing project.
func IsProjectCreation(ctx context.Context) bool {
	created, _ := ctx.Value(openHookContextKey{}).(bool)
	return created
}

type LifecycleEvent struct {
	ProjectUUID string
	Open        bool
}

type LifecycleHook func(LifecycleEvent)

type noopRuntime struct{}

func (noopRuntime) StopProject(context.Context, string) error { return nil }

func (noopRuntime) HasActiveWork(context.Context, string) (bool, error) { return false, nil }

type projectEntryState uint8

const (
	projectOpening projectEntryState = iota
	projectOpen
	projectDraining
	projectClosing
	projectFailed
	projectUnavailable
)

type projectEntry struct {
	uuid         string
	root         string
	store        *Store
	state        projectEntryState
	openErr      error
	leases       int
	presences    int
	lastActivity time.Time
	openedAt     time.Time
	changed      chan struct{}
}

type ProjectActivity struct {
	RequestLeases  int
	PresenceLeases int
	LastActivity   time.Time
}

type Manager struct {
	mu        sync.Mutex
	app       *appstore.Store
	projects  map[string]*projectEntry
	runtime   Runtime
	openHooks []OpenHook
	lifecycle LifecycleHook
	now       func() time.Time
}

type Summary struct {
	UUID         string              `json:"uuid"`
	Name         string              `json:"name"`
	RootPath     string              `json:"root_path"`
	Status       string              `json:"status"`
	StatusDetail string              `json:"status_detail"`
	Available    bool                `json:"available"`
	Open         bool                `json:"open"`
	UpdatedAt    time.Time           `json:"updated_at"`
	LastOpenedAt time.Time           `json:"last_opened_at"`
	PictureBook  *PictureBookProfile `json:"picture_book,omitempty"`
}

type CreateInput struct {
	Name               string
	GenerationLanguage string
	PictureBook        *PictureBookInput
	OverallStyle       string
}

func NewManager(app *appstore.Store) *Manager {
	return &Manager{app: app, projects: make(map[string]*projectEntry), runtime: noopRuntime{}, now: time.Now}
}

func (manager *Manager) WithRuntime(runtime Runtime) *Manager {
	if runtime != nil {
		manager.runtime = runtime
	}
	return manager
}

func (manager *Manager) WithOpenHook(hook OpenHook) *Manager {
	if hook != nil {
		manager.openHooks = append(manager.openHooks, hook)
	}
	return manager
}

func (manager *Manager) WithLifecycleHook(hook LifecycleHook) *Manager {
	manager.lifecycle = hook
	return manager
}

func (manager *Manager) notifyLifecycle(projectUUID string, open bool) {
	if manager.lifecycle != nil {
		manager.lifecycle(LifecycleEvent{ProjectUUID: projectUUID, Open: open})
	}
}

func (manager *Manager) signalLocked(entry *projectEntry) {
	close(entry.changed)
	entry.changed = make(chan struct{})
}

func waitForChange(ctx context.Context, changed <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
		return nil
	}
}

// WithStore pins one open project for the duration of a project-scoped
// operation. Closing that project waits for the callback to return, while
// operations for other projects remain independent.
func (manager *Manager) WithStore(ctx context.Context, projectUUID string, callback func(*Store) error) error {
	if !isUUIDv7(projectUUID) {
		return projectError(CodeInvalidUUID, "项目 UUID 无效", "项目 UUID 必须是 UUIDv7。", nil)
	}
	manager.mu.Lock()
	entry := manager.projects[projectUUID]
	if entry == nil {
		manager.mu.Unlock()
		if _, err := manager.app.RecentProject(ctx, projectUUID); err != nil {
			if errors.Is(err, appstore.ErrRecentProjectNotFound) {
				return projectError(CodeProjectNotFound, "项目不存在", "该项目不在已打开集合或最近项目索引中。", err)
			}
			return err
		}
		return projectError(CodeProjectNotOpen, "项目尚未打开", "请先打开该项目，再访问项目资源。", nil)
	}
	if entry.state != projectOpen {
		manager.mu.Unlock()
		return projectError(CodeProjectNotOpen, "项目尚未打开", "请先打开该项目，再访问项目资源。", nil)
	}
	entry.leases++
	entry.lastActivity = manager.now().UTC()
	store := entry.store
	manager.mu.Unlock()

	defer func() {
		manager.mu.Lock()
		entry.leases--
		entry.lastActivity = manager.now().UTC()
		manager.signalLocked(entry)
		manager.mu.Unlock()
	}()
	return callback(store)
}

// WithCurrentStore is retained for internal source compatibility. Project
// authorization is UUID-scoped; there is no global current project.
func (manager *Manager) WithCurrentStore(ctx context.Context, projectUUID string, callback func(*Store) error) error {
	return manager.WithStore(ctx, projectUUID, callback)
}

func (manager *Manager) SyncProjectName(ctx context.Context, projectUUID string) error {
	var name string
	if err := manager.WithStore(ctx, projectUUID, func(store *Store) error {
		name = store.ProjectName()
		return nil
	}); err != nil {
		return err
	}
	return manager.app.UpdateProjectName(ctx, projectUUID, name, manager.now().UTC())
}

// SyncCurrentProjectName is retained for internal source compatibility.
func (manager *Manager) SyncCurrentProjectName(ctx context.Context, projectUUID string) error {
	return manager.SyncProjectName(ctx, projectUUID)
}

func (manager *Manager) runOpenHooks(ctx context.Context, store *Store) error {
	for _, hook := range manager.openHooks {
		if err := hook(ctx, store); err != nil {
			return err
		}
	}
	return nil
}

func (manager *Manager) RecentProjects(ctx context.Context) ([]Summary, error) {
	recents, err := manager.app.RecentProjects(ctx)
	if err != nil {
		return nil, err
	}
	manager.mu.Lock()
	openRoots := make(map[string]string, len(manager.projects))
	for uuid, entry := range manager.projects {
		if entry.state == projectOpen {
			openRoots[uuid] = entry.root
		}
	}
	manager.mu.Unlock()

	items := make([]Summary, 0, len(recents))
	for _, recent := range recents {
		if !isUUIDv7(recent.UUID) {
			return nil, projectError(CodeInvalidUUID, "最近项目 UUID 无效", "应用数据库中的最近项目索引已损坏。", nil)
		}
		item := Summary{
			UUID: recent.UUID, Name: recent.Name, RootPath: recent.RootPath,
			Status: "recent", Available: true, UpdatedAt: recent.UpdatedAt, LastOpenedAt: recent.LastOpenedAt,
		}
		if openRoots[recent.UUID] == recent.RootPath {
			item.Open = true
			item.Status = "open"
		}
		items = append(items, item)
	}
	return items, nil
}

func (manager *Manager) RecentProject(ctx context.Context, projectUUID string) (Summary, error) {
	if !isUUIDv7(projectUUID) {
		return Summary{}, projectError(CodeInvalidUUID, "项目 UUID 无效", "项目 UUID 必须是 UUIDv7。", nil)
	}
	items, err := manager.RecentProjects(ctx)
	if err != nil {
		return Summary{}, err
	}
	for _, item := range items {
		if item.UUID == projectUUID {
			return item, nil
		}
	}
	return Summary{}, projectError(CodeProjectNotFound, "最近项目不存在", "该项目可能已从最近列表移除。", appstore.ErrRecentProjectNotFound)
}

func (manager *Manager) OpenProjects(ctx context.Context) ([]Summary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	manager.mu.Lock()
	items := make([]Summary, 0, len(manager.projects))
	for _, entry := range manager.projects {
		if entry.state == projectOpen {
			items = append(items, summaryForStore(entry.store, entry.openedAt))
		}
	}
	manager.mu.Unlock()
	sort.Slice(items, func(first, second int) bool {
		return items[first].LastOpenedAt.After(items[second].LastOpenedAt)
	})
	return items, nil
}

func (manager *Manager) OpenProjectUUIDs() []string {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	items := make([]string, 0, len(manager.projects))
	for uuid, entry := range manager.projects {
		if entry.state == projectOpen {
			items = append(items, uuid)
		}
	}
	sort.Strings(items)
	return items
}

func (manager *Manager) IsOpen(projectUUID string) bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	entry := manager.projects[projectUUID]
	return entry != nil && entry.state == projectOpen
}

func (manager *Manager) Activity(projectUUID string) (ProjectActivity, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	entry := manager.projects[projectUUID]
	if entry == nil || entry.state != projectOpen {
		return ProjectActivity{}, false
	}
	return ProjectActivity{RequestLeases: entry.leases, PresenceLeases: entry.presences, LastActivity: entry.lastActivity}, true
}

func (manager *Manager) HasActiveWork(ctx context.Context, projectUUID string) (bool, error) {
	manager.mu.Lock()
	entry := manager.projects[projectUUID]
	if entry == nil || entry.state != projectOpen {
		manager.mu.Unlock()
		return false, nil
	}
	entry.leases++
	manager.mu.Unlock()
	defer func() {
		manager.mu.Lock()
		entry.leases--
		manager.signalLocked(entry)
		manager.mu.Unlock()
	}()
	return manager.runtime.HasActiveWork(ctx, projectUUID)
}

// AcquirePresence pins an open project while a realtime project topic is
// joined. The returned release function is safe to call more than once.
func (manager *Manager) AcquirePresence(projectUUID string) (func(), error) {
	if !isUUIDv7(projectUUID) {
		return nil, projectError(CodeInvalidUUID, "项目 UUID 无效", "项目 UUID 必须是 UUIDv7。", nil)
	}
	manager.mu.Lock()
	entry := manager.projects[projectUUID]
	if entry == nil || entry.state != projectOpen {
		manager.mu.Unlock()
		return nil, projectError(CodeProjectNotOpen, "项目尚未打开", "请先打开该项目，再订阅项目事件。", nil)
	}
	entry.presences++
	entry.lastActivity = manager.now().UTC()
	manager.signalLocked(entry)
	manager.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			manager.mu.Lock()
			entry.presences--
			entry.lastActivity = manager.now().UTC()
			manager.signalLocked(entry)
			manager.mu.Unlock()
		})
	}, nil
}

// Current is retained for legacy internal tests and returns a value only when
// exactly one project is open. Application behavior must use URL UUIDs or the
// open-project collection instead.
func (manager *Manager) Current() *Summary {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	var found *projectEntry
	for _, entry := range manager.projects {
		if entry.state != projectOpen {
			continue
		}
		if found != nil {
			return nil
		}
		found = entry
	}
	if found == nil {
		return nil
	}
	result := summaryForStore(found.store, found.openedAt)
	return &result
}

func summaryForStore(store *Store, openedAt time.Time) Summary {
	profile := store.PictureBookProfile()
	return Summary{
		UUID: store.ProjectUUID(), Name: store.ProjectName(), RootPath: store.Root(),
		Status: "open", Available: true, Open: true, UpdatedAt: openedAt, LastOpenedAt: openedAt,
		PictureBook: &profile,
	}
}

func (manager *Manager) Create(ctx context.Context, name string, selector NewProjectParentSelector) (Summary, error) {
	return manager.CreateWithInput(ctx, CreateInput{Name: name, GenerationLanguage: DefaultGenerationLanguage}, selector)
}

func (manager *Manager) CreateWithInput(ctx context.Context, input CreateInput, selector NewProjectParentSelector) (Summary, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 120 {
		return Summary{}, projectError(CodeInvalidProject, "项目名称无效", "项目名称需包含 1 到 120 个字符。", nil)
	}
	generationLanguage, valid := NormalizeGenerationLanguage(input.GenerationLanguage)
	if !valid {
		return Summary{}, projectError(CodeInvalidProject, "项目生成语言无效", "generation_language 只支持 zh-Hans 或 en。", nil)
	}
	pictureBook, err := NormalizePictureBookInput(input.PictureBook)
	if err != nil {
		return Summary{}, err
	}
	overallStyle := strings.TrimSpace(input.OverallStyle)
	if len([]rune(overallStyle)) > 12000 {
		return Summary{}, projectError(CodeInvalidOverallStyle, "整体画风提示词过长", "overall_style 最多 12000 个字符。", nil)
	}
	if overallStyle == "" {
		overallStyle = strings.TrimSpace(promptcatalog.DefaultProjectStyle(generationLanguage))
	}
	parent, err := selector.SelectNewProjectParent(ctx)
	if err != nil {
		return Summary{}, err
	}
	directoryName := projectDirectoryName(name)
	if directoryName == "" {
		return Summary{}, projectError(CodeInvalidProject, "项目名称无法用作目录", "请在名称中加入字母、数字或文字。", nil)
	}
	root, err := reserveNewProjectDirectory(parent, directoryName)
	if err != nil {
		return Summary{}, err
	}
	committed := false
	defer func() {
		if !committed {
			removeNewProject(root)
		}
	}()
	if err := createProjectLayout(root, name); err != nil {
		return Summary{}, projectError(CodePermissionDenied, "无法创建项目文件", "请检查目录写权限与磁盘可用空间。", err)
	}
	projectUUID, err := newUUIDv7()
	if err != nil {
		return Summary{}, err
	}
	actorUUID, err := newUUIDv7()
	if err != nil {
		return Summary{}, err
	}
	now := manager.now().UTC()
	lock, err := acquireProjectLock(root, projectUUID, now)
	if err != nil {
		return Summary{}, err
	}
	store, err := initializeStore(ctx, root, projectUUID, name, generationLanguage, actorUUID, pictureBook, overallStyle, now, lock)
	if err != nil {
		_ = lock.Close()
		return Summary{}, err
	}
	entry, owner, err := manager.startOpening(ctx, projectUUID, root)
	if err != nil || !owner {
		_ = store.Close()
		if err != nil {
			return Summary{}, err
		}
		return summaryForStore(entry.store, entry.openedAt), nil
	}
	if err := manager.runOpenHooks(context.WithValue(ctx, openHookContextKey{}, true), store); err != nil {
		openErr, retained := manager.failOpening(ctx, entry, store, err)
		committed = retained
		return Summary{}, openErr
	}
	if err := manager.app.RecordProject(ctx, projectUUID, name, root, now); err != nil {
		openErr, retained := manager.failOpening(ctx, entry, store, err)
		committed = retained
		return Summary{}, openErr
	}
	manager.finishOpening(entry, store, nil)
	manager.notifyLifecycle(projectUUID, true)
	committed = true
	return summaryForStore(store, now), nil
}

func (manager *Manager) OpenRecent(ctx context.Context, projectUUID string) (Summary, error) {
	if !isUUIDv7(projectUUID) {
		return Summary{}, projectError(CodeInvalidUUID, "项目 UUID 无效", "项目 UUID 必须是 UUIDv7。", nil)
	}
	recent, err := manager.app.RecentProject(ctx, projectUUID)
	if err != nil {
		if errors.Is(err, appstore.ErrRecentProjectNotFound) {
			return Summary{}, projectError(CodeProjectNotFound, "最近项目不存在", "该项目可能已从最近列表移除。", err)
		}
		return Summary{}, err
	}
	return manager.open(ctx, ExplicitExistingDirectory(recent.RootPath), projectUUID)
}

func (manager *Manager) OpenSelected(ctx context.Context, selector ExistingDirectorySelector) (Summary, error) {
	return manager.open(ctx, selector, "")
}

func (manager *Manager) Relocate(ctx context.Context, projectUUID string, selector ExistingDirectorySelector) (Summary, error) {
	if !isUUIDv7(projectUUID) {
		return Summary{}, projectError(CodeInvalidUUID, "项目 UUID 无效", "项目 UUID 必须是 UUIDv7。", nil)
	}
	if _, err := manager.app.RecentProject(ctx, projectUUID); err != nil {
		if errors.Is(err, appstore.ErrRecentProjectNotFound) {
			return Summary{}, projectError(CodeProjectNotFound, "最近项目不存在", "无法重新定位已移除的最近项目。", err)
		}
		return Summary{}, err
	}
	root, header, err := prepareExistingProject(ctx, selector, projectUUID)
	if err != nil {
		return Summary{}, err
	}
	manager.mu.Lock()
	entry := manager.projects[projectUUID]
	sameRoot := entry != nil && entry.state == projectOpen && entry.root == root
	manager.mu.Unlock()
	if sameRoot {
		return summaryForStore(entry.store, entry.openedAt), nil
	}
	if _, err := manager.CloseProject(ctx, projectUUID); err != nil {
		return Summary{}, err
	}
	return manager.openPrepared(ctx, root, header)
}

func prepareExistingProject(ctx context.Context, selector ExistingDirectorySelector, expectedUUID string) (string, Header, error) {
	root, err := selector.SelectExistingDirectory(ctx)
	if err != nil {
		return "", Header{}, err
	}
	if err := validateProjectContract(root); err != nil {
		return "", Header{}, err
	}
	header, err := readHeader(ctx, root)
	if err != nil {
		return "", Header{}, err
	}
	if expectedUUID != "" && header.UUID != expectedUUID {
		return "", Header{}, projectError(CodeIdentityMismatch, "所选目录属于另一个项目", "请选择 UUID 与最近项目一致的目录；原记录未被修改。", nil)
	}
	return root, header, nil
}

func (manager *Manager) open(ctx context.Context, selector ExistingDirectorySelector, expectedUUID string) (Summary, error) {
	root, header, err := prepareExistingProject(ctx, selector, expectedUUID)
	if err != nil {
		return Summary{}, err
	}
	return manager.openPrepared(ctx, root, header)
}

func (manager *Manager) openPrepared(ctx context.Context, root string, header Header) (Summary, error) {
	entry, owner, err := manager.startOpening(ctx, header.UUID, root)
	if err != nil {
		return Summary{}, err
	}
	if !owner {
		return summaryForStore(entry.store, entry.openedAt), nil
	}
	now := manager.now().UTC()
	lock, err := acquireProjectLock(root, header.UUID, now)
	if err != nil {
		manager.finishOpening(entry, nil, err)
		return Summary{}, err
	}
	if err := ensureManagedProjectDirectories(root); err != nil {
		_ = lock.Close()
		manager.finishOpening(entry, nil, err)
		return Summary{}, err
	}
	store, err := openStore(ctx, root, header, now, lock)
	if err != nil {
		_ = lock.Close()
		manager.finishOpening(entry, nil, err)
		return Summary{}, err
	}
	if err := manager.runOpenHooks(ctx, store); err != nil {
		openErr, _ := manager.failOpening(ctx, entry, store, err)
		return Summary{}, openErr
	}
	if err := manager.app.RecordProject(ctx, header.UUID, header.Name, root, now); err != nil {
		openErr, _ := manager.failOpening(ctx, entry, store, err)
		return Summary{}, openErr
	}
	manager.finishOpening(entry, store, nil)
	manager.notifyLifecycle(header.UUID, true)
	return summaryForStore(store, now), nil
}

func (manager *Manager) startOpening(ctx context.Context, projectUUID, root string) (*projectEntry, bool, error) {
	for {
		manager.mu.Lock()
		entry := manager.projects[projectUUID]
		if entry == nil {
			entry = &projectEntry{
				uuid: projectUUID, root: root, state: projectOpening,
				lastActivity: manager.now().UTC(), changed: make(chan struct{}),
			}
			manager.projects[projectUUID] = entry
			manager.mu.Unlock()
			return entry, true, nil
		}
		if entry.root != root {
			manager.mu.Unlock()
			return nil, false, projectError(CodeIdentityMismatch, "项目已从另一个目录打开", "请先关闭已打开的项目，再显式重新定位。", nil)
		}
		switch entry.state {
		case projectOpen:
			manager.mu.Unlock()
			return entry, false, nil
		case projectFailed, projectUnavailable:
			err := entry.openErr
			manager.mu.Unlock()
			return nil, false, err
		default:
			changed := entry.changed
			manager.mu.Unlock()
			if err := waitForChange(ctx, changed); err != nil {
				return nil, false, err
			}
			manager.mu.Lock()
			if entry.state == projectFailed || entry.state == projectUnavailable {
				err := entry.openErr
				manager.mu.Unlock()
				return nil, false, err
			}
			manager.mu.Unlock()
		}
	}
}

func (manager *Manager) finishOpening(entry *projectEntry, store *Store, openErr error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if openErr != nil {
		entry.store = store
		entry.state = projectFailed
		if store != nil {
			entry.state = projectUnavailable
		}
		entry.openErr = openErr
		manager.signalLocked(entry)
		if store == nil && manager.projects[entry.uuid] == entry {
			delete(manager.projects, entry.uuid)
		}
		return
	}
	entry.store = store
	entry.state = projectOpen
	entry.openedAt = manager.now().UTC()
	entry.lastActivity = entry.openedAt
	manager.signalLocked(entry)
}

func (manager *Manager) cleanupFailedOpen(ctx context.Context, store *Store) error {
	if stopErr := manager.runtime.StopProject(ctx, store.ProjectUUID()); stopErr != nil {
		return stopErr
	}
	return store.Close()
}

func (manager *Manager) failOpening(ctx context.Context, entry *projectEntry, store *Store, cause error) (error, bool) {
	cleanupErr := manager.cleanupFailedOpen(ctx, store)
	result := errors.Join(cause, cleanupErr)
	retained := cleanupErr != nil && store.db != nil
	if retained {
		manager.finishOpening(entry, store, result)
	} else {
		manager.finishOpening(entry, nil, result)
	}
	return result, retained
}

func (manager *Manager) Forget(ctx context.Context, projectUUID string) error {
	if !isUUIDv7(projectUUID) {
		return projectError(CodeInvalidUUID, "项目 UUID 无效", "项目 UUID 必须是 UUIDv7。", nil)
	}
	if err := manager.app.ForgetProject(ctx, projectUUID); err != nil {
		if errors.Is(err, appstore.ErrRecentProjectNotFound) {
			return projectError(CodeProjectNotFound, "最近项目不存在", "该记录可能已被移除。", err)
		}
		return err
	}
	return nil
}

func (manager *Manager) CloseProject(ctx context.Context, projectUUID string) (bool, error) {
	return manager.closeProject(ctx, projectUUID, false)
}

func (manager *Manager) CloseProjectIfIdle(ctx context.Context, projectUUID string) (bool, error) {
	return manager.closeProject(ctx, projectUUID, true)
}

func (manager *Manager) closeProject(ctx context.Context, projectUUID string, idleOnly bool) (bool, error) {
	if !isUUIDv7(projectUUID) {
		return false, projectError(CodeInvalidUUID, "项目 UUID 无效", "项目 UUID 必须是 UUIDv7。", nil)
	}
	for {
		manager.mu.Lock()
		entry := manager.projects[projectUUID]
		if entry == nil {
			manager.mu.Unlock()
			return false, nil
		}
		originalState := entry.state
		if originalState == projectUnavailable && idleOnly {
			manager.mu.Unlock()
			return false, nil
		}
		if originalState != projectOpen && originalState != projectUnavailable {
			changed := entry.changed
			manager.mu.Unlock()
			if err := waitForChange(ctx, changed); err != nil {
				return false, err
			}
			continue
		}
		if originalState == projectOpen && idleOnly && (entry.presences > 0 || entry.leases > 0) {
			manager.mu.Unlock()
			return false, nil
		}
		entry.state = projectDraining
		manager.signalLocked(entry)
		manager.mu.Unlock()

		if err := manager.waitForLeases(ctx, entry); err != nil {
			manager.restoreState(entry, originalState)
			return false, err
		}
		if originalState == projectOpen && idleOnly {
			busy, err := manager.runtime.HasActiveWork(ctx, projectUUID)
			if err != nil || busy {
				manager.restoreState(entry, originalState)
				return false, err
			}
		}

		manager.mu.Lock()
		entry.state = projectClosing
		manager.signalLocked(entry)
		manager.mu.Unlock()
		if err := manager.runtime.StopProject(ctx, projectUUID); err != nil {
			manager.restoreState(entry, originalState)
			return false, err
		}
		closeErr := entry.store.Close()
		manager.mu.Lock()
		if manager.projects[projectUUID] == entry {
			delete(manager.projects, projectUUID)
		}
		manager.signalLocked(entry)
		manager.mu.Unlock()
		if originalState == projectOpen {
			manager.notifyLifecycle(projectUUID, false)
		}
		return true, closeErr
	}
}

func (manager *Manager) waitForLeases(ctx context.Context, entry *projectEntry) error {
	for {
		manager.mu.Lock()
		if entry.leases == 0 {
			manager.mu.Unlock()
			return nil
		}
		changed := entry.changed
		manager.mu.Unlock()
		if err := waitForChange(ctx, changed); err != nil {
			return err
		}
	}
}

func (manager *Manager) restoreState(entry *projectEntry, state projectEntryState) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.projects[entry.uuid] != entry {
		return
	}
	entry.state = state
	entry.lastActivity = manager.now().UTC()
	manager.signalLocked(entry)
}

// CloseCurrent is retained for single-project internal callers. It never
// chooses among multiple open projects.
func (manager *Manager) CloseCurrent(ctx context.Context) error {
	uuids := manager.OpenProjectUUIDs()
	if len(uuids) == 0 {
		return nil
	}
	if len(uuids) != 1 {
		return fmt.Errorf("close current project: %d projects are open", len(uuids))
	}
	_, err := manager.CloseProject(ctx, uuids[0])
	return err
}

// CloseCurrentIfIdle is retained for source compatibility and closes only the
// explicitly supplied UUID.
func (manager *Manager) CloseCurrentIfIdle(ctx context.Context, projectUUID string) (bool, error) {
	return manager.CloseProjectIfIdle(ctx, projectUUID)
}

func (manager *Manager) CloseAll(ctx context.Context) error {
	var result error
	manager.mu.Lock()
	projectUUIDs := make([]string, 0, len(manager.projects))
	for projectUUID := range manager.projects {
		projectUUIDs = append(projectUUIDs, projectUUID)
	}
	manager.mu.Unlock()
	sort.Strings(projectUUIDs)
	for _, projectUUID := range projectUUIDs {
		_, err := manager.CloseProject(ctx, projectUUID)
		result = errors.Join(result, err)
	}
	return result
}

func (manager *Manager) Close() error {
	return manager.CloseAll(context.Background())
}

// MigrateDirectory performs the same identity, lock, online-backup and restore
// protocol as a normal open, without adding the project to an app recent list.
func MigrateDirectory(ctx context.Context, rawRoot string) error {
	root, err := normalizeDirectory(rawRoot)
	if err != nil {
		return err
	}
	if err := validateProjectContract(root); err != nil {
		return err
	}
	header, err := readHeader(ctx, root)
	if err != nil {
		return err
	}
	lock, err := acquireProjectLock(root, header.UUID, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := ensureManagedProjectDirectories(root); err != nil {
		_ = lock.Close()
		return err
	}
	store, err := openStore(ctx, root, header, time.Now().UTC(), lock)
	if err != nil {
		_ = lock.Close()
		return err
	}
	return store.Close()
}
