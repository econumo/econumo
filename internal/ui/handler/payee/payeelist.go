package payee

import (
	"net/http"

	apppayee "github.com/econumo/econumo/internal/app/payee"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}

// OrderPayeeList handles POST /api/v1/payee/order-payee-list (auth).
//
// @Summary     Reorder the payee list
// @Description Applies position changes to the user's payees and returns the full ordered list.
// @Tags        Payee
// @Accept      json
// @Produce     json
// @Param       request body     apppayee.OrderPayeeListRequest true "Order payee list request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apppayee.OrderPayeeListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/order-payee-list [post]
func (h *Handlers) OrderPayeeList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req apppayee.OrderPayeeListRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.OrderPayeeList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// GetPayeeList handles GET /api/v1/payee/get-payee-list (auth). The request has
// no body, so there is nothing to decode; the handler returns the user's payees
// ordered by position.
//
// @Summary     Get the payee list
// @Description Returns all the authenticated user's payees (archived and not) ordered by position.
// @Tags        Payee
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=apppayee.GetPayeeListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/get-payee-list [get]
func (h *Handlers) GetPayeeList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	res, err := h.read.GetPayeeList(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
