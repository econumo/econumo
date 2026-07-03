package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/endpoint"
	appuser "github.com/econumo/econumo/internal/user"
)

// _ keeps the apidoc/appuser import aliases visible to swag's annotation
// parser (this file's handler body no longer references appuser types
// directly, since it delegates to a method value).
var _ = apidoc.JsonResponseError{}
var _ = appuser.UpdateReportPeriodResult{}

// UpdateReportPeriod handles POST /api/v1/user/update-report-period (auth). The
// JSON field is "value"; tier-1 validates NotBlank, the service VO constructor
// enforces the allowed-period invariant.
//
// @Summary     Update report period
// @Description Updates the authenticated user's reporting period option and returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     appuser.UpdateReportPeriodRequest true "Update report period request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appuser.UpdateReportPeriodResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-report-period [post]
func (h *Handlers) UpdateReportPeriod(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdateReportPeriod)
}
