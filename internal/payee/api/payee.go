package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc/model import aliases visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}
var _ = model.CreatePayeeResult{}

// CreatePayee handles POST /api/v1/payee/create-payee (auth).
//
// @Summary     Create a payee
// @Description Creates a payee for the authenticated user. Idempotent on the request id; the name must be unique among the user's payees.
// @Tags        Payee
// @Accept      json
// @Produce     json
// @Param       request body     model.CreatePayeeRequest true "Create payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreatePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/create-payee [post]
func (h *Handlers) CreatePayee(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.CreatePayee)
}

// UpdatePayee handles POST /api/v1/payee/update-payee (auth).
//
// @Summary     Update a payee
// @Description Updates a payee's name. Requires ownership; the new name must be unique among the user's payees.
// @Tags        Payee
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdatePayeeRequest true "Update payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdatePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/update-payee [post]
func (h *Handlers) UpdatePayee(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdatePayee)
}

// ArchivePayee handles POST /api/v1/payee/archive-payee (auth).
//
// @Summary     Archive a payee
// @Description Marks a payee archived. Requires ownership.
// @Tags        Payee
// @Accept      json
// @Produce     json
// @Param       request body     model.ArchivePayeeRequest true "Archive payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ArchivePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/archive-payee [post]
func (h *Handlers) ArchivePayee(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.ArchivePayee)
}

// UnarchivePayee handles POST /api/v1/payee/unarchive-payee (auth).
//
// @Summary     Unarchive a payee
// @Description Clears a payee's archived flag. Requires ownership.
// @Tags        Payee
// @Accept      json
// @Produce     json
// @Param       request body     model.UnarchivePayeeRequest true "Unarchive payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UnarchivePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/unarchive-payee [post]
func (h *Handlers) UnarchivePayee(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UnarchivePayee)
}

// DeletePayee handles POST /api/v1/payee/delete-payee (auth).
//
// @Summary     Delete a payee
// @Description Deletes a payee. Transactions referencing it have their payee cleared. Requires ownership.
// @Tags        Payee
// @Accept      json
// @Produce     json
// @Param       request body     model.DeletePayeeRequest true "Delete payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DeletePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/delete-payee [post]
func (h *Handlers) DeletePayee(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.DeletePayee)
}
