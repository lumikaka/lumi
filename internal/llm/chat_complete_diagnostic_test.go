package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type unexpectedEOFReadCloser struct {
	body []byte
	done bool
}

func (reader *unexpectedEOFReadCloser) Read(buffer []byte) (int, error) {
	if reader.done {
		return 0, io.EOF
	}
	reader.done = true
	count := copy(buffer, reader.body)
	return count, io.ErrUnexpectedEOF
}

func (*unexpectedEOFReadCloser) Close() error { return nil }

type partialErrorReadCloser struct {
	body []byte
	err  error
	done bool
}

func (reader *partialErrorReadCloser) Read(buffer []byte) (int, error) {
	if reader.done {
		return 0, io.EOF
	}
	reader.done = true
	count := copy(buffer, reader.body)
	return count, reader.err
}

func (*partialErrorReadCloser) Close() error { return nil }

func TestCompleteKeepsInvalidArgumentJSONForAgentRepair(t *testing.T) {
	t.Parallel()
	client := completeTestClient(func() *http.Response {
		return response(http.StatusOK, "application/json", `{
			"choices":[{"message":{"tool_calls":[{"id":"call_1","function":{"name":"request_api","arguments":"{broken"}}]},"finish_reason":"tool_calls"}],
			"usage":{"prompt_tokens":13,"completion_tokens":7}
		}`)
	})

	got, err := client.Complete(context.Background(), validChatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Message.ToolCalls) != 1 || got.Message.ToolCalls[0].Arguments != "{broken" {
		t.Fatalf("tool calls = %+v", got.Message.ToolCalls)
	}
	if got.Usage.InputTokens != 13 || got.Usage.OutputTokens != 7 || got.FinishReason != "tool_calls" {
		t.Fatalf("response = %+v", got)
	}
}

func TestCompleteParsesValidToolCallsWithStablePairing(t *testing.T) {
	t.Parallel()
	client := completeTestClient(func() *http.Response {
		return response(http.StatusOK, "application/json", `{
			"choices":[{"message":{"tool_calls":[
				{"id":"call_a","function":{"name":"request_api","arguments":"{\"method\":\"GET\"}"}},
				{"id":"call_b","function":{"name":"read_agent_doc","arguments":"{\"path\":\"/api/v1/agent-docs/overview.md\"}"}}
			]},"finish_reason":"tool_calls"}],
			"usage":{"prompt_tokens":21,"completion_tokens":9,"prompt_tokens_details":{"cached_tokens":4}}
		}`)
	})

	got, err := client.Complete(context.Background(), validChatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Message.ToolCalls) != 2 || got.Message.ToolCalls[0].ID != "call_a" || got.Message.ToolCalls[0].Name != "request_api" || got.Message.ToolCalls[1].ID != "call_b" || got.Message.ToolCalls[1].Name != "read_agent_doc" {
		t.Fatalf("tool calls=%+v", got.Message.ToolCalls)
	}
	if got.Usage.InputTokens != 21 || got.Usage.OutputTokens != 9 || got.Usage.CachedInputTokens == nil || *got.Usage.CachedInputTokens != 4 {
		t.Fatalf("usage=%+v", got.Usage)
	}
}

func TestCompletePreservesProviderToolCallIDAndNameVerbatim(t *testing.T) {
	t.Parallel()
	client := completeTestClient(func() *http.Response {
		return response(http.StatusOK, "application/json", `{
			"choices":[{"message":{"tool_calls":[{
				"id":"  call_with_provider_spacing  ",
				"function":{"name":"  request_api  ","arguments":"{}"}
			}]} ,"finish_reason":"tool_calls"}]
		}`)
	})

	got, err := client.Complete(context.Background(), validChatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Message.ToolCalls) != 1 || got.Message.ToolCalls[0].ID != "  call_with_provider_spacing  " || got.Message.ToolCalls[0].Name != "  request_api  " {
		t.Fatalf("tool calls=%+v", got.Message.ToolCalls)
	}
}

