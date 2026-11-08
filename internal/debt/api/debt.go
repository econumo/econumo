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

// CreateDebt handles POST /api/v1/debt/create-debt (auth).
//
// @Summary     Create a debt
// @Description Creates a debt tracked through an expense category, optionally narrowed by payee, tag, label and start date. Balance and lastPaymentDate are optional as a pair: without them the debt is created not set up (isSetUp 0), collecting its payments without figures.
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
// @Description Updates a debt's name, key and terms; changed terms start a new snapshot. A body without balance and lastPaymentDate keeps the stored terms (no snapshot), and a payment or schedule left out keeps the stored one; on a set-up debt a payment or schedule that differs from the stored one is a change of terms and needs balance and lastPaymentDate too. A debt not set up is set up by a body carrying balance and lastPaymentDate (its first snapshot, as of today); without them only its payment, schedule and key change. The currency never changes.
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
