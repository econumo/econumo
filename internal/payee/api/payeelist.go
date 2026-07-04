package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc/model import aliases visible to swag's annotation
// parser (this file's handler bodies no longer reference these types
// directly, since they delegate to method values).
var _ = apidoc.JsonResponseError{}
var _ = model.GetPayeeListResult{}

// OrderPayeeList handles POST /api/v1/payee/order-payee-list (auth).
//
// @Summary     Reorder the payee list
// @Description Applies position changes to the user's payees and returns the full ordered list.
// @Tags        Payee
// @Accept      json
// @Produce     json
// @Param       request body     model.OrderPayeeListRequest true "Order payee list request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.OrderPayeeListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/order-payee-list [post]
func (h *Handlers) OrderPayeeList(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.OrderPayeeList)
}

// GetPayeeList handles GET /api/v1/payee/get-payee-list (auth). The request has
// no body, so there is nothing to decode; the handler returns the user's payees
// ordered by position.
//
// @Summary     Get the payee list
// @Description Returns all the authenticated user's payees (archived and not) ordered by position.
// @Tags        Payee
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetPayeeListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/get-payee-list [get]
func (h *Handlers) GetPayeeList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.read.GetPayeeList)
}
