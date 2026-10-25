package agent

// This adapter substitutes only the environment. All turns still run through
// ExecuteJob, durable tool intents, context reconstruction, repair and budgets.
import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm/logger"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/llm"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/taskexecution"
)

// IsolatedEnvironment must expose only fixture data and simulated tools. Names
// reserved by the business runtime are rejected by NewIsolatedSession.
type IsolatedEnvironment interface {
	Tools() []llm.ToolDefinition
	Validate(name, arguments string) (map[string]any, error)
	Execute(context.Context, string, map[string]any) (any, error)
}

type IsolatedConfig struct {
	Request      llm.ChatRequest
	SystemPrompt string
	MaxRequests  int
	Environment  IsolatedEnvironment
	// Opt-in names used by the historical reference fixtures. Dispatch still
	// goes exclusively to Environment, never to production file tools.
	ReferenceFileTools bool
}

type isolatedRuntime struct {
	request     llm.ChatRequest
	provider    provider.Resolved
	prompt      string
	environment IsolatedEnvironment
}

// IsolatedSession owns an entirely new app/project store. It never opens the
// user's project manager or imports business fixtures. Close removes its files.
type IsolatedSession struct {
	root     string
	app      *appstore.Store
	projects *project.Manager
	store    *project.Store
	service  *Service
	thread   Thread
}

func NewIsolatedSession(ctx context.Context, cfg IsolatedConfig, model llm.ToolClient) (_ *IsolatedSession, err error) {
	if model == nil || cfg.Environment == nil || strings.TrimSpace(cfg.SystemPrompt) == "" || cfg.Request.Model == "" || cfg.MaxRequests < 2 {
		return nil, fmt.Errorf("isolated session requires model, environment, prompt and at least two requests")
	}
	for _, tool := range cfg.Environment.Tools() {
		referenceName := tool.Name == "read" || tool.Name == "write" || tool.Name == "edit" || tool.Name == "ls" || tool.Name == "find" || tool.Name == "grep"
		if !strings.HasPrefix(tool.Name, "eval_") && !(cfg.ReferenceFileTools && referenceName) {
			return nil, fmt.Errorf("isolated tools must use the eval_ prefix: %s", tool.Name)
		}
	}
	s := &IsolatedSession{}
	s.root, err = os.MkdirTemp("", "lumi-agent-eval-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()
	appDir := filepath.Join(s.root, "app")
	s.app, err = appstore.Open(appDir, config.SQLiteDSN(filepath.Join(appDir, "lumi.sqlite")))
	if err != nil {
		return nil, err
	}
	s.projects = project.NewManager(s.app)
	created, err := s.projects.CreateWithInput(ctx, project.CreateInput{Name: "Isolated agent evaluation"}, project.ExplicitNewProjectParent(s.root))
	if err != nil {
		return nil, err
	}
	if err = s.projects.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error { s.store = store; return nil }); err != nil {
		return nil, err
	}
	s.store.DB().Logger = logger.Default.LogMode(logger.Silent)
	providerUUID, err := newUUIDv7()
	if err != nil {
		return nil, err
	}
	resolved := provider.Resolved{Provider: provider.Provider{UUID: providerUUID, ProviderType: cfg.Request.ProviderType, BaseURL: cfg.Request.BaseURL, DefaultModel: cfg.Request.Model}, APIKey: cfg.Request.APIKey}
	s.service = NewService(s.projects, nil, model, &isolatedQueue{}, nil)
	s.service.isolation = &isolatedRuntime{request: cfg.Request, provider: resolved, prompt: cfg.SystemPrompt, environment: cfg.Environment}
	s.service.turnBudget.MaxModelRequests = cfg.MaxRequests
	s.thread, err = s.service.CreateThread(ctx, created.UUID, CreateThreadInput{Title: "Evaluation"})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *IsolatedSession) Prompt(ctx context.Context, input string) (Turn, error) {
	turn, err := s.service.CreateTurn(ctx, s.store.ProjectUUID(), s.thread.UUID, CreateTurnInput{InputText: input})
	if err != nil {
		return turn, err
	}
	err = s.service.ExecuteJob(ctx, s.store, JobSpec{Version: 1, ProjectUUID: s.store.ProjectUUID(), ThreadUUID: s.thread.UUID, ResourceUUID: turn.UUID, JobKind: JobChatTurn})
	var row turnRecord
	readErr := s.store.DB().WithContext(context.WithoutCancel(ctx)).Where("uuid=?", turn.UUID).First(&row).Error
	if readErr != nil {
		return turn, errors.Join(err, readErr)
	}
	return turnDTO(row, s.thread.UUID, ""), err
}

// Items includes every persisted item, including errors and tool arguments.
func (s *IsolatedSession) Items(ctx context.Context) ([]Item, error) {
	var rows []itemRecord
	if err := s.store.DB().WithContext(ctx).Order("sequence").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	var turns []turnRecord
	var runs []runRecord
	if err := s.store.DB().WithContext(ctx).Find(&turns).Error; err != nil {
		return nil, err
	}
	if err := s.store.DB().WithContext(ctx).Find(&runs).Error; err != nil {
		return nil, err
	}
	turnUUIDs, runUUIDs := map[int64]string{}, map[int64]string{}
	for _, turn := range turns {
		turnUUIDs[turn.ID] = turn.UUID
	}
	for _, run := range runs {
		runUUIDs[run.ID] = run.UUID
	}
	for _, row := range rows {
		var turnUUID, runUUID string
		if row.TurnID != nil {
			turnUUID = turnUUIDs[*row.TurnID]
		}
		if row.RunID != nil {
			runUUID = runUUIDs[*row.RunID]
		}
		items = append(items, itemDTO(row, s.thread.UUID, turnUUID, runUUID))
	}
	return items, nil
}

func (s *IsolatedSession) Close() error {
	var errs []error
	if s.projects != nil {
		errs = append(errs, s.projects.Close())
	}
	if s.app != nil {
		errs = append(errs, s.app.Close())
	}
	if s.root != "" {
		errs = append(errs, os.RemoveAll(s.root))
	}
	return errors.Join(errs...)
}

// No worker or domain queue is started. Prompt dispatches the recorded turn
// synchronously; tool execution routes exclusively to the isolated environment.
type isolatedQueue struct {
	Queue
	next int64
}

func (q *isolatedQueue) EnqueueAgentTx(context.Context, string, *sql.Tx, JobSpec) (taskexecution.Binding, error) {
	q.next++
	return taskexecution.Binding{JobID: q.next}, nil
}
