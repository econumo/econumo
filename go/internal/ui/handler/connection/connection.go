package connection

import (
	"net/http"

	appconnection "github.com/econumo/econumo/internal/app/connection"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

var _ = apidoc.JsonResponseError{}

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
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	res, err := h.svc.GetConnectionList(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
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
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appconnection.SetAccountAccessRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.SetAccountAccess(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
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
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appconnection.RevokeAccountAccessRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.RevokeAccountAccess(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// GenerateInvite handles POST /api/v1/connection/generate-invite (auth).
// Self-hosted: not supported -> 501.
//
// @Summary     Generate an invite (not supported in self-hosted)
// @Tags        Connection
// @Produce     json
// @Success     501 {object} apidoc.JsonResponseError
// @Security    Bearer
// @Router      /api/v1/connection/generate-invite [post]
func (h *Handlers) GenerateInvite(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented(w, notImplementedMessage)
}

// DeleteInvite handles POST /api/v1/connection/delete-invite (auth).
// Self-hosted: not supported -> 501.
//
// @Summary     Delete an invite (not supported in self-hosted)
// @Tags        Connection
// @Produce     json
// @Success     501 {object} apidoc.JsonResponseError
// @Security    Bearer
// @Router      /api/v1/connection/delete-invite [post]
func (h *Handlers) DeleteInvite(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented(w, notImplementedMessage)
}

// AcceptInvite handles POST /api/v1/connection/accept-invite (auth).
// Self-hosted: not supported -> 501.
//
// @Summary     Accept an invite (not supported in self-hosted)
// @Tags        Connection
// @Produce     json
// @Success     501 {object} apidoc.JsonResponseError
// @Security    Bearer
// @Router      /api/v1/connection/accept-invite [post]
func (h *Handlers) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented(w, notImplementedMessage)
}

// DeleteConnection handles POST /api/v1/connection/delete-connection (auth).
// Self-hosted: not supported -> 501.
//
// @Summary     Delete a connection (not supported in self-hosted)
// @Tags        Connection
// @Produce     json
// @Success     501 {object} apidoc.JsonResponseError
// @Security    Bearer
// @Router      /api/v1/connection/delete-connection [post]
func (h *Handlers) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented(w, notImplementedMessage)
}
