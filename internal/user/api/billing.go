package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = model.CreateBillingLinkResult{}
var _ = model.GetBillingAccessResult{}

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
	endpoint.Handle(w, r, h.billing.CreateBillingLink)
}

var _ = apidoc.JsonResponseError{}

// GetBillingAccess handles GET /api/v1/user/get-billing-access (auth). Asks
// the payment portal for the caller's subscription view (cached per user) and
// never fails on billing's account: without billing, or when it does not
// answer, the result is {available: false}.
//
// @Summary     Get billing access
// @Description Returns the payment portal's view of the caller's subscription and of their connections' (paid, renewing). available is false, with user null and connections empty, when billing is not configured or did not answer.
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetBillingAccessResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/get-billing-access [get]
func (h *Handlers) GetBillingAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.billingAccess.GetBillingAccess)
}
