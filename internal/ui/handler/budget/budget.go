package budget

import (
	"net/http"

	appbudget "github.com/econumo/econumo/internal/app/budget"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

var _ = apidoc.JsonResponseError{}

// handle is the shared adapter: require user, optionally decode+validate the
// request, call fn, emit the OK envelope (or the mapped error). Pass decode=false
// for GET endpoints with no body.
func handle[Req any, Res any](h *Handlers, w http.ResponseWriter, r *http.Request, decode bool, fn func(ctx ctxUser, req Req) (*Res, error)) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req Req
	if decode {
		if err := httpx.DecodeValidate(r, &req); err != nil {
			httpx.WriteError(w, err, h.dev)
			return
		}
	}
	res, err := fn(ctxUser{r: r, id: userID}, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

type ctxUser struct {
	r  *http.Request
	id vo.Id
}

// CreateBudget handles POST /api/v1/budget/create-budget.
//
// @Summary  Create a budget
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body appbudget.CreateBudgetRequest true "Create budget"
// @Success  200 {object} apidoc.JsonResponseOk{data=appbudget.CreateBudgetResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/budget/create-budget [post]
func (h *Handlers) CreateBudget(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.CreateBudgetRequest) (*appbudget.CreateBudgetResult, error) {
		return h.svc.CreateBudget(c.r.Context(), c.id, req)
	})
}

// UpdateBudget handles POST /api/v1/budget/update-budget.
//
// @Summary  Update a budget
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body appbudget.UpdateBudgetRequest true "Update budget"
// @Success  200 {object} apidoc.JsonResponseOk{data=appbudget.UpdateBudgetResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router   /api/v1/budget/update-budget [post]
func (h *Handlers) UpdateBudget(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.UpdateBudgetRequest) (*appbudget.UpdateBudgetResult, error) {
		return h.svc.UpdateBudget(c.r.Context(), c.id, req)
	})
}

// DeleteBudget handles POST /api/v1/budget/delete-budget.
//
// @Summary  Delete a budget
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body appbudget.DeleteBudgetRequest true "Delete budget"
// @Success  200 {object} apidoc.JsonResponseOk{data=appbudget.DeleteBudgetResult}
// @Security Bearer
// @Router   /api/v1/budget/delete-budget [post]
func (h *Handlers) DeleteBudget(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.DeleteBudgetRequest) (*appbudget.DeleteBudgetResult, error) {
		return h.svc.DeleteBudget(c.r.Context(), c.id, req)
	})
}

// ResetBudget handles POST /api/v1/budget/reset-budget.
//
// @Summary  Reset a budget
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body appbudget.ResetBudgetRequest true "Reset budget"
// @Success  200 {object} apidoc.JsonResponseOk{data=appbudget.ResetBudgetResult}
// @Security Bearer
// @Router   /api/v1/budget/reset-budget [post]
func (h *Handlers) ResetBudget(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.ResetBudgetRequest) (*appbudget.ResetBudgetResult, error) {
		return h.svc.ResetBudget(c.r.Context(), c.id, req)
	})
}

// GetBudget handles GET /api/v1/budget/get-budget.
//
// @Summary  Get a budget
// @Tags     Budget
// @Produce  json
// @Param    id    query string true  "Budget id"
// @Param    date  query string false "Period date (Y-m-d)"
// @Success  200 {object} apidoc.JsonResponseOk{data=appbudget.GetBudgetResult}
// @Security Bearer
// @Router   /api/v1/budget/get-budget [get]
func (h *Handlers) GetBudget(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	req := appbudget.GetBudgetRequest{Id: r.URL.Query().Get("id"), Date: r.URL.Query().Get("date")}
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
// @Success  200 {object} apidoc.JsonResponseOk{data=appbudget.GetBudgetTransactionListResult}
// @Security Bearer
// @Router   /api/v1/budget/get-transaction-list [get]
func (h *Handlers) GetTransactionList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := appbudget.GetTransactionListRequest{
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
// @Success  200 {object} apidoc.JsonResponseOk{data=appbudget.GetBudgetListResult}
// @Security Bearer
// @Router   /api/v1/budget/get-budget-list [get]
func (h *Handlers) GetBudgetList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	res, err := h.svc.GetBudgetList(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
