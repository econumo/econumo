package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

// RegisterAPI mounts the 12 debt endpoints, each wrapped in the auth middleware.
func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("GET /api/v1/debt/get-debt-list", auth(h.GetDebtList))
		mux.Handle("GET /api/v1/debt/get-debt", auth(h.GetDebt))
		mux.Handle("GET /api/v1/debt/get-debt-forecast", auth(h.GetDebtForecast))
		mux.Handle("POST /api/v1/debt/create-debt", auth(h.CreateDebt))
		mux.Handle("POST /api/v1/debt/update-debt", auth(h.UpdateDebt))
		mux.Handle("POST /api/v1/debt/delete-debt", auth(h.DeleteDebt))
		mux.Handle("POST /api/v1/debt/mark-debt-paid-off", auth(h.MarkDebtPaidOff))
		mux.Handle("POST /api/v1/debt/reopen-debt", auth(h.ReopenDebt))
		mux.Handle("POST /api/v1/debt/reconcile-debt", auth(h.ReconcileDebt))
		mux.Handle("POST /api/v1/debt/delete-debt-reconciliation", auth(h.DeleteDebtReconciliation))
		mux.Handle("POST /api/v1/debt/add-debt-rate-change", auth(h.AddDebtRateChange))
		mux.Handle("POST /api/v1/debt/delete-debt-rate-change", auth(h.DeleteDebtRateChange))
	}
}
