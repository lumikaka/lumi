package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"lumi/internal/providerdiag"
)

const (
	CodeNetwork          = "llm_network_error"
	CodeTimeout          = "llm_timeout"
	CodeAuthentication   = "llm_authentication_failed"
	CodeRateLimited      = "llm_rate_limited"
	CodeProviderResponse = "llm_provider_response_error"
	CodeInvalidContent   = "llm_invalid_content"
	CodeModelUnavailable = "llm_model_unavailable"
	CodeCancelled        = "llm_cancelled"
)

type Error struct {
	Code        string
	SafeMessage string
	Retryable   bool
	Cause       error
	Diagnostic  providerdiag.Details
}

func (err *Error) Error() string {
	if err.Cause != nil {
		return fmt.Sprintf("%s: %v", err.Code, err.Cause)
	}
	return err.Code
}

func (err *Error) Unwrap() error { return err.Cause }

func (err *Error) ProviderDiagnostic() providerdiag.Details { return err.Diagnostic }

type Request struct {
	BaseURL      string
	APIKey       string `json:"-"`
	Model        string
	SystemPrompt string
	Prompt       string
	Images       []ImageInput
	Temperature  *float64
	MaxTokens    int
}

type ImageInput struct {
	MIMEType string
	Data     []byte
	Detail   string
}

type Usage struct {
	InputTokens       int  `json:"input_tokens"`
	CachedInputTokens *int `json:"cached_input_tokens,omitempty"`
	OutputTokens      int  `json:"output_tokens"`
}

type Response struct {
	Content      string
	Usage        Usage
	FinishReason string
}

// ChatMessage and ToolDefinition are the deliberately small OpenAI-compatible
// subset used by Lumi's project agent runtime. They live in the provider
// package so the agent package does not need to know about HTTP wire details.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ChatRequest struct {
	BaseURL     string
	APIKey      string `json:"-"`
	Model       string
	Messages    []ChatMessage
	Tools       []ToolDefinition
	Temperature *float64
	MaxTokens   int
}

type ChatResponse struct {
	Message      ChatMessage
	Usage        Usage
	FinishReason string
}

type ToolClient interface {
	Complete(context.Context, ChatRequest) (ChatResponse, error)
}

type Client interface {
	Generate(context.Context, Request, func(string) error) (Response, error)
}

type OpenAICompatibleClient struct {
	http *http.Client
}

func NewOpenAICompatibleClient(client *http.Client) *OpenAICompatibleClient {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	return &OpenAICompatibleClient{http: client}
}

