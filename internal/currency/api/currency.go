package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
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
	endpoint.HandleNoBody(w, r, h.read.GetCurrencyList)
}

// GetCurrencyRateList handles GET /api/v1/currency/get-currency-rate-list
// (auth). Without from/to it returns the latest rate per currency; with both,
// the rates published in that window plus the latest one before it.
//
// @Summary     Get currency rates
// @Description Returns the latest published rate per currency. With from and to (both, YYYY-MM-DD, at most 366 days) it returns every rate published within that window plus, per currency, the latest one published before it. Each item carries its published day as date.
// @Tags        Currency
// @Produce     json
// @Param       from query    string false "Start day (YYYY-MM-DD); requires to"
// @Param       to   query    string false "End day (YYYY-MM-DD), inclusive; requires from"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetCurrencyRateListResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/get-currency-rate-list [get]
func (h *Handlers) GetCurrencyRateList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := model.CurrencyRateListRequest{
		From: q.Get("from"), To: q.Get("to"),
		HasFrom: q.Has("from"), HasTo: q.Has("to"),
	}
	if err := req.Validate(); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	var (
		res *model.GetCurrencyRateListResult
		err error
	)
	if from, to, ranged := req.Range(); ranged {
		res, err = h.read.GetCurrencyRateHistory(r.Context(), userID, from, to)
	} else {
		res, err = h.read.GetCurrencyRateList(r.Context(), userID)
	}
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
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
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/create-currency [post]
func (h *Handlers) CreateCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.CreateCurrencyRequest) (*model.CreateCurrencyResult, error) {
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
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/update-currency [post]
func (h *Handlers) UpdateCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.UpdateCustomCurrencyRequest) (*model.UpdateCustomCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.Id)
		return h.manage.UpdateCurrency(ctx, userID, req)
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
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/delete-currency [post]
func (h *Handlers) DeleteCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.DeleteCurrencyRequest) (*model.DeleteCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.Id)
		return h.manage.DeleteCurrency(ctx, userID, req)
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
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/hide-currency [post]
func (h *Handlers) HideCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.HideCurrencyRequest) (*model.HideCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.Id)
		return h.manage.HideCurrency(ctx, userID, req)
	})
}

// HideAllCurrencies handles POST /api/v1/currency/hide-all-currencies (auth).
//
// @Summary     Hide all global currencies
// @Description Hides every global currency for the caller in one step; the base currency and the caller's profile currency stay visible. Custom currencies keep their per-currency preference.
// @Tags        Currency
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.HideAllCurrenciesResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     402 {object} apidoc.JsonResponseError
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/hide-all-currencies [post]
func (h *Handlers) HideAllCurrencies(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.manage.HideAllCurrencies)
}

// ShowAllCurrencies handles POST /api/v1/currency/show-all-currencies (auth).
//
// @Summary     Show all global currencies
// @Description Clears the caller's hidden flag on every global currency. Custom currencies keep their per-currency preference.
// @Tags        Currency
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.ShowAllCurrenciesResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     402 {object} apidoc.JsonResponseError
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/show-all-currencies [post]
func (h *Handlers) ShowAllCurrencies(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.manage.ShowAllCurrencies)
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
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/show-currency [post]
func (h *Handlers) ShowCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.ShowCurrencyRequest) (*model.ShowCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.Id)
		return h.manage.ShowCurrency(ctx, userID, req)
	})
}
