package sitesettings

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	ActiveProviderKey = "ai_provider.active"

	// The persisted prefix remains unchanged so existing encrypted secrets keep
	// their AES-GCM associated-data binding. The public provider contract is
	// Cloudflare AI Gateway, not a generic OpenAI-compatible endpoint.
	CloudflareUUIDKey                = "ai_providers.openai_compatible.uuid"
	CloudflareAccountIDKey           = "ai_providers.openai_compatible.account_id"
	CloudflareDefaultModelKey        = "ai_providers.openai_compatible.default_model"
	CloudflareDefaultImageModelKey   = "ai_providers.openai_compatible.default_image_model"
	CloudflareAPITokenKey            = "ai_providers.openai_compatible.api_key"
	CloudflareVerifiedKey            = "ai_providers.openai_compatible.verified"
	CloudflareVerifiedAtKey          = "ai_providers.openai_compatible.verified_at"
	CloudflareVerifiedFingerprintKey = "ai_providers.openai_compatible.verified_fingerprint"

	BailianUUIDKey                = "ai_providers.aliyun_bailian.uuid"
	BailianWorkspaceKey           = "ai_providers.aliyun_bailian.workspace_id"
	BailianRegionKey              = "ai_providers.aliyun_bailian.region"
	BailianAPIKeyKey              = "ai_providers.aliyun_bailian.api_key"
	BailianVerifiedKey            = "ai_providers.aliyun_bailian.verified"
	BailianVerifiedAtKey          = "ai_providers.aliyun_bailian.verified_at"
	BailianVerifiedFingerprintKey = "ai_providers.aliyun_bailian.verified_fingerprint"
)

type definition struct {
	Key       string
	Default   any
	Secret    bool
	Public    bool
	Mutable   bool
	Normalize func(any) (any, error)
}

var orderedDefinitions = []definition{
	{Key: ActiveProviderKey, Default: "none", Public: true, Mutable: true, Normalize: member("none", "cloudflare_ai_gateway", "openai_compatible", "aliyun_bailian")},
	{Key: CloudflareAccountIDKey, Default: "", Public: true, Mutable: true, Normalize: cloudflareAccountID},
	{Key: CloudflareDefaultModelKey, Default: "deepseek/deepseek-v4-pro", Public: true, Mutable: true, Normalize: cloudflareModel},
	{Key: CloudflareDefaultImageModelKey, Default: "openai/gpt-5.5", Public: true, Mutable: true, Normalize: cloudflareModel},
	{Key: CloudflareAPITokenKey, Default: nil, Secret: true, Public: true, Mutable: true, Normalize: requiredString(8192)},
	{Key: BailianWorkspaceKey, Default: "", Public: true, Mutable: true, Normalize: workspaceID},
	{Key: BailianRegionKey, Default: "cn-beijing", Public: true, Mutable: true, Normalize: member("cn-beijing", "ap-southeast-1", "eu-central-1", "ap-northeast-1")},
	{Key: BailianAPIKeyKey, Default: nil, Secret: true, Public: true, Mutable: true, Normalize: requiredString(8192)},
	{Key: CloudflareUUIDKey, Default: "", Normalize: requiredString(64)},
	{Key: CloudflareVerifiedKey, Default: false, Normalize: boolean},
	{Key: CloudflareVerifiedAtKey, Default: nil, Normalize: optionalString(64)},
	{Key: CloudflareVerifiedFingerprintKey, Default: nil, Normalize: optionalString(64)},
	{Key: BailianUUIDKey, Default: "", Normalize: requiredString(64)},
	{Key: BailianVerifiedKey, Default: false, Normalize: boolean},
	{Key: BailianVerifiedAtKey, Default: nil, Normalize: optionalString(64)},
	{Key: BailianVerifiedFingerprintKey, Default: nil, Normalize: optionalString(64)},
}

func cloudflareAccountID(input any) (any, error) {
	value, ok := input.(string)
	value = strings.ToLower(strings.TrimSpace(value))
	if !ok || len(value) != 32 {
		return nil, fmt.Errorf("must be a 32-character Cloudflare account ID")
	}
	for _, character := range value {
		if (character < 'a' || character > 'f') && (character < '0' || character > '9') {
			return nil, fmt.Errorf("must be a hexadecimal Cloudflare account ID")
		}
	}
	return value, nil
}

func cloudflareModel(input any) (any, error) {
	value, ok := input.(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" || len([]rune(value)) > 255 || strings.ContainsAny(value, " \t\r\n") {
		return nil, fmt.Errorf("must be a non-empty Cloudflare model ID no longer than 255 characters")
	}
	parts := strings.Split(value, "/")
	if len(parts) < 2 || parts[0] == "" || parts[len(parts)-1] == "" {
		return nil, fmt.Errorf("must use Cloudflare's author/model format")
	}
	if parts[0] == "@cf" && len(parts) < 3 {
		return nil, fmt.Errorf("Workers AI models must use @cf/author/model format")
	}
	return value, nil
}

var definitions = func() map[string]definition {
	items := make(map[string]definition, len(orderedDefinitions))
	for _, item := range orderedDefinitions {
		items[item.Key] = item
	}
	return items
}()

func member(values ...string) func(any) (any, error) {
	return func(input any) (any, error) {
		value, ok := input.(string)
		value = strings.TrimSpace(value)
		if !ok {
			return nil, fmt.Errorf("must be a string")
		}
		for _, candidate := range values {
			if value == candidate {
				return value, nil
			}
		}
		return nil, fmt.Errorf("unsupported value")
	}
}

func requiredString(max int) func(any) (any, error) {
	return func(input any) (any, error) {
		value, ok := input.(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" || len([]rune(value)) > max {
			return nil, fmt.Errorf("must be a non-empty string no longer than %d characters", max)
		}
		return value, nil
	}
}

func optionalString(max int) func(any) (any, error) {
	return func(input any) (any, error) {
		if input == nil {
			return nil, nil
		}
		value, ok := input.(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" || len([]rune(value)) > max {
			return nil, fmt.Errorf("must be null or a bounded string")
		}
		return value, nil
	}
}

func boolean(input any) (any, error) {
	value, ok := input.(bool)
	if !ok {
		return nil, fmt.Errorf("must be a boolean")
	}
	return value, nil
}

func httpURL(input any) (any, error) {
	value, ok := input.(string)
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if !ok || value == "" || len(value) > 2048 {
		return nil, fmt.Errorf("must be a non-empty URL")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("must be an HTTP(S) URL without credentials, query, or fragment")
	}
	return value, nil
}

func workspaceID(input any) (any, error) {
	value, ok := input.(string)
	value = strings.TrimSpace(value)
	if !ok || len(value) == 0 || len(value) > 63 || strings.HasPrefix(value, "-") || strings.HasSuffix(value, "-") {
		return nil, fmt.Errorf("invalid workspace ID")
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' {
			return nil, fmt.Errorf("invalid workspace ID")
		}
	}
	return value, nil
}
