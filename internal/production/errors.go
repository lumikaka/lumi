package production

import "fmt"

const (
	CodeNotFound          = "production_resource_not_found"
	CodeValidation        = "production_validation_failed"
	CodeConflict          = "production_conflict"
	CodeStateConflict     = "production_state_conflict"
	CodeSnapshotInvalid   = "production_snapshot_invalid"
	CodeSnapshotBusy      = "production_snapshot_restore_blocked"
	CodeExportEmpty       = "comic_export_empty"
	CodeExportIncomplete  = "comic_export_incomplete"
	CodeExportChanged     = "comic_export_readiness_changed"
	CodeExportExpired     = "comic_export_expired"
	CodeExportUnavailable = "comic_export_unavailable"
	CodeDeleteBlocked     = "premise_asset_delete_blocked"
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

// GeneratedSectionsConflict carries the public, revision-based confirmation
// boundary needed when a generated storyboard would replace active sections.
// The wrapped domain error preserves the existing production_conflict API
// classification for callers that do not implement the confirmation flow.
type GeneratedSectionsConflict struct {
	ExistingCount      int
	GeneratedCount     int
	ComicStateRevision int64
	cause              *Error
}

func (err *GeneratedSectionsConflict) Error() string {
	if err == nil || err.cause == nil {
		return "generated comic sections conflict"
	}
	return err.cause.Error()
}
func (err *GeneratedSectionsConflict) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func generatedSectionsConflict(message, details string, existingCount, generatedCount int, revision int64) error {
	return &GeneratedSectionsConflict{
		ExistingCount: existingCount, GeneratedCount: generatedCount, ComicStateRevision: revision,
		cause: &Error{Code: CodeConflict, Message: message, Details: details},
	}
}
