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
	// nil when the transaction has no category (shown as Uncategorized).
	CategoryID *string
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

// MonthlySpendingRow is one (month, category?, tag?, currency) expense total
// for the plan sheet. Month is the first-of-month TEXT "YYYY-MM-01", rendered
// identically by both engines so the wire output stays byte-identical.
type MonthlySpendingRow struct {
	Month      string
	CategoryID *string
	TagID      *string
	CurrencyID string
	Amount     string
}

// MonthlyIncomeRow is one (month, category?, currency) income total.
// CategoryID nil = uncategorized income.
type MonthlyIncomeRow struct {
	Month      string
	CategoryID *string
	CurrencyID string
	Amount     string
}

// MonthlyTransferRow is one (month, currency) total of transfers that crossed
// the budget boundary: In sums transfers INTO the included set (recipient
// inside, source outside) in the recipient account's currency; Out sums
// transfers OUT of it in the source account's currency. Either side may be
// "0" when only the other direction moved money that month.
type MonthlyTransferRow struct {
	Month      string
	CurrencyID string
	In         string
	Out        string
}

// MonthlyLimitRow is one element's limit in one month (the plan sheet's
// planned cells), keyed by the element's external id + stored type.
type MonthlyLimitRow struct {
	ExternalID string
	Type       int16
	Month      string
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
	// Direction is set only on rows from BudgetTransactionsTransfers: "out"
	// when the included account is the source, "in" when it is the recipient.
	// Amount/CurrencyID are then the included side's figures.
	Direction string
}

// LabelSpendingRow is one (label, category, currency) spending total in a
// period. Unlike SpendingRow this is NOT grouped alongside a tag: a
// transaction with N labels contributes N rows, each carrying the FULL
// amount, so these totals deliberately overlap and must never feed envelope
// math (see CountSpendingByLabel). CategoryID is nil when the transaction has
// no category.
type LabelSpendingRow struct {
	LabelID    string
	CategoryID *string
	CurrencyID string
	Amount     string
}

// LabelMeta is a label's display metadata for the budget view.
type LabelMeta struct {
	ID      string
	OwnerID string
	Name    string
	Icon    string
	// SortKey decides the label block's order; it never leaves the server (the
	// labels block carries no position field at all).
	SortKey    string
	IsArchived bool
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
