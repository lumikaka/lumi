package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestFakeImageProviderSuccessAndInvalidContent(t *testing.T) {
	valid := base64.StdEncoding.EncodeToString(tinyPNG(t))
	calls := 0
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/responses" || request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("Cloudflare request=%s authorization=%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var requestBody struct {
			Model string `json:"model"`
			Tools []struct {
				Type  string `json:"type"`
				Model string `json:"model"`
				Size  string `json:"size"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		if requestBody.Model != "openai/gpt-5.6-terra" || len(requestBody.Tools) != 1 || requestBody.Tools[0].Type != "image_generation" || requestBody.Tools[0].Model != "gpt-image-2.5-flare" || requestBody.Tools[0].Size != "1536x1152" {
			t.Fatalf("request body=%+v", requestBody)
		}
		calls++
		payload := valid
		if calls == 2 {
			payload = base64.StdEncoding.EncodeToString([]byte("not an image"))
		}
		return response(200, fmt.Sprintf(`{"output":[{"type":"image_generation_call","result":%q}]}`, payload)), nil
	})})
	result, err := client.Generate(context.Background(), Request{ProviderType: "cloudflare_ai_gateway", BaseURL: "https://fake.test/v1", APIKey: "secret", Model: "openai/gpt-5.6-terra", Prompt: "draw", Size: "1536x1152"})
	if err != nil || result.MIMEType != "image/png" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := client.Generate(context.Background(), Request{ProviderType: "cloudflare_ai_gateway", BaseURL: "https://fake.test/v1", APIKey: "secret", Model: "openai/gpt-5.6-terra", Prompt: "invalid", Size: "1536x1152"}); err == nil {
		t.Fatal("invalid provider content was accepted")
	}
}

func TestFakeImageProviderInvalidCancelAndTimeout(t *testing.T) {
	t.Run("invalid response", func(t *testing.T) {
		client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return response(200, `{"output":[]}`), nil
		})})
		_, err := client.Generate(context.Background(), Request{ProviderType: "cloudflare_ai_gateway", BaseURL: "https://fake.test/v1", APIKey: "secret", Model: "openai/gpt-5.5", Prompt: "invalid"})
		if err == nil || !strings.Contains(err.Error(), "image_invalid_response") {
			t.Fatalf("invalid response error=%v", err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		})
		client := NewOpenAICompatibleClient(&http.Client{Transport: transport})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := client.Generate(ctx, Request{ProviderType: "cloudflare_ai_gateway", BaseURL: "https://fake.test/v1", APIKey: "secret", Model: "openai/gpt-5.5", Prompt: "cancel"})
		if err == nil || !strings.Contains(err.Error(), "image_cancelled") {
			t.Fatalf("cancel error=%v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		})
		client := NewOpenAICompatibleClient(&http.Client{Transport: transport})
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		_, err := client.Generate(ctx, Request{ProviderType: "cloudflare_ai_gateway", BaseURL: "https://fake.test/v1", APIKey: "secret", Model: "openai/gpt-5.5", Prompt: "timeout"})
		if err == nil || !strings.Contains(err.Error(), "image_timeout") {
			t.Fatalf("timeout error=%v", err)
		}
	})
}

func TestFakeImageProviderDuplicateCallbackIsDeterministic(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString(tinyPNG(t))
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(200, fmt.Sprintf(`{"output":[{"type":"image_generation_call","result":%q}]}`, payload)), nil
	})})
	first, err := client.Generate(context.Background(), Request{ProviderType: "cloudflare_ai_gateway", BaseURL: "https://fake.test/v1", APIKey: "secret", Model: "openai/gpt-5.5", Prompt: "same"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Generate(context.Background(), Request{ProviderType: "cloudflare_ai_gateway", BaseURL: "https://fake.test/v1", APIKey: "secret", Model: "openai/gpt-5.5", Prompt: "same"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes, second.Bytes) {
		t.Fatal("duplicate callback payload changed")
	}
}

func TestImageProviderRejectsPrivateDownloadURLs(t *testing.T) {
	for _, raw := range []string{"http://cdn.example/image.png", "https://localhost/image.png", "https://[::1]/image.png", "https://10.0.0.1/image.png"} {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil || safeDownloadURL(parsed) {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
	t.Run("private redirect", func(t *testing.T) {
		client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			redirect := response(http.StatusFound, "")
			redirect.Header.Set("Location", "https://127.0.0.1/private.png")
			return redirect, nil
		})})
		_, err := client.download(context.Background(), "https://cdn.example.test/start.png")
		if err == nil || !strings.Contains(err.Error(), "image_invalid_response") {
			t.Fatalf("private redirect error=%v", err)
		}
	})
}

func TestBailianImageRequestSizeAndResponse(t *testing.T) {
	imageBytes := tinyPNG(t)
	endpoint := "https://ws-123.cn-beijing.maas.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation"
	var generationPayload map[string]any
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case endpoint:
			if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer bailian-key" {
				t.Fatalf("generation request method=%s authorization=%q", request.Method, request.Header.Get("Authorization"))
			}
			if err := json.NewDecoder(request.Body).Decode(&generationPayload); err != nil {
				t.Fatal(err)
			}
			return response(200, `{"output":{"rewrite_status":"REWRITE_SUCCESS","choices":[{"message":{"content":[{"text":"done"},{"image":"https://cdn.example.test/result.png"}]}}]}}`), nil
		case "https://cdn.example.test/result.png":
			result := response(200, string(imageBytes))
			result.ContentLength = int64(len(imageBytes))
			return result, nil
		default:
			t.Fatalf("unexpected request URL %s", request.URL)
			return nil, nil
		}
	})})
	result, err := client.Generate(context.Background(), Request{ProviderType: "aliyun_bailian", BaseURL: endpoint, APIKey: "bailian-key", Model: "qwen-image-3.0-pro", Prompt: "draw", Size: "1024x1536"})
	if err != nil || !bytes.Equal(result.Bytes, imageBytes) || result.MIMEType != "image/png" || result.RewriteStatus != "REWRITE_SUCCESS" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	parameters := generationPayload["parameters"].(map[string]any)
	if generationPayload["model"] != "qwen-image-3.0-pro" || parameters["size"] != "1024*1536" || parameters["n"] != float64(1) || parameters["prompt_extend"] != true || parameters["enable_thinking"] != true {
		t.Fatalf("Bailian payload=%+v", generationPayload)
	}
}

func TestCloudflareReferenceImagesUseResponsesInput(t *testing.T) {
	reference := tinyPNG(t)
	resultImage := base64.StdEncoding.EncodeToString(reference)
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/responses" || request.Method != http.MethodPost {
			t.Fatalf("reference request=%s %s", request.Method, request.URL.Path)
		}
		var payload struct {
			Model string `json:"model"`
			Input []struct {
				Content []map[string]any `json:"content"`
			} `json:"input"`
			Tools []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "openai/gpt-5.5" || len(payload.Input) != 1 || len(payload.Input[0].Content) != 2 || payload.Input[0].Content[0]["text"] != "keep the character" {
			t.Fatalf("Cloudflare payload=%+v", payload)
		}
		expectedImage := "data:image/png;base64," + base64.StdEncoding.EncodeToString(reference)
		if payload.Input[0].Content[1]["image_url"] != expectedImage || len(payload.Tools) != 1 || payload.Tools[0]["size"] != "1024x1536" {
			t.Fatalf("Cloudflare reference payload=%+v", payload)
		}
		return response(200, fmt.Sprintf(`{"output":[{"type":"image_generation_call","result":%q}]}`, resultImage)), nil
	})})
	result, err := client.Generate(context.Background(), Request{ProviderType: "cloudflare_ai_gateway", BaseURL: "https://fake.test/v1", APIKey: "secret", Model: "openai/gpt-5.5", Prompt: "keep the character", Size: "1024x1536", Images: []ImageInput{{MIMEType: "image/png", Data: reference}}})
	if err != nil || !bytes.Equal(result.Bytes, reference) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestBailianReferenceImagesAreFrozenDataURLContent(t *testing.T) {
	reference := tinyPNG(t)
	endpoint := "https://ws-123.cn-beijing.maas.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation"
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() == endpoint {
			var payload struct {
				Input struct {
					Messages []struct {
						Content []map[string]string `json:"content"`
					} `json:"messages"`
				} `json:"input"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			content := payload.Input.Messages[0].Content
			if len(content) != 2 || content[0]["image"] != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(reference) || content[1]["text"] != "redraw" {
				t.Fatalf("Bailian reference content=%+v", content)
			}
			return response(200, `{"output":{"choices":[{"message":{"content":[{"image":"https://cdn.example.test/result.png"}]}}]}}`), nil
		}
		value := response(200, string(reference))
		value.ContentLength = int64(len(reference))
		return value, nil
	})})
	result, err := client.Generate(context.Background(), Request{ProviderType: "aliyun_bailian", BaseURL: endpoint, APIKey: "secret", Model: "image-model", Prompt: "redraw", Images: []ImageInput{{MIMEType: "image/png", Data: reference}}})
	if err != nil || !bytes.Equal(result.Bytes, reference) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestBailianFailureCapturesProviderDiagnostics(t *testing.T) {
	endpoint := "https://ws-123.cn-beijing.maas.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation"
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		result := response(http.StatusBadRequest, `{"code":"InvalidParameter","message":"unsupported size for secret-key","request_id":"dashscope-request"}`)
		return result, nil
	})})
	_, err := client.Generate(context.Background(), Request{ProviderType: "aliyun_bailian", BaseURL: endpoint, APIKey: "secret-key", Model: "qwen-image-3.0", Prompt: "draw"})
	var imageErr *Error
	if !errors.As(err, &imageErr) {
		t.Fatalf("error = %#v", err)
	}
	diagnostic := imageErr.ProviderDiagnostic()
	if diagnostic.HTTPStatus != http.StatusBadRequest || diagnostic.ProviderCode != "InvalidParameter" || diagnostic.RequestID != "dashscope-request" {
		t.Fatalf("diagnostic = %+v", diagnostic)
	}
	if strings.Contains(diagnostic.Message, "secret-key") || !strings.Contains(diagnostic.Message, "[REDACTED]") {
		t.Fatalf("unsafe diagnostic message = %q", diagnostic.Message)
	}
}

