package providerlimit

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"lumi/internal/provider"
	"lumi/internal/providerdiag"

	"golang.org/x/time/rate"
)

type RequestClass string

const (
	RequestText  RequestClass = "text"
	RequestImage RequestClass = "image"
)

const (
	defaultRequestsPerSecond = 3
	defaultTextConcurrency   = 3
	defaultImageConcurrency  = 1
	defaultImageMaximum      = 2
	defaultImageSuccesses    = 5
	defaultCooldown          = time.Minute
	defaultMaximumCooldown   = 5 * time.Minute
)

type Lease interface {
	Finish(error)
}

type Limiter interface {
	Acquire(context.Context, string, string, RequestClass) (Lease, error)
}

type WaitFunc func(context.Context, time.Duration, <-chan struct{}) error

type Config struct {
	RequestsPerSecond rate.Limit
	Burst             int
	TextConcurrency   int
	ImageConcurrency  int
	ImageMaximum      int
	ImageSuccesses    int
	DefaultCooldown   time.Duration
	MaximumCooldown   time.Duration
	Now               func() time.Time
	Wait              WaitFunc
	Logger            *slog.Logger
}

func DefaultConfig() Config {
	return Config{
		RequestsPerSecond: defaultRequestsPerSecond,
		Burst:             1,
		TextConcurrency:   defaultTextConcurrency,
		ImageConcurrency:  defaultImageConcurrency,
		ImageMaximum:      defaultImageMaximum,
		ImageSuccesses:    defaultImageSuccesses,
		DefaultCooldown:   defaultCooldown,
		MaximumCooldown:   defaultMaximumCooldown,
		Now:               time.Now,
		Wait:              waitContext,
		Logger:            slog.Default(),
	}
}

type Coordinator struct {
	config Config
	mu     sync.Mutex
	pools  map[string]*gatewayPool
}

type gatewayPool struct {
	mu sync.Mutex

	rate *rate.Limiter

	textActive    int
	imageActive   int
	imageLimit    int
	imageSuccess  int
	textCooldown  time.Time
	imageCooldown time.Time
	changed       chan struct{}
}

type lease struct {
	coordinator *Coordinator
	pool        *gatewayPool
	class       RequestClass
	once        sync.Once
}

type noopLease struct{}

func (noopLease) Finish(error) {}

type noopLimiter struct{}

func (noopLimiter) Acquire(context.Context, string, string, RequestClass) (Lease, error) {
	return noopLease{}, nil
}

func Noop() Limiter { return noopLimiter{} }

func NewCoordinator() *Coordinator {
	return NewCoordinatorWithConfig(DefaultConfig())
}

func NewCoordinatorWithConfig(config Config) *Coordinator {
	defaults := DefaultConfig()
	if config.RequestsPerSecond <= 0 {
		config.RequestsPerSecond = defaults.RequestsPerSecond
	}
	if config.Burst <= 0 {
		config.Burst = defaults.Burst
	}
	if config.TextConcurrency <= 0 {
		config.TextConcurrency = defaults.TextConcurrency
	}
	if config.ImageConcurrency <= 0 {
		config.ImageConcurrency = defaults.ImageConcurrency
	}
	if config.ImageMaximum < config.ImageConcurrency {
		config.ImageMaximum = max(defaults.ImageMaximum, config.ImageConcurrency)
	}
	if config.ImageSuccesses <= 0 {
		config.ImageSuccesses = defaults.ImageSuccesses
	}
	if config.DefaultCooldown <= 0 {
		config.DefaultCooldown = defaults.DefaultCooldown
	}
	if config.MaximumCooldown < config.DefaultCooldown {
		config.MaximumCooldown = max(defaults.MaximumCooldown, config.DefaultCooldown)
	}
	if config.Now == nil {
		config.Now = defaults.Now
	}
	if config.Wait == nil {
		config.Wait = defaults.Wait
	}
	if config.Logger == nil {
		config.Logger = defaults.Logger
	}
	return &Coordinator{config: config, pools: make(map[string]*gatewayPool)}
}

func (coordinator *Coordinator) Acquire(ctx context.Context, providerType, baseURL string, class RequestClass) (Lease, error) {
	if coordinator == nil || providerType != provider.TypeCloudflareAIGateway || (class != RequestText && class != RequestImage) {
		return noopLease{}, nil
	}
	pool := coordinator.pool(providerType, baseURL)
	waitStarted := coordinator.config.Now()
	waitReason := ""

	for {
		granted, delay, wake, reason := coordinator.acquireSlot(pool, class)
		if !granted {
			waitReason = joinReason(waitReason, reason)
			if err := coordinator.config.Wait(ctx, delay, wake); err != nil {
				return nil, err
			}
			continue
		}

		now := coordinator.config.Now()
		reservation := pool.rate.ReserveN(now, 1)
		if !reservation.OK() {
			coordinator.releaseSlot(pool, class)
			return nil, errors.New("cloudflare provider request exceeds limiter burst")
		}
		delay = reservation.DelayFrom(now)
		if delay > 0 {
			waitReason = joinReason(waitReason, "rate")
			if err := coordinator.config.Wait(ctx, delay, nil); err != nil {
				reservation.CancelAt(coordinator.config.Now())
				coordinator.releaseSlot(pool, class)
				return nil, err
			}
		}

		if coordinator.cooldownRemaining(pool, class) > 0 {
			coordinator.releaseSlot(pool, class)
			waitReason = joinReason(waitReason, "cooldown")
			continue
		}

		waited := coordinator.config.Now().Sub(waitStarted)
		if waitReason != "" {
			coordinator.config.Logger.InfoContext(ctx, "Cloudflare provider capacity acquired",
				"request_type", class,
				"wait_ms", waited.Milliseconds(),
				"wait_reason", waitReason,
				"image_concurrency", coordinator.imageLimit(pool),
			)
		}
		return &lease{coordinator: coordinator, pool: pool, class: class}, nil
	}
}

