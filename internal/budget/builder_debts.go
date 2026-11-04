package budget

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// debtRow is one debt's in-progress row, shared by the monthly and plan
// builders.
type debtRow struct {
	view    model.DebtView
	sortKey sortkey.Key
}

// debtRows orders the budget's debts by their element's sort key. Rows derive
// from the debts themselves, not the element table, because sync is lazy; a
// debt with no element row yet trails in creation order, the order the first
// sync keys it in.
func debtRows(f filters, options map[string]elementOption) []debtRow {
	out := make([]debtRow, 0, len(f.debtViews))
	for _, v := range f.debtViews {
		out = append(out, debtRow{view: v, sortKey: options[elementKey(v.ID, model.ElementDebt)].sortKey})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ki, kj := out[i].sortKey, out[j].sortKey
		if ki == "" || kj == "" {
			return ki != "" && kj == ""
		}
		if ki != kj {
			return ki < kj
		}
		return out[i].view.ID < out[j].view.ID
	})
	return out
}

// plannable reports whether month lies within the debt's life.
func plannable(v model.DebtView, month time.Time) bool {
	return !month.Before(v.StartMonth) && !month.After(v.EndMonth)
}

// debtTimelines projects every debt of the budget over months. The projection
// takes this budget's plans of the debt from the caller's current month to the
// debt's end, wherever the window is, so the payoff figures agree between
// views; a plan stored outside the debt's life is ignored. Nil while no debt
// source is wired: the upcoming pass unwires it, as its debt rows are dropped.
func (s *Service) debtTimelines(ctx context.Context, b *budgetAggregate, f filters, months []time.Time) (map[string]model.DebtTimeline, error) {
	if s.debts == nil || len(f.debtViews) == 0 {
		return nil, nil
	}
	today := localToday(s.clock.Now(), reqctx.Location(ctx))
	current := firstOfMonth(today)
	last := current
	ids := make([]string, 0, len(f.debtViews))
	for _, v := range f.debtViews {
		ids = append(ids, v.ID)
		if v.EndMonth.After(last) {
			last = v.EndMonth
		}
	}
	rows, err := s.read.LimitsByMonth(ctx, b.budget.ID, current, last.AddDate(0, 1, 0))
	if err != nil {
		return nil, err
	}
	plans := map[string]map[string]string{}
	for _, l := range rows {
		v, ok := f.debtByID[l.ExternalID]
		if !ok || model.ElementType(l.Type) != model.ElementDebt {
			continue
		}
		m, perr := time.Parse(datetime.DateLayout, l.Month)
		if perr != nil || !plannable(v, m) {
			continue
		}
		if plans[v.ID] == nil {
			plans[v.ID] = map[string]string{}
		}
		plans[v.ID][l.Month] = vo.NewDecimal(l.Amount).String()
	}
	return s.debts.Timelines(ctx, model.DebtTimelineRequest{DebtIDs: ids, Months: months, Plans: plans, Today: today})
}

// timelineMonth is the debt's projection for month, zero when there is none.
// Outside the debt's life nothing is scheduled or due, and before its start
// there is no balance or payment count to show yet.
func timelineMonth(tl model.DebtTimeline, v model.DebtView, month time.Time) model.DebtTimelineMonth {
	m, ok := tl.Months[month.Format(datetime.DateLayout)]
	if !ok {
		m = model.DebtTimelineMonth{ClosingBalance: "0"}
	}
	if !plannable(v, month) {
		m.ScheduledTotal, m.StillDue = "0", "0"
	}
	if month.Before(v.StartMonth) {
		m.ClosingBalance, m.PaymentsLeft = "0", 0
	}
	return m
}

func decimalString(s string) string { return vo.NewDecimal(s).String() }

