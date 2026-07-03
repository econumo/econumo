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
var _ = appuser.UpdateBudgetResult{}

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
	endpoint.Handle(w, r, h.dev, h.svc.UpdateBudget)
}
