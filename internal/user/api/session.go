package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/middleware"
)

// GetSessionList handles GET /api/v1/user/get-session-list (auth). Returns the
// caller's live sessions; isCurrent marks the session whose token
// authenticated this request.
//
// @Summary     List sessions
// @Description Returns the authenticated user's live sessions; isCurrent marks the presenting one.
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=[]model.SessionItem}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/get-session-list [get]
func (h *Handlers) GetSessionList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, func(ctx context.Context, userID vo.Id) ([]model.SessionItem, error) {
		tokenID, _ := middleware.TokenIDFromCtx(ctx)
		return h.svc.ListSessions(ctx, userID, tokenID)
	})
}

// RevokeSession handles POST /api/v1/user/revoke-session (auth). Revokes one of
// the caller's sessions by id; a foreign or unknown id surfaces as the generic
// 400 error envelope ("Session not found"), the project-wide domain-not-found
// convention. Revoking the current session is allowed (equivalent to logout).
//
// @Summary     Revoke a session
// @Description Revokes one of the authenticated user's sessions. Returns an empty success envelope.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.RevokeSessionRequest true "Revoke session request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.RevokeSessionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/revoke-session [post]
func (h *Handlers) RevokeSession(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.RevokeSessionRequest) (*model.RevokeSessionResult, error) {
		reqctx.AddLogAttr(ctx, "session_id", req.Id)
		return h.svc.RevokeSession(ctx, userID, req)
	})
}

// RevokeOtherSessions handles POST /api/v1/user/revoke-other-sessions (auth).
// Signs the user out of every session except the presenting one.
//
// @Summary     Revoke other sessions
// @Description Revokes all the authenticated user's sessions except the presenting one. Returns an empty success envelope.
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.RevokeOtherSessionsResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/revoke-other-sessions [post]
func (h *Handlers) RevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, func(ctx context.Context, userID vo.Id) (*model.RevokeOtherSessionsResult, error) {
		tokenID, _ := middleware.TokenIDFromCtx(ctx)
		return h.svc.RevokeOtherSessions(ctx, userID, tokenID)
	})
}
