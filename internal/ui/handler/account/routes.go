package account

import (
	"net/http"

	"github.com/econumo/econumo/internal/ui/middleware"
	"github.com/econumo/econumo/internal/ui/router"
)

// RegisterAPI mounts all 12 account+folder endpoints (5 account + 7 folder) on
// the API mux with their exact paths and methods, all JWT-protected. The router
// already applies the global chain; RegisterAPI must not re-add it.
func RegisterAPI(h *Handlers, verifier middleware.TokenVerifier, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		jwt := middleware.JWT(verifier, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return jwt(fn) }

		// account
		mux.Handle("POST /api/v1/account/create-account", auth(h.CreateAccount))
		mux.Handle("POST /api/v1/account/update-account", auth(h.UpdateAccount))
		mux.Handle("POST /api/v1/account/delete-account", auth(h.DeleteAccount))
		mux.Handle("GET /api/v1/account/get-account-list", auth(h.GetAccountList))
		mux.Handle("POST /api/v1/account/order-account-list", auth(h.OrderAccountList))

		// folder
		mux.Handle("POST /api/v1/account/create-folder", auth(h.CreateFolder))
		mux.Handle("POST /api/v1/account/update-folder", auth(h.UpdateFolder))
		mux.Handle("POST /api/v1/account/hide-folder", auth(h.HideFolder))
		mux.Handle("POST /api/v1/account/show-folder", auth(h.ShowFolder))
		mux.Handle("POST /api/v1/account/replace-folder", auth(h.ReplaceFolder))
		mux.Handle("GET /api/v1/account/get-folder-list", auth(h.GetFolderList))
		mux.Handle("POST /api/v1/account/order-folder-list", auth(h.OrderFolderList))
	}
}
