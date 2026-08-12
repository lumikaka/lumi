package sitesettings

import "fmt"

const (
	CodeUnknownSetting    = "unknown_site_setting"
	CodeInvalidSetting    = "invalid_site_setting"
	CodeSecretUnavailable = "secret_unavailable"
	CodeStorageFailed     = "site_settings_storage_failed"
)

type Error struct {
	Code, Message, Details string
	Cause                  error
}

func (err *Error) Error() string {
	if err.Cause != nil {
		return fmt.Sprintf("%s: %v", err.Code, err.Cause)
	}
	return err.Code
}

func (err *Error) Unwrap() error { return err.Cause }

func settingError(code, message, details string, cause error) error {
	return &Error{Code: code, Message: message, Details: details, Cause: cause}
}
