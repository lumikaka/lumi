package jobqueue

import (
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"os"
	"testing"

	"lumi/internal/production"
)

func TestSectionPremiseByteBaseline(t *testing.T) {
	for _, count := range []int{1, 4, 12} {
		for _, chinese := range []bool{false, true} {
			sources := make([]sectionPremiseSource, count)
			for i := range sources {
				picture := image.NewNRGBA(image.Rect(0, 0, 19+i*7, 37+i*3))
				for y := 0; y < picture.Bounds().Dy(); y++ {
					for x := 0; x < picture.Bounds().Dx(); x++ {
						picture.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 7), G: uint8(y * 3), B: uint8(i * 17), A: uint8(80 + x%176)})
					}
				}
				title := fmt.Sprintf("Asset %d", i+1)
				if chinese {
					title = fmt.Sprintf("月光小狐狸与黄昏灯塔 %d", i+1)
				}
				sources[i] = sectionPremiseSource{Reference: production.PremiseAssetReference{Title: title}, Image: picture}
			}
			result, err := composeSectionPremise(sources)
			if err != nil {
				if chinese {
					t.Skipf("CJK font unavailable: %v", err)
				}
				t.Fatal(err)
			}
			t.Logf("count=%d chinese=%v sha256=%x", count, chinese, sha256.Sum256(result.Bytes))
			// Frozen before extracting the Web compositor; ASCII uses the bundled font.
			golden := map[int]string{1: "62058236cba9a4eb3b04825984ce51aa5ae199fc5a204627f284e1bf8b027445", 4: "aad3f18977e2ab98eeecd3a0dad3de7fedb0d434fcbd00be8234a21443176349", 12: "e19579cbcd50ed29d1efcf98e8f51c62058995c56c92da66c9c9098df8fa5a3d"}
			if !chinese && fmt.Sprintf("%x", sha256.Sum256(result.Bytes)) != golden[count] {
				t.Fatal("Web compositor PNG bytes changed")
			}
			if dir := os.Getenv("LUMI_COLLAGE_BASELINE"); dir != "" {
				if err := os.WriteFile(fmt.Sprintf("%s/%d-%v.png", dir, count, chinese), result.Bytes, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
