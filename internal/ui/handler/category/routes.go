package category

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/middleware"
	"github.com/econumo/econumo/internal/ui/router"
)

// RegisterAPI returns a router.RegisterAPI that mounts all 7 category endpoints
// on the API mux with their exact API paths and methods. Every category endpoint
// is authenticated, so each handler is wrapped in the JWT middleware (an
// absent/invalid token yields the 401 envelope before the handler runs).
//
// The router already wraps the whole /api subtree in the global chain
// (requestid -> recover -> cors -> timezone); RegisterAPI must not re-add it.
func RegisterAPI(h *Handlers, verifier middleware.TokenVerifier, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		jwt := middleware.JWT(verifier, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return jwt(fn) }

		mux.Handle("POST /api/v1/category/create-category", auth(h.CreateCategory))
		mux.Handle("POST /api/v1/category/update-category", auth(h.UpdateCategory))
		mux.Handle("POST /api/v1/category/archive-category", auth(h.ArchiveCategory))
		mux.Handle("POST /api/v1/category/unarchive-category", auth(h.UnarchiveCategory))
		mux.Handle("POST /api/v1/category/delete-category", auth(h.DeleteCategory))
		mux.Handle("POST /api/v1/category/order-category-list", auth(h.OrderCategoryList))
		mux.Handle("GET /api/v1/category/get-category-list", auth(h.GetCategoryList))
	}
}
