package llmlog

import (
	"lumi/internal/llm"
	"lumi/internal/provider"
	"strings"
	"testing"
)

func TestRelaySnapshotsUseResponsesAndRedactSecrets(t *testing.T) {
	request, err := EncodeChatRequest(llm.ChatRequest{ProviderType: provider.TypeOpenAIRelay, Model: "gpt-6", APIKey: "text-secret", Messages: []llm.ChatMessage{{Role: "user", Content: "text-secret"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(request), `"api_format":"responses"`) || strings.Contains(string(request), "text-secret") {
		t.Fatalf("request=%s", request)
	}
}
