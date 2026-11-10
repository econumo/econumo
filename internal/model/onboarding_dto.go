// Request/result DTOs for the onboarding use cases.
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MaxOnboardingAccountIds caps the accounts one onboarding call names: one
// onboarding screen's worth, and one AI prompt.
const MaxOnboardingAccountIds = 50

// OnboardingCreateBudgetRequest is the onboarding create-budget body: the
// budget create-budget fields onboarding uses, without savings (the server
// decides those in the background when an AI provider is configured).
type OnboardingCreateBudgetRequest struct {
	Id         string   `json:"id"`
	Name       string   `json:"name"`
	StartDate  string   `json:"startDate"`
	CurrencyId string   `json:"currencyId"`
	AccountIds []string `json:"accountIds"`
}

// Validate enforces id + name NotBlank (as budget create-budget does) and at
// most MaxOnboardingAccountIds account ids.
func (r OnboardingCreateBudgetRequest) Validate() error {
	if err := ValidateBlank(map[string]string{"id": r.Id, "name": r.Name}); err != nil {
		return err
	}
	if len(r.AccountIds) > MaxOnboardingAccountIds {
		return accountIdsError("This value is too long.", errs.CodeTooLong)
	}
	return nil
}

// OnboardingAddBudgetAccountsRequest adds accounts to a budget as everyday members.
type OnboardingAddBudgetAccountsRequest struct {
	BudgetId   string   `json:"budgetId"`
	AccountIds []string `json:"accountIds"`
}

// Validate requires budgetId and 1..MaxOnboardingAccountIds UUIDs on accountIds.
func (r OnboardingAddBudgetAccountsRequest) Validate() error {
	if err := ValidateBlank(map[string]string{"budgetId": r.BudgetId}); err != nil {
		return err
	}
	if len(r.AccountIds) == 0 {
		return accountIdsError("This value should not be blank.", errs.CodeIsBlank)
	}
	if len(r.AccountIds) > MaxOnboardingAccountIds {
		return accountIdsError("This value is too long.", errs.CodeTooLong)
	}
	for _, id := range r.AccountIds {
		if _, err := vo.ParseId(strings.TrimSpace(id)); err != nil {
			return accountIdsError("This value is not a valid UUID.", errs.CodeInvalidUUID)
		}
	}
	return nil
}

// OnboardingAddBudgetAccountsResult is {item: MetaResult}: the budget after
// the last account joined.
type OnboardingAddBudgetAccountsResult struct {
	Item MetaResult `json:"item"`
}