func (coordinator *Coordinator) pool(providerType, baseURL string) *gatewayPool {
	key := providerType + "\x00" + normalizeBaseURL(baseURL)
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if existing := coordinator.pools[key]; existing != nil {
		return existing
	}
	created := &gatewayPool{
		rate:       rate.NewLimiter(coordinator.config.RequestsPerSecond, coordinator.config.Burst),
		imageLimit: coordinator.config.ImageConcurrency,
		changed:    make(chan struct{}),
	}
	coordinator.pools[key] = created
	return created
}

func (coordinator *Coordinator) acquireSlot(pool *gatewayPool, class RequestClass) (bool, time.Duration, <-chan struct{}, string) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	now := coordinator.config.Now()
	cooldown := pool.textCooldown
	if class == RequestImage {
		cooldown = pool.imageCooldown
	}
	if cooldown.After(now) {
		return false, cooldown.Sub(now), pool.changed, "cooldown"
	}
	if class == RequestText {
		if pool.textActive >= coordinator.config.TextConcurrency {
			return false, -1, pool.changed, "concurrency"
		}
		pool.textActive++
		return true, 0, nil, ""
	}
	if pool.imageActive >= pool.imageLimit {
		return false, -1, pool.changed, "concurrency"
	}
	pool.imageActive++
	return true, 0, nil, ""
}

func (coordinator *Coordinator) cooldownRemaining(pool *gatewayPool, class RequestClass) time.Duration {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	cooldown := pool.textCooldown
	if class == RequestImage {
		cooldown = pool.imageCooldown
	}
	if now := coordinator.config.Now(); cooldown.After(now) {
		return cooldown.Sub(now)
	}
	return 0
}

func (coordinator *Coordinator) releaseSlot(pool *gatewayPool, class RequestClass) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if class == RequestText {
		if pool.textActive > 0 {
			pool.textActive--
		}
	} else if pool.imageActive > 0 {
		pool.imageActive--
	}
	pool.signalLocked()
}

func (coordinator *Coordinator) imageLimit(pool *gatewayPool) int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	return pool.imageLimit
}

func (item *lease) Finish(err error) {
	if item == nil || item.coordinator == nil || item.pool == nil {
		return
	}
	item.once.Do(func() {
		item.coordinator.finish(item.pool, item.class, err)
	})
}

func (coordinator *Coordinator) finish(pool *gatewayPool, class RequestClass, callErr error) {
	diagnostic := providerdiag.FromError(callErr)
	rateLimited := diagnostic.HTTPStatus == 429
	now := coordinator.config.Now()

	pool.mu.Lock()
	if class == RequestText {
		if pool.textActive > 0 {
			pool.textActive--
		}
	} else {
		if pool.imageActive > 0 {
			pool.imageActive--
		}
		switch {
		case rateLimited:
			pool.imageSuccess = 0
			pool.imageLimit = coordinator.config.ImageConcurrency
		case callErr == nil:
			pool.imageSuccess++
			if pool.imageLimit < coordinator.config.ImageMaximum && pool.imageSuccess >= coordinator.config.ImageSuccesses {
				pool.imageLimit = coordinator.config.ImageMaximum
				pool.imageSuccess = 0
			}
		default:
			pool.imageSuccess = 0
		}
	}

	cooldown := time.Duration(0)
	if rateLimited {
		cooldown = diagnostic.RetryAfter
		if cooldown <= 0 {
			cooldown = coordinator.config.DefaultCooldown
		}
		if cooldown > coordinator.config.MaximumCooldown {
			cooldown = coordinator.config.MaximumCooldown
		}
		until := now.Add(cooldown)
		if class == RequestText && until.After(pool.textCooldown) {
			pool.textCooldown = until
		}
		if class == RequestImage && until.After(pool.imageCooldown) {
			pool.imageCooldown = until
		}
	}
	imageLimit := pool.imageLimit
	pool.signalLocked()
	pool.mu.Unlock()

	if rateLimited {
		coordinator.config.Logger.Warn("Cloudflare provider rate limited",
			"request_type", class,
			"cooldown_ms", cooldown.Milliseconds(),
			"image_concurrency", imageLimit,
		)
	}
}

func (pool *gatewayPool) signalLocked() {
	close(pool.changed)
	pool.changed = make(chan struct{})
}

func normalizeBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func joinReason(existing, next string) string {
	if existing == "" {
		return next
	}
	for _, item := range strings.Split(existing, ",") {
		if item == next {
			return existing
		}
	}
	return existing + "," + next
}

func waitContext(ctx context.Context, duration time.Duration, wake <-chan struct{}) error {
	if duration == 0 {
		return nil
	}
	if duration < 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
			return nil
		}
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wake:
		return nil
	case <-timer.C:
		return nil
	}
}
