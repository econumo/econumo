// A real transaction dated after the caller's today has not happened yet: the
// budget's spent, received, saved and balance figures leave it out, and the
// upcoming pass counts it next to the projected recurring occurrences.
package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// localTomorrow is the start of the caller's tomorrow as a UTC-tagged wall
// clock, the form spent_at is stored in: the first instant that has not
// happened yet.
func localTomorrow(ctx context.Context, now time.Time) time.Time {
	return localToday(now, reqctx.Location(ctx)).AddDate(0, 0, 1)
}

// windowRead answers the money reads from the stored transactions dated in
// [from, until); a zero bound is open. Limits, labels and the membership
// activity read are not about money that moved, so they pass through.
type windowRead struct {
	ReadModel
	from, until time.Time
}

var _ ReadModel = windowRead{}

// asOfTomorrow is s reading only the transactions dated before the caller's
// tomorrow.
func (s *Service) asOfTomorrow(ctx context.Context) *Service {
	sp := *s
	sp.read = windowRead{ReadModel: s.read, until: localTomorrow(ctx, s.clock.Now())}
	return &sp
}

// clamp narrows [start, end) to the window; ok is false when nothing is left,
// so the read is skipped rather than sent to the database.
func (w windowRead) clamp(start, end time.Time) (time.Time, time.Time, bool) {
	if !w.from.IsZero() && start.Before(w.from) {
		start = w.from
	}
	if !w.until.IsZero() && end.After(w.until) {
		end = w.until
	}
	return start, end, start.Before(end)
}

// AccountsBalancesBeforeDate is the balance change of the window's
// transactions dated before date.
func (w windowRead) AccountsBalancesBeforeDate(ctx context.Context, accountIDs []vo.Id, date time.Time) ([]model.AccountBalanceRow, error) {
	if !w.until.IsZero() && date.After(w.until) {
		date = w.until
	}
	rows, err := w.ReadModel.AccountsBalancesBeforeDate(ctx, accountIDs, date)
	if err != nil || w.from.IsZero() {
		return rows, err
	}
	if !date.After(w.from) {
		for i := range rows {
			rows[i].Balance = "0"
		}
		return rows, nil
	}
	before, err := w.ReadModel.AccountsBalancesBeforeDate(ctx, accountIDs, w.from)
	if err != nil {
		return nil, err
	}
	sub := make(map[string]vo.DecimalNumber, len(before))
	for _, b := range before {
		sub[b.AccountID] = vo.NewDecimal(b.Balance)
	}
	for i := range rows {
		if b, ok := sub[rows[i].AccountID]; ok {
			rows[i].Balance = vo.NewDecimal(rows[i].Balance).Sub(b).String()
		}
	}
	return rows, nil
}

func (w windowRead) AccountsReport(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.AccountReportRow, error) {
	start, end, ok := w.clamp(start, end)
	if !ok {
		return nil, nil
	}
	return w.ReadModel.AccountsReport(ctx, accountIDs, start, end)
}

func (w windowRead) HoldingsReport(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.HoldingsRow, error) {
	start, end, ok := w.clamp(start, end)
	if !ok {
		return nil, nil
	}
	return w.ReadModel.HoldingsReport(ctx, accountIDs, start, end)
}

func (w windowRead) CountSpending(ctx context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]model.SpendingRow, error) {
	start, end, ok := w.clamp(start, end)
	if !ok {
		return nil, nil
	}
	return w.ReadModel.CountSpending(ctx, categoryIDs, accountIDs, start, end)
}

func (w windowRead) SpendingByMonth(ctx context.Context, categoryIDs, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.MonthlySpendingRow, error) {
	from, to, ok := w.clamp(from, to)
	if !ok {
		return nil, nil
	}
	return w.ReadModel.SpendingByMonth(ctx, categoryIDs, accountIDs, from, to, startDay)
}

func (w windowRead) IncomeByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.MonthlyIncomeRow, error) {
	from, to, ok := w.clamp(from, to)
	if !ok {
		return nil, nil
	}
	return w.ReadModel.IncomeByMonth(ctx, accountIDs, from, to, startDay)
}

func (w windowRead) TransfersByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.MonthlyTransferRow, error) {
	from, to, ok := w.clamp(from, to)
	if !ok {
		return nil, nil
	}
	return w.ReadModel.TransfersByMonth(ctx, accountIDs, from, to, startDay)
}

func (w windowRead) AccountsNetByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.SavingsMonthRow, error) {
	from, to, ok := w.clamp(from, to)
	if !ok {
		return nil, nil
	}
	return w.ReadModel.AccountsNetByMonth(ctx, accountIDs, from, to, startDay)
}

func (w windowRead) AccountsIncomeExpenseByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.SavingsMonthRow, error) {
	from, to, ok := w.clamp(from, to)
	if !ok {
		return nil, nil
	}
	return w.ReadModel.AccountsIncomeExpenseByMonth(ctx, accountIDs, from, to, startDay)
}

func (w windowRead) CountSpendingByLabel(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.LabelSpendingRow, error) {
	start, end, ok := w.clamp(start, end)
	if !ok {
		return nil, nil
	}
	return w.ReadModel.CountSpendingByLabel(ctx, accountIDs, start, end)
}

// upcomingRead is what the upcoming pass of get-budget and get-budget-plan
// reads: the projected recurring occurrences plus the stored transactions
// dated from the caller's tomorrow to the window's end. It is nil when the
// window ended before the caller's current month or holds neither.
func (s *Service) upcomingRead(ctx context.Context, f filters, now time.Time) (ReadModel, error) {
	if !upcomingShown(ctx, f.periodEnd, now) {
		return nil, nil
	}
	pr, err := s.newProjectedRead(ctx, f, f.periodEnd)
	if err != nil {
		return nil, err
	}
	tomorrow := localTomorrow(ctx, now)
	var dated []vo.Id
	if len(f.includedAccountIDs) > 0 && f.periodEnd.After(tomorrow) {
		from := tomorrow
		if f.periodStart.After(from) {
			from = f.periodStart
		}
		if dated, err = s.read.AccountsWithTransactions(ctx, f.includedAccountIDs, from, f.periodEnd); err != nil {
			return nil, err
		}
	}
	switch {
	case len(dated) == 0 && pr == nil:
		return nil, nil
	case len(dated) == 0:
		return pr, nil
	}
	return unionRead{ReadModel: windowRead{ReadModel: s.read, from: tomorrow}, rec: pr}, nil
}

// unionRead adds the projected recurring occurrences (rec, possibly nil) to
// the embedded future-dated read, summing rows that share a key. Only the
// aggregates the builders read are merged: the transaction lists answer from
// the embedded read alone, since get-transaction-list's upcoming[] stays
// recurring-only and reads the projection directly.
type unionRead struct {
	ReadModel
	rec *projectedRead
}

// mergeRows appends b's rows to a, adding a row whose key a already holds
// into it; first-seen key order is kept.
func mergeRows[R any, K comparable](a, b []R, key func(R) K, add func(x, y R) R) []R {
	if len(b) == 0 {
		return a
	}
	at := make(map[K]int, len(a)+len(b))
	out := make([]R, 0, len(a)+len(b))
	for _, r := range append(append([]R{}, a...), b...) {
		k := key(r)
		if i, ok := at[k]; ok {
			out[i] = add(out[i], r)
			continue
		}
		at[k] = len(out)
		out = append(out, r)
	}
	return out
}

func addAmounts(x, y string) string { return vo.NewDecimal(x).Add(vo.NewDecimal(y)).String() }

func (u unionRead) AccountsBalancesBeforeDate(ctx context.Context, accountIDs []vo.Id, date time.Time) ([]model.AccountBalanceRow, error) {
	a, err := u.ReadModel.AccountsBalancesBeforeDate(ctx, accountIDs, date)
	if err != nil || u.rec == nil {
		return a, err
	}
	b, err := u.rec.AccountsBalancesBeforeDate(ctx, accountIDs, date)
	if err != nil {
		return nil, err
	}
	return mergeRows(a, b, func(r model.AccountBalanceRow) string { return r.AccountID },
		func(x, y model.AccountBalanceRow) model.AccountBalanceRow {
			x.Balance = addAmounts(x.Balance, y.Balance)
			return x
		}), nil
}

func (u unionRead) CountSpending(ctx context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]model.SpendingRow, error) {
	a, err := u.ReadModel.CountSpending(ctx, categoryIDs, accountIDs, start, end)
	if err != nil || u.rec == nil {
		return a, err
	}
	b, err := u.rec.CountSpending(ctx, categoryIDs, accountIDs, start, end)
	if err != nil {
		return nil, err
	}
	return mergeRows(a, b, func(r model.SpendingRow) spendingKey {
		return spendingKey{category: deref(r.CategoryID), tag: deref(r.TagID), currency: r.CurrencyID}
	}, func(x, y model.SpendingRow) model.SpendingRow {
		x.Amount = addAmounts(x.Amount, y.Amount)
		return x
	}), nil
}

