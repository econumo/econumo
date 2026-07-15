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
		mux.Handle("POST /api/v1/currency/create-currency", auth(h.CreateCurrency))
		mux.Handle("POST /api/v1/currency/update-currency", auth(h.UpdateCurrency))
		mux.Handle("POST /api/v1/currency/archive-currency", auth(h.ArchiveCurrency))
		mux.Handle("POST /api/v1/currency/unarchive-currency", auth(h.UnarchiveCurrency))
		mux.Handle("POST /api/v1/currency/delete-currency", auth(h.DeleteCurrency))
		mux.Handle("POST /api/v1/currency/set-currency-rate", auth(h.SetCurrencyRate))
		mux.Handle("POST /api/v1/currency/hide-currency", auth(h.HideCurrency))
		mux.Handle("POST /api/v1/currency/show-currency", auth(h.ShowCurrency))
	}
}
