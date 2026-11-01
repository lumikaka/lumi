package production

import (
	"image"
	"image/draw"
	"math"
)

// CropBox is relative to the complete, unmodified setting board.
type CropBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (b CropBox) valid() bool {
	for _, n := range []float64{b.X, b.Y, b.Width, b.Height} {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return false
		}
	}
	return b.X >= 0 && b.Y >= 0 && b.Width > 0 && b.Height > 0 && b.X+b.Width <= 1.0001 && b.Y+b.Height <= 1.0001
}

// CropSettingImage is shared by provider workflows and host-authored plans.
// It preserves the existing pixel rounding and adds no padding or inference.
func CropSettingImage(source image.Image, crop CropBox) (image.Image, image.Rectangle, error) {
	box, err := settingCropBounds(source.Bounds(), crop)
	if err != nil {
		return nil, box, err
	}
	target := image.NewRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	draw.Draw(target, target.Bounds(), source, box.Min, draw.Src)
	return target, box, nil
}

func settingCropBounds(bounds image.Rectangle, crop CropBox) (image.Rectangle, error) {
	if !crop.valid() {
		return image.Rectangle{}, domainError(CodeValidation, "拆分裁剪框越界", "crop_box 必须是整图的有效归一化坐标。", nil)
	}
	x := bounds.Min.X + int(crop.X*float64(bounds.Dx()))
	y := bounds.Min.Y + int(crop.Y*float64(bounds.Dy()))
	w := int(crop.Width * float64(bounds.Dx()))
	h := int(crop.Height * float64(bounds.Dy()))
	box := image.Rect(x, y, x+w, y+h).Intersect(bounds)
	if box.Dx() < 2 || box.Dy() < 2 {
		return box, domainError(CodeValidation, "拆分裁剪框过小", "每个裁切结果至少为 2×2 像素。", nil)
	}
	return box, nil
}
