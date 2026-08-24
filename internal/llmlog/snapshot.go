package llmlog

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"lumi/internal/imagegen"
	"lumi/internal/llm"
)

const redactedSnapshotValue = "[REDACTED]"

var (
	snapshotBearerPattern      = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]+`)
	snapshotKeyPattern         = regexp.MustCompile(`(?i)((?:api[_ -]?key|authorization|access[_ -]?token)\s*[:=]\s*)[^\s,;]+`)
	snapshotJSONKeyPattern     = regexp.MustCompile(`(?i)("(?:api[_ -]?key|authorization|access[_ -]?token)"\s*:\s*")[^"]*(")`)
	snapshotUnixPathPattern    = regexp.MustCompile(`(?m)(^|[\s"'\x60(])(?:file://)?/(?:Users|Volumes|home|root|private|var|tmp|opt|etc|mnt|srv|workspace)/[^\s"'\x60)]+`)
	snapshotWindowsPathPattern = regexp.MustCompile(`(?i)(^|[\s"'\x60(])[a-z]:\\[^\s"'\x60)]+`)
)

type textRequestSnapshot struct {
	Model        string              `json:"model"`
	SystemPrompt string              `json:"system_prompt,omitempty"`
	Prompt       string              `json:"prompt"`
	Images       []textImageSnapshot `json:"images,omitempty"`
	Temperature  *float64            `json:"temperature,omitempty"`
	MaxTokens    int                 `json:"max_tokens,omitempty"`
	Stream       bool                `json:"stream"`
}

type textImageSnapshot struct {
	MIMEType string `json:"mime_type"`
	ByteSize int    `json:"byte_size"`
	Detail   string `json:"detail,omitempty"`
}

type chatRequestSnapshot struct {
	Model       string               `json:"model"`
	Messages    []llm.ChatMessage    `json:"messages"`
	Tools       []llm.ToolDefinition `json:"tools,omitempty"`
	Temperature *float64             `json:"temperature,omitempty"`
	MaxTokens   int                  `json:"max_tokens,omitempty"`
	Stream      bool                 `json:"stream"`
}

type textResponseSnapshot struct {
	Content      string    `json:"content"`
	Usage        llm.Usage `json:"usage"`
	FinishReason string    `json:"finish_reason"`
}

type chatResponseSnapshot struct {
	Message      llm.ChatMessage `json:"message"`
	Usage        llm.Usage       `json:"usage"`
	FinishReason string          `json:"finish_reason"`
}

type imageRequestSnapshot struct {
	Model   string               `json:"model"`
	Prompt  string               `json:"prompt"`
	Size    string               `json:"size,omitempty"`
	Quality string               `json:"quality,omitempty"`
	Images  []imageInputSnapshot `json:"images,omitempty"`
}

type imageInputSnapshot struct {
	MIMEType string `json:"mime_type"`
	ByteSize int    `json:"byte_size"`
}

type imageResponseSnapshot struct {
	MIMEType      string `json:"mime_type"`
	ByteSize      int    `json:"byte_size"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

func EncodeTextRequest(input llm.Request) (json.RawMessage, error) {
	images := make([]textImageSnapshot, 0, len(input.Images))
	for _, image := range input.Images {
		images = append(images, textImageSnapshot{MIMEType: image.MIMEType, ByteSize: len(image.Data), Detail: image.Detail})
	}
	return encodeSnapshot(textRequestSnapshot{
		Model: input.Model, SystemPrompt: input.SystemPrompt, Prompt: input.Prompt, Images: images,
		Temperature: input.Temperature, MaxTokens: input.MaxTokens, Stream: true,
	}, input.APIKey)
}

func EncodeTextResponse(input llm.Response, apiKey string) (json.RawMessage, error) {
	return encodeSnapshot(textResponseSnapshot{Content: input.Content, Usage: input.Usage, FinishReason: input.FinishReason}, apiKey)
}

func EncodeChatRequest(input llm.ChatRequest) (json.RawMessage, error) {
	return encodeSnapshot(chatRequestSnapshot{
		Model: input.Model, Messages: input.Messages, Tools: input.Tools,
		Temperature: input.Temperature, MaxTokens: input.MaxTokens, Stream: false,
	}, input.APIKey)
}

func EncodeChatResponse(input llm.ChatResponse, apiKey string) (json.RawMessage, error) {
	return encodeSnapshot(chatResponseSnapshot{Message: input.Message, Usage: input.Usage, FinishReason: input.FinishReason}, apiKey)
}

func EncodeImageRequest(input imagegen.Request) (json.RawMessage, error) {
	images := make([]imageInputSnapshot, 0, len(input.Images))
	for _, image := range input.Images {
		images = append(images, imageInputSnapshot{MIMEType: image.MIMEType, ByteSize: len(image.Data)})
	}
	return encodeSnapshot(imageRequestSnapshot{
		Model: input.Model, Prompt: input.Prompt, Size: input.Size, Quality: input.Quality, Images: images,
	}, input.APIKey)
}

func EncodeImageResponse(input imagegen.Response, apiKey string) (json.RawMessage, error) {
	return encodeSnapshot(imageResponseSnapshot{MIMEType: input.MIMEType, ByteSize: len(input.Bytes), RevisedPrompt: input.RevisedPrompt}, apiKey)
}

func encodeSnapshot(value any, apiKey string) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	redactSnapshotValue(decoded, strings.TrimSpace(apiKey))
	return json.Marshal(decoded)
}

func redactSnapshotValue(value any, apiKey string) {
	switch current := value.(type) {
	case map[string]any:
		for key, item := range current {
			redactedKey := redactSnapshotText(key, apiKey)
			if redactedKey != key {
				delete(current, key)
				current[redactedKey] = item
				key = redactedKey
			}
			if text, ok := item.(string); ok {
				current[key] = redactSnapshotText(text, apiKey)
				continue
			}
			redactSnapshotValue(item, apiKey)
		}
	case []any:
		for index, item := range current {
			if text, ok := item.(string); ok {
				current[index] = redactSnapshotText(text, apiKey)
				continue
			}
			redactSnapshotValue(item, apiKey)
		}
	}
}

func redactSnapshotText(value, apiKey string) string {
	if apiKey != "" {
		value = strings.ReplaceAll(value, apiKey, redactedSnapshotValue)
	}
	value = snapshotBearerPattern.ReplaceAllString(value, "Bearer "+redactedSnapshotValue)
	value = snapshotKeyPattern.ReplaceAllString(value, "${1}"+redactedSnapshotValue)
	value = snapshotJSONKeyPattern.ReplaceAllString(value, "${1}"+redactedSnapshotValue+"${2}")
	value = snapshotUnixPathPattern.ReplaceAllString(value, "${1}[REDACTED_PATH]")
	return snapshotWindowsPathPattern.ReplaceAllString(value, "${1}[REDACTED_PATH]")
}
