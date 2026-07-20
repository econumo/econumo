package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}

// CreateAccount handles POST /api/v1/account/create-account (auth).
//
// @Summary     Create an account
// @Description Creates an account (idempotent on the request id), optionally seeding its balance, in the given folder.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateAccountRequest true "Create account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/create-account [post]
func (h *Handlers) CreateAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.CreateAccountRequest) (*model.CreateAccountResult, error) {
		reqctx.AddLogAttr(ctx, "account_id", req.Id)
		return h.svc.CreateAccount(ctx, userID, req)
	})
}

// UpdateAccount handles POST /api/v1/account/update-account (auth).
//
// @Summary     Update an account
// @Description Updates an account's name/icon/currency and reconciles its balance via a correction transaction. Requires ownership.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateAccountRequest true "Update account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/update-account [post]
func (h *Handlers) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdateAccount)
}

// DeleteAccount handles POST /api/v1/account/delete-account (auth).
//
// @Summary     Delete an account
// @Description Soft-deletes an account. Requires ownership.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteAccountRequest true "Delete account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.DeleteAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/delete-account [post]
func (h *Handlers) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.DeleteAccount)
}

// GetAccountList handles GET /api/v1/account/get-account-list (auth).
//
// @Summary     Get the account list
// @Description Returns all the user's available accounts (each with owner, currency, folder, position, balance) in reverse order.
// @Tags        Account
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetAccountListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/get-account-list [get]
func (h *Handlers) GetAccountList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.svc.GetAccountList)
}

// OrderAccountList handles POST /api/v1/account/order-account-list (auth).
//
// @Summary     Reorder the account list
// @Description Repositions accounts and moves them between folders, returning the full list.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     model.OrderAccountListRequest true "Order account list request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.OrderAccountListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/order-account-list [post]
func (h *Handlers) OrderAccountList(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.OrderAccountList)
}
