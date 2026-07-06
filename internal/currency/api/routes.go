package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

func RegisterAPI(h *Handlers, verifier middleware.TokenVerifier, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		jwt := middleware.JWT(verifier, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return jwt(fn) }

		mux.Handle("GET /api/v1/currency/get-currency-list", auth(h.GetCurrencyList))
		mux.Handle("GET /api/v1/currency/get-currency-rate-list", auth(h.GetCurrencyRateList))
	}
}
