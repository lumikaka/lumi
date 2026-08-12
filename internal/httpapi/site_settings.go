package httpapi

import (
	"errors"
	"net/http"
	"sort"

	"lumi/internal/provider"
	"lumi/internal/realtime"
	"lumi/internal/sitesettings"

	"github.com/labstack/echo/v4"
)

type SiteSettingsHandler struct {
	settings  *sitesettings.Service
	providers *provider.Service
	hub       *realtime.Hub
}

func NewSiteSettingsHandler(providers *provider.Service, hub *realtime.Hub) *SiteSettingsHandler {
	return &SiteSettingsHandler{settings: providers.Settings(), providers: providers, hub: hub}
}

func (handler *SiteSettingsHandler) Index(c echo.Context) error {
	response, err := handler.settings.List(c.Request().Context())
	if err != nil {
		return siteSettingsAPIError(err)
	}
	return Success(c, http.StatusOK, response)
}

func (handler *SiteSettingsHandler) Update(c echo.Context) error {
	var request struct {
		Settings map[string]any `json:"settings"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	if len(request.Settings) == 0 {
		return siteSettingsAPIError(&sitesettings.Error{Code: sitesettings.CodeInvalidSetting, Message: "全局设置无效", Details: "settings 不能为空。"})
	}
	activeValue, activating := request.Settings[sitesettings.ActiveProviderKey]
	if activating && len(request.Settings) != 1 {
		return siteSettingsAPIError(&sitesettings.Error{Code: sitesettings.CodeInvalidSetting, Message: "激活 Provider 失败", Details: "请先保存并验证 Provider，再单独设置 ai_provider.active。"})
	}
	var changed []string
	if activating {
		active, ok := activeValue.(string)
		if !ok || (active != provider.TypeCloudflareAIGateway && active != provider.TypeAliyunBailian) {
			return siteSettingsAPIError(&sitesettings.Error{Code: sitesettings.CodeInvalidSetting, Message: "激活 Provider 失败", Details: "ai_provider.active 只能设置为 cloudflare_ai_gateway 或 aliyun_bailian。"})
		}
		if _, err := handler.providers.Activate(c.Request().Context(), active); err != nil {
			return providerAPIError(err)
		}
		changed = []string{sitesettings.ActiveProviderKey}
	} else {
		_, updated, err := handler.settings.Update(c.Request().Context(), request.Settings)
		if err != nil {
			return siteSettingsAPIError(err)
		}
		changed = updated
	}
	response, err := handler.settings.List(c.Request().Context())
	if err != nil {
		return siteSettingsAPIError(err)
	}
	handler.broadcast(changed)
	return Success(c, http.StatusOK, response)
}

func (handler *SiteSettingsHandler) Reset(c echo.Context) error {
	var request struct {
		Keys []string `json:"keys"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	response, changed, err := handler.settings.Reset(c.Request().Context(), request.Keys)
	if err != nil {
		return siteSettingsAPIError(err)
	}
	handler.broadcast(changed)
	return Success(c, http.StatusOK, response)
}

func (handler *SiteSettingsHandler) broadcast(keys []string) {
	if len(keys) == 0 || handler.hub == nil {
		return
	}
	keys = append([]string(nil), keys...)
	sort.Strings(keys)
	handler.hub.Broadcast(realtime.SystemTopic, "site_settings:updated", map[string]any{"keys": keys})
}

func siteSettingsAPIError(err error) error {
	var domainErr *sitesettings.Error
	if !errors.As(err, &domainErr) {
		return NewError(http.StatusInternalServerError, sitesettings.CodeStorageFailed, "全局设置操作失败", "发生了未预期的本地存储错误。", err)
	}
	status := http.StatusUnprocessableEntity
	switch domainErr.Code {
	case sitesettings.CodeSecretUnavailable:
		status = http.StatusServiceUnavailable
	case sitesettings.CodeStorageFailed:
		status = http.StatusInternalServerError
	}
	return NewError(status, domainErr.Code, domainErr.Message, domainErr.Details, err)
}
