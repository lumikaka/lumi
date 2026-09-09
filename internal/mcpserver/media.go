package mcpserver

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"lumi/internal/files"
	"lumi/internal/project"
)

func (s *Service) readMedia(ctx context.Context, g Grant, fileUUID string) *mcp.CallToolResult {
	var b []byte
	var mime string
	err := s.projects.WithStore(ctx, g.ProjectUUID, func(store *project.Store) error {
		content, err := files.NewService(store, nil).OpenContent(ctx, fileUUID)
		if err != nil {
			return err
		}
		defer content.File.Close()
		mime = content.Asset.MIMEType
		switch mime {
		case "image/png", "image/jpeg", "image/webp", "image/gif":
		default:
			return &mediaError{}
		}
		if content.Asset.ByteSize > 4<<20 {
			return &mediaError{}
		}
		b, err = io.ReadAll(io.LimitReader(content.File, (4<<20)+1))
		if len(b) > 4<<20 {
			return &mediaError{}
		}
		return err
	})
	if err != nil {
		return toolResult(failure("mcp_media_unavailable", "媒体必须为本项目内可读取的 PNG/JPEG/WebP/GIF，且不超过 4 MiB。"))
	}
	result := toolResult(success(map[string]any{"file_uuid": fileUUID, "mime_type": mime, "byte_size": len(b)}))
	result.Content = append(result.Content, &mcp.ImageContent{Data: b, MIMEType: mime})
	return result
}

type mediaError struct{}

func (*mediaError) Error() string { return "media unavailable" }
