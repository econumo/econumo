package budget

import (
	"net/http"

	appbudget "github.com/econumo/econumo/internal/app/budget"
)

// CreateFolder handles POST /api/v1/budget/create-folder.
//
// @Summary Create a budget folder
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.CreateBudgetFolderRequest true "Create folder"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.CreateBudgetFolderResult}
// @Security Bearer
// @Router /api/v1/budget/create-folder [post]
func (h *Handlers) CreateFolder(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.CreateBudgetFolderRequest) (*appbudget.CreateBudgetFolderResult, error) {
		return h.svc.CreateFolder(c.r.Context(), c.id, req)
	})
}

// UpdateFolder handles POST /api/v1/budget/update-folder.
//
// @Summary Update a budget folder
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.UpdateBudgetFolderRequest true "Update folder"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.UpdateBudgetFolderResult}
// @Security Bearer
// @Router /api/v1/budget/update-folder [post]
func (h *Handlers) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.UpdateBudgetFolderRequest) (*appbudget.UpdateBudgetFolderResult, error) {
		return h.svc.UpdateFolder(c.r.Context(), c.id, req)
	})
}

// DeleteFolder handles POST /api/v1/budget/delete-folder.
//
// @Summary Delete a budget folder
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.DeleteFolderRequest true "Delete folder"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.DeleteFolderResult}
// @Security Bearer
// @Router /api/v1/budget/delete-folder [post]
func (h *Handlers) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.DeleteFolderRequest) (*appbudget.DeleteFolderResult, error) {
		return h.svc.DeleteFolder(c.r.Context(), c.id, req)
	})
}

// OrderFolderList handles POST /api/v1/budget/order-folder-list.
//
// @Summary Reorder budget folders
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.OrderBudgetFolderListRequest true "Order folders"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.OrderBudgetFolderListResult}
// @Security Bearer
// @Router /api/v1/budget/order-folder-list [post]
func (h *Handlers) OrderFolderList(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.OrderBudgetFolderListRequest) (*appbudget.OrderBudgetFolderListResult, error) {
		return h.svc.OrderFolderList(c.r.Context(), c.id, req)
	})
}

// CreateEnvelope handles POST /api/v1/budget/create-envelope.
//
// @Summary Create an envelope
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.CreateEnvelopeRequest true "Create envelope"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.CreateEnvelopeResult}
// @Security Bearer
// @Router /api/v1/budget/create-envelope [post]
func (h *Handlers) CreateEnvelope(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.CreateEnvelopeRequest) (*appbudget.CreateEnvelopeResult, error) {
		return h.svc.CreateEnvelope(c.r.Context(), c.id, req)
	})
}

// UpdateEnvelope handles POST /api/v1/budget/update-envelope.
//
// @Summary Update an envelope
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.UpdateEnvelopeRequest true "Update envelope"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.UpdateEnvelopeResult}
// @Security Bearer
// @Router /api/v1/budget/update-envelope [post]
func (h *Handlers) UpdateEnvelope(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.UpdateEnvelopeRequest) (*appbudget.UpdateEnvelopeResult, error) {
		return h.svc.UpdateEnvelope(c.r.Context(), c.id, req)
	})
}

// DeleteEnvelope handles POST /api/v1/budget/delete-envelope.
//
// @Summary Delete an envelope
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.DeleteEnvelopeRequest true "Delete envelope"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.DeleteEnvelopeResult}
// @Security Bearer
// @Router /api/v1/budget/delete-envelope [post]
func (h *Handlers) DeleteEnvelope(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.DeleteEnvelopeRequest) (*appbudget.DeleteEnvelopeResult, error) {
		return h.svc.DeleteEnvelope(c.r.Context(), c.id, req)
	})
}

// GrantAccess handles POST /api/v1/budget/grant-access.
//
// @Summary Grant budget access
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.GrantAccessRequest true "Grant access"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.GrantAccessResult}
// @Security Bearer
// @Router /api/v1/budget/grant-access [post]
func (h *Handlers) GrantAccess(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.GrantAccessRequest) (*appbudget.GrantAccessResult, error) {
		return h.svc.GrantAccess(c.r.Context(), c.id, req)
	})
}

