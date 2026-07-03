package api

import (
	"net/http"

	appconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = appconnection.GetConnectionListResult{}

// GetConnectionList handles GET /api/v1/connection/get-connection-list (auth).
//
// @Summary     List connections
// @Description Returns the user's connected users, each with the accounts shared between them.
// @Tags        Connection
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=appconnection.GetConnectionListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/get-connection-list [get]
func (h *Handlers) GetConnectionList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.svc.GetConnectionList)
}

// SetAccountAccess handles POST /api/v1/connection/set-account-access (auth).
//
// @Summary     Set account access
// @Description Grants or updates a connected user's role on an account you own or administer.
// @Tags        Connection
// @Accept      json
// @Produce     json
// @Param       request body     appconnection.SetAccountAccessRequest true "Set account access request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appconnection.SetAccountAccessResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/set-account-access [post]
func (h *Handlers) SetAccountAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.SetAccountAccess)
}

// RevokeAccountAccess handles POST /api/v1/connection/revoke-account-access (auth).
//
// @Summary     Revoke account access
// @Description Removes a connected user's grant on an account you own or administer.
// @Tags        Connection
// @Accept      json
// @Produce     json
// @Param       request body     appconnection.RevokeAccountAccessRequest true "Revoke account access request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appconnection.RevokeAccountAccessResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/revoke-account-access [post]
func (h *Handlers) RevokeAccountAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.RevokeAccountAccess)
}

// GenerateInvite handles POST /api/v1/connection/generate-invite (auth).
// Ported from EconumoCloudBundle: creates/refreshes the user's invite code.
//
// @Summary     Generate an invite
// @Description Generates (or refreshes) the user's connection invite code.
// @Tags        Connection
// @Accept      json
// @Produce     json
// @Param       request body     appconnection.GenerateInviteRequest false "Generate invite request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appconnection.GenerateInviteResult}
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/generate-invite [post]
func (h *Handlers) GenerateInvite(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.GenerateInvite)
}

// DeleteInvite handles POST /api/v1/connection/delete-invite (auth).
// Ported from EconumoCloudBundle: clears the user's outstanding invite.
//
// @Summary     Delete an invite
// @Description Clears the user's outstanding connection invite (no-op if none).
// @Tags        Connection
// @Accept      json
// @Produce     json
// @Param       request body     appconnection.DeleteInviteRequest false "Delete invite request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appconnection.DeleteInviteResult}
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/delete-invite [post]
func (h *Handlers) DeleteInvite(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.DeleteInvite)
}

// AcceptInvite handles POST /api/v1/connection/accept-invite (auth).
// Ported from EconumoCloudBundle: redeems a code and connects the two users.
//
// @Summary     Accept an invite
// @Description Redeems an invite code, connecting the user with the invite's owner.
// @Tags        Connection
// @Accept      json
// @Produce     json
// @Param       request body     appconnection.AcceptInviteRequest true "Accept invite request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appconnection.AcceptInviteResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/accept-invite [post]
func (h *Handlers) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.AcceptInvite)
}

// DeleteConnection handles POST /api/v1/connection/delete-connection (auth).
// Ported from EconumoCloudBundle: disconnects two users, revoking the account
// and budget access shared between them.
//
// @Summary     Delete a connection
// @Description Disconnects the user from a connected user, revoking shared access.
// @Tags        Connection
// @Accept      json
// @Produce     json
// @Param       request body     appconnection.DeleteConnectionRequest true "Delete connection request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appconnection.DeleteConnectionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/delete-connection [post]
func (h *Handlers) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.DeleteConnection)
}
