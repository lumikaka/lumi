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

	"github.com/labstack/echo/v4"
	"lumi/internal/cindyarchive"
	"lumi/internal/httpapi"
	"lumi/internal/project"
	"lumi/internal/realtime"
)

const cindySessionImportsRoute = "/api/v1/cindy-session-imports"

type cindySessionImportRequest struct {
	RootPath        string          `json:"root_path"`
	SourceSessionID string          `json:"source_session_id"`
	Export          json.RawMessage `json:"export"`
}

// This manual write route exists only on the nonce-protected local CLI server.
// The desktop exposes history reads; it never reads arbitrary Host tasks/files.
func registerCindySessionImports(e *echo.Echo, identity instance, projects *project.Manager, events *realtime.Hub) {
	service := &cindyarchive.Service{Projects: projects, Events: events}
	e.POST(cindySessionImportsRoute, func(c echo.Context) error {
		if c.Request().Header.Get("If-Match") != identity.Nonce {
			return httpapi.NewError(409, "cli_instance_changed", "CLI 实例已更换", "重新读取 CLI 发现信息后重试。", nil)
		}
		var input cindySessionImportRequest
		decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, cindyarchive.MaxExportBytes+(16<<10)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return httpapi.NewError(400, "invalid_arguments", "导入参数无效或超出大小限制", "", nil)
		}
		if decoder.Decode(new(any)) != io.EOF {
			return httpapi.NewError(400, "invalid_arguments", "只能提交一个 JSON 对象", "", nil)
		}
		result, err := service.ImportAt(c.Request().Context(), input.RootPath, input.SourceSessionID, bytes.NewReader(input.Export))
		if err != nil {
			return httpapi.CindyArchiveError(err)
		}
		return httpapi.Success(c, http.StatusOK, result)
	})
}

func importCindySessionCommand(ctx context.Context, opt options, out, stderr io.Writer) int {
	if opt.cindyExport == "" || opt.cindySource == "" {
		return printError(opt, out, stderr, "cindy_archive_selection_required", errors.New("use lumi cindy-session import --file PATH --source-session ID in the explicitly selected target project"), 2)
	}
	stat, err := os.Stat(opt.cindyExport)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > cindyarchive.MaxExportBytes {
		return printError(opt, out, stderr, "cindy_archive_read_failed", errors.New("export must be a regular file of at most 16 MiB"), 1)
	}
	f, err := os.Open(opt.cindyExport)
	if err != nil {
		return printError(opt, out, stderr, "cindy_archive_read_failed", errors.New("cannot read selected export file"), 1)
	}
	defer f.Close()
	stat, err = f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > cindyarchive.MaxExportBytes {
		return printError(opt, out, stderr, "cindy_archive_read_failed", errors.New("export must be a regular file of at most 16 MiB"), 1)
	}
	raw, err := io.ReadAll(io.LimitReader(f, cindyarchive.MaxExportBytes+1))
	if err != nil {
		return printError(opt, out, stderr, "cindy_archive_read_failed", errors.New("cannot read selected export file"), 1)
	}
	// Reject malformed or mismatched sources before starting a server or opening
	// a project. The server validates again at its own trust boundary.
	if _, err := cindyarchive.Parse(ctx, bytes.NewReader(raw), opt.cindySource); err != nil {
		return printError(opt, out, stderr, "cindy_archive_invalid", err, 1)
	}
	root, err := os.Getwd()
	if err != nil {
		return printError(opt, out, stderr, "cindy_archive_target_unavailable", errors.New("cannot select target project directory"), 1)
	}
	status, err := ensureInstance(ctx, opt)
	if err != nil {
		return printError(opt, out, stderr, "instance_unavailable", err, 1)
	}
	result, err := forwardCindySessionImport(ctx, status.instance, cindySessionImportRequest{RootPath: root, SourceSessionID: opt.cindySource, Export: raw})
	if err != nil {
		return printError(opt, out, stderr, "cindy_archive_import_failed", err, 1)
	}
	if opt.json {
		if json.NewEncoder(out).Encode(httpapi.Envelope{Success: true, Data: result}) != nil {
			return 1
		}
	} else {
		fmt.Fprintf(out, "Imported: %d\nReplayed: %t\nThread: %s\n", result.Imported, result.Replayed, result.ThreadUUID)
	}
	return 0
}

func forwardCindySessionImport(ctx context.Context, identity instance, input cindySessionImportRequest) (cindyarchive.Result, error) {
	var result cindyarchive.Result
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false) // Do not expand a bounded export's text in transit.
	if err := encoder.Encode(input); err != nil {
		return result, errors.New("invalid export transport")
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, identity.Endpoint+cindySessionImportsRoute, &body)
	if err != nil {
		return result, errors.New("invalid CLI endpoint")
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("If-Match", identity.Nonce)
	client := *localClient
	client.Timeout = 0
	resp, err := client.Do(r)
	if err != nil {
		return result, errors.New("CLI import request did not complete; retry the same file after checking the CLI instance")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return result, errors.New("CLI instance requires an update; run lumi stop with the same environment/data-dir, then retry")
	}
	var envelope struct {
		Success bool                  `json:"success"`
		Data    cindyarchive.Result   `json:"data"`
		Error   struct{ Code string } `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&envelope) != nil {
		return result, errors.New("invalid CLI import response")
	}
	if !envelope.Success || resp.StatusCode != http.StatusOK {
		// Only locally defined codes can reach terminal/logs, never server text.
		if envelope.Error.Code == "cli_instance_changed" || stringsHasArchiveCode(envelope.Error.Code) {
			return result, errors.New(envelope.Error.Code)
		}
		return result, errors.New("cindy_archive_import_failed")
	}
	return envelope.Data, nil
}

func stringsHasArchiveCode(code string) bool {
	// Do not echo arbitrary untrusted values returned by an outdated server.
	for _, r := range code {
		if (r < 'a' || r > 'z') && r != '_' {
			return false
		}
	}
	return len(code) <= 80 && len(code) > len("cindy_archive_") && code[:len("cindy_archive_")] == "cindy_archive_"
}
