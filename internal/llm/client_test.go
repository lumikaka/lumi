package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestGenerateSendsMultimodalImageContent(t *testing.T) {
	t.Parallel()
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
			t.Fatalf("messages = %+v", payload.Messages)
		}
		content, ok := payload.Messages[0].Content.([]any)
		if !ok || len(content) != 2 {
			t.Fatalf("content = %#v", payload.Messages[0].Content)
		}
		imagePart, ok := content[1].(map[string]any)
		imageURL, _ := imagePart["image_url"].(map[string]any)
		if !ok || imagePart["type"] != "image_url" || imageURL["detail"] != "high" || imageURL["url"] != "data:image/png;base64,iVBORw==" {
			t.Fatalf("image content = %#v", imagePart)
		}
		return response(http.StatusOK, "application/json", `{"choices":[{"message":{"content":"[]"},"finish_reason":"stop"}]}`), nil
	})})
	_, err := client.Generate(context.Background(), Request{
		BaseURL: "https://provider.example/v1", APIKey: "secret", Model: "vision", Prompt: "inspect",
		Images: []ImageInput{{MIMEType: "image/png", Data: []byte{0x89, 0x50, 0x4e, 0x47}, Detail: "high"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func response(status int, contentType, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestGenerateReadsOpenAICompatibleStream(t *testing.T) {
	t.Parallel()
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://provider.example/v1/chat/completions" {
			t.Fatalf("URL = %s", request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer top-secret" {
			t.Fatalf("authorization header was not set")
		}
		return response(http.StatusOK, "text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"第一\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"章\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"prompt_tokens_details\":{\"cached_tokens\":5},\"completion_tokens\":2}}\n\ndata: [DONE]\n\n"), nil
	})})
	var deltas []string
	result, err := client.Generate(context.Background(), Request{BaseURL: "https://provider.example/v1", APIKey: "top-secret", Model: "story", Prompt: "write"}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "第一章" || result.Usage.InputTokens != 12 || result.Usage.CachedInputTokens == nil || *result.Usage.CachedInputTokens != 5 || result.Usage.OutputTokens != 2 || result.FinishReason != "stop" || strings.Join(deltas, "") != result.Content {
		t.Fatalf("response = %+v, deltas = %v", result, deltas)
	}
}

func TestCloudflareModelsUseGatewaySpecificRequestOptions(t *testing.T) {
	t.Parallel()
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["max_completion_tokens"] != float64(12) || payload["max_tokens"] != nil || payload["temperature"] != nil {
			t.Fatalf("GPT-5 payload = %+v", payload)
		}
		return response(http.StatusOK, "application/json", `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
	})})
	temperature := 0.2
	if _, err := client.Generate(context.Background(), Request{BaseURL: "https://api.cloudflare.com/client/v4/accounts/test/ai/v1", APIKey: "secret", Model: "openai/gpt-5.5", Prompt: "ping", MaxTokens: 12, Temperature: &temperature}, nil); err != nil {
		t.Fatal(err)
	}

	workersClient := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("cf-aig-gateway-id") != "default" {
			t.Fatalf("Workers AI gateway header = %q", request.Header.Get("cf-aig-gateway-id"))
		}
		return response(http.StatusOK, "application/json", `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
	})})
	if _, err := workersClient.Generate(context.Background(), Request{BaseURL: "https://api.cloudflare.com/client/v4/accounts/test/ai/v1", APIKey: "secret", Model: "@cf/meta/llama", Prompt: "ping"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateClassifiesProviderFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		transport roundTripperFunc
		code      string
		retryable bool
	}{
		{name: "authentication", code: CodeAuthentication, transport: func(*http.Request) (*http.Response, error) {
			return response(http.StatusUnauthorized, "application/json", `{}`), nil
		}},
		{name: "rate limit", code: CodeRateLimited, retryable: true, transport: func(*http.Request) (*http.Response, error) {
			return response(http.StatusTooManyRequests, "application/json", `{}`), nil
		}},
		{name: "provider unavailable", code: CodeProviderResponse, retryable: true, transport: func(*http.Request) (*http.Response, error) {
			return response(http.StatusBadGateway, "application/json", `{}`), nil
		}},
		{name: "timeout", code: CodeTimeout, retryable: true, transport: func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }},
		{name: "invalid response", code: CodeProviderResponse, retryable: true, transport: func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, "application/json", `{not-json`), nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewOpenAICompatibleClient(&http.Client{Transport: test.transport})
			_, err := client.Generate(context.Background(), Request{BaseURL: "https://provider.example/v1", APIKey: "secret", Model: "story", Prompt: "write"}, nil)
			var modelErr *Error
			if !errors.As(err, &modelErr) || modelErr.Code != test.code || modelErr.Retryable != test.retryable {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestGeneratePropagatesUserCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})})
	_, err := client.Generate(ctx, Request{BaseURL: "https://provider.example/v1", APIKey: "secret", Model: "story", Prompt: "write"}, nil)
	var modelErr *Error
	if !errors.As(err, &modelErr) || modelErr.Code != CodeCancelled || modelErr.Retryable {
		t.Fatalf("error = %#v", err)
	}
}

func TestGenerateCapturesSanitizedProviderDiagnostics(t *testing.T) {
	t.Parallel()
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		result := response(http.StatusBadRequest, "application/json", `{"error":{"code":"invalid_parameter","message":"bad api_key=top-secret Bearer leaked-token"}}`)
		result.Header.Set("X-Request-Id", "request-400")
		return result, nil
	})})
	_, err := client.Generate(context.Background(), Request{BaseURL: "https://provider.example/v1", APIKey: "top-secret", Model: "story", Prompt: "write"}, nil)
	var modelErr *Error
	if !errors.As(err, &modelErr) {
		t.Fatalf("error = %#v", err)
	}
	diagnostic := modelErr.ProviderDiagnostic()
	if diagnostic.HTTPStatus != http.StatusBadRequest || diagnostic.ProviderCode != "invalid_parameter" || diagnostic.RequestID != "request-400" {
		t.Fatalf("diagnostic = %+v", diagnostic)
	}
	if strings.Contains(diagnostic.Message, "top-secret") || strings.Contains(diagnostic.Message, "leaked-token") || !strings.Contains(diagnostic.Message, "[REDACTED]") {
		t.Fatalf("unsafe diagnostic message = %q", diagnostic.Message)
	}
}
