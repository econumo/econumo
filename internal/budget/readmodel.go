// This file holds the ReadModel port the BudgetBuilder depends on. The infra
// budget read repo implements it with hand-built dynamic-IN queries (the
// account id / category id sets are variadic). Row shapes live in
// internal/model (budget_view.go).
package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ReadModel is the budget read port the BudgetBuilder depends on. The infra
// budget read repo implements it with hand-built dynamic-IN queries (the account
// id / category id sets are variadic).
type ReadModel interface {
	// AccountsBalancesOnDate: balance per account with spent_at <= date.
	AccountsBalancesOnDate(ctx context.Context, accountIDs []vo.Id, date time.Time) ([]model.AccountBalanceRow, error)
	// AccountsBalancesBeforeDate: balance per account with spent_at < date.
	AccountsBalancesBeforeDate(ctx context.Context, accountIDs []vo.Id, date time.Time) ([]model.AccountBalanceRow, error)
	// AccountsReport: per-account flow totals in [start, end).
	AccountsReport(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.AccountReportRow, error)
	// HoldingsReport: per-currency from/to holdings (same-amount transfers in/out
	// of the account set) in [start, end).
	HoldingsReport(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.HoldingsRow, error)
	// CountSpending: per (category, tag, currency) spending over [start, end) for
	// the given categories + included accounts.
	CountSpending(ctx context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]model.SpendingRow, error)
	// SummarizedLimits: summed element limits over [start, end) (for budgetedBefore).
	SummarizedLimits(ctx context.Context, budgetID vo.Id, start, end time.Time) ([]model.SummarizedLimitRow, error)

	// BudgetTransactionsByCategories returns expense transactions (type=0, tag IS
	// NULL) in [start, end) on the given accounts, in the given categories,
	// newest first. Used for the category + envelope transaction lists.
	BudgetTransactionsByCategories(ctx context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)
	// BudgetTransactionsByTag returns expense transactions (type=0) in [start, end)
	// on the given accounts tagged with tagID, optionally filtered to a category,
	// newest first.
	BudgetTransactionsByTag(ctx context.Context, tagID vo.Id, categoryID *vo.Id, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)
}
