package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.GetRecurringTransactionListResult{}

// GetRecurringTransactionList handles GET /api/v1/recurring/get-recurring-transaction-list (auth).
//
// @Summary     List recurring transactions
// @Description Returns every recurring transaction template on accounts the caller can access.
// @Tags        Recurring
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetRecurringTransactionListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/recurring/get-recurring-transaction-list [get]
func (h *Handlers) GetRecurringTransactionList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.svc.GetRecurringTransactionList)
}
