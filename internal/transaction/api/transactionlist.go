package api

import (
	"net/http"

	apptransaction "github.com/econumo/econumo/internal/transaction"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

var _ = apidoc.JsonResponseUnauthorized{}

// GetTransactionList handles GET /api/v1/transaction/get-transaction-list (auth).
// Optional query params: accountId, periodStart, periodEnd.
//
// @Summary     Get the transaction list
// @Description Returns transactions for an account, or a date window across visible accounts, or all visible-account transactions.
// @Tags        Transaction
// @Produce     json
// @Param       accountId   query    string false "Account id filter"
// @Param       periodStart query    string false "Period start (Y-m-d H:i:s or Y-m-d)"
// @Param       periodEnd   query    string false "Period end (Y-m-d H:i:s or Y-m-d)"
// @Success     200 {object} apidoc.JsonResponseOk{data=apptransaction.GetTransactionListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/transaction/get-transaction-list [get]
func (h *Handlers) GetTransactionList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := apptransaction.GetTransactionListRequest{
		AccountId:   q.Get("accountId"),
		PeriodStart: q.Get("periodStart"),
		PeriodEnd:   q.Get("periodEnd"),
	}
	if err := req.Validate(); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.GetTransactionList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
