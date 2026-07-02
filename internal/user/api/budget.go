package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	appuser "github.com/econumo/econumo/internal/user"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}

// UpdateBudget handles POST /api/v1/user/update-budget (auth). The JSON field is
// "value" (a budget id); tier-1 validates NotBlank + Uuid, and the service
// confirms the budget exists (miss -> 400 "Plan not found") before setting it as
// the user's default budget. Returns the refreshed user.
//
// @Summary     Update budget
// @Description Sets the authenticated user's default budget and returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     appuser.UpdateBudgetRequest true "Update budget request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appuser.UpdateBudgetResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-budget [post]
func (h *Handlers) UpdateBudget(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appuser.UpdateBudgetRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.UpdateBudget(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
