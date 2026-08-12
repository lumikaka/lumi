package files

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	_ "golang.org/x/image/webp"
)

type inspection struct {
	MIMEType   string
	Extension  string
	ByteSize   int64
	SHA256     string
	Width      *int
	Height     *int
	DurationMS *int64
}

func inspectContent(path, filename string, policy purposePolicy) (inspection, error) {
	file, err := os.Open(path)
	if err != nil {
		return inspection{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return inspection{}, err
	}
	if info.Size() <= 0 {
		return inspection{}, fileError(CodeInvalidContent, "Asset 内容为空", "空文件不能提交为正式 Asset。", nil)
	}
	if info.Size() > policy.MaxBytes {
		return inspection{}, fileError(CodeFileTooLarge, "Asset 超过大小限制", "文件大小超过 purpose 允许值。", nil)
	}
	reader := bufio.NewReader(file)
	header, _ := reader.Peek(512)
	mimeType := strings.Split(http.DetectContentType(header), ";")[0]
	if len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WEBP" {
		mimeType = "image/webp"
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if (ext == ".txt" || ext == ".md") && (mimeType == "application/octet-stream" || strings.HasPrefix(mimeType, "text/")) {
		if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
			return inspection{}, seekErr
		}
		content, readErr := io.ReadAll(file)
		if readErr != nil {
			return inspection{}, readErr
		}
		if !utf8.Valid(content) || strings.ContainsRune(string(content), 0) || !mostlyText(string(content)) {
			return inspection{}, fileError(CodeInvalidContent, "文本 Asset 内容无效", "文本必须是 UTF-8 且不能包含二进制控制内容。", nil)
		}
		if ext == ".md" {
			mimeType = "text/markdown"
		} else {
			mimeType = "text/plain"
		}
	}
	if _, ok := policy.AllowedMIME[mimeType]; !ok {
		return inspection{}, fileError(CodeTypeNotAllowed, "Asset 类型不允许", "检测到的实际 MIME 为 "+mimeType+"。", nil)
	}
	canonicalExt, ok := extensionForMIME(mimeType)
	if !ok {
		return inspection{}, fileError(CodeTypeNotAllowed, "Asset 类型缺少安全扩展名", "该 MIME 尚未注册正式扩展名。", nil)
	}
	result := inspection{MIMEType: mimeType, Extension: canonicalExt, ByteSize: info.Size()}
	if strings.HasPrefix(mimeType, "image/") {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return inspection{}, err
		}
		config, _, err := image.DecodeConfig(file)
		if err != nil {
			return inspection{}, fileError(CodeInvalidContent, "图片无法解码", "文件头与完整图片结构不一致。", err)
		}
		pixels := int64(config.Width) * int64(config.Height)
		if config.Width <= 0 || config.Height <= 0 || (policy.MaxPixels > 0 && pixels > policy.MaxPixels) {
			return inspection{}, fileError(CodePixelsTooLarge, "图片像素超限", "图片宽高或总像素超过 purpose 限制。", nil)
		}
		result.Width, result.Height = &config.Width, &config.Height
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return inspection{}, err
		}
		decoded, _, err := image.Decode(file)
		if err != nil || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
			return inspection{}, fileError(CodeInvalidContent, "图片内容不完整", "完整图片解码失败或尺寸与文件头不一致。", err)
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return inspection{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return inspection{}, err
	}
	result.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return result, nil
}

func mostlyText(value string) bool {
	controls, runes := 0, 0
	for _, r := range value {
		runes++
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			controls++
		}
	}
	return runes == 0 || controls*100 <= runes
}
