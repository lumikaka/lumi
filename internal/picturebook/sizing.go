package picturebook

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
)

const CodeAspectRatioUnsupported = "image_aspect_ratio_unsupported"

type UnsupportedError struct {
	ProviderType      string
	Model             string
	Ratio             project.AspectRatio
	ImageAPIMode      string
	ModelModeMismatch bool
}

func (err *UnsupportedError) Error() string {
	if err.ModelModeMismatch {
		return fmt.Sprintf("%s model %s has no registered capability for image API mode %s", err.ProviderType, err.Model, err.ImageAPIMode)
	}
	return fmt.Sprintf("%s model %s does not support exact %d:%d output", err.ProviderType, err.Model, err.Ratio.Width, err.Ratio.Height)
}

// SizeErrorDetails adds public relay capability context without changing the
// existing error code or other providers' messages.
func SizeErrorDetails(err error, fallback string) string {
	var unsupported *UnsupportedError
	if !errors.As(err, &unsupported) || unsupported.ProviderType != provider.TypeOpenAIRelay {
		return fallback
	}
	reason := "中转站当前仅按 1024x1024（1:1）、1536x1024（3:2）、1024x1536（2:3）三档尺寸接入，目标比例无法精确生成"
	if unsupported.ModelModeMismatch {
		reason = "图片调用模式未知，或模型未登记在该模式下；请检查图片调用模式与模型配置"
	}
	return fmt.Sprintf("Provider=%s，image_api_mode=%s，model=%s，目标比例=%d:%d：%s。%s", unsupported.ProviderType, unsupported.ImageAPIMode, unsupported.Model, unsupported.Ratio.Width, unsupported.Ratio.Height, reason, fallback)
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
func ResolveImageSize(profile project.PictureBookProfile, providerType, model, imageAPIMode string) (ImageSize, error) {
	providerType = strings.ToLower(strings.TrimSpace(providerType))
	model = strings.ToLower(strings.TrimSpace(model))
	imageAPIMode = strings.ToLower(strings.TrimSpace(imageAPIMode))
	if providerType == provider.TypeOpenAIRelay {
		if imageAPIMode == "" {
			imageAPIMode = sitesettings.ImageAPIResponses
		}
		if !slices.Contains(sitesettings.RelayImageModels(imageAPIMode), model) {
			return ImageSize{}, &UnsupportedError{ProviderType: providerType, Model: model, Ratio: profile.AspectRatio, ImageAPIMode: imageAPIMode, ModelModeMismatch: true}
		}
	}
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
			if size, ok := gptImageCustomSize(ratio); ok {
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
	case provider.TypeOpenAIRelay:
		// Relay adapters may expose newer model names while still accepting only
		// standard sizes. Keep both API modes on the same conservative contract.
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
	return ImageSize{}, &UnsupportedError{ProviderType: providerType, Model: model, Ratio: ratio, ImageAPIMode: imageAPIMode}
}

func registeredCloudflareImageModel(model string) bool {
	// Cloudflare's Responses adapter currently exposes OpenAI image-capable
	// model identifiers. Keep this explicit so arbitrary/unknown models cannot
	// silently inherit an output-size capability.
	return strings.HasPrefix(model, "openai/gpt-image-")
}

// The pinned Cloudflare image tool accepts dimensions in multiples of
// 16. Prefer a 1536px long edge (1024px for square images), increasing it only
// when needed to preserve an exact ratio within the tool's pixel limits.
func gptImageCustomSize(ratio project.AspectRatio) (ImageSize, bool) {
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
