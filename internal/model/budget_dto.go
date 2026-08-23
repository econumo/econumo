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
	Id          string `json:"id"`
	OwnerUserId string `json:"ownerUserId"`
	Name        string `json:"name"`
	StartedAt   string `json:"startedAt"`
	// EndedAt is the last covered month, "" when the budget is open-ended.
	EndedAt    string         `json:"endedAt"`
	CurrencyId string         `json:"currencyId"`
	IsArchived int            `json:"isArchived"`
	Access     []AccessResult `json:"access"`
}

// BudgetAccountFilter is one member account of the budget as the requester sees
// it: Removable is false once the account has transactions in a closed month,
// because dropping it then would rewrite history the budget's limits stand on.
type BudgetAccountFilter struct {
	Id        string `json:"id"`
	Removable bool   `json:"removable"`
}

// FiltersResult is the budget's period + the requester's member accounts.
type FiltersResult struct {
	PeriodStart string                `json:"periodStart"`
	PeriodEnd   string                `json:"periodEnd"`
	Accounts    []BudgetAccountFilter `json:"accounts"`
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

// LabelSpendResult is one reporting label in the budget's labels block. It has
// no budgeted/available fields on purpose: a label carries no limit and takes no
// part in envelope math, so only its period spend is meaningful. Amounts across
// labels deliberately overlap and do not sum to total spend.
//
// Children break THIS label's own spend down by category and do sum to Spent;
// the overlap is between labels, not within one.
type LabelSpendResult struct {
	Id          string               `json:"id"`
	Name        string               `json:"name"`
	Icon        string               `json:"icon"`
	IsArchived  int                  `json:"isArchived"`
	Spent       string               `json:"spent"`
	OwnerUserId string               `json:"ownerUserId"`
	Children    []ChildElementResult `json:"children"`
}

// StructureResult is the budget's folders + ordered elements + labels.
type StructureResult struct {
	Folders  []BudgetFolderResult  `json:"folders"`
	Elements []ParentElementResult `json:"elements"`
	Labels   []LabelSpendResult    `json:"labels"`
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
	Id         string `json:"id"`
	Name       string `json:"name"`
	StartDate  string `json:"startDate"`
	CurrencyId string `json:"currencyId"`
	// AccountIds are the budget's initial member accounts. At least one owned,
	// non-deleted account is required — enforced as a coded error in the use
	// case, not here, so the wire carries the catalogue code.
	AccountIds []string `json:"accountIds"`
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
	Id         string `json:"id"`
	Name       string `json:"name"`
	CurrencyId string `json:"currencyId"`
	// AccountIds is nil when the client omits the field, which leaves membership
	// untouched; a present list replaces the caller's own member set.
	AccountIds []string `json:"accountIds"`
	// EndDate is nil when the client omits the field (end month untouched);
	// "" clears it, "2006-01-02" sets it (snapped to first-of-month).
	EndDate *string `json:"endDate"`
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

// GetBudgetPlanRequest selects a budget + a month window for the plan read.
// From and Months arrive as raw query strings; the service parses them.
type GetBudgetPlanRequest struct {
	Id     string `json:"id"`
	From   string `json:"from"`
	Months string `json:"months"`
}

func (r GetBudgetPlanRequest) Validate() error {
	return ValidateBlank(map[string]string{"id": r.Id})
}

// PlanCellResult is one (element, month) plan cell. Planned is "" when no
// limit row exists for that month — the frozen empty-cell encoding.
type PlanCellResult struct {
	Actual  string `json:"actual"`
	Planned string `json:"planned"`
}

// PlanChildCellResult is one (envelope child, month) cell — actual only;
// planned lives on the parent, exactly like budgeted on the budget page.
type PlanChildCellResult struct {
	Actual string `json:"actual"`
}

// PlanChildResult is a category nested under an envelope in the plan sheet.
// Cells align index-for-index with BudgetPlanResult.Months.
type PlanChildResult struct {
	Id          string                `json:"id"`
	Type        int                   `json:"type"`
	Name        string                `json:"name"`
	Icon        string                `json:"icon"`
	IsArchived  int                   `json:"isArchived"`
	OwnerUserId string                `json:"ownerUserId"`
	Cells       []PlanChildCellResult `json:"cells"`
}

// PlanElementResult is one plan-sheet row (income and expense alike; the
// element type encodes the side). Cells align with BudgetPlanResult.Months.
type PlanElementResult struct {
	Id          string            `json:"id"`
	Type        int               `json:"type"`
	Name        string            `json:"name"`
	Icon        string            `json:"icon"`
	CurrencyId  string            `json:"currencyId"`
	IsArchived  int               `json:"isArchived"`
	FolderId    *string           `json:"folderId"`
	Position    int               `json:"position"`
	OwnerUserId *string           `json:"ownerUserId"`
	Cells       []PlanCellResult  `json:"cells"`
	Children    []PlanChildResult `json:"children"`
}

// OpeningBalanceResult is one currency's real account balance strictly
// BEFORE the window start (transactions dated < months[0]) — the client-side
// Balance row's seed. This deliberately differs from the budget page's
// startBalance bound (<=): the plan's Balance row arithmetically chains this
// seed to the per-month nets (seed + net(month0) + ...), and month 0's
// actual cells already cover [months[0], months[0]+1mo), so the boundary
// instant must not appear in both or it is double-counted.
type OpeningBalanceResult struct {
	CurrencyId string `json:"currencyId"`
	Amount     string `json:"amount"`
}

// PlanTransferResult is one currency's transfers across the budget boundary
// in one window month: In = moved into included accounts (recipient side's
// amount and currency), Out = moved out of them (source side's). Neither is
// planned; the client nets them into the month's Net and Balance.
type PlanTransferResult struct {
	CurrencyId string `json:"currencyId"`
	In         string `json:"in"`
	Out        string `json:"out"`
}

// PlanMonthTransfersResult is one window month's boundary transfers; Items is
// [] (never null) for a month nothing crossed, ordered budget currency first
// then by currency id.
type PlanMonthTransfersResult struct {
	Period string               `json:"period"`
	Items  []PlanTransferResult `json:"items"`
}

// PlanMonthRatesResult is one window month's average currency rates. Period is
// the REQUESTED month; each rate row reports its own snapped period, exactly
// as the budget page's currencyRates block does.
type PlanMonthRatesResult struct {
	Period string                      `json:"period"`
	Rates  []AverageCurrencyRateResult `json:"rates"`
}

// PlanStructureResult is all folders (income-, expense-sided and neutral) +
// all plan rows.
type PlanStructureResult struct {
	Folders  []BudgetFolderResult `json:"folders"`
	Elements []PlanElementResult  `json:"elements"`
}

// BudgetPlanResult is the full get-budget-plan shape.
type BudgetPlanResult struct {
	Meta            MetaResult                 `json:"meta"`
	Months          []string                   `json:"months"`
	OpeningBalances []OpeningBalanceResult     `json:"openingBalances"`
	CurrencyRates   []PlanMonthRatesResult     `json:"currencyRates"`
	Transfers       []PlanMonthTransfersResult `json:"transfers"`
	Structure       PlanStructureResult        `json:"structure"`
}

// GetBudgetPlanResult is {item: BudgetPlanResult}.
type GetBudgetPlanResult struct {
	Item BudgetPlanResult `json:"item"`
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

// MoveBudgetFolderRequest is the budget move-folder request body. AfterId is the
// id of the folder this one should land immediately after; null means "move to
// the front".
type MoveBudgetFolderRequest struct {
	BudgetId string  `json:"budgetId"`
	Id       string  `json:"id"`
	AfterId  *string `json:"afterId"`
}

func (r MoveBudgetFolderRequest) Validate() error {
	if _, err := vo.ParseId(r.BudgetId); err != nil {
		return ValidateBlank(map[string]string{"budgetId": ""})
	}
	if _, err := vo.ParseId(r.Id); err != nil {
		return err
	}
	if r.AfterId != nil {
		if _, err := vo.ParseId(*r.AfterId); err != nil {
			return err
		}
	}
	return nil
}

// MoveBudgetFolderResult is the budget move-folder response: {}.
type MoveBudgetFolderResult struct{}

// CreateEnvelopeRequest creates an envelope (+ its budget element).
type CreateEnvelopeRequest struct {
	BudgetId   string   `json:"budgetId"`
	Id         string   `json:"id"`
	Name       string   `json:"name"`
	Icon       string   `json:"icon"`
	CurrencyId string   `json:"currencyId"`
	FolderId   *string  `json:"folderId"`
	Categories []string `json:"categories"`
	// Side selects the envelope's (immutable) side: "" or "expense" (default).
	// "income" is reserved for the plan view's income envelopes and currently
	// rejected (see EnvelopeTypeFromSide).
	Side string `json:"side"`
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

// AddAccountRequest / RemoveAccountRequest change a budget's account
// membership. The request field for the budget id is "id" (not "budgetId") —
// the membership forms carry the budget under "id", and validation reports the
// blank field under "id" to match the frozen wire contract.
type AddAccountRequest struct {
	BudgetId  string `json:"id"`
	AccountId string `json:"accountId"`
}

func (r AddAccountRequest) Validate() error {
	return ValidateBlank(map[string]string{"id": r.BudgetId, "accountId": r.AccountId})
}

// AddAccountResult / RemoveAccountResult are {item: MetaResult}.
type AddAccountResult struct {
	Item MetaResult `json:"item"`
}

// RemoveAccountRequest drops an account from the budget. The budget id arrives
// under "id" (see AddAccountRequest).
type RemoveAccountRequest struct {
	BudgetId  string `json:"id"`
	AccountId string `json:"accountId"`
}

func (r RemoveAccountRequest) Validate() error {
	return ValidateBlank(map[string]string{"id": r.BudgetId, "accountId": r.AccountId})
}

// RemoveAccountResult is {item: MetaResult}.
type RemoveAccountResult struct {
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

// MoveElementRequest is the move-element request body. The element is
// identified by its EXTERNAL id (no type discriminator, first match wins).
// AfterId is the element it should land immediately after within FolderId; null
// means "move to the front". FolderId null puts it in the no-folder group.
type MoveElementRequest struct {
	BudgetId string  `json:"budgetId"`
	Id       string  `json:"id"`
	FolderId *string `json:"folderId"`
	AfterId  *string `json:"afterId"`
}

func (r MoveElementRequest) Validate() error {
	if _, err := vo.ParseId(r.BudgetId); err != nil {
		return ValidateBlank(map[string]string{"budgetId": ""})
	}
	if _, err := vo.ParseId(r.Id); err != nil {
		return err
	}
	for _, opt := range []*string{r.FolderId, r.AfterId} {
		if opt == nil || *opt == "" {
			continue
		}
		if _, err := vo.ParseId(*opt); err != nil {
			return err
		}
	}
	return nil
}

// MoveElementResult is the move-element response: {}.
type MoveElementResult struct{}

// BudgetTransactionListRequest is the budget transaction-list query.
type BudgetTransactionListRequest struct {
	BudgetId      string  `json:"budgetId"`
	PeriodStart   string  `json:"periodStart"`
	CategoryId    *string `json:"categoryId"`
	TagId         *string `json:"tagId"`
	EnvelopeId    *string `json:"envelopeId"`
	LabelId       *string `json:"labelId"`
	Uncategorized bool    `json:"uncategorized,omitempty"`
	// Transfers selects the transfers that crossed the budget boundary (one
	// side included, the other not) — the plan sheet's Transfers drill-down.
	// Mutually exclusive with every other selector.
	Transfers bool `json:"transfers,omitempty"`
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
	// Reporting labels attached to the row. Always non-nil ([] when none) so a
	// client can render it without a null check, matching labelIds on the
	// transaction feature's own wire.
	LabelIds []string `json:"labelIds"`
	SpentAt  string   `json:"spentAt"`
	// Direction is present only on rows of the transfers selector: "out" when
	// the included account is the source, "in" when it is the recipient —
	// Amount/CurrencyId are that side's. Omitted on every other list so their
	// bytes are unchanged.
	Direction string `json:"direction,omitempty"`
}

// GetBudgetTransactionListResult is {items: [...]}.
type GetBudgetTransactionListResult struct {
	Items []BudgetTransactionResult `json:"items"`
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
