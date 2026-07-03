package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/endpoint"
	appuser "github.com/econumo/econumo/internal/user"
)

// _ keeps the apidoc and appuser import aliases visible to swag's annotation parser.
var (
	_ = apidoc.JsonResponseError{}
	_ = appuser.CompleteOnboardingResult{}
)

// CompleteOnboarding handles POST /api/v1/user/complete-onboarding (auth). No
// request fields; marks onboarding complete and returns the refreshed user.
//
// @Summary     Complete onboarding
// @Description Marks the authenticated user's onboarding complete and returns the refreshed user.
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=appuser.CompleteOnboardingResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/complete-onboarding [post]
func (h *Handlers) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.svc.CompleteOnboarding)
}
