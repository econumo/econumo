package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/endpoint"
)

// GetSuggestionList handles GET /api/v1/import/get-suggestion-list (auth).
//
// @Summary     List import rule suggestions
// @Description Returns the caller's open AI rule suggestions, oldest first, each with the imported transactions it would still fill, newest first.
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

// SuggestMatchText handles POST /api/v1/import/suggest-match-text (auth).
//
// @Summary     Suggest rule match text for an imported transaction
// @Description Asks the AI provider for the shortest part of the payee that identifies the merchant and returns it only when the transaction really contains it; empty fields when no AI is configured or it had nothing usable. Counts against the preview-rule limit.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.SuggestImportMatchTextRequest true "Suggest match text request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.SuggestImportMatchTextResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     404     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/suggest-match-text [post]
func (h *Handlers) SuggestMatchText(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SuggestMatchText)
}

// GetTransactionSuggestionList handles GET /api/v1/import/get-transaction-suggestion-list (auth).
//
// @Summary     List per-transaction import suggestions
// @Description Returns the caller's open AI suggestions for single imported transactions, newest transaction first. Each carries only the category and payee Fill in would still write into an empty field. Empty while AI is off, the caller is read-only or category and payee suggestions are switched off.
// @Tags        Import
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportTransactionSuggestionListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-transaction-suggestion-list [get]
func (h *Handlers) GetTransactionSuggestionList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetTransactionSuggestionList)
}

// FillTransactionSuggestion handles POST /api/v1/import/fill-transaction-suggestion (auth).
//
// @Summary     Fill in a per-transaction import suggestion
// @Description Writes the suggested category and payee into the transaction's empty fields, never overwriting one, and settles the suggestion. A later apply-rule treats the filled values as a hand edit. updated is 0 when nothing was left to fill.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.FillImportTransactionSuggestionRequest true "Fill transaction suggestion request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.FillImportTransactionSuggestionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/fill-transaction-suggestion [post]
func (h *Handlers) FillTransactionSuggestion(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.FillTransactionSuggestion)
}

// DismissTransactionSuggestion handles POST /api/v1/import/dismiss-transaction-suggestion (auth).
//
// @Summary     Dismiss a per-transaction import suggestion
// @Description Deletes the suggestion without changing the transaction and returns the remaining list.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.DismissImportTransactionSuggestionRequest true "Dismiss transaction suggestion request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportTransactionSuggestionListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/dismiss-transaction-suggestion [post]
func (h *Handlers) DismissTransactionSuggestion(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DismissTransactionSuggestion)
}
