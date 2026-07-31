package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc and model import aliases visible to swag's annotation
// parser (a type reference's leading identifier must resolve to an import alias).
var (
	_ = apidoc.JsonResponseError{}
	_ = model.GetUpdateInfoResult{}
)

// GetUpdateInfo handles GET /api/v1/system/get-update-info (auth). No request
// body; returns the latest release published on econumo.com, or empty strings
// when unknown (update checks disabled or the feed not fetched yet).
//
// @Summary     Get the latest published release
// @Description Returns the latest Econumo release published on econumo.com (version + release-page URL), or empty strings when unknown. The client compares against its own version.
// @Tags        System
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetUpdateInfoResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/system/get-update-info [get]
func (h *Handlers) GetUpdateInfo(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetUpdateInfo)
}
