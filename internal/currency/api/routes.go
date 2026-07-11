package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("GET /api/v1/currency/get-currency-list", auth(h.GetCurrencyList))
		mux.Handle("GET /api/v1/currency/get-currency-rate-list", auth(h.GetCurrencyRateList))
	}
}
