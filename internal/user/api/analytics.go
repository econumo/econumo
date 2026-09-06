package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc/model import aliases visible to swag's annotation
// parser (this file's handler body no longer references model types
// directly, since it delegates to a method value).
var _ = apidoc.JsonResponseError{}
var _ = model.UpdateAnalyticsResult{}

// UpdateAnalytics handles POST /api/v1/user/update-analytics (auth). It
// deliberately documents no 402 response: the route is in the read-only
// allowlist, since withdrawing consent to analytics is a privacy right, not a
// paid feature.
//
// @Summary     Update analytics preference
// @Description Enables or disables product analytics for the authenticated user and returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateAnalyticsRequest true "Update analytics request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateAnalyticsResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-analytics [post]
func (h *Handlers) UpdateAnalytics(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateAnalytics)
}
