package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

// RegisterAPI mounts the 6 transaction endpoints, all JWT-protected.
func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("POST /api/v1/transaction/create-transaction", auth(h.CreateTransaction))
		mux.Handle("POST /api/v1/transaction/update-transaction", auth(h.UpdateTransaction))
		mux.Handle("POST /api/v1/transaction/delete-transaction", auth(h.DeleteTransaction))
		mux.Handle("GET /api/v1/transaction/get-transaction-list", auth(h.GetTransactionList))
		mux.Handle("GET /api/v1/transaction/export-transaction-list", auth(h.ExportTransactionList))
		mux.Handle("POST /api/v1/transaction/import-transaction-list", auth(h.ImportTransactionList))
	}
}
