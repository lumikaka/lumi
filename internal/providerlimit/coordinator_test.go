package providerlimit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"lumi/internal/provider"
	"lumi/internal/providerdiag"
)

type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	waits []time.Duration
}

func (clock *fakeClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *fakeClock) Wait(ctx context.Context, duration time.Duration, _ <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	clock.mu.Lock()
	clock.waits = append(clock.waits, duration)
	clock.now = clock.now.Add(duration)
	clock.mu.Unlock()
	return nil
}

func (clock *fakeClock) Waits() []time.Duration {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return append([]time.Duration(nil), clock.waits...)
}

type diagnosticError struct{ details providerdiag.Details }

func (err diagnosticError) Error() string { return "provider error" }

func (err diagnosticError) ProviderDiagnostic() providerdiag.Details { return err.details }

func testConfig() Config {
	config := DefaultConfig()
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	return config
}

func TestCoordinatorAppliesRollingRPMAndRPSAcrossRequestClasses(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	config := testConfig()
	config.Now = clock.Now
	config.Wait = clock.Wait
	coordinator := NewCoordinatorWithConfig(config)
	coordinator.ReplaceLimits(1, []ModelLimit{{ProviderType: provider.TypeCloudflareAIGateway, Model: "openai/gpt-6-sol", RPM: 3, RPS: 2}})

	for _, class := range []RequestClass{RequestText, RequestImage, RequestText, RequestImage} {
		item, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "HTTPS://EXAMPLE.COM/ai/v1/", "openai/gpt-6-sol", class)
		if err != nil {
			t.Fatal(err)
		}
		item.Finish(nil)
	}

	waits := clock.Waits()
	if len(waits) != 2 || waits[0] != time.Second || waits[1] != 59*time.Second {
		t.Fatalf("rate waits = %v", waits)
	}
}

func TestCoordinatorSharesTextConcurrencyAcrossCallers(t *testing.T) {
	config := testConfig()
	waiting := make(chan struct{}, 1)
	config.Wait = func(ctx context.Context, duration time.Duration, wake <-chan struct{}) error {
		if duration >= 0 {
			return waitContext(ctx, duration, wake)
		}
		select {
		case waiting <- struct{}{}:
		default:
		}
		return waitContext(ctx, duration, wake)
	}
	coordinator := NewCoordinatorWithConfig(config)

	leases := make([]Lease, 0, 3)
	for range 3 {
		lease, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "https://example.com/ai/v1", "openai/gpt-6-sol", RequestText)
		if err != nil {
			t.Fatal(err)
		}
		leases = append(leases, lease)
	}
	type result struct {
		lease Lease
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		lease, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "HTTPS://EXAMPLE.COM/ai/v1/", "openai/gpt-6-sol", RequestText)
		resultCh <- result{lease: lease, err: err}
	}()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("fourth text request did not wait for shared concurrency")
	}
	select {
	case got := <-resultCh:
		got.lease.Finish(nil)
		t.Fatal("fourth text request acquired before a slot was released")
	default:
	}
	leasing := leases[0]
	leasing.Finish(nil)
	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatal(got.err)
		}
		got.lease.Finish(nil)
	case <-time.After(time.Second):
		t.Fatal("fourth text request did not acquire after release")
	}
	for _, lease := range leases[1:] {
		lease.Finish(nil)
	}
}

func TestCoordinatorAdaptsImageConcurrencyAndResetsOn429(t *testing.T) {
	coordinator := NewCoordinatorWithConfig(testConfig())
	baseURL := "https://example.com/ai/v1"
	pool := coordinator.pool(provider.TypeCloudflareAIGateway, baseURL)

	for range 5 {
		lease, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, baseURL, "openai/gpt-6-sol", RequestImage)
		if err != nil {
			t.Fatal(err)
		}
		lease.Finish(nil)
	}
	if limit := coordinator.imageLimit(pool); limit != 2 {
		t.Fatalf("image concurrency after five successes = %d", limit)
	}

	first, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, baseURL, "openai/gpt-6-sol", RequestImage)
	if err != nil {
		t.Fatal(err)
	}
	second, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, baseURL, "openai/gpt-6-sol", RequestImage)
	if err != nil {
		t.Fatal(err)
	}
	first.Finish(diagnosticError{details: providerdiag.Details{HTTPStatus: 429}})
	if limit := coordinator.imageLimit(pool); limit != 1 {
		t.Fatalf("image concurrency after 429 = %d", limit)
	}
	second.Finish(nil)
}

