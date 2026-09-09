package llm

import (
	"context"

	"lumi/internal/provider"
)

type ConnectionChecker interface {
	CheckConnection(context.Context, string, Request) error
}

// CheckConnection selects the provider's verification protocol explicitly.
// Keep probe options separate from normal generation and from other providers.
func (client *OpenAICompatibleClient) CheckConnection(ctx context.Context, providerType string, input Request) error {
	switch providerType {
	case provider.TypeCloudflareAIGateway:
		return client.checkCloudflareConnection(ctx, input)
	case provider.TypeAliyunBailian:
		return client.checkBailianConnection(ctx, input)
	default:
		return &Error{Code: CodeInvalidContent, SafeMessage: "不支持该 Provider 的连接检查。"}
	}
}

func (client *OpenAICompatibleClient) checkBailianConnection(ctx context.Context, input Request) error {
	_, err := client.Generate(ctx, Request{
		BaseURL: input.BaseURL, APIKey: input.APIKey, Model: input.Model,
		Prompt: "Reply with OK.", MaxTokens: 1,
	}, nil)
	return err
}
