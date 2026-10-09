package modeltest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/provider"
	"lumi/internal/providerlimit"
	"lumi/internal/realtime"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TextClient interface {
	llm.ConnectionChecker
	Generate(context.Context, llm.Request, func(string) error) (llm.Response, error)
}

type Publisher interface{ Broadcast(string, string, any) }

// Service owns one worker for application-wide diagnostic tests. Provider
// requests are never tied to an HTTP request's lifetime or automatically replayed.
type Service struct {
	app       *appstore.Store
	providers *provider.Service
	text      TextClient
	images    imagegen.Client
	limiter   providerlimit.Limiter
	events    Publisher
	mu        sync.Mutex
	closed    bool
	cancel    context.CancelFunc
	active    map[string]context.CancelFunc
	done      chan struct{}
	wake      chan struct{}
	timeouts  [3]time.Duration
}

func NewService(app *appstore.Store, providers *provider.Service, text TextClient, images imagegen.Client, limiter providerlimit.Limiter, events Publisher) *Service {
	if limiter == nil {
		limiter = providerlimit.Noop()
	}
	return &Service{app: app, providers: providers, text: text, images: images, limiter: limiter, events: events,
		wake: make(chan struct{}, 1), active: make(map[string]context.CancelFunc), timeouts: [3]time.Duration{time.Minute, 2 * time.Minute, imagegen.RequestTimeout}}
}

func validUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.Version() == 7 && parsed.String() == value
}

func validateSelection(value Selection) bool {
	return validUUID(value.ProviderUUID) && strings.TrimSpace(value.Model) != "" && value.Model == strings.TrimSpace(value.Model) && len(value.Model) <= 255
}

func (s *Service) Create(ctx context.Context, input ExecutionInput) (Test, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Test{}, ErrClosed
	}
	textProvider, err := s.resolve(ctx, input.TextModel, false)
	if err != nil {
		return Test{}, err
	}
	imageProvider, err := s.resolve(ctx, input.ImageModel, true)
	if err != nil {
		return Test{}, err
	}
	now := time.Now().UTC()
	value, err := uuid.NewV7()
	if err != nil {
		return Test{}, err
	}
	row := Test{UUID: value.String(), Status: "queued", CreatedAt: now, UpdatedAt: now,
		TextModel: frozenModel(input.TextModel, textProvider), ImageModel: frozenModel(input.ImageModel, imageProvider)}
	err = s.app.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for i, kind := range []string{"connection", "text", "image"} {
			stepUUID, err := uuid.NewV7()
			if err != nil {
				return err
			}
			step := Step{UUID: stepUUID.String(), ModelTestID: row.ID, Position: i + 1, Kind: kind, Status: "pending"}
			if err := tx.Create(&step).Error; err != nil {
				return err
			}
			row.Steps = append(row.Steps, step)
		}
		return nil
	})
	if err == nil {
		s.queued(row.UUID)
	}
	return row, err
}

func (s *Service) Get(ctx context.Context, testUUID string) (Test, error) {
	if !validUUID(testUUID) {
		return Test{}, ErrNotFound
	}
	var row Test
	err := s.app.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("uuid = ?", testUUID).Take(&row).Error; err != nil {
			return err
		}
		return tx.Where("model_test_id = ?", row.ID).Order("position").Find(&row.Steps).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Test{}, ErrNotFound
	}
	for i := range row.Steps {
		step := &row.Steps[i]
		if step.Status == "succeeded" && step.ImageMIMEType != "" {
			step.ContentURL = fmt.Sprintf("/media/model-tests/%s/steps/%s/content", row.UUID, step.UUID)
		}
	}
	return row, err
}

func (s *Service) List(ctx context.Context, page, perPage int) (Page, error) {
	if page < 1 || page > 1000000 || perPage < 1 || perPage > 100 {
		return Page{}, ErrInvalid
	}
	result := Page{Items: []Test{}, Pagination: Pagination{PerPage: perPage, CurrentPage: page}}
	err := s.app.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Test{}).Where("status <> 'draft'").Count(&result.Pagination.Total).Error; err != nil {
			return err
		}
		result.Pagination.LastPage = max(1, int((result.Pagination.Total+int64(perPage)-1)/int64(perPage)))
		return tx.Where("status <> 'draft'").Order("created_at DESC, id DESC").Limit(perPage).Offset((page - 1) * perPage).Find(&result.Items).Error
	})
	return result, err
}

func (s *Service) resolve(ctx context.Context, selection Selection, image bool) (provider.Resolved, error) {
	if !validateSelection(selection) {
		return provider.Resolved{}, ErrInvalid
	}
	resolved, err := s.providers.Resolve(ctx, selection.ProviderUUID)
	if err != nil {
		return resolved, err
	}
	models := provider.SupportedTextModels(resolved.Provider)
	if image {
		models = provider.SupportedImageModels(resolved.Provider)
	}
	if !slices.Contains(models, selection.Model) {
		return provider.Resolved{}, ErrInvalid
	}
	return resolved, nil
}

