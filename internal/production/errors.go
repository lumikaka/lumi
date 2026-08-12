package production

import "fmt"

const (
	CodeNotFound         = "production_resource_not_found"
	CodeValidation       = "production_validation_failed"
	CodeConflict         = "production_conflict"
	CodeStateConflict    = "production_state_conflict"
	CodeSnapshotInvalid  = "production_snapshot_invalid"
	CodeSnapshotBusy     = "production_snapshot_restore_blocked"
	CodeExportEmpty      = "comic_export_empty"
	CodeExportIncomplete = "comic_export_incomplete"
	CodeExportChanged    = "comic_export_readiness_changed"
	CodeDeleteBlocked    = "premise_asset_delete_blocked"
)

type Error struct {
	Code, Message, Details string
	Cause                  error
}

func (err *Error) Error() string {
	if err.Cause != nil {
		return fmt.Sprintf("%s: %v", err.Message, err.Cause)
	}
	return err.Message
}
func (err *Error) Unwrap() error { return err.Cause }
func domainError(code, message, details string, cause error) error {
	return &Error{Code: code, Message: message, Details: details, Cause: cause}
}
