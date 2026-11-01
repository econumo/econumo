package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
)

// RegisterAdmin registers the private admin routes. Deliberately NOT a
// router.RegisterAPI: that type feeds the public mux, and these routes must
// never be mounted there.
func RegisterAdmin(h *Handlers) func(mux *http.ServeMux) {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("POST /admin/set-access", h.SetAccess)
		mux.HandleFunc("GET /admin/user-context", h.UserContext)
		mux.HandleFunc("GET "+middleware.AdminShareStatusPath, h.UserStatus)
	}
}
