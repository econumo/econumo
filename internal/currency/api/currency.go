package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc and model import aliases visible to swag's annotation
// parser (a type reference's leading identifier must resolve to an import alias).
var (
	_ = apidoc.JsonResponseError{}
	_ = model.GetCurrencyListResult{}
	_ = model.GetCurrencyRateListResult{}
)

// GetCurrencyList handles GET /api/v1/currency/get-currency-list (auth). No
// request body; returns all currencies ordered by code.
//
// @Summary     Get the currency list
// @Description Returns all currencies ordered by ISO code. The name is the English display name.
// @Tags        Currency
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetCurrencyListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/get-currency-list [get]
func (h *Handlers) GetCurrencyList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.read.GetCurrencyList)
}

// GetCurrencyRateList handles GET /api/v1/currency/get-currency-rate-list
// (auth). No request body; returns every rate published on the most-recent date.
//
// @Summary     Get the latest currency rates
// @Description Returns all currency rates published on the most-recent available date.
// @Tags        Currency
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetCurrencyRateListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/get-currency-rate-list [get]
func (h *Handlers) GetCurrencyRateList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.read.GetCurrencyRateList)
}

// CreateCurrency handles POST /api/v1/currency/create-currency (auth).
//
// @Summary     Create a custom currency
// @Description Creates a per-user custom currency. Idempotent on the request id.
// @Tags        Currency
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateCurrencyRequest true "Create currency request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateCurrencyResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/create-currency [post]
func (h *Handlers) CreateCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.CreateCurrencyRequest) (*model.CreateCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_code", req.Code)
		return h.manage.CreateCurrency(ctx, userID, req)
	})
}

// UpdateCurrency handles POST /api/v1/currency/update-currency (auth).
//
// @Summary     Update a custom currency
// @Description Full replace of a custom currency's name, symbol and fraction digits. Requires ownership; the code is immutable.
// @Tags        Currency
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateCustomCurrencyRequest true "Update currency request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateCustomCurrencyResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/update-currency [post]
func (h *Handlers) UpdateCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.UpdateCustomCurrencyRequest) (*model.UpdateCustomCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.Id)
		return h.manage.UpdateCurrency(ctx, userID, req)
	})
}

// ArchiveCurrency handles POST /api/v1/currency/archive-currency (auth).
//
// @Summary     Archive a custom currency
// @Description Marks a custom currency archived. Requires ownership.
// @Tags        Currency
// @Accept      json
// @Produce     json
// @Param       request body     model.ArchiveCurrencyRequest true "Archive currency request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ArchiveCurrencyResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/archive-currency [post]
func (h *Handlers) ArchiveCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.ArchiveCurrencyRequest) (*model.ArchiveCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.Id)
		return h.manage.ArchiveCurrency(ctx, userID, req)
	})
}

// UnarchiveCurrency handles POST /api/v1/currency/unarchive-currency (auth).
//
// @Summary     Unarchive a custom currency
// @Description Clears a custom currency's archived flag. Requires ownership.
// @Tags        Currency
// @Accept      json
// @Produce     json
// @Param       request body     model.UnarchiveCurrencyRequest true "Unarchive currency request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UnarchiveCurrencyResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/unarchive-currency [post]
func (h *Handlers) UnarchiveCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.UnarchiveCurrencyRequest) (*model.UnarchiveCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.Id)
		return h.manage.UnarchiveCurrency(ctx, userID, req)
	})
}

// DeleteCurrency handles POST /api/v1/currency/delete-currency (auth).
//
// @Summary     Delete a custom currency
// @Description Deletes a custom currency. Requires ownership; fails if the currency is still referenced by an account, budget, budget element, or a user's profile currency.
// @Tags        Currency
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteCurrencyRequest true "Delete currency request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DeleteCurrencyResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/delete-currency [post]
func (h *Handlers) DeleteCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.DeleteCurrencyRequest) (*model.DeleteCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.Id)
		return h.manage.DeleteCurrency(ctx, userID, req)
	})
}

// SetCurrencyRate handles POST /api/v1/currency/set-currency-rate (auth).
//
// @Summary     Set a custom currency's rate
// @Description Upserts one dated rate for an owned custom currency against the instance base currency. Date defaults to today in the caller's timezone.
// @Tags        Currency
// @Accept      json
// @Produce     json
// @Param       request body     model.SetCurrencyRateRequest true "Set currency rate request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.SetCurrencyRateResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/set-currency-rate [post]
func (h *Handlers) SetCurrencyRate(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.SetCurrencyRateRequest) (*model.SetCurrencyRateResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.CurrencyId)
		return h.manage.SetCurrencyRate(ctx, userID, req)
	})
}

// HideCurrency handles POST /api/v1/currency/hide-currency (auth).
//
// @Summary     Hide a global currency
// @Description Removes a global currency from the caller's dropdowns. The base currency and the caller's own profile currency cannot be hidden.
// @Tags        Currency
// @Accept      json
// @Produce     json
// @Param       request body     model.HideCurrencyRequest true "Hide currency request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.HideCurrencyResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/hide-currency [post]
func (h *Handlers) HideCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.HideCurrencyRequest) (*model.HideCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.Id)
		return h.manage.HideCurrency(ctx, userID, req)
	})
}

// ShowCurrency handles POST /api/v1/currency/show-currency (auth).
//
// @Summary     Show a hidden global currency
// @Description Clears a caller's hide flag on a global currency.
// @Tags        Currency
// @Accept      json
// @Produce     json
// @Param       request body     model.ShowCurrencyRequest true "Show currency request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ShowCurrencyResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/show-currency [post]
func (h *Handlers) ShowCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.ShowCurrencyRequest) (*model.ShowCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.Id)
		return h.manage.ShowCurrency(ctx, userID, req)
	})
}
