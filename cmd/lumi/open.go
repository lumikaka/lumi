package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

func openProjectCommand(ctx context.Context, opt options, out, stderr io.Writer) int {
	root, err := os.Getwd()
	if err != nil {
		return printError(opt, out, stderr, "project_operation_failed", err, 1)
	}
	status, err := ensureInstance(ctx, opt)
	if err != nil {
		return printError(opt, out, stderr, "instance_unavailable", err, 1)
	}
	result := forwardCreation(ctx, status.instance, creationRequest{RootPath: root, Tool: "project_overview", Arguments: json.RawMessage(`{}`)})
	if !result.Success {
		return printError(opt, out, stderr, result.Error.Code, fmt.Errorf("%s: %s", result.Error.Message, result.Error.Details), 1)
	}
	data, _ := result.Data.(map[string]any)
	project, _ := data["project"].(map[string]any)
	uuid, _ := project["uuid"].(string)
	if uuid == "" {
		return printError(opt, out, stderr, "invalid_response", fmt.Errorf("CLI returned no project UUID"), 1)
	}
	address := status.Endpoint + "/projects/" + uuid + "/chapters"
	if opt.json {
		if err := json.NewEncoder(out).Encode(map[string]any{"success": true, "data": map[string]any{"url": address}}); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.CommandContext(ctx, "open", address)
	case "windows":
		command = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", address)
	default:
		command = exec.CommandContext(ctx, "xdg-open", address)
	}
	if err := command.Run(); err != nil {
		return printError(opt, out, stderr, "browser_open_failed", fmt.Errorf("%w; open %s", err, address), 1)
	}
	fmt.Fprintln(out, address)
	return 0
}