func TestCompleteRejectsWholeMultiCallResponseWhenOneArgumentsValueHasWrongType(t *testing.T) {
	t.Parallel()
	client := completeTestClient(func() *http.Response {
		return response(http.StatusOK, "application/json", `{
			"choices":[{"message":{"tool_calls":[
				{"id":"call_good","function":{"name":"request_api","arguments":"{}"}},
				{"id":"call_bad","function":{"name":"read_agent_doc","arguments":{}}}
			]},"finish_reason":"tool_calls"}],
			"usage":{"prompt_tokens":8,"completion_tokens":3}
		}`)
	})

	partial, err := client.Complete(context.Background(), validChatRequest())
	var modelErr *Error
	if !errors.As(err, &modelErr) || modelErr.ResponseDiagnostic == nil {
		t.Fatalf("error=%#v", err)
	}
	diagnostic := modelErr.ResponseDiagnostic
	if diagnostic.Reason != ProviderResponseToolArgumentsWrongType || diagnostic.ToolIndex == nil || *diagnostic.ToolIndex != 1 {
		t.Fatalf("diagnostic=%+v", diagnostic)
	}
	if len(partial.Message.ToolCalls) != 1 || partial.Message.ToolCalls[0].ID != "call_good" {
		t.Fatalf("partial=%+v", partial)
	}
}

