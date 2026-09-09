package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"lumi/internal/provider"
)

const responsesOK = `{"status":"completed","output":[{"type":"message","role":"assistant","status":"completed","phase":"final_answer","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":12,"input_tokens_details":{"cached_tokens":5},"output_tokens":3}}`

func TestResponsesGenerateStreamsTextAndMapsImageInput(t *testing.T) {
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != "POST" || req.URL.Path != "/ai/v1/responses" {
			t.Fatalf("request=%s %s", req.Method, req.URL)
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "openai/gpt-5.6-terra" || payload["stream"] != true || payload["store"] != false || payload["max_output_tokens"] != float64(4096) || payload["messages"] != nil || payload["max_tokens"] != nil || payload["stream_options"] != nil {
			t.Fatalf("payload=%#v", payload)
		}
		items := payload["input"].([]any)
		if items[0].(map[string]any)["role"] != "system" {
			t.Fatal("missing system prompt")
		}
		content := items[1].(map[string]any)["content"].([]any)
		image := content[1].(map[string]any)
		if image["type"] != "input_image" || image["image_url"] != "data:image/png;base64,AQI=" || image["detail"] != "high" {
			t.Fatalf("image=%#v", image)
		}
		return response(200, "text/event-stream", "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"O\"}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"K\"}\n\ndata: {\"type\":\"response.completed\",\"response\":"+responsesOK+"}\n\n"), nil
	})})
	var deltas []string
	result, err := client.Generate(context.Background(), Request{ProviderType: provider.TypeCloudflareAIGateway, BaseURL: "https://test.example/ai/v1", APIKey: "secret", Model: "openai/gpt-5.6-terra", SystemPrompt: "system", Prompt: "inspect", Images: []ImageInput{{MIMEType: "image/png", Data: []byte{1, 2}}}, MaxTokens: 4096}, func(delta string) error { deltas = append(deltas, delta); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "OK" || strings.Join(deltas, "") != "OK" || len(deltas) != 2 || result.FinishReason != "stop" || !result.Usage.InputKnown || result.Usage.InputTokens != 12 || result.Usage.CachedInputTokens == nil || *result.Usage.CachedInputTokens != 5 || !result.Usage.OutputKnown || result.Usage.OutputTokens != 3 {
		t.Fatalf("result=%+v deltas=%v", result, deltas)
	}
}

func TestResponsesToolRoundTripPreservesReasoningAndCallIdentity(t *testing.T) {
	const toolResponse = `{"status":"completed","output":[{"type":"reasoning","id":"rs-1","summary":[],"encrypted_content":"opaque-reasoning"},{"type":"message","id":"msg-1","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"Looking up"}]},{"type":"function_call","id":"fc-item-1","call_id":"call-1","name":"lookup","arguments":"{\"query\":\"test\"}"}],"usage":{"input_tokens":20,"output_tokens":4}}`
	count := 0
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		count++
		if req.URL.Path != "/v1/responses" {
			t.Fatalf("path=%s", req.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["stream"] != false || payload["store"] != false || payload["include"].([]any)[0] != "reasoning.encrypted_content" {
			t.Fatalf("options=%#v", payload)
		}
		tool := payload["tools"].([]any)[0].(map[string]any)
		if tool["name"] != "lookup" || tool["type"] != "function" || tool["strict"] != false || tool["function"] != nil {
			t.Fatalf("tool=%#v", tool)
		}
		if count == 1 {
			return response(200, "application/json", toolResponse), nil
		}
		items := payload["input"].([]any)
		if len(items) != 5 {
			t.Fatalf("input=%#v", items)
		}
		if items[1].(map[string]any)["encrypted_content"] != "opaque-reasoning" || items[2].(map[string]any)["phase"] != "commentary" || items[3].(map[string]any)["call_id"] != "call-1" || items[3].(map[string]any)["id"] != "fc-item-1" {
			t.Fatalf("lost continuation metadata: %#v", items)
		}
		out := items[4].(map[string]any)
		if out["type"] != "function_call_output" || out["call_id"] != "call-1" || out["output"] != "found" {
			t.Fatalf("tool output=%#v", out)
		}
		return response(200, "application/json", responsesOK), nil
	})})
	request := ChatRequest{ProviderType: provider.TypeCloudflareAIGateway, BaseURL: "https://test.example/v1", Model: "openai/gpt-5.6-terra", Messages: []ChatMessage{{Role: "user", Content: "find"}}, Tools: []ToolDefinition{{Name: "lookup", Parameters: map[string]any{"type": "object"}}}}
	first, err := client.Complete(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.FinishReason != "tool_calls" || len(first.Message.ToolCalls) != 1 || first.Message.ToolCalls[0].ID != "call-1" {
		t.Fatalf("result=%+v", first)
	}
	// Serialize as the persisted log does, then reconstruct the next request.
	encoded, _ := json.Marshal(first.Message)
	var restored ChatMessage
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	request.Messages = append(request.Messages, restored, ChatMessage{Role: "tool", ToolCallID: restored.ToolCalls[0].ID, Content: "found"})
	final, err := client.Complete(context.Background(), request)
	if err != nil || final.Message.Content != "OK" || count != 2 {
		t.Fatalf("result=%+v err=%v requests=%d", final, err, count)
	}
}

