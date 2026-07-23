package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}

// RequestEmailChange handles POST /api/v1/user/request-email-change (auth).
// Validates newEmail Email/NotBlank + password NotBlank; the service verifies
// the password (wrong -> 400) and rejects newEmail == the current email before
// issuing a code to the new address. Returns an empty success envelope.
//
// @Summary     Request an email change
// @Description Starts an email change for the authenticated user after verifying the password; emails a confirmation code to the new address. Returns an empty success envelope.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.RequestEmailChangeRequest true "Request email change request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.RequestEmailChangeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/request-email-change [post]
func (h *Handlers) RequestEmailChange(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.RequestEmailChangeRequest) (*model.RequestEmailChangeResult, error) {
		return h.svc.RequestEmailChange(ctx, userID, req)
	})
}

// ConfirmEmailChange handles POST /api/v1/user/confirm-email-change (auth). It
// validates the emailed code (proof of ownership of the new address; no
// password) and swaps the user's email, returning the refreshed current user.
//
// @Summary     Confirm an email change
// @Description Confirms a pending email change with the code emailed to the new address and returns the refreshed current user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.ConfirmEmailChangeRequest true "Confirm email change request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CurrentUserResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/confirm-email-change [post]
func (h *Handlers) ConfirmEmailChange(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.ConfirmEmailChangeRequest) (model.CurrentUserResult, error) {
		tokenID, _ := middleware.TokenIDFromCtx(ctx)
		return h.svc.ConfirmEmailChange(ctx, userID, tokenID, req)
	})
}

// ResendEmailChangeCode handles POST /api/v1/user/resend-email-change-code
// (auth). It re-sends the confirmation code for the authenticated user's
// pending email change, at most once per cooldown.
//
// @Summary     Resend the email change code
// @Description Re-sends the pending email change's confirmation code, at most once per cooldown. The Retry-After header carries the seconds until another code may be requested.
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.ResendEmailChangeCodeResult} "Retry-After: seconds until another code may be requested"
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     402 {object} apidoc.JsonResponseError
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/resend-email-change-code [post]
//
// Hand-written rather than an endpoint.Handle* combinator: the cooldown
// travels on the Retry-After header (not in the body), and the combinators
// cannot set response headers. Mirrors ResendVerificationCode.
func (h *Handlers) ResendEmailChangeCode(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	res, retryAfter, err := h.svc.ResendEmailChangeCode(r.Context(), userID)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter/time.Second)))
	}
	httpx.OK(w, res)
}
