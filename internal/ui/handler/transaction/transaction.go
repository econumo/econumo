package transaction

import (
	"net/http"

	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

var _ = apidoc.JsonResponseError{}

// CreateTransaction handles POST /api/v1/transaction/create-transaction (auth).
//
// @Summary     Create a transaction
// @Description Creates a transaction (idempotent on the request id) and returns it plus the refreshed account list.
// @Tags        Transaction
// @Accept      json
// @Produce     json
// @Param       request body     apptransaction.CreateTransactionRequest true "Create transaction request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apptransaction.CreateTransactionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/transaction/create-transaction [post]
func (h *Handlers) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req apptransaction.CreateTransactionRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.CreateTransaction(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// UpdateTransaction handles POST /api/v1/transaction/update-transaction (auth).
//
// @Summary     Update a transaction
// @Description Updates a transaction and returns it plus the refreshed account list.
// @Tags        Transaction
// @Accept      json
// @Produce     json
// @Param       request body     apptransaction.UpdateTransactionRequest true "Update transaction request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apptransaction.UpdateTransactionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/transaction/update-transaction [post]
func (h *Handlers) UpdateTransaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req apptransaction.UpdateTransactionRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.UpdateTransaction(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// DeleteTransaction handles POST /api/v1/transaction/delete-transaction (auth).
//
// @Summary     Delete a transaction
// @Description Deletes a transaction and returns it plus the refreshed account list.
// @Tags        Transaction
// @Accept      json
// @Produce     json
// @Param       request body     apptransaction.DeleteTransactionRequest true "Delete transaction request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apptransaction.DeleteTransactionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/transaction/delete-transaction [post]
func (h *Handlers) DeleteTransaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req apptransaction.DeleteTransactionRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.DeleteTransaction(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
