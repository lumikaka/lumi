package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

type Envelope struct {
	Success bool       `json:"success"`
	Data    any        `json:"data"`
	Error   *ErrorBody `json:"error,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details"`
}

type APIError struct {
	Status  int
	Code    string
	Message string
	Details string
	Cause   error
}

func (err *APIError) Error() string {
	if err.Cause != nil {
		return fmt.Sprintf("%s: %v", err.Message, err.Cause)
	}
	return err.Message
}

func NewError(status int, code, message, details string, cause error) *APIError {
	return &APIError{Status: status, Code: code, Message: message, Details: details, Cause: cause}
}

func Success(c echo.Context, status int, data any) error {
	return c.JSON(status, Envelope{Success: true, Data: data})
}

func ErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	status := http.StatusInternalServerError
	body := ErrorBody{
		Code: "internal_error", Message: "Internal server error", Details: "An unexpected error occurred",
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		status = apiErr.Status
		body = ErrorBody{Code: apiErr.Code, Message: apiErr.Message, Details: apiErr.Details}
	} else {
		var echoErr *echo.HTTPError
		if errors.As(err, &echoErr) {
			status = echoErr.Code
			body = ErrorBody{
				Code: httpErrorCode(status), Message: http.StatusText(status),
				Details: "The requested operation could not be completed",
			}
		}
	}

	if jsonErr := c.JSON(status, Envelope{Success: false, Data: nil, Error: &body}); jsonErr != nil {
		c.Logger().Error(jsonErr)
	}
}

func httpErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusConflict:
		return "conflict"
	case http.StatusUnprocessableEntity:
		return "validation_failed"
	case http.StatusBadGateway:
		return "bad_gateway"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	default:
		return "http_error"
	}
}
