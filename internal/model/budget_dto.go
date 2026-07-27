// Budget module application DTOs: request bodies (with tier-1 Validate()) and
// response result shapes, frozen to the existing wire contract.
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// AccessResult is one access entry in a budget's meta: {user, role, isAccepted}.
// User embeds the shared UserResult {id, avatar, name} shape.
type AccessResult struct {
	User       UserResult `json:"user"`
	Role       string     `json:"role"`
	IsAccepted int        `json:"isAccepted"`
}

// MetaResult is a budget's metadata block.
type MetaResult struct {
	Id          string         `json:"id"`
	OwnerUserId string         `json:"ownerUserId"`
	Name        string         `json:"name"`
	StartedAt   string         `json:"startedAt"`
	CurrencyId  string         `json:"currencyId"`
	Access      []AccessResult `json:"access"`
}

// FiltersResult is the budget's period + excluded accounts.
type FiltersResult struct {
	PeriodStart         string   `json:"periodStart"`
	PeriodEnd           string   `json:"periodEnd"`
	ExcludedAccountsIds []string `json:"excludedAccountsIds"`
}

// CurrencyBalanceResult is one currency's period financial summary. The amount
// fields are null when the period has not started/ended yet.
type CurrencyBalanceResult struct {
	CurrencyId   string  `json:"currencyId"`
	StartBalance *string `json:"startBalance"`
	EndBalance   *string `json:"endBalance"`
	Income       *string `json:"income"`
	Expenses     *string `json:"expenses"`
	Exchanges    *string `json:"exchanges"`
	Holdings     *string `json:"holdings"`
}

// AverageCurrencyRateResult is one currency's average rate over the period.
type AverageCurrencyRateResult struct {
	CurrencyId     string `json:"currencyId"`
	BaseCurrencyId string `json:"baseCurrencyId"`
	Rate           string `json:"rate"`
	PeriodStart    string `json:"periodStart"`
	PeriodEnd      string `json:"periodEnd"`
}

// BudgetFolderResult is one budget folder.
type BudgetFolderResult struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

