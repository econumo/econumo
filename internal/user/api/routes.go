package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

// RegisterAPI mounts the 21 user endpoints. The public group (login/register/
// remind/reset) is mounted bare; auth is applied per-handler so the public
// group stays unauthenticated.
func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		// Public group (no auth).
		mux.HandleFunc("POST /api/v1/user/login-user", h.LoginUser)
		mux.HandleFunc("POST /api/v1/user/register-user", h.RegisterUser)
		mux.HandleFunc("POST /api/v1/user/remind-password", h.RemindPassword)
		mux.HandleFunc("POST /api/v1/user/reset-password", h.ResetPassword)

		// Authenticated group.
		mux.Handle("POST /api/v1/user/logout-user", auth(h.LogoutUser))
		mux.Handle("GET /api/v1/user/get-session-list", auth(h.GetSessionList))
		mux.Handle("POST /api/v1/user/revoke-session", auth(h.RevokeSession))
		mux.Handle("POST /api/v1/user/revoke-other-sessions", auth(h.RevokeOtherSessions))
		mux.Handle("GET /api/v1/user/get-personal-token-list", auth(h.GetPersonalTokenList))
		mux.Handle("POST /api/v1/user/create-personal-token", auth(h.CreatePersonalToken))
		mux.Handle("POST /api/v1/user/revoke-personal-token", auth(h.RevokePersonalToken))
		mux.Handle("GET /api/v1/user/get-user-data", auth(h.GetUserData))
		mux.Handle("GET /api/v1/user/get-option-list", auth(h.GetOptionList))
		mux.Handle("POST /api/v1/user/update-budget", auth(h.UpdateBudget))
		mux.Handle("POST /api/v1/user/update-currency", auth(h.UpdateCurrency))
		mux.Handle("POST /api/v1/user/update-name", auth(h.UpdateName))
		mux.Handle("POST /api/v1/user/update-avatar", auth(h.UpdateAvatar))
		mux.Handle("POST /api/v1/user/update-password", auth(h.UpdatePassword))
		mux.Handle("POST /api/v1/user/create-billing-link", auth(h.CreateBillingLink))
		mux.Handle("POST /api/v1/user/update-report-period", auth(h.UpdateReportPeriod))
		mux.Handle("POST /api/v1/user/update-language", auth(h.UpdateLanguage))
		mux.Handle("POST /api/v1/user/complete-onboarding", auth(h.CompleteOnboarding))
	}
}
