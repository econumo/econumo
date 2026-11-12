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

// GetDebtList handles GET /api/v1/debt/get-debt-list (auth).
//
// @Summary     Get the debt list
// @Description Returns the authenticated user's debts with their projections.
// @Tags        Debt
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.DebtListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/get-debt-list [get]
func (h *Handlers) GetDebtList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetDebtList)
}

// GetDebt handles GET /api/v1/debt/get-debt (auth).
//
// @Summary     Get a debt
// @Description Returns one debt with its terms history and the payments split into interest and principal. A debt of another user is not found.
// @Tags        Debt
// @Produce     json
// @Param       id  query string true "Debt id"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.DebtDetailsResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/get-debt [get]
func (h *Handlers) GetDebt(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	req := model.GetDebtRequest{Id: r.URL.Query().Get("id")}
	if err := req.Validate(); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.GetDebt(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

// GetDebtForecast handles GET /api/v1/debt/get-debt-forecast (auth).
//
// @Summary     Get a debt's future payments
// @Description Projects the payments still to come, soonest first, to the payoff: the regular schedule plus the extras planned for the debt's row in its budget. A budget that holds no row for the debt leaves the regular payments alone; budgetId and budgetName name the budget only when its plans add an extra. paymentsLeft, payoffDate and overpaymentLeft are the debt's figures with those extras (get-debt's own when there are none); extraPlanned sums the planned extras those payments carry ("0" with none). A debt not set up has no items, 0 payments left and empty figures. A debt of another user is not found.
// @Tags        Debt
// @Produce     json
// @Param       id       query string true  "Debt id"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.DebtForecastResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/get-debt-forecast [get]
func (h *Handlers) GetDebtForecast(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := model.GetDebtForecastRequest{Id: q.Get("id")}
	if err := req.Validate(); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.GetDebtForecast(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

// CreateDebt handles POST /api/v1/debt/create-debt (auth).
//
// @Summary     Create a debt
// @Description Creates a debt tracked through an expense category, optionally narrowed by payee, tag and label. amountBorrowed, startDate and lastPaymentDate are the loan's facts and come together (a missing one is common.is_blank): the debt's first snapshot is amountBorrowed as of startDate (debt.start_in_future after today, debt.last_payment_before_start unless lastPaymentDate is later), and what is owed follows from the payments recorded since. An optional balance adds a statement balance as of balanceDate (default today). Without the facts the debt is created not set up (isSetUp 0), collecting its payments without figures; a startDate alone then only bounds which payments count.
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateDebtRequest true "Create debt request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/create-debt [post]
func (h *Handlers) CreateDebt(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreateDebt)
}

// UpdateDebt handles POST /api/v1/debt/update-debt (auth).
//
// @Summary     Update a debt
// @Description Updates a debt's name, key and terms. A debt not set up is set up by a body carrying amountBorrowed, startDate and lastPaymentDate (its first snapshot as of startDate, plus a statement balance when a balance is sent); without them only its payment, schedule, start date and key change. On a set-up debt startDate, amountBorrowed, lastPaymentDate, payment or schedule left out keep the stored ones; one that differs corrects the loan's facts: the first snapshot is rewritten (a later one takes the corrected payment, schedule and last payment date, keeping its date and balance; debt.conflicts_with_reconciliation when the new start would follow it or the new last payment date not follow it) and every payment is split again. amountBorrowed and lastPaymentDate come as a pair. A balance adds a statement balance as of balanceDate (default today), as reconcile-debt does. A currencyId other than the stored one moves the debt to that currency (refused with currency.not_available when the user may not use it): the amounts sent, or the stored ones when left out, are read in it unconverted, the payments are split again and the budget rows follow; empty keeps the stored currency. tagId and labelId are exclusive (common.invalid_choice on labelId).
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateDebtRequest true "Update debt request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/update-debt [post]
func (h *Handlers) UpdateDebt(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateDebt)
}

// DeleteDebt handles POST /api/v1/debt/delete-debt (auth).
//
// @Summary     Delete a debt
// @Description Deletes a debt. A narrowed debt that holds budget plans or comments needs confirmDataRemoval.
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteDebtRequest true "Delete debt request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DeleteDebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/delete-debt [post]
func (h *Handlers) DeleteDebt(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteDebt)
}

// MarkDebtPaidOff handles POST /api/v1/debt/mark-debt-paid-off (auth).
//
// @Summary     Mark a debt paid off
// @Description Writes a zero-balance snapshot as of today. A debt not set up is refused.
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.MarkDebtPaidOffRequest true "Mark debt paid off request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/mark-debt-paid-off [post]
func (h *Handlers) MarkDebtPaidOff(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.MarkDebtPaidOff)
}

// ReconcileDebt handles POST /api/v1/debt/reconcile-debt (auth).
//
// @Summary     Reconcile a debt with a statement
// @Description Writes the balance a statement gives for a set-up debt as of balanceDate (default today; after the start date and no later than today, else common.out_of_range on balanceDate). The balance is the principal still owed: interest since the last payment comes with the next due: a snapshot under the latest terms that overrides what the recorded payments give from that date on. A statement on a date that already has one replaces it. The payments after that date are split again from it, and every due up to the latest statement counts as paid. A debt not set up is refused (debt.not_set_up).
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.ReconcileDebtRequest true "Reconcile debt request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/reconcile-debt [post]
func (h *Handlers) ReconcileDebt(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ReconcileDebt)
}

// DeleteDebtReconciliation handles POST /api/v1/debt/delete-debt-reconciliation (auth).
//
// @Summary     Delete a debt's statement balance
// @Description Removes one statement balance (a history entry after the origin, by its id); the payments after its date are split again from what comes before it. The origin cannot be deleted (not found).
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteDebtReconciliationRequest true "Delete debt reconciliation request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/delete-debt-reconciliation [post]
func (h *Handlers) DeleteDebtReconciliation(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteDebtReconciliation)
}
