package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

var _ = apidoc.JsonResponseError{}

// CreateBudget handles POST /api/v1/budget/create-budget.
//
// @Summary  Create a budget
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body model.CreateBudgetRequest true "Create budget"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.CreateBudgetResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/budget/create-budget [post]
func (h *Handlers) CreateBudget(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.CreateBudget)
}

// UpdateBudget handles POST /api/v1/budget/update-budget.
//
// @Summary  Update a budget
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body model.UpdateBudgetRequest true "Update budget"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.UpdateBudgetResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router   /api/v1/budget/update-budget [post]
func (h *Handlers) UpdateBudget(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdateBudget)
}

// DeleteBudget handles POST /api/v1/budget/delete-budget.
//
// @Summary  Delete a budget
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body model.DeleteBudgetRequest true "Delete budget"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.DeleteBudgetResult}
// @Security Bearer
// @Router   /api/v1/budget/delete-budget [post]
func (h *Handlers) DeleteBudget(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.DeleteBudget)
}

// ResetBudget handles POST /api/v1/budget/reset-budget.
//
// @Summary  Reset a budget
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body model.ResetBudgetRequest true "Reset budget"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.ResetBudgetResult}
// @Security Bearer
// @Router   /api/v1/budget/reset-budget [post]
func (h *Handlers) ResetBudget(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.ResetBudget)
}

// GetBudget handles GET /api/v1/budget/get-budget.
//
// @Summary  Get a budget
// @Tags     Budget
// @Produce  json
// @Param    id    query string true  "Budget id"
// @Param    date  query string false "Period date (Y-m-d)"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.GetBudgetResult}
// @Security Bearer
// @Router   /api/v1/budget/get-budget [get]
func (h *Handlers) GetBudget(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	req := model.GetBudgetRequest{Id: r.URL.Query().Get("id"), Date: r.URL.Query().Get("date")}
	res, err := h.svc.GetBudget(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// GetTransactionList handles GET /api/v1/budget/get-transaction-list.
//
// @Summary  Budget transaction list
// @Tags     Budget
// @Produce  json
// @Param    budgetId    query string true  "Budget id"
// @Param    periodStart query string true  "Period start (Y-m-d)"
// @Param    categoryId  query string false "Category id"
// @Param    tagId       query string false "Tag id"
// @Param    envelopeId  query string false "Envelope id"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.GetBudgetTransactionListResult}
// @Security Bearer
// @Router   /api/v1/budget/get-transaction-list [get]
func (h *Handlers) GetTransactionList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := model.BudgetTransactionListRequest{
		BudgetId:    q.Get("budgetId"),
		PeriodStart: q.Get("periodStart"),
		CategoryId:  optQuery(q.Get("categoryId")),
		TagId:       optQuery(q.Get("tagId")),
		EnvelopeId:  optQuery(q.Get("envelopeId")),
	}
	res, err := h.svc.GetTransactionList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

func optQuery(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// GetBudgetList handles GET /api/v1/budget/get-budget-list.
//
// @Summary  List budgets
// @Tags     Budget
// @Produce  json
// @Success  200 {object} apidoc.JsonResponseOk{data=model.GetBudgetListResult}
// @Security Bearer
// @Router   /api/v1/budget/get-budget-list [get]
func (h *Handlers) GetBudgetList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.svc.GetBudgetList)
}
