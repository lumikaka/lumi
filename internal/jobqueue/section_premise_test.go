package jobqueue

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"lumi/internal/production"
)

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

func TestSectionPremiseCJKFontUnavailableIsExplicit(t *testing.T) {
	_, err := loadSectionPremiseFont([]string{"1. 月光小狐狸"}, []string{"/definitely/not/a/font.ttc"})
	if err == nil || !strings.Contains(err.Error(), "CJK font") {
		t.Fatalf("font error=%v", err)
	}
}