func frozenModel(selection Selection, resolved provider.Resolved) Model {
	return Model{Selection: selection, ProviderType: resolved.ProviderType, ProviderName: resolved.DisplayName}
}

func (s *Service) Execute(ctx context.Context, testUUID string, input ExecutionInput) (Test, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Test{}, ErrClosed
	}
	row, err := s.Get(ctx, testUUID)
	if err != nil {
		return Test{}, err
	}
	if row.Status != "draft" {
		if row.TextModel.Selection != input.TextModel || row.ImageModel.Selection != input.ImageModel {
			return Test{}, ErrConflict
		}
		return row, nil
	}
	textProvider, err := s.resolve(ctx, input.TextModel, false)
	if err != nil {
		return Test{}, err
	}
	imageProvider, err := s.resolve(ctx, input.ImageModel, true)
	if err != nil {
		return Test{}, err
	}
	row.TextModel, row.ImageModel = frozenModel(input.TextModel, textProvider), frozenModel(input.ImageModel, imageProvider)
	row.Status, row.UpdatedAt = "queued", time.Now().UTC()
	result := s.app.DB().WithContext(ctx).Model(&Test{}).Where("id = ? AND status = 'draft'", row.ID).
		Updates(map[string]any{"status": row.Status, "updated_at": row.UpdatedAt,
			"text_provider_uuid": row.TextModel.ProviderUUID, "text_model": row.TextModel.Model,
			"text_provider_type": row.TextModel.ProviderType, "text_provider_name": row.TextModel.ProviderName,
			"image_provider_uuid": row.ImageModel.ProviderUUID, "image_model": row.ImageModel.Model,
			"image_provider_type": row.ImageModel.ProviderType, "image_provider_name": row.ImageModel.ProviderName})
	if result.Error != nil {
		return Test{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Test{}, ErrConflict
	}
	s.queued(testUUID)
	return row, nil
}

func (s *Service) queued(testUUID string) {
	s.changed(testUUID)
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Stop is idempotent and preserves completed steps. Persist the interruption
// before cancelling the request so late provider results cannot overwrite it.
func (s *Service) Stop(ctx context.Context, testUUID string) (Test, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Test{}, ErrClosed
	}
	row, err := s.Get(ctx, testUUID)
	if err != nil {
		return Test{}, err
	}
	if row.Status != "queued" && row.Status != "running" {
		return row, nil
	}
	err = s.app.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return interruptTest(tx, row.ID, time.Now().UTC())
	})
	if err != nil {
		return Test{}, err
	}
	if cancel := s.active[testUUID]; cancel != nil {
		cancel()
	}
	s.changed(testUUID)
	return s.Get(ctx, testUUID)
}

func (s *Service) changed(testUUID string) {
	if s.events != nil {
		s.events.Broadcast(realtime.SystemTopic, EventChanged, map[string]any{"test_uuid": testUUID})
	}
}

// Start reconciles abandoned requests before allowing the worker to receive new
// ones. A crash can leave an upstream charge uncertain, so recovery never retries.
func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if s.cancel != nil {
		return nil
	}
	if err := s.interruptOutstanding(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	go func() {
		defer close(s.done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
				for ctx.Err() == nil {
					var row Test
					err := s.app.DB().WithContext(ctx).Where("status = 'queued'").Order("updated_at,id").Take(&row).Error
					if errors.Is(err, gorm.ErrRecordNotFound) {
						break
					}
					if err == nil {
						err = s.run(ctx, row.UUID)
					}
					if err != nil {
						slog.Error("model test worker storage failure", "error", err)
						if recoveryErr := s.interruptOutstanding(); recoveryErr != nil {
							slog.Error("model test interruption failed", "error", recoveryErr)
						}
						break
					}
				}
			}
		}
	}()
	return nil
}

func (s *Service) Close() error {
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	done := s.done
	s.mu.Unlock()
	if done != nil {
		<-done
	}
	return s.interruptOutstanding()
}

func (s *Service) interruptOutstanding() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var rows []Test
	now := time.Now().UTC()
	err := s.app.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("status IN ?", []string{"queued", "running"}).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := interruptTest(tx, row.ID, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		for _, row := range rows {
			s.changed(row.UUID)
		}
	}
	return err
}

func interruptTest(tx *gorm.DB, testID int64, now time.Time) error {
	if err := tx.Model(&Step{}).Where("model_test_id = ? AND status = 'running'", testID).Updates(map[string]any{
		"status": "interrupted", "error_code": "model_test_interrupted", "completed_at": now,
		"duration_ms": gorm.Expr("MAX(0, CAST((julianday(?) - julianday(started_at)) * 86400000 AS INTEGER))", now),
	}).Error; err != nil {
		return err
	}
	if err := tx.Model(&Step{}).Where("model_test_id = ? AND status = 'pending'", testID).
		Updates(map[string]any{"status": "skipped", "completed_at": now}).Error; err != nil {
		return err
	}
	return tx.Model(&Test{}).Where("id = ? AND status IN ?", testID, []string{"queued", "running"}).
		Updates(map[string]any{"status": "interrupted", "updated_at": now, "completed_at": now}).Error
}
