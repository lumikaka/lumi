package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"lumi/internal/llm"
	"lumi/internal/provider"
	"lumi/internal/realtime"
	"lumi/internal/sitesettings"

	"github.com/labstack/echo/v4"
)

type ProviderHandler struct {
	providers *provider.Service
	client    llm.ConnectionChecker
	hub       *realtime.Hub
}

func NewProviderHandler(providers *provider.Service, client llm.ConnectionChecker, hubs ...*realtime.Hub) *ProviderHandler {
	var hub *realtime.Hub
	if len(hubs) > 0 {
		hub = hubs[0]
	}
	return &ProviderHandler{providers: providers, client: client, hub: hub}
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
	checkErr := handler.client.CheckConnection(c.Request().Context(), resolved.ProviderType, llm.Request{
		BaseURL: resolved.BaseURL, APIKey: resolved.APIKey, Model: resolved.DefaultModel,
	})
	if checkErr != nil {
		return llmAPIError(checkErr)
	}
	verified, err := handler.providers.MarkVerified(c.Request().Context(), resolved.UUID, resolved.ConfigFingerprint)
	if err != nil {
		return providerAPIError(err)
	}
	if handler.hub != nil {
		keys := []string{sitesettings.CloudflareVerifiedKey, sitesettings.CloudflareVerifiedAtKey, sitesettings.CloudflareVerifiedFingerprintKey}
		if verified.ProviderType == provider.TypeAliyunBailian {
			keys = []string{sitesettings.BailianVerifiedKey, sitesettings.BailianVerifiedAtKey, sitesettings.BailianVerifiedFingerprintKey}
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
		return NewError(http.StatusBadGateway, "provider_check_failed", "Provider 连接检查失败", "无法完成 Provider 请求。", err)
	}
	status := http.StatusBadGateway
	switch domainErr.Code {
	case llm.CodeNetwork, llm.CodeTimeout:
		status = http.StatusServiceUnavailable
	case llm.CodeRateLimited:
		status = http.StatusTooManyRequests
	case llm.CodeModelUnavailable, llm.CodeInvalidContent:
		status = http.StatusUnprocessableEntity
	case llm.CodeCancelled:
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
