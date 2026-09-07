package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/endpoint"
)

// LinkAccount handles POST /api/v1/import/link-account (auth).
//
// @Summary     Map a card to an account
// @Description Maps an external account (card) of a source onto one of the caller's accounts and converts every queued event of that card in one run. Refuses a card whose reported currency differs from the account's.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.LinkImportAccountRequest true "Link account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateImportAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/link-account [post]
func (h *Handlers) LinkAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.LinkAccount)
}

// IgnoreAccount handles POST /api/v1/import/ignore-account (auth).
//
// @Summary     Ignore a card
// @Description Marks a card ignored: its queued events are dropped and future events for it are skipped on arrival.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ImportAccountActionRequest true "Ignore account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateImportAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/ignore-account [post]
func (h *Handlers) IgnoreAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.IgnoreAccount)
}

// UnlinkAccount handles POST /api/v1/import/unlink-account (auth).
//
// @Summary     Unmap a card
// @Description Removes a card's mapping (or its ignore flag). Already-imported transactions are untouched; new events for the card queue again.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ImportAccountActionRequest true "Unlink account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateImportAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/unlink-account [post]
func (h *Handlers) UnlinkAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UnlinkAccount)
}