func TestCompleteClassifiesStructurallyInvalidProviderResponses(t *testing.T) {
	t.Parallel()
	largeArguments, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"message": map[string]any{"tool_calls": []any{map[string]any{
				"id": "call_1", "function": map[string]any{"name": "request_api", "arguments": strings.Repeat("x", maxToolArgumentsBytes+1)},
			}}},
			"finish_reason": "tool_calls",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name            string
		body            string
		reason          ProviderResponseFailureReason
		wantChoiceIndex *int
		wantToolIndex   *int
	}{
		{name: "empty body", body: "", reason: ProviderResponseEmptyBody},
		{name: "whitespace body", body: " \n\t", reason: ProviderResponseEmptyBody},
		{name: "malformed json", body: `{not-json`, reason: ProviderResponseMalformedJSON},
		{name: "trailing json", body: `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3}} {"extra":true}`, reason: ProviderResponseTrailingJSON, wantChoiceIndex: intPointer(0)},
		{name: "empty choices", body: `{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2}}`, reason: ProviderResponseEmptyChoices},
		{name: "empty message", body: `{"choices":[{"message":{},"finish_reason":"stop"}]}`, reason: ProviderResponseEmptyMessage, wantChoiceIndex: intPointer(0)},
		{name: "missing call id", body: `{"choices":[{"message":{"tool_calls":[{"function":{"name":"request_api","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, reason: ProviderResponseMissingToolCallID, wantChoiceIndex: intPointer(0), wantToolIndex: intPointer(0)},
		{name: "duplicate call id", body: `{"choices":[{"message":{"tool_calls":[{"id":"same","function":{"name":"request_api","arguments":"{}"}},{"id":"same","function":{"name":"read_agent_doc","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, reason: ProviderResponseDuplicateToolCallID, wantChoiceIndex: intPointer(0), wantToolIndex: intPointer(1)},
		{name: "missing tool name", body: `{"choices":[{"message":{"tool_calls":[{"id":"call_1","function":{"arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, reason: ProviderResponseMissingToolName, wantChoiceIndex: intPointer(0), wantToolIndex: intPointer(0)},
		{name: "arguments object", body: `{"choices":[{"message":{"tool_calls":[{"id":"call_1","function":{"name":"request_api","arguments":{}}}]},"finish_reason":"tool_calls"}]}`, reason: ProviderResponseToolArgumentsWrongType, wantChoiceIndex: intPointer(0), wantToolIndex: intPointer(0)},
		{name: "arguments null", body: `{"choices":[{"message":{"tool_calls":[{"id":"call_1","function":{"name":"request_api","arguments":null}}]},"finish_reason":"tool_calls"}]}`, reason: ProviderResponseToolArgumentsWrongType, wantChoiceIndex: intPointer(0), wantToolIndex: intPointer(0)},
		{name: "arguments too large", body: string(largeArguments), reason: ProviderResponseToolArgumentsTooLarge, wantChoiceIndex: intPointer(0), wantToolIndex: intPointer(0)},
		{name: "request user input mixed with sibling", body: `{"choices":[{"message":{"tool_calls":[{"id":"ask","function":{"name":"request_user_input","arguments":"{}"}},{"id":"write","function":{"name":"request_api","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, reason: ProviderResponseRequestUserInputMixed, wantChoiceIndex: intPointer(0), wantToolIndex: intPointer(0)},
		{name: "length finish", body: `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}],"usage":{"prompt_tokens":11,"completion_tokens":5}}`, reason: ProviderResponseFinishReasonLength, wantChoiceIndex: intPointer(0)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := response(http.StatusOK, "application/json; charset=utf-8", test.body)
			result.Header.Set("X-Request-Id", "provider-request")
			client := completeTestClient(func() *http.Response { return result })
			partial, err := client.Complete(context.Background(), validChatRequest())
			var modelErr *Error
			if !errors.As(err, &modelErr) {
				t.Fatalf("error = %#v", err)
			}
			if modelErr.Code != CodeInvalidContent || modelErr.Retryable {
				t.Fatalf("classification = %+v", modelErr)
			}
			diagnostic := modelErr.InvalidProviderResponse()
			if diagnostic == nil || diagnostic.Reason != test.reason {
				t.Fatalf("diagnostic = %+v", diagnostic)
			}
			if !sameOptionalInt(diagnostic.ChoiceIndex, test.wantChoiceIndex) || !sameOptionalInt(diagnostic.ToolIndex, test.wantToolIndex) {
				t.Fatalf("indexes = choice:%v tool:%v", diagnostic.ChoiceIndex, diagnostic.ToolIndex)
			}
			if diagnostic.HTTPStatus != http.StatusOK || diagnostic.ProviderRequestID != "provider-request" || diagnostic.ContentType != "application/json; charset=utf-8" || diagnostic.BodyLength != int64(len(test.body)) || diagnostic.BodyTruncated {
				t.Fatalf("response metadata = %+v", diagnostic)
			}
			if test.reason == ProviderResponseFinishReasonLength {
				if partial.FinishReason != "length" || partial.Usage.InputTokens != 11 || partial.Usage.OutputTokens != 5 || partial.Message.Content != "partial" {
					t.Fatalf("partial response = %+v", partial)
				}
				if modelErr.PartialChatResponse() == nil {
					t.Fatal("partial response was not attached to error")
				}
			}
			if test.reason == ProviderResponseEmptyChoices {
				if partial.Usage.InputTokens != 3 || partial.Usage.OutputTokens != 2 || modelErr.PartialChatResponse() == nil {
					t.Fatalf("usage-only partial response = %+v, error partial = %+v", partial, modelErr.PartialChatResponse())
				}
			}
		})
	}
}

func TestCompleteBoundsOversizedBodyBeforeParsing(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("x", maxChatResponseBodyBytes+257)
	result := response(http.StatusOK, "application/json", body)
	result.ContentLength = int64(len(body))
	client := completeTestClient(func() *http.Response { return result })

	_, err := client.Complete(context.Background(), validChatRequest())
	var modelErr *Error
	if !errors.As(err, &modelErr) || modelErr.ResponseDiagnostic == nil {
		t.Fatalf("error = %#v", err)
	}
	diagnostic := modelErr.ResponseDiagnostic
	if diagnostic.Reason != ProviderResponseBodyTooLarge || !diagnostic.BodyTruncated || diagnostic.BodyLength != int64(len(body)) {
		t.Fatalf("diagnostic = %+v", diagnostic)
	}
	if len(diagnostic.Preview) > maxDiagnosticPreview {
		t.Fatalf("preview length = %d", len(diagnostic.Preview))
	}
}

func TestCompleteTurnsPartialUnexpectedEOFIntoStructuralDiagnostic(t *testing.T) {
	t.Parallel()
	partialBody := `{"choices":[{"message":{"content":"partial response`
	result := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"short-read-request"}},
		Body:          &unexpectedEOFReadCloser{body: []byte(partialBody)},
		ContentLength: int64(len(partialBody) + 100),
	}
	client := completeTestClient(func() *http.Response { return result })

	_, err := client.Complete(context.Background(), validChatRequest())
	var modelErr *Error
	if !errors.As(err, &modelErr) || modelErr.Code != CodeInvalidContent || modelErr.ResponseDiagnostic == nil {
		t.Fatalf("error=%#v", err)
	}
	diagnostic := modelErr.ResponseDiagnostic
	if diagnostic.Reason != ProviderResponseBodyReadError || diagnostic.BodyLength != int64(len(partialBody)) || !diagnostic.BodyTruncated {
		t.Fatalf("diagnostic=%+v", diagnostic)
	}
	if diagnostic.ProviderRequestID != "short-read-request" || diagnostic.Preview != partialBody {
		t.Fatalf("diagnostic metadata=%+v", diagnostic)
	}
	if !errors.Is(modelErr, io.ErrUnexpectedEOF) {
		t.Fatalf("cause=%v", modelErr)
	}
}

func TestCompleteTurnsPartialNetworkReadFailureIntoStructuralDiagnostic(t *testing.T) {
	t.Parallel()
	partialBody := `{"choices":[{"message":{"content":"partial bearer leaked-token /Users/private/file`
	result := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"partial-network-read"},
		},
		Body: &partialErrorReadCloser{
			body: []byte(partialBody),
			err:  &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")},
		},
		ContentLength: int64(len(partialBody) + 100),
	}
	client := completeTestClient(func() *http.Response { return result })

	_, err := client.Complete(context.Background(), validChatRequest())
	var modelErr *Error
	if !errors.As(err, &modelErr) || modelErr.Code != CodeInvalidContent || modelErr.ResponseDiagnostic == nil {
		t.Fatalf("error=%#v", err)
	}
	diagnostic := modelErr.ResponseDiagnostic
	if diagnostic.Reason != ProviderResponseBodyReadError || diagnostic.BodyLength != int64(len(partialBody)) || !diagnostic.BodyTruncated {
		t.Fatalf("diagnostic=%+v", diagnostic)
	}
	if diagnostic.ProviderRequestID != "partial-network-read" || diagnostic.Preview == "" {
		t.Fatalf("diagnostic metadata=%+v", diagnostic)
	}
	for _, forbidden := range []string{"leaked-token", "/Users/private/file"} {
		if strings.Contains(diagnostic.Preview, forbidden) {
			t.Fatalf("diagnostic preview leaked %q: %s", forbidden, diagnostic.Preview)
		}
	}
}

func TestCompleteKeepsCancelledOrTimedOutPartialReadOutOfDiagnostics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "cancelled", err: context.Canceled, code: CodeCancelled},
		{name: "deadline", err: context.DeadlineExceeded, code: CodeTimeout},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := completeTestClient(func() *http.Response {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       &partialErrorReadCloser{body: []byte(`{"secret":"must-not-persist"`), err: test.err},
				}
			})
			_, err := client.Complete(context.Background(), validChatRequest())
			var modelErr *Error
			if !errors.As(err, &modelErr) || modelErr.Code != test.code {
				t.Fatalf("error=%#v", err)
			}
			if modelErr.ResponseDiagnostic != nil || modelErr.PartialResponse != nil {
				t.Fatalf("cancelled/timeout read persisted partial Provider body: %+v", modelErr)
			}
		})
	}
}

func TestCompleteRejectsNegativeUsageWithoutReturningItForAccounting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		usage         string
		wantInput     int
		wantOutput    int
		wantCached    int
		cachedPresent bool
	}{
		{name: "prompt tokens", usage: `"prompt_tokens":-1,"completion_tokens":2`, wantInput: -1, wantOutput: 2},
		{name: "completion tokens", usage: `"prompt_tokens":3,"completion_tokens":-2`, wantInput: 3, wantOutput: -2},
		{name: "cached tokens", usage: `"prompt_tokens":3,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":-4}`, wantInput: 3, wantOutput: 2, wantCached: -4, cachedPresent: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := completeTestClient(func() *http.Response {
				return response(http.StatusOK, "application/json", `{"choices":[{"message":{"content":"unsafe usage"},"finish_reason":"stop"}],"usage":{`+test.usage+`}}`)
			})

			partial, err := client.Complete(context.Background(), validChatRequest())
			var modelErr *Error
			if !errors.As(err, &modelErr) || modelErr.ResponseDiagnostic == nil || modelErr.ResponseDiagnostic.Reason != ProviderResponseNegativeUsage {
				t.Fatalf("error=%#v", err)
			}
			diagnosticUsage := modelErr.ResponseDiagnostic.Usage
			if diagnosticUsage.InputTokens != test.wantInput || diagnosticUsage.OutputTokens != test.wantOutput {
				t.Fatalf("diagnostic usage=%+v", diagnosticUsage)
			}
			if test.cachedPresent {
				if diagnosticUsage.CachedInputTokens == nil || *diagnosticUsage.CachedInputTokens != test.wantCached {
					t.Fatalf("diagnostic cached usage=%+v", diagnosticUsage)
				}
			}
			if partial.Usage != (Usage{}) {
				t.Fatalf("accounting partial usage=%+v", partial.Usage)
			}
			if modelErr.PartialChatResponse() == nil || modelErr.PartialChatResponse().Usage != (Usage{}) {
				t.Fatalf("error partial=%+v", modelErr.PartialChatResponse())
			}
		})
	}
}

