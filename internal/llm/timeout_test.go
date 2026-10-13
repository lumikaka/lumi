package llm

import (
	"context"
	"net/http"
	"testing"
	"time"

	"lumi/internal/provider"
)

func TestRequestTimeoutDoesNotChangeSharedClient(t *testing.T) {
	for _, providerType := range []string{provider.TypeOpenAIRelay, provider.TypeAliyunBailian} {
		for _, generate := range []bool{false, true} {
			t.Run(providerType+map[bool]string{true: "/generate", false: "/complete"}[generate], func(t *testing.T) {
				want := 15 * time.Minute
				transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					deadline, ok := req.Context().Deadline()
					remaining := time.Until(deadline)
					if !ok || remaining > want || remaining < want-time.Second {
						t.Fatalf("request deadline=%s want=%s", remaining, want)
					}
					if providerType == provider.TypeOpenAIRelay {
						return response(200, "application/json", responsesOK), nil
					}
					return response(200, "application/json", `{"choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`), nil
				})
				httpClient := &http.Client{Timeout: 5 * time.Minute, Transport: transport}
				client := NewOpenAICompatibleClient(httpClient)
				call := func(timeout time.Duration) {
					t.Helper()
					var err error
					if generate {
						_, err = client.Generate(t.Context(), Request{ProviderType: providerType, BaseURL: "https://provider.test/v1", Model: "test", Prompt: "OK", Timeout: timeout}, nil)
					} else {
						_, err = client.Complete(t.Context(), ChatRequest{ProviderType: providerType, BaseURL: "https://provider.test/v1", Model: "test", Messages: []ChatMessage{{Role: "user", Content: "OK"}}, Timeout: timeout})
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				call(want)
				if httpClient.Timeout != 5*time.Minute {
					t.Fatal("shared client timeout changed")
				}
				want = 5 * time.Minute
				call(0)
			})
		}
	}
}

func TestRequestTimeoutHonorsEarlierCancellation(t *testing.T) {
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := client.Complete(ctx, ChatRequest{ProviderType: provider.TypeOpenAIRelay, BaseURL: "https://provider.test/v1", Model: "test", Messages: []ChatMessage{{Role: "user", Content: "OK"}}, Timeout: 15 * time.Minute})
	if err == nil {
		t.Fatal("cancelled request succeeded")
	}
}
