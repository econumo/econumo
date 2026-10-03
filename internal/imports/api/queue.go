package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// GetQueuedEventList handles GET /api/v1/import/get-queued-event-list (auth).
//
// @Summary     Get the import queue
// @Description Returns the caller's queued events (waiting for a card mapping or a manual import), skipped events, and failed events (unparsable payloads awaiting retry/discard).
// @Tags        Import
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportQueueResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-queued-event-list [get]
func (h *Handlers) GetQueuedEventList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetQueue)
}

// ImportQueuedEvent handles POST /api/v1/import/import-queued-event (auth).
//
// @Summary     Import one queued event by hand
// @Description Creates the given transaction (same body as create-transaction, prefilled by the SPA from the queued event) and links the queued event to it.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ImportQueuedEventRequest true "Import queued event request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateTransactionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/import-queued-event [post]
func (h *Handlers) ImportQueuedEvent(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ImportQueuedEvent)
}

// SkipQueuedEvent handles POST /api/v1/import/skip-queued-event (auth).
//
// @Summary     Skip a queued event
// @Description Moves a queued event to the skipped list; a later push of the same event stays skipped.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ImportLinkActionRequest true "Skip request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportQueueResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/skip-queued-event [post]
func (h *Handlers) SkipQueuedEvent(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SkipQueuedEvent)
}

// UnskipQueuedEvent handles POST /api/v1/import/unskip-queued-event (auth).
//
// @Summary     Un-skip an event
// @Description Moves a skipped event back to the queue.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ImportLinkActionRequest true "Un-skip request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportQueueResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/unskip-queued-event [post]
func (h *Handlers) UnskipQueuedEvent(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UnskipQueuedEvent)
}

// RetryEvent handles POST /api/v1/import/retry-event (auth).
//
// @Summary     Retry a failed event
// @Description Re-runs the pipeline on a failed event's stored payload (after a normalizer fix, for instance). Returns the new ingest status.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.RetryImportEventRequest true "Retry request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.IngestEventResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/retry-event [post]
func (h *Handlers) RetryEvent(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.RetryEvent)
}

// DiscardEvent handles POST /api/v1/import/discard-event (auth).
//
// @Summary     Discard a failed event
// @Description Deletes a failed event and its payload for good.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.DiscardImportEventRequest true "Discard request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportQueueResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/discard-event [post]
func (h *Handlers) DiscardEvent(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DiscardEvent)
}

// GetTransactionImportList handles GET /api/v1/import/get-transaction-import-list (auth).
// Required query param: transactionId.
//
// @Summary     Get a transaction's import provenance
// @Description Returns the import ledger rows linked to one of the caller's visible transactions (source, external payee/amount/currency, posted and imported timestamps).
// @Tags        Import
// @Produce     json
// @Param       transactionId query    string true "Transaction id"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetTransactionImportListResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-transaction-import-list [get]
func (h *Handlers) GetTransactionImportList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	req := model.TransactionImportListRequest{TransactionId: r.URL.Query().Get("transactionId")}
	if err := req.Validate(); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.GetTransactionImportList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}
