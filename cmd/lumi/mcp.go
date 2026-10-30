package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
	"lumi/internal/project"
	"lumi/internal/realtime"
	"lumi/internal/textmcp"
)

const creationRoute = "/api/v1/creation-requests"

// root_path is an instance transport field, never part of any MCP schema.
type creationRequest struct {
	RootPath  string          `json:"root_path"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}

func registerTextAuthoring(e *echo.Echo, identity instance, projects *project.Manager, events *realtime.Hub) {
	service := &textmcp.Service{Projects: projects, Events: events}
	e.POST(creationRoute, func(c echo.Context) error {
		if c.Request().Header.Get("If-Match") != identity.Nonce {
			return httpapi.NewError(409, "cli_instance_changed", "CLI 实例已更换", "重新读取 CLI 发现信息后重试。", nil)
		}
		// This CLI-only endpoint shares the existing local-user trust boundary.
		var input creationRequest
		decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, textmcp.MaxBytes+4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return httpapi.NewError(400, "invalid_arguments", "创作请求无效", "请求必须是有限大小的 JSON 对象。", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return httpapi.NewError(400, "invalid_arguments", "创作请求无效", "只能提交一个 JSON 对象。", err)
		}
		return c.JSON(http.StatusOK, service.Call(c.Request().Context(), input.RootPath, input.Tool, input.Arguments))
	})
}

func runMCP(opt options) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	// Do not resolve roots, environment workspace hints, or tool-supplied paths.
	// Only the CLI discovery file is consulted, with live identity verification.
	protocol := textmcp.Protocol(func(ctx context.Context, tool string, args json.RawMessage) httpapi.Envelope {
		callCtx, cancel := context.WithTimeout(ctx, opt.timeout)
		defer cancel()
		status, err := ensureInstance(callCtx, opt)
		if err != nil {
			return textmcp.Failure("instance_unavailable", "CLI 实例不可用", err.Error())
		}
		return forwardCreation(callCtx, status.instance, creationRequest{RootPath: root, Tool: tool, Arguments: args})
	})
	return protocol.Run(context.Background(), &mcp.StdioTransport{})
}

func forwardCreation(ctx context.Context, identity instance, input creationRequest) httpapi.Envelope {
	b, err := json.Marshal(input)
	if err != nil {
		return textmcp.Failure("invalid_arguments", "工具输入无效", err.Error())
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, identity.Endpoint+creationRoute, bytes.NewReader(b))
	if err != nil {
		return textmcp.Failure("instance_unavailable", "CLI 请求失败", err.Error())
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("If-Match", identity.Nonce)
	// Deliberately no Origin, desktop token, Authorization, grant, or OAuth.
	client := *localClient
	client.Timeout = 0
	resp, err := client.Do(r)
	if err != nil {
		return textmcp.Failure("instance_unavailable", "CLI 请求失败", err.Error()+"；写入结果可能已提交，请先重读目标再决定是否重试。")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return textmcp.Failure("cli_upgrade_required", "CLI 实例尚不支持文本创作", "使用相同 environment / data-dir 执行 lumi stop 后重试。")
	}
	b, err = io.ReadAll(io.LimitReader(resp.Body, textmcp.MaxBytes+1))
	if err != nil || len(b) > textmcp.MaxBytes {
		return textmcp.Failure("result_too_large", "CLI 结果超限或读取失败", "请缩小范围或分页读取。")
	}
	var result httpapi.Envelope
	if err := json.Unmarshal(b, &result); err != nil {
		return textmcp.Failure("instance_unavailable", "CLI 返回无效 JSON", fmt.Sprintf("HTTP %d", resp.StatusCode))
	}
	return result
}
