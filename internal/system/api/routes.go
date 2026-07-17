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

		mux.Handle("GET /api/v1/system/get-update-info", auth(h.GetUpdateInfo))
	}
}
