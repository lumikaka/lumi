package llmlog

import (
	"encoding/json"
	"strings"
	"testing"

	"lumi/internal/imagegen"
	"lumi/internal/llm"
)

func TestSafeSnapshotsKeepFullTextAndExcludeCredentialsAndImageBytes(t *testing.T) {
	longPrompt := strings.Repeat("月光", 800)
	secret := "project-must-never-contain-this-api-key"
	request, err := EncodeTextRequest(llm.Request{
		BaseURL: "https://provider.example/v1", APIKey: secret, Model: "vision-model",
		SystemPrompt: "system Bearer inline-bearer-token", Prompt: longPrompt + " api_key: inline-key " + secret,
		Images: []llm.ImageInput{{MIMEType: "image/png", Data: []byte("raw-image-bytes"), Detail: "high"}}, MaxTokens: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(request)
	for _, forbidden := range []string{secret, "inline-bearer-token", "inline-key", "raw-image-bytes", "provider.example"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("request snapshot leaked %q: %s", forbidden, text)
		}
	}
	var decoded struct {
		Prompt string `json:"prompt"`
		Images []struct {
			MIMEType string `json:"mime_type"`
			ByteSize int    `json:"byte_size"`
			Detail   string `json:"detail"`
		} `json:"images"`
	}
	if err := json.Unmarshal(request, &decoded); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(decoded.Prompt, longPrompt) || len([]rune(decoded.Prompt)) < len([]rune(longPrompt)) || len(decoded.Images) != 1 || decoded.Images[0].MIMEType != "image/png" || decoded.Images[0].ByteSize != len("raw-image-bytes") || decoded.Images[0].Detail != "high" {
		t.Fatalf("request snapshot lost full text or image metadata: %+v", decoded)
	}
}

func TestChatAndImageResponsesCaptureSafeStructuredResults(t *testing.T) {
	secret := "configured-secret"
	chatRequest, err := EncodeChatRequest(llm.ChatRequest{
		BaseURL: "https://provider.example/v1", APIKey: secret, Model: "agent-model", MaxTokens: 2048,
		Messages: []llm.ChatMessage{{Role: "user", Content: "hello /Users/private/lumi/story.md"}},
		Tools:    []llm.ToolDefinition{{Name: "lookup", Description: "authorization=tool-token", Parameters: map[string]any{"type": "object", "description": "uses " + secret}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	chatResponse, err := EncodeChatResponse(llm.ChatResponse{
		Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "lookup", Arguments: `{"api_key":"unconfigured-inline-key","token":"Bearer response-token"}`}}},
		Usage:   llm.Usage{InputTokens: 11, OutputTokens: 7}, FinishReason: "tool_calls",
	}, secret)
	if err != nil {
		t.Fatal(err)
	}
	imageRequest, err := EncodeImageRequest(imagegen.Request{
		BaseURL: "https://image.example/v1", APIKey: secret, Model: "image-model", Prompt: "draw", Size: "1024x1024", Quality: "high",
		Images: []imagegen.ImageInput{{MIMEType: "image/jpeg", Data: []byte("reference-image-bytes")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	imageResponse, err := EncodeImageResponse(imagegen.Response{Bytes: []byte("generated-image-bytes"), MIMEType: "image/png", RevisedPrompt: "revised"}, secret)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join([]string{string(chatRequest), string(chatResponse), string(imageRequest), string(imageResponse)}, "\n")
	for _, forbidden := range []string{secret, "tool-token", "response-token", "unconfigured-inline-key", "reference-image-bytes", "generated-image-bytes", "provider.example", "image.example", "/Users/private/lumi/story.md"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("safe snapshots leaked %q: %s", forbidden, joined)
		}
	}
	for _, expected := range []string{`"tool_calls"`, `"input_tokens":11`, `"output_tokens":7`, `"finish_reason":"tool_calls"`, `"mime_type":"image/jpeg"`, `"byte_size":21`, `"mime_type":"image/png"`, `"byte_size":21`, `"revised_prompt":"revised"`} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("safe snapshots missing %q: %s", expected, joined)
		}
	}
}
