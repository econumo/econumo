package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.GrantAccountAccessResult{}

// GrantAccess handles POST /api/v1/account/grant-access (auth).
//
// @Summary     Grant account access
// @Description Grants or updates a connected user's role on an account you own or administer. New grants are pending until accepted.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     model.GrantAccountAccessRequest true "Grant account access request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GrantAccountAccessResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/grant-access [post]
func (h *Handlers) GrantAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.GrantAccess)
}

// AcceptAccess handles POST /api/v1/account/accept-access (auth).
//
// @Summary     Accept account access
// @Description Accepts a pending account-access grant, placing the account in the given folder (or a newly created default folder when the caller has none).
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     model.AcceptAccountAccessRequest true "Accept account access request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.AcceptAccountAccessResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/accept-access [post]
func (h *Handlers) AcceptAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.AcceptAccess)
}

// DeclineAccess handles POST /api/v1/account/decline-access (auth).
//
// @Summary     Decline account access
// @Description Removes the caller's own grant on an account, pending or accepted.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     model.DeclineAccountAccessRequest true "Decline account access request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DeclineAccountAccessResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/decline-access [post]
func (h *Handlers) DeclineAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.DeclineAccess)
}

// RevokeAccess handles POST /api/v1/account/revoke-access (auth).
//
// @Summary     Revoke account access
// @Description Removes a connected user's grant on an account you own or administer.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     model.RevokeAccountAccessRequest true "Revoke account access request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.RevokeAccountAccessResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/revoke-access [post]
func (h *Handlers) RevokeAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.RevokeAccess)
}
