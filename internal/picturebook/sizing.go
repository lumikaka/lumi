package picturebook

import (
	"fmt"
	"strings"

	"lumi/internal/project"
	"lumi/internal/provider"
)

const CodeAspectRatioUnsupported = "image_aspect_ratio_unsupported"

type UnsupportedError struct {
	ProviderType string
	Model        string
	Ratio        project.AspectRatio
}

func (err *UnsupportedError) Error() string {
	return fmt.Sprintf("%s model %s does not support exact %d:%d output", err.ProviderType, err.Model, err.Ratio.Width, err.Ratio.Height)
}

type ImageSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (size ImageSize) String() string { return fmt.Sprintf("%dx%d", size.Width, size.Height) }

func exactRatio(ratio project.AspectRatio, width, height int) bool {
	return ratio.Width*height == ratio.Height*width
}

// ResolveImageSize is the single provider/model capability registry used both
// by preflight and durable image task creation. It never crops or approximates.
func ResolveImageSize(profile project.PictureBookProfile, providerType, model string) (ImageSize, error) {
	providerType = strings.ToLower(strings.TrimSpace(providerType))
	model = strings.ToLower(strings.TrimSpace(model))
	if profile.Format == project.PictureBookVertical {
		if providerType == provider.TypeAliyunBailian {
			return ImageSize{Width: 768, Height: 2304}, nil
		}
		return ImageSize{Width: 1024, Height: 1536}, nil
	}
	ratio := profile.AspectRatio
	switch providerType {
	case provider.TypeCloudflareAIGateway, provider.LegacyTypeOpenAICompatible:
		if !registeredCloudflareImageModel(model) {
			break
		}
		for _, candidate := range []ImageSize{{Width: 1024, Height: 1024}, {Width: 1536, Height: 1024}, {Width: 1024, Height: 1536}} {
			if exactRatio(ratio, candidate.Width, candidate.Height) {
				return candidate, nil
			}
		}
	case provider.TypeAliyunBailian:
		if model == "qwen-image-3.0" {
			switch ratio.Mode {
			case project.AspectLandscape:
				return ImageSize{Width: 1536, Height: 1152}, nil
			case project.AspectSquare:
				return ImageSize{Width: 1536, Height: 1536}, nil
			case project.AspectPortrait:
				return ImageSize{Width: 1152, Height: 1536}, nil
			case project.AspectCustom:
				longest := ratio.Width
				if ratio.Height > longest {
					longest = ratio.Height
				}
				scale := 1536 / longest
				if scale > 0 {
					return ImageSize{Width: ratio.Width * scale, Height: ratio.Height * scale}, nil
				}
			}
		}
	}
	return ImageSize{}, &UnsupportedError{ProviderType: providerType, Model: model, Ratio: ratio}
}

func registeredCloudflareImageModel(model string) bool {
	// Cloudflare's Responses adapter currently exposes OpenAI image-capable
	// model identifiers. Keep this explicit so arbitrary/unknown models cannot
	// silently inherit an output-size capability.
	return strings.HasPrefix(model, "openai/gpt-image-") || strings.HasPrefix(model, "openai/gpt-5")
}
