package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

// RegisterAPI mounts the 7 tag endpoints, each wrapped in the JWT middleware.
func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("POST /api/v1/tag/create-tag", auth(h.CreateTag))
		mux.Handle("POST /api/v1/tag/update-tag", auth(h.UpdateTag))
		mux.Handle("POST /api/v1/tag/archive-tag", auth(h.ArchiveTag))
		mux.Handle("POST /api/v1/tag/unarchive-tag", auth(h.UnarchiveTag))
		mux.Handle("POST /api/v1/tag/delete-tag", auth(h.DeleteTag))
		mux.Handle("POST /api/v1/tag/order-tag-list", auth(h.OrderTagList))
		mux.Handle("GET /api/v1/tag/get-tag-list", auth(h.GetTagList))
	}
}
