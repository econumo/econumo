package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("GET /api/v1/authserver/get-authorization-request", auth(h.GetAuthorizationRequest))
		mux.Handle("POST /api/v1/authserver/approve-authorization", auth(h.ApproveAuthorization))
		mux.Handle("POST /api/v1/authserver/decline-authorization", auth(h.DeclineAuthorization))
		mux.Handle("GET /api/v1/authserver/get-connected-app-list", auth(h.GetConnectedAppList))
		mux.Handle("POST /api/v1/authserver/revoke-connected-app", auth(h.RevokeConnectedApp))
	}
}
