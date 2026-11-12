package debt

import (
	"context"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetDebtForecast lists the debt's payments still to come, to the payoff: the
// regular schedule plus the extras one budget plans for the debt's row. That
// budget is the one asked for, else the owner's active budget; one the caller
// cannot read, or that holds no row for the debt, leaves the schedule alone,
// and so does a budget whose plans add no extra (the result then names none).
// It is the projection behind the debt's own figures, on that budget's periods,
// run from the later of today and the last recorded payment's day.
func (s *Service) GetDebtForecast(ctx context.Context, userID vo.Id, req model.GetDebtForecastRequest) (*model.DebtForecastResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	d, err := s.ownedDebt(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	st, err := s.stateOf(ctx, d)
	if err != nil {
		return nil, err
	}
	res := &model.DebtForecastResult{Items: []model.DebtForecastItem{}}
	if !d.IsSetUp() {
		return res, nil
	}
	budgetID, bp, err := s.forecastPlans(ctx, userID, req.BudgetId, d.ID)
	if err != nil {
		return nil, err
	}
	today := s.today(ctx)
	startDay := model.MinPeriodStartDay
	var plans map[string]float64
	if bp != nil {
		startDay = bp.PeriodStartDay
		plans = plannedInLife(st, st.project(today, nil, nil), bp)
	}
	// A payment recorded ahead of today is booked, not still to come: the run
	// starts on the last recorded payment's day, so its due is paid and the
	// first row follows the balance the booked list ends on.
	from := today
	if n := len(st.pays); n > 0 && debtDate(st.pays[n-1].date).After(from) {
		from = debtDate(st.pays[n-1].date)
	}
	digits := st.digits
	// The summary runs from today, as the debt's own figures do, so with no
	// plans it is exactly them.
	summary := st.project(today, nil, nil)
	if len(plans) > 0 {
		summary = st.projectPeriods(today, nil, plans, startDay)
	}
	res.PaymentsLeft, res.OverpaymentLeft = summary.PaymentsLeft, amountString(summary.OverpaymentLeft, digits)
	var extraPlanned float64
	for _, p := range summary.Payments {
		extraPlanned += p.Extra
	}
	res.ExtraPlanned = amountString(extraPlanned, digits)
	if !summary.PaidOff {
		res.PayoffDate = dateString(summary.PayoffDate)
	}
	proj := st.projectPeriods(from, nil, plans, startDay)
	extra := false
	for _, p := range proj.Payments {
		pay, interest := roundTo(p.Payment, digits), roundTo(p.Interest, digits)
		res.Items = append(res.Items, model.DebtForecastItem{Date: dateString(p.Date), Payment: amountString(pay, digits),
			Interest: amountString(interest, digits), Principal: amountString(pay-interest, digits), Balance: amountString(p.Balance, digits)})
		if roundTo(p.Extra, digits) > 0 {
			extra = true
		}
	}
	if extra {
		res.BudgetId, res.BudgetName = budgetID.String(), bp.Name
	}
	return res, nil
}

// forecastPlans resolves the budget whose plans the forecast follows; nil
// plans when there is none to follow.
func (s *Service) forecastPlans(ctx context.Context, userID vo.Id, raw string, debtID vo.Id) (vo.Id, *model.DebtBudgetPlans, error) {
	if s.plans == nil {
		return vo.Id{}, nil, nil
	}
	var budgetID vo.Id
	if raw != "" {
		id, err := vo.ParseId(raw)
		if err != nil {
			return vo.Id{}, nil, err
		}
		budgetID = id
	} else {
		active, err := s.plans.ActiveBudget(ctx, userID)
		if err != nil || active == nil {
			return vo.Id{}, nil, err
		}
		budgetID = *active
	}
	bp, ok, err := s.plans.DebtPlans(ctx, userID, budgetID, debtID)
	if err != nil || !ok {
		return vo.Id{}, nil, err
	}
	return budgetID, &bp, nil
}

// plannedInLife keeps the plans of the periods the debt can be planned in, the
// ones a budget counts.
func plannedInLife(st *debtState, proj model.Projection, bp *model.DebtBudgetPlans) map[string]float64 {
	v := st.view
	v.FirstDay, v.LastDay = planningBounds(st, proj)
	first, last := v.PlanningBounds(bp.PeriodStartDay)
	out := make(map[string]float64, len(bp.Plans))
	for key, amount := range bp.Plans {
		m, err := time.Parse(datetime.DateLayout, key)
		if err != nil || m.Before(first) || (!last.IsZero() && m.After(last)) {
			continue
		}
		if f, err := strconv.ParseFloat(amount, 64); err == nil {
			out[key] = f
		}
	}
	return out
}
