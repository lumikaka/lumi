package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"lumi/internal/llm"
	"lumi/internal/project"
)

const (
	maxConsecutiveInvalidProviderResponses = 2
	invalidProviderResponseRetryPrompt     = "The previous Provider response was discarded in full because its structure was incomplete or invalid. No tool call from it was persisted or executed. Return one short, complete response; if a tool is needed, prefer exactly one tool call and emit a complete JSON arguments object."
)

func requestUserInputMixedProviderError(response llm.ChatResponse) error {
	if len(response.Message.ToolCalls) <= 1 {
		return nil
	}
	for toolIndex, call := range response.Message.ToolCalls {
		if call.Name != "request_user_input" {
			continue
		}
		choiceIndex := 0
		preview, _ := json.Marshal(response)
		partial := response
		return &llm.Error{
			Code:        llm.CodeInvalidContent,
			SafeMessage: "request_user_input 必须是 Provider 响应中的唯一工具调用。",
			ResponseDiagnostic: &llm.ProviderResponseDiagnostic{
				Reason: llm.ProviderResponseRequestUserInputMixed, ChoiceIndex: &choiceIndex, ToolIndex: &toolIndex,
				FinishReason: response.FinishReason, Usage: response.Usage, BodyLength: int64(len(preview)), Preview: string(preview),
			},
			PartialResponse: &partial,
		}
	}
	return nil
}

func invalidProviderResponse(err error) *llm.ProviderResponseDiagnostic {
	var modelErr *llm.Error
	if !errors.As(err, &modelErr) {
		return nil
	}
	return modelErr.InvalidProviderResponse()
}

func invalidProviderResponseExhaustedError() error {
	return &llm.Error{
		Code:        llm.CodeInvalidContent,
		SafeMessage: "Provider 连续返回了无法安全解析的内容，本轮已停止且未执行其中的工具调用。",
		Retryable:   false,
	}
}

func providerResponseRetryMessages(messages []llm.ChatMessage) []llm.ChatMessage {
	result := make([]llm.ChatMessage, 0, len(messages)+1)
	result = append(result, messages...)
	result = append(result, llm.ChatMessage{Role: "system", Content: invalidProviderResponseRetryPrompt})
	return result
}

// consecutiveInvalidProviderResponses restores the wire-retry budget from
// durable LLM logs. An interrupted request carrying the retry marker consumes
// the retry slot too: once llmlog.Begin has committed, a process crash cannot
// safely prove that the Provider did not receive the physical request.
func (service *Service) consecutiveInvalidProviderResponses(ctx context.Context, store *project.Store, runID int64) (int, error) {
	var rows []struct {
		RequestPayload string  `gorm:"column:request_payload"`
		Response       *string `gorm:"column:response"`
		Status         string  `gorm:"column:status"`
		ErrorCode      string  `gorm:"column:error_code"`
	}
	if err := store.DB().WithContext(ctx).Table("llm_logs").Select("request_payload,response,status,error_code").Where("chat_run_id=?", runID).Order("id DESC").Limit(maxConsecutiveInvalidProviderResponses + 1).Scan(&rows).Error; err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		if row.Response == nil || strings.TrimSpace(*row.Response) == "" {
			// A pending physical request has crossed the durable Begin boundary,
			// so after a crash we cannot prove it was never sent. Conservatively
			// consume one wire slot. Explicit network/timeout failures, however,
			// are completed non-structural attempts and must not be mistaken for
			// malformed Provider content merely because response remains NULL.
			if row.Status == "pending" || row.ErrorCode == "provider_call_interrupted" {
				count++
				continue
			}
			break
		}
		var discriminator struct {
			SnapshotType  string `json:"snapshot_type"`
			SchemaVersion int    `json:"schema_version"`
		}
		if json.Unmarshal([]byte(*row.Response), &discriminator) != nil || discriminator.SnapshotType != "provider_response_diagnostic" || discriminator.SchemaVersion != 1 {
			break
		}
		count++
	}
	if count > maxConsecutiveInvalidProviderResponses {
		count = maxConsecutiveInvalidProviderResponses
	}
	return count, nil
}
