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

// plannable reports whether the period keyed month lies within the debt's
// life, on a budget whose periods start on startDay. A debt not set up has no
// last period yet: it is plannable from its first period on.
func plannable(v model.DebtView, startDay int, month time.Time) bool {
	first, last := v.PlanningBounds(startDay)
	return !month.Before(first) && (last.IsZero() || !month.After(last))
}

// debtTimelines projects every debt of the budget over months. The projection
// takes this budget's plans of the debt from the caller's current period to
// the debt's end, wherever the window is, so the payoff figures agree between
// views; a plan stored outside the debt's life is ignored. Nil while no debt
// source is wired: the upcoming pass unwires it, as its debt rows are dropped.
// A debt not set up has nothing to project and is left out.
func (s *Service) debtTimelines(ctx context.Context, b *budgetAggregate, f filters, months []time.Time) (map[string]model.DebtTimeline, error) {
	if s.debts == nil {
		return nil, nil
	}
	today := localToday(s.clock.Now(), reqctx.Location(ctx))
	current := model.PeriodOf(today, f.startDay)
	last := current
	ids := make([]string, 0, len(f.debtViews))
	for _, v := range f.debtViews {
		if !v.SetUp {
			continue
		}
		ids = append(ids, v.ID)
		if _, end := v.PlanningBounds(f.startDay); end.After(last) {
			last = end
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.read.LimitsByMonth(ctx, b.budget.ID, current, last.AddDate(0, 1, 0))
	if err != nil {
		return nil, err
	}
	plans := map[string]map[string]string{}
	for _, l := range rows {
		v, ok := f.debtByID[l.ExternalID]
		if !ok || !v.SetUp || model.ElementType(l.Type) != model.ElementDebt {
			continue
		}
		m, perr := time.Parse(datetime.DateLayout, l.Month)
		if perr != nil || !plannable(v, f.startDay, m) {
			continue
		}
		if plans[v.ID] == nil {
			plans[v.ID] = map[string]string{}
		}
		plans[v.ID][l.Month] = vo.NewDecimal(l.Amount).String()
	}
	return s.debts.Timelines(ctx, model.DebtTimelineRequest{Scope: b.debtScope(), DebtIDs: ids, Months: months, Plans: plans, Today: today, PeriodStartDay: f.startDay})
}

// timelineMonth is the debt's projection for month, zero when there is none.
// Outside the debt's life nothing is scheduled or due, and before its start
// there is no balance or payment count to show yet.
func timelineMonth(tl model.DebtTimeline, v model.DebtView, startDay int, month time.Time) model.DebtTimelineMonth {
	m, ok := tl.Months[month.Format(datetime.DateLayout)]
	if !ok {
		m = model.DebtTimelineMonth{ClosingBalance: "0"}
	}
	if !plannable(v, startDay, month) {
		m.ScheduledTotal, m.StillDue = "0", "0"
	}
	if first, _ := v.PlanningBounds(startDay); month.Before(first) {
		m.ClosingBalance, m.PaymentsLeft = "0", 0
	}
	return m
}

func decimalString(s string) string { return vo.NewDecimal(s).String() }

// debtMonthFigures are a debt row's projected figures for one month.
type debtMonthFigures struct {
	closingBalance           string
	paymentsLeft             int
	scheduledTotal, stillDue string
}

// monthFigures is the row's month of the projection; a debt not set up has
// no balance to count down and nothing scheduled.
func monthFigures(tl model.DebtTimeline, v model.DebtView, startDay int, month time.Time) debtMonthFigures {
	if !v.SetUp {
		return debtMonthFigures{scheduledTotal: "0", stillDue: "0"}
	}
	tm := timelineMonth(tl, v, startDay, month)
	return debtMonthFigures{
		closingBalance: decimalString(tm.ClosingBalance), paymentsLeft: tm.PaymentsLeft,
		scheduledTotal: decimalString(tm.ScheduledTotal), stillDue: decimalString(tm.StillDue),
	}
}

// debtPayoffFigures are the payoff date and overpayment, with and without
// the planned extras; all "" for a debt not set up.
type debtPayoffFigures struct {
	payoffDate, basePayoffDate           string
	overpaymentLeft, baseOverpaymentLeft string
}

func payoffFigures(tl model.DebtTimeline, v model.DebtView) debtPayoffFigures {
	if !v.SetUp {
		return debtPayoffFigures{}
	}
	return debtPayoffFigures{
		payoffDate: tl.PayoffDate, basePayoffDate: tl.BasePayoffDate,
		overpaymentLeft: decimalString(tl.OverpaymentLeft), baseOverpaymentLeft: decimalString(tl.BaseOverpaymentLeft),
	}
}

// emitMonthlyDebts renders get-budget's debt rows. A row shows in a month of
// the debt's life, and outside it only when something was paid then or earlier
// months left something over: the Available total counts that carry.
func emitMonthlyDebts(rows []debtRow, month time.Time, startDay int, limits map[string]budgetedAmount, paid map[string]debtPaid, timelines map[string]model.DebtTimeline) []model.DebtElementResult {
	zero := vo.NewDecimal("0")
	out := []model.DebtElementResult{}
	for _, r := range rows {
		v := r.view
		p := paid[v.ID]
		paidNow, budgetPaid, paidBefore := orZero(p.paid, zero), orZero(p.budgetPaid, zero), orZero(p.paidBefore, zero)
		in := plannable(v, startDay, month)
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
		mf, pf := monthFigures(tl, v, startDay, month), payoffFigures(tl, v)
		out = append(out, model.DebtElementResult{
			Id: v.ID, Type: int(model.ElementDebt.Int16()), Name: v.Name, Icon: v.Icon,
			CurrencyId: v.CurrencyID, Position: len(out),
			Budgeted: budgeted.String(), IsBudgeted: boolToInt(hasLimit),
			Spent: paidNow.String(), BudgetSpent: budgetPaid.String(), Available: available.String(),
			ClosingBalance: mf.closingBalance, PaymentsLeft: mf.paymentsLeft,
			ScheduledTotal: mf.scheduledTotal, Upcoming: mf.stillDue,
			PayoffDate: pf.payoffDate, BasePayoffDate: pf.basePayoffDate,
			OverpaymentLeft: pf.overpaymentLeft, BaseOverpaymentLeft: pf.baseOverpaymentLeft,
			IsPaidOff: boolToInt(v.PaidOff), IsPlannable: boolToInt(in), IsSetUp: boolToInt(v.SetUp),
		})
	}
	return out
}

// emitPlanDebts renders the plan's debt rows: a row shows when a window month
// lies in the debt's life or holds a payment; a cell outside the debt's life
// shows no plan.
func emitPlanDebts(rows []debtRow, months []time.Time, startDay int, plannedFor func(string) []string, get func(string) vo.DecimalNumber, timelines map[string]model.DebtTimeline) []model.PlanDebtElementResult {
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
			in := plannable(v, startDay, m)
			visible = visible || in || !actual.IsZero()
			plan := planned[i]
			if !in {
				plan = ""
			}
			mf := monthFigures(tl, v, startDay, m)
			cells[i] = model.PlanDebtCellResult{
				Actual: actual.String(), Planned: plan,
				ClosingBalance: mf.closingBalance, PaymentsLeft: mf.paymentsLeft,
				ScheduledTotal: mf.scheduledTotal, Upcoming: mf.stillDue,
				IsPlannable: boolToInt(in),
			}
		}
		if !visible {
			continue
		}
		pf := payoffFigures(tl, v)
		out = append(out, model.PlanDebtElementResult{
			Id: v.ID, Type: int(model.ElementDebt.Int16()), Name: v.Name, Icon: v.Icon,
			CurrencyId: v.CurrencyID, Position: len(out),
			PayoffDate: pf.payoffDate, BasePayoffDate: pf.basePayoffDate,
			OverpaymentLeft: pf.overpaymentLeft, BaseOverpaymentLeft: pf.baseOverpaymentLeft,
			IsPaidOff: boolToInt(v.PaidOff), IsSetUp: boolToInt(v.SetUp), Cells: cells,
		})
	}
	return out
}