func TestCompleteSanitizesDiagnosticPreview(t *testing.T) {
	t.Parallel()
	secret := "configured-api-key"
	body := `{"choices":[],"api_key":"configured-api-key","authorization":"Bearer bearer-value","token":"token-value","password":"password-value","cookie":"cookie-value","secret":"secret-value","credential":"credential-value","signed_url":"https://cdn.example/file?signature=signed-value","unix":"/Users/qingyang/private/file.txt","windows":"C:\\Users\\private\\file.txt"}` + strings.Repeat("敏感", maxDiagnosticPreview)
	client := completeTestClient(func() *http.Response { return response(http.StatusOK, "application/json", body) })
	request := validChatRequest()
	request.APIKey = secret

	_, err := client.Complete(context.Background(), request)
	var modelErr *Error
	if !errors.As(err, &modelErr) || modelErr.ResponseDiagnostic == nil {
		t.Fatalf("error = %#v", err)
	}
	preview := modelErr.ResponseDiagnostic.Preview
	if len(preview) > maxDiagnosticPreview {
		t.Fatalf("preview is %d bytes", len(preview))
	}
	for _, forbidden := range []string{secret, "bearer-value", "token-value", "password-value", "cookie-value", "secret-value", "credential-value", "signed-value", "/Users/qingyang", `C:\Users\private`} {
		if strings.Contains(preview, forbidden) {
			t.Fatalf("preview leaked %q: %s", forbidden, preview)
		}
	}
	for _, expected := range []string{"[REDACTED]", "[REDACTED_URL]", "[REDACTED_PATH]"} {
		if !strings.Contains(preview, expected) {
			t.Fatalf("preview missing %q: %s", expected, preview)
		}
	}
}

func TestCompleteNetworkFailureHasNoProviderResponseDiagnostic(t *testing.T) {
	t.Parallel()
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, &net.DNSError{Err: "temporary failure", Name: "provider.example", IsTemporary: true}
	})})
	_, err := client.Complete(context.Background(), validChatRequest())
	var modelErr *Error
	if !errors.As(err, &modelErr) || modelErr.Code != CodeNetwork {
		t.Fatalf("error = %#v", err)
	}
	if modelErr.ResponseDiagnostic != nil || modelErr.PartialResponse != nil {
		t.Fatalf("network error has response diagnostics: %+v", modelErr)
	}
}

func completeTestClient(responseFactory func() *http.Response) *OpenAICompatibleClient {
	return NewOpenAICompatibleClient(&http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return responseFactory(), nil
	})})
}

func validChatRequest() ChatRequest {
	return ChatRequest{
		BaseURL: "https://provider.example/v1",
		APIKey:  "secret",
		Model:   "agent-model",
		Messages: []ChatMessage{{
			Role: "user", Content: "hello",
		}},
	}
}

func sameOptionalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
