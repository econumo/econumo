package api

import (
	"net/http"

	apppayee "github.com/econumo/econumo/internal/payee"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}
var _ = apppayee.CreatePayeeResult{}

// CreatePayee handles POST /api/v1/payee/create-payee (auth).
//
// @Summary     Create a payee
// @Description Creates a payee for the authenticated user. Idempotent on the request id; the name must be unique among the user's payees.
// @Tags        Payee
// @Accept      json
// @Produce     json
// @Param       request body     apppayee.CreatePayeeRequest true "Create payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apppayee.CreatePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
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
// @Param       request body     apppayee.UpdatePayeeRequest true "Update payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apppayee.UpdatePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
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
// @Param       request body     apppayee.ArchivePayeeRequest true "Archive payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apppayee.ArchivePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
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
// @Param       request body     apppayee.UnarchivePayeeRequest true "Unarchive payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apppayee.UnarchivePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
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
// @Param       request body     apppayee.DeletePayeeRequest true "Delete payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=apppayee.DeletePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/delete-payee [post]
func (h *Handlers) DeletePayee(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.DeletePayee)
}
