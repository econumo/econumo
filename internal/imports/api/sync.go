package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/endpoint"
)

// SyncSource handles POST /api/v1/import/sync-source (auth).
//
// @Summary     Pull transactions from a bank source
// @Description Fetches the date range from the provider using the client-decrypted access URL and runs every returned row through the import pipeline (create / match / queue / skip). Returns the run summary and the provider's accounts with their mapping state. endDate defaults to today; the span is capped at 400 days.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.SyncImportSourceRequest true "Sync request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.SyncImportSourceResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/sync-source [post]
func (h *Handlers) SyncSource(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.Sync)
}
