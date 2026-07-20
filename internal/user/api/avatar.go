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
var _ = model.UpdateAvatarResult{}

// UpdateAvatar handles POST /api/v1/user/update-avatar (auth). Validates the
// icon name format and color choice, then stores "<icon>:<color>" and returns
// the refreshed user.
//
// @Summary     Update avatar
// @Description Updates the authenticated user's avatar (Material icon name + color slug) and returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateAvatarRequest true "Update avatar request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateAvatarResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-avatar [post]
func (h *Handlers) UpdateAvatar(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdateAvatar)
}
