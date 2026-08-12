package files

import (
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

const thumbnailProcessorVersion = "v1"

var thumbnailProfiles = map[string]int{"grid_256": 256, "detail_1024": 1024}

func (service *Service) EnsureThumbnail(ctx context.Context, assetUUID, profile string) (string, error) {
	var relative string
	err := service.store.WithFileCommit(func() error {
		var err error
		relative, err = service.ensureThumbnail(ctx, assetUUID, profile)
		return err
	})
	return relative, err
}

func (service *Service) ensureThumbnail(ctx context.Context, assetUUID, profile string) (string, error) {
	maxDimension, ok := thumbnailProfiles[profile]
	if !ok {
		return "", fileError(CodeValidationFailed, "缩略图 profile 无效", "profile 只支持 grid_256 或 detail_1024。", nil)
	}
	row, err := service.assetRowByUUID(ctx, assetUUID, false)
	if err != nil {
		return "", err
	}
	if row.ObjectState != ObjectReady || row.Kind != "image" {
		return "", fileError(CodeObjectUnavailable, "Asset 不能生成缩略图", "只有 ready 图片对象支持缩略图。", nil)
	}
	relative := filepath.ToSlash(filepath.Join(".lumi", "thumbnails", row.SHA256, thumbnailProcessorVersion+"-"+profile+".jpg"))
	target, err := service.store.ResolvePath(relative)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Lstat(target); statErr == nil && info.Mode().IsRegular() && info.Size() > 0 {
		return relative, nil
	}
	source, err := service.assetPath(row.KeyPath)
	if err != nil {
		return "", err
	}
	file, err := os.Open(source)
	if err != nil {
		return "", err
	}
	decoded, _, decodeErr := image.Decode(file)
	_ = file.Close()
	if decodeErr != nil {
		return "", fileError(CodeInvalidContent, "图片无法生成缩略图", "源图片解码失败。", decodeErr)
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return "", fileError(CodeInvalidContent, "图片尺寸无效", "源图片宽高必须大于零。", nil)
	}
	scale := float64(maxDimension) / float64(width)
	if height > width {
		scale = float64(maxDimension) / float64(height)
	}
	if scale > 1 {
		scale = 1
	}
	targetWidth, targetHeight := int(float64(width)*scale), int(float64(height)*scale)
	if targetWidth < 1 {
		targetWidth = 1
	}
	if targetHeight < 1 {
		targetHeight = 1
	}
	resized := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.CatmullRom.Scale(resized, resized.Bounds(), decoded, bounds, draw.Over, nil)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", err
	}
	temp := target + ".part"
	output, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			_ = os.Remove(temp)
			output, err = os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		}
		if err != nil {
			return "", err
		}
	}
	encodeErr := jpeg.Encode(output, resized, &jpeg.Options{Quality: 88})
	syncErr := output.Sync()
	closeErr := output.Close()
	if encodeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(temp)
		if encodeErr != nil {
			return "", encodeErr
		}
		if syncErr != nil {
			return "", syncErr
		}
		return "", closeErr
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Remove(temp)
		return "", err
	}
	_ = syncDirectory(filepath.Dir(target))
	service.emit("thumbnail/ready", map[string]any{"asset_uuid": assetUUID, "profile": profile, "status": "ready"})
	return relative, nil
}
