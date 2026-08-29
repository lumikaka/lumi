package agent

import (
	"errors"
	"fmt"
)

const (
	CodeValidation       = "agent_validation_failed"
	CodeNotFound         = "agent_not_found"
	CodeBusy             = "agent_thread_busy"
	CodeStateConflict    = "agent_state_conflict"
	CodeToolNotAllowed   = "agent_tool_not_allowed"
	CodeToolValidation   = "agent_tool_validation_failed"
	CodeToolConfirmation = "agent_tool_confirmation_required"
	CodeContextTooLarge  = "agent_context_too_large"
	CodeResultTooLarge   = "agent_tool_result_too_large"
	CodeMaxSteps         = "agent_max_steps_exceeded"
	CodeTurnBudget       = "agent_turn_budget_exceeded"
	CodeCancelled        = "agent_cancelled"
	CodeInterrupted      = "agent_interrupted"
	CodeProvider         = "agent_provider_failed"
	CodeWorkflowNotReady = "workflow_not_ready"

	CodeBootstrapProductionRequiresYolo = "bootstrap_production_requires_yolo"
	CodeBootstrapYoloNotAuthorized      = "bootstrap_yolo_not_authorized"

	CodeReferenceLimit       = "chat_reference_limit_exceeded"
	CodeReferenceInvalidType = "chat_reference_invalid_type"
	CodeReferenceInvalidUUID = "chat_reference_invalid_uuid"
	CodeReferenceDuplicate   = "chat_reference_duplicate"
	CodeReferenceNotFound    = "chat_reference_not_found"
	CodeReferenceProject     = "chat_reference_project_mismatch"
	CodeReferenceSnapshot    = "chat_reference_snapshot_too_large"
)

type Error struct {
	Code       string
	Message    string
	Details    string
	Retryable  bool
	HTTPStatus int
	Cause      error
	violation  *toolValidationViolation
}

// toolValidationViolation is internal repair context for model-authored tool
// arguments. It intentionally describes only the failed schema rule and never
// carries the rejected value.
type toolValidationViolation struct {
	Path          string   `json:"path"`
	Rule          string   `json:"rule"`
	ExpectedType  string   `json:"expected_type,omitempty"`
	AllowedValues []string `json:"allowed_values,omitempty"`
}

func (err *Error) Error() string {
	if err.Cause != nil {
		return fmt.Sprintf("%s: %v", err.Code, err.Cause)
	}
	return err.Code
}

func (err *Error) Unwrap() error { return err.Cause }

func domainError(code, message, details string, cause error) error {
	return &Error{Code: code, Message: message, Details: details, Cause: cause}
}

func toolValidationError(message, details string, violation toolValidationViolation) error {
	copy := violation
	copy.AllowedValues = append([]string(nil), violation.AllowedValues...)
	return &Error{Code: CodeToolValidation, Message: message, Details: details, violation: &copy}
}

func toolValidationViolationFromError(err error) (toolValidationViolation, bool) {
	var agentErr *Error
	if !errors.As(err, &agentErr) || agentErr.violation == nil {
		return toolValidationViolation{}, false
	}
	result := *agentErr.violation
	result.AllowedValues = append([]string(nil), agentErr.violation.AllowedValues...)
	return result, true
}

var (
	ErrJobNotReady     = errors.New("agent job is not ready")
	ErrWaitingInput    = errors.New("agent waiting for user input")
	ErrWaitingWorkflow = errors.New("agent waiting for workflow")
)