func (u unionRead) SpendingByMonth(ctx context.Context, categoryIDs, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.MonthlySpendingRow, error) {
	a, err := u.ReadModel.SpendingByMonth(ctx, categoryIDs, accountIDs, from, to, startDay)
	if err != nil || u.rec == nil {
		return a, err
	}
	b, err := u.rec.SpendingByMonth(ctx, categoryIDs, accountIDs, from, to, startDay)
	if err != nil {
		return nil, err
	}
	return mergeRows(a, b, func(r model.MonthlySpendingRow) spendingKey {
		return spendingKey{month: r.Month, category: deref(r.CategoryID), tag: deref(r.TagID), currency: r.CurrencyID}
	}, func(x, y model.MonthlySpendingRow) model.MonthlySpendingRow {
		x.Amount = addAmounts(x.Amount, y.Amount)
		return x
	}), nil
}

func (u unionRead) CountSpendingByLabel(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.LabelSpendingRow, error) {
	a, err := u.ReadModel.CountSpendingByLabel(ctx, accountIDs, start, end)
	if err != nil || u.rec == nil {
		return a, err
	}
	b, err := u.rec.CountSpendingByLabel(ctx, accountIDs, start, end)
	if err != nil {
		return nil, err
	}
	type key struct{ label, category, currency string }
	return mergeRows(a, b, func(r model.LabelSpendingRow) key {
		return key{r.LabelID, deref(r.CategoryID), r.CurrencyID}
	}, func(x, y model.LabelSpendingRow) model.LabelSpendingRow {
		x.Amount = addAmounts(x.Amount, y.Amount)
		return x
	}), nil
}

func (u unionRead) IncomeByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.MonthlyIncomeRow, error) {
	a, err := u.ReadModel.IncomeByMonth(ctx, accountIDs, from, to, startDay)
	if err != nil || u.rec == nil {
		return a, err
	}
	b, err := u.rec.IncomeByMonth(ctx, accountIDs, from, to, startDay)
	if err != nil {
		return nil, err
	}
	type key struct{ month, category, currency string }
	return mergeRows(a, b, func(r model.MonthlyIncomeRow) key {
		return key{r.Month, deref(r.CategoryID), r.CurrencyID}
	}, func(x, y model.MonthlyIncomeRow) model.MonthlyIncomeRow {
		x.Amount = addAmounts(x.Amount, y.Amount)
		return x
	}), nil
}

func (u unionRead) TransfersByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.MonthlyTransferRow, error) {
	a, err := u.ReadModel.TransfersByMonth(ctx, accountIDs, from, to, startDay)
	if err != nil || u.rec == nil {
		return a, err
	}
	b, err := u.rec.TransfersByMonth(ctx, accountIDs, from, to, startDay)
	if err != nil {
		return nil, err
	}
	type key struct{ month, currency string }
	return mergeRows(a, b, func(r model.MonthlyTransferRow) key { return key{r.Month, r.CurrencyID} },
		func(x, y model.MonthlyTransferRow) model.MonthlyTransferRow {
			x.In, x.Out = addAmounts(x.In, y.In), addAmounts(x.Out, y.Out)
			return x
		}), nil
}

type accountMonthKey struct{ account, month string }

func mergeAccountMonths(a, b []model.SavingsMonthRow) []model.SavingsMonthRow {
	return mergeRows(a, b, func(r model.SavingsMonthRow) accountMonthKey { return accountMonthKey{r.AccountID, r.Month} },
		func(x, y model.SavingsMonthRow) model.SavingsMonthRow {
			x.Amount = addAmounts(x.Amount, y.Amount)
			return x
		})
}

func (u unionRead) AccountsNetByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.SavingsMonthRow, error) {
	a, err := u.ReadModel.AccountsNetByMonth(ctx, accountIDs, from, to, startDay)
	if err != nil || u.rec == nil {
		return a, err
	}
	b, err := u.rec.AccountsNetByMonth(ctx, accountIDs, from, to, startDay)
	if err != nil {
		return nil, err
	}
	return mergeAccountMonths(a, b), nil
}

func (u unionRead) AccountsIncomeExpenseByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.SavingsMonthRow, error) {
	a, err := u.ReadModel.AccountsIncomeExpenseByMonth(ctx, accountIDs, from, to, startDay)
	if err != nil || u.rec == nil {
		return a, err
	}
	b, err := u.rec.AccountsIncomeExpenseByMonth(ctx, accountIDs, from, to, startDay)
	if err != nil {
		return nil, err
	}
	return mergeAccountMonths(a, b), nil
}
