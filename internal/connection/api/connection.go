package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.GetConnectionListResult{}

// GetConnectionList handles GET /api/v1/connection/get-connection-list (auth).
//
// @Summary     List connections
// @Description Returns the user's connected users, each with the accounts shared between them.
// @Tags        Connection
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetConnectionListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/get-connection-list [get]
func (h *Handlers) GetConnectionList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetConnectionList)
}

// GenerateInvite handles POST /api/v1/connection/generate-invite (auth).
// Ported from EconumoCloudBundle: creates/refreshes the user's invite code.
//
// @Summary     Generate an invite
// @Description Generates (or refreshes) the user's connection invite code.
// @Tags        Connection
// @Accept      json
// @Produce     json
// @Param       request body     model.GenerateInviteRequest false "Generate invite request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GenerateInviteResult}
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/generate-invite [post]
func (h *Handlers) GenerateInvite(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.GenerateInvite)
}

// DeleteInvite handles POST /api/v1/connection/delete-invite (auth).
// Ported from EconumoCloudBundle: clears the user's outstanding invite.
//
// @Summary     Delete an invite
// @Description Clears the user's outstanding connection invite (no-op if none).
// @Tags        Connection
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteInviteRequest false "Delete invite request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DeleteInviteResult}
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/delete-invite [post]
func (h *Handlers) DeleteInvite(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteInvite)
}

// AcceptInvite handles POST /api/v1/connection/accept-invite (auth).
// Ported from EconumoCloudBundle: redeems a code and connects the two users.
//
// @Summary     Accept an invite
// @Description Redeems an invite code, connecting the user with the invite's owner.
// @Tags        Connection
// @Accept      json
// @Produce     json
// @Param       request body     model.AcceptInviteRequest true "Accept invite request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.AcceptInviteResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/accept-invite [post]
func (h *Handlers) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.AcceptInvite)
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
// @Param       request body     model.DeleteConnectionRequest true "Delete connection request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DeleteConnectionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/connection/delete-connection [post]
func (h *Handlers) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteConnection)
}
