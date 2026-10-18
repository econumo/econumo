package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.UpdateThemeResult{}

// UpdateTheme handles POST /api/v1/user/update-theme (auth).
//
// @Summary     Update theme preference
// @Description Saves system, light, or dark appearance for the authenticated user and returns the refreshed user. An absent preference defaults to system.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body model.UpdateThemeRequest true "Theme preference"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.UpdateThemeResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-theme [post]
func (h *Handlers) UpdateTheme(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateTheme)
}
