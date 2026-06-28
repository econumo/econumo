package user

import (
	"net/http"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// _ keeps the apidoc import alias visible to swag's per-file annotation parser
// (the @Success {object} apidoc.* references below). No runtime effect.
var _ = apidoc.JsonResponseError{}

// LoginUser handles POST /api/v1/user/login-user (public). The handler decodes
// the credentials, calls Service.Login (which verifies the password, issues the
// JWT, and builds the current user), and returns {token, user}. Bad credentials
// surface as an *errs.UnauthorizedError -> 401.
//
// @Summary     Log in
// @Description Authenticates a user by username/password and returns a JWT plus the current user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     appuser.LoginRequest true "Login request"
// @Success     200     {object} appuser.LoginResult "Raw {token,user} body — NOT wrapped in the standard envelope (matches PHP login)."
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/login-user [post]
func (h *Handlers) LoginUser(w http.ResponseWriter, r *http.Request) {
	var req appuser.LoginRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.Login(r.Context(), req, h.now.Now())
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	// PHP's LoginUserV1Controller returns `new JsonResponse($result)` — the raw
	// {token,user} at the top level, NOT the {success,message,data} envelope. The
	// SPA reads response.token off the top level, so emit the body unwrapped.
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
// @Param       request body     appuser.RegisterRequest true "Register request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appuser.CurrentUserResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/register-user [post]
func (h *Handlers) RegisterUser(w http.ResponseWriter, r *http.Request) {
	var req appuser.RegisterRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.Register(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// LogoutUser handles POST /api/v1/user/logout-user (auth). JWT is stateless, so
// there is nothing to invalidate server-side; the handler just returns a success
// envelope. The result serializes as {} (see LogoutResult).
//
// @Summary     Log out
// @Description Stateless logout; returns an empty success envelope (JWT is not invalidated server-side).
// @Tags        User
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=appuser.LogoutResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/logout-user [post]
func (h *Handlers) LogoutUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	res, err := h.svc.Logout(r.Context())
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
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
// @Param       request body     appuser.RemindPasswordRequest true "Remind password request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appuser.RemindPasswordResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/remind-password [post]
func (h *Handlers) RemindPassword(w http.ResponseWriter, r *http.Request) {
	var req appuser.RemindPasswordRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.RemindPassword(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// ResetPassword handles POST /api/v1/user/reset-password (public). It validates
// the (email, code) pair, sets the new password, and consumes the code.
//
// @Summary     Reset password
// @Description Resets a user's password using a reminder code. Returns an empty success envelope.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     appuser.ResetPasswordRequest true "Reset password request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appuser.ResetPasswordResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/reset-password [post]
func (h *Handlers) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req appuser.ResetPasswordRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.ResetPassword(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
