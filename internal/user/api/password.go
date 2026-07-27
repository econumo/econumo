package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/middleware"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}

// UpdatePassword handles POST /api/v1/user/update-password (auth). Validates
// oldPassword NotBlank + newPassword NotBlank/min4; the service verifies the old
// password (wrong -> 400) before storing the new hash and revoking every OTHER
// session (the presenting one survives). The result is {}.
//
// @Summary     Update password
// @Description Changes the authenticated user's password after verifying the old one; other sessions are signed out. Returns an empty success envelope.
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
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.UpdatePasswordRequest) (*model.UpdatePasswordResult, error) {
		tokenID, _ := middleware.TokenIDFromCtx(ctx)
		return h.svc.UpdatePassword(ctx, userID, tokenID, req)
	})
}