func (client *OpenAICompatibleClient) Generate(ctx context.Context, input Request, onDelta func(string) error) (Response, error) {
	if strings.TrimSpace(input.Model) == "" || strings.TrimSpace(input.Prompt) == "" {
		return Response{}, &Error{Code: CodeInvalidContent, SafeMessage: "模型或 Prompt 不能为空。"}
	}
	messages := make([]map[string]any, 0, 2)
	if strings.TrimSpace(input.SystemPrompt) != "" {
		messages = append(messages, map[string]any{"role": "system", "content": input.SystemPrompt})
	}
	userContent := any(input.Prompt)
	if len(input.Images) > 0 {
		content := []any{map[string]any{"type": "text", "text": input.Prompt}}
		for _, image := range input.Images {
			mimeType := strings.ToLower(strings.TrimSpace(image.MIMEType))
			if len(image.Data) == 0 || len(image.Data) > 64<<20 || (mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/gif" && mimeType != "image/webp") {
				return Response{}, &Error{Code: CodeInvalidContent, SafeMessage: "模型图片输入为空、过大或格式不受支持。"}
			}
			detail := strings.ToLower(strings.TrimSpace(image.Detail))
			if detail != "low" && detail != "high" && detail != "auto" {
				detail = "high"
			}
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{
				"url":    "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(image.Data),
				"detail": detail,
			}})
		}
		userContent = content
	}
	messages = append(messages, map[string]any{"role": "user", "content": userContent})
	payload := map[string]any{"model": input.Model, "messages": messages, "stream": true, "stream_options": map[string]any{"include_usage": true}}
	if input.Temperature != nil && !openAIGPT5Model(input.Model) {
		payload["temperature"] = *input.Temperature
	}
	if input.MaxTokens > 0 {
		payload[tokenLimitKey(input.Model)] = input.MaxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, &Error{Code: CodeInvalidContent, SafeMessage: "无法编码模型请求。", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(input.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, classify(err, 0)
	}
	req.Header.Set("Authorization", "Bearer "+input.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	setCloudflareGatewayHeader(req, input.Model)
	response, err := client.http.Do(req)
	if err != nil {
		return Response{}, classify(err, 0)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		diagnostic := providerdiag.ReadHTTPError(response, input.APIKey)
		return Response{}, classify(fmt.Errorf("provider returned HTTP %d", response.StatusCode), response.StatusCode, diagnostic)
	}
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return readStream(ctx, response.Body, onDelta)
	}
	return readJSON(response.Body, onDelta)
}

func (client *OpenAICompatibleClient) Complete(ctx context.Context, input ChatRequest) (ChatResponse, error) {
	if strings.TrimSpace(input.Model) == "" || len(input.Messages) == 0 {
		return ChatResponse{}, &Error{Code: CodeInvalidContent, SafeMessage: "模型或消息不能为空。"}
	}
	tools := make([]map[string]any, 0, len(input.Tools))
	for _, tool := range input.Tools {
		if strings.TrimSpace(tool.Name) == "" || tool.Parameters == nil {
			return ChatResponse{}, &Error{Code: CodeInvalidContent, SafeMessage: "工具定义无效。"}
		}
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
			"name": tool.Name, "description": tool.Description, "parameters": tool.Parameters,
		}})
	}
	messages := make([]map[string]any, 0, len(input.Messages))
	for _, message := range input.Messages {
		item := map[string]any{"role": message.Role}
		if message.Content != "" || message.Role != "assistant" {
			item["content"] = message.Content
		}
		if message.ToolCallID != "" {
			item["tool_call_id"] = message.ToolCallID
		}
		if len(message.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				calls = append(calls, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": call.Arguments}})
			}
			item["tool_calls"] = calls
		}
		messages = append(messages, item)
	}
	payload := map[string]any{"model": input.Model, "messages": messages, "stream": false}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}
	if input.Temperature != nil && !openAIGPT5Model(input.Model) {
		payload["temperature"] = *input.Temperature
	}
	if input.MaxTokens > 0 {
		payload[tokenLimitKey(input.Model)] = input.MaxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, &Error{Code: CodeInvalidContent, SafeMessage: "无法编码模型请求。", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(input.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, classify(err, 0)
	}
	req.Header.Set("Authorization", "Bearer "+input.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	setCloudflareGatewayHeader(req, input.Model)
	response, err := client.http.Do(req)
	if err != nil {
		return ChatResponse{}, classify(err, 0)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		diagnostic := providerdiag.ReadHTTPError(response, input.APIKey)
		return ChatResponse{}, classify(fmt.Errorf("provider returned HTTP %d", response.StatusCode), response.StatusCode, diagnostic)
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&envelope); err != nil {
		return ChatResponse{}, &Error{Code: CodeProviderResponse, SafeMessage: "Provider 返回了无法解析的响应。", Retryable: true, Cause: err}
	}
	if len(envelope.Choices) == 0 {
		return ChatResponse{}, &Error{Code: CodeInvalidContent, SafeMessage: "Provider 未返回可用消息。"}
	}
	choice := envelope.Choices[0]
	message := ChatMessage{Role: "assistant", Content: strings.TrimSpace(choice.Message.Content)}
	for _, call := range choice.Message.ToolCalls {
		if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Function.Name) == "" || !json.Valid([]byte(call.Function.Arguments)) {
			return ChatResponse{}, &Error{Code: CodeInvalidContent, SafeMessage: "Provider 返回了无效工具调用。"}
		}
		message.ToolCalls = append(message.ToolCalls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
	}
	if message.Content == "" && len(message.ToolCalls) == 0 {
		return ChatResponse{}, &Error{Code: CodeInvalidContent, SafeMessage: "Provider 未返回可用正文或工具调用。"}
	}
	return ChatResponse{Message: message, Usage: Usage{InputTokens: envelope.Usage.PromptTokens, CachedInputTokens: cachedTokens(envelope.Usage.PromptTokensDetails), OutputTokens: envelope.Usage.CompletionTokens}, FinishReason: choice.FinishReason}, nil
}

func tokenLimitKey(model string) string {
	if openAIGPT5Model(model) {
		return "max_completion_tokens"
	}
	return "max_tokens"
}

func openAIGPT5Model(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "openai/gpt-5") || strings.HasPrefix(model, "gpt-5")
}

func setCloudflareGatewayHeader(request *http.Request, model string) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "@cf/") {
		request.Header.Set("cf-aig-gateway-id", "default")
	}
}

