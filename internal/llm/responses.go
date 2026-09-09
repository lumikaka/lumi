package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"lumi/internal/providerdiag"
)

func responsesPayload(model string, input []any, temperature *float64, maxTokens int, stream bool) map[string]any {
	payload := map[string]any{"model": model, "input": input, "stream": stream, "store": false}
	if maxTokens > 0 {
		payload["max_output_tokens"] = maxTokens
	}
	if temperature != nil && !openAIGPT5Model(model) {
		payload["temperature"] = *temperature
	}
	return payload
}

func (client *OpenAICompatibleClient) generateResponses(ctx context.Context, input Request, onDelta func(string) error) (Response, error) {
	if strings.TrimSpace(input.Model) == "" || strings.TrimSpace(input.Prompt) == "" {
		return Response{}, &Error{Code: CodeInvalidContent, SafeMessage: "模型或 Prompt 不能为空。"}
	}
	items := make([]any, 0, 2)
	if strings.TrimSpace(input.SystemPrompt) != "" {
		items = append(items, map[string]any{"role": "system", "content": input.SystemPrompt})
	}
	content := []any{map[string]any{"type": "input_text", "text": input.Prompt}}
	for _, image := range input.Images {
		mime := strings.ToLower(strings.TrimSpace(image.MIMEType))
		if len(image.Data) == 0 || len(image.Data) > 64<<20 || (mime != "image/png" && mime != "image/jpeg" && mime != "image/gif" && mime != "image/webp") {
			return Response{}, &Error{Code: CodeInvalidContent, SafeMessage: "模型图片输入为空、过大或格式不受支持。"}
		}
		detail := strings.ToLower(strings.TrimSpace(image.Detail))
		if detail != "low" && detail != "high" && detail != "auto" {
			detail = "high"
		}
		content = append(content, map[string]any{"type": "input_image", "image_url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image.Data), "detail": detail})
	}
	items = append(items, map[string]any{"role": "user", "content": content})
	result, err := client.sendResponses(ctx, input.BaseURL, input.APIKey, responsesPayload(input.Model, items, input.Temperature, input.MaxTokens, true), onDelta, false)
	return Response{Content: result.Message.Content, Usage: result.Usage, FinishReason: result.FinishReason}, err
}

func (client *OpenAICompatibleClient) completeResponses(ctx context.Context, input ChatRequest) (ChatResponse, error) {
	if strings.TrimSpace(input.Model) == "" || len(input.Messages) == 0 {
		return ChatResponse{}, &Error{Code: CodeInvalidContent, SafeMessage: "模型或消息不能为空。"}
	}
	items := make([]any, 0, len(input.Messages))
	for _, message := range input.Messages {
		if message.Role == "assistant" && len(message.ResponsesOutput) > 0 {
			for _, item := range message.ResponsesOutput {
				items = append(items, item)
			}
			continue
		}
		if message.Role == "tool" {
			if strings.TrimSpace(message.ToolCallID) == "" {
				return ChatResponse{}, &Error{Code: CodeInvalidContent, SafeMessage: "工具结果缺少调用标识。"}
			}
			items = append(items, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": message.Content})
			continue
		}
		if message.Content != "" || message.Role != "assistant" {
			items = append(items, map[string]any{"role": message.Role, "content": message.Content})
		}
		for _, call := range message.ToolCalls {
			items = append(items, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": call.Arguments})
		}
	}
	payload := responsesPayload(input.Model, items, input.Temperature, input.MaxTokens, false)
	// Keep reasoning items usable after process restarts without server-side state.
	payload["include"] = []string{"reasoning.encrypted_content"}
	if len(input.Tools) > 0 {
		tools := make([]any, 0, len(input.Tools))
		for _, tool := range input.Tools {
			if strings.TrimSpace(tool.Name) == "" || tool.Parameters == nil {
				return ChatResponse{}, &Error{Code: CodeInvalidContent, SafeMessage: "工具定义无效。"}
			}
			tools = append(tools, map[string]any{"type": "function", "name": tool.Name, "description": tool.Description, "parameters": tool.Parameters, "strict": false})
		}
		payload["tools"], payload["tool_choice"] = tools, "auto"
	}
	return client.sendResponses(ctx, input.BaseURL, input.APIKey, payload, nil, false)
}

func (client *OpenAICompatibleClient) sendResponses(ctx context.Context, baseURL, apiKey string, payload map[string]any, onDelta func(string) error, probe bool) (ChatResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, &Error{Code: CodeInvalidContent, SafeMessage: "无法编码模型请求。", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, classify(err, 0)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if payload["stream"] == true {
		req.Header.Set("Accept", "text/event-stream, application/json")
	}
	model, _ := payload["model"].(string)
	setCloudflareGatewayHeader(req, model)
	response, err := client.http.Do(req)
	if err != nil {
		return ChatResponse{}, classify(err, 0)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		diagnostic := providerdiag.ReadHTTPError(response, apiKey)
		return ChatResponse{}, classify(fmt.Errorf("provider returned HTTP %d", response.StatusCode), response.StatusCode, diagnostic)
	}
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return readResponsesStream(ctx, response, apiKey, onDelta, probe)
	}
	body, length, truncated, err := readBoundedChatResponse(response.Body, response.ContentLength)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ChatResponse{}, classify(err, 0)
		}
		return invalidChatResponse(response, apiKey, body, length, true, ProviderResponseBodyReadError, nil, nil, ChatResponse{}, err)
	}
	if truncated {
		return invalidChatResponse(response, apiKey, body, length, true, ProviderResponseBodyTooLarge, nil, nil, ChatResponse{}, nil)
	}
	result, err := parseResponsesBody(response, apiKey, body, probe)
	if err == nil && onDelta != nil && result.Message.Content != "" {
		err = onDelta(result.Message.Content)
	}
	return result, err
}

type responsesEnvelope struct {
	Status            string            `json:"status"`
	Output            []json.RawMessage `json:"output"`
	Error             json.RawMessage   `json:"error"`
	IncompleteDetails struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Usage struct {
		InputTokens        *int `json:"input_tokens"`
		OutputTokens       *int `json:"output_tokens"`
		InputTokensDetails *struct {
			CachedTokens *int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

type responsesOutputItem struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Status    string          `json:"status"`
	CallID    json.RawMessage `json:"call_id"`
	Name      json.RawMessage `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func parseResponsesBody(response *http.Response, apiKey string, body []byte, probe bool) (ChatResponse, error) {
	partial := ChatResponse{Message: ChatMessage{Role: "assistant"}}
	invalid := func(reason ProviderResponseFailureReason, toolIndex *int, cause error) (ChatResponse, error) {
		return invalidChatResponse(response, apiKey, body, int64(len(body)), false, reason, nil, toolIndex, partial, cause)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return invalid(ProviderResponseEmptyBody, nil, nil)
	}
	var envelope responsesEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&envelope); err != nil {
		return invalid(ProviderResponseMalformedJSON, nil, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return invalid(ProviderResponseTrailingJSON, nil, nil)
	}
	partial.Usage = wireUsage{PromptTokens: envelope.Usage.InputTokens, CompletionTokens: envelope.Usage.OutputTokens, PromptTokensDetails: envelope.Usage.InputTokensDetails}.normalized()
	if hasNegativeUsage(partial.Usage) {
		return invalid(ProviderResponseNegativeUsage, nil, nil)
	}
	partial.FinishReason = envelope.Status
	if envelope.Status == "failed" || (len(envelope.Error) > 0 && string(envelope.Error) != "null") {
		diagnosticResponse := *response
		diagnosticResponse.Body = io.NopCloser(bytes.NewReader(body))
		diagnostic := providerdiag.ReadHTTPError(&diagnosticResponse, apiKey)
		failure := &Error{Code: CodeProviderResponse, SafeMessage: "Provider 拒绝了模型请求。", Diagnostic: diagnostic, PartialResponse: &partial}
		return partial, failure
	}
	if envelope.Status != "completed" && envelope.Status != "incomplete" {
		return invalid(ProviderResponseInvalidStatus, nil, nil)
	}
	// Check status before accepting any tool call; incomplete calls never execute.
	if envelope.Status == "incomplete" {
		if envelope.IncompleteDetails.Reason == "max_output_tokens" {
			partial.FinishReason = "length"
			if probe {
				return partial, nil
			}
			return invalid(ProviderResponseFinishReasonLength, nil, nil)
		}
		return invalid(ProviderResponseInvalidStatus, nil, nil)
	}
	seen := map[string]bool{}
	var content strings.Builder
	for index, raw := range envelope.Output {
		var item responsesOutputItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return invalid(ProviderResponseMalformedJSON, intPointer(index), err)
		}
		switch item.Type {
		case "message":
			if item.Role != "assistant" || (item.Status != "" && item.Status != "completed") {
				return invalid(ProviderResponseInvalidStatus, intPointer(index), nil)
			}
			for _, part := range item.Content {
				if part.Type == "output_text" {
					content.WriteString(part.Text)
				}
			}
		case "function_call":
			if item.Status != "" && item.Status != "completed" {
				return invalid(ProviderResponseInvalidStatus, intPointer(index), nil)
			}
			callID, ok := requiredJSONString(item.CallID)
			if !ok {
				return invalid(ProviderResponseMissingToolCallID, intPointer(index), nil)
			}
			if seen[callID] {
				return invalid(ProviderResponseDuplicateToolCallID, intPointer(index), nil)
			}
			seen[callID] = true
			name, ok := requiredJSONString(item.Name)
			if !ok {
				return invalid(ProviderResponseMissingToolName, intPointer(index), nil)
			}
			arguments, ok := jsonString(item.Arguments)
			if !ok {
				return invalid(ProviderResponseToolArgumentsWrongType, intPointer(index), nil)
			}
			if len(arguments) > maxToolArgumentsBytes {
				return invalid(ProviderResponseToolArgumentsTooLarge, intPointer(index), nil)
			}
			partial.Message.ToolCalls = append(partial.Message.ToolCalls, ToolCall{ID: callID, Name: name, Arguments: arguments})
		}
	}
	partial.Message.Content = strings.TrimSpace(content.String())
	if len(partial.Message.ToolCalls) > 1 {
		for index, call := range partial.Message.ToolCalls {
			if call.Name == "request_user_input" {
				return invalid(ProviderResponseRequestUserInputMixed, intPointer(index), nil)
			}
		}
	}
	if partial.Message.Content == "" && len(partial.Message.ToolCalls) == 0 {
		return invalid(ProviderResponseEmptyMessage, nil, nil)
	}
	partial.FinishReason = "stop"
	if len(partial.Message.ToolCalls) > 0 {
		partial.FinishReason = "tool_calls"
	}
	partial.Message.ResponsesOutput = envelope.Output
	return partial, nil
}
