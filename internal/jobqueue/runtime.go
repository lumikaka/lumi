package jobqueue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"lumi/internal/agent"
	"lumi/internal/config"
	"lumi/internal/database"
	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/modelsettings"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/realtime"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riversqlite"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
)

type riverErrorLogger struct {
	logger *slog.Logger
}

func (handler *riverErrorLogger) HandleError(context.Context, *rivertype.JobRow, error) *river.ErrorHandlerResult {
	return nil
}

func (handler *riverErrorLogger) HandlePanic(ctx context.Context, job *rivertype.JobRow, value any, trace string) *river.ErrorHandlerResult {
	handler.logger.ErrorContext(ctx, "River worker panic", "job_kind", job.Kind, "panic", value, "trace", trace)
	return nil
}

type Manager struct {
	mu          sync.RWMutex
	runtimes    map[string]*projectRuntime
	starts      map[string]*runtimeStart
	stops       map[string]*runtimeStop
	unavailable map[string]error
	providers   *provider.Service
	models      *modelsettings.Resolver
	llm         llm.Client
	image       imagegen.Client
	hub         *realtime.Hub
	now         func() time.Time
	agents      *agent.Service
}

type runtimeStart struct {
	done chan struct{}
	err  error
}

type runtimeStop struct {
	done chan struct{}
	err  error
}

func (manager *Manager) WithAgentService(service *agent.Service) *Manager {
	manager.agents = service
	return manager
}

func NewManager(providers *provider.Service, modelClient llm.Client, hub *realtime.Hub) *Manager {
	if modelClient == nil {
		modelClient = llm.NewOpenAICompatibleClient(nil)
	}
	return &Manager{
		runtimes: make(map[string]*projectRuntime), starts: make(map[string]*runtimeStart),
		stops: make(map[string]*runtimeStop), unavailable: make(map[string]error),
		providers: providers, models: modelsettings.NewResolver(providers), llm: modelClient,
		image: imagegen.NewOpenAICompatibleClient(nil), hub: hub, now: time.Now,
	}
}

func (manager *Manager) WithImageClient(client imagegen.Client) *Manager {
	if client != nil {
		manager.image = client
	}
	return manager
}

type projectRuntime struct {
	store       *project.Store
	projectID   int64
	projectUUID string
	sqlDB       *sql.DB
	client      *river.Client[*sql.Tx]
	manager     *Manager
	cancel      context.CancelFunc
	eventsDone  chan struct{}
	workMu      sync.Mutex
	workCancels map[string]context.CancelFunc
}

func (manager *Manager) StartProject(ctx context.Context, store *project.Store) error {
	if manager == nil || manager.providers == nil {
		return taskError(CodeProjectRuntimeUnavailable, "AI 运行时未配置", "Provider 服务不可用。", nil)
	}
	projectUUID := store.ProjectUUID()
	for {
		manager.mu.Lock()
		if runtime := manager.runtimes[projectUUID]; runtime != nil {
			err := manager.unavailable[projectUUID]
			manager.mu.Unlock()
			if err != nil {
				return taskError(CodeProjectRuntimeUnavailable, "项目 AI 运行时不可用", "该项目运行时停止失败，请重启 Lumi 后重试。", err)
			}
			return nil
		}
		if call := manager.starts[projectUUID]; call != nil {
			done := call.done
			manager.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				return call.err
			}
		}
		if call := manager.stops[projectUUID]; call != nil {
			done := call.done
			manager.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				continue
			}
		}
		call := &runtimeStart{done: make(chan struct{})}
		manager.starts[projectUUID] = call
		manager.mu.Unlock()

		runtime, err := manager.openRuntime(ctx, store)
		manager.mu.Lock()
		call.err = err
		if err == nil {
			manager.runtimes[projectUUID] = runtime
			delete(manager.unavailable, projectUUID)
		}
		delete(manager.starts, projectUUID)
		close(call.done)
		manager.mu.Unlock()
		return err
	}
}

