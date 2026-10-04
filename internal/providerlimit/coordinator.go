package providerlimit

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"lumi/internal/provider"
	"lumi/internal/providerdiag"
)

type RequestClass string

const (
	RequestText  RequestClass = "text"
	RequestImage RequestClass = "image"
)

const (
	defaultTextConcurrency  = 3
	defaultImageConcurrency = 1
	defaultImageMaximum     = 2
	defaultImageSuccesses   = 5
	defaultCooldown         = time.Minute
	defaultMaximumCooldown  = 5 * time.Minute
)

type ModelLimit struct {
	Scope        string
	ProviderType string
	Model        string
	RPM          int
	RPS          int
}

const (
	ScopeProvider = "provider"
	ScopeModel    = "model"
)

type Lease interface {
	Finish(error)
}

type Limiter interface {
	Acquire(context.Context, string, string, string, RequestClass) (Lease, error)
}

type WaitFunc func(context.Context, time.Duration, <-chan struct{}) error

type Config struct {
	TextConcurrency  int
	ImageConcurrency int
	ImageMaximum     int
	ImageSuccesses   int
	DefaultCooldown  time.Duration
	MaximumCooldown  time.Duration
	Now              func() time.Time
	Wait             WaitFunc
	Logger           *slog.Logger
}

func DefaultConfig() Config {
	return Config{
		TextConcurrency:  defaultTextConcurrency,
		ImageConcurrency: defaultImageConcurrency,
		ImageMaximum:     defaultImageMaximum,
		ImageSuccesses:   defaultImageSuccesses,
		DefaultCooldown:  defaultCooldown,
		MaximumCooldown:  defaultMaximumCooldown,
		Now:              time.Now,
		Wait:             waitContext,
		Logger:           slog.Default(),
	}
}

type Coordinator struct {
	config   Config
	mu       sync.Mutex
	pools    map[string]*gatewayPool
	windows  map[string]*requestWindow
	limits   map[string]ModelLimit
	revision int
}

type gatewayPool struct {
	providerType  string
	textActive    int
	imageActive   int
	imageLimit    int
	imageSuccess  int
	textCooldown  time.Time
	imageCooldown time.Time
	changed       chan struct{}
}

