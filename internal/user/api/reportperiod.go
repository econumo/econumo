package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
	appuser "github.com/econumo/econumo/internal/user"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}

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
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appuser.UpdateReportPeriodRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.UpdateReportPeriod(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
