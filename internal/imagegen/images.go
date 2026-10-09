package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	_ "golang.org/x/image/webp"
	"lumi/internal/pricing"
	"lumi/internal/provider"
	"lumi/internal/providerdiag"
	"lumi/internal/sitesettings"
)

// APIMode resolves legacy requests without changing other providers' protocols.
func (input Request) APIMode() string {
	switch input.ProviderType {
	case provider.TypeAliyunBailian:
		return "dashscope"
	case provider.TypeCloudflareAIGateway:
		return sitesettings.ImageAPIResponses
	case provider.TypeOpenAIRelay:
		if input.ImageAPIMode == "" {
			return sitesettings.ImageAPIResponses
		}
		return input.ImageAPIMode
	default:
		return ""
	}
}

func (input Request) Endpoint() string {
	base := strings.TrimRight(input.BaseURL, "/")
	switch input.APIMode() {
	case sitesettings.ImageAPIResponses:
		return base + "/responses"
	case sitesettings.ImageAPIImages:
		if len(input.Images) > 0 {
			return base + "/images/edits"
		}
		return base + "/images/generations"
	default:
		return input.BaseURL
	}
}

func (client *OpenAICompatibleClient) generateImages(ctx context.Context, input Request) (Response, error) {
	fields := map[string]string{"model": input.Model, "prompt": input.Prompt, "n": "1", "output_format": "png"}
	if size := strings.TrimSpace(input.Size); size != "" {
		fields["size"] = size
	}
	if quality := strings.TrimSpace(input.Quality); quality != "" {
		fields["quality"] = quality
	}
	var body io.Reader
	var contentLength int64
	contentType := "application/json"
	if len(input.Images) == 0 {
		payload := make(map[string]any, len(fields))
		for key, value := range fields {
			payload[key] = value
		}
		payload["n"] = 1
		encoded, err := json.Marshal(payload)
		if err != nil {
			return Response{}, &Error{Code: "image_invalid_input", SafeMessage: "无法编码图片请求。", Cause: err}
		}
		body = bytes.NewReader(encoded)
		contentLength = int64(len(encoded))
	} else {
		if len(input.Images) > 16 {
			return Response{}, &Error{Code: "image_invalid_input", SafeMessage: "Images API 最多支持 16 张参考图。"}
		}
		// Stream the existing frozen bytes; do not copy all reference images into a
		// second large buffer. Multipart headers and fields remain small.
		var buffer bytes.Buffer
		writer := multipart.NewWriter(&buffer)
		contentType = writer.FormDataContentType()
		for key, value := range fields {
			if err := writer.WriteField(key, value); err != nil {
				return Response{}, classify(err, 0)
			}
		}
		readers := make([]io.Reader, 0, 2*len(input.Images)+1)
		for i, picture := range input.Images {
			mimeType := strings.ToLower(strings.TrimSpace(picture.MIMEType))
			extension := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[mimeType]
			if len(picture.Data) == 0 || len(picture.Data) >= 50<<20 || extension == "" {
				return Response{}, &Error{Code: "image_invalid_input", SafeMessage: "参考图片必须为 PNG、JPEG 或 WebP，且每张小于 50MB。"}
			}
			header := make(textproto.MIMEHeader)
			header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="reference-%d.%s"`, i+1, extension))
			header.Set("Content-Type", mimeType)
			if _, err := writer.CreatePart(header); err != nil {
				return Response{}, classify(err, 0)
			}
			contentLength += int64(buffer.Len()) + int64(len(picture.Data))
			readers = append(readers, bytes.NewReader(bytes.Clone(buffer.Bytes())), bytes.NewReader(picture.Data))
			buffer.Reset()
		}
		if err := writer.Close(); err != nil {
			return Response{}, classify(err, 0)
		}
		contentLength += int64(buffer.Len())
		readers = append(readers, bytes.NewReader(buffer.Bytes()))
		body = io.MultiReader(readers...)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, input.Endpoint(), body)
	if err != nil {
		return Response{}, classify(err, 0)
	}
	req.ContentLength = contentLength
	req.Header.Set("Authorization", "Bearer "+input.APIKey)
	req.Header.Set("Content-Type", contentType)
	response, err := client.http.Do(req)
	if err != nil {
		return Response{}, classify(err, 0)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		diagnostic := providerdiag.ReadHTTPError(response, input.APIKey)
		return Response{}, classify(fmt.Errorf("Provider returned HTTP %d", response.StatusCode), response.StatusCode, diagnostic)
	}
	var envelope struct {
		Data []struct {
			Base64        string `json:"b64_json"`
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
		Usage   *responsesUsage `json:"usage"`
		Size    string          `json:"size"`
		Quality string          `json:"quality"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 96<<20)).Decode(&envelope); err != nil {
		return Response{}, &Error{Code: "image_invalid_response", SafeMessage: "Provider 返回了无法解析的 Images 响应。", Cause: err}
	}
	result := Response{Usage: pricing.Usage{ImageModel: input.Model, InputImages: pricing.Int(int64(len(input.Images))), Size: input.Size, Quality: input.Quality}}
	// Images API has one image model, not a parent-model/tool billing pair.
	if envelope.Usage != nil {
		result.Usage.InputTokens = envelope.Usage.InputTokens
		result.Usage.OutputTokens = envelope.Usage.OutputTokens
		result.Usage.CachedInputTokens = envelope.Usage.InputTokensDetails.CachedTokens
		result.Usage.ImageInputTokens = envelope.Usage.InputTokensDetails.ImageTokens
		result.Usage.ImageOutputTokens = envelope.Usage.OutputTokens
		result.Usage.CachedImageInputTokens = envelope.Usage.InputTokensDetails.CachedTokensDetails.ImageTokens
	}
	if envelope.Size != "" && envelope.Size != "auto" {
		result.Usage.Size = envelope.Size
	}
	if envelope.Quality != "" && envelope.Quality != "auto" {
		result.Usage.Quality = envelope.Quality
	}
	if len(envelope.Data) != 1 {
		return result, &Error{Code: "image_invalid_response", SafeMessage: "Provider 未返回预期的一张图片。"}
	}
	item := envelope.Data[0]
	if item.Base64 != "" || item.URL != "" {
		result.Usage.OutputImages = pricing.Int(1)
	}
	var content []byte
	if item.Base64 != "" {
		content, err = base64.StdEncoding.DecodeString(item.Base64)
	} else if item.URL != "" {
		content, err = client.download(ctx, item.URL)
		if err != nil {
			return result, err
		}
	}
	if err != nil {
		return result, &Error{Code: "image_invalid_response", SafeMessage: "无法读取 Provider 图片。", Cause: err}
	}
	if len(content) == 0 || len(content) > maxImageBytes {
		return result, &Error{Code: "image_invalid_response", SafeMessage: "Provider 图片为空或超过 64MB。"}
	}
	mimeType := http.DetectContentType(content)
	if mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/webp" {
		return result, &Error{Code: "image_invalid_response", SafeMessage: "Provider 返回的内容不是支持的图片。"}
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(content)); err != nil {
		return result, &Error{Code: "image_invalid_response", SafeMessage: "Provider 返回的图片内容无效。", Cause: err}
	}
	result.Bytes, result.MIMEType, result.RevisedPrompt = content, mimeType, item.RevisedPrompt
	return result, nil
}
