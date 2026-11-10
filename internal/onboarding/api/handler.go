package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	apponboarding "github.com/econumo/econumo/internal/onboarding"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
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
// @Description In one transaction: creates the categories in display order (validated like category/create-category; a row named like an active category of its type, or like an earlier row, maps to that category instead), applies the optional order (existing category ids and row keys, each type ordered on its own), archives the categories in archive and restores those in restore (the caller's own; another id is a 400 on that list), creates the budget when the user owns a live account but has no budget, and marks the categories step done. The operation id makes it idempotent: a replay, or a call after the step is done, writes nothing and answers with the current state (row keys matched by name).
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
// @Description In one transaction: creates the accounts (validated like account/create-account, opening balance as a correction; the first account of a user with no folder creates one), sets the profile currency to mainCurrencyId, keeps only the main, listed and account currencies visible, moves the accounts in hide into the user's hidden archive folder (named in the request language; created hidden when missing, with a number appended when a visible folder holds the name) and those in unhide into the first visible folder (a "General" folder when there is none; the caller's own live accounts only, another id is a 400 on that list), then creates the budget over the user's live accounts outside hidden folders when they have none, else adds the new and unhidden accounts to their default budget and removes the hidden ones from it (an account that has transactions in a past period, or a savings member with plans or comments, stays a member), and marks the accounts step done. After the commit, when the server has an AI provider, a background job sends the new accounts' names and currencies with the caller's timezone and language to it and flags the ones it judges savings in that budget. Idempotent on the operation id like onboarding/save-categories.
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

// FinishImport handles POST /api/v1/onboarding/finish-import (auth).
//
// @Summary     Finish the onboarding import step
// @Description In one transaction: creates the budget when the user has none (over their live accounts outside hidden folders, in the profile currency, named in the request language; no budget without such an account) and marks the import step done. The categories and accounts steps stay open for review. After the commit, when the server has an AI provider, a background job sends the user's active categories still on a default icon (names and types) and live accounts (names and currencies), with the caller's timezone and language, and applies the icons it picks from the app's icon catalogue (only to items still on a default icon that has not changed since) and flags the savings accounts it picks in the budget this call created. Idempotent on the operation id like onboarding/save-categories.
// @Tags        Onboarding
// @Accept      json
// @Produce     json
// @Param       request body     model.OnboardingFinishImportRequest true "Finish import request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.OnboardingFinishImportResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/onboarding/finish-import [post]
func (h *Handlers) FinishImport(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.FinishImport)
}

// GetPlanSuggestions handles GET /api/v1/onboarding/get-plan-suggestions (auth).
//
// @Summary     Suggest the onboarding plan
// @Description For the current period of the budget (budgetId, which the caller must be able to update; else the user's default budget, when they can update it), suggests per own active category, income and expense, the average monthly amount over the last three complete periods (sum divided by three, in the element currency, rounded to its fraction digits), only for categories with transactions in at least two of them. Read-only. budgetId is "" and items empty when the user has no default budget they can update.
// @Tags        Onboarding
// @Produce     json
// @Param       budgetId query    string false "Budget id (default: the user's default budget)"
// @Success     200      {object} apidoc.JsonResponseOk{data=model.OnboardingGetPlanSuggestionsResult}
// @Failure     400      {object} apidoc.JsonResponseError
// @Failure     401      {object} apidoc.JsonResponseUnauthorized
// @Failure     403      {object} apidoc.JsonResponseError
// @Failure     500      {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/onboarding/get-plan-suggestions [get]
func (h *Handlers) GetPlanSuggestions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	req := model.OnboardingGetPlanSuggestionsRequest{BudgetId: r.URL.Query().Get("budgetId")}
	if err := req.Validate(); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.GetPlanSuggestions(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}
