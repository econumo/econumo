package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.GetRecurringTransactionListResult{}
var _ = model.CreateRecurringTransactionResult{}
var _ = model.UpdateRecurringTransactionResult{}
var _ = model.DeleteRecurringTransactionResult{}

// GetRecurringTransactionList handles GET /api/v1/recurring/get-recurring-transaction-list (auth).
//
// @Summary     List recurring transactions
// @Description Returns every recurring transaction template on accounts the caller can access.
// @Tags        Recurring
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetRecurringTransactionListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/recurring/get-recurring-transaction-list [get]
func (h *Handlers) GetRecurringTransactionList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.svc.GetRecurringTransactionList)
}

// CreateRecurringTransaction handles POST /api/v1/recurring/create-recurring-transaction (auth).
//
// @Summary     Create a recurring transaction
// @Description Creates a recurring transaction template. Idempotent on the request id.
// @Tags        Recurring
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateRecurringTransactionRequest true "Create recurring transaction request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateRecurringTransactionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/recurring/create-recurring-transaction [post]
func (h *Handlers) CreateRecurringTransaction(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.CreateRecurringTransaction)
}

// UpdateRecurringTransaction handles POST /api/v1/recurring/update-recurring-transaction (auth).
//
// @Summary     Update a recurring transaction
// @Description Updates an existing recurring transaction template.
// @Tags        Recurring
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateRecurringTransactionRequest true "Update recurring transaction request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateRecurringTransactionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/recurring/update-recurring-transaction [post]
func (h *Handlers) UpdateRecurringTransaction(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdateRecurringTransaction)
}

// DeleteRecurringTransaction handles POST /api/v1/recurring/delete-recurring-transaction (auth).
//
// @Summary     Delete a recurring transaction
// @Description Deletes an existing recurring transaction template.
// @Tags        Recurring
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteRecurringTransactionRequest true "Delete recurring transaction request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DeleteRecurringTransactionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/recurring/delete-recurring-transaction [post]
func (h *Handlers) DeleteRecurringTransaction(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.DeleteRecurringTransaction)
}
