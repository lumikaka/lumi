package files

import "fmt"

const (
	CodeUploadNotFound       = "asset_upload_not_found"
	CodeUploadExpired        = "asset_upload_expired"
	CodeUploadNotReady       = "asset_upload_not_ready"
	CodeUploadConsumed       = "asset_upload_consumed"
	CodePurposeMismatch      = "asset_purpose_mismatch"
	CodeActorMismatch        = "asset_actor_mismatch"
	CodeTypeNotAllowed       = "asset_type_not_allowed"
	CodeFileTooLarge         = "asset_too_large"
	CodePixelsTooLarge       = "asset_pixels_too_large"
	CodeInvalidContent       = "asset_content_invalid"
	CodeAssetNotFound        = "asset_not_found"
	CodeObjectUnavailable    = "asset_object_unavailable"
	CodeUnsafePath           = "asset_path_unsafe"
	CodeReferenced           = "asset_still_referenced"
	CodeInvalidState         = "asset_state_conflict"
	CodeScanNotFound         = "integrity_scan_not_found"
	CodeGCPlanNotFound       = "asset_gc_plan_not_found"
	CodeGCPlanStale          = "asset_gc_plan_stale"
	CodeValidationFailed     = "validation_failed"
	CodeOperationUnavailable = "asset_store_unavailable"
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

func fileError(code, message, details string, err error) *Error {
	return &Error{Code: code, Message: message, Details: details, Err: err}
}
