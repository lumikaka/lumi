package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"lumi/internal/provider"
)

func TestConnectionChecksUseIndependentProviderRequests(t *testing.T) {
	t.Parallel()
	const cloudflareURL = "https://api.cloudflare.com/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/v1"
	for _, test := range []struct {
		name, providerType, baseURL, model, gateway, tokenKey string
		stream                                                bool
		tokens                                                float64
	}{
		{"cloudflare", provider.TypeCloudflareAIGateway, cloudflareURL, "openai/gpt-5.6-terra", "", "max_output_tokens", false, 512},
		{"cloudflare gpt5", provider.TypeCloudflareAIGateway, cloudflareURL, "openai/gpt-5.6-sol", "", "max_output_tokens", false, 512},
		{"workers ai", provider.TypeCloudflareAIGateway, cloudflareURL, "@cf/openai/gpt-oss-120b", "default", "max_output_tokens", false, 512},
		{"bailian", provider.TypeAliyunBailian, "https://ws-123.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", provider.BailianTextModel, "", "max_tokens", true, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				suffix, inputKey := "/responses", "input"
				if test.stream {
					suffix, inputKey = "/chat/completions", "messages"
				}
				if req.Method != http.MethodPost || req.URL.String() != test.baseURL+suffix {
					t.Fatalf("request = %s %s", req.Method, req.URL)
				}
				if req.Header.Get("Authorization") != "Bearer test-secret" || req.Header.Get("Content-Type") != "application/json" || req.Header.Get("cf-aig-gateway-id") != test.gateway || req.Header.Get("cf-aig-authorization") != "" {
					t.Fatal("incorrect provider authentication or gateway headers")
				}
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if payload["model"] != test.model || payload["stream"] != test.stream || payload[test.tokenKey] != test.tokens {
					t.Fatalf("payload = %#v", payload)
				}
				messages, ok := payload[inputKey].([]any)
				if !ok || len(messages) != 1 {
					t.Fatalf("messages = %#v", payload[inputKey])
				}
				message := messages[0].(map[string]any)
				if message["role"] != "user" || message["content"] != "Reply with OK." {
					t.Fatalf("message = %#v", message)
				}
				if test.stream {
					options, ok := payload["stream_options"].(map[string]any)
					if !ok || options["include_usage"] != true || len(payload) != 5 {
						t.Fatalf("Bailian streaming options changed: %#v", payload)
					}
					return response(http.StatusOK, "text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"), nil
				}
				if len(payload) != 5 || payload["store"] != false || req.Header.Get("Accept") != "application/json" {
					t.Fatalf("unexpected Cloudflare probe options: %#v", payload)
				}
				return response(http.StatusOK, "application/json", responsesOK), nil
			})})
			if err := client.CheckConnection(context.Background(), test.providerType, Request{BaseURL: test.baseURL, APIKey: "test-secret", Model: test.model}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCloudflareConnectionCheckResponses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body, code string
		status           int
	}{
		{"success", responsesOK, "", 200},
		{"reasoning budget exhausted", `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`, "", 200},
		{"empty output", `{"status":"completed","output":[]}`, CodeInvalidContent, 200},
		{"missing status", `{"output":[]}`, CodeInvalidContent, 200},
		{"malformed", `{broken`, CodeInvalidContent, 200},
		{"trailing JSON", `{"status":"completed","output":[]} {}`, CodeInvalidContent, 200},
		{"content filter", `{"status":"incomplete","incomplete_details":{"reason":"content_filter"}}`, CodeInvalidContent, 200},
		{"authentication", `{"success":false,"errors":[{"code":10000,"message":"Authentication error"}]}`, CodeAuthentication, 401},
		{"rate limited", `{}`, CodeRateLimited, 429},
		{"server failure", `{}`, CodeProviderResponse, 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return response(test.status, "application/json", test.body), nil
			})})
			err := client.CheckConnection(context.Background(), provider.TypeCloudflareAIGateway, Request{BaseURL: "https://api.cloudflare.com/client/v4/accounts/test/ai/v1", APIKey: "secret", Model: "deepseek/deepseek-v4-pro"})
			if test.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var checkErr *Error
			if !errors.As(err, &checkErr) || checkErr.Code != test.code {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
		})
	}
}
