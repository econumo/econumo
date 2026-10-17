package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

var _ = apidoc.JsonResponseUnauthorized{}

// GetAccountTransactionList handles GET /api/v1/transaction/get-account-transaction-list (auth).
//
// @Summary     Get transactions per account with revisions
// @Description Returns the transactions of the requested accounts (all accessible accounts when accountIds is omitted) with each account's transactions revision. Requested accounts the caller cannot access are omitted from the response.
// @Tags        Transaction
// @Produce     json
// @Param       accountIds query    string false "Comma-separated account ids (max 200); omit for all accessible accounts"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetAccountTransactionListResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/transaction/get-account-transaction-list [get]
func (h *Handlers) GetAccountTransactionList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := model.AccountTransactionListRequest{
		AccountIds: q.Get("accountIds"),
		Provided:   q.Has("accountIds"),
	}
	if err := req.Validate(); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.GetAccountTransactionList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}
