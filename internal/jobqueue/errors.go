package jobqueue

import "fmt"

const (
	CodeProjectRuntimeUnavailable = "project_runtime_unavailable"
	CodeTaskNotFound              = "task_not_found"
	CodeTaskConflict              = "task_conflict"
	CodeTaskStateConflict         = "task_state_conflict"
	CodeTaskPersistenceFailed     = "task_persistence_failed"
	CodeInvalidTask               = "invalid_task"
)

type Error struct {
	Code    string
	Message string
	Details string
	Cause   error
}

func (err *Error) Error() string {
	if err.Cause != nil {
		return fmt.Sprintf("%s: %v", err.Message, err.Cause)
	}
	return err.Message
}

func (err *Error) Unwrap() error { return err.Cause }

func taskError(code, message, details string, cause error) error {
	return &Error{Code: code, Message: message, Details: details, Cause: cause}
}
