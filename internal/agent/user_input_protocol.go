package agent

import (
	"encoding/json"
	"strings"
)

const (
	userInputSchemaLegacyChoice   = "legacy_choice_v1"
	userInputSchemaCodexQuestions = "codex_questions_v1"
)

func usesCodexUserInputProtocol(tc toolContext) bool {
	return normalizedToolMode(tc.ToolMode) == ToolModeProjectAPI && tc.ToolProtocol != ToolProtocolProjectV2 && tc.ToolProtocol != ToolProtocolProjectV3
}

func validateCodexUserInputResponse(row userInputRow, input UserInputResponse) (map[string]any, map[string]any, error) {
	var request struct {
		Questions []UserInputQuestion `json:"questions"`
	}
	if err := json.Unmarshal([]byte(row.RequestJSON), &request); err != nil || len(request.Questions) < 1 || len(request.Questions) > 3 {
		return nil, nil, domainError(CodeStateConflict, "用户输入问题损坏", "无法安全提交回答。", err)
	}
	if len(input.SelectedOptionUUIDs) > 0 || strings.TrimSpace(input.OtherText) != "" || len(input.Answers) != len(request.Questions) {
		return nil, nil, domainError(CodeValidation, "回答结构无效", "answers 必须为每个 question id 提交且只能提交一个回答。", nil)
	}

	persistedAnswers := make(map[string]any, len(request.Questions))
	modelAnswers := make(map[string]any, len(request.Questions))
	knownQuestions := make(map[string]struct{}, len(request.Questions))
	for _, question := range request.Questions {
		knownQuestions[question.ID] = struct{}{}
		answer, exists := input.Answers[question.ID]
		if !exists {
			return nil, nil, domainError(CodeValidation, "回答不完整", "answers 必须包含请求中的每个 question id。", nil)
		}
		selectedUUID := strings.TrimSpace(answer.SelectedOptionUUID)
		other := strings.TrimSpace(answer.OtherText)
		if (selectedUUID == "") == (other == "") || len([]rune(other)) > 4000 {
			return nil, nil, domainError(CodeValidation, "单题回答无效", "每题必须且只能选择一个选项或填写 Other，Other 最多 4000 字符。", nil)
		}
		modelAnswer := other
		if selectedUUID != "" {
			if !isUUIDv7(selectedUUID) {
				return nil, nil, domainError(CodeValidation, "选项 UUID 无效", "selected_option_uuid 必须是公开 UUIDv7。", nil)
			}
			matched := ""
			for _, option := range question.Options {
				if option.UUID == selectedUUID {
					matched = option.Label
					break
				}
			}
			if matched == "" {
				return nil, nil, domainError(CodeValidation, "选项不存在", "只能提交对应问题中列出的选项。", nil)
			}
			modelAnswer = matched
		}
		persistedAnswers[question.ID] = map[string]any{"selected_option_uuid": selectedUUID, "other_text": other}
		modelAnswers[question.ID] = map[string]any{"answers": []string{modelAnswer}}
	}
	for questionID := range input.Answers {
		if _, exists := knownQuestions[questionID]; !exists {
			return nil, nil, domainError(CodeValidation, "回答包含未知问题", "answers 只能使用请求中列出的 question id。", nil)
		}
	}
	return map[string]any{"answers": persistedAnswers}, map[string]any{"answers": modelAnswers}, nil
}
