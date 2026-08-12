package project

import (
	"context"
	"reflect"
	"testing"
)

func TestNormalizePictureBookInputAppliesFormatDefaults(t *testing.T) {
	minimal := true
	tests := []struct {
		name  string
		input *PictureBookInput
		want  PictureBookProfile
	}{
		{
			name: "classic default",
			want: PictureBookProfile{Format: PictureBookClassic, AspectRatio: AspectRatio{Mode: AspectLandscape, Width: 4, Height: 3}, LargeImageMinimalText: boolPointer(false)},
		},
		{
			name:  "classic portrait minimal text",
			input: &PictureBookInput{Format: PictureBookClassic, AspectRatio: &AspectRatioInput{Mode: AspectPortrait}, LargeImageMinimalText: &minimal},
			want:  PictureBookProfile{Format: PictureBookClassic, AspectRatio: AspectRatio{Mode: AspectPortrait, Width: 3, Height: 4}, LargeImageMinimalText: boolPointer(true)},
		},
		{
			name:  "wordless default",
			input: &PictureBookInput{Format: PictureBookWordless},
			want:  PictureBookProfile{Format: PictureBookWordless, AspectRatio: AspectRatio{Mode: AspectLandscape, Width: 4, Height: 3}},
		},
		{
			name:  "interactive default",
			input: &PictureBookInput{Format: PictureBookInteractive},
			want:  PictureBookProfile{Format: PictureBookInteractive, AspectRatio: AspectRatio{Mode: AspectLandscape, Width: 4, Height: 3}, InteractionMode: stringPointer(InteractionFindIt)},
		},
		{
			name:  "comic default",
			input: &PictureBookInput{Format: PictureBookComicStory},
			want:  PictureBookProfile{Format: PictureBookComicStory, AspectRatio: AspectRatio{Mode: AspectLandscape, Width: 4, Height: 3}, ComicLayout: stringPointer(ComicLayoutPageComic)},
		},
		{
			name:  "custom is reduced",
			input: &PictureBookInput{Format: PictureBookWordless, AspectRatio: &AspectRatioInput{Mode: AspectCustom, Width: 100, Height: 75}},
			want:  PictureBookProfile{Format: PictureBookWordless, AspectRatio: AspectRatio{Mode: AspectCustom, Width: 4, Height: 3}},
		},
		{
			name:  "vertical strip",
			input: &PictureBookInput{Format: PictureBookVertical},
			want:  PictureBookProfile{Format: PictureBookVertical, AspectRatio: AspectRatio{Mode: AspectFixed, Width: 1, Height: 3}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizePictureBookInput(test.input)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("profile=%+v, want %+v", got, test.want)
			}
		})
	}
}

func TestNormalizePictureBookInputRejectsInvalidAndIrrelevantFields(t *testing.T) {
	minimal := false
	interaction := InteractionGuess
	layout := ComicLayoutFourPanel
	tests := []*PictureBookInput{
		{Format: PictureBookVertical, AspectRatio: &AspectRatioInput{Mode: AspectPortrait}},
		{Format: PictureBookWordless, LargeImageMinimalText: &minimal},
		{Format: PictureBookClassic, InteractionMode: &interaction},
		{Format: PictureBookInteractive, ComicLayout: &layout},
		{Format: PictureBookInteractive, AspectRatio: &AspectRatioInput{Mode: AspectSquare}},
		{Format: PictureBookComicStory, InteractionMode: &interaction},
		{Format: PictureBookWordless, AspectRatio: &AspectRatioInput{Mode: AspectLandscape, Width: 4, Height: 3}},
		{Format: PictureBookWordless, AspectRatio: &AspectRatioInput{Mode: AspectCustom, Width: 0, Height: 3}},
		{Format: PictureBookWordless, AspectRatio: &AspectRatioInput{Mode: AspectCustom, Width: 100, Height: 1}},
		{Format: "unknown"},
	}
	for index, input := range tests {
		if _, err := NormalizePictureBookInput(input); errorCode(err) != CodeInvalidPictureBook {
			t.Errorf("case %d error=%v", index, err)
		}
	}
}

func TestCreatePersistsOneCanonicalPictureBookProfile(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.CreateWithInput(ctx, CreateInput{
		Name: "Page Comic",
		PictureBook: &PictureBookInput{
			Format:      PictureBookComicStory,
			AspectRatio: &AspectRatioInput{Mode: AspectCustom, Width: 8, Height: 6},
			ComicLayout: stringPointer(ComicLayoutFourPanel),
		},
	}, ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if created.PictureBook == nil || created.PictureBook.Format != PictureBookComicStory || created.PictureBook.AspectRatio != (AspectRatio{Mode: AspectCustom, Width: 4, Height: 3}) || created.PictureBook.ComicLayout == nil || *created.PictureBook.ComicLayout != ComicLayoutFourPanel {
		t.Fatalf("created profile=%+v", created.PictureBook)
	}
	if err := manager.WithCurrentStore(ctx, created.UUID, func(store *Store) error {
		var count int64
		if err := store.DB().Model(&pictureBookProfileRecord{}).Where("project_id = ?", store.project.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("profile count=%d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	reopened, err := manager.OpenRecent(ctx, created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened.PictureBook, created.PictureBook) {
		t.Fatalf("reopened profile=%+v, want %+v", reopened.PictureBook, created.PictureBook)
	}
}

func TestInteractivePictureBookReloadsWithoutTreatingInternalAspectAsInput(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.CreateWithInput(ctx, CreateInput{
		Name:        "Interactive Book",
		PictureBook: &PictureBookInput{Format: PictureBookInteractive},
	}, ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	reopened, err := manager.OpenRecent(ctx, created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened.PictureBook, created.PictureBook) {
		t.Fatalf("reopened profile=%+v, want %+v", reopened.PictureBook, created.PictureBook)
	}
}
