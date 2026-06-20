package connection

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/middleware"
	"github.com/econumo/econumo/internal/ui/router"
)

// RegisterAPI mounts all 7 connection endpoints on the API mux with their exact
// paths and methods, all JWT-protected. Four are self-hosted 501 stubs; three
// are live. The router already applies the global chain; RegisterAPI must not
// re-add it.
func RegisterAPI(h *Handlers, verifier middleware.TokenVerifier, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		jwt := middleware.JWT(verifier, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return jwt(fn) }

		// live
		mux.Handle("GET /api/v1/connection/get-connection-list", auth(h.GetConnectionList))
		mux.Handle("POST /api/v1/connection/set-account-access", auth(h.SetAccountAccess))
		mux.Handle("POST /api/v1/connection/revoke-account-access", auth(h.RevokeAccountAccess))

		// self-hosted 501 stubs
		mux.Handle("POST /api/v1/connection/generate-invite", auth(h.GenerateInvite))
		mux.Handle("POST /api/v1/connection/delete-invite", auth(h.DeleteInvite))
		mux.Handle("POST /api/v1/connection/accept-invite", auth(h.AcceptInvite))
		mux.Handle("POST /api/v1/connection/delete-connection", auth(h.DeleteConnection))
	}
}
