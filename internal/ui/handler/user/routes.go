package user

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/middleware"
	"github.com/econumo/econumo/internal/ui/router"
)

// RegisterAPI returns a router.RegisterAPI that mounts all 13 user endpoints on
// the API mux with their exact API paths and methods. Public endpoints
// (login/register/remind/reset) are mounted bare; the eight authenticated
// endpoints are each wrapped in the JWT middleware so an absent/invalid token
// yields the 401 envelope before the handler runs.
//
// The router already wraps the whole /api subtree in the global chain
// (requestid -> recover -> cors -> timezone); RegisterAPI must not re-add it.
// JWT is applied here, per-handler, so the public group stays unauthenticated.
func RegisterAPI(h *Handlers, verifier middleware.TokenVerifier, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		jwt := middleware.JWT(verifier, dev)

		// auth wraps a handler func in the JWT middleware.
		auth := func(fn http.HandlerFunc) http.Handler { return jwt(fn) }

		// Public group (no JWT).
		mux.HandleFunc("POST /api/v1/user/login-user", h.LoginUser)
		mux.HandleFunc("POST /api/v1/user/register-user", h.RegisterUser)
		mux.HandleFunc("POST /api/v1/user/remind-password", h.RemindPassword)
		mux.HandleFunc("POST /api/v1/user/reset-password", h.ResetPassword)

		// Authenticated group (JWT-wrapped).
		mux.Handle("POST /api/v1/user/logout-user", auth(h.LogoutUser))
		mux.Handle("GET /api/v1/user/get-user-data", auth(h.GetUserData))
		mux.Handle("GET /api/v1/user/get-option-list", auth(h.GetOptionList))
		mux.Handle("POST /api/v1/user/update-budget", auth(h.UpdateBudget))
		mux.Handle("POST /api/v1/user/update-currency", auth(h.UpdateCurrency))
		mux.Handle("POST /api/v1/user/update-name", auth(h.UpdateName))
		mux.Handle("POST /api/v1/user/update-password", auth(h.UpdatePassword))
		mux.Handle("POST /api/v1/user/update-report-period", auth(h.UpdateReportPeriod))
		mux.Handle("POST /api/v1/user/complete-onboarding", auth(h.CompleteOnboarding))
	}
}
