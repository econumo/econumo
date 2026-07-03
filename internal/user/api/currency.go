package api

import (
	"net/http"

	appuser "github.com/econumo/econumo/internal/user"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc/appuser import aliases visible to swag's annotation
// parser (this file's handler body no longer references appuser types
// directly, since it delegates to a method value).
var _ = apidoc.JsonResponseError{}
var _ = appuser.UpdateCurrencyResult{}

// UpdateCurrency handles POST /api/v1/user/update-currency (auth). Tier-1
// validates NotBlank; the service's value-object constructor enforces the
// 3-char invariant and confirms the code resolves to a real currency.
//
// @Summary     Update currency
// @Description Updates the authenticated user's base currency and returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     appuser.UpdateCurrencyRequest true "Update currency request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appuser.UpdateCurrencyResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-currency [post]
func (h *Handlers) UpdateCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdateCurrency)
}
