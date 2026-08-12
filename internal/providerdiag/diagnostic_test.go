package providerdiag

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type countingReadCloser struct {
	reader io.Reader
	read   int
}

func (body *countingReadCloser) Read(buffer []byte) (int, error) {
	count, err := body.reader.Read(buffer)
	body.read += count
	return count, err
}

func (*countingReadCloser) Close() error { return nil }

func TestReadHTTPErrorRecognizesKnownProviderShapes(t *testing.T) {
	tests := []struct {
		name, body, code, message, requestID string
		headers                              http.Header
	}{
		{
			name: "OpenAI nested error", body: `{"error":{"type":"invalid_request_error","message":"invalid input","request_id":"body-request"}}`,
			code: "invalid_request_error", message: "invalid input", requestID: "header-request", headers: http.Header{"X-Request-Id": []string{"header-request"}},
		},
		{
			name: "Bailian top-level error", body: `{"code":"InvalidParameter","message":"unsupported size","request_id":"dashscope-request"}`,
			code: "InvalidParameter", message: "unsupported size", requestID: "dashscope-request",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := &http.Response{StatusCode: http.StatusBadRequest, Header: test.headers, Body: io.NopCloser(strings.NewReader(test.body))}
			details := ReadHTTPError(response, "")
			if details.HTTPStatus != http.StatusBadRequest || details.ProviderCode != test.code || details.Message != test.message || details.RequestID != test.requestID {
				t.Fatalf("details = %+v", details)
			}
		})
	}
}

func TestReadHTTPErrorBoundsBodyAndPersistsNoUnknownPayload(t *testing.T) {
	body := &countingReadCloser{reader: strings.NewReader(strings.Repeat("not-json-secret-data", 5000))}
	response := &http.Response{StatusCode: http.StatusBadGateway, Header: make(http.Header), Body: body}
	details := ReadHTTPError(response, "secret-data")
	if body.read > maxErrorBodyBytes {
		t.Fatalf("read %d bytes, limit is %d", body.read, maxErrorBodyBytes)
	}
	if details.Message != "" || details.ProviderCode != "" || details.RequestID != "" {
		t.Fatalf("unknown response body was persisted: %+v", details)
	}
}

func TestReadHTTPErrorSanitizesAndBoundsMessage(t *testing.T) {
	message := "api_key=top-secret Bearer leaked-token https://cdn.example/image.png?signature=signed-secret " + strings.Repeat("x", 3000)
	response := &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"X-Request-Id": []string{"top-secret"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"top-secret","message":` + quoted(message) + `}}`))}
	details := ReadHTTPError(response, "top-secret")
	if strings.Contains(details.ProviderCode, "top-secret") || strings.Contains(details.RequestID, "top-secret") {
		t.Fatalf("unsafe structured diagnostics = %+v", details)
	}
	if strings.Contains(details.Message, "top-secret") || strings.Contains(details.Message, "leaked-token") || !strings.Contains(details.Message, "[REDACTED]") {
		t.Fatalf("unsafe message = %q", details.Message)
	}
	if strings.Contains(details.Message, "signed-secret") || !strings.Contains(details.Message, "[REDACTED_URL]") {
		t.Fatalf("signed URL was not redacted: %q", details.Message)
	}
	if len([]rune(details.Message)) > 2000 {
		t.Fatalf("message length = %d", len([]rune(details.Message)))
	}
}

type diagnosticError struct{ details Details }

func (err diagnosticError) Error() string               { return "diagnostic error" }
func (err diagnosticError) ProviderDiagnostic() Details { return err.details }
func (err diagnosticError) Unwrap() error               { return nil }

type emptyDiagnosticWrapper struct{ cause error }

func (err emptyDiagnosticWrapper) Error() string               { return "wrapped" }
func (err emptyDiagnosticWrapper) Unwrap() error               { return err.cause }
func (err emptyDiagnosticWrapper) ProviderDiagnostic() Details { return Details{} }

func TestFromErrorFindsNestedNonEmptyDiagnostic(t *testing.T) {
	want := Details{HTTPStatus: 400, ProviderCode: "bad_request", Message: "safe", RequestID: "request-id"}
	if got := FromError(emptyDiagnosticWrapper{cause: diagnosticError{details: want}}); got != want {
		t.Fatalf("details = %+v, want %+v", got, want)
	}
}

func quoted(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