// AcceptAccess handles POST /api/v1/budget/accept-access.
//
// @Summary Accept budget access
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.AcceptAccessRequest true "Accept access"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.AcceptAccessResult}
// @Security Bearer
// @Router /api/v1/budget/accept-access [post]
func (h *Handlers) AcceptAccess(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.AcceptAccessRequest) (*appbudget.AcceptAccessResult, error) {
		return h.svc.AcceptAccess(c.r.Context(), c.id, req)
	})
}

// RevokeAccess handles POST /api/v1/budget/revoke-access.
//
// @Summary Revoke budget access
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.RevokeAccessRequest true "Revoke access"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.RevokeAccessResult}
// @Security Bearer
// @Router /api/v1/budget/revoke-access [post]
func (h *Handlers) RevokeAccess(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.RevokeAccessRequest) (*appbudget.RevokeAccessResult, error) {
		return h.svc.RevokeAccess(c.r.Context(), c.id, req)
	})
}

// DeclineAccess handles POST /api/v1/budget/decline-access.
//
// @Summary Decline budget access
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.DeclineAccessRequest true "Decline access"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.DeclineAccessResult}
// @Security Bearer
// @Router /api/v1/budget/decline-access [post]
func (h *Handlers) DeclineAccess(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.DeclineAccessRequest) (*appbudget.DeclineAccessResult, error) {
		return h.svc.DeclineAccess(c.r.Context(), c.id, req)
	})
}

// ExcludeAccount handles POST /api/v1/budget/exclude-account.
//
// @Summary Exclude an account from a budget
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.ExcludeAccountRequest true "Exclude account"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.ExcludeAccountResult}
// @Security Bearer
// @Router /api/v1/budget/exclude-account [post]
func (h *Handlers) ExcludeAccount(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.ExcludeAccountRequest) (*appbudget.ExcludeAccountResult, error) {
		return h.svc.ExcludeAccount(c.r.Context(), c.id, req)
	})
}

// IncludeAccount handles POST /api/v1/budget/include-account.
//
// @Summary Include an account in a budget
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.IncludeAccountRequest true "Include account"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.IncludeAccountResult}
// @Security Bearer
// @Router /api/v1/budget/include-account [post]
func (h *Handlers) IncludeAccount(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.IncludeAccountRequest) (*appbudget.IncludeAccountResult, error) {
		return h.svc.IncludeAccount(c.r.Context(), c.id, req)
	})
}

// ChangeElementCurrency handles POST /api/v1/budget/change-element-currency.
//
// @Summary Change a budget element's currency
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.ChangeElementCurrencyRequest true "Change element currency"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.ChangeElementCurrencyResult}
// @Security Bearer
// @Router /api/v1/budget/change-element-currency [post]
func (h *Handlers) ChangeElementCurrency(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.ChangeElementCurrencyRequest) (*appbudget.ChangeElementCurrencyResult, error) {
		return h.svc.ChangeElementCurrency(c.r.Context(), c.id, req)
	})
}

// SetLimit handles POST /api/v1/budget/set-limit.
//
// @Summary Set a budget element limit
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.SetLimitRequest true "Set limit"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.SetLimitResult}
// @Security Bearer
// @Router /api/v1/budget/set-limit [post]
func (h *Handlers) SetLimit(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.SetLimitRequest) (*appbudget.SetLimitResult, error) {
		return h.svc.SetLimit(c.r.Context(), c.id, req)
	})
}

// MoveElementList handles POST /api/v1/budget/move-element-list.
//
// @Summary Move/reorder budget elements
// @Tags Budget
// @Accept json
// @Produce json
// @Param request body appbudget.MoveElementListRequest true "Move elements"
// @Success 200 {object} apidoc.JsonResponseOk{data=appbudget.MoveElementListResult}
// @Security Bearer
// @Router /api/v1/budget/move-element-list [post]
func (h *Handlers) MoveElementList(w http.ResponseWriter, r *http.Request) {
	handle(h, w, r, true, func(c ctxUser, req appbudget.MoveElementListRequest) (*appbudget.MoveElementListResult, error) {
		return h.svc.MoveElementList(c.r.Context(), c.id, req)
	})
}
