package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// GetPersonalTokenList handles GET /api/v1/user/get-personal-token-list (auth).
// Returns the caller's live personal access tokens; the raw token string is
// never included (it is shown exactly once, in the create response).
//
// @Summary     List personal access tokens
// @Description Returns the authenticated user's live personal access tokens (without token material).
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=[]model.PersonalTokenItem}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/get-personal-token-list [get]
func (h *Handlers) GetPersonalTokenList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.ListPersonalTokens)
}

// CreatePersonalToken handles POST /api/v1/user/create-personal-token (auth).
// Mints a full-access personal token with an optional expiry; the response is
// the ONLY place the raw token ever appears.
//
// @Summary     Create a personal access token
// @Description Creates a personal access token. The token value is returned once and never again.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.CreatePersonalTokenRequest true "Create personal token request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreatePersonalTokenResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/create-personal-token [post]
func (h *Handlers) CreatePersonalToken(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreatePersonalToken)
}

// RevokePersonalToken handles POST /api/v1/user/revoke-personal-token (auth).
// Revokes one of the caller's personal tokens; a foreign or unknown id
// surfaces as the generic 400 error envelope ("Token not found").
//
// @Summary     Revoke a personal access token
// @Description Revokes one of the authenticated user's personal access tokens. Returns an empty success envelope.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.RevokePersonalTokenRequest true "Revoke personal token request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.RevokePersonalTokenResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/revoke-personal-token [post]
func (h *Handlers) RevokePersonalToken(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.RevokePersonalTokenRequest) (*model.RevokePersonalTokenResult, error) {
		reqctx.AddLogAttr(ctx, "token_id", req.Id)
		return h.svc.RevokePersonalToken(ctx, userID, req)
	})
}
