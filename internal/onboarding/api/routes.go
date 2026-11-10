package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("POST /api/v1/onboarding/create-budget", auth(h.CreateBudget))
		mux.Handle("POST /api/v1/onboarding/add-budget-accounts", auth(h.AddBudgetAccounts))
		mux.Handle("POST /api/v1/onboarding/save-categories", auth(h.SaveCategories))
		mux.Handle("POST /api/v1/onboarding/save-accounts", auth(h.SaveAccounts))
		mux.Handle("POST /api/v1/onboarding/save-plan", auth(h.SavePlan))
	}
}
