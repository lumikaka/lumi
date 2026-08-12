package files

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

type purposePolicy struct {
	Namespace    string
	AllowedMIME  map[string]struct{}
	MaxBytes     int64
	MaxPixels    int64
	MetadataKeys map[string]struct{}
}

func mimeSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

var purposeRegistry = map[string]purposePolicy{
	"story_import":                        {Namespace: "story/imports", AllowedMIME: mimeSet("text/plain", "text/markdown"), MaxBytes: 2 << 20, MetadataKeys: mimeSet("chapter_code", "content_format")},
	"premise_setting_image":               {Namespace: "premise/setting-images", AllowedMIME: mimeSet("image/png", "image/jpeg", "image/gif", "image/webp"), MaxBytes: 32 << 20, MaxPixels: 40_000_000, MetadataKeys: mimeSet("generation", "prompt_revision")},
	"premise_asset":                       {Namespace: "premise/assets", AllowedMIME: mimeSet("image/png", "image/jpeg", "image/gif", "image/webp"), MaxBytes: 32 << 20, MaxPixels: 40_000_000, MetadataKeys: mimeSet("tags", "crop_profile")},
	"comic_section_image":                 {Namespace: "comic/sections", AllowedMIME: mimeSet("image/png", "image/jpeg", "image/gif", "image/webp"), MaxBytes: 64 << 20, MaxPixels: 80_000_000, MetadataKeys: mimeSet("section_uuid", "variant")},
	"comic_section_premise":               {Namespace: "comic/section-premises", AllowedMIME: mimeSet("image/png"), MaxBytes: 64 << 20, MaxPixels: 80_000_000, MetadataKeys: mimeSet("section_uuid", "generation_uuid", "composer_version", "selected_titles")},
	"project_chatbot_reference":           {Namespace: "chat/references", AllowedMIME: mimeSet("image/png", "image/jpeg", "image/webp"), MaxBytes: 32 << 20, MaxPixels: 40_000_000, MetadataKeys: mimeSet("source")},
	"project_chat_asset_image_generation": {Namespace: "chat/generated-assets", AllowedMIME: mimeSet("image/png", "image/jpeg", "image/webp"), MaxBytes: 64 << 20, MaxPixels: 80_000_000, MetadataKeys: mimeSet("source", "tool_execution_uuid", "chat_thread_uuid", "chat_run_uuid", "reference_file_uuids", "revised_prompt")},
	"project_chat_asset_reference_image":  {Namespace: "chat/generated-references", AllowedMIME: mimeSet("image/png", "image/jpeg", "image/webp"), MaxBytes: 64 << 20, MaxPixels: 80_000_000, MetadataKeys: mimeSet("source", "tool_execution_uuid", "chat_thread_uuid", "chat_run_uuid", "premise_asset_uuid", "reference_file_uuids", "revised_prompt")},
	"export":                              {Namespace: "exports", AllowedMIME: mimeSet("application/zip", "application/pdf"), MaxBytes: 256 << 20, MetadataKeys: mimeSet("format", "snapshot_uuid")},
}

func policyFor(purpose string) (purposePolicy, error) {
	purpose = strings.TrimSpace(purpose)
	policy, ok := purposeRegistry[purpose]
	if !ok {
		return purposePolicy{}, fileError(CodePurposeMismatch, "Asset purpose 不受支持", "purpose 必须来自服务端允许列表。", nil)
	}
	return policy, nil
}

func canonicalFilename(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = filepath.Base(value)
	value = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == 0 || unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return "asset.bin"
	}
	runes := []rune(value)
	if len(runes) > 255 {
		value = string(runes[:255])
	}
	return value
}

func safeStem(filename, fallback string) string {
	stem := strings.TrimSuffix(canonicalFilename(filename), filepath.Ext(filename))
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(stem) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
		if builder.Len() >= 80 {
			break
		}
	}
	result := strings.Trim(builder.String(), "-. ")
	reserved := map[string]bool{"con": true, "prn": true, "aux": true, "nul": true, "com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true, "lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true}
	if result == "" || reserved[result] {
		result = fallback
	}
	return result
}

func filteredMetadata(policy purposePolicy, metadata map[string]any) (map[string]any, error) {
	result := map[string]any{}
	for key, value := range metadata {
		if _, ok := policy.MetadataKeys[key]; ok {
			safe, err := safeMetadataValue(value)
			if err != nil {
				return nil, fileError(CodeValidationFailed, "Asset metadata 无效", "允许的 metadata 值只能是有界标量或字符串数组。", err)
			}
			result[key] = safe
		}
	}
	return result, nil
}

func safeMetadataValue(value any) (any, error) {
	switch typed := value.(type) {
	case nil, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return typed, nil
	case string:
		if len([]rune(typed)) > 1024 {
			return nil, fmt.Errorf("metadata string exceeds 1024 characters")
		}
		return typed, nil
	case []string:
		if len(typed) > 64 {
			return nil, fmt.Errorf("metadata string array exceeds 64 items")
		}
		for _, item := range typed {
			if len([]rune(item)) > 256 {
				return nil, fmt.Errorf("metadata array item exceeds 256 characters")
			}
		}
		return typed, nil
	case []any:
		if len(typed) > 64 {
			return nil, fmt.Errorf("metadata array exceeds 64 items")
		}
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok || len([]rune(text)) > 256 {
				return nil, fmt.Errorf("metadata arrays must contain bounded strings")
			}
			result = append(result, text)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("metadata type %T is not allowed", value)
	}
}

func extensionForMIME(mimeType string) (string, bool) {
	extensions := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp", "text/plain": "txt", "text/markdown": "md", "application/zip": "zip", "application/pdf": "pdf", "audio/mpeg": "mp3", "audio/wav": "wav", "video/mp4": "mp4"}
	ext, ok := extensions[mimeType]
	return ext, ok
}

func kindForMIME(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "text/"):
		return "text"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case mimeType == "application/zip":
		return "archive"
	case mimeType == "application/pdf":
		return "document"
	default:
		return "binary"
	}
}

func validateKeyPath(key string) error {
	windowsDrive := len(key) >= 2 && ((key[0] >= 'a' && key[0] <= 'z') || (key[0] >= 'A' && key[0] <= 'Z')) && key[1] == ':'
	if key == "" || windowsDrive || strings.ContainsRune(key, 0) || strings.Contains(key, "\\") || strings.HasPrefix(key, "/") || filepath.IsAbs(key) {
		return fileError(CodeUnsafePath, "Asset 路径不安全", "对象路径必须是规范化项目内相对路径。", nil)
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return fileError(CodeUnsafePath, "Asset 路径越界", fmt.Sprintf("路径段 %q 无效。", part), nil)
		}
	}
	return nil
}
