package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = model.CreateBillingLinkResult{}

// CreateBillingLink handles POST /api/v1/user/create-billing-link (auth). Mints
// a short-lived signed handoff token and returns the assembled portal URL.
// Reachable while read-only: a lapsed user needs this to restore their access.
//
// @Summary     Create billing link
// @Description Returns a payment portal URL carrying a short-lived signed identity assertion. Optionally preselects a beneficiary via "for". Returns 400 when the portal is not configured.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateBillingLinkRequest true "Create billing link request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateBillingLinkResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/create-billing-link [post]
func (h *Handlers) CreateBillingLink(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.billing.CreateBillingLink)
}

var _ = apidoc.JsonResponseError{}
