// Request/result DTOs for the onboarding use cases.
package model

import (
	"regexp"
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Caps on one onboarding step call: a screen's worth of rows.
const (
	MaxOnboardingCategories = 60
	MaxOnboardingAccounts   = 50
	MaxOnboardingAmounts    = 200
	MaxOnboardingCurrencies = 200
	MaxOnboardingReviewIDs  = 200
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
// existing category ids and row keys (each type ordered on its own). Archive
// and Restore name existing own categories to archive or bring back.
type OnboardingSaveCategoriesRequest struct {
	Id         string                  `json:"id"`
	Categories []OnboardingCategoryRow `json:"categories"`
	Order      []string                `json:"order"`
	Archive    []string                `json:"archive"`
	Restore    []string                `json:"restore"`
}

// Validate checks the shape; names, types and icons are checked by the
// category create use case itself. No rows is valid: a user who already has
// categories finishes the step by reordering them or by keeping them as they are.
func (r OnboardingSaveCategoriesRequest) Validate() error {
	if err := validateOperationID(r.Id); err != nil {
		return err
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
	return validateReviewIDs("archive", r.Archive, "restore", r.Restore)
}

// validateReviewIDs checks two id lists that toggle one state: UUIDs, at most
// MaxOnboardingReviewIDs each, and no id in both.
func validateReviewIDs(onKey string, on []string, offKey string, off []string) error {
	seen := map[string]bool{}
	for _, list := range []struct {
		key string
		ids []string
	}{{onKey, on}, {offKey, off}} {
		if len(list.ids) > MaxOnboardingReviewIDs {
			return tooLongError(list.key)
		}
		for _, raw := range list.ids {
			id, err := vo.ParseId(strings.TrimSpace(raw))
			if err != nil {
				return invalidUUIDError(list.key)
			}
			if list.key == offKey && seen[id.String()] {
				return onboardingFieldError(offKey, "The value you selected is not a valid choice.", errs.CodeInvalidChoice)
			}
			if list.key == onKey {
				seen[id.String()] = true
			}
		}
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

// OnboardingAccountCurrency moves an existing own account to another currency
// (a CSV import creates every account in the instance base currency).
type OnboardingAccountCurrency struct {
	AccountId  string `json:"accountId"`
	CurrencyId string `json:"currencyId"`
}

// OnboardingSaveAccountsRequest is the save-accounts body. CurrencyIds are the
// currencies the user keeps visible (with the main one and every account's
// own); MainCurrencyId becomes the profile currency. Hide and Unhide name
// existing own accounts to move into the hidden archive folder (and out of the
// budget) or back; Currencies changes existing own accounts' currency.
type OnboardingSaveAccountsRequest struct {
	Id             string                      `json:"id"`
	Accounts       []OnboardingAccountRow      `json:"accounts"`
	CurrencyIds    []string                    `json:"currencyIds"`
	MainCurrencyId string                      `json:"mainCurrencyId"`
	Hide           []string                    `json:"hide"`
	Unhide         []string                    `json:"unhide"`
	Currencies     []OnboardingAccountCurrency `json:"currencies"`
}

// Validate checks the shape and the balances; names, icons and currencies are
// checked by the account create use case itself. No rows is valid: a user who
// already has accounts still picks the currencies.
func (r OnboardingSaveAccountsRequest) Validate() error {
	if err := validateOperationID(r.Id); err != nil {
		return err
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
	if len(r.Currencies) > MaxOnboardingReviewIDs {
		return tooLongError("currencies")
	}
	seen := map[string]bool{}
	for _, c := range r.Currencies {
		id, err := vo.ParseId(strings.TrimSpace(c.AccountId))
		if err != nil {
			return invalidUUIDError("currencies")
		}
		if _, err := vo.ParseId(strings.TrimSpace(c.CurrencyId)); err != nil {
			return invalidUUIDError("currencies")
		}
		if seen[id.String()] {
			return onboardingFieldError("currencies", "The value you selected is not a valid choice.", errs.CodeInvalidChoice)
		}
		seen[id.String()] = true
	}
	return validateReviewIDs("hide", r.Hide, "unhide", r.Unhide)
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

// OnboardingFinishImportRequest is the finish-import body: the operation id.
type OnboardingFinishImportRequest struct {
	Id string `json:"id"`
}

func (r OnboardingFinishImportRequest) Validate() error {
	return validateOperationID(r.Id)
}

// OnboardingFinishImportResult answers finish-import: the user (import step
// marked) and the budget id ("" when the user has no account to budget).
type OnboardingFinishImportResult struct {
	User     CurrentUserResult `json:"user"`
	BudgetId string            `json:"budgetId"`
}

// OnboardingGetPlanSuggestionsRequest selects the budget; "" is the default one.
type OnboardingGetPlanSuggestionsRequest struct {
	BudgetId string `json:"budgetId"`
}

func (r OnboardingGetPlanSuggestionsRequest) Validate() error {
	if b := strings.TrimSpace(r.BudgetId); b != "" {
		if _, err := vo.ParseId(b); err != nil {
			return invalidUUIDError("budgetId")
		}
	}
	return nil
}

// OnboardingPlanSuggestion is one category's suggested plan for the current
// period, in the category element's currency.
type OnboardingPlanSuggestion struct {
	CategoryId string `json:"categoryId"`
	Amount     string `json:"amount"`
}

// OnboardingGetPlanSuggestionsResult answers get-plan-suggestions; BudgetId is
// "" (and Items empty) when the user has no budget.
type OnboardingGetPlanSuggestionsResult struct {
	BudgetId string                     `json:"budgetId"`
	Items    []OnboardingPlanSuggestion `json:"items"`
}