// emitMonthlyDebts renders get-budget's debt rows. A row shows in a month of
// the debt's life, and outside it only when something was paid then or earlier
// months left something over: the Available total counts that carry.
func emitMonthlyDebts(rows []debtRow, month time.Time, limits map[string]budgetedAmount, paid map[string]debtPaid, timelines map[string]model.DebtTimeline) []model.DebtElementResult {
	zero := vo.NewDecimal("0")
	out := []model.DebtElementResult{}
	for _, r := range rows {
		v := r.view
		p := paid[v.ID]
		paidNow, budgetPaid, paidBefore := orZero(p.paid, zero), orZero(p.budgetPaid, zero), orZero(p.paidBefore, zero)
		in := plannable(v, month)
		lim := limits[elementKey(v.ID, model.ElementDebt)]
		budgeted, hasLimit := orZero(lim.budgeted, zero), lim.hasLimit
		if !in {
			budgeted, hasLimit = zero, false
		}
		available := orZero(lim.budgetedBefore, zero).Sub(paidBefore).Add(budgeted).Sub(paidNow)
		if !in && paidNow.IsZero() && available.IsZero() {
			continue
		}
		tl := timelines[v.ID]
		tm := timelineMonth(tl, v, month)
		out = append(out, model.DebtElementResult{
			Id: v.ID, Type: int(model.ElementDebt.Int16()), Name: v.Name, Icon: v.Icon,
			CurrencyId: v.CurrencyID, OwnerUserId: v.OwnerID, Position: len(out),
			Budgeted: budgeted.String(), IsBudgeted: boolToInt(hasLimit),
			Spent: paidNow.String(), BudgetSpent: budgetPaid.String(), Available: available.String(),
			ClosingBalance: decimalString(tm.ClosingBalance), PaymentsLeft: tm.PaymentsLeft,
			ScheduledTotal: decimalString(tm.ScheduledTotal), Upcoming: decimalString(tm.StillDue),
			PayoffDate: tl.PayoffDate, BasePayoffDate: tl.BasePayoffDate,
			OverpaymentLeft: decimalString(tl.OverpaymentLeft), BaseOverpaymentLeft: decimalString(tl.BaseOverpaymentLeft),
			IsPaidOff: boolToInt(v.PaidOff), IsPlannable: boolToInt(in),
		})
	}
	return out
}

// emitPlanDebts renders the plan's debt rows: a row shows when a window month
// lies in the debt's life or holds a payment; a cell outside the debt's life
// shows no plan.
func emitPlanDebts(rows []debtRow, months []time.Time, plannedFor func(string) []string, get func(string) vo.DecimalNumber, timelines map[string]model.DebtTimeline) []model.PlanDebtElementResult {
	out := []model.PlanDebtElementResult{}
	for _, r := range rows {
		v := r.view
		index := elementKey(v.ID, model.ElementDebt)
		planned := plannedFor(index)
		tl := timelines[v.ID]
		visible := false
		cells := make([]model.PlanDebtCellResult, len(months))
		for i, m := range months {
			actual := get(planKey(i, index))
			in := plannable(v, m)
			visible = visible || in || !actual.IsZero()
			plan := planned[i]
			if !in {
				plan = ""
			}
			tm := timelineMonth(tl, v, m)
			cells[i] = model.PlanDebtCellResult{
				Actual: actual.String(), Planned: plan,
				ClosingBalance: decimalString(tm.ClosingBalance), PaymentsLeft: tm.PaymentsLeft,
				ScheduledTotal: decimalString(tm.ScheduledTotal), Upcoming: decimalString(tm.StillDue),
				IsPlannable: boolToInt(in),
			}
		}
		if !visible {
			continue
		}
		out = append(out, model.PlanDebtElementResult{
			Id: v.ID, Type: int(model.ElementDebt.Int16()), Name: v.Name, Icon: v.Icon,
			CurrencyId: v.CurrencyID, OwnerUserId: v.OwnerID, Position: len(out),
			PayoffDate: tl.PayoffDate, BasePayoffDate: tl.BasePayoffDate,
			OverpaymentLeft: decimalString(tl.OverpaymentLeft), BaseOverpaymentLeft: decimalString(tl.BaseOverpaymentLeft),
			IsPaidOff: boolToInt(v.PaidOff), Cells: cells,
		})
	}
	return out
}
