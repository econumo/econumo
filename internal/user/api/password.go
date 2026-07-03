package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc/model import aliases visible to swag's annotation
// parser (this file's handler body no longer references model types
// directly, since it delegates to a method value).
var _ = apidoc.JsonResponseError{}
var _ = model.UpdatePasswordResult{}

// UpdatePassword handles POST /api/v1/user/update-password (auth). Validates
// oldPassword NotBlank + newPassword NotBlank/min4; the service verifies the old
// password (wrong -> 400) before storing the new hash. The result is {}.
//
// @Summary     Update password
// @Description Changes the authenticated user's password after verifying the old one. Returns an empty success envelope.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdatePasswordRequest true "Update password request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdatePasswordResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-password [post]
func (h *Handlers) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdatePassword)
}