type completionEnvelope struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

func cachedTokens(details *struct {
	CachedTokens int `json:"cached_tokens"`
}) *int {
	if details == nil {
		return nil
	}
	value := details.CachedTokens
	return &value
}

func readStream(ctx context.Context, reader io.Reader, onDelta func(string) error) (Response, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 2<<20)
	var result Response
	var content strings.Builder
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return Response{}, classify(ctx.Err(), 0)
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event completionEnvelope
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return Response{}, &Error{Code: CodeProviderResponse, SafeMessage: "Provider 返回了无法解析的流式响应。", Retryable: true, Cause: err}
		}
		if event.Usage.PromptTokens > 0 {
			result.Usage.InputTokens = event.Usage.PromptTokens
		}
		if event.Usage.CompletionTokens > 0 {
			result.Usage.OutputTokens = event.Usage.CompletionTokens
		}
		if event.Usage.PromptTokensDetails != nil {
			result.Usage.CachedInputTokens = cachedTokens(event.Usage.PromptTokensDetails)
		}
		for _, choice := range event.Choices {
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				if onDelta != nil {
					if err := onDelta(choice.Delta.Content); err != nil {
						return Response{}, err
					}
				}
			}
			if choice.FinishReason != "" {
				result.FinishReason = choice.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Response{}, classify(err, 0)
	}
	result.Content = strings.TrimSpace(content.String())
	if result.Content == "" {
		return Response{}, &Error{Code: CodeInvalidContent, SafeMessage: "Provider 未返回可用正文。"}
	}
	return result, nil
}

func readJSON(reader io.Reader, onDelta func(string) error) (Response, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 4<<20))
	var envelope completionEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return Response{}, &Error{Code: CodeProviderResponse, SafeMessage: "Provider 返回了无法解析的响应。", Retryable: true, Cause: err}
	}
	if len(envelope.Choices) == 0 || strings.TrimSpace(envelope.Choices[0].Message.Content) == "" {
		return Response{}, &Error{Code: CodeInvalidContent, SafeMessage: "Provider 未返回可用正文。"}
	}
	content := strings.TrimSpace(envelope.Choices[0].Message.Content)
	if onDelta != nil {
		if err := onDelta(content); err != nil {
			return Response{}, err
		}
	}
	return Response{Content: content, Usage: Usage{InputTokens: envelope.Usage.PromptTokens, CachedInputTokens: cachedTokens(envelope.Usage.PromptTokensDetails), OutputTokens: envelope.Usage.CompletionTokens}, FinishReason: envelope.Choices[0].FinishReason}, nil
}

func classify(err error, status int, diagnostics ...providerdiag.Details) error {
	diagnostic := providerdiag.Details{}
	if len(diagnostics) > 0 {
		diagnostic = diagnostics[0]
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Code: CodeCancelled, SafeMessage: "请求已取消。", Cause: err, Diagnostic: diagnostic}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: CodeTimeout, SafeMessage: "Provider 请求超时。", Retryable: true, Cause: err, Diagnostic: diagnostic}
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return &Error{Code: CodeNetwork, SafeMessage: "无法连接 Provider。", Retryable: true, Cause: err, Diagnostic: diagnostic}
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &Error{Code: CodeAuthentication, SafeMessage: "Provider 鉴权失败，请检查 API Key。", Cause: err, Diagnostic: diagnostic}
	case http.StatusTooManyRequests:
		return &Error{Code: CodeRateLimited, SafeMessage: "Provider 当前限流，请稍后重试。", Retryable: true, Cause: err, Diagnostic: diagnostic}
	}
	if status >= 500 {
		return &Error{Code: CodeProviderResponse, SafeMessage: "Provider 服务暂时不可用。", Retryable: true, Cause: err, Diagnostic: diagnostic}
	}
	return &Error{Code: CodeProviderResponse, SafeMessage: "Provider 拒绝了模型请求。", Cause: err, Diagnostic: diagnostic}
}
