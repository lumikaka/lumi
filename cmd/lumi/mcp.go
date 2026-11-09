package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
	"lumi/internal/jobqueue"
	"lumi/internal/project"
	"lumi/internal/realtime"
	"lumi/internal/textmcp"
)

const creationRoute = "/api/v1/creation-requests"

// Version 4 requires version-bound setting and page reviews, compact resume reads,
// and scoped exports. A v3 daemon accepts signed imports without review evidence.
const reviewedCreationVersion = 4

// Version 5 adds the app-only high-resolution image read.
const appImageDetailVersion = 5

// Version 6 adds the Claude-host page generation task and forwards --host.
// Only generate_page_image requires it; every other tool keeps v4/v5.
const creationProtocolVersion = 6

// root_path is an instance transport field, never part of any MCP schema.
type creationRequest struct {
	RootPath            string          `json:"root_path"`
	Tool                string          `json:"tool"`
	Arguments           json.RawMessage `json:"arguments"`
	ExpectedProjectUUID string          `json:"expected_project_uuid,omitempty"`
	ReadOnly            bool            `json:"read_only,omitempty"`
	Host                string          `json:"host,omitempty"`
}

func registerTextAuthoring(e *echo.Echo, identity instance, projects *project.Manager, events *realtime.Hub, tasks *jobqueue.Manager) func() {
	service := &textmcp.Service{Projects: projects, Events: events, Endpoint: identity.Endpoint}
	if tasks != nil {
		service.PageImages = pageImageTasks{tasks}
	}
	service.AppRequest = func(ctx context.Context, uuid string, input textmcp.AppResourceRequest) (any, error) {
		// Run the same router, middleware and handlers as the Web UI. The
		// caller has already leased and verified the cwd-bound project.
		if err := textmcp.ValidateAppResource(input); err != nil {
			return nil, err
		}
		var payload bytes.Buffer
		contentType := "application/json"
		if input.Resource == "asset-uploads" && input.Method == "POST" {
			var upload struct {
				Purpose     string `json:"purpose"`
				DisplayName string `json:"display_name"`
				FileName    string `json:"file_name"`
				Data        string `json:"upload_data"`
			}
			if err := json.Unmarshal(input.Body, &upload); err != nil {
				return nil, httpapi.NewError(400, "invalid_arguments", "文件元数据无效", "", err)
			}
			data, err := base64.StdEncoding.DecodeString(upload.Data)
			if err != nil || len(data) > 6<<20 {
				return nil, httpapi.NewError(400, "asset_too_large", "文件传输无效或超限", "单次支持 6 MiB。", err)
			}
			filename := path.Base(strings.ReplaceAll(upload.FileName, "\\", "/"))
			if filename == "." || filename == "/" || filename == "" {
				return nil, httpapi.NewError(400, "invalid_arguments", "文件名无效", "", nil)
			}
			form := multipart.NewWriter(&payload)
			_ = form.WriteField("purpose", upload.Purpose)
			_ = form.WriteField("display_name", upload.DisplayName)
			part, err := form.CreateFormFile("file", filename)
			if err != nil {
				return nil, err
			}
			if _, err = part.Write(data); err != nil {
				return nil, err
			}
			if err = form.Close(); err != nil {
				return nil, err
			}
			contentType = form.FormDataContentType()
		} else {
			payload.Write(input.Body)
		}
		request := httptest.NewRequest(input.Method, identity.Endpoint+input.Path(uuid), &payload).WithContext(ctx)
		if payload.Len() > 0 {
			request.Header.Set("Content-Type", contentType)
		}

		response := httptest.NewRecorder()
		e.ServeHTTP(response, request)
		var envelope httpapi.Envelope
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			return nil, httpapi.NewError(502, "app_response_invalid", "业务接口响应无效", "请在 Lumi 中打开。", err)
		}
		if !envelope.Success {
			if envelope.Error == nil {
				return nil, httpapi.NewError(502, "app_response_invalid", "业务接口响应无效", "缺少错误详情。", nil)
			}
			return nil, httpapi.NewError(response.Code, envelope.Error.Code, envelope.Error.Message, envelope.Error.Details, nil)
		}
		return envelope.Data, nil
	}
	service.AppConnection = func(uuid string) any { return appConnection(identity, uuid, time.Now()) }
	ws := realtime.NewWebSocketHandler(events, identity.Endpoint)
	// Only the CLI instance accepts scoped App tickets. Desktop Origin policy is unchanged.
	e.GET("/api/v1/ws", func(c echo.Context) error {
		if ticket := c.QueryParam("app_ticket"); ticket != "" {
			uuid, expires, err := verifyAppTicket(identity.Nonce, ticket, time.Now())
			if err != nil {
				return httpapi.NewError(403, "app_ticket_invalid", "实时连接票据失效", "重新打开审阅界面。", err)
			}
			return ws.ServeProjectApp(c, uuid, expires)
		}
		return ws.Serve(c)
	})
	e.POST(creationRoute, func(c echo.Context) error {
		if c.Request().Header.Get("If-Match") != identity.Nonce {
			return httpapi.NewError(409, "cli_instance_changed", "CLI 实例已更换", "重新读取 CLI 发现信息后重试。", nil)
		}
		// This CLI-only endpoint shares the existing local-user trust boundary.
		var input creationRequest
		decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, textmcp.MaxAppInputBytes+4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return httpapi.NewError(400, "invalid_arguments", "创作请求无效", "请求必须是有限大小的 JSON 对象。", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return httpapi.NewError(400, "invalid_arguments", "创作请求无效", "只能提交一个 JSON 对象。", err)
		}
		if !textmcp.ValidHost(input.Host) {
			return httpapi.NewError(400, "invalid_arguments", "创作请求宿主无效", "host 只能是 claude、codex 或 cindy。", nil)
		}
		ctx := textmcp.WithHost(textmcp.WithExpectedProjectUUID(c.Request().Context(), input.ExpectedProjectUUID), input.Host)
		if input.ReadOnly {
			return c.JSON(http.StatusOK, service.CallReadOnly(ctx, input.RootPath, input.Tool, input.Arguments))
		}
		return c.JSON(http.StatusOK, service.Call(ctx, input.RootPath, input.Tool, input.Arguments))
	})
	return service.PageReferences.Close
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
	protocol := textmcp.ProtocolFor(opt.host, func(ctx context.Context, tool string, args json.RawMessage) httpapi.Envelope {
		timeout := opt.timeout
		if tool == "generate_page_image" {
			// The instance waits up to PageImageWait for the task, then returns
			// its status; keep the transport open slightly longer than that.
			timeout += textmcp.PageImageWait
		}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		var status instanceStatus
		var err error
		if opt.readOnly {
			status, err = inspectLiveInstance(callCtx, opt)
		} else {
			status, err = ensureInstance(callCtx, opt)
		}
		if err != nil {
			return textmcp.Failure("instance_unavailable", "CLI 实例不可用", err.Error())
		}
		requiredVersion := reviewedCreationVersion
		if tool == "read_app_image_detail" {
			requiredVersion = appImageDetailVersion
		}
		if tool == "generate_page_image" {
			requiredVersion = creationProtocolVersion
		}
		if status.CreationProtocolVersion < requiredVersion {
			return textmcp.Failure("cli_upgrade_required", "共享 CLI 服务需要升级", fmt.Sprintf("当前共享服务创作协议 v%d，此操作要求 v%d。先协调同环境使用者，使用随包 CLI 执行 stop --environment %s --data-dir %q，再重连并重读进度。", status.CreationProtocolVersion, requiredVersion, opt.environment, opt.dataDir))
		}
		return forwardCreation(callCtx, status.instance, creationRequest{RootPath: root, Tool: tool, Arguments: args, ExpectedProjectUUID: textmcp.ExpectedProjectUUID(ctx), ReadOnly: opt.readOnly, Host: opt.host})
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
	resultLimit := textmcp.ResultLimit(input.Tool)
	b, err = io.ReadAll(io.LimitReader(resp.Body, int64(resultLimit+1)))
	if err != nil || len(b) > resultLimit {
		return textmcp.Failure("result_too_large", "CLI 结果超限或读取失败", "请缩小范围或分页读取。")
	}
	var result httpapi.Envelope
	if err := json.Unmarshal(b, &result); err != nil {
		return textmcp.Failure("instance_unavailable", "CLI 返回无效 JSON", fmt.Sprintf("HTTP %d", resp.StatusCode))
	}
	return result
}

// pageImageTasks adapts the shared production task manager. It creates the
// same comic image task as the Web UI; host mode only pins the prompt and
// selected references so the result matches prepare_page_image.
type pageImageTasks struct{ manager *jobqueue.Manager }

func publicPageImageTask(task jobqueue.ProductionTask) textmcp.PageImageTask {
	return textmcp.PageImageTask{UUID: task.UUID, ResourceUUID: task.ResourceUUID, Status: task.Status, Stage: task.Stage, Progress: task.Progress, ErrorCode: task.ErrorCode, ErrorMessage: task.ErrorMessage}
}

func (tasks pageImageTasks) CreatePageImage(ctx context.Context, input textmcp.PageImageRequest) (textmcp.PageImageTask, error) {
	selected := input.SettingUUIDs
	if selected == nil {
		selected = []string{}
	}
	task, err := tasks.manager.CreateComicImageGeneration(ctx, input.ProjectUUID, input.ChapterUUID, input.PageUUID, jobqueue.CreateProductionGenerationInput{Model: input.Model, Prompt: input.AdditionalDirection, PremiseAssetUUIDs: selected, IdempotencyKey: input.IdempotencyKey, HostPagePrompt: input.HostPagePrompt})
	if err != nil {
		return textmcp.PageImageTask{}, httpapi.ProductionAPIError(err)
	}
	return publicPageImageTask(task), nil
}

func (tasks pageImageTasks) FindPageImage(ctx context.Context, projectUUID, key string) (textmcp.PageImageTask, bool, error) {
	task, found, err := tasks.manager.FindProductionTask(ctx, projectUUID, jobqueue.KindComicImageGeneration, key)
	if err != nil {
		return textmcp.PageImageTask{}, false, httpapi.ProductionAPIError(err)
	}
	return publicPageImageTask(task), found, nil
}

func (tasks pageImageTasks) PageImage(ctx context.Context, projectUUID, taskUUID string) (textmcp.PageImageTask, error) {
	task, err := tasks.manager.GetProductionTask(ctx, projectUUID, taskUUID)
	if err != nil {
		return textmcp.PageImageTask{}, httpapi.ProductionAPIError(err)
	}
	return publicPageImageTask(task), nil
}
