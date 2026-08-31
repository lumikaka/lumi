package jobqueue

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"lumi/internal/production"

	"golang.org/x/image/font/basicfont"
)

func TestCreationReferenceEXIFOrientationIsAppliedBeforeComposition(t *testing.T) {
	tiff := make([]byte, 26)
	copy(tiff[:2], "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x0112)
	binary.LittleEndian.PutUint16(tiff[12:14], 3)
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint16(tiff[18:20], 6)
	payload := append([]byte{'E', 'x', 'i', 'f', 0, 0}, tiff...)
	if got := creationReferenceEXIFOrientation(payload); got != 6 {
		t.Fatalf("orientation=%d", got)
	}

	source := image.NewRGBA(image.Rect(10, 20, 12, 23))
	source.Set(10, 20, color.RGBA{R: 10, G: 20, A: 255})
	source.Set(10, 22, color.RGBA{R: 10, G: 22, A: 255})
	source.Set(11, 22, color.RGBA{R: 11, G: 22, A: 255})
	oriented := orientCreationReferenceImage(source, 6)
	if oriented.Bounds() != image.Rect(0, 0, 3, 2) {
		t.Fatalf("oriented bounds=%v", oriented.Bounds())
	}
	for point, want := range map[image.Point]color.RGBA{
		{X: 0, Y: 0}: {R: 10, G: 22, A: 255},
		{X: 2, Y: 0}: {R: 10, G: 20, A: 255},
		{X: 0, Y: 1}: {R: 11, G: 22, A: 255},
	} {
		got := color.RGBAModel.Convert(oriented.At(point.X, point.Y)).(color.RGBA)
		if got != want {
			t.Fatalf("oriented pixel %v=%v want=%v", point, got, want)
		}
	}
	if got := creationReferenceEXIFOrientation([]byte("not exif")); got != 1 {
		t.Fatalf("invalid orientation=%d", got)
	}
}

func TestPremiseReferenceUUIDsPreservesPersistedSelectionOrder(t *testing.T) {
	references := []production.PremiseAssetReference{{AssetUUID: "asset-b"}, {AssetUUID: "asset-a"}}
	if got := premiseReferenceUUIDs(references); len(got) != 2 || got[0] != "asset-b" || got[1] != "asset-a" {
		t.Fatalf("premise reference UUIDs=%v", got)
	}
	if got := premiseReferenceUUIDs(nil); len(got) != 0 {
		t.Fatalf("empty premise reference UUIDs=%v", got)
	}
}

func TestComposeSectionPremiseFiveAssetsUsesSelectionGridAndCJKLabels(t *testing.T) {
	sources := make([]sectionPremiseSource, 5)
	colors := []color.RGBA{{R: 210, A: 255}, {G: 210, A: 255}, {B: 210, A: 255}, {R: 210, G: 160, A: 255}, {G: 180, B: 210, A: 255}}
	for index := range sources {
		picture := image.NewRGBA(image.Rect(0, 0, 120+index*7, 80+index*5))
		for y := 0; y < picture.Bounds().Dy(); y++ {
			for x := 0; x < picture.Bounds().Dx(); x++ {
				picture.Set(x, y, colors[index])
			}
		}
		sources[index] = sectionPremiseSource{Reference: production.PremiseAssetReference{Title: []string{"月光小狐狸", "黄昏灯塔", "银色邮包", "山谷入口", "星空地图"}[index]}, Image: picture}
	}
	composition, err := composeSectionPremise(sources)
	if err != nil {
		if strings.Contains(err.Error(), "CJK font") {
			t.Skipf("system CJK font unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if composition.Width != 1176 || composition.Height != 816 {
		t.Fatalf("composition size=%dx%d", composition.Width, composition.Height)
	}
	decoded, err := png.Decode(bytes.NewReader(composition.Bytes))
	if err != nil {
		t.Fatal(err)
	}
	for index, wanted := range colors {
		column, row := index%3, index/3
		x := sectionPremiseGap + column*(sectionPremiseTileWidth+sectionPremiseGap) + sectionPremiseTileWidth/2
		y := sectionPremiseGap + row*(sectionPremiseTileHeight+sectionPremiseLabelHeight+sectionPremiseGap) + sectionPremiseTileHeight/2
		actual := color.RGBAModel.Convert(decoded.At(x, y)).(color.RGBA)
		if actual.R != wanted.R || actual.G != wanted.G || actual.B != wanted.B {
			t.Fatalf("asset %d center=%v want=%v", index+1, actual, wanted)
		}
	}
	labelHasInk := false
	for y := sectionPremiseGap + sectionPremiseTileHeight; y < sectionPremiseGap+sectionPremiseTileHeight+sectionPremiseLabelHeight; y++ {
		for x := sectionPremiseGap; x < sectionPremiseGap+sectionPremiseTileWidth; x++ {
			pixel := color.RGBAModel.Convert(decoded.At(x, y)).(color.RGBA)
			if pixel.R < 200 && pixel.G < 200 && pixel.B < 200 {
				labelHasInk = true
				break
			}
		}
	}
	if !labelHasInk {
		t.Fatal("first CJK label was not rendered")
	}
}

func TestComposeSectionPremiseTwelveAssets(t *testing.T) {
	sources := make([]sectionPremiseSource, maxSectionPremiseAssets)
	for index := range sources {
		picture := image.NewRGBA(image.Rect(0, 0, 20, 20))
		sources[index] = sectionPremiseSource{Reference: production.PremiseAssetReference{Title: "Asset"}, Image: picture}
	}
	composition, err := composeSectionPremise(sources)
	if err != nil {
		t.Fatal(err)
	}
	if composition.Width != 1176 || composition.Height != 1608 {
		t.Fatalf("composition size=%dx%d", composition.Width, composition.Height)
	}
	if _, err := png.Decode(bytes.NewReader(composition.Bytes)); err != nil {
		t.Fatal(err)
	}
}

func TestCreationReferenceBoardSupportsSixteenTransparentImagesDeterministically(t *testing.T) {
	sources := make([]sectionPremiseSource, maxCreationReferenceFiles)
	labels := make([]string, maxCreationReferenceFiles)
	for index := range sources {
		picture := image.NewNRGBA(image.Rect(0, 0, 32+index, 24+index))
		picture.SetNRGBA(index%picture.Bounds().Dx(), index%picture.Bounds().Dy(), color.NRGBA{R: uint8(index * 11), G: 80, B: 170, A: 128})
		sources[index] = sectionPremiseSource{Reference: production.PremiseAssetReference{Title: "Asset"}, Image: picture}
		labels[index] = fmt.Sprintf("%d. [style] Asset %d", index+1, index+1)
	}

	first, err := composeSectionPremiseWithFace(sources, labels, basicfont.Face7x13)
	if err != nil {
		t.Fatal(err)
	}
	second, err := composeSectionPremiseWithFace(sources, labels, basicfont.Face7x13)
	if err != nil {
		t.Fatal(err)
	}
	if first.Width != 1176 || first.Height != 2400 {
		t.Fatalf("composition size=%dx%d", first.Width, first.Height)
	}
	if !bytes.Equal(first.Bytes, second.Bytes) {
		t.Fatal("creation reference board output was not deterministic")
	}
	if _, err := png.Decode(bytes.NewReader(first.Bytes)); err != nil {
		t.Fatal(err)
	}
}

func TestSectionPremiseCJKFontUnavailableIsExplicit(t *testing.T) {
	_, err := loadSectionPremiseFont([]string{"1. 月光小狐狸"}, []string{"/definitely/not/a/font.ttc"})
	if err == nil || !strings.Contains(err.Error(), "CJK font") {
		t.Fatalf("font error=%v", err)
	}
}
