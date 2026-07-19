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

		mux.Handle("GET /api/v1/connection/get-connection-list", auth(h.GetConnectionList))

		mux.Handle("POST /api/v1/connection/generate-invite", auth(h.GenerateInvite))
		mux.Handle("POST /api/v1/connection/delete-invite", auth(h.DeleteInvite))
		mux.Handle("POST /api/v1/connection/accept-invite", auth(h.AcceptInvite))
		mux.Handle("POST /api/v1/connection/delete-connection", auth(h.DeleteConnection))
	}
}
