package llm

import (
	"context"
	"strings"
)

func (client *OpenAICompatibleClient) checkCloudflareConnection(ctx context.Context, input Request) error {
	if strings.TrimSpace(input.Model) == "" {
		return &Error{Code: CodeInvalidContent, SafeMessage: "模型不能为空。"}
	}
	payload := responsesPayload(input.Model, []any{map[string]any{"role": "user", "content": "Reply with OK."}}, nil, 512, false)
	_, err := client.sendResponses(ctx, input.BaseURL, input.APIKey, payload, nil, true)
	return err
}
