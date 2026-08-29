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

var (
	ErrJobNotReady     = errors.New("agent job is not ready")
	ErrWaitingInput    = errors.New("agent waiting for user input")
	ErrWaitingWorkflow = errors.New("agent waiting for workflow")
)
