package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc and model import aliases visible to swag's annotation parser.
var (
	_ = apidoc.JsonResponseError{}
	_ = model.CompleteOnboardingResult{}
	_ = model.UpdateOnboardingResult{}
)

// CompleteOnboarding handles POST /api/v1/user/complete-onboarding (auth). No
// request fields; marks onboarding complete and returns the refreshed user.
//
// @Summary     Complete onboarding
// @Description Marks the authenticated user's onboarding complete and returns the refreshed user.
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.CompleteOnboardingResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     402 {object} apidoc.JsonResponseError
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/complete-onboarding [post]
func (h *Handlers) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.CompleteOnboarding)
}

// UpdateOnboarding handles POST /api/v1/user/update-onboarding (auth).
//
// @Summary     Update onboarding progress
// @Description Stores which onboarding steps the authenticated user finished or skipped ("started" = none yet) and returns the refreshed user. A completed onboarding is left unchanged.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateOnboardingRequest true "Update onboarding request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateOnboardingResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-onboarding [post]
func (h *Handlers) UpdateOnboarding(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateOnboarding)
}
