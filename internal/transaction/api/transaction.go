package api

import (
	"net/http"

	apptransaction "github.com/econumo/econumo/internal/transaction"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/endpoint"
)

// _ keeps the apidoc/apptransaction import aliases visible to swag's
// annotation parser (this file's handler bodies no longer reference
// apptransaction types directly, since they delegate to method values).
var _ = apidoc.JsonResponseError{}
var _ = apptransaction.CreateTransactionResult{}

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
	endpoint.Handle(w, r, h.dev, h.svc.CreateTransaction)
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
	endpoint.Handle(w, r, h.dev, h.svc.UpdateTransaction)
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
	endpoint.Handle(w, r, h.dev, h.svc.DeleteTransaction)
}
