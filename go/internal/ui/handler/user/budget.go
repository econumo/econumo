package user

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// _ keeps the apidoc import alias visible to swag's per-file annotation parser.
var _ = apidoc.JsonResponseError{}

// UpdateBudget handles POST /api/v1/user/update-budget (auth).
//
// TODO(budget-module): this endpoint sets the active budget on the user. That
// feature is not yet ported, and the CORE-phase user service exposes no
// UpdateBudget method, so
// this endpoint currently returns the frozen 501 envelope (NotImplemented)
// rather than silently succeeding. Wire it to the service once the budget
// module lands. The route is registered so the surface is complete and the 501
// is observable.
//
// @Summary     Update budget (not implemented)
// @Description Sets the authenticated user's active budget. Not yet implemented — currently returns a 501 NotImplemented envelope.
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Failure     501 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-budget [post]
func (h *Handlers) UpdateBudget(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	httpx.NotImplemented(w, "Not implemented")
}
