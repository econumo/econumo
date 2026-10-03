package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// GetAuthorizationRequest handles GET /api/v1/authserver/get-authorization-request.
// A request with a verified redirect but a bad OAuth parameter answers 200 with
// errorRedirectUrl set; one whose client or redirect cannot be trusted is a 400.
//
// @Summary     Describe a pending MCP authorization request
// @Description Validates the OAuth parameters the consent page received and returns the app name and redirect host to show the user.
// @Tags        AuthServer
// @Produce     json
// @Param       clientId            query    string true  "Registered client id"
// @Param       redirectUri         query    string true  "Redirect URI"
// @Param       responseType        query    string true  "Must be code"
// @Param       codeChallenge       query    string true  "PKCE S256 challenge"
// @Param       codeChallengeMethod query    string true  "Must be S256"
// @Param       resource            query    string false "Resource indicator"
// @Param       scope               query    string false "Scope"
// @Param       state               query    string false "Opaque client state"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.AuthorizationRequestResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/authserver/get-authorization-request [get]
func (h *Handlers) GetAuthorizationRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := model.AuthorizationRequest{
		ClientID: q.Get("clientId"), RedirectURI: q.Get("redirectUri"), ResponseType: q.Get("responseType"),
		CodeChallenge: q.Get("codeChallenge"), CodeChallengeMethod: q.Get("codeChallengeMethod"),
		Resource: q.Get("resource"), Scope: q.Get("scope"), State: q.Get("state"),
	}
	res, err := h.svc.DescribeAuthorization(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

// ApproveAuthorization handles POST /api/v1/authserver/approve-authorization.
//
// @Summary     Approve an MCP authorization request
// @Description Issues a one-time authorization code for the client and returns the URL the browser must navigate to.
// @Tags        AuthServer
// @Accept      json
// @Produce     json
// @Param       request body     model.AuthorizationRequest true "Authorization request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.AuthorizationDecisionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/authserver/approve-authorization [post]
func (h *Handlers) ApproveAuthorization(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.AuthorizationRequest) (model.AuthorizationDecisionResult, error) {
		return h.svc.ApproveAuthorization(ctx, userID, req)
	})
}

// DeclineAuthorization handles POST /api/v1/authserver/decline-authorization.
//
// @Summary     Decline an MCP authorization request
// @Description Returns the URL that sends the access_denied error back to the client.
// @Tags        AuthServer
// @Accept      json
// @Produce     json
// @Param       request body     model.AuthorizationRequest true "Authorization request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.AuthorizationDecisionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/authserver/decline-authorization [post]
func (h *Handlers) DeclineAuthorization(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.AuthorizationRequest) (model.AuthorizationDecisionResult, error) {
		return h.svc.DeclineAuthorization(ctx, userID, req)
	})
}

// GetConnectedAppList handles GET /api/v1/authserver/get-connected-app-list.
//
// @Summary     List connected apps
// @Description Returns the apps the authenticated user has authorized to use their account over MCP.
// @Tags        AuthServer
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=[]model.ConnectedAppResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/authserver/get-connected-app-list [get]
func (h *Handlers) GetConnectedAppList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.ListConnectedApps)
}

// RevokeConnectedApp handles POST /api/v1/authserver/revoke-connected-app. A
// foreign, unknown or already revoked grant is the coded 400 grant_not_found.
//
// @Summary     Revoke a connected app
// @Description Revokes one connected app's grant and every access token issued under it. Returns an empty success envelope.
// @Tags        AuthServer
// @Accept      json
// @Produce     json
// @Param       request body     model.RevokeConnectedAppRequest true "Revoke connected app request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.RevokeConnectedAppResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/authserver/revoke-connected-app [post]
func (h *Handlers) RevokeConnectedApp(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.RevokeConnectedAppRequest) (model.RevokeConnectedAppResult, error) {
		id, err := vo.ParseId(req.ID)
		if err != nil {
			return model.RevokeConnectedAppResult{}, err
		}
		return model.RevokeConnectedAppResult{}, h.svc.RevokeConnectedApp(ctx, userID, id)
	})
}
