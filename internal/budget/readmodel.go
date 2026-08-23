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
	// SpendingByMonth: expense spending per (month, category/tag, currency)
	// over [from, to) — the plan sheet's actual-expense cells in ONE query for
	// the whole window. Bucketing matches CountSpending (rows outside the
	// category set are dropped except NULL-category rows; a tag_id row keeps
	// its tag).
	SpendingByMonth(ctx context.Context, categoryIDs, accountIDs []vo.Id, from, to time.Time) ([]model.MonthlySpendingRow, error)
	// IncomeByMonth: income (type=1) per (month, category, currency) over
	// [from, to) on the given accounts. Deliberately NO category filter: the
	// plan builder files rows whose category is not a participant income
	// category — and NULL-category rows — under the income-side Uncategorized
	// row, so no income ever silently vanishes from the Balance projection.
	IncomeByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time) ([]model.MonthlyIncomeRow, error)
	// TransfersByMonth: transfers (type=2) that crossed the budget boundary,
	// per (month, currency) over [from, to) — the plan sheet's Transfers
	// line. "In" = recipient in accountIDs and source not, in the recipient
	// account's currency; "Out" = the mirror, in the source account's
	// currency. Transfers with both or neither side included are not rows.
	// Cross-currency transfers count (each side is measured in its own
	// account's currency, so no same-amount guard is needed).
	TransfersByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time) ([]model.MonthlyTransferRow, error)
	// LimitsByMonth: every element limit of the budget in [from, to) as
	// (external_id, type, month, amount) — the plan sheet's planned cells.
	LimitsByMonth(ctx context.Context, budgetID vo.Id, from, to time.Time) ([]model.MonthlyLimitRow, error)

	// BudgetTransactionsByCategories returns expense transactions (type=0, tag IS
	// NULL) in [start, end) on the given accounts, in the given categories,
	// newest first. Used for the category + envelope transaction lists.
	BudgetTransactionsByCategories(ctx context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)
	// BudgetTransactionsByTag returns expense transactions (type=0) in [start, end)
	// on the given accounts tagged with tagID, newest first. categoryID and
	// uncategorized are mutually exclusive narrowing modes: categoryID non-nil
	// narrows to that category; uncategorized true narrows to category_id IS
	// NULL; both nil/false leaves the tag's transactions unnarrowed by category.
	BudgetTransactionsByTag(ctx context.Context, tagID vo.Id, categoryID *vo.Id, uncategorized bool, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)
	// BudgetTransactionsUncategorized returns expense transactions (type=0) in
	// [start, end) on the given accounts with no category and no tag, newest
	// first. Backs the top-level "uncategorized" element's drill-down.
	BudgetTransactionsUncategorized(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)
	// BudgetTransactionsTransfers returns transfers (type=2) in [start, end)
	// with exactly one side in accountIDs, newest first. Amount/CurrencyID
	// are the included side's; Direction says which side that is. Backs the
	// plan sheet's Transfers drill-down (the counterpart of TransfersByMonth).
	BudgetTransactionsTransfers(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)
	// BudgetTransactionsByLabel returns expense transactions (type=0) in
	// [start, end) on the given accounts carrying labelID, newest first. The
	// link is many-to-many (transactions_labels), unlike the single tag_id
	// column BudgetTransactionsByTag compares against.
	BudgetTransactionsByLabel(ctx context.Context, labelID vo.Id, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)
	// BudgetTransactionsByLabelAndCategory narrows BudgetTransactionsByLabel to
	// one category. Backs the per-category child rows of an expanded reporting-
	// tag folder.
	BudgetTransactionsByLabelAndCategory(ctx context.Context, labelID, categoryID vo.Id, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)
	// BudgetTransactionsByLabelUncategorized narrows BudgetTransactionsByLabel
	// to category_id IS NULL - that folder's uncategorized child row.
	BudgetTransactionsByLabelUncategorized(ctx context.Context, labelID vo.Id, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)

	// CountSpendingByLabel: per (label, currency) spending over [start, end) for
	// the given accounts. Deliberately separate from CountSpending: that query
	// assumes one bucket per row, and joining the many-to-many
	// transactions_labels there would fan one transaction into N rows and
	// double-count the category/tag/currency bucket totals. Here the fan-out is
	// the point: a transaction with two labels contributes its full amount to
	// each.
	CountSpendingByLabel(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.LabelSpendingRow, error)
	// LabelsForUsers returns label metadata keyed by label id for the given
	// owners (the budget's account owners).
	LabelsForUsers(ctx context.Context, userIDs []vo.Id) (map[string]model.LabelMeta, error)

	// AccountsWithTransactions returns the subset of accountIDs (input order)
	// that appear as source or transfer recipient on any transaction with
	// start <= spent_at < end. Drives the membership removal rule.
	AccountsWithTransactions(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]vo.Id, error)
}
