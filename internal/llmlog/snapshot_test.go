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
		Messages: []llm.ChatMessage{{Role: "user", Content: "hello /Users/private/lumi/story.md https://public.example/docs"}},
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
	for _, forbidden := range []string{secret, "tool-token", "response-token", "unconfigured-inline-key", "reference-image-bytes", "generated-image-bytes", "provider.example", "image.example"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("safe snapshots leaked %q: %s", forbidden, joined)
		}
	}
	for _, expected := range []string{`"tool_calls"`, `"input_tokens":11`, `"output_tokens":7`, `"finish_reason":"tool_calls"`, `"mime_type":"image/jpeg"`, `"byte_size":21`, `"mime_type":"image/png"`, `"byte_size":21`, `"revised_prompt":"revised"`} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("safe snapshots missing %q: %s", expected, joined)
		}
	}
	for _, expected := range []string{"/Users/private/lumi/story.md", "https://public.example/docs"} {
		if !strings.Contains(string(chatRequest), expected) {
			t.Fatalf("normal chat snapshot over-redacted %q: %s", expected, chatRequest)
		}
	}
}

func TestChatSnapshotHidesOnlyMarkedSyntheticConfirmationCallIDs(t *testing.T) {
	syntheticID := "call_3a48bfcef1afc30785f1efb5"
	realProviderID := "call_0123456789abcdef01234567"
	encoded, err := EncodeChatRequest(llm.ChatRequest{
		APIKey: "secret", Model: "agent-model",
		Messages: []llm.ChatMessage{
			{Role: "assistant", ToolCalls: []llm.ToolCall{
				{ID: syntheticID, Name: "request_api", Arguments: `{}`, SyntheticID: true},
				{ID: realProviderID, Name: "lookup", Arguments: `{}`},
			}},
			{Role: "tool", ToolCallID: syntheticID, ToolCallIDSynthetic: true, Content: `{"success":true}`},
			{Role: "tool", ToolCallID: realProviderID, Content: `{"success":true}`},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := string(encoded)
	if strings.Contains(snapshot, syntheticID) {
		t.Fatalf("synthetic confirmation call ID leaked into LLM Log snapshot: %s", snapshot)
	}
	if strings.Count(snapshot, internalSyntheticCallIDSnapshotValue) != 2 {
		t.Fatalf("synthetic call/result pairing was not consistently masked: %s", snapshot)
	}
	if !strings.Contains(snapshot, realProviderID) {
		t.Fatalf("real Provider call ID was unexpectedly rewritten: %s", snapshot)
	}
}

func TestEncodeProviderResponseDiagnosticUsesVersionedSanitizedShape(t *testing.T) {
	choiceIndex, toolIndex := 0, 2
	encoded, err := EncodeProviderResponseDiagnostic(llm.ProviderResponseDiagnostic{
		Reason:            llm.ProviderResponseToolArgumentsWrongType,
		ChoiceIndex:       &choiceIndex,
		ToolIndex:         &toolIndex,
		HTTPStatus:        200,
		ProviderRequestID: "request-configured-secret",
		ContentType:       "application/json",
		FinishReason:      "tool_calls",
		Usage:             llm.Usage{InputTokens: 17, OutputTokens: 9},
		BodyLength:        2048,
		BodyTruncated:     false,
		Preview:           `{"authorization":"Bearer leaked-token","password":"password-value","url":"https://cdn.example/file?signature=signed-value","path":"/Users/private/lumi/file.txt"}`,
	}, "configured-secret")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		SnapshotType      string                            `json:"snapshot_type"`
		SchemaVersion     int                               `json:"schema_version"`
		Reason            llm.ProviderResponseFailureReason `json:"reason"`
		ChoiceIndex       *int                              `json:"choice_index"`
		ToolIndex         *int                              `json:"tool_index"`
		HTTPStatus        int                               `json:"http_status"`
		ProviderRequestID string                            `json:"provider_request_id"`
		ContentType       string                            `json:"content_type"`
		FinishReason      string                            `json:"finish_reason"`
		Usage             llm.Usage                         `json:"usage"`
		BodyLength        int64                             `json:"body_length"`
		BodyTruncated     bool                              `json:"body_truncated"`
		Preview           string                            `json:"preview"`
	}
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.SnapshotType != "provider_response_diagnostic" || snapshot.SchemaVersion != 1 || snapshot.Reason != llm.ProviderResponseToolArgumentsWrongType {
		t.Fatalf("snapshot discriminator = %+v", snapshot)
	}
	if snapshot.ChoiceIndex == nil || *snapshot.ChoiceIndex != 0 || snapshot.ToolIndex == nil || *snapshot.ToolIndex != 2 || snapshot.HTTPStatus != 200 {
		t.Fatalf("snapshot location = %+v", snapshot)
	}
	if snapshot.ContentType != "application/json" || snapshot.FinishReason != "tool_calls" || snapshot.Usage.InputTokens != 17 || snapshot.Usage.OutputTokens != 9 || snapshot.BodyLength != 2048 || snapshot.BodyTruncated {
		t.Fatalf("snapshot metadata = %+v", snapshot)
	}
	joined := string(encoded)
	for _, forbidden := range []string{"configured-secret", "leaked-token", "password-value", "signed-value", "/Users/private"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("diagnostic snapshot leaked %q: %s", forbidden, joined)
		}
	}
	for _, expected := range []string{"[REDACTED]", "[REDACTED_URL]", "[REDACTED_PATH]"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("diagnostic snapshot missing %q: %s", expected, joined)
		}
	}
}

func TestEncodeProviderResponseDiagnosticBoundsAndRedactsFinishReason(t *testing.T) {
	encoded, err := EncodeProviderResponseDiagnostic(llm.ProviderResponseDiagnostic{
		Reason:       llm.ProviderResponseFinishReasonLength,
		HTTPStatus:   200,
		FinishReason: `Bearer finish-secret https:\u002f\u002fcdn.example\u002fsigned \u002fUsers\u002fprivate\u002ffile ` + strings.Repeat("诊", 300),
	}, "finish-secret")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		FinishReason string `json:"finish_reason"`
	}
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.FinishReason) > maxProviderDiagnosticFinishReasonBytes {
		t.Fatalf("finish reason bytes=%d", len(snapshot.FinishReason))
	}
	for _, forbidden := range []string{"finish-secret", "cdn.example", "Users", "private"} {
		if strings.Contains(snapshot.FinishReason, forbidden) {
			t.Fatalf("finish reason leaked %q: %s", forbidden, snapshot.FinishReason)
		}
	}
	for _, expected := range []string{"[REDACTED]", "[REDACTED_URL]", "[REDACTED_PATH]"} {
		if !strings.Contains(snapshot.FinishReason, expected) {
			t.Fatalf("finish reason missing %q: %s", expected, snapshot.FinishReason)
		}
	}
}
