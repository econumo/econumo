package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// GetBalanceSnapshot handles GET /api/v1/account/get-balance-snapshot (auth).
//
// @Summary     Snapshot account balances before an import
// @Description Returns each requested account's balance and whether it holds nothing but its opening correction. Accounts the caller may not edit are omitted.
// @Tags        Account
// @Produce     json
// @Param       accountIds query    string true "Comma-separated account ids (max 200)"
// @Success     200        {object} apidoc.JsonResponseOk{data=model.BalanceSnapshotResult}
// @Failure     400        {object} apidoc.JsonResponseError
// @Failure     401        {object} apidoc.JsonResponseUnauthorized
// @Failure     500        {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/get-balance-snapshot [get]
func (h *Handlers) GetBalanceSnapshot(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	req := model.BalanceSnapshotRequest{AccountIds: r.URL.Query().Get("accountIds")}
	if err := req.Validate(); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.GetBalanceSnapshot(r.Context(), userID, req.IDs())
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

// RestoreBalance handles POST /api/v1/account/restore-balance (auth).
//
// @Summary     Restore an account balance after an import
// @Description Brings the account back to the snapshot balance by moving its opening correction, or adding a correction, just before the earliest transaction created since the snapshot. Requires ownership or admin access.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     model.RestoreBalanceRequest true "Restore balance request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.RestoreBalanceResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/restore-balance [post]
func (h *Handlers) RestoreBalance(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.RestoreBalanceRequest) (*model.RestoreBalanceResult, error) {
		reqctx.AddLogAttr(ctx, "account_id", req.AccountId)
		return h.svc.RestoreBalance(ctx, userID, req)
	})
}
