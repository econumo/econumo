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
// @Description Returns the debts, active and paid off, of every budget the caller can read, archived ones included, with their projections. Each names its element (elementId, elementType category|envelope) and takes the element's name, icon and currency; status is active or paid_off, endedAt the last day payments count. canEdit is 1 where the caller may change the debt (plan access to an unarchived budget), whatever the debt's status.
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
// @Description Returns one debt with its terms history and the payments split into interest and principal. A debt of a budget the caller cannot read is not found.
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
// @Description Projects the payments still to come, soonest first, to the payoff: the regular schedule plus the extras its element's plans add in its budget (a period's plan above its scheduled payments). budgetId and budgetName name that budget only when its plans add an extra. paymentsLeft, payoffDate and overpaymentLeft are the debt's figures with those extras (get-debt's own when there are none); extraPlanned sums the planned extras those payments carry ("0" with none). A debt not set up has no items, 0 payments left and empty figures. A debt of a budget the caller cannot read is not found.
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
// @Description Attaches a debt to an element of a budget the caller may plan in (403 for a guest, budget.archived for an archived budget): elementId is a top-level expense category or an envelope of the budget, else debt.element_not_eligible on elementId; an element that already carries an active debt is refused with debt.element_has_debt. The debt is created not set up, in the element row's currency, with its payment and schedule; its payments are the untagged expenses the element's Spent counts (on the budget's everyday accounts, in its category or the envelope's categories). An optional startDate bounds which payments count and must follow the end of the element's previous debt (debt.start_before_previous_end).
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateDebtRequest true "Create debt request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/create-debt [post]
func (h *Handlers) CreateDebt(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreateDebt)
}

// UpdateDebt handles POST /api/v1/debt/update-debt (auth).
//
// @Summary     Update a debt
// @Description Updates a debt's start date and terms; its element and currency stay. A debt not set up is set up by a body carrying amountBorrowed, startDate and lastPaymentDate (its first snapshot as of startDate, plus a statement balance when a balance is sent); without them only its payment, schedule and start date change. On a set-up debt startDate, amountBorrowed, lastPaymentDate, payment or schedule left out keep the stored ones; one that differs corrects the loan's facts: the first snapshot is rewritten (a later one takes the corrected payment, schedule and last payment date, keeping its date and balance; debt.conflicts_with_reconciliation when the new start would follow it or the new last payment date not follow it) and every payment is split again. amountBorrowed and lastPaymentDate come as a pair. A balance adds a statement balance as of balanceDate (default today), as reconcile-debt does. A start on or before the end of the element's previous debt is refused (debt.start_before_previous_end). A paid-off debt is refused (debt.paid_off).
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateDebtRequest true "Update debt request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/update-debt [post]
func (h *Handlers) UpdateDebt(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateDebt)
}

// DeleteDebt handles POST /api/v1/debt/delete-debt (auth).
//
// @Summary     Delete a debt
// @Description Deletes a debt, active or paid off, with its history (snapshots, payment splits, rate changes). Its element's plans, comments and transactions are untouched.
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteDebtRequest true "Delete debt request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DeleteDebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/delete-debt [post]
func (h *Handlers) DeleteDebt(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteDebt)
}

// MarkDebtPaidOff handles POST /api/v1/debt/mark-debt-paid-off (auth).
//
// @Summary     Mark a debt paid off
// @Description Ends a set-up debt as of date (default today; after its start date and no later than today, else common.out_of_range on date): a statement of 0 as of that date (replacing one entered that day), status paid_off and endedAt that date. Payments after it no longer count. A debt that still owes may be marked (refinancing: mark the old loan paid off, then track a new one on the same element). A debt not set up is refused (debt.not_set_up), a paid-off one with debt.paid_off.
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.MarkDebtPaidOffRequest true "Mark debt paid off request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/mark-debt-paid-off [post]
func (h *Handlers) MarkDebtPaidOff(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.MarkDebtPaidOff)
}

// ReopenDebt handles POST /api/v1/debt/reopen-debt (auth).
//
// @Summary     Reopen a paid-off debt
// @Description Takes a paid-off debt back to active: its end and the statement of 0 dated on it go, and the payments after it count again (reopen, then mark paid off again, fixes a wrong paid-off date). Refused with debt.cannot_reopen while its element carries another active debt or a debt created after it. A statement mark-debt-paid-off replaced on its end date is not restored. The debt takes its element's current currency. An active debt is returned unchanged.
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.ReopenDebtRequest true "Reopen debt request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/reopen-debt [post]
func (h *Handlers) ReopenDebt(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ReopenDebt)
}

// ReconcileDebt handles POST /api/v1/debt/reconcile-debt (auth).
//
// @Summary     Reconcile a debt with a statement
// @Description Writes the balance a statement gives for a set-up debt as of balanceDate (default today; after the start date and no later than today, else common.out_of_range on balanceDate). The balance is the principal still owed: interest since the last payment comes with the next due: a snapshot under the latest terms that overrides what the recorded payments give from that date on. A statement on a date that already has one replaces it. The payments after that date are split again from it, and every due up to the latest statement counts as paid. A debt not set up is refused (debt.not_set_up). A paid-off debt is refused (debt.paid_off).
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.ReconcileDebtRequest true "Reconcile debt request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/reconcile-debt [post]
func (h *Handlers) ReconcileDebt(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ReconcileDebt)
}

// DeleteDebtReconciliation handles POST /api/v1/debt/delete-debt-reconciliation (auth).
//
// @Summary     Delete a debt's statement balance
// @Description Removes one statement balance (a history entry after the origin, by its id); the payments after its date are split again from what comes before it. The origin cannot be deleted (not found). A paid-off debt is refused (debt.paid_off).
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteDebtReconciliationRequest true "Delete debt reconciliation request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/delete-debt-reconciliation [post]
func (h *Handlers) DeleteDebtReconciliation(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteDebtReconciliation)
}

// AddDebtRateChange handles POST /api/v1/debt/add-debt-rate-change (auth).
//
// @Summary     Add a rate change to a debt
// @Description Records that from date on (on or after the start date) the loan's yearly rate is annualRate percent (0 to 100) and its regular payment is payment; interest and the schedule's dues follow it, and the payments are split again from the start of the schedule window holding the date. A payment that can never repay the balance is refused (debt.payments_dont_cover). A change on a date that already has one replaces it. A debt not set up is refused (debt.not_set_up). A paid-off debt is refused (debt.paid_off).
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.AddDebtRateChangeRequest true "Add debt rate change request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/add-debt-rate-change [post]
func (h *Handlers) AddDebtRateChange(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.AddDebtRateChange)
}

// DeleteDebtRateChange handles POST /api/v1/debt/delete-debt-rate-change (auth).
//
// @Summary     Delete a debt's rate change
// @Description Removes one rate change by its id; the payments from its date are split again. Deleting the one stored on the start date brings back the rate the loan's terms imply; that implied first row, shown when none sits on the start date, has no id and cannot be deleted (not found). A paid-off debt is refused (debt.paid_off).
// @Tags        Debt
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteDebtRateChangeRequest true "Delete debt rate change request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DebtResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/debt/delete-debt-rate-change [post]
func (h *Handlers) DeleteDebtRateChange(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteDebtRateChange)
}
