package providerdiag

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

const maxErrorBodyBytes = 64 << 10

var (
	bearerPattern = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]+`)
	apiKeyPattern = regexp.MustCompile(`(?i)(api[_ -]?key\s*[:=]\s*)[^\s,;]+`)
	urlPattern    = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)
)

// Details is the deliberately small, safe subset of a Provider error response
// that Lumi may persist and expose in project diagnostics.
type Details struct {
	HTTPStatus   int
	ProviderCode string
	Message      string
	RequestID    string
}

type Carrier interface {
	ProviderDiagnostic() Details
}

func FromError(err error) Details {
	if err == nil {
		return Details{}
	}
	if carrier, ok := err.(Carrier); ok {
		if details := carrier.ProviderDiagnostic(); details != (Details{}) {
			return details
		}
	}
	type multiUnwrapper interface{ Unwrap() []error }
	if joined, ok := err.(multiUnwrapper); ok {
		for _, nested := range joined.Unwrap() {
			if details := FromError(nested); details != (Details{}) {
				return details
			}
		}
		return Details{}
	}
	if nested := errors.Unwrap(err); nested != nil {
		return FromError(nested)
	}
	return Details{}
}

func ReadHTTPError(response *http.Response, apiKey string) Details {
	if response == nil {
		return Details{}
	}
	details := Details{HTTPStatus: response.StatusCode, RequestID: requestID(response.Header)}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes))
	if err != nil || len(body) == 0 {
		return sanitize(details, apiKey)
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return sanitize(details, apiKey)
	}
	candidates := []map[string]any{payload}
	for _, key := range []string{"error", "output"} {
		if nested, ok := payload[key].(map[string]any); ok {
			candidates = append(candidates, nested)
		}
	}
	for _, candidate := range candidates {
		if details.ProviderCode == "" {
			details.ProviderCode = firstString(candidate, "code", "type", "error_code")
		}
		if details.Message == "" {
			details.Message = firstString(candidate, "message", "error_message", "detail")
		}
		if details.RequestID == "" {
			details.RequestID = firstString(candidate, "request_id", "requestId", "request-id")
		}
	}
	return sanitize(details, apiKey)
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, exists := values[key]
		if !exists || value == nil {
			continue
		}
		if text, ok := value.(string); ok {
			if strings.TrimSpace(text) != "" {
				return text
			}
			continue
		}
		switch value.(type) {
		case float64, bool, json.Number:
			return fmt.Sprint(value)
		}
	}
	return ""
}

func requestID(headers http.Header) string {
	for _, key := range []string{"X-Request-Id", "Request-Id", "X-Dashscope-Request-Id", "X-Trace-Id"} {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func sanitize(details Details, apiKey string) Details {
	details.ProviderCode = clean(redact(details.ProviderCode, apiKey), 255)
	details.RequestID = clean(redact(details.RequestID, apiKey), 255)
	details.Message = clean(redact(details.Message, apiKey), 2000)
	return details
}

func redact(value, apiKey string) string {
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		value = strings.ReplaceAll(value, apiKey, "[REDACTED]")
	}
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = apiKeyPattern.ReplaceAllString(value, "${1}[REDACTED]")
	return urlPattern.ReplaceAllString(value, "[REDACTED_URL]")
}

func clean(value string, limit int) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value))
	runes := []rune(value)
	if len(runes) > limit {
		if limit == 1 {
			return "…"
		}
		return string(runes[:limit-1]) + "…"
	}
	return value
}
