package picturebook

import (
	"errors"
	"strings"
	"testing"

	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
)

func TestRelayResponsesKeepsExactStandardImageSizes(t *testing.T) {
	for _, mode := range []string{"", sitesettings.ImageAPIResponses} {
		for _, model := range provider.RelayModels() {
			for _, tc := range []struct {
				w, h int
				size ImageSize
			}{
				{1, 1, ImageSize{1024, 1024}}, {3, 2, ImageSize{1536, 1024}}, {2, 3, ImageSize{1024, 1536}},
			} {
				got, err := ResolveImageSize(profile(project.PictureBookClassic, project.AspectCustom, tc.w, tc.h), provider.TypeOpenAIRelay, model, mode)
				if err != nil || got != tc.size {
					t.Fatalf("mode=%q model=%s size=%v err=%v", mode, model, got, err)
				}
			}
			if _, err := ResolveImageSize(profile(project.PictureBookClassic, project.AspectLandscape, 4, 3), provider.TypeOpenAIRelay, model, mode); err == nil {
				t.Fatal("approximated unsupported ratio")
			}
		}
	}
}

func TestRelayImagesKeepsExactStandardSizes(t *testing.T) {
	for _, model := range sitesettings.RelayImageModels(sitesettings.ImageAPIImages) {
		for _, tc := range []struct {
			name    string
			profile project.PictureBookProfile
			want    ImageSize
		}{
			{"landscape", profile(project.PictureBookComicStory, project.AspectCustom, 3, 2), ImageSize{1536, 1024}},
			{"portrait", profile(project.PictureBookClassic, project.AspectCustom, 2, 3), ImageSize{1024, 1536}},
			{"square", profile(project.PictureBookClassic, project.AspectSquare, 1, 1), ImageSize{1024, 1024}},
			{"vertical compatibility", profile(project.PictureBookVertical, project.AspectFixed, 1, 3), ImageSize{1024, 1536}},
		} {
			t.Run(model+"/"+tc.name, func(t *testing.T) {
				got, err := ResolveImageSize(tc.profile, provider.TypeOpenAIRelay, model, sitesettings.ImageAPIImages)
				if err != nil || got != tc.want {
					t.Fatalf("size=%v want=%v err=%v", got, tc.want, err)
				}
			})
		}
		for _, ratio := range [][2]int{{4, 3}, {3, 4}, {5, 2}, {100, 99}, {3, 1}} {
			_, err := ResolveImageSize(profile(project.PictureBookClassic, project.AspectCustom, ratio[0], ratio[1]), provider.TypeOpenAIRelay, model, sitesettings.ImageAPIImages)
			var unsupported *UnsupportedError
			if !errors.As(err, &unsupported) || unsupported.ModelModeMismatch {
				t.Fatalf("model=%s ratio=%v err=%v", model, ratio, err)
			}
			if !strings.Contains(SizeErrorDetails(err, ""), "三档尺寸") {
				t.Fatal("missing relay size restriction in error details")
			}
		}
	}
}

func TestRelayRejectsUnknownModeModelAndOutOfBoundsRatio(t *testing.T) {
	for _, tc := range []struct {
		mode, model string
		w, h        int
		mismatch    bool
	}{
		{"images", "gpt-6.1-sol", 1, 1, true},
		{"responses", "gpt-image-2.5-flare", 1, 1, true},
		{"", "gpt-image-2", 1, 1, true},
		{"unknown", "gpt-image-2", 1, 1, true},
		{"images", "gpt-image-unknown", 1, 1, true},
		{"images", "gpt-image-2.5-flare", 4, 1, false},
		{"images", "gpt-image-2.5-flare", 1, 4, false},
		{"images", "gpt-image-2.5-flare", 0, 1, false},
		{"images", "gpt-image-2.5-flare", 241, 240, false},
	} {
		_, err := ResolveImageSize(profile(project.PictureBookClassic, project.AspectCustom, tc.w, tc.h), provider.TypeOpenAIRelay, tc.model, tc.mode)
		var unsupported *UnsupportedError
		if !errors.As(err, &unsupported) || unsupported.ModelModeMismatch != tc.mismatch {
			t.Fatalf("case=%+v err=%v", tc, err)
		}
		details := SizeErrorDetails(err, "请检查配置。")
		for _, want := range []string{"image_api_mode=", "model=" + tc.model, "目标比例=", "请检查配置。"} {
			if !strings.Contains(details, want) {
				t.Fatalf("details=%s missing=%s", details, want)
			}
		}
		if strings.Contains(details, "模型未登记") != tc.mismatch {
			t.Fatalf("misleading details: %s", details)
		}
	}
	// The vertical-strip shortcut must not admit invalid relay configurations.
	if _, err := ResolveImageSize(profile(project.PictureBookVertical, project.AspectFixed, 1, 3), provider.TypeOpenAIRelay, "gpt-6.1-sol", "images"); err == nil {
		t.Fatal("vertical strip bypassed model validation")
	}
}

func TestOtherProvidersIgnoreRelayAPIMode(t *testing.T) {
	for _, tc := range []struct {
		providerType, model string
		want                ImageSize
	}{
		{provider.TypeAliyunBailian, provider.BailianImageModel, ImageSize{1536, 1152}},
		{provider.TypeAliyunBailian, provider.BailianImageModelPro, ImageSize{1536, 1152}},
		{provider.TypeCloudflareAIGateway, "openai/gpt-6-sol", ImageSize{1536, 1152}},
		{provider.LegacyTypeOpenAICompatible, "openai/gpt-5.6-terra", ImageSize{1536, 1152}},
	} {
		for _, mode := range []string{"", "images", "responses", "unknown"} {
			got, err := ResolveImageSize(profile(project.PictureBookClassic, project.AspectLandscape, 4, 3), tc.providerType, tc.model, mode)
			if err != nil || got != tc.want {
				t.Fatalf("provider=%s mode=%s size=%v err=%v", tc.providerType, mode, got, err)
			}
		}
	}
	err := &UnsupportedError{ProviderType: provider.TypeCloudflareAIGateway, Model: "unknown"}
	if got := SizeErrorDetails(err, "existing details"); got != "existing details" {
		t.Fatalf("changed other provider details: %s", got)
	}
}
