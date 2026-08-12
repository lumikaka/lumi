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
	CodeContextTooLarge  = "agent_context_too_large"
	CodeResultTooLarge   = "agent_tool_result_too_large"
	CodeMaxSteps         = "agent_max_steps_exceeded"
	CodeCancelled        = "agent_cancelled"
	CodeInterrupted      = "agent_interrupted"
	CodeProvider         = "agent_provider_failed"
	CodeWorkflowNotReady = "workflow_not_ready"

	CodeImageReferenceLimit       = "chat_image_reference_limit_exceeded"
	CodeImageReferenceUnsupported = "chat_image_reference_scene_unsupported"
	CodeImageReferenceInvalidUUID = "chat_image_reference_invalid_uuid"
	CodeImageReferenceNotFound    = "chat_image_reference_not_found"
	CodeImageReferenceProject     = "chat_image_reference_project_mismatch"
	CodeImageReferenceIncomplete  = "chat_image_reference_incomplete"
	CodeImageReferenceInvalidMIME = "chat_image_reference_invalid_mime"
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
	ErrJobNotReady  = errors.New("agent job is not ready")
	ErrWaitingInput = errors.New("agent waiting for user input")
)
