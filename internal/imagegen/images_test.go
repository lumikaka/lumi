package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"lumi/internal/provider"
	"lumi/internal/sitesettings"
)

func TestImagesAPIGenerationAndMultipartReferences(t *testing.T) {
	picture := tinyPNG(t)
	for _, model := range sitesettings.RelayImageModels(sitesettings.ImageAPIImages) {
		for _, count := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("%s/%d", model, count), func(t *testing.T) {
				client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					wantPath := "/api/v1/images/generations"
					if count > 0 {
						wantPath = "/api/v1/images/edits"
					}
					if req.URL.Path != wantPath || req.Header.Get("Authorization") != "Bearer image-secret" || req.Header.Get("cf-aig-max-attempts") != "" {
						t.Fatal("incorrect endpoint or credentials")
					}
					fields := map[string]string{}
					if count == 0 {
						var payload map[string]any
						if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
							t.Fatal(err)
						}
						if payload["n"] != float64(1) || payload["tools"] != nil || payload["input"] != nil || payload["response_format"] != nil {
							t.Fatalf("payload=%v", payload)
						}
						for _, key := range []string{"model", "prompt", "size", "quality", "output_format"} {
							fields[key], _ = payload[key].(string)
						}
					} else {
						if err := req.ParseMultipartForm(1 << 20); err != nil {
							t.Fatal(err)
						}
						defer req.MultipartForm.RemoveAll()
						refs := req.MultipartForm.File["image[]"]
						if len(refs) != count {
							t.Fatalf("references=%d", len(refs))
						}
						for i, ref := range refs {
							f, err := ref.Open()
							if err != nil {
								t.Fatal(err)
							}
							data, err := io.ReadAll(f)
							f.Close()
							if err != nil || !bytes.Equal(data, picture) || ref.Header.Get("Content-Type") != "image/png" || ref.Filename != fmt.Sprintf("reference-%d.png", i+1) {
								t.Fatal("reference bytes/order/type changed")
							}
						}
						for key, values := range req.MultipartForm.Value {
							fields[key] = values[0]
						}
						if fields["n"] != "1" || fields["response_format"] != "" {
							t.Fatalf("fields=%v", fields)
						}
					}
					if fields["model"] != model || fields["prompt"] != "draw" || fields["size"] != "1536x1152" || fields["quality"] != "low" || fields["output_format"] != "png" {
						t.Fatalf("fields=%v", fields)
					}
					return response(200, fmt.Sprintf(`{"data":[{"b64_json":%q,"revised_prompt":"revised"}],"usage":{"input_tokens":12,"output_tokens":20,"input_tokens_details":{"text_tokens":4,"image_tokens":8}}}`, base64.StdEncoding.EncodeToString(picture))), nil
				})})
				refs := make([]ImageInput, count)
				for i := range refs {
					refs[i] = ImageInput{MIMEType: "image/png", Data: picture}
				}
				result, err := client.Generate(context.Background(), Request{ProviderType: provider.TypeOpenAIRelay, ImageAPIMode: sitesettings.ImageAPIImages, BaseURL: "https://image.example/api/v1/", APIKey: "image-secret", Model: model, Prompt: "draw", Size: "1536x1152", Quality: "low", Images: refs})
				if err != nil || !bytes.Equal(result.Bytes, picture) || result.RevisedPrompt != "revised" {
					t.Fatalf("result=%+v err=%v", result, err)
				}
				if result.Usage.Disjoint || result.Usage.ToolInputTokens != nil || *result.Usage.InputTokens != 12 || *result.Usage.OutputImages != 1 || result.Usage.ImageModel != model {
					t.Fatalf("incorrect billing: %+v", result.Usage)
				}
			})
		}
	}
}

func TestImagesAPIRejectsInvalidResponsesWithoutFallback(t *testing.T) {
	for _, body := range []string{`{}`, `{"data":[]}`, `{"data":[{"b64_json":"invalid"}]}`, `{"data":[{"b64_json":"aGVsbG8="}]}`, `{"data":[{},{}]}`, `{"data":[{"url":"http://127.0.0.1/private"}]}`, `{"output":[{"type":"image_generation_call","result":"aGVsbG8="}]}`} {
		calls := 0
		client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) { calls++; return response(200, body), nil })})
		_, err := client.Generate(context.Background(), Request{ProviderType: provider.TypeOpenAIRelay, ImageAPIMode: "images", BaseURL: "https://example.com/v1", Model: "gpt-image-2", Prompt: "draw"})
		if err == nil || calls != 1 {
			t.Fatalf("body=%s err=%v calls=%d", body, err, calls)
		}
	}
}

func TestImagesAPIURLDownloadDoesNotForwardSecret(t *testing.T) {
	picture := tinyPNG(t)
	calls := 0
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(200, `{"data":[{"url":"https://cdn.example/image.png"}]}`), nil
		}
		if req.URL.String() != "https://cdn.example/image.png" || req.Header.Get("Authorization") != "" {
			t.Fatal("unsafe download")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(picture)), Header: http.Header{}}, nil
	})})
	got, err := client.Generate(context.Background(), Request{ProviderType: provider.TypeOpenAIRelay, ImageAPIMode: "images", BaseURL: "https://example.com/v1", APIKey: "secret", Model: "gpt-image-2", Prompt: "draw"})
	if err != nil || !bytes.Equal(got.Bytes, picture) || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestImagesAPIRejectsUnknownModeAndSanitizesUpstreamError(t *testing.T) {
	calls := 0
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return response(401, `{"error":{"message":"denied private-key"}}`), nil
	})})
	input := Request{ProviderType: provider.TypeOpenAIRelay, ImageAPIMode: "unknown", BaseURL: "https://example.com/v1", APIKey: "private-key", Model: "gpt-image-2", Prompt: "draw"}
	if _, err := client.Generate(context.Background(), input); err == nil || calls != 0 {
		t.Fatal("unknown mode accepted")
	}
	input.ImageAPIMode = "images"
	_, err := client.Generate(context.Background(), input)
	if err == nil || calls != 1 || strings.Contains(fmt.Sprintf("%+v", err), "private-key") {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
