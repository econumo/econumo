package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// RequestUserDeletion handles POST /api/v1/user/request-user-deletion (auth).
//
// @Summary     Request deleting the user's data
// @Description Verifies the password (an empty one for a user without a password, or when password sign-in is disabled on the server) and emails a code that confirms the deletion. Returns an empty success envelope.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.RequestUserDeletionRequest true "Request user deletion request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.RequestUserDeletionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/request-user-deletion [post]
func (h *Handlers) RequestUserDeletion(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.RequestUserDeletionRequest) (*model.RequestUserDeletionResult, error) {
		return h.svc.RequestUserDeletion(ctx, userID, req)
	})
}

// ConfirmUserDeletion handles POST /api/v1/user/confirm-user-deletion (auth).
// The mark revokes every credential, the presenting session included, so this
// is the session's last successful request.
//
// @Summary     Confirm deleting the user's data
// @Description Confirms the deletion with the emailed code and marks the user for deletion, which signs them out everywhere. Signing in again before the wipe cancels it. Returns an empty success envelope.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.ConfirmUserDeletionRequest true "Confirm user deletion request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ConfirmUserDeletionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/confirm-user-deletion [post]
func (h *Handlers) ConfirmUserDeletion(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.ConfirmUserDeletionRequest) (*model.ConfirmUserDeletionResult, error) {
		return h.svc.ConfirmUserDeletion(ctx, userID, req)
	})
}
