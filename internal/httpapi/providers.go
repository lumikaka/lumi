package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/provider"
	"lumi/internal/providerlimit"
	"lumi/internal/realtime"
	"lumi/internal/sitesettings"

	"github.com/labstack/echo/v4"
)

type ProviderHandler struct {
	providers *provider.Service
	client    llm.ConnectionChecker
	images    imagegen.Client
	limiter   providerlimit.Limiter
	hub       *realtime.Hub
}

func NewProviderHandler(providers *provider.Service, client llm.ConnectionChecker, hubs ...*realtime.Hub) *ProviderHandler {
	var hub *realtime.Hub
	if len(hubs) > 0 {
		hub = hubs[0]
	}
	return &ProviderHandler{providers: providers, client: client, images: imagegen.NewOpenAICompatibleClient(nil), limiter: providerlimit.Noop(), hub: hub}
}

func (handler *ProviderHandler) WithImageClient(client imagegen.Client) *ProviderHandler {
	if client != nil {
		handler.images = client
	}
	return handler
}

func (handler *ProviderHandler) WithProviderLimiter(limiter providerlimit.Limiter) *ProviderHandler {
	if limiter != nil {
		handler.limiter = limiter
	}
	return handler
}

func (handler *ProviderHandler) Index(c echo.Context) error {
	items, err := handler.providers.List(c.Request().Context())
	if err != nil {
		return providerAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}

func (handler *ProviderHandler) Show(c echo.Context) error {
	item, err := handler.providers.Get(c.Request().Context(), c.Param("provider_uuid"))
	if err != nil {
		return providerAPIError(err)
	}
	return Success(c, http.StatusOK, item)
}

func (handler *ProviderHandler) Active(c echo.Context) error {
	item, err := handler.providers.Active(c.Request().Context())
	if err != nil {
		return providerAPIError(err)
	}
	return Success(c, http.StatusOK, item.Provider)
}

func (handler *ProviderHandler) Check(c echo.Context) error {
	resolved, err := handler.providers.Resolve(c.Request().Context(), c.Param("provider_uuid"))
	if err != nil {
		return providerAPIError(err)
	}
	checks := []struct {
		group   string
		request llm.Request
	}{
		{"文本组", llm.Request{ProviderType: resolved.ProviderType, BaseURL: resolved.BaseURL, APIKey: resolved.APIKey, Model: resolved.DefaultModel}},
	}
	if resolved.ProviderType == provider.TypeOpenAIRelay {
		checks = append(checks, struct {
			group   string
			request llm.Request
		}{"生图组", llm.Request{ProviderType: resolved.ProviderType, BaseURL: resolved.ImageBaseURL, APIKey: resolved.ImageAPIKey, Model: resolved.DefaultImageModel}})
	}
	for _, check := range checks {
		class := providerlimit.RequestText
		if check.group == "生图组" {
			class = providerlimit.RequestImage
		}
		lease, err := handler.limiter.Acquire(c.Request().Context(), resolved.ProviderType, check.request.BaseURL, check.request.Model, class)
		if err != nil {
			return llmAPIError(err)
		}
		var checkErr error
		if check.group == "生图组" {
			_, checkErr = handler.images.Generate(c.Request().Context(), imagegen.Request{
				ProviderType: resolved.ProviderType, ImageAPIMode: resolved.ImageAPIMode,
				BaseURL: check.request.BaseURL, APIKey: check.request.APIKey, Model: check.request.Model,
				Prompt: "A small blue circle on a plain white background, no text.", Size: "1024x1024", Quality: "low",
			})
		} else {
			checkErr = handler.client.CheckConnection(c.Request().Context(), resolved.ProviderType, check.request)
		}
		lease.Finish(checkErr)
		if checkErr != nil {
			apiErr := llmAPIError(checkErr)
			if resolved.ProviderType == provider.TypeOpenAIRelay {
				var detail *APIError
				if errors.As(apiErr, &detail) {
					upstreamCode := detail.Code
					detail.Code = "provider_text_check_failed"
					if check.group == "生图组" {
						detail.Code = "provider_image_check_failed"
					}
					detail.Message = check.group + "连接检查失败"
					detail.Details = check.group + "：" + upstreamCode + "\n" + detail.Details
				}
			}
			return apiErr
		}
	}
	verified, err := handler.providers.MarkVerified(c.Request().Context(), resolved.UUID, resolved.ConfigFingerprint)
	if err != nil {
		return providerAPIError(err)
	}
	if !verified.Verified {
		return NewError(http.StatusConflict, "provider_configuration_changed", "Provider 配置已变化", "请重新检查连接。", nil)
	}
	if handler.hub != nil {
		keys := []string{sitesettings.CloudflareVerifiedKey, sitesettings.CloudflareVerifiedAtKey, sitesettings.CloudflareVerifiedFingerprintKey}
		if verified.ProviderType == provider.TypeAliyunBailian {
			keys = []string{sitesettings.BailianVerifiedKey, sitesettings.BailianVerifiedAtKey, sitesettings.BailianVerifiedFingerprintKey}
		}
		if verified.ProviderType == provider.TypeOpenAIRelay {
			keys = []string{sitesettings.RelayVerifiedKey, sitesettings.RelayVerifiedAtKey, sitesettings.RelayVerifiedFingerprintKey}
		}
		handler.hub.Broadcast(realtime.SystemTopic, "site_settings:updated", map[string]any{"keys": keys})
	}
	return Success(c, http.StatusOK, map[string]any{"provider_uuid": verified.UUID, "status": "ok", "verified_at": verified.VerifiedAt})
}

func providerAPIError(err error) error {
	var domainErr *provider.Error
	if !errors.As(err, &domainErr) {
		return NewError(http.StatusInternalServerError, "provider_operation_failed", "Provider 操作失败", "发生了未预期的本地存储错误。", err)
	}
	status := http.StatusUnprocessableEntity
	switch domainErr.Code {
	case provider.CodeProviderNotFound:
		status = http.StatusNotFound
	case provider.CodeSecretMissing:
		status = http.StatusConflict
	case provider.CodeSecretStoreFailed:
		status = http.StatusServiceUnavailable
	case provider.CodeNoActiveProvider, provider.CodeProviderNotReady:
		status = http.StatusConflict
	}
	return NewError(status, domainErr.Code, domainErr.Message, domainErr.Details, err)
}

func llmAPIError(err error) error {
	var domainErr *llm.Error
	if !errors.As(err, &domainErr) {
		var imageErr *imagegen.Error
		if errors.As(err, &imageErr) {
			domainErr = &llm.Error{Code: imageErr.Code, SafeMessage: imageErr.SafeMessage, Diagnostic: imageErr.Diagnostic}
		} else {
			return NewError(http.StatusBadGateway, "provider_check_failed", "Provider 连接检查失败", "无法完成 Provider 请求。", err)
		}
	}
	status := http.StatusBadGateway
	switch domainErr.Code {
	case llm.CodeNetwork, llm.CodeTimeout, "image_network_error", "image_timeout", "image_provider_unavailable":
		status = http.StatusServiceUnavailable
	case llm.CodeRateLimited, imagegen.CodeRateLimited:
		status = http.StatusTooManyRequests
	case llm.CodeModelUnavailable, llm.CodeInvalidContent, "image_invalid_input":
		status = http.StatusUnprocessableEntity
	case llm.CodeCancelled, "image_cancelled":
		status = http.StatusRequestTimeout
	}
	details := []string{"请检查 Provider 配置后重试。"}
	diagnostic := domainErr.ProviderDiagnostic()
	if diagnostic.HTTPStatus > 0 {
		details = append(details, fmt.Sprintf("Provider HTTP status: %d", diagnostic.HTTPStatus))
	}
	if diagnostic.ProviderCode != "" {
		details = append(details, "Provider code: "+diagnostic.ProviderCode)
	}
	if diagnostic.Message != "" {
		details = append(details, "Provider message: "+diagnostic.Message)
	}
	if diagnostic.RequestID != "" {
		details = append(details, "Provider request ID: "+diagnostic.RequestID)
	}
	return NewError(status, domainErr.Code, domainErr.SafeMessage, strings.Join(details, "\n"), err)
}
