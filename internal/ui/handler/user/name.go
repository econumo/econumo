package user

import (
	"net/http"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}

// UpdateName handles POST /api/v1/user/update-name (auth). Validates the name
// (NotBlank, length 3..20) then updates it, returning the refreshed user.
//
// @Summary     Update name
// @Description Updates the authenticated user's display name (length 3..20) and returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     appuser.UpdateNameRequest true "Update name request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appuser.UpdateNameResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-name [post]
func (h *Handlers) UpdateName(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appuser.UpdateNameRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.UpdateName(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
