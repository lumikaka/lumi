package textmcp

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed appdist/review.html
var reviewHTML string
var ReviewURI = fmt.Sprintf("ui://lumi/review/%x.html", sha256.Sum256([]byte(reviewHTML)))

const ReviewMIME = "text/html;profile=mcp-app"

func appReadAnnotations() *mcp.ToolAnnotations {
	no := false
	return &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &no, OpenWorldHint: &no}
}

func registerReviewResource(s *mcp.Server) {
	s.AddResource(&mcp.Resource{URI: ReviewURI, Name: "lumi-review", MIMEType: ReviewMIME}, func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		// Images and live hints both use the host bridge. The View needs no
		// external network origins, including loopback connections.
		html := strings.ReplaceAll(reviewHTML, "__LUMI_CONNECT_SRC__", "'none'")
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: ReviewURI, MIMEType: ReviewMIME, Text: html, Meta: mcp.Meta{"ui": map[string]any{"prefersBorder": true, "csp": map[string]any{"connectDomains": []string{}, "resourceDomains": []string{}, "frameDomains": []string{}}}}}}}, nil
	})
}
