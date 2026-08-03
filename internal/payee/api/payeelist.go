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

// MovePayee handles POST /api/v1/payee/move-payee (auth).
//
// @Summary     Move a payee
// @Description Places one payee immediately after another (afterId null = first) and returns the full ordered list.
// @Tags        Payee
// @Accept      json
// @Produce     json
// @Param       request body     model.MovePayeeRequest true "Move payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.MovePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/move-payee [post]
func (h *Handlers) MovePayee(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.MovePayee)
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
	endpoint.HandleNoBody(w, r, h.read.GetPayeeList)
}
