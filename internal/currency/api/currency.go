package api

import (
	"net/http"

	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/internal/ui/middleware"
)

// _ keeps the apidoc and appcurrency import aliases visible to swag's annotation
// parser (a type reference's leading identifier must resolve to an import alias).
var (
	_ = apidoc.JsonResponseError{}
	_ = appcurrency.GetCurrencyListResult{}
)

// GetCurrencyList handles GET /api/v1/currency/get-currency-list (auth). No
// request body; returns all currencies ordered by code.
//
// @Summary     Get the currency list
// @Description Returns all currencies ordered by ISO code. The name is the English display name.
// @Tags        Currency
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=appcurrency.GetCurrencyListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/get-currency-list [get]
func (h *Handlers) GetCurrencyList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	res, err := h.read.GetCurrencyList(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// GetCurrencyRateList handles GET /api/v1/currency/get-currency-rate-list
// (auth). No request body; returns every rate published on the most-recent date.
//
// @Summary     Get the latest currency rates
// @Description Returns all currency rates published on the most-recent available date.
// @Tags        Currency
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=appcurrency.GetCurrencyRateListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/get-currency-rate-list [get]
func (h *Handlers) GetCurrencyRateList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	res, err := h.read.GetCurrencyRateList(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