func TestResponsesRejectsUnsafeOutputsAndRetainsUsage(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		reason     ProviderResponseFailureReason
	}{
		{"incomplete", `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":10,"output_tokens":512}}`, ProviderResponseFinishReasonLength},
		{"missing call ID", `{"status":"completed","output":[{"type":"function_call","id":"not-call-id","name":"lookup","arguments":"{}"}]}`, ProviderResponseMissingToolCallID},
		{"duplicate call ID", `{"status":"completed","output":[{"type":"function_call","call_id":"same","name":"lookup","arguments":"{}"},{"type":"function_call","call_id":"same","name":"lookup","arguments":"{}"}]}`, ProviderResponseDuplicateToolCallID},
		{"wrong arguments type", `{"status":"completed","output":[{"type":"function_call","call_id":"call","name":"lookup","arguments":{}}]}`, ProviderResponseToolArgumentsWrongType},
		{"mixed user input", `{"status":"completed","output":[{"type":"function_call","call_id":"one","name":"request_user_input","arguments":"{}"},{"type":"function_call","call_id":"two","name":"lookup","arguments":"{}"}]}`, ProviderResponseRequestUserInputMixed},
		{"negative usage", `{"status":"completed","output":[],"usage":{"input_tokens":-1}}`, ProviderResponseNegativeUsage},
		{"refusal", `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"no"}]}]}`, ProviderResponseEmptyMessage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) { return response(200, "application/json", tc.body), nil })})
			result, err := client.Complete(context.Background(), ChatRequest{ProviderType: provider.TypeCloudflareAIGateway, BaseURL: "https://test.example/v1", Model: "openai/gpt-5.6-terra", Messages: []ChatMessage{{Role: "user", Content: "test"}}})
			var modelErr *Error
			if !errors.As(err, &modelErr) || modelErr.ResponseDiagnostic == nil || modelErr.ResponseDiagnostic.Reason != tc.reason {
				t.Fatalf("error=%#v", err)
			}
			if tc.name == "incomplete" && (result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 512 || modelErr.PartialResponse == nil) {
				t.Fatalf("usage lost: %+v", result)
			}
		})
	}
}

func TestResponsesStreamRequiresTerminalEventAndPropagatesCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		cancel     bool
	}{
		{"truncated", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n", false},
		{"upstream error", "data: {\"type\":\"error\",\"code\":\"server_error\",\"message\":\"failed\"}\n\n", false},
		{"cancelled", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if err := req.Context().Err(); err != nil {
					return nil, err
				}
				return response(200, "text/event-stream", tc.body), nil
			})})
			_, err := client.Generate(ctx, Request{ProviderType: provider.TypeCloudflareAIGateway, BaseURL: "https://test.example/v1", Model: "openai/gpt-5.6-terra", Prompt: "test"}, nil)
			if err == nil {
				t.Fatal("incomplete stream succeeded")
			}
			var modelErr *Error
			if tc.cancel && (!errors.As(err, &modelErr) || modelErr.Code != CodeCancelled) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestBailianGenerationKeepsChatCompletionsAfterResponsesMigration(t *testing.T) {
	calls := 0
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/compatible-mode/v1/chat/completions" || req.Header.Get("cf-aig-gateway-id") != "" {
			t.Fatalf("Bailian request=%s", req.URL)
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["input"] != nil || payload["max_output_tokens"] != nil || payload["store"] != nil || payload["max_tokens"] != float64(16) || payload["messages"] == nil {
			t.Fatalf("Bailian payload=%#v", payload)
		}
		return response(200, "application/json", `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`), nil
	})})
	if _, err := client.Generate(context.Background(), Request{ProviderType: provider.TypeAliyunBailian, BaseURL: "https://bailian.example/compatible-mode/v1", Model: provider.BailianTextModel, Prompt: "test", MaxTokens: 16}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), ChatRequest{ProviderType: provider.TypeAliyunBailian, BaseURL: "https://bailian.example/compatible-mode/v1", Model: provider.BailianTextModel, Messages: []ChatMessage{{Role: "user", Content: "test"}}, MaxTokens: 16}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}
