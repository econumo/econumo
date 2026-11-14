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

		mux.Handle("POST /api/v1/onboarding/save-categories", auth(h.SaveCategories))
		mux.Handle("POST /api/v1/onboarding/save-accounts", auth(h.SaveAccounts))
		mux.Handle("POST /api/v1/onboarding/save-plan", auth(h.SavePlan))
		mux.Handle("POST /api/v1/onboarding/finish-import", auth(h.FinishImport))
		mux.Handle("GET /api/v1/onboarding/get-plan-suggestions", auth(h.GetPlanSuggestions))
		mux.Handle("GET /api/v1/onboarding/suggest-icon", auth(h.SuggestIcon))
		mux.Handle("POST /api/v1/onboarding/apply-suggested-icon", auth(h.ApplySuggestedIcon))
	}
}
