package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.CreateImportSourceResult{}

// CreateSource handles POST /api/v1/import/create-source (auth).
//
// @Summary     Connect an import source
// @Description Creates an import source (currently only the "apple-wallet" provider) for the authenticated user. One source per provider per user.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateImportSourceRequest true "Create source request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateImportSourceResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/create-source [post]
func (h *Handlers) CreateSource(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreateSource)
}

// GetSourceList handles GET /api/v1/import/get-source-list (auth).
//
// @Summary     List import sources
// @Description Returns the caller's import sources with their cards (external accounts seen so far) and each card's mapping state.
// @Tags        Import
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportSourceListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-source-list [get]
func (h *Handlers) GetSourceList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetSourceList)
}

// DeleteSource handles POST /api/v1/import/delete-source (auth).
//
// @Summary     Disconnect an import source
// @Description Deletes a source and cascades its events, runs, account links and ledger rows. Imported transactions stay (they become plain transactions).
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteImportSourceRequest true "Delete source request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportSourceListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/delete-source [post]
func (h *Handlers) DeleteSource(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteSource)
}
