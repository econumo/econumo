package user

import (
	"net/http"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// _ keeps the apidoc import alias visible to swag's per-file annotation parser.
var _ = apidoc.JsonResponseError{}

// UpdatePassword handles POST /api/v1/user/update-password (auth). Validates
// oldPassword NotBlank + newPassword NotBlank/min4; the service verifies the old
// password (wrong -> 400) before storing the new hash. The result is {}.
//
// @Summary     Update password
// @Description Changes the authenticated user's password after verifying the old one. Returns an empty success envelope.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     appuser.UpdatePasswordRequest true "Update password request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appuser.UpdatePasswordResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-password [post]
func (h *Handlers) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appuser.UpdatePasswordRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.UpdatePassword(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
