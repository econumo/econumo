package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/endpoint"
)

// GetSuggestionList handles GET /api/v1/import/get-suggestion-list (auth).
//
// @Summary     List import rule suggestions
// @Description Returns the caller's open AI rule suggestions with the imported transactions each would still fill, newest first.
// @Tags        Import
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportSuggestionListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-suggestion-list [get]
func (h *Handlers) GetSuggestionList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetSuggestionList)
}

// AcceptSuggestion handles POST /api/v1/import/accept-suggestion (auth).
//
// @Summary     Accept an import rule suggestion
// @Description After a rule was created from the suggestion, fills that rule's category and payee into the empty fields of the suggestion's transactions the rule matches, and closes the suggestion.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.AcceptImportSuggestionRequest true "Accept suggestion request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.AcceptImportSuggestionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/accept-suggestion [post]
func (h *Handlers) AcceptSuggestion(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.AcceptSuggestion)
}

// DismissSuggestion handles POST /api/v1/import/dismiss-suggestion (auth).
//
// @Summary     Dismiss an import rule suggestion
// @Description Deletes the suggestion and returns the remaining list. A later import from the same merchant may bring a new one.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.DismissImportSuggestionRequest true "Dismiss suggestion request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportSuggestionListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/dismiss-suggestion [post]
func (h *Handlers) DismissSuggestion(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DismissSuggestion)
}