func TestCoordinatorUsesRetryAfterDefaultAndMaximumCooldown(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter time.Duration
		want       time.Duration
	}{
		{name: "retry after", retryAfter: 2 * time.Minute, want: 2 * time.Minute},
		{name: "default", want: time.Minute},
		{name: "maximum", retryAfter: 10 * time.Minute, want: 5 * time.Minute},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &fakeClock{now: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
			config := testConfig()
			config.Now = clock.Now
			config.Wait = clock.Wait
			coordinator := NewCoordinatorWithConfig(config)
			lease, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "https://example.com/ai/v1", "openai/gpt-6-sol", RequestImage)
			if err != nil {
				t.Fatal(err)
			}
			lease.Finish(diagnosticError{details: providerdiag.Details{HTTPStatus: 429, RetryAfter: test.retryAfter}})
			next, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "https://example.com/ai/v1", "openai/gpt-6-sol", RequestImage)
			if err != nil {
				t.Fatal(err)
			}
			next.Finish(nil)
			waits := clock.Waits()
			if len(waits) != 1 || waits[0] != test.want {
				t.Fatalf("cooldown waits = %v, want %s", waits, test.want)
			}
		})
	}
}

func TestCoordinatorWaitCanBeCancelledAndOtherProvidersRemainIndependent(t *testing.T) {
	coordinator := NewCoordinatorWithConfig(testConfig())
	baseURL := "https://example.com/ai/v1"
	lease, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, baseURL, "openai/gpt-6-sol", RequestImage)
	if err != nil {
		t.Fatal(err)
	}
	lease.Finish(diagnosticError{details: providerdiag.Details{HTTPStatus: 429}})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordinator.Acquire(cancelled, provider.TypeCloudflareAIGateway, baseURL, "openai/gpt-6-sol", RequestImage); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait error = %v", err)
	}
	bypass, err := coordinator.Acquire(context.Background(), provider.TypeAliyunBailian, "https://dashscope.example/v1", "qwen-image-3.0", RequestImage)
	if err != nil {
		t.Fatalf("Bailian should bypass Cloudflare limiter: %v", err)
	}
	bypass.Finish(nil)
}

func TestCoordinatorLimitsModelsAndProvidersIndependently(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	config := testConfig()
	config.Now, config.Wait = clock.Now, clock.Wait
	coordinator := NewCoordinatorWithConfig(config)
	coordinator.ReplaceLimits(1, []ModelLimit{{ProviderType: provider.TypeAliyunBailian, Model: "qwen3.7-plus", RPM: 1}})
	for _, model := range []string{"qwen3.7-plus", "qwen3.8-max", "qwen3.7-plus"} {
		item, err := coordinator.Acquire(context.Background(), provider.TypeAliyunBailian, "https://dashscope.example/v1", model, RequestText)
		if err != nil {
			t.Fatal(err)
		}
		item.Finish(nil)
	}
	if waits := clock.Waits(); len(waits) != 1 || waits[0] != time.Minute {
		t.Fatalf("model waits = %v", waits)
	}
}

func TestCoordinatorSharesModelRateAcrossEndpointsAndCountsFailures(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	config := testConfig()
	config.Now, config.Wait = clock.Now, clock.Wait
	coordinator := NewCoordinatorWithConfig(config)
	coordinator.ReplaceLimits(1, []ModelLimit{{ProviderType: provider.TypeAliyunBailian, Model: "qwen3.7-plus", RPS: 1}})
	first, err := coordinator.Acquire(context.Background(), provider.TypeAliyunBailian, "https://dashscope.example/compatible-mode/v1", "qwen3.7-plus", RequestText)
	if err != nil {
		t.Fatal(err)
	}
	first.Finish(errors.New("upstream failed"))
	second, err := coordinator.Acquire(context.Background(), provider.TypeAliyunBailian, "https://dashscope.example/other-endpoint", "qwen3.7-plus", RequestImage)
	if err != nil {
		t.Fatal(err)
	}
	second.Finish(nil)
	if waits := clock.Waits(); len(waits) != 1 || waits[0] != time.Second {
		t.Fatalf("shared model waits = %v", waits)
	}
}

