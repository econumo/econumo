package user

import (
	"net/http"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// _ keeps the apidoc import alias visible to swag's per-file annotation parser.
var _ = apidoc.JsonResponseError{}

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
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appuser.UpdateCurrencyRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.UpdateCurrency(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
