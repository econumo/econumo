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

// CreateBudget handles POST /api/v1/onboarding/create-budget (auth).
//
// @Summary     Create the onboarding budget
// @Description Creates a budget exactly like budget/create-budget (same validation and id idempotency) and answers at once. When the server has an AI provider, a background job then sends the accounts' names and currencies and the caller's timezone and language to it, and flags the accounts it judges savings as savings members of the budget.
// @Tags        Onboarding
// @Accept      json
// @Produce     json
// @Param       request body     model.OnboardingCreateBudgetRequest true "Onboarding create budget request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateBudgetResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/onboarding/create-budget [post]
func (h *Handlers) CreateBudget(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreateBudget)
}

// AddBudgetAccounts handles POST /api/v1/onboarding/add-budget-accounts (auth).
//
// @Summary     Add onboarding accounts to a budget
// @Description Adds each account to the budget like budget/add-account (everyday members; an existing member keeps its flag) and answers at once. When the server has an AI provider, the same background savings classification as onboarding/create-budget follows for these accounts.
// @Tags        Onboarding
// @Accept      json
// @Produce     json
// @Param       request body     model.OnboardingAddBudgetAccountsRequest true "Onboarding add budget accounts request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.OnboardingAddBudgetAccountsResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/onboarding/add-budget-accounts [post]
func (h *Handlers) AddBudgetAccounts(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.AddBudgetAccounts)
}
