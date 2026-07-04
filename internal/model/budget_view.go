// View-row shapes the budget read repo/port interfaces exchange with the
// BudgetBuilder (not wire DTOs — no JSON tags). Money values in the row types
// are raw NUMERIC(19,8) strings; the builder normalizes via vo.DecimalNumber.
package model

// AccountBalanceRow is one account's balance as of a date (per-currency via the
// account's currency).
type AccountBalanceRow struct {
	AccountID  string
	CurrencyID string
	Balance    string
}

// AccountReportRow is one account's period totals split by flow type.
type AccountReportRow struct {
	AccountID        string
	CurrencyID       string
	Incomes          string
	TransferIncomes  string
	ExchangeIncomes  string
	Expenses         string
	TransferExpenses string
	ExchangeExpenses string
}

// HoldingsRow is the per-currency from/to holdings totals.
type HoldingsRow struct {
	CurrencyID   string
	FromHoldings string
	ToHoldings   string
}

// SpendingRow is one (category, optional tag, currency) spending total in a
// period (grouped by category_id, tag_id, currency_id).
type SpendingRow struct {
	CategoryID string
	TagID      *string
	CurrencyID string
	Amount     string
}

// SummarizedLimitRow is the summed limit for one element (external id + type)
// over a period.
type SummarizedLimitRow struct {
	ExternalID string
	Type       int16
	Amount     string
}

// BudgetTransactionRow is one transaction in the budget transaction list, with
// the account's currency joined and the optional category/payee/tag ids.
type BudgetTransactionRow struct {
	ID          string
	UserID      string
	CurrencyID  string
	Amount      string
	Description string
	SpentAt     string // raw DATETIME string from the DB
	CategoryID  *string
	PayeeID     *string
	TagID       *string
}

// AccountView is an account as the budget filters builder needs it: id +
// currency + owner.
type AccountView struct {
	ID         string
	CurrencyID string
	OwnerID    string
}

// CategoryMeta is a category's display metadata for the budget structure.
type CategoryMeta struct {
	ID         string
	OwnerID    string
	Name       string
	Icon       string
	IsIncome   bool
	IsArchived bool
}

// TagMeta is a tag's display metadata for the budget structure.
type TagMeta struct {
	ID         string
	OwnerID    string
	Name       string
	IsArchived bool
}

// PayeeMeta is a payee's display metadata (for the budget transaction list).
type PayeeMeta struct {
	ID   string
	Name string
}
