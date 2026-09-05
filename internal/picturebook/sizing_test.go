package picturebook

import (
	"errors"
	"testing"

	"lumi/internal/project"
	"lumi/internal/provider"
)

func profile(format, mode string, width, height int) project.PictureBookProfile {
	return project.PictureBookProfile{Format: format, AspectRatio: project.AspectRatio{Mode: mode, Width: width, Height: height}}
}

func TestResolveImageSizeUsesExactRegisteredCapabilities(t *testing.T) {
	tests := []struct {
		name         string
		profile      project.PictureBookProfile
		providerType string
		model        string
		want         ImageSize
	}{
		{"bailian landscape", profile(project.PictureBookClassic, project.AspectLandscape, 4, 3), provider.TypeAliyunBailian, "qwen-image-3.0", ImageSize{1536, 1152}},
		{"bailian square", profile(project.PictureBookClassic, project.AspectSquare, 1, 1), provider.TypeAliyunBailian, "qwen-image-3.0", ImageSize{1536, 1536}},
		{"bailian portrait", profile(project.PictureBookClassic, project.AspectPortrait, 3, 4), provider.TypeAliyunBailian, "qwen-image-3.0", ImageSize{1152, 1536}},
		{"bailian custom exact", profile(project.PictureBookClassic, project.AspectCustom, 5, 2), provider.TypeAliyunBailian, "qwen-image-3.0", ImageSize{1535, 614}},
		{"bailian pro landscape", profile(project.PictureBookClassic, project.AspectLandscape, 4, 3), provider.TypeAliyunBailian, provider.BailianImageModelPro, ImageSize{1536, 1152}},
		{"bailian pro square", profile(project.PictureBookClassic, project.AspectSquare, 1, 1), provider.TypeAliyunBailian, provider.BailianImageModelPro, ImageSize{1536, 1536}},
		{"bailian pro portrait", profile(project.PictureBookClassic, project.AspectPortrait, 3, 4), provider.TypeAliyunBailian, provider.BailianImageModelPro, ImageSize{1152, 1536}},
		{"bailian pro custom exact", profile(project.PictureBookClassic, project.AspectCustom, 5, 2), provider.TypeAliyunBailian, provider.BailianImageModelPro, ImageSize{1535, 614}},
		{"vertical strip bailian pro", profile(project.PictureBookVertical, project.AspectFixed, 1, 3), provider.TypeAliyunBailian, provider.BailianImageModelPro, ImageSize{768, 2304}},
		{"cloudflare square", profile(project.PictureBookClassic, project.AspectSquare, 1, 1), provider.TypeCloudflareAIGateway, "openai/gpt-image-1.5", ImageSize{1024, 1024}},
		{"cloudflare landscape", profile(project.PictureBookClassic, project.AspectCustom, 3, 2), provider.TypeCloudflareAIGateway, "openai/gpt-image-1.5", ImageSize{1536, 1024}},
		{"cloudflare portrait", profile(project.PictureBookClassic, project.AspectCustom, 2, 3), provider.TypeCloudflareAIGateway, "openai/gpt-image-1.5", ImageSize{1024, 1536}},
		{"vertical strip bailian", profile(project.PictureBookVertical, project.AspectFixed, 1, 3), provider.TypeAliyunBailian, "anything", ImageSize{768, 2304}},
		{"vertical strip cloudflare", profile(project.PictureBookVertical, project.AspectFixed, 1, 3), provider.TypeCloudflareAIGateway, "anything", ImageSize{1024, 1536}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveImageSize(test.profile, test.providerType, test.model)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("size=%+v, want %+v", got, test.want)
			}
		})
	}
}

func TestResolveImageSizeRejectsUnknownOrInexactCapabilities(t *testing.T) {
	for _, test := range []struct {
		providerType string
		model        string
		profile      project.PictureBookProfile
	}{
		{provider.TypeCloudflareAIGateway, "openai/gpt-image-1.5", profile(project.PictureBookClassic, project.AspectLandscape, 4, 3)},
		{provider.TypeAliyunBailian, "qwen-image-2.0", profile(project.PictureBookClassic, project.AspectLandscape, 4, 3)},
		{provider.TypeAliyunBailian, "qwen-image-3.0-unknown", profile(project.PictureBookClassic, project.AspectSquare, 1, 1)},
		{provider.TypeCloudflareAIGateway, "unknown/image-model", profile(project.PictureBookClassic, project.AspectSquare, 1, 1)},
		{"unknown", "unknown", profile(project.PictureBookClassic, project.AspectSquare, 1, 1)},
	} {
		_, err := ResolveImageSize(test.profile, test.providerType, test.model)
		var unsupported *UnsupportedError
		if !errors.As(err, &unsupported) {
			t.Errorf("error=%v, want UnsupportedError", err)
		}
	}
}
