package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc/model import aliases visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}
var _ = model.UpdateLastSeenVersionResult{}

// UpdateLastSeenVersion handles POST /api/v1/user/update-last-seen-version
// (auth). Like update-analytics it documents no 402: the route is in the
// read-only allowlist.
//
// @Summary     Update last seen version
// @Description Records the release (vX.Y.Z) whose "what's new" notice the authenticated user has seen and returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateLastSeenVersionRequest true "Update last seen version request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateLastSeenVersionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-last-seen-version [post]
func (h *Handlers) UpdateLastSeenVersion(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateLastSeenVersion)
}
