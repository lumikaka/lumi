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
		if provider.UsesCloudflareImageTool(model) {
			if size, ok := cloudflareCustomImageSize(ratio); ok {
				return size, nil
			}
			break
		}
		if !registeredCloudflareImageModel(model) {
			break
		}
		for _, candidate := range []ImageSize{{Width: 1024, Height: 1024}, {Width: 1536, Height: 1024}, {Width: 1024, Height: 1536}} {
			if exactRatio(ratio, candidate.Width, candidate.Height) {
				return candidate, nil
			}
		}
	case provider.TypeAliyunBailian:
		if model == provider.BailianImageModel || model == provider.BailianImageModelPro {
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
	return strings.HasPrefix(model, "openai/gpt-image-")
}

// The pinned Cloudflare image tool accepts custom dimensions in multiples of
// 16. Prefer a 1536px long edge (1024px for square images), increasing it only
// when needed to preserve an exact ratio within the tool's pixel limits.
func cloudflareCustomImageSize(ratio project.AspectRatio) (ImageSize, bool) {
	if ratio.Width < 1 || ratio.Height < 1 || ratio.Width > ratio.Height*3 || ratio.Height > ratio.Width*3 {
		return ImageSize{}, false
	}
	longest, target := max(ratio.Width, ratio.Height), 1536
	if ratio.Width == ratio.Height {
		target = 1024
	}
	var best ImageSize
	for scale := 16; scale*longest <= 3840; scale += 16 {
		size := ImageSize{Width: ratio.Width * scale, Height: ratio.Height * scale}
		pixels := size.Width * size.Height
		if pixels < 655360 || pixels > 8294400 {
			continue
		}
		if scale*longest > target {
			if best.Width == 0 {
				best = size
			}
			break
		}
		best = size
	}
	return best, best.Width != 0
}
