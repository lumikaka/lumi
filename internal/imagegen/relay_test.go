package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"lumi/internal/provider"
	"net/http"
	"testing"
)

func TestRelayImageGenerationUsesStandardToolAndReferenceInput(t *testing.T) {
	picture := tinyPNG(t)
	for _, model := range provider.RelayModels() {
		t.Run(model, func(t *testing.T) {
			client := NewOpenAICompatibleClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != "https://image.example/api/v1/responses" || req.Header.Get("Authorization") != "Bearer image-secret" || req.Header.Get("cf-aig-max-attempts") != "" || req.Header.Get("cf-aig-gateway-id") != "" {
					t.Fatal("incorrect relay request")
				}
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				tool := payload["tools"].([]any)[0].(map[string]any)
				if payload["model"] != model || payload["store"] != false || tool["type"] != "image_generation" || tool["model"] != nil || tool["size"] != "1024x1024" || tool["quality"] != "low" {
					t.Fatalf("payload=%v", payload)
				}
				content := payload["input"].([]any)[0].(map[string]any)["content"].([]any)
				if len(content) != 2 || content[1].(map[string]any)["image_url"] != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(picture) {
					t.Fatal("missing reference image")
				}
				return response(200, fmt.Sprintf(`{"output":[{"type":"image_generation_call","result":%q,"revised_prompt":"revised"}]}`, base64.StdEncoding.EncodeToString(picture))), nil
			})})
			got, err := client.Generate(context.Background(), Request{ProviderType: provider.TypeOpenAIRelay, BaseURL: "https://image.example/api/v1/", APIKey: "image-secret", Model: model, Prompt: "draw", Size: "1024x1024", Quality: "low", Images: []ImageInput{{MIMEType: "image/png", Data: picture}}})
			if err != nil || !bytes.Equal(got.Bytes, picture) || got.RevisedPrompt != "revised" {
				t.Fatalf("result=%+v error=%v", got, err)
			}
		})
	}
}
