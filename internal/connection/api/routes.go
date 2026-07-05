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

		mux.Handle("GET /api/v1/connection/get-connection-list", auth(h.GetConnectionList))
		mux.Handle("POST /api/v1/connection/set-account-access", auth(h.SetAccountAccess))
		mux.Handle("POST /api/v1/connection/revoke-account-access", auth(h.RevokeAccountAccess))

		mux.Handle("POST /api/v1/connection/generate-invite", auth(h.GenerateInvite))
		mux.Handle("POST /api/v1/connection/delete-invite", auth(h.DeleteInvite))
		mux.Handle("POST /api/v1/connection/accept-invite", auth(h.AcceptInvite))
		mux.Handle("POST /api/v1/connection/delete-connection", auth(h.DeleteConnection))
	}
}
