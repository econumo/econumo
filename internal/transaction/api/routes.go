package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/middleware"
	"github.com/econumo/econumo/internal/ui/router"
)

// RegisterAPI mounts the 6 transaction endpoints, all JWT-protected.
func RegisterAPI(h *Handlers, verifier middleware.TokenVerifier, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		jwt := middleware.JWT(verifier, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return jwt(fn) }

		mux.Handle("POST /api/v1/transaction/create-transaction", auth(h.CreateTransaction))
		mux.Handle("POST /api/v1/transaction/update-transaction", auth(h.UpdateTransaction))
		mux.Handle("POST /api/v1/transaction/delete-transaction", auth(h.DeleteTransaction))
		mux.Handle("GET /api/v1/transaction/get-transaction-list", auth(h.GetTransactionList))
		mux.Handle("GET /api/v1/transaction/export-transaction-list", auth(h.ExportTransactionList))
		mux.Handle("POST /api/v1/transaction/import-transaction-list", auth(h.ImportTransactionList))
	}
}
