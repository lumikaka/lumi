package sitesettings

import (
	"context"
	"testing"
)

func TestRelayImageModeAndModelAreValidatedAtomically(t *testing.T) {
	_, _, s := testService(t)
	ctx := context.Background()
	mode, _ := s.Value(ctx, RelayImageAPIModeKey)
	if mode != ImageAPIResponses {
		t.Fatalf("legacy mode=%v", mode)
	}
	for _, changes := range []map[string]any{
		{RelayImageAPIModeKey: "auto"},
		{RelayImageAPIModeKey: ImageAPIImages},
		{RelayDefaultImageModelKey: "gpt-image-2"},
		{RelayImageAPIModeKey: ImageAPIImages, RelayDefaultImageModelKey: "gpt-6.1-sol"},
	} {
		if _, _, err := s.Update(ctx, changes); err == nil {
			t.Fatalf("accepted invalid changes: %v", changes)
		}
		current, _ := s.Value(ctx, RelayImageAPIModeKey)
		if current != ImageAPIResponses {
			t.Fatal("partial write")
		}
	}
	if _, _, err := s.Update(ctx, map[string]any{RelayImageAPIModeKey: ImageAPIImages, RelayDefaultImageModelKey: "gpt-image-2"}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{RelayImageAPIModeKey, RelayDefaultImageModelKey} {
		if _, _, err := s.Reset(ctx, []string{key}); err == nil {
			t.Fatal("reset accepted incompatible configuration")
		}
	}
	if _, _, err := s.Reset(ctx, []string{RelayImageAPIModeKey, RelayDefaultImageModelKey}); err != nil {
		t.Fatal(err)
	}
	mode, _ = s.Value(ctx, RelayImageAPIModeKey)
	model, _ := s.Value(ctx, RelayDefaultImageModelKey)
	if mode != ImageAPIResponses || model != "gpt-6.1-sol" {
		t.Fatalf("mode=%v model=%v", mode, model)
	}
}
