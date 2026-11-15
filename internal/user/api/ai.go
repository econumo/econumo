package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.UpdateAIResult{}

// UpdateAI handles POST /api/v1/user/update-ai (auth). Like update-analytics
// it documents no 402: the route is on the read-only allowlist.
//
// @Summary     Update AI preference
// @Description Enables or disables AI features for the authenticated user (nothing of theirs is sent to the AI provider while off) and returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateAIRequest true "Update AI request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateAIResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-ai [post]
func (h *Handlers) UpdateAI(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateAI)
}
