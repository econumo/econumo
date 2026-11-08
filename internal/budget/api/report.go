package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// GetFlowReport handles GET /api/v1/report/get-flow-report.
//
// @Summary  Monthly income/expense/transfer flow
// @Tags     Report
// @Produce  json
// @Param    budgetId query string false "Budget id (omit for the caller's own accounts)"
// @Param    from     query string false "First month (Y-m-d, snapped to first of month; defaults to the first transaction's month, at most 240 months back)"
// @Param    to       query string false "Last month (Y-m-d, snapped to first of month; defaults to the current month; window is at most 240 months)"
// @Param    groupBy  query string false "category (default), tag or payee; budget scope supports category only"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.GetFlowReportResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/report/get-flow-report [get]
func (h *Handlers) GetFlowReport(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := model.GetFlowReportRequest{BudgetId: q.Get("budgetId"), From: q.Get("from"), To: q.Get("to"), GroupBy: q.Get("groupBy")}
	res, err := h.svc.GetFlowReport(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

// GetNetWorthReport handles GET /api/v1/report/get-net-worth-report.
//
// @Summary  Account balances at the end of each month
// @Tags     Report
// @Produce  json
// @Param    budgetId query string false "Budget id (omit for the caller's own accounts)"
// @Param    from     query string false "First month (Y-m-d, snapped to first of month; defaults to the month before the first transaction's month, at most 240 months back)"
// @Param    to       query string false "Last month (Y-m-d, snapped to first of month; defaults to the current month; window is at most 240 months)"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.GetNetWorthReportResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/report/get-net-worth-report [get]
func (h *Handlers) GetNetWorthReport(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := model.GetNetWorthReportRequest{BudgetId: q.Get("budgetId"), From: q.Get("from"), To: q.Get("to")}
	res, err := h.svc.GetNetWorthReport(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}
