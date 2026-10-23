package production

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"lumi/internal/project"
)

// Opt-in fixtures for checking the actual exported reader in a browser. The
// output has no Lumi server dependency; serve the destination with any HTTP server.
func TestWebsiteBrowserFixtures(t *testing.T) {
	destination := os.Getenv("LUMI_WEBSITE_PREVIEW_ROOT")
	if destination == "" {
		t.Skip("set LUMI_WEBSITE_PREVIEW_ROOT to export browser fixtures")
	}
	for _, format := range []string{project.PictureBookClassic, project.PictureBookVertical} {
		h := newProductionHarnessWithFormat(t, format)
		c := websiteChapter(t, h)
		h.service.store.DB().Exec(`UPDATE projects SET name='月光邮局 · The Moonlight Post'`)
		for i := 1; i <= 3; i++ {
			height := 900
			if format == project.PictureBookVertical {
				height = 3600
			}
			canvas := image.NewRGBA(image.Rect(0, 0, 1200, height))
			draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{R: 35 + uint8(i*5), G: 62, B: 76, A: 255}), image.Point{}, draw.Src)
			for y := 100; y < height-100; y++ {
				for x := 100; x < 1100; x++ {
					if (x-800)*(x-800)+(y-260)*(y-260) < 140*140 {
						canvas.Set(x, y, color.RGBA{R: 240, G: 211, B: 142, A: 255})
					}
				}
			}
			for y := 480; y < height-150; y += 320 {
				label := image.NewRGBA(image.Rect(0, 0, 140, 30))
				painter := font.Drawer{Dst: label, Src: image.NewUniform(color.White), Face: basicfont.Face7x13, Dot: fixed.P(5, 18)}
				painter.DrawString(fmt.Sprintf("PAGE %d / MOON MAIL", i))
				draw.CatmullRom.Scale(canvas, image.Rect(140, y, 980, y+180), label, label.Bounds(), draw.Over, nil)
			}
			var encoded bytes.Buffer
			if err := png.Encode(&encoded, canvas); err != nil {
				t.Fatal(err)
			}
			websiteSection(t, h, c.UUID, encoded.Bytes())
		}
		runWebsite(t, h.service)
		if err := os.CopyFS(filepath.Join(destination, format), os.DirFS(filepath.Join(h.service.store.Root(), "website"))); err != nil {
			t.Fatal(err)
		}
	}
}
