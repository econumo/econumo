package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/middleware"
	"github.com/econumo/econumo/internal/ui/router"
)

// RegisterAPI mounts the 7 tag endpoints, each wrapped in the JWT middleware.
func RegisterAPI(h *Handlers, verifier middleware.TokenVerifier, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		jwt := middleware.JWT(verifier, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return jwt(fn) }

		mux.Handle("POST /api/v1/tag/create-tag", auth(h.CreateTag))
		mux.Handle("POST /api/v1/tag/update-tag", auth(h.UpdateTag))
		mux.Handle("POST /api/v1/tag/archive-tag", auth(h.ArchiveTag))
		mux.Handle("POST /api/v1/tag/unarchive-tag", auth(h.UnarchiveTag))
		mux.Handle("POST /api/v1/tag/delete-tag", auth(h.DeleteTag))
		mux.Handle("POST /api/v1/tag/order-tag-list", auth(h.OrderTagList))
		mux.Handle("GET /api/v1/tag/get-tag-list", auth(h.GetTagList))
	}
}
