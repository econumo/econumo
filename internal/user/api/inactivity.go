package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.UpdateInactivityPeriodResult{}

// UpdateInactivityPeriod handles POST /api/v1/user/update-inactivity-period
// (auth). Like update-ai it documents no 402: the route is on the read-only
// allowlist.
//
// @Summary     Update inactivity period
// @Description Sets after how many months without activity the authenticated user's data is deleted. months must be one of the user's inactivityChoices; while the server deletes no inactive accounts every value is refused. Returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateInactivityPeriodRequest true "Update inactivity period request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateInactivityPeriodResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-inactivity-period [post]
func (h *Handlers) UpdateInactivityPeriod(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateInactivityPeriod)
}
