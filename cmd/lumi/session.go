package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"lumi/internal/httpapi"
	"lumi/internal/project"
	"lumi/internal/realtime"
	"lumi/internal/sessionimport"
)

const sessionImportsRoute = "/api/v1/session-imports"

type sessionImportRequest struct {
	RootPath string `json:"root_path"`
	Rollout  string `json:"rollout_path"`
}

// CLI-only transport. The desktop only exposes the read-only history route.
func registerSessionImports(e *echo.Echo, identity instance, projects *project.Manager, events *realtime.Hub) {
	service := &sessionimport.Service{Projects: projects, Events: events}
	e.POST(sessionImportsRoute, func(c echo.Context) error {
		if c.Request().Header.Get("If-Match") != identity.Nonce {
			return httpapi.NewError(409, "cli_instance_changed", "CLI 实例已更换", "重新读取 CLI 发现信息后重试。", nil)
		}
		var input sessionImportRequest
		decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return httpapi.NewError(400, "invalid_arguments", "导入参数无效", "", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return httpapi.NewError(400, "invalid_arguments", "只能提交一个 JSON 对象", "", nil)
		}
		result, err := service.Import(c.Request().Context(), input.RootPath, input.Rollout)
		if err != nil {
			var pe *project.Error
			if errors.As(err, &pe) {
				return httpapi.ProjectAPIError(err)
			}
			return httpapi.NewError(422, "session_import_failed", "会话导入失败", err.Error(), err)
		}
		return httpapi.Success(c, http.StatusOK, result)
	})
}

func importSessionCommand(ctx context.Context, opt options, out, stderr io.Writer) int {
	if opt.rollout == "" {
		return printError(opt, out, stderr, "rollout_required", errors.New("use lumi session import --rollout PATH; automatic hooks are unavailable until Codex App hook trust and lifecycle behavior are verified"), 2)
	}
	root, err := os.Getwd()
	if err != nil {
		return printError(opt, out, stderr, "session_import_failed", err, 1)
	}
	path, err := filepath.Abs(opt.rollout)
	if err != nil {
		return printError(opt, out, stderr, "session_import_failed", err, 1)
	}
	// Invalid/missing rollouts should not start a daemon or touch a project.
	if stat, err := os.Stat(path); err != nil || !stat.Mode().IsRegular() {
		return printSessionResult(opt, out, stderr, sessionimport.Result{Skipped: true, Warnings: []string{"rollout_unavailable"}})
	}
	status, err := ensureInstance(ctx, opt)
	if err != nil {
		return printError(opt, out, stderr, "instance_unavailable", err, 1)
	}
	result, err := forwardSessionImport(ctx, status.instance, sessionImportRequest{RootPath: root, Rollout: path})
	if err != nil {
		return printError(opt, out, stderr, "session_import_failed", err, 1)
	}
	return printSessionResult(opt, out, stderr, result)
}

func printSessionResult(opt options, out, stderr io.Writer, result sessionimport.Result) int {
	for _, warning := range result.Warnings {
		fmt.Fprintln(stderr, "lumi session import: warning:", warning)
	}
	if opt.json {
		if err := json.NewEncoder(out).Encode(httpapi.Envelope{Success: true, Data: result}); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		fmt.Fprintf(out, "Imported: %d\nSkipped: %t\n", result.Imported, result.Skipped)
		if result.ThreadUUID != "" {
			fmt.Fprintln(out, "Thread:", result.ThreadUUID)
		}
	}
	return 0
}

func forwardSessionImport(ctx context.Context, identity instance, input sessionImportRequest) (sessionimport.Result, error) {
	var result sessionimport.Result
	body, err := json.Marshal(input)
	if err != nil {
		return result, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, identity.Endpoint+sessionImportsRoute, bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("If-Match", identity.Nonce)
	client := *localClient
	client.Timeout = 0
	resp, err := client.Do(r)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return result, errors.New("CLI instance requires an update; run lumi stop with the same environment/data-dir, then retry")
	}
	var envelope struct {
		Success bool                                    `json:"success"`
		Data    sessionimport.Result                    `json:"data"`
		Error   struct{ Code, Message, Details string } `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&envelope); err != nil {
		return result, err
	}
	if !envelope.Success || resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("%s: %s %s", envelope.Error.Code, envelope.Error.Message, envelope.Error.Details)
	}
	return envelope.Data, nil
}
