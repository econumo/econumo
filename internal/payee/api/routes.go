package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

// RegisterAPI mounts the 7 payee endpoints, each wrapped in the auth middleware.
func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("POST /api/v1/payee/create-payee", auth(h.CreatePayee))
		mux.Handle("POST /api/v1/payee/update-payee", auth(h.UpdatePayee))
		mux.Handle("POST /api/v1/payee/archive-payee", auth(h.ArchivePayee))
		mux.Handle("POST /api/v1/payee/unarchive-payee", auth(h.UnarchivePayee))
		mux.Handle("POST /api/v1/payee/delete-payee", auth(h.DeletePayee))
		mux.Handle("POST /api/v1/payee/order-payee-list", auth(h.OrderPayeeList))
		mux.Handle("GET /api/v1/payee/get-payee-list", auth(h.GetPayeeList))
	}
}