func TestCoordinatorAppliesProviderAndModelPoolsTogether(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	config := testConfig()
	config.Now, config.Wait = clock.Now, clock.Wait
	coordinator := NewCoordinatorWithConfig(config)
	coordinator.ReplaceLimits(1, []ModelLimit{
		{Scope: ScopeProvider, ProviderType: provider.TypeAliyunBailian, RPM: 3},
		{Scope: ScopeModel, ProviderType: provider.TypeAliyunBailian, Model: "qwen3.7-plus", RPS: 1},
	})
	for _, model := range []string{"qwen3.7-plus", "qwen3.8-max", "qwen3.7-plus", "qwen3.8-max"} {
		item, err := coordinator.Acquire(context.Background(), provider.TypeAliyunBailian, "https://dashscope.example/v1", model, RequestText)
		if err != nil {
			t.Fatal(err)
		}
		item.Finish(nil)
	}
	if waits := clock.Waits(); len(waits) != 2 || waits[0] != time.Second || waits[1] != 59*time.Second {
		t.Fatalf("provider and model waits = %v", waits)
	}
}

func TestCoordinatorCancelledWaitDoesNotSpendRateAllowance(t *testing.T) {
	entered := make(chan struct{}, 1)
	config := testConfig()
	config.Wait = func(ctx context.Context, duration time.Duration, wake <-chan struct{}) error {
		entered <- struct{}{}
		return waitContext(ctx, duration, wake)
	}
	coordinator := NewCoordinatorWithConfig(config)
	coordinator.ReplaceLimits(1, []ModelLimit{{ProviderType: provider.TypeAliyunBailian, Model: "qwen3.7-plus", RPM: 1}})
	baseURL := "https://dashscope.example/v1"
	first, err := coordinator.Acquire(context.Background(), provider.TypeAliyunBailian, baseURL, "qwen3.7-plus", RequestText)
	if err != nil {
		t.Fatal(err)
	}
	first.Finish(nil)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := coordinator.Acquire(ctx, provider.TypeAliyunBailian, baseURL, "qwen3.7-plus", RequestText)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("second request did not wait")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request error = %v", err)
	}
	coordinator.ReplaceLimits(2, []ModelLimit{{ProviderType: provider.TypeAliyunBailian, Model: "qwen3.7-plus", RPM: 2}})
	thirdCtx, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	third, err := coordinator.Acquire(thirdCtx, provider.TypeAliyunBailian, baseURL, "qwen3.7-plus", RequestText)
	if err != nil {
		t.Fatal(err)
	}
	third.Finish(nil)
}

func TestCoordinatorAppliesChangedLimitToExistingHistoryAndWakesWaiters(t *testing.T) {
	entered := make(chan time.Duration, 1)
	config := testConfig()
	config.Wait = func(ctx context.Context, duration time.Duration, wake <-chan struct{}) error {
		entered <- duration
		select {
		case <-wake:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	coordinator := NewCoordinatorWithConfig(config)
	baseURL := "https://dashscope.example/v1"
	first, err := coordinator.Acquire(context.Background(), provider.TypeAliyunBailian, baseURL, "qwen3.7-plus", RequestText)
	if err != nil {
		t.Fatal(err)
	}
	first.Finish(nil)
	coordinator.ReplaceLimits(1, []ModelLimit{{ProviderType: provider.TypeAliyunBailian, Model: "qwen3.7-plus", RPM: 1}})
	result := make(chan error, 1)
	go func() {
		item, err := coordinator.Acquire(context.Background(), provider.TypeAliyunBailian, baseURL, "qwen3.7-plus", RequestText)
		if err == nil {
			item.Finish(nil)
		}
		result <- err
	}()
	select {
	case duration := <-entered:
		if duration < 59*time.Second {
			t.Fatalf("wait duration = %s", duration)
		}
	case <-time.After(time.Second):
		t.Fatal("updated limit did not make request wait")
	}
	coordinator.ReplaceLimits(2, nil)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("settings update did not wake waiting request")
	}
}
