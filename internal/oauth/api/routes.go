package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

// RegisterAPI mounts the nine oauth endpoints. The callbacks are one literal
// route per provider so the apiparity route scanner keeps them under guard.
func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		// Public group (no auth).
		mux.HandleFunc("GET /api/v1/oauth/get-provider-list", h.GetProviderList)
		mux.HandleFunc("POST /api/v1/oauth/start-login", h.StartLogin)
		mux.HandleFunc("GET /api/v1/oauth/callback-google", h.CallbackGoogle)
		mux.HandleFunc("GET /api/v1/oauth/callback-oidc", h.CallbackOIDC)
		mux.HandleFunc("POST /api/v1/oauth/callback-apple", h.CallbackApple)
		mux.HandleFunc("POST /api/v1/oauth/exchange-handoff", h.ExchangeHandoff)

		// Authenticated group.
		mux.Handle("POST /api/v1/oauth/start-link", auth(h.StartLink))
		mux.Handle("GET /api/v1/oauth/get-identity-list", auth(h.GetIdentityList))
		mux.Handle("POST /api/v1/oauth/unlink-identity", auth(h.UnlinkIdentity))
	}
}
