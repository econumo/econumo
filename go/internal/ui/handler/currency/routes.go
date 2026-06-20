package currency

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/middleware"
	"github.com/econumo/econumo/internal/ui/router"
)

// RegisterAPI returns a router.RegisterAPI that mounts the 2 currency endpoints
// on the API mux. Both are authenticated GET reads, so each handler is wrapped
// in the JWT middleware (an absent/invalid token yields the 401 envelope before
// the handler runs).
func RegisterAPI(h *Handlers, verifier middleware.TokenVerifier, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		jwt := middleware.JWT(verifier, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return jwt(fn) }

		mux.Handle("GET /api/v1/currency/get-currency-list", auth(h.GetCurrencyList))
		mux.Handle("GET /api/v1/currency/get-currency-rate-list", auth(h.GetCurrencyRateList))
	}
}
