package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"lumi/internal/provider"
	"lumi/internal/providerdiag"
)

const maxImageBytes = 64 << 20

var errUnsafeDownloadURL = errors.New("unsafe provider image URL")

type Error struct {
	Code, SafeMessage string
	Retryable         bool
	Cause             error
	Diagnostic        providerdiag.Details
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
	ProviderType, BaseURL, APIKey, Model, Prompt, Size, Quality string
	Images                                                      []ImageInput
}

type ImageInput struct {
	MIMEType string
	Data     []byte
}
type Response struct {
	Bytes                   []byte
	MIMEType, RevisedPrompt string
}
type Client interface {
	Generate(context.Context, Request) (Response, error)
}

type OpenAICompatibleClient struct{ http *http.Client }

func NewOpenAICompatibleClient(client *http.Client) *OpenAICompatibleClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	return &OpenAICompatibleClient{http: client}
}

func (client *OpenAICompatibleClient) Generate(ctx context.Context, input Request) (Response, error) {
	if strings.TrimSpace(input.Model) == "" || strings.TrimSpace(input.Prompt) == "" {
		return Response{}, &Error{Code: "image_invalid_input", SafeMessage: "图片模型与 Prompt 不能为空。"}
	}
	switch input.ProviderType {
	case provider.TypeAliyunBailian:
		return client.generateBailian(ctx, input)
	case provider.TypeCloudflareAIGateway:
		return client.generateCloudflare(ctx, input)
	default:
		return Response{}, &Error{Code: "image_provider_unsupported", SafeMessage: "图片 Provider 不受支持。"}
	}
}

