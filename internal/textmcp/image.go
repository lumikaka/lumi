package textmcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"math"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	xdraw "golang.org/x/image/draw"
	"lumi/internal/files"
	"lumi/internal/httpapi"
)

const maxPreviewBytes = 120 << 10

// ImageData is only an internal CLI transport field. Protocol moves it into
// MCP image content, never the JSON text or structured content shown to Codex.
type imageResult struct {
	AssetUUID string `json:"asset_uuid"`
	MIMEType  string `json:"mime_type"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	ImageData []byte `json:"image_data,omitempty"`
}

func readImage(ctx context.Context, service *files.Service, uuid string, maxEdge int) (imageResult, error) {
	var result imageResult
	content, err := service.OpenContent(ctx, uuid)
	if err != nil {
		return result, err
	}
	defer content.File.Close()
	if content.Asset.Kind != "image" {
		return result, &files.Error{Code: files.CodeTypeNotAllowed, Message: "Asset 不是图片", Details: "请使用项目内图片的 Asset UUID。"}
	}
	data, err := io.ReadAll(io.LimitReader(content.File, (64<<20)+1))
	if err != nil {
		return result, err
	}
	if len(data) > 64<<20 {
		return result, &files.Error{Code: files.CodeFileTooLarge, Message: "参考图片过大", Details: "参考图片最多 64 MiB。"}
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 80_000_000 {
		return result, &files.Error{Code: files.CodeInvalidContent, Message: "参考图片无效", Details: "图片无法解码或像素超限。"}
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return result, &files.Error{Code: files.CodeInvalidContent, Message: "参考图片无法解码", Details: "请检查项目图片内容。"}
	}
	if maxEdge == 0 {
		maxEdge = 1024
	}
	for {
		scale := math.Min(1, float64(maxEdge)/float64(max(config.Width, config.Height)))
		w, h := max(1, int(float64(config.Width)*scale)), max(1, int(float64(config.Height)*scale))
		preview := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(preview, preview.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		xdraw.CatmullRom.Scale(preview, preview.Bounds(), source, source.Bounds(), draw.Over, nil)
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, preview, &jpeg.Options{Quality: 85}); err != nil {
			return result, err
		}
		if encoded.Len() <= maxPreviewBytes {
			return imageResult{AssetUUID: uuid, MIMEType: "image/jpeg", Width: w, Height: h, ImageData: encoded.Bytes()}, nil
		}
		maxEdge = maxEdge * 3 / 4
		if err := ctx.Err(); err != nil {
			return result, err
		}
	}
}

func imageToolResult(envelope httpapi.Envelope) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(envelope.Data)
	if err != nil {
		return nil, err
	}
	var preview imageResult
	if err := json.Unmarshal(b, &preview); err != nil || len(preview.ImageData) == 0 || len(preview.ImageData) > maxPreviewBytes {
		return boundedResult(Failure("asset_content_invalid", "图片预览无效", "重新读取图片。"))
	}
	content := &mcp.ImageContent{MIMEType: preview.MIMEType, Data: preview.ImageData}
	preview.ImageData = nil
	envelope.Data = preview
	return boundedResult(envelope, content)
}

// The App gets exactly one representation. Only structured content carries the
// data URI; the text block remains a small, readable metadata envelope.
func appImageToolResult(envelope httpapi.Envelope) (*mcp.CallToolResult, error) {
	if !envelope.Success {
		return boundedResult(envelope)
	}
	b, err := json.Marshal(envelope.Data)
	if err != nil {
		return nil, err
	}
	var p imageResult
	if json.Unmarshal(b, &p) != nil || len(p.ImageData) == 0 || len(p.ImageData) > maxPreviewBytes {
		return boundedResult(Failure("asset_content_invalid", "图片预览无效", "重新读取图片。"))
	}
	uri := "data:" + p.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(p.ImageData)
	p.ImageData = nil
	summary := httpapi.Envelope{Success: true, Data: p}
	b, err = json.Marshal(summary)
	if err != nil {
		return nil, err
	}
	envelope.Data = map[string]any{"asset_uuid": p.AssetUUID, "mime_type": p.MIMEType, "width": p.Width, "height": p.Height, "data_uri": uri}
	result := &mcp.CallToolResult{StructuredContent: envelope, Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
	wire, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if len(wire) > MaxBytes-1024 {
		return boundedResult(Failure("result_too_large", "图片结果超过 256 KiB", "使用更小的 max_edge。"))
	}
	return result, nil
}
