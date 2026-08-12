package provider

import "fmt"

const (
	CodeInvalidProvider   = "invalid_provider"
	CodeProviderNotFound  = "provider_not_found"
	CodeSecretMissing     = "provider_secret_missing"
	CodeSecretStoreFailed = "provider_secret_store_failed"
	CodeNoActiveProvider  = "no_active_provider"
	CodeProviderNotReady  = "provider_not_ready"
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

func providerError(code, message, details string, cause error) error {
	return &Error{Code: code, Message: message, Details: details, Cause: cause}
}