func (client *OpenAICompatibleClient) generateCloudflare(ctx context.Context, input Request) (Response, error) {
	content := []any{map[string]any{"type": "input_text", "text": input.Prompt}}
	for _, image := range input.Images {
		mimeType := strings.ToLower(strings.TrimSpace(image.MIMEType))
		if len(image.Data) == 0 || len(image.Data) > maxImageBytes || (mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/webp") {
			return Response{}, &Error{Code: "image_invalid_input", SafeMessage: "参考图片为空、过大或格式不受支持。"}
		}
		content = append(content, map[string]any{"type": "input_image", "image_url": "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(image.Data)})
	}
	tool := map[string]any{"type": "image_generation", "action": "generate", "moderation": "low"}
	if strings.TrimSpace(input.Size) != "" {
		tool["size"] = strings.TrimSpace(input.Size)
	}
	if strings.TrimSpace(input.Quality) != "" {
		tool["quality"] = strings.TrimSpace(input.Quality)
	}
	payload := map[string]any{
		"model": input.Model,
		"input": []any{map[string]any{"role": "user", "content": content}},
		"tools": []any{tool},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, &Error{Code: "image_invalid_input", SafeMessage: "无法编码 Cloudflare 图片请求。", Cause: err}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(input.BaseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return Response{}, classify(err, 0)
	}
	request.Header.Set("Authorization", "Bearer "+input.APIKey)
	request.Header.Set("Content-Type", "application/json")
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(input.Model)), "@cf/") {
		request.Header.Set("cf-aig-gateway-id", "default")
	}
	response, err := client.http.Do(request)
	if err != nil {
		return Response{}, classify(err, 0)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		diagnostic := providerdiag.ReadHTTPError(response, input.APIKey)
		return Response{}, classify(fmt.Errorf("Cloudflare returned HTTP %d", response.StatusCode), response.StatusCode, diagnostic)
	}
	var envelope struct {
		Output []struct {
			Type          string `json:"type"`
			Result        string `json:"result"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"output"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 96<<20)).Decode(&envelope); err != nil {
		return Response{}, &Error{Code: "image_invalid_response", SafeMessage: "Cloudflare 返回了无法解析的图片响应。", Retryable: true, Cause: err}
	}
	for _, item := range envelope.Output {
		if item.Type != "image_generation_call" || strings.TrimSpace(item.Result) == "" {
			continue
		}
		imageBytes, err := base64.StdEncoding.DecodeString(item.Result)
		if err != nil || len(imageBytes) == 0 || len(imageBytes) > maxImageBytes {
			return Response{}, &Error{Code: "image_invalid_response", SafeMessage: "Cloudflare 图片内容无效。", Cause: err}
		}
		mimeType := http.DetectContentType(imageBytes)
		if mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/webp" {
			return Response{}, &Error{Code: "image_invalid_response", SafeMessage: "Cloudflare 返回的内容不是支持的图片。"}
		}
		return Response{Bytes: imageBytes, MIMEType: mimeType, RevisedPrompt: item.RevisedPrompt}, nil
	}
	return Response{}, &Error{Code: "image_invalid_response", SafeMessage: "Cloudflare 未返回 image_generation_call。", Retryable: true}
}

func (client *OpenAICompatibleClient) generateBailian(ctx context.Context, input Request) (Response, error) {
	parameters := map[string]any{"prompt_extend": true, "n": 1, "watermark": false}
	if size := strings.TrimSpace(input.Size); size != "" {
		parameters["size"] = strings.ReplaceAll(size, "x", "*")
	}
	messageContent := make([]any, 0, len(input.Images)+1)
	for _, image := range input.Images {
		if len(image.Data) == 0 || len(image.Data) > maxImageBytes || (image.MIMEType != "image/png" && image.MIMEType != "image/jpeg" && image.MIMEType != "image/webp") {
			return Response{}, &Error{Code: "image_invalid_input", SafeMessage: "参考图片为空、过大或格式不受支持。"}
		}
		messageContent = append(messageContent, map[string]any{"image": "data:" + image.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(image.Data)})
	}
	messageContent = append(messageContent, map[string]any{"text": input.Prompt})
	payload := map[string]any{
		"model": input.Model,
		"input": map[string]any{"messages": []any{map[string]any{
			"role": "user", "content": messageContent,
		}}},
		"parameters": parameters,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, &Error{Code: "image_invalid_input", SafeMessage: "无法编码百炼图片请求。", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(input.BaseURL), bytes.NewReader(body))
	if err != nil {
		return Response{}, classify(err, 0)
	}
	req.Header.Set("Authorization", "Bearer "+input.APIKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(req)
	if err != nil {
		return Response{}, classify(err, 0)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		diagnostic := providerdiag.ReadHTTPError(response, input.APIKey)
		return Response{}, classify(fmt.Errorf("Bailian returned HTTP %d", response.StatusCode), response.StatusCode, diagnostic)
	}
	var envelope struct {
		Output struct {
			Choices []struct {
				Message struct {
					Content []struct {
						Image string `json:"image"`
					} `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		} `json:"output"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&envelope); err != nil || len(envelope.Output.Choices) == 0 || len(envelope.Output.Choices[0].Message.Content) == 0 {
		return Response{}, &Error{Code: "image_invalid_response", SafeMessage: "百炼未返回可用图片。", Retryable: true, Cause: err}
	}
	imageURL := ""
	for _, choice := range envelope.Output.Choices {
		for _, content := range choice.Message.Content {
			if strings.TrimSpace(content.Image) != "" {
				imageURL = content.Image
				break
			}
		}
		if imageURL != "" {
			break
		}
	}
	if strings.TrimSpace(imageURL) == "" {
		return Response{}, &Error{Code: "image_invalid_response", SafeMessage: "百炼未返回图片 URL。"}
	}
	content, err := client.download(ctx, imageURL)
	if err != nil {
		return Response{}, err
	}
	if len(content) == 0 || len(content) > maxImageBytes {
		return Response{}, &Error{Code: "image_invalid_response", SafeMessage: "百炼图片为空或超过 64MB。"}
	}
	mime := http.DetectContentType(content)
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/gif" && mime != "image/webp" {
		return Response{}, &Error{Code: "image_invalid_response", SafeMessage: "百炼返回的内容不是支持的图片。"}
	}
	return Response{Bytes: content, MIMEType: mime}, nil
}

func (client *OpenAICompatibleClient) download(ctx context.Context, raw string) ([]byte, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !safeDownloadURL(parsed) {
		return nil, &Error{Code: "image_invalid_response", SafeMessage: "Provider 图片 URL 不安全。", Cause: err}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	downloadClient := *client.http
	previousRedirectPolicy := downloadClient.CheckRedirect
	downloadClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if !safeDownloadURL(next.URL) {
			return errUnsafeDownloadURL
		}
		if previousRedirectPolicy != nil {
			return previousRedirectPolicy(next, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	response, err := downloadClient.Do(req)
	if err != nil {
		if errors.Is(err, errUnsafeDownloadURL) {
			return nil, &Error{Code: "image_invalid_response", SafeMessage: "Provider 图片重定向 URL 不安全。", Cause: err}
		}
		return nil, classify(err, 0)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		diagnostic := providerdiag.ReadHTTPError(response, "")
		return nil, classify(fmt.Errorf("image download HTTP %d", response.StatusCode), response.StatusCode, diagnostic)
	}
	if response.Request != nil && response.Request.URL != nil && !safeDownloadURL(response.Request.URL) {
		return nil, &Error{Code: "image_invalid_response", SafeMessage: "Provider 图片重定向 URL 不安全。"}
	}
	if response.ContentLength > maxImageBytes {
		return nil, &Error{Code: "image_too_large", SafeMessage: "Provider 图片超过 64MB。"}
	}
	limited := &io.LimitedReader{R: response.Body, N: maxImageBytes + 1}
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, classify(err, 0)
	}
	if len(content) > maxImageBytes {
		return nil, &Error{Code: "image_too_large", SafeMessage: "Provider 图片超过 64MB。"}
	}
	return content, nil
}

func safeDownloadURL(parsed *url.URL) bool {
	if parsed == nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if address := net.ParseIP(host); address != nil {
		return address.IsGlobalUnicast() && !address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast() && !address.IsLinkLocalMulticast()
	}
	return true
}

func classify(err error, status int, diagnostics ...providerdiag.Details) error {
	diagnostic := providerdiag.Details{}
	if len(diagnostics) > 0 {
		diagnostic = diagnostics[0]
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Code: "image_cancelled", SafeMessage: "图片生成已取消。", Cause: err, Diagnostic: diagnostic}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: "image_timeout", SafeMessage: "图片生成超时。", Retryable: true, Cause: err, Diagnostic: diagnostic}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return &Error{Code: "image_network_error", SafeMessage: "无法连接图片 Provider。", Retryable: true, Cause: err, Diagnostic: diagnostic}
	}
	if status == 429 || status >= 500 {
		return &Error{Code: "image_provider_unavailable", SafeMessage: "图片 Provider 暂时不可用。", Retryable: true, Cause: err, Diagnostic: diagnostic}
	}
	if status == 401 || status == 403 {
		return &Error{Code: "image_authentication_failed", SafeMessage: "图片 Provider 鉴权失败。", Cause: err, Diagnostic: diagnostic}
	}
	return &Error{Code: "image_provider_error", SafeMessage: "图片 Provider 拒绝了请求。", Cause: err, Diagnostic: diagnostic}
}
