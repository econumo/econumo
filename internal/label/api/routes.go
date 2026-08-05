package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

// RegisterAPI mounts the 8 label endpoints, each wrapped in the auth middleware.
func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("POST /api/v1/label/create-label", auth(h.CreateLabel))
		mux.Handle("POST /api/v1/label/update-label", auth(h.UpdateLabel))
		mux.Handle("POST /api/v1/label/archive-label", auth(h.ArchiveLabel))
		mux.Handle("POST /api/v1/label/unarchive-label", auth(h.UnarchiveLabel))
		mux.Handle("POST /api/v1/label/delete-label", auth(h.DeleteLabel))
		mux.Handle("POST /api/v1/label/move-label", auth(h.MoveLabel))
		mux.Handle("POST /api/v1/label/sort-label-list", auth(h.SortLabelList))
		mux.Handle("GET /api/v1/label/get-label-list", auth(h.GetLabelList))
	}
}
