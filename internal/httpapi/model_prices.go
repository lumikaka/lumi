package httpapi

import (
	"errors"
	"github.com/labstack/echo/v4"
	"lumi/internal/llmlog"
	"lumi/internal/pricing"
	"lumi/internal/project"
	"lumi/internal/provider"
	"lumi/internal/realtime"
	"net/http"
)

type ModelPriceHandler struct {
	providers *provider.Service
	prices    *pricing.Service
	projects  *project.Manager
	hub       *realtime.Hub
}

func NewModelPriceHandler(providers *provider.Service, projects *project.Manager, hub *realtime.Hub) *ModelPriceHandler {
	return &ModelPriceHandler{providers, providers.Prices(), projects, hub}
}
func priceAPIError(err error) error {
	var pe *project.Error
	if errors.As(err, &pe) {
		return projectAPIError(err)
	}
	if errors.Is(err, pricing.ErrInvalid) || errors.Is(err, llmlog.ErrInvalidFilter) {
		return NewError(http.StatusUnprocessableEntity, "model_price_invalid", "计价配置或筛选无效", "请检查模型、地域、币种、价格档位及所需用量。", err)
	}
	if errors.Is(err, pricing.ErrConflict) {
		return NewError(http.StatusConflict, "model_price_conflict", "价格版本已变化", "请刷新价格目录后重试。", err)
	}
	if errors.Is(err, pricing.ErrNotFound) || errors.Is(err, llmlog.ErrNotFound) {
		return NewError(http.StatusNotFound, "model_price_not_found", "计价资源不存在", "请刷新后重试。", err)
	}
	return NewError(http.StatusInternalServerError, "model_price_unavailable", "费用估算暂不可用", "本地计价数据读取或写入失败。", err)
}
func (h *ModelPriceHandler) Index(c echo.Context) error {
	items, err := h.prices.List(c.Request().Context())
	if err != nil {
		return priceAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": items})
}
func (h *ModelPriceHandler) Create(c echo.Context) error {
	var input struct {
		Rule         pricing.Rule `json:"rule"`
		ExpectedUUID string       `json:"expected_uuid"`
	}
	if err := decodeJSON(c, &input); err != nil {
		return err
	}
	p, err := h.providers.Get(c.Request().Context(), input.Rule.ProviderUUID)
	if err != nil {
		return providerAPIError(err)
	}
	if p.ProviderType != input.Rule.ProviderType {
		return priceAPIError(pricing.ErrInvalid)
	}
	item, err := h.prices.Create(c.Request().Context(), input.Rule, input.ExpectedUUID)
	if err != nil {
		return priceAPIError(err)
	}
	h.changed(item.UUID)
	return Success(c, http.StatusCreated, item)
}
func (h *ModelPriceHandler) Delete(c echo.Context) error {
	if err := h.prices.Delete(c.Request().Context(), c.Param("price_uuid")); err != nil {
		return priceAPIError(err)
	}
	h.changed(c.Param("price_uuid"))
	return Success(c, http.StatusOK, nil)
}
func (h *ModelPriceHandler) changed(id string) {
	if h.hub != nil {
		h.hub.Broadcast(realtime.SystemTopic, "model_price:changed", map[string]any{"price_uuid": id})
	}
}
func costQueryFilter(c echo.Context) llmlog.Filter {
	return llmlog.Filter{Scope: c.QueryParam("scope"), ProviderUUID: c.QueryParam("provider_uuid"), ProviderType: c.QueryParam("provider_type"), Model: c.QueryParam("model"), Scenario: c.QueryParam("scenario"), Status: c.QueryParam("status"), RequestType: c.QueryParam("request_type"), Keyword: c.QueryParam("keyword"), From: c.QueryParam("from"), To: c.QueryParam("to")}
}
func (h *ModelPriceHandler) Summary(c echo.Context) error {
	var out llmlog.CostSummary
	err := h.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var e error
		out, e = llmlog.NewService(store).CostSummary(c.Request().Context(), costQueryFilter(c))
		return e
	})
	if err != nil {
		return priceAPIError(err)
	}
	return Success(c, http.StatusOK, out)
}
func (h *ModelPriceHandler) Preview(c echo.Context) error {
	var input struct {
		Filter     llmlog.Filter `json:"filter"`
		PriceUUIDs []string      `json:"price_uuids"`
	}
	if err := decodeJSON(c, &input); err != nil {
		return err
	}
	if len(input.PriceUUIDs) == 0 || len(input.PriceUUIDs) > 100 {
		return priceAPIError(pricing.ErrInvalid)
	}
	prices := []pricing.Price{}
	for _, id := range input.PriceUUIDs {
		p, e := h.prices.Get(c.Request().Context(), id)
		if e != nil {
			return priceAPIError(e)
		}
		prices = append(prices, p)
	}
	var out llmlog.Backfill
	err := h.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var e error
		out, e = llmlog.NewService(store).PreviewBackfill(c.Request().Context(), input.Filter, prices)
		return e
	})
	if err != nil {
		return priceAPIError(err)
	}
	return Success(c, http.StatusCreated, out)
}
func (h *ModelPriceHandler) Apply(c echo.Context) error {
	var out llmlog.Backfill
	err := h.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var e error
		out, e = llmlog.NewService(store).ApplyBackfill(c.Request().Context(), c.Param("backfill_uuid"), h.hub)
		return e
	})
	if err != nil {
		return priceAPIError(err)
	}
	return Success(c, http.StatusOK, out)
}
func (h *ModelPriceHandler) Backfills(c echo.Context) error {
	var out []llmlog.Backfill
	err := h.projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
		var e error
		out, e = llmlog.NewService(store).ListBackfills(c.Request().Context())
		return e
	})
	if err != nil {
		return priceAPIError(err)
	}
	return Success(c, http.StatusOK, map[string]any{"items": out})
}