func (manager *Manager) openRuntime(ctx context.Context, store *project.Store) (*projectRuntime, error) {
	sqlDB, err := store.DB().DB()
	if err != nil {
		return nil, fmt.Errorf("get project database for River: %w", err)
	}
	if sqlDB.Stats().MaxOpenConnections != 1 {
		return nil, taskError(CodeProjectRuntimeUnavailable, "项目数据库连接边界不安全", "River SQLite 运行时要求 MaxOpenConns(1)。", nil)
	}
	driver := riversqlite.New(sqlDB)
	if err := migrateRiver(ctx, store, driver); err != nil {
		return nil, err
	}
	var projectID int64
	if err := store.DB().WithContext(ctx).Raw("SELECT id FROM projects WHERE uuid = ?", store.ProjectUUID()).Scan(&projectID).Error; err != nil || projectID == 0 {
		return nil, fmt.Errorf("resolve project identity for runtime: %w", err)
	}
	if err := reconcileProductTasks(ctx, sqlDB, projectID, manager.now().UTC()); err != nil {
		return nil, err
	}
	runtimeCtx, cancel := context.WithCancel(context.Background())
	runtime := &projectRuntime{store: store, projectID: projectID, projectUUID: store.ProjectUUID(), sqlDB: sqlDB, manager: manager, cancel: cancel, eventsDone: make(chan struct{}), workCancels: make(map[string]context.CancelFunc)}
	workers := river.NewWorkers()
	river.AddWorker(workers, &storyGenerationWorker{runtime: runtime})
	river.AddWorker(workers, &assetMaintenanceWorker{runtime: runtime})
	river.AddWorker(workers, &productionWorker{runtime: runtime})
	if manager.agents != nil {
		river.AddWorker(workers, &agentWorker{runtime: runtime, service: manager.agents})
	}
	logger := slog.Default().With("component", "river", "project_uuid", store.ProjectUUID(), "river_version", RiverVersion)
	client, err := river.NewClient(driver, &river.Config{
		Queues: map[string]river.QueueConfig{QueueStory: {MaxWorkers: 1}, QueueAssetMaintenance: {MaxWorkers: 1}, QueueProduction: {MaxWorkers: 1}, QueueAgent: {MaxWorkers: 1}}, Workers: workers,
		MaxAttempts: 3, PollOnly: true, FetchPollInterval: 200 * time.Millisecond,
		JobTimeout: 5 * time.Minute, RescueStuckJobsAfter: 6 * time.Minute,
		SoftStopTimeout: 5 * time.Second,
		Logger:          logger,
		ErrorHandler:    &riverErrorLogger{logger: logger},
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create River client: %w", err)
	}
	runtime.client = client
	events, unsubscribe := client.Subscribe(river.EventKindJobCompleted, river.EventKindJobFailed, river.EventKindJobCancelled, river.EventKindJobSnoozed)
	go runtime.consumeRiverEvents(runtimeCtx, events, unsubscribe)
	if err := client.Start(runtimeCtx); err != nil {
		cancel()
		unsubscribe()
		<-runtime.eventsDone
		return nil, fmt.Errorf("start River client: %w", err)
	}
	return runtime, nil
}

func (manager *Manager) StopProject(ctx context.Context, projectUUID string) error {
	for {
		manager.mu.Lock()
		if call := manager.starts[projectUUID]; call != nil {
			done := call.done
			manager.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				continue
			}
		}
		if call := manager.stops[projectUUID]; call != nil {
			done := call.done
			manager.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				return call.err
			}
		}
		runtime := manager.runtimes[projectUUID]
		if runtime == nil {
			manager.mu.Unlock()
			return nil
		}
		call := &runtimeStop{done: make(chan struct{})}
		manager.stops[projectUUID] = call
		manager.mu.Unlock()

		stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
		err := runtime.client.Stop(stopCtx)
		runtime.cancel()
		select {
		case <-runtime.eventsDone:
		case <-stopCtx.Done():
			err = errors.Join(err, stopCtx.Err())
		}
		stopCancel()

		manager.mu.Lock()
		call.err = err
		if err == nil {
			delete(manager.runtimes, projectUUID)
			delete(manager.unavailable, projectUUID)
		} else {
			manager.unavailable[projectUUID] = err
		}
		delete(manager.stops, projectUUID)
		close(call.done)
		manager.mu.Unlock()
		return err
	}
}

