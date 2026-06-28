package payee

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/middleware"
	"github.com/econumo/econumo/internal/ui/router"
)

// RegisterAPI returns a router.RegisterAPI that mounts all 7 payee endpoints on
// the API mux with their exact API paths and methods. Every payee endpoint is
// authenticated, so each handler is wrapped in the JWT middleware (an
// absent/invalid token yields the 401 envelope before the handler runs).
//
// The router already wraps the whole /api subtree in the global chain
// (requestid -> recover -> cors -> timezone); RegisterAPI must not re-add it.
func RegisterAPI(h *Handlers, verifier middleware.TokenVerifier, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		jwt := middleware.JWT(verifier, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return jwt(fn) }

		mux.Handle("POST /api/v1/payee/create-payee", auth(h.CreatePayee))
		mux.Handle("POST /api/v1/payee/update-payee", auth(h.UpdatePayee))
		mux.Handle("POST /api/v1/payee/archive-payee", auth(h.ArchivePayee))
		mux.Handle("POST /api/v1/payee/unarchive-payee", auth(h.UnarchivePayee))
		mux.Handle("POST /api/v1/payee/delete-payee", auth(h.DeletePayee))
		mux.Handle("POST /api/v1/payee/order-payee-list", auth(h.OrderPayeeList))
		mux.Handle("GET /api/v1/payee/get-payee-list", auth(h.GetPayeeList))
	}
}
