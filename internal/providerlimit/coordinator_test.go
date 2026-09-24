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

	"golang.org/x/time/rate"
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
	config.RequestsPerSecond = rate.Inf
	config.Burst = 100
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	return config
}

func TestCoordinatorAppliesThreeStartsPerSecondWithBurstOne(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	config := testConfig()
	config.RequestsPerSecond = 3
	config.Burst = 1
	config.Now = clock.Now
	config.Wait = clock.Wait
	coordinator := NewCoordinatorWithConfig(config)

	first, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "HTTPS://EXAMPLE.COM/ai/v1/", RequestText)
	if err != nil {
		t.Fatal(err)
	}
	first.Finish(nil)
	second, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "https://example.com/ai/v1", RequestImage)
	if err != nil {
		t.Fatal(err)
	}
	second.Finish(nil)

	waits := clock.Waits()
	if len(waits) != 1 || waits[0] < 330*time.Millisecond || waits[0] > 340*time.Millisecond {
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
		lease, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "https://example.com/ai/v1", RequestText)
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
		lease, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "HTTPS://EXAMPLE.COM/ai/v1/", RequestText)
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
		lease, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, baseURL, RequestImage)
		if err != nil {
			t.Fatal(err)
		}
		lease.Finish(nil)
	}
	if limit := coordinator.imageLimit(pool); limit != 2 {
		t.Fatalf("image concurrency after five successes = %d", limit)
	}

	first, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, baseURL, RequestImage)
	if err != nil {
		t.Fatal(err)
	}
	second, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, baseURL, RequestImage)
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
			lease, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "https://example.com/ai/v1", RequestImage)
			if err != nil {
				t.Fatal(err)
			}
			lease.Finish(diagnosticError{details: providerdiag.Details{HTTPStatus: 429, RetryAfter: test.retryAfter}})
			next, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, "https://example.com/ai/v1", RequestImage)
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

func TestCoordinatorWaitCanBeCancelledAndOtherProvidersBypassIt(t *testing.T) {
	coordinator := NewCoordinatorWithConfig(testConfig())
	baseURL := "https://example.com/ai/v1"
	lease, err := coordinator.Acquire(context.Background(), provider.TypeCloudflareAIGateway, baseURL, RequestImage)
	if err != nil {
		t.Fatal(err)
	}
	lease.Finish(diagnosticError{details: providerdiag.Details{HTTPStatus: 429}})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordinator.Acquire(cancelled, provider.TypeCloudflareAIGateway, baseURL, RequestImage); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait error = %v", err)
	}
	bypass, err := coordinator.Acquire(cancelled, provider.TypeAliyunBailian, "https://dashscope.example/v1", RequestImage)
	if err != nil {
		t.Fatalf("Bailian should bypass Cloudflare limiter: %v", err)
	}
	bypass.Finish(nil)
}
