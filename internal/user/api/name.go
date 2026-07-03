package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/endpoint"
	appuser "github.com/econumo/econumo/internal/user"
)

// _ keeps the apidoc/appuser import aliases visible to swag's annotation
// parser (this file's handler body no longer references appuser types
// directly, since it delegates to a method value).
var _ = apidoc.JsonResponseError{}
var _ = appuser.UpdateNameResult{}

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
	endpoint.Handle(w, r, h.dev, h.svc.UpdateName)
}
