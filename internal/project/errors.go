package project

import "fmt"

const (
	CodeInvalidPath                        = "invalid_project_path"
	CodeDefaultProjectParentUnavailable    = "default_project_parent_unavailable"
	CodeInvalidUUID                        = "invalid_uuid"
	CodeProjectNotFound                    = "project_not_found"
	CodeProjectCoverNotFound               = "project_cover_not_found"
	CodeProjectNotOpen                     = "project_not_open"
	CodePermissionDenied                   = "project_permission_denied"
	CodeInvalidProject                     = "invalid_project"
	CodeIdentityMismatch                   = "project_identity_mismatch"
	CodeFormatTooNew                       = "project_format_too_new"
	CodeMigrationFailed                    = "project_migration_failed"
	CodeLocked                             = "project_locked"
	CodeProjectDirectoryNameExhausted      = "project_directory_name_exhausted"
	CodeInvalidPictureBook                 = "invalid_picture_book_profile"
	CodePictureBookImmutable               = "picture_book_profile_immutable"
	CodeInvalidOverallStyle                = "invalid_overall_style"
	CodeProjectSetupIncomplete             = "project_setup_incomplete"
	CodeProjectSetupConflict               = "project_setup_revision_conflict"
	CodeProjectSetupInvalid                = "invalid_project_setup"
	CodeProjectSetupReferenceSystemManaged = "project_setup_reference_system_managed"
)

type Error struct {
	Code    string
	Message string
	Details string
	Err     error
}

func (err *Error) Error() string {
	if err.Err != nil {
		return fmt.Sprintf("%s: %v", err.Message, err.Err)
	}
	return err.Message
}

func (err *Error) Unwrap() error { return err.Err }

func projectError(code, message, details string, err error) *Error {
	return &Error{Code: code, Message: message, Details: details, Err: err}
}