func (manager *Manager) HasActiveWork(ctx context.Context, projectUUID string) (bool, error) {
	manager.mu.RLock()
	runtime := manager.runtimes[projectUUID]
	manager.mu.RUnlock()
	if runtime == nil {
		return false, nil
	}
	return hasActiveProjectWork(ctx, runtime.sqlDB, runtime.projectID)
}

func hasActiveProjectWork(ctx context.Context, db *sql.DB, projectID int64) (bool, error) {
	var active bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM task_runs WHERE project_id = ? AND status IN ('queued', 'running')
			UNION ALL
			SELECT 1 FROM asset_maintenance_runs WHERE project_id = ? AND status IN ('queued', 'running')
			UNION ALL
			SELECT 1 FROM production_task_runs WHERE project_id = ? AND status IN ('queued', 'running')
			UNION ALL
			SELECT 1 FROM chat_turns turns
			JOIN chat_threads threads ON threads.id = turns.thread_id
			WHERE threads.project_id = ? AND turns.status IN ('queued', 'in_progress')
			UNION ALL
			SELECT 1 FROM workflows WHERE project_id = ? AND status IN ('queued', 'running')
			LIMIT 1
		)
	`, projectID, projectID, projectID, projectID, projectID).Scan(&active)
	return active, err
}

func (manager *Manager) runtimeFor(projectUUID string) (*projectRuntime, error) {
	if !isUUIDv7(projectUUID) {
		return nil, taskError(CodeInvalidTask, "项目 UUID 无效", "项目资源标识必须是 UUIDv7。", nil)
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	if manager.runtimes[projectUUID] == nil {
		return nil, taskError(CodeProjectRuntimeUnavailable, "项目 AI 运行时未启动", "请先打开该项目。", nil)
	}
	if err := manager.unavailable[projectUUID]; err != nil {
		return nil, taskError(CodeProjectRuntimeUnavailable, "项目 AI 运行时不可用", "该项目运行时停止失败，请重启 Lumi 后重试。", err)
	}
	if manager.stops[projectUUID] != nil {
		return nil, taskError(CodeProjectRuntimeUnavailable, "项目 AI 运行时正在关闭", "请重新打开项目后重试。", nil)
	}
	return manager.runtimes[projectUUID], nil
}

func (runtime *projectRuntime) broadcast(event string, task Task) {
	if runtime.manager.hub == nil {
		return
	}
	runtime.manager.hub.Broadcast(realtime.ProjectTopic(runtime.projectUUID), event, taskRealtimePayload(runtime.projectUUID, task))
}

// taskRealtimePayload stays independent from River's job row so the public
// realtime projection cannot accidentally inherit an internal queue ID.
func taskRealtimePayload(projectUUID string, task Task) map[string]any {
	return map[string]any{
		"project_uuid": projectUUID, "task_uuid": task.UUID, "kind": task.Kind,
		"resource_uuid": task.ResourceUUID, "status": task.Status, "progress": task.Progress,
		"attempt": task.Attempt, "error_code": task.ErrorCode, "error_message": task.ErrorMessage,
	}
}

func (runtime *projectRuntime) consumeRiverEvents(ctx context.Context, events <-chan *river.Event, unsubscribe func()) {
	defer close(runtime.eventsDone)
	defer unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if event == nil || event.Job == nil {
				continue
			}
			if err := runtime.projectRiverEvent(context.WithoutCancel(ctx), event); err != nil {
				slog.Error("project River event projection failed", "project_uuid", runtime.projectUUID, "event", event.Kind, "error", err)
			}
		}
	}
}

func (runtime *projectRuntime) registerWork(taskUUID string, cancel context.CancelFunc) {
	runtime.workMu.Lock()
	defer runtime.workMu.Unlock()
	runtime.workCancels[taskUUID] = cancel
}

func (runtime *projectRuntime) unregisterWork(taskUUID string) {
	runtime.workMu.Lock()
	defer runtime.workMu.Unlock()
	delete(runtime.workCancels, taskUUID)
}

func (runtime *projectRuntime) cancelWork(taskUUID string) {
	runtime.workMu.Lock()
	cancel := runtime.workCancels[taskUUID]
	runtime.workMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func migrateRiver(ctx context.Context, store *project.Store, driver *riversqlite.Driver) error {
	migrator, err := rivermigrate.New(driver, nil)
	if err != nil {
		return taskError(CodeProjectRuntimeUnavailable, "无法初始化 River migration", "项目队列尚未启动。", err)
	}
	existing, err := migrator.ExistingVersions(ctx)
	if err != nil {
		return taskError(CodeProjectRuntimeUnavailable, "无法读取 River migration", "项目队列尚未启动。", err)
	}
	allVersions := migrator.AllVersions()
	needsMigration := len(existing) != len(allVersions)
	if !needsMigration {
		for index := range existing {
			if existing[index].Version != allVersions[index].Version {
				needsMigration = true
				break
			}
		}
	}
	if needsMigration {
		backupUUID, uuidErr := uuid.NewV7()
		if uuidErr != nil {
			return uuidErr
		}
		backupPath := filepath.Join(store.Root(), ".lumi", "backups", fmt.Sprintf("project-before-river-%s-%s.sqlite", time.Now().UTC().Format("20060102T150405.000000000Z"), backupUUID.String()))
		if err := database.OnlineBackup(ctx, config.SQLiteDSN(filepath.Join(store.Root(), "project.sqlite")), backupPath); err != nil {
			return taskError(CodeProjectRuntimeUnavailable, "无法创建 River migration 备份", "River migration 尚未执行。", err)
		}
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return taskError(CodeProjectRuntimeUnavailable, "River migration 失败", "项目队列尚未启动；Story 手动功能仍可使用。", err)
	}
	validation, err := migrator.Validate(ctx, nil)
	if err != nil {
		return taskError(CodeProjectRuntimeUnavailable, "无法验证 River schema", "项目队列尚未启动。", err)
	}
	if !validation.OK {
		return taskError(CodeProjectRuntimeUnavailable, "River schema 不完整", strings.Join(validation.Messages, "; "), nil)
	}
	return nil
}

func reconcileProductTasks(ctx context.Context, db *sql.DB, projectID int64, now time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE llm_logs SET status='failed',error_code='provider_call_interrupted',error_message='应用在 Provider 请求期间退出。',completed_at=COALESCE(completed_at,?) WHERE project_id=? AND status='pending'`, now, projectID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE task_runs
		SET status = 'interrupted', error_code = 'unsafe_restart', error_message = '应用在任务执行期间退出，请显式重试。', completed_at = ?, updated_at = ?
		WHERE project_id = ? AND status = 'running' AND retryable = 0`, now, now, projectID); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `UPDATE asset_maintenance_runs SET status='queued',progress=0,updated_at=?,error_code='',error_message='' WHERE project_id=? AND status='running'`, now, projectID)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE production_task_runs SET status='queued',progress=0,updated_at=?,error_code='',error_message='' WHERE project_id=? AND status='running'`, now, projectID)
	if err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `UPDATE premise_generation_steps SET status='queued' WHERE status='running' AND task_uuid IN (SELECT uuid FROM production_task_runs WHERE project_id=? AND status='queued')`, projectID); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `UPDATE comic_image_generations SET status='queued' WHERE status='running' AND task_uuid IN (SELECT uuid FROM production_task_runs WHERE project_id=? AND status='queued')`, projectID); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `UPDATE comic_exports SET status='queued' WHERE status='running' AND task_uuid IN (SELECT uuid FROM production_task_runs WHERE project_id=? AND status='queued')`, projectID); err != nil {
		return err
	}
	if err := reconcileComicImageWorkflows(ctx, db, projectID, now); err != nil {
		return err
	}
	return reconcileComicStoryboardWorkflows(ctx, db, projectID, now)
}
