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

func TestRedactPreviewCoversCredentialsURLsAndLocalPaths(t *testing.T) {
	preview := RedactPreview(`api_key=configured-key authorization: Bearer bearer-value token=token-value password=password-value cookie=cookie-value secret=secret-value credential=credential-value {"refresh_token":"refresh-value","client_secret":"client-secret-value","credentials":"credentials-value"} https://cdn.example/file?signature=signed-value /Users/qingyang/private/file.txt C:\Users\private\file.txt `+strings.Repeat("月", 5000), "configured-key", 8192)
	if len(preview) > 8192 {
		t.Fatalf("preview length = %d", len(preview))
	}
	for _, forbidden := range []string{"configured-key", "bearer-value", "token-value", "password-value", "cookie-value", "secret-value", "credential-value", "refresh-value", "client-secret-value", "credentials-value", "signed-value", "/Users/qingyang", `C:\Users\private`} {
		if strings.Contains(preview, forbidden) {
			t.Fatalf("preview leaked %q: %s", forbidden, preview)
		}
	}
	for _, expected := range []string{"[REDACTED]", "[REDACTED_URL]", "[REDACTED_PATH]"} {
		if !strings.Contains(preview, expected) {
			t.Fatalf("preview missing %q: %s", expected, preview)
		}
	}
}

func TestRedactPreviewCoversEscapedURLsGenericPOSIXPathsAndUNCPaths(t *testing.T) {
	input := `{"escaped_url":"https:\/\/cdn.example\/private\/asset.png?signature=signed-value","escaped_path":"\/Applications\/Lumi\/private.db","generic_path":"/custom-mount/team/private.db","space_path":"/custom mount/team secret/private.db","unc":"\\\\fileserver\\secret-share\\private.db","relative":"docs/api/overview.md"}`
	preview := RedactPreview(input, "", 8192)

	for _, forbidden := range []string{
		"cdn.example", "signed-value", "Applications", "custom-mount", "team secret", "fileserver", "secret-share",
	} {
		if strings.Contains(preview, forbidden) {
			t.Fatalf("preview leaked %q: %s", forbidden, preview)
		}
	}
	if strings.Count(preview, "[REDACTED_URL]") != 1 || strings.Count(preview, "[REDACTED_PATH]") != 4 {
		t.Fatalf("unexpected redaction result: %s", preview)
	}
	if !strings.Contains(preview, "docs/api/overview.md") {
		t.Fatalf("relative path should remain readable: %s", preview)
	}
}

func TestRedactPreviewNormalizesSecurityRelevantUnicodeEscapes(t *testing.T) {
	input := `{"t\u006fken":"unicode-token-secret","url":"https:\u002f\u002fcdn.example\u002fasset?signature=unicode-signature","path":"\u002fUsers\u002fq\u002fprivate.db"}`
	preview := RedactPreview(input, "", 8192)
	for _, forbidden := range []string{"unicode-token-secret", "cdn.example", "unicode-signature", "Users", "private.db"} {
		if strings.Contains(preview, forbidden) {
			t.Fatalf("preview leaked %q: %s", forbidden, preview)
		}
	}
	for _, expected := range []string{"[REDACTED]", "[REDACTED_URL]", "[REDACTED_PATH]"} {
		if !strings.Contains(preview, expected) {
			t.Fatalf("preview missing %q: %s", expected, preview)
		}
	}
}

func TestRedactPreviewAlwaysHonorsByteLimit(t *testing.T) {
	input := strings.Repeat("诊", 5000)
	for _, limit := range []int{0, 1, 2, 7, 8191, 8192} {
		preview := RedactPreview(input, "", limit)
		if len(preview) > limit {
			t.Fatalf("limit=%d preview bytes=%d", limit, len(preview))
		}
		if strings.ToValidUTF8(preview, "?") != preview {
			t.Fatalf("limit=%d produced invalid UTF-8", limit)
		}
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
