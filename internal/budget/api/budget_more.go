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
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
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
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
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
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router /api/v1/budget/delete-folder [post]
func (h *Handlers) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteFolder)
}

// MoveFolder handles POST /api/v1/budget/move-folder.
//
// @Summary Move a budget folder
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.MoveBudgetFolderRequest true "Move folder"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.MoveBudgetFolderResult}
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router /api/v1/budget/move-folder [post]
func (h *Handlers) MoveFolder(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.MoveFolder)
}

// CreateEnvelope handles POST /api/v1/budget/create-envelope.
//
// @Summary Create an envelope
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.CreateEnvelopeRequest true "Create envelope"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.CreateEnvelopeResult}
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
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
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
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
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
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
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
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
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
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
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
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
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router /api/v1/budget/decline-access [post]
func (h *Handlers) DeclineAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeclineAccess)
}

// CloneBudget handles POST /api/v1/budget/clone-budget.
//
// @Summary Clone a budget
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.CloneBudgetRequest true "Clone budget"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.CloneBudgetResult}
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 403 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router /api/v1/budget/clone-budget [post]
func (h *Handlers) CloneBudget(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CloneBudget)
}

// ArchiveBudget handles POST /api/v1/budget/archive-budget.
//
// @Summary Archive a budget
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.ArchiveBudgetRequest true "Archive budget"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.ArchiveBudgetResult}
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 403 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router /api/v1/budget/archive-budget [post]
func (h *Handlers) ArchiveBudget(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ArchiveBudget)
}

// UnarchiveBudget handles POST /api/v1/budget/unarchive-budget.
//
// @Summary Unarchive a budget
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.UnarchiveBudgetRequest true "Unarchive budget"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.UnarchiveBudgetResult}
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 403 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router /api/v1/budget/unarchive-budget [post]
func (h *Handlers) UnarchiveBudget(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UnarchiveBudget)
}

// AddAccount handles POST /api/v1/budget/add-account.
//
// @Summary Add an account to a budget
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.AddAccountRequest true "Add account"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.AddAccountResult}
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router /api/v1/budget/add-account [post]
func (h *Handlers) AddAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.AddAccount)
}

// RemoveAccount handles POST /api/v1/budget/remove-account.
//
// @Summary Remove an account from a budget
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.RemoveAccountRequest true "Remove account"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.RemoveAccountResult}
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router /api/v1/budget/remove-account [post]
func (h *Handlers) RemoveAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.RemoveAccount)
}

// ChangeElementCurrency handles POST /api/v1/budget/change-element-currency.
//
// @Summary Change a budget element's currency
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.ChangeElementCurrencyRequest true "Change element currency"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.ChangeElementCurrencyResult}
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
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
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router /api/v1/budget/set-limit [post]
func (h *Handlers) SetLimit(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SetLimit)
}

// MoveElement handles POST /api/v1/budget/move-element.
//
// @Summary Move a budget element
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body model.MoveElementRequest true "Move element"
// @Success 200 {object} apidoc.JsonResponseOk{data=model.MoveElementResult}
// @Failure 400 {object} apidoc.JsonResponseError
// @Failure 401 {object} apidoc.JsonResponseUnauthorized
// @Failure 402 {object} apidoc.JsonResponseError
// @Failure 403 {object} apidoc.JsonResponseError
// @Failure 500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router /api/v1/budget/move-element [post]
func (h *Handlers) MoveElement(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.MoveElement)
}
