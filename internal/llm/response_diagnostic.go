package llm

// ProviderResponseFailureReason is a stable, machine-readable classification
// for structurally unusable 2xx chat-completion responses. These values are
// persisted in LLM log response snapshots and must remain backwards compatible.
type ProviderResponseFailureReason string

const (
	ProviderResponseEmptyBody              ProviderResponseFailureReason = "empty_body"
	ProviderResponseBodyReadError          ProviderResponseFailureReason = "body_read_error"
	ProviderResponseBodyTooLarge           ProviderResponseFailureReason = "body_too_large"
	ProviderResponseMalformedJSON          ProviderResponseFailureReason = "malformed_json"
	ProviderResponseTrailingJSON           ProviderResponseFailureReason = "trailing_json"
	ProviderResponseEmptyChoices           ProviderResponseFailureReason = "empty_choices"
	ProviderResponseEmptyMessage           ProviderResponseFailureReason = "empty_message"
	ProviderResponseMissingToolCallID      ProviderResponseFailureReason = "missing_tool_call_id"
	ProviderResponseDuplicateToolCallID    ProviderResponseFailureReason = "duplicate_tool_call_id"
	ProviderResponseMissingToolName        ProviderResponseFailureReason = "missing_tool_name"
	ProviderResponseToolArgumentsWrongType ProviderResponseFailureReason = "tool_arguments_wrong_type"
	ProviderResponseToolArgumentsTooLarge  ProviderResponseFailureReason = "tool_arguments_too_large"
	ProviderResponseFinishReasonLength     ProviderResponseFailureReason = "finish_reason_length"
	ProviderResponseNegativeUsage          ProviderResponseFailureReason = "negative_usage"
	ProviderResponseRequestUserInputMixed  ProviderResponseFailureReason = "request_user_input_not_exclusive"

	// ProviderResponseInvalidUsage is a compatibility alias for callers that
	// use the broader name for the currently supported invalid-usage class.
	ProviderResponseInvalidUsage = ProviderResponseNegativeUsage
)

// ProviderResponseDiagnostic contains only a bounded, sanitized view of an
// invalid Provider response. Preview never contains the unredacted wire body.
// ChoiceIndex and ToolIndex are pointers because index zero is meaningful.
type ProviderResponseDiagnostic struct {
	Reason            ProviderResponseFailureReason
	ChoiceIndex       *int
	ToolIndex         *int
	HTTPStatus        int
	ProviderRequestID string
	ContentType       string
	FinishReason      string
	Usage             Usage
	BodyLength        int64
	BodyTruncated     bool
	Preview           string
}
