// Request/result DTOs for the onboarding use cases.
package model

import (
	"regexp"
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
// NewAccountIds names the accounts the onboarding flow itself created; only
// those are classified, so the user's earlier accounts never reach the AI.
type OnboardingCreateBudgetRequest struct {
	Id            string   `json:"id"`
	Name          string   `json:"name"`
	StartDate     string   `json:"startDate"`
	CurrencyId    string   `json:"currencyId"`
	AccountIds    []string `json:"accountIds"`
	NewAccountIds []string `json:"newAccountIds"`
}

// Validate enforces id + name NotBlank (as budget create-budget does; the
// account list is checked by the budget use case) and newAccountIds: at most
// MaxOnboardingAccountIds UUIDs, each one of accountIds.
func (r OnboardingCreateBudgetRequest) Validate() error {
	if err := ValidateBlank(map[string]string{"id": r.Id, "name": r.Name}); err != nil {
		return err
	}
	if len(r.NewAccountIds) > MaxOnboardingAccountIds {
		return newAccountIdsError("This value is too long.", errs.CodeTooLong)
	}
	members := map[string]bool{}
	for _, id := range r.AccountIds {
		members[strings.ToLower(strings.TrimSpace(id))] = true
	}
	for _, id := range r.NewAccountIds {
		if _, err := vo.ParseId(strings.TrimSpace(id)); err != nil {
			return newAccountIdsError("This value is not a valid UUID.", errs.CodeInvalidUUID)
		}
		if !members[strings.ToLower(strings.TrimSpace(id))] {
			return newAccountIdsError("The value you selected is not a valid choice.", errs.CodeInvalidChoice)
		}
	}
	return nil
}

func newAccountIdsError(msg, code string) error {
	return errs.NewValidation("Form validation error", errs.FieldError{Key: "newAccountIds", Message: msg, Code: code})
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

// Caps on one onboarding step call: a screen's worth of rows.
const (
	MaxOnboardingCategories = 60
	MaxOnboardingAccounts   = 50
	MaxOnboardingAmounts    = 200
	MaxOnboardingCurrencies = 200
	maxOnboardingKeyLength  = 64
)

var onboardingDecimalRe = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

func onboardingFieldError(key, msg, code string) error {
	return errs.NewValidation("Validation failed", errs.FieldError{Key: key, Message: msg, Code: code})
}

func blankError(key string) error {
	return onboardingFieldError(key, "This value should not be blank.", errs.CodeIsBlank)
}

func tooLongError(key string) error {
	return onboardingFieldError(key, "This value is too long.", errs.CodeTooLong)
}

func invalidUUIDError(key string) error {
	return onboardingFieldError(key, "This value is not a valid UUID.", errs.CodeInvalidUUID)
}

func invalidFormatError(key string) error {
	return onboardingFieldError(key, "This value is not valid.", errs.CodeInvalidFormat)
}

// validateOperationID requires the operation id to be a UUID.
func validateOperationID(id string) error {
	if strings.TrimSpace(id) == "" {
		return blankError("id")
	}
	if _, err := vo.ParseId(strings.TrimSpace(id)); err != nil {
		return invalidUUIDError("id")
	}
	return nil
}

// validateRowKeys requires every row key non-blank, at most 64 characters and
// unique within the call: the response maps each key to the id it produced.
func validateRowKeys(field string, keys []string) error {
	seen := map[string]bool{}
	for _, k := range keys {
		if strings.TrimSpace(k) == "" {
			return blankError(field)
		}
		if len(k) > maxOnboardingKeyLength {
			return tooLongError(field)
		}
		if seen[k] {
			return onboardingFieldError(field, "The value you selected is not a valid choice.", errs.CodeInvalidChoice)
		}
		seen[k] = true
	}
	return nil
}

// OnboardingCategoryRow is one category the categories step creates. Key is
// the client's own row key; the response maps it to the category id.
type OnboardingCategoryRow struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Type string `json:"type"`
	Icon string `json:"icon"`
}

// OnboardingSaveCategoriesRequest is the save-categories body. Categories are
// in display order; Order, when present, is the user's full wanted order of
// existing category ids and row keys (each type ordered on its own).
type OnboardingSaveCategoriesRequest struct {
	Id         string                  `json:"id"`
	Categories []OnboardingCategoryRow `json:"categories"`
	Order      []string                `json:"order"`
}

// Validate checks the shape; names, types and icons are checked by the
// category create use case itself.
func (r OnboardingSaveCategoriesRequest) Validate() error {
	if err := validateOperationID(r.Id); err != nil {
		return err
	}
	if len(r.Categories) == 0 {
		return blankError("categories")
	}
	if len(r.Categories) > MaxOnboardingCategories {
		return tooLongError("categories")
	}
	keys := make([]string, 0, len(r.Categories))
	for _, c := range r.Categories {
		keys = append(keys, c.Key)
	}
	if err := validateRowKeys("categories", keys); err != nil {
		return err
	}
	if len(r.Order) > MaxOnboardingCategories*4 {
		return tooLongError("order")
	}
	return nil
}

// OnboardingSaveCategoriesResult answers save-categories: the user (with the
// step marked), the category id each row key maps to, and the budget id ("" when
// the user still has none).
type OnboardingSaveCategoriesResult struct {
	User       CurrentUserResult `json:"user"`
	Categories map[string]string `json:"categories"`
	BudgetId   string            `json:"budgetId"`
}

// OnboardingAccountRow is one account the accounts step creates. Balance is
// the opening balance; "" means zero.
type OnboardingAccountRow struct {
	Key        string        `json:"key"`
	Name       string        `json:"name"`
	Balance    vo.FlexString `json:"balance" swaggertype:"string"`
	CurrencyId string        `json:"currencyId"`
	Icon       string        `json:"icon"`
}

// OnboardingSaveAccountsRequest is the save-accounts body. CurrencyIds are the
// currencies the user keeps visible (with the main one and every account's
// own); MainCurrencyId becomes the profile currency.
type OnboardingSaveAccountsRequest struct {
	Id             string                 `json:"id"`
	Accounts       []OnboardingAccountRow `json:"accounts"`
	CurrencyIds    []string               `json:"currencyIds"`
	MainCurrencyId string                 `json:"mainCurrencyId"`
}

// Validate checks the shape and the balances; names, icons and currencies are
// checked by the account create use case itself.
func (r OnboardingSaveAccountsRequest) Validate() error {
	if err := validateOperationID(r.Id); err != nil {
		return err
	}
	if len(r.Accounts) == 0 {
		return blankError("accounts")
	}
	if len(r.Accounts) > MaxOnboardingAccounts {
		return tooLongError("accounts")
	}
	keys := make([]string, 0, len(r.Accounts))
	for _, a := range r.Accounts {
		keys = append(keys, a.Key)
		if b := strings.TrimSpace(a.Balance.String()); b != "" && !onboardingDecimalRe.MatchString(b) {
			return invalidFormatError("balance")
		}
		if _, err := vo.ParseId(strings.TrimSpace(a.CurrencyId)); err != nil {
			return invalidUUIDError("currencyId")
		}
	}
	if err := validateRowKeys("accounts", keys); err != nil {
		return err
	}
	if len(r.CurrencyIds) > MaxOnboardingCurrencies {
		return tooLongError("currencyIds")
	}
	for _, id := range r.CurrencyIds {
		if _, err := vo.ParseId(strings.TrimSpace(id)); err != nil {
			return invalidUUIDError("currencyIds")
		}
	}
	if strings.TrimSpace(r.MainCurrencyId) == "" {
		return blankError("mainCurrencyId")
	}
	if _, err := vo.ParseId(strings.TrimSpace(r.MainCurrencyId)); err != nil {
		return invalidUUIDError("mainCurrencyId")
	}
	return nil
}

// OnboardingSaveAccountsResult answers save-accounts.
type OnboardingSaveAccountsResult struct {
	User     CurrentUserResult `json:"user"`
	Accounts map[string]string `json:"accounts"`
	BudgetId string            `json:"budgetId"`
}

// OnboardingPlanAmount is one category's plan for the budget's current period;
// "0" is a real zero plan.
type OnboardingPlanAmount struct {
	CategoryId string        `json:"categoryId"`
	Amount     vo.FlexString `json:"amount" swaggertype:"string"`
}

// OnboardingSavePlanRequest is the save-plan body. BudgetId is optional: the
// user's default budget is used without it.
type OnboardingSavePlanRequest struct {
	Id       string                 `json:"id"`
	BudgetId string                 `json:"budgetId"`
	Amounts  []OnboardingPlanAmount `json:"amounts"`
}

// Validate checks ids and amounts: a plan is never below zero.
func (r OnboardingSavePlanRequest) Validate() error {
	if err := validateOperationID(r.Id); err != nil {
		return err
	}
	if b := strings.TrimSpace(r.BudgetId); b != "" {
		if _, err := vo.ParseId(b); err != nil {
			return invalidUUIDError("budgetId")
		}
	}
	if len(r.Amounts) > MaxOnboardingAmounts {
		return tooLongError("amounts")
	}
	for _, a := range r.Amounts {
		if _, err := vo.ParseId(strings.TrimSpace(a.CategoryId)); err != nil {
			return invalidUUIDError("categoryId")
		}
		amount := strings.TrimSpace(a.Amount.String())
		if amount == "" {
			return blankError("amount")
		}
		if !onboardingDecimalRe.MatchString(amount) || strings.HasPrefix(amount, "-") {
			return invalidFormatError("amount")
		}
	}
	return nil
}

// OnboardingSavePlanResult answers save-plan.
type OnboardingSavePlanResult struct {
	User     CurrentUserResult `json:"user"`
	BudgetId string            `json:"budgetId"`
}
