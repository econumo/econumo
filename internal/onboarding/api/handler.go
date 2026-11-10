package api

import (
	"net/http"

	apponboarding "github.com/econumo/econumo/internal/onboarding"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// Forces the apidoc import so swag annotations can resolve its envelope schemas.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *apponboarding.Service
}

func NewHandlers(svc *apponboarding.Service) *Handlers {
	return &Handlers{svc: svc}
}

// SaveCategories handles POST /api/v1/onboarding/save-categories (auth).
//
// @Summary     Save the onboarding categories step
// @Description In one transaction: creates the categories in display order (validated like category/create-category; a row named like an active category of its type, or like an earlier row, maps to that category instead), applies the optional order (existing category ids and row keys, each type ordered on its own), creates the budget when the user owns a live account but has no budget, and marks the categories step done. The operation id makes it idempotent: a replay, or a call after the step is done, writes nothing and answers with the current state (row keys matched by name).
// @Tags        Onboarding
// @Accept      json
// @Produce     json
// @Param       request body     model.OnboardingSaveCategoriesRequest true "Save categories request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.OnboardingSaveCategoriesResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/onboarding/save-categories [post]
func (h *Handlers) SaveCategories(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SaveCategories)
}

// SaveAccounts handles POST /api/v1/onboarding/save-accounts (auth).
//
// @Summary     Save the onboarding accounts step
// @Description In one transaction: creates the accounts (validated like account/create-account, opening balance as a correction; the first account of a user with no folder creates one), sets the profile currency to mainCurrencyId, keeps only the main, listed and account currencies visible, then creates the budget over all the user's live accounts when they have none, else adds the new accounts to their default budget, and marks the accounts step done. After the commit, when the server has an AI provider, a background job sends the new accounts' names and currencies with the caller's timezone and language to it and flags the ones it judges savings in that budget. Idempotent on the operation id like onboarding/save-categories.
// @Tags        Onboarding
// @Accept      json
// @Produce     json
// @Param       request body     model.OnboardingSaveAccountsRequest true "Save accounts request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.OnboardingSaveAccountsResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/onboarding/save-accounts [post]
func (h *Handlers) SaveAccounts(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SaveAccounts)
}

// SavePlan handles POST /api/v1/onboarding/save-plan (auth).
//
// @Summary     Save the onboarding plan step
// @Description In one transaction: writes each amount as the category's limit for the budget's current period (budgetId, else the user's default budget; the caller must be able to update it) like budget/set-limit, and marks the plan step done. Idempotent on the operation id like onboarding/save-categories.
// @Tags        Onboarding
// @Accept      json
// @Produce     json
// @Param       request body     model.OnboardingSavePlanRequest true "Save plan request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.OnboardingSavePlanResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/onboarding/save-plan [post]
func (h *Handlers) SavePlan(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SavePlan)
}