func TestBailianThinkingFalseAndGenerationDeadline(t *testing.T) {
	for _, model := range []string{"qwen-image-3.0", "qwen-image-3.0-pro"} {
		t.Run(model, func(t *testing.T) {
			disabled := false
			imageBytes := tinyPNG(t)
			var generationDeadline time.Time
			client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > RequestTimeout {
					t.Fatal("generation request has no bounded deadline")
				}
				if r.Method == http.MethodPost {
					generationDeadline = deadline
					var body struct {
						Parameters map[string]any `json:"parameters"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body.Parameters["enable_thinking"] != false || body.Parameters["prompt_extend"] != true {
						t.Fatalf("parameters=%+v", body.Parameters)
					}
					return response(200, `{"output":{"choices":[{"message":{"content":[{"image":"https://cdn.example.test/result.png"}]}}]}}`), nil
				}
				if !deadline.Equal(generationDeadline) {
					t.Fatal("result download reset the generation deadline")
				}
				return response(200, string(imageBytes)), nil
			})})
			_, err := client.Generate(context.Background(), Request{ProviderType: "aliyun_bailian", BaseURL: "https://fake.test/image", Model: model, Prompt: "draw", EnableThinking: &disabled})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBailianPromptExtensionThinkingDependency(t *testing.T) {
	on, off := true, false
	for _, model := range []string{"qwen-image-3.0", "qwen-image-3.0-pro"} {
		for _, tc := range []struct {
			name                     string
			extend, thinking         *bool
			wantExtend, wantThinking bool
		}{
			{"defaults", nil, nil, true, true},
			{"rewrite_only", &on, &off, true, false},
			{"rewrite_off_default_thinking", &off, nil, false, false},
			{"rewrite_off_explicit_thinking", &off, &on, false, false},
		} {
			t.Run(model+"/"+tc.name, func(t *testing.T) {
				imageBytes := tinyPNG(t)
				client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if r.Method != http.MethodPost {
						return response(200, string(imageBytes)), nil
					}
					var body struct {
						Parameters map[string]any `json:"parameters"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body.Parameters["prompt_extend"] != tc.wantExtend || body.Parameters["enable_thinking"] != tc.wantThinking {
						t.Fatalf("parameters=%+v", body.Parameters)
					}
					return response(200, `{"output":{"choices":[{"message":{"content":[{"image":"https://cdn.example.test/result.png"}]}}]}}`), nil
				})})
				_, err := client.Generate(context.Background(), Request{ProviderType: "aliyun_bailian", BaseURL: "https://fake.test/image", Model: model, Prompt: "draw", EnablePromptExtend: tc.extend, EnableThinking: tc.thinking})
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
