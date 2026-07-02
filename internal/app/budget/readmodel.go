// Package budget's read model: the row types the budget read repo returns and
// the ReadModel port the BudgetBuilder depends on — the budget reports (account
// balances/report/holdings + per-element spending) and the limit aggregations.
// All money values are raw NUMERIC(19,8) strings; the builder normalizes via
// vo.DecimalNumber.
package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

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

// ReadModel is the budget read port the BudgetBuilder depends on. The infra
// budget read repo implements it with hand-built dynamic-IN queries (the account
// id / category id sets are variadic).
type ReadModel interface {
	// AccountsBalancesOnDate: balance per account with spent_at <= date.
	AccountsBalancesOnDate(ctx context.Context, accountIDs []vo.Id, date time.Time) ([]AccountBalanceRow, error)
	// AccountsBalancesBeforeDate: balance per account with spent_at < date.
	AccountsBalancesBeforeDate(ctx context.Context, accountIDs []vo.Id, date time.Time) ([]AccountBalanceRow, error)
	// AccountsReport: per-account flow totals in [start, end).
	AccountsReport(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]AccountReportRow, error)
	// HoldingsReport: per-currency from/to holdings (same-amount transfers in/out
	// of the account set) in [start, end).
	HoldingsReport(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]HoldingsRow, error)
	// CountSpending: per (category, tag, currency) spending over [start, end) for
	// the given categories + included accounts.
	CountSpending(ctx context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]SpendingRow, error)
	// SummarizedLimits: summed element limits over [start, end) (for budgetedBefore).
	SummarizedLimits(ctx context.Context, budgetID vo.Id, start, end time.Time) ([]SummarizedLimitRow, error)

	// BudgetTransactionsByCategories returns expense transactions (type=0, tag IS
	// NULL) in [start, end) on the given accounts, in the given categories,
	// newest first. Used for the category + envelope transaction lists.
	BudgetTransactionsByCategories(ctx context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]BudgetTransactionRow, error)
	// BudgetTransactionsByTag returns expense transactions (type=0) in [start, end)
	// on the given accounts tagged with tagID, optionally filtered to a category,
	// newest first.
	BudgetTransactionsByTag(ctx context.Context, tagID vo.Id, categoryID *vo.Id, accountIDs []vo.Id, start, end time.Time) ([]BudgetTransactionRow, error)
}
