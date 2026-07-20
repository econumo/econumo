package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the model import visible to swag's annotation parser (this file's
// handler bodies no longer reference model types directly, since they
// delegate to method values).
var _ = model.CreateBudgetFolderResult{}

// CreateFolder handles POST /api/v1/budget/create-folder.
//
// @Summary Create a budget folder
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.CreateBudgetFolderRequest true "Create folder"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.CreateBudgetFolderResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/create-folder [post]
func (h *Handlers) CreateFolder(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreateFolder)
}

// UpdateFolder handles POST /api/v1/budget/update-folder.
//
// @Summary Update a budget folder
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.UpdateBudgetFolderRequest true "Update folder"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.UpdateBudgetFolderResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/update-folder [post]
func (h *Handlers) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateFolder)
}

// DeleteFolder handles POST /api/v1/budget/delete-folder.
//
// @Summary Delete a budget folder
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.DeleteFolderRequest true "Delete folder"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.DeleteFolderResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/delete-folder [post]
func (h *Handlers) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteFolder)
}

// OrderFolderList handles POST /api/v1/budget/order-folder-list.
//
// @Summary Reorder budget folders
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.OrderBudgetFolderListRequest true "Order folders"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.OrderBudgetFolderListResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/order-folder-list [post]
func (h *Handlers) OrderFolderList(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.OrderFolderList)
}

// CreateEnvelope handles POST /api/v1/budget/create-envelope.
//
// @Summary Create an envelope
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.CreateEnvelopeRequest true "Create envelope"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.CreateEnvelopeResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/create-envelope [post]
func (h *Handlers) CreateEnvelope(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreateEnvelope)
}

// UpdateEnvelope handles POST /api/v1/budget/update-envelope.
//
// @Summary Update an envelope
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.UpdateEnvelopeRequest true "Update envelope"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.UpdateEnvelopeResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/update-envelope [post]
func (h *Handlers) UpdateEnvelope(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateEnvelope)
}

// DeleteEnvelope handles POST /api/v1/budget/delete-envelope.
//
// @Summary Delete an envelope
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.DeleteEnvelopeRequest true "Delete envelope"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.DeleteEnvelopeResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/delete-envelope [post]
func (h *Handlers) DeleteEnvelope(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteEnvelope)
}

// GrantAccess handles POST /api/v1/budget/grant-access.
//
// @Summary Grant budget access
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.GrantAccessRequest true "Grant access"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.GrantAccessResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/grant-access [post]
func (h *Handlers) GrantAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.GrantAccess)
}

// AcceptAccess handles POST /api/v1/budget/accept-access.
//
// @Summary Accept budget access
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.AcceptAccessRequest true "Accept access"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.AcceptAccessResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/accept-access [post]
func (h *Handlers) AcceptAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.AcceptAccess)
}

// RevokeAccess handles POST /api/v1/budget/revoke-access.
//
// @Summary Revoke budget access
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.RevokeAccessRequest true "Revoke access"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.RevokeAccessResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/revoke-access [post]
func (h *Handlers) RevokeAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.RevokeAccess)
}

// DeclineAccess handles POST /api/v1/budget/decline-access.
//
// @Summary Decline budget access
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.DeclineAccessRequest true "Decline access"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.DeclineAccessResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/decline-access [post]
func (h *Handlers) DeclineAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeclineAccess)
}

// ExcludeAccount handles POST /api/v1/budget/exclude-account.
//
// @Summary Exclude an account from a budget
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.ExcludeAccountRequest true "Exclude account"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.ExcludeAccountResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/exclude-account [post]
func (h *Handlers) ExcludeAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ExcludeAccount)
}

// IncludeAccount handles POST /api/v1/budget/include-account.
//
// @Summary Include an account in a budget
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.IncludeAccountRequest true "Include account"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.IncludeAccountResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/include-account [post]
func (h *Handlers) IncludeAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.IncludeAccount)
}

// ChangeElementCurrency handles POST /api/v1/budget/change-element-currency.
//
// @Summary Change a budget element's currency
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.ChangeElementCurrencyRequest true "Change element currency"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.ChangeElementCurrencyResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/change-element-currency [post]
func (h *Handlers) ChangeElementCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ChangeElementCurrency)
}

// SetLimit handles POST /api/v1/budget/set-limit.
//
// @Summary Set a budget element limit
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.SetLimitRequest true "Set limit"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.SetLimitResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/set-limit [post]
func (h *Handlers) SetLimit(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SetLimit)
}

// MoveElementList handles POST /api/v1/budget/move-element-list.
//
// @Summary Move/reorder budget elements
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.MoveElementListRequest true "Move elements"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.MoveElementListResult}
// @Failure 402 {object} apidoc.JsonResponseError
// @Security Bearer
// @Router /api/v1/budget/move-element-list [post]
func (h *Handlers) MoveElementList(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.MoveElementList)
}