type requestWindow struct {
	starts []time.Time
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

func (noopLimiter) Acquire(context.Context, string, string, string, RequestClass) (Lease, error) {
	return noopLease{}, nil
}

func Noop() Limiter { return noopLimiter{} }

func NewCoordinator() *Coordinator { return NewCoordinatorWithConfig(DefaultConfig()) }

func NewCoordinatorWithConfig(config Config) *Coordinator {
	defaults := DefaultConfig()
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
	return &Coordinator{config: config, pools: make(map[string]*gatewayPool), windows: make(map[string]*requestWindow), limits: make(map[string]ModelLimit)}
}

func providerLimitKey(providerType string) string { return ScopeProvider + "\x00" + providerType }

func modelLimitKey(providerType, model string) string {
	return ScopeModel + "\x00" + providerType + "\x00" + model
}

// ReplaceLimits applies a committed settings snapshot and wakes pending requests.
// Request history is retained so lowering a limit cannot grant a fresh burst.
func (coordinator *Coordinator) ReplaceLimits(revision int, limits []ModelLimit) {
	if coordinator == nil {
		return
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if revision < coordinator.revision {
		return
	}
	next := make(map[string]ModelLimit, len(limits))
	for _, limit := range limits {
		if limit.RPM > 0 || limit.RPS > 0 {
			key := modelLimitKey(limit.ProviderType, limit.Model)
			if limit.Scope == ScopeProvider {
				key = providerLimitKey(limit.ProviderType)
			}
			next[key] = limit
		}
	}
	coordinator.limits = next
	coordinator.revision = revision
	for _, pool := range coordinator.pools {
		pool.signalLocked()
	}
}

func (coordinator *Coordinator) Acquire(ctx context.Context, providerType, baseURL, model string, class RequestClass) (Lease, error) {
	if coordinator == nil || (providerType != provider.TypeCloudflareAIGateway && providerType != provider.TypeAliyunBailian) || (class != RequestText && class != RequestImage) {
		return noopLease{}, nil
	}
	waitStarted := coordinator.config.Now()
	waitReason := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		coordinator.mu.Lock()
		pool := coordinator.poolLocked(providerType, baseURL)
		now := coordinator.config.Now()
		providerKey := providerLimitKey(providerType)
		modelKey := modelLimitKey(providerType, model)
		providerWindow := coordinator.windowLocked(providerKey, now)
		modelWindow := coordinator.windowLocked(modelKey, now)
		delay := providerWindow.delay(now, coordinator.limits[providerKey])
		if modelDelay := modelWindow.delay(now, coordinator.limits[modelKey]); modelDelay > delay {
			delay = modelDelay
		}
		reason := ""
		if delay > 0 {
			reason = joinReason(reason, "rate")
		}
		if providerType == provider.TypeCloudflareAIGateway {
			cooldown := pool.textCooldown
			if class == RequestImage {
				cooldown = pool.imageCooldown
			}
			if cooldown.After(now) {
				if remaining := cooldown.Sub(now); remaining > delay {
					delay = remaining
				}
				reason = joinReason(reason, "cooldown")
			}
			if (class == RequestText && pool.textActive >= coordinator.config.TextConcurrency) || (class == RequestImage && pool.imageActive >= pool.imageLimit) {
				reason = joinReason(reason, "concurrency")
				if delay == 0 {
					delay = -1
				}
			}
		}
		if reason == "" {
			providerWindow.starts = append(providerWindow.starts, now)
			modelWindow.starts = append(modelWindow.starts, now)
			if providerType == provider.TypeCloudflareAIGateway {
				if class == RequestText {
					pool.textActive++
				} else {
					pool.imageActive++
				}
			}
			imageLimit := pool.imageLimit
			coordinator.mu.Unlock()
			if waitReason != "" {
				coordinator.config.Logger.InfoContext(ctx, "Provider model capacity acquired", "provider_type", providerType, "model", model, "request_type", class, "wait_ms", coordinator.config.Now().Sub(waitStarted).Milliseconds(), "wait_reason", waitReason, "image_concurrency", imageLimit)
			}
			return &lease{coordinator: coordinator, pool: pool, class: class}, nil
		}
		for _, item := range strings.Split(reason, ",") {
			waitReason = joinReason(waitReason, item)
		}
		wake := pool.changed
		coordinator.mu.Unlock()
		if err := coordinator.config.Wait(ctx, delay, wake); err != nil {
			return nil, err
		}
	}
}

func (coordinator *Coordinator) windowLocked(key string, now time.Time) *requestWindow {
	window := coordinator.windows[key]
	if window == nil {
		window = &requestWindow{}
		coordinator.windows[key] = window
	}
	window.prune(now)
	return window
}

func (coordinator *Coordinator) pool(providerType, baseURL string) *gatewayPool {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.poolLocked(providerType, baseURL)
}

func (coordinator *Coordinator) poolLocked(providerType, baseURL string) *gatewayPool {
	key := providerType + "\x00" + normalizeBaseURL(baseURL)
	if existing := coordinator.pools[key]; existing != nil {
		return existing
	}
	created := &gatewayPool{providerType: providerType, imageLimit: coordinator.config.ImageConcurrency, changed: make(chan struct{})}
	coordinator.pools[key] = created
	return created
}

func (window *requestWindow) prune(now time.Time) {
	cutoff := now.Add(-time.Minute)
	index := 0
	for index < len(window.starts) && !window.starts[index].After(cutoff) {
		index++
	}
	if index > 0 {
		window.starts = append([]time.Time(nil), window.starts[index:]...)
	}
}

func (window *requestWindow) delay(now time.Time, limit ModelLimit) time.Duration {
	var delay time.Duration
	if limit.RPM > 0 && len(window.starts) >= limit.RPM {
		delay = window.starts[len(window.starts)-limit.RPM].Add(time.Minute).Sub(now)
	}
	if limit.RPS > 0 {
		cutoff := now.Add(-time.Second)
		first := len(window.starts)
		for index, started := range window.starts {
			if started.After(cutoff) {
				first = index
				break
			}
		}
		if count := len(window.starts) - first; count >= limit.RPS {
			if secondDelay := window.starts[len(window.starts)-limit.RPS].Add(time.Second).Sub(now); secondDelay > delay {
				delay = secondDelay
			}
		}
	}
	return delay
}

func (coordinator *Coordinator) imageLimit(pool *gatewayPool) int {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return pool.imageLimit
}

func (item *lease) Finish(err error) {
	if item == nil || item.coordinator == nil || item.pool == nil {
		return
	}
	item.once.Do(func() { item.coordinator.finish(item.pool, item.class, err) })
}

func (coordinator *Coordinator) finish(pool *gatewayPool, class RequestClass, callErr error) {
	if pool.providerType != provider.TypeCloudflareAIGateway {
		return
	}
	diagnostic := providerdiag.FromError(callErr)
	rateLimited := diagnostic.HTTPStatus == 429
	now := coordinator.config.Now()
	coordinator.mu.Lock()
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
	coordinator.mu.Unlock()
	if rateLimited {
		coordinator.config.Logger.Warn("Cloudflare provider rate limited", "request_type", class, "cooldown_ms", cooldown.Milliseconds(), "image_concurrency", imageLimit)
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