// ChildElementResult is a category nested under an envelope/tag element.
type ChildElementResult struct {
	Id          string `json:"id"`
	Type        int    `json:"type"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	IsArchived  int    `json:"isArchived"`
	Spent       string `json:"spent"`
	BudgetSpent string `json:"budgetSpent"`
	OwnerUserId string `json:"ownerUserId"`
}

// ParentElementResult is a top-level budget element (envelope/tag/category).
type ParentElementResult struct {
	Id          string               `json:"id"`
	Type        int                  `json:"type"`
	Name        string               `json:"name"`
	Icon        string               `json:"icon"`
	CurrencyId  string               `json:"currencyId"`
	IsArchived  int                  `json:"isArchived"`
	FolderId    *string              `json:"folderId"`
	Position    int                  `json:"position"`
	Budgeted    string               `json:"budgeted"`
	Available   string               `json:"available"`
	Spent       string               `json:"spent"`
	BudgetSpent string               `json:"budgetSpent"`
	Children    []ChildElementResult `json:"children"`
	OwnerUserId *string              `json:"ownerUserId"`
}

// StructureResult is the budget's folders + ordered elements.
type StructureResult struct {
	Folders  []BudgetFolderResult  `json:"folders"`
	Elements []ParentElementResult `json:"elements"`
}

// BudgetResult is the full get-budget shape.
type BudgetResult struct {
	Meta          MetaResult                  `json:"meta"`
	Filters       FiltersResult               `json:"filters"`
	Balances      []CurrencyBalanceResult     `json:"balances"`
	CurrencyRates []AverageCurrencyRateResult `json:"currencyRates"`
	Structure     StructureResult             `json:"structure"`
}

// CreateBudgetRequest is the create-budget body.
type CreateBudgetRequest struct {
	Id               string   `json:"id"`
	Name             string   `json:"name"`
	StartDate        string   `json:"startDate"`
	CurrencyId       string   `json:"currencyId"`
	ExcludedAccounts []string `json:"excludedAccounts"`
}

// Validate enforces id + name NotBlank.
func (r CreateBudgetRequest) Validate() error {
	return ValidateBlank(map[string]string{"id": r.Id, "name": r.Name})
}

// CreateBudgetResult is {item: BudgetResult}.
type CreateBudgetResult struct {
	Item BudgetResult `json:"item"`
}

// UpdateBudgetRequest is the update-budget body.
type UpdateBudgetRequest struct {
	Id               string   `json:"id"`
	Name             string   `json:"name"`
	CurrencyId       string   `json:"currencyId"`
	ExcludedAccounts []string `json:"excludedAccounts"`
}

// Validate enforces id, name, currencyId NotBlank.
func (r UpdateBudgetRequest) Validate() error {
	return ValidateBlank(map[string]string{"id": r.Id, "name": r.Name, "currencyId": r.CurrencyId})
}

// UpdateBudgetResult is {item: MetaResult}.
type UpdateBudgetResult struct {
	Item MetaResult `json:"item"`
}

// DeleteBudgetRequest / ResetBudgetRequest / GetBudgetRequest bodies.
type DeleteBudgetRequest struct {
	Id string `json:"id"`
}

func (r DeleteBudgetRequest) Validate() error { return ValidateBlank(map[string]string{"id": r.Id}) }

// DeleteBudgetResult is empty.
type DeleteBudgetResult struct{}

// ResetBudgetRequest resets a budget's start month.
type ResetBudgetRequest struct {
	Id        string `json:"id"`
	StartedAt string `json:"startedAt"`
}

func (r ResetBudgetRequest) Validate() error {
	return ValidateBlank(map[string]string{"id": r.Id, "startedAt": r.StartedAt})
}

// ResetBudgetResult is {item: MetaResult}.
type ResetBudgetResult struct {
	Item MetaResult `json:"item"`
}

// GetBudgetRequest selects a budget + period date.
type GetBudgetRequest struct {
	Id   string `json:"id"`
	Date string `json:"date"`
}

func (r GetBudgetRequest) Validate() error { return ValidateBlank(map[string]string{"id": r.Id}) }

// GetBudgetResult is {item: BudgetResult}.
type GetBudgetResult struct {
	Item BudgetResult `json:"item"`
}

// GetBudgetListResult is {items: [MetaResult]}.
type GetBudgetListResult struct {
	Items []MetaResult `json:"items"`
}

// CreateBudgetFolderRequest / UpdateBudgetFolderRequest bodies.
type CreateBudgetFolderRequest struct {
	BudgetId string `json:"budgetId"`
	Id       string `json:"id"`
	Name     string `json:"name"`
}

func (r CreateBudgetFolderRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId, "id": r.Id, "name": r.Name})
}

// CreateBudgetFolderResult is {item: BudgetFolderResult}.
type CreateBudgetFolderResult struct {
	Item BudgetFolderResult `json:"item"`
}

// UpdateBudgetFolderRequest updates a folder name.
type UpdateBudgetFolderRequest struct {
	BudgetId string `json:"budgetId"`
	Id       string `json:"id"`
	Name     string `json:"name"`
}

func (r UpdateBudgetFolderRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId, "id": r.Id, "name": r.Name})
}

// UpdateBudgetFolderResult is {item: BudgetFolderResult}.
type UpdateBudgetFolderResult struct {
	Item BudgetFolderResult `json:"item"`
}

// DeleteFolderRequest deletes a folder.
type DeleteFolderRequest struct {
	BudgetId string `json:"budgetId"`
	Id       string `json:"id"`
}

func (r DeleteFolderRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId, "id": r.Id})
}

// DeleteFolderResult is empty.
type DeleteFolderResult struct{}

// OrderFolderListItem is one folder reorder instruction.
type OrderFolderListItem struct {
	Id       string `json:"id"`
	Position int    `json:"position"`
}

// OrderBudgetFolderListRequest reorders folders.
type OrderBudgetFolderListRequest struct {
	BudgetId string                `json:"budgetId"`
	Items    []OrderFolderListItem `json:"items"`
}

func (r OrderBudgetFolderListRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId})
}

// OrderBudgetFolderListResult is empty.
type OrderBudgetFolderListResult struct{}

// CreateEnvelopeRequest creates an envelope (+ its budget element).
type CreateEnvelopeRequest struct {
	BudgetId   string   `json:"budgetId"`
	Id         string   `json:"id"`
	Name       string   `json:"name"`
	Icon       string   `json:"icon"`
	CurrencyId string   `json:"currencyId"`
	FolderId   *string  `json:"folderId"`
	Categories []string `json:"categories"`
}

func (r CreateEnvelopeRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId, "id": r.Id, "name": r.Name, "icon": r.Icon, "currencyId": r.CurrencyId})
}

// CreateEnvelopeResult is {item: ParentElementResult}.
type CreateEnvelopeResult struct {
	Item ParentElementResult `json:"item"`
}

// UpdateEnvelopeRequest updates an envelope.
type UpdateEnvelopeRequest struct {
	BudgetId   string   `json:"budgetId"`
	Id         string   `json:"id"`
	Name       string   `json:"name"`
	Icon       string   `json:"icon"`
	CurrencyId string   `json:"currencyId"`
	IsArchived int      `json:"isArchived"`
	Categories []string `json:"categories"`
}

func (r UpdateEnvelopeRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId, "id": r.Id, "name": r.Name, "icon": r.Icon, "currencyId": r.CurrencyId})
}

// UpdateEnvelopeResult is {item: ParentElementResult}.
type UpdateEnvelopeResult struct {
	Item ParentElementResult `json:"item"`
}

// DeleteEnvelopeRequest deletes an envelope.
type DeleteEnvelopeRequest struct {
	BudgetId string `json:"budgetId"`
	Id       string `json:"id"`
}

func (r DeleteEnvelopeRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId, "id": r.Id})
}

// DeleteEnvelopeResult is empty.
type DeleteEnvelopeResult struct{}

// GrantAccessRequest grants a user access to a budget.
type GrantAccessRequest struct {
	BudgetId string `json:"budgetId"`
	UserId   string `json:"userId"`
	Role     string `json:"role"`
}

func (r GrantAccessRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId, "userId": r.UserId, "role": r.Role})
}

// GrantAccessResult / AcceptAccessResult are {items: [MetaResult]}.
type GrantAccessResult struct {
	Items []MetaResult `json:"items"`
}

// AcceptAccessRequest / DeclineAccessRequest accept/decline an invite.
type AcceptAccessRequest struct {
	BudgetId string `json:"budgetId"`
}

func (r AcceptAccessRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId})
}

// AcceptAccessResult is {items: [MetaResult]}.
type AcceptAccessResult struct {
	Items []MetaResult `json:"items"`
}

// DeclineAccessRequest declines an invite.
type DeclineAccessRequest struct {
	BudgetId string `json:"budgetId"`
}

func (r DeclineAccessRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId})
}

// DeclineAccessResult is empty.
type DeclineAccessResult struct{}

// RevokeAccessRequest revokes a user's access.
type RevokeAccessRequest struct {
	BudgetId string `json:"budgetId"`
	UserId   string `json:"userId"`
}

func (r RevokeAccessRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId, "userId": r.UserId})
}

// RevokeAccessResult is empty.
type RevokeAccessResult struct{}

// ExcludeAccountRequest / IncludeAccountRequest toggle an account in the budget.
// The request field for the budget id is "id" (not "budgetId") — the exclude/
// include forms carry the budget under "id", and validation reports the blank
// field under "id" to match the frozen wire contract.
type ExcludeAccountRequest struct {
	BudgetId  string `json:"id"`
	AccountId string `json:"accountId"`
}

func (r ExcludeAccountRequest) Validate() error {
	return ValidateBlank(map[string]string{"id": r.BudgetId, "accountId": r.AccountId})
}

// ExcludeAccountResult / IncludeAccountResult are {item: MetaResult}.
type ExcludeAccountResult struct {
	Item MetaResult `json:"item"`
}

// IncludeAccountRequest includes a previously-excluded account. The budget id
// arrives under "id" (see ExcludeAccountRequest).
type IncludeAccountRequest struct {
	BudgetId  string `json:"id"`
	AccountId string `json:"accountId"`
}

func (r IncludeAccountRequest) Validate() error {
	return ValidateBlank(map[string]string{"id": r.BudgetId, "accountId": r.AccountId})
}

// IncludeAccountResult is {item: MetaResult}.
type IncludeAccountResult struct {
	Item MetaResult `json:"item"`
}

// ChangeElementCurrencyRequest changes a budget element's display currency.
type ChangeElementCurrencyRequest struct {
	BudgetId   string `json:"budgetId"`
	ElementId  string `json:"elementId"`
	CurrencyId string `json:"currencyId"`
}

func (r ChangeElementCurrencyRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId, "elementId": r.ElementId, "currencyId": r.CurrencyId})
}

// ChangeElementCurrencyResult is empty.
type ChangeElementCurrencyResult struct{}

// SetLimitRequest sets/clears an element's period limit.
type SetLimitRequest struct {
	BudgetId  string         `json:"budgetId"`
	ElementId string         `json:"elementId"`
	Period    string         `json:"period"`
	Amount    *vo.FlexString `json:"amount" swaggertype:"string"`
}

func (r SetLimitRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId, "elementId": r.ElementId, "period": r.Period})
}

// SetLimitResult is empty.
type SetLimitResult struct{}

// MoveElementListItem is one element move/reorder instruction. The wire shape is
// {id, position, folderId?} — the element is identified by its EXTERNAL id alone
// (no type), matched against the budget element whose external id equals it
// (first-seen wins).
type MoveElementListItem struct {
	Id       string  `json:"id"`
	FolderId *string `json:"folderId"`
	Position int     `json:"position"`
}

// MoveElementListRequest moves/reorders elements.
type MoveElementListRequest struct {
	BudgetId string                `json:"budgetId"`
	Items    []MoveElementListItem `json:"items"`
}

func (r MoveElementListRequest) Validate() error {
	if err := ValidateBlank(map[string]string{"budgetId": r.BudgetId}); err != nil {
		return err
	}
	var fields []errs.FieldError
	for _, it := range r.Items {
		fields = append(fields, validatePositionField("position", it.Position)...)
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// MoveElementListResult is empty.
type MoveElementListResult struct{}

// BudgetTransactionListRequest is the budget transaction-list query.
type BudgetTransactionListRequest struct {
	BudgetId    string  `json:"budgetId"`
	PeriodStart string  `json:"periodStart"`
	CategoryId  *string `json:"categoryId"`
	TagId       *string `json:"tagId"`
	EnvelopeId  *string `json:"envelopeId"`
}

// TxCategoryResult / TxPayeeResult / TxTagResult are the optional embeds.
type TxCategoryResult struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}
type TxPayeeResult struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}
type TxTagResult struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// BudgetTransactionResult is one transaction in the budget transaction list.
type BudgetTransactionResult struct {
	Id          string            `json:"id"`
	Author      UserResult        `json:"author"`
	CurrencyId  string            `json:"currencyId"`
	Amount      string            `json:"amount"`
	Description string            `json:"description"`
	Category    *TxCategoryResult `json:"category"`
	Payee       *TxPayeeResult    `json:"payee"`
	Tag         *TxTagResult      `json:"tag"`
	SpentAt     string            `json:"spentAt"`
}

// GetBudgetTransactionListResult is {items: [...]}.
type GetBudgetTransactionListResult struct {
	Items []BudgetTransactionResult `json:"items"`
}

// Positions are persisted into an int16 column; a value outside this range would
// wrap silently and corrupt ordering, so it is rejected at the edge.
const (
	positionMin = -32768
	positionMax = 32767
)

// validatePositionField reports a single out-of-int16-range position.
func validatePositionField(key string, pos int) []errs.FieldError {
	if pos < positionMin || pos > positionMax {
		return []errs.FieldError{{Key: key, Message: "This value is out of range.", Code: errs.CodeOutOfRange}}
	}
	return nil
}

// ValidateBlank returns a ValidationError listing every blank field with the
// frozen "This value should not be blank." message.
func ValidateBlank(fields map[string]string) error {
	var fe []errs.FieldError
	for key, val := range fields {
		if strings.TrimSpace(val) == "" {
			fe = append(fe, errs.FieldError{Key: key, Message: "This value should not be blank.", Code: errs.CodeIsBlank})
		}
	}
	if len(fe) > 0 {
		return errs.NewValidation("Validation failed", fe...)
	}
	return nil
}
