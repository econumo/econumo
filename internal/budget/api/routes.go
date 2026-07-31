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

		mux.Handle("POST /api/v1/budget/create-budget", auth(h.CreateBudget))
		mux.Handle("POST /api/v1/budget/update-budget", auth(h.UpdateBudget))
		mux.Handle("POST /api/v1/budget/delete-budget", auth(h.DeleteBudget))
		mux.Handle("POST /api/v1/budget/reset-budget", auth(h.ResetBudget))
		mux.Handle("GET /api/v1/budget/get-budget", auth(h.GetBudget))
		mux.Handle("GET /api/v1/budget/get-budget-list", auth(h.GetBudgetList))
		mux.Handle("GET /api/v1/budget/get-transaction-list", auth(h.GetTransactionList))

		mux.Handle("POST /api/v1/budget/create-folder", auth(h.CreateFolder))
		mux.Handle("POST /api/v1/budget/update-folder", auth(h.UpdateFolder))
		mux.Handle("POST /api/v1/budget/delete-folder", auth(h.DeleteFolder))
		mux.Handle("POST /api/v1/budget/order-folder-list", auth(h.OrderFolderList))

		mux.Handle("POST /api/v1/budget/create-envelope", auth(h.CreateEnvelope))
		mux.Handle("POST /api/v1/budget/update-envelope", auth(h.UpdateEnvelope))
		mux.Handle("POST /api/v1/budget/delete-envelope", auth(h.DeleteEnvelope))

		mux.Handle("POST /api/v1/budget/grant-access", auth(h.GrantAccess))
		mux.Handle("POST /api/v1/budget/accept-access", auth(h.AcceptAccess))
		mux.Handle("POST /api/v1/budget/revoke-access", auth(h.RevokeAccess))
		mux.Handle("POST /api/v1/budget/decline-access", auth(h.DeclineAccess))

		mux.Handle("POST /api/v1/budget/exclude-account", auth(h.ExcludeAccount))
		mux.Handle("POST /api/v1/budget/include-account", auth(h.IncludeAccount))
		mux.Handle("POST /api/v1/budget/change-element-currency", auth(h.ChangeElementCurrency))
		mux.Handle("POST /api/v1/budget/set-limit", auth(h.SetLimit))
		mux.Handle("POST /api/v1/budget/move-element-list", auth(h.MoveElementList))
	}
}
