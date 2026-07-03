package api

import (
	"net/http"

	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/reqctx"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/httpx"
)

var _ = apidoc.JsonResponseError{}

// CreateAccount handles POST /api/v1/account/create-account (auth).
//
// @Summary     Create an account
// @Description Creates an account (idempotent on the request id), optionally seeding its balance, in the given folder.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     appaccount.CreateAccountRequest true "Create account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appaccount.CreateAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/create-account [post]
func (h *Handlers) CreateAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appaccount.CreateAccountRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	reqctx.AddLogAttr(r.Context(), "account_id", req.Id)
	res, err := h.svc.CreateAccount(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// UpdateAccount handles POST /api/v1/account/update-account (auth).
//
// @Summary     Update an account
// @Description Updates an account's name/icon/currency and reconciles its balance via a correction transaction. Requires ownership.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     appaccount.UpdateAccountRequest true "Update account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appaccount.UpdateAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/update-account [post]
func (h *Handlers) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appaccount.UpdateAccountRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.UpdateAccount(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// DeleteAccount handles POST /api/v1/account/delete-account (auth).
//
// @Summary     Delete an account
// @Description Soft-deletes an account. Requires ownership.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     appaccount.DeleteAccountRequest true "Delete account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appaccount.DeleteAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/delete-account [post]
func (h *Handlers) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appaccount.DeleteAccountRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.DeleteAccount(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// GetAccountList handles GET /api/v1/account/get-account-list (auth).
//
// @Summary     Get the account list
// @Description Returns all the user's available accounts (each with owner, currency, folder, position, balance) in reverse order.
// @Tags        Account
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=appaccount.GetAccountListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/get-account-list [get]
func (h *Handlers) GetAccountList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	res, err := h.svc.GetAccountList(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

// OrderAccountList handles POST /api/v1/account/order-account-list (auth).
//
// @Summary     Reorder the account list
// @Description Repositions accounts and moves them between folders, returning the full list.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     appaccount.OrderAccountListRequest true "Order account list request"
// @Success     200     {object} apidoc.JsonResponseOk{data=appaccount.OrderAccountListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/order-account-list [post]
func (h *Handlers) OrderAccountList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req appaccount.OrderAccountListRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.OrderAccountList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
