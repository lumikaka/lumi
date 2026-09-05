package imagegen

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestBailianDownloadFailureRetainsBillableOutput(t *testing.T) {
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			return response(200, `{"output":{"choices":[{"message":{"content":[{"image":"https://example.com/image.png"}]}}]},"usage":{"input_image_count":2,"output_image_count":1,"output_image_type":"qima_output_2k","output_width":2048,"output_height":2048}}`), nil
		}
		return nil, errors.New("download failed")
	})})
	result, err := client.Generate(context.Background(), Request{ProviderType: "aliyun_bailian", BaseURL: "https://fake.test/image", Model: "qwen-image-3.0-pro", Prompt: "image"})
	if err == nil || result.Usage.OutputImages == nil || *result.Usage.OutputImages != 1 || result.Usage.Resolution != "2k" || *result.Usage.InputImages != 2 {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestResponsesUsageKeepsParentAndToolSeparate(t *testing.T) {
	raw := `{"usage":{"input_tokens":100,"output_tokens":10,"input_tokens_details":{"cached_tokens":20}},"tools":[{"type":"image_generation","model":"gpt-image-test"}],"output":[{"type":"image_generation_call","usage":{"input_tokens":1200,"output_tokens":1000,"input_tokens_details":{"text_tokens":200,"image_tokens":1000,"cached_tokens":0}}}]}`
	var e responsesEnvelope
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatal(err)
	}
	u := e.billingUsage(Request{Model: "openai/gpt-test"})
	if !u.Disjoint || *u.InputTokens != 100 || *u.ToolInputTokens != 200 || *u.ImageInputTokens != 1000 || u.ImageModel != "gpt-image-test" {
		t.Fatalf("%+v", u)
	}
	e.Output[0].Usage = nil
	u = e.billingUsage(Request{Model: "openai/gpt-test"})
	if u.Disjoint || u.ImageOutputTokens != nil || *u.InputTokens != 100 {
		t.Fatalf("missing tool usage guessed: %+v", u)
	}
}

func TestBailianMissingImageURLRetainsReportedUsage(t *testing.T) {
	client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return response(200, `{"output":{"choices":[]},"usage":{"input_image_count":0,"output_image_count":1,"output_image_type":"qima_output_1k"}}`), nil
	})})
	result, err := client.Generate(context.Background(), Request{ProviderType: "aliyun_bailian", BaseURL: "https://fake.test/image", Model: "qwen-image-3.0-pro", Prompt: "image"})
	if err == nil || result.Usage.OutputImages == nil || *result.Usage.OutputImages != 1 || result.Usage.Resolution != "1k" {
		t.Fatalf("%+v %v", result, err)
	}
}
