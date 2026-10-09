package modeltest

import (
	"bytes"
	"context"
	"image/png"
	"lumi/internal/provider"
	"lumi/internal/sitesettings"
	"testing"
)

func TestRelayModelTestUsesGroupCredentials(t *testing.T) {
	for _, mode := range []string{"responses", "images"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness(t)
			ctx := context.Background()
			model := "gpt-5.6-sol"
			if mode == "images" {
				model = "gpt-image-2"
			}
			if _, _, err := h.providers.Settings().Update(ctx, map[string]any{sitesettings.RelayImageAPIModeKey: mode, sitesettings.RelayDefaultImageModelKey: model, sitesettings.RelayBaseURLKey: "https://text.example/v1", sitesettings.RelayAPIKeyKey: "text-key", sitesettings.RelayImageBaseURLKey: "https://image.example/v1", sitesettings.RelayImageAPIKeyKey: "image-key"}); err != nil {
				t.Fatal(err)
			}
			items, err := h.providers.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range items {
				if item.ProviderType == provider.TypeOpenAIRelay {
					h.input = ExecutionInput{TextModel: Selection{item.UUID, "gpt-6.1-sol"}, ImageModel: Selection{item.UUID, model}}
				}
			}
			row := h.run(t)
			if row.Status != "succeeded" {
				t.Fatalf("test=%+v", row)
			}
			text, image := h.models.textRequest, h.models.imageRequest
			if text.BaseURL != "https://text.example/v1" || text.APIKey != "text-key" || text.Model != "gpt-6.1-sol" {
				t.Fatal("incorrect text credentials")
			}
			if image.BaseURL != "https://image.example/v1" || image.APIKey != "image-key" || image.Model != model || image.ImageAPIMode != mode {
				t.Fatal("incorrect image credentials")
			}

			if mode == "images" {
				if len(image.Images) != 1 || image.Images[0].MIMEType != "image/png" {
					t.Fatal("edit probe missing reference")
				}
				decoded, err := png.Decode(bytes.NewReader(image.Images[0].Data))
				if err != nil || decoded.Bounds().Dx() != 64 {
					t.Fatalf("invalid reference: %v", err)
				}
			} else if len(image.Images) != 0 {
				t.Fatal("Responses probe changed")
			}
		})
	}
}
