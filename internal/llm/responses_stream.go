package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"lumi/internal/providerdiag"
)

func readResponsesStream(ctx context.Context, response *http.Response, apiKey string, onDelta func(string) error, probe bool) (ChatResponse, error) {
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), maxChatResponseBodyBytes+1)
	var data strings.Builder
	var text strings.Builder
	partial := ChatResponse{Message: ChatMessage{Role: "assistant"}}
	var terminal bool
	var result ChatResponse
	var resultErr error
	consume := func() error {
		if data.Len() == 0 {
			return nil
		}
		body := []byte(strings.TrimSuffix(data.String(), "\n"))
		data.Reset()
		if string(body) == "[DONE]" {
			return nil
		}
		var event struct {
			Type     string          `json:"type"`
			Delta    string          `json:"delta"`
			Response json.RawMessage `json:"response"`
		}
		if err := json.Unmarshal(body, &event); err != nil {
			_, resultErr = invalidChatResponse(response, apiKey, body, int64(len(body)), false, ProviderResponseMalformedJSON, nil, nil, partial, err)
			return resultErr
		}
		switch event.Type {
		case "response.output_text.delta":
			if text.Len()+len(event.Delta) > maxChatResponseBodyBytes {
				_, resultErr = invalidChatResponse(response, apiKey, nil, int64(text.Len()+len(event.Delta)), true, ProviderResponseBodyTooLarge, nil, nil, partial, nil)
				return resultErr
			}
			text.WriteString(event.Delta)
			partial.Message.Content = text.String()
			if onDelta != nil && event.Delta != "" {
				return onDelta(event.Delta)
			}
		case "response.completed", "response.incomplete", "response.failed":
			terminal = true
			result, resultErr = parseResponsesBody(response, apiKey, event.Response, probe)
		case "error":
			failureResponse := *response
			failureResponse.Body = io.NopCloser(bytes.NewReader(body))
			resultErr = &Error{Code: CodeProviderResponse, SafeMessage: "Provider 返回了流式错误。", Diagnostic: providerdiag.ReadHTTPError(&failureResponse, apiKey), PartialResponse: &partial}
			return resultErr
		}
		return nil
	}
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return partial, classify(err, 0)
		}
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := consume(); err != nil {
				return partial, err
			}
			if terminal {
				return result, resultErr
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			if data.Len()+len(value)+1 > maxChatResponseBodyBytes {
				return invalidChatResponse(response, apiKey, nil, int64(data.Len()+len(value)), true, ProviderResponseBodyTooLarge, nil, nil, partial, nil)
			}
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return partial, classify(err, 0)
	}
	if err := ctx.Err(); err != nil {
		return partial, classify(err, 0)
	}
	if err := consume(); err != nil {
		return partial, err
	}
	if terminal {
		return result, resultErr
	}
	return invalidChatResponse(response, apiKey, nil, 0, true, ProviderResponseStreamIncomplete, nil, nil, partial, io.ErrUnexpectedEOF)
}
