package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// debtRowKey is the elementKey of the budget row a debt is attached to.
func debtRowKey(v model.DebtView) string {
	typ := model.ElementCategory
	if v.ElementType == model.DebtElementEnvelope {
		typ = model.ElementEnvelope
	}
	return elementKey(v.ElementID, typ)
}

// debtsInForce maps each element row to the debt in force in the period: one
// whose first period is not after it and, once paid off, whose end period is
// not before it. One element's debts never overlap in time, but an old debt
// ended and a new one started in the same period are both in force there; the
// newer one is shown.
func debtsInForce(views []model.DebtView, startDay int, period time.Time) map[string]model.DebtView {
	out := map[string]model.DebtView{}
	for _, v := range views {
		if first, _ := v.PlanningBounds(startDay); period.Before(first) {
			continue
		}
		if v.Until != nil && period.After(model.PeriodOf(*v.Until, startDay)) {
			continue
		}
		out[debtRowKey(v)] = v
	}
	return out
}

// activeDebts maps each element row to its active (not paid-off) debt.
func activeDebts(views []model.DebtView) map[string]model.DebtView {
	out := map[string]model.DebtView{}
	for _, v := range views {
		if v.Until == nil {
			out[debtRowKey(v)] = v
		}
	}
	return out
}

// plannable reports whether the period keyed month lies within the debt's
// life, on a budget whose periods start on startDay. A debt not set up has no
// last period yet: it is plannable from its first period on.
func plannable(v model.DebtView, startDay int, month time.Time) bool {
	first, last := v.PlanningBounds(startDay)
	return !month.Before(first) && (last.IsZero() || !month.After(last))
}

// debtTimelines projects the debts over months. The projection takes the
// plans of each debt's element row from the caller's current period to the
// debt's end, wherever the window is, so the payoff figures agree between
// views; a plan outside the debt's life is an ordinary plan. Nil while no debt
// source is wired: the upcoming pass unwires it. A debt not set up has
// nothing to project and is left out.
func (s *Service) debtTimelines(ctx context.Context, b *budgetAggregate, f filters, views []model.DebtView, months []time.Time) (map[string]model.DebtTimeline, error) {
	if s.debts == nil {
		return nil, nil
	}
	today := localToday(s.clock.Now(), reqctx.Location(ctx))
	current := model.PeriodOf(today, f.startDay)
	last := current
	ids := make([]string, 0, len(views))
	byRow := map[string][]model.DebtView{}
	for _, v := range views {
		if !v.SetUp {
			continue
		}
		ids = append(ids, v.ID)
		byRow[debtRowKey(v)] = append(byRow[debtRowKey(v)], v)
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
		m, perr := time.Parse(datetime.DateLayout, l.Month)
		if perr != nil {
			continue
		}
		for _, v := range byRow[elementKey(l.ExternalID, model.ElementType(l.Type))] {
			if !plannable(v, f.startDay, m) {
				continue
			}
			if plans[v.ID] == nil {
				plans[v.ID] = map[string]string{}
			}
			plans[v.ID][l.Month] = vo.NewDecimal(l.Amount).String()
		}
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

// elementDebt is the debt's figures for month on its element. A debt not set
// up has no balance to count down, nothing scheduled and no payoff.
func elementDebt(v model.DebtView, timelines map[string]model.DebtTimeline, startDay int, month time.Time) *model.ElementDebtResult {
	out := &model.ElementDebtResult{Id: v.ID, IsSetUp: boolToInt(v.SetUp), IsPaidOff: boolToInt(v.PaidOff), ScheduledTotal: "0", Upcoming: "0"}
	if !v.SetUp {
		return out
	}
	tl := timelines[v.ID]
	tm := timelineMonth(tl, v, startDay, month)
	out.ClosingBalance, out.PaymentsLeft = decimalString(tm.ClosingBalance), tm.PaymentsLeft
	out.ScheduledTotal, out.Upcoming = decimalString(tm.ScheduledTotal), decimalString(tm.StillDue)
	out.PayoffDate, out.BasePayoffDate = tl.PayoffDate, tl.BasePayoffDate
	out.OverpaymentLeft, out.BaseOverpaymentLeft = decimalString(tl.OverpaymentLeft), decimalString(tl.BaseOverpaymentLeft)
	return out
}

// carriesDebt reports whether a row of this type can carry a debt: a
// top-level expense category or an expense envelope.
func carriesDebt(t model.ElementType) bool {
	return t == model.ElementCategory || t == model.ElementEnvelope
}
