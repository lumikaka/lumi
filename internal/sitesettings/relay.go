package sitesettings

import "fmt"

const (
	ImageAPIResponses = "responses"
	ImageAPIImages    = "images"
)

func RelayTextModels() []string { return []string{"gpt-6.1-sol", "gpt-6", "gpt-5.6-sol"} }

// Image model IDs follow GPT-Image2-Skill's explicit model choices. Availability
// depends on the relay and is checked using the selected group's credentials.
func RelayImageModels(mode string) []string {
	switch mode {
	case "", ImageAPIResponses:
		return RelayTextModels()
	case ImageAPIImages:
		return []string{"gpt-image-2", "gpt-image-2.5-flare", "gpt-image-2.5-sunburst"}
	default:
		return nil
	}
}

func relayImageModel(value any) (any, error) {
	return member(append(RelayTextModels(), RelayImageModels(ImageAPIImages)...)...)(value)
}

// Validate the merged configuration under the settings lock, before any writes.
func validateRelayImageSettings(rows map[string]cachedValue, changes map[string]any, resets map[string]struct{}) error {
	_, modeChanged := changes[RelayImageAPIModeKey]
	_, modelChanged := changes[RelayDefaultImageModelKey]
	_, modeReset := resets[RelayImageAPIModeKey]
	_, modelReset := resets[RelayDefaultImageModelKey]
	if !modeChanged && !modelChanged && !modeReset && !modelReset {
		return nil
	}
	value := func(key string) any {
		if next, ok := changes[key]; ok {
			return next
		}
		if _, ok := resets[key]; ok {
			return definitions[key].Default
		}
		return effectiveValue(definitions[key], rows[key])
	}
	mode := value(RelayImageAPIModeKey).(string)
	if _, err := member(RelayImageModels(mode)...)(value(RelayDefaultImageModelKey)); err != nil {
		return invalidSetting(RelayDefaultImageModelKey, fmt.Errorf("图片模型与 %s 调用方式不匹配，请同时选择兼容的模型", mode))
	}
	return nil
}
