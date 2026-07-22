package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseError{}

// LoginUser handles POST /api/v1/user/login-user (public). The handler decodes
// the credentials, calls Service.Login (which verifies the password, mints an
// opaque session token, and builds the current user), and returns
// {token, user}. Bad credentials surface as an *errs.UnauthorizedError -> 401.
//
// @Summary     Log in
// @Description Authenticates a user by username/password and returns an opaque access token plus the current user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.LoginRequest true "Login request"
// @Success     200     {object} model.LoginResult "Raw {token,user} body — NOT wrapped in the standard envelope (matches PHP login)."
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     403     {object} apidoc.JsonResponseError "Email verification required (ECONUMO_EMAIL_VERIFICATION): confirm via /api/v1/user/confirm-email, then log in again."
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/login-user [post]
func (h *Handlers) LoginUser(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.Login(r.Context(), req, r.Header.Get("User-Agent"), h.now.Now())
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	// Record the user on this public route's operation log line (login has no
	// auth middleware to do it).
	reqctx.AddLogAttr(r.Context(), "user_id", res.User.Id)
	// Login returns the raw {token,user} at the top level, NOT the
	// {success,message,data} envelope — the SPA reads response.token off the top
	// level, so this frozen shape must stay unwrapped.
	httpx.Raw(w, res)
}

// RegisterUser handles POST /api/v1/user/register-user (public). Creates the
// user and returns the current user WITHOUT a token (distinct from login).
// Registration being disabled, or a duplicate email, surface as validation
// errors -> 400.
//
// @Summary     Register a user
// @Description Creates a new user (when registration is enabled) and returns the current user without a token.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.RegisterRequest true "Register request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CurrentUserResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/register-user [post]
func (h *Handlers) RegisterUser(w http.ResponseWriter, r *http.Request) {
	endpoint.HandlePublic(w, r, h.svc.Register)
}

// LogoutUser handles POST /api/v1/user/logout-user (auth). It revokes the
// session that authenticated this request, so the presented token stops
// working immediately.
//
// @Summary     Log out
// @Description Revokes the presenting session token.
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.LogoutResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/logout-user [post]
func (h *Handlers) LogoutUser(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, func(ctx context.Context, _ vo.Id) (*model.LogoutResult, error) {
		tokenID, _ := middleware.TokenIDFromCtx(ctx)
		return h.svc.Logout(ctx, tokenID)
	})
}

// RemindPassword handles POST /api/v1/user/remind-password (public). It issues a
// fresh reset code and emails it (when a mailer is configured), always returning
// success to avoid account enumeration.
//
// @Summary     Remind password
// @Description Triggers a password reminder email. Always returns success (anti-enumeration).
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.RemindPasswordRequest true "Remind password request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.RemindPasswordResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/remind-password [post]
func (h *Handlers) RemindPassword(w http.ResponseWriter, r *http.Request) {
	endpoint.HandlePublic(w, r, h.svc.RemindPassword)
}

// ResetPassword handles POST /api/v1/user/reset-password (public). It validates
// the (email, code) pair, sets the new password, and consumes the code.
//
// @Summary     Reset password
// @Description Resets a user's password using a reminder code. Returns an empty success envelope.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.ResetPasswordRequest true "Reset password request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ResetPasswordResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/reset-password [post]
func (h *Handlers) ResetPassword(w http.ResponseWriter, r *http.Request) {
	endpoint.HandlePublic(w, r, h.svc.ResetPassword)
}

// ConfirmEmail handles POST /api/v1/user/confirm-email (public). It validates
// the (email, code) pair and marks the email verified; unknown users and bad
// codes yield the same generic error (anti-enumeration).
//
// @Summary     Confirm email
// @Description Confirms a user's email with the emailed verification code (ECONUMO_EMAIL_VERIFICATION). Returns an empty success envelope.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.ConfirmEmailRequest true "Confirm email request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ConfirmEmailResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/confirm-email [post]
func (h *Handlers) ConfirmEmail(w http.ResponseWriter, r *http.Request) {
	endpoint.HandlePublic(w, r, h.svc.ConfirmEmail)
}

// ResendVerificationCode handles POST /api/v1/user/resend-verification-code
// (public). It re-sends the verification code to an unverified user, always
// returning success (anti-enumeration).
//
// @Summary     Resend verification code
// @Description Re-sends the email verification code. Always returns success (anti-enumeration).
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.ResendVerificationCodeRequest true "Resend verification code request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ResendVerificationCodeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/resend-verification-code [post]
func (h *Handlers) ResendVerificationCode(w http.ResponseWriter, r *http.Request) {
	endpoint.HandlePublic(w, r, h.svc.ResendVerificationCode)
}
