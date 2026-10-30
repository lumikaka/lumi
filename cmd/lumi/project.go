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
	"lumi/internal/httpapi"
	"lumi/internal/project"
)

const initializationRoute = "/api/v1/project-initializations"

type initializationRequest struct {
	RootPath string `json:"root_path"`
	Name     string `json:"name"`
	Language string `json:"generation_language"`
}

// This resource exists only on the CLI instance; desktop creation is unchanged.
func registerInitialization(e *echo.Echo, value instance, projects *project.Manager) {
	e.POST(initializationRoute, func(c echo.Context) error {
		if c.Request().Header.Get("If-Match") != value.Nonce {
			return httpapi.NewError(409, "cli_instance_changed", "CLI 实例已更换", "请重新读取 CLI 发现信息。", nil)
		}
		var input initializationRequest
		if err := c.Bind(&input); err != nil {
			return httpapi.NewError(400, "invalid_arguments", "初始化参数无效", "请使用 JSON 对象。", err)
		}
		result, err := projects.InitAt(c.Request().Context(), input.RootPath, project.CreateInput{Name: input.Name, GenerationLanguage: input.Language})
		if err != nil {
			var pe *project.Error
			if errors.As(err, &pe) && pe.Code == project.CodeInitializationConflict {
				return httpapi.NewError(409, pe.Code, pe.Message, pe.Details, err)
			}
			return httpapi.ProjectAPIError(err)
		}
		status := http.StatusOK
		if result.Created {
			status = http.StatusCreated
		}
		return httpapi.Success(c, status, result)
	})
}

type commandProjectInfo struct {
	project.DirectoryInfo
	Created         *bool           `json:"created,omitempty"`
	Guidance        *guidanceResult `json:"guidance,omitempty"`
	WriteProtection protectionInfo  `json:"write_protection"`
}

type projectRequestError struct{ Code, Message, Details string }

func (e *projectRequestError) Error() string { return e.Message }

func initializeThroughInstance(ctx context.Context, value instance, input initializationRequest) (project.InitResult, error) {
	var result project.InitResult
	body, err := json.Marshal(input)
	if err != nil {
		return result, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, value.Endpoint+initializationRoute, bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("If-Match", value.Nonce)
	// Initialization includes migrations and runtime hooks; use the command's
	// total deadline rather than the identity probe's two-second timeout.
	client := *localClient
	client.Timeout = 0
	response, err := client.Do(r)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return result, &projectRequestError{"cli_upgrade_required", "CLI 实例尚不支持原地初始化", "请用相同 environment / data-dir 执行 lumi stop 后重试。"}
	}
	var envelope struct {
		Success bool               `json:"success"`
		Data    project.InitResult `json:"data"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details string `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&envelope); err != nil {
		return result, err
	}
	if !envelope.Success || (response.StatusCode != 200 && response.StatusCode != 201) {
		return result, &projectRequestError{envelope.Error.Code, envelope.Error.Message, envelope.Error.Details}
	}
	return envelope.Data, nil
}

func projectCommand(ctx context.Context, name string, opt options, out, stderr io.Writer) int {
	root, err := os.Getwd()
	var result commandProjectInfo
	if err == nil && name == "init" {
		// Inspect locally first so existing projects can receive guidance even
		// while owned by the desktop, without running hooks or touching recents.
		result.DirectoryInfo, err = project.InspectDirectory(ctx, root)
		created := false
		if err != nil {
			var status instanceStatus
			status, err = ensureInstance(ctx, opt)
			if err == nil {
				var initialized project.InitResult
				initialized, err = initializeThroughInstance(ctx, status.instance, initializationRequest{RootPath: root, Name: opt.projectName, Language: opt.language})
				result.DirectoryInfo, created = initialized.DirectoryInfo, initialized.Created
			}
		}
		if err == nil {
			guidance := ensureGuidance(result.RootPath)
			if guidance.Config == "manual" {
				guidance.Messages = append(guidance.Messages, "请检查权限选择后手工合并以下配置（default_permissions 必须位于顶层）：\n"+manualProtectionConfig())
			}
			result.Guidance, result.Created = &guidance, &created
		}
	} else if err == nil {
		result.DirectoryInfo, err = project.InspectDirectory(ctx, root)
	}
	if err != nil {
		code, details := "project_operation_failed", ""
		var pe *project.Error
		var re *projectRequestError
		if errors.As(err, &pe) {
			code, details = pe.Code, pe.Details
		}
		if errors.As(err, &re) {
			code, details = re.Code, re.Details
		}
		if opt.json {
			_ = json.NewEncoder(out).Encode(map[string]any{"success": false, "data": nil, "error": map[string]string{"code": code, "message": err.Error(), "details": details}})
			return 1
		}
		if details != "" {
			err = fmt.Errorf("%w\n%s", err, details)
		}
		return printError(opt, out, stderr, code, err, 1)
	}
	result.WriteProtection = inspectProtection(result.RootPath)
	if opt.json {
		if err := json.NewEncoder(out).Encode(map[string]any{"success": true, "data": result}); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		fmt.Fprintf(out, "UUID: %s\nName: %s\nPath: %s\nOpen: %t\nCodex write protection: %s (configured: %t)\n%s\n", result.UUID, result.Name, result.RootPath, result.Open, result.WriteProtection.Effective, result.WriteProtection.Configured, result.WriteProtection.Reason)
		for _, warning := range result.WriteProtection.Warnings {
			fmt.Fprintln(out, warning)
		}
		if result.Guidance != nil {
			for _, message := range result.Guidance.Messages {
				fmt.Fprintln(out, message)
			}
		}
	}
	return 0
}
