package providerdiag

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxErrorBodyBytes = 64 << 10

var (
	bearerPattern              = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]+`)
	sensitiveAssignmentPattern = regexp.MustCompile(`(?i)((?:[a-z0-9_.-]*(?:api[_ -]?key|authorization|token|password|cookie|secret|credential)[a-z0-9_.-]*)\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;}\]]+)`)
	sensitiveJSONPattern       = regexp.MustCompile(`(?i)("[^"]*(?:api[_ -]?key|authorization|token|password|cookie|secret|credential)[^"]*"\s*:\s*")(?:\\.|[^"\\])*(")`)
	unicodeEscapePattern       = regexp.MustCompile(`(?i)\\u[0-9a-f]{4}`)
	urlPattern                 = regexp.MustCompile(`(?i)https?:\\?/\\?/[^\s<>"']+`)
	quotedUNCPathPattern       = regexp.MustCompile(`"(?:\\\\)+(?:\\.|[^"\\])*"`)
	quotedUnixPathPattern      = regexp.MustCompile(`"(?:\\?/)+(?:\\.|[^"\\])*"`)
	quotedWindowsPathPattern   = regexp.MustCompile(`(?i)"[a-z]:(?:\\+|\\?/)(?:\\.|[^"\\])*"`)
	uncPathPattern             = regexp.MustCompile(`(?m)(^|[\s"'\x60(=:\[,])(?:\\\\)+[^\s<>"'\x60,;)}\]]+`)
	unixPathPattern            = regexp.MustCompile(`(?m)(^|[\s"'\x60(=:\[,])(?:\\?/)+(?:[^\s<>"'\x60,;)}\]]+)`)
	windowsPathPattern         = regexp.MustCompile(`(?i)(^|[\s"'\x60(=:\[,])[a-z]:(?:\\+|\\?/)[^\s<>"'\x60,;)}\]]+`)
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

// RequestID returns the first known Provider request-id header.
func RequestID(headers http.Header) string {
	for _, key := range []string{"X-Request-Id", "Request-Id", "X-Dashscope-Request-Id", "X-Trace-Id"} {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func requestID(headers http.Header) string { return RequestID(headers) }

func sanitize(details Details, apiKey string) Details {
	details.ProviderCode = clean(redact(details.ProviderCode, apiKey), 255)
	details.RequestID = clean(redact(details.RequestID, apiKey), 255)
	details.Message = clean(redact(details.Message, apiKey), 2000)
	return details
}

func redact(value, apiKey string) string {
	value = normalizeSecurityEscapes(value)
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		value = strings.ReplaceAll(value, apiKey, "[REDACTED]")
	}
	value = sensitiveJSONPattern.ReplaceAllString(value, "${1}[REDACTED]${2}")
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = sensitiveAssignmentPattern.ReplaceAllString(value, "${1}[REDACTED]")
	value = urlPattern.ReplaceAllString(value, "[REDACTED_URL]")
	value = quotedUNCPathPattern.ReplaceAllString(value, `"[REDACTED_PATH]"`)
	value = quotedWindowsPathPattern.ReplaceAllString(value, `"[REDACTED_PATH]"`)
	value = quotedUnixPathPattern.ReplaceAllString(value, `"[REDACTED_PATH]"`)
	value = uncPathPattern.ReplaceAllString(value, "${1}[REDACTED_PATH]")
	value = windowsPathPattern.ReplaceAllString(value, "${1}[REDACTED_PATH]")
	value = unixPathPattern.ReplaceAllString(value, "${1}[REDACTED_PATH]")
	return value
}

func normalizeSecurityEscapes(value string) string {
	// Provider bodies are persisted as raw JSON. Decode ASCII Unicode escapes
	// before applying security patterns so keys and path separators such as
	// t\u006fken and \u002f cannot bypass redaction. A few passes also cover
	// nested escaped JSON without making this an unbounded normalization loop.
	for pass := 0; pass < 4; pass++ {
		changed := false
		normalized := unicodeEscapePattern.ReplaceAllStringFunc(value, func(match string) string {
			codePoint, err := strconv.ParseUint(match[2:], 16, 16)
			if err != nil || codePoint > 0x7f {
				return match
			}
			changed = true
			return string(rune(codePoint))
		})
		value = normalized
		if !changed {
			break
		}
	}
	return value
}

// RedactPreview removes credential-like material, signed URLs, and local
// absolute paths before a bounded Provider response preview is persisted.
// maxBytes is a byte limit (not a rune limit) so callers can enforce storage
// budgets precisely.
func RedactPreview(value, apiKey string, maxBytes int) string {
	value = redact(value, apiKey)
	value = clean(value, len([]rune(value))+1)
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	if maxBytes == 1 {
		return "."
	}
	limit := maxBytes - len("…")
	if limit < 0 {
		return strings.Repeat(".", maxBytes)
	}
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit] + "…"
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
