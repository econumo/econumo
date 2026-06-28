package tag

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/middleware"
	"github.com/econumo/econumo/internal/ui/router"
)

// RegisterAPI returns a router.RegisterAPI that mounts all 7 tag endpoints on
// the API mux with their exact API paths and methods. Every tag endpoint is
// authenticated, so each handler is wrapped in the JWT middleware (an
// absent/invalid token yields the 401 envelope before the handler runs).
//
// The router already wraps the whole /api subtree in the global chain
// (requestid -> recover -> cors -> timezone); RegisterAPI must not re-add it.
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
