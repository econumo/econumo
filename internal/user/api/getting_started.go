package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.UpdateGettingStartedResult{}

// UpdateGettingStarted handles POST /api/v1/user/update-getting-started (auth).
//
// @Summary     Update getting-started progress
// @Description Stores which get-ready steps the authenticated user finished or dismissed and returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateGettingStartedRequest true "Update getting-started request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateGettingStartedResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-getting-started [post]
func (h *Handlers) UpdateGettingStarted(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateGettingStarted)
}
