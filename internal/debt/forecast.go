package debt

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetDebtForecast lists the debt's payments still to come, to the payoff: the
// regular schedule plus the extras its element's plans add (a period's plan
// above its scheduled payments); plans that add no extra leave the schedule
// alone (the result then names no budget). It is the projection behind the debt's own figures, on the
// budget's periods, run from the later of today and the last recorded
// payment's day.
func (s *Service) GetDebtForecast(ctx context.Context, userID vo.Id, req model.GetDebtForecastRequest) (*model.DebtForecastResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	d, b, err := s.readableDebt(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	st, err := s.stateOf(ctx, b, d)
	if err != nil {
		return nil, err
	}
	res := &model.DebtForecastResult{Items: []model.DebtForecastItem{}}
	if !d.IsSetUp() {
		return res, nil
	}
	if s.elements == nil {
		return nil, errNoElements
	}
	elementPlans, err := s.elements.Plans(ctx, d.BudgetID, d.ElementID)
	if err != nil {
		return nil, err
	}
	today := s.today(ctx)
	startDay := b.PeriodStartDay
	if startDay == 0 {
		startDay = model.MinPeriodStartDay
	}
	plans := plannedInLife(st, st.project(today, nil, nil), elementPlans, startDay)
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
		res.BudgetId, res.BudgetName = d.BudgetID.String(), b.Name
	}
	return res, nil
}

// plannedInLife keeps the plans of the periods the debt can be planned in:
// outside its life they are the element's ordinary plans.
func plannedInLife(st *debtState, proj model.Projection, plans map[string]float64, startDay int) map[string]float64 {
	v := st.view
	v.FirstDay, v.LastDay = planningBounds(st, proj)
	first, last := v.PlanningBounds(startDay)
	out := make(map[string]float64, len(plans))
	for key, amount := range plans {
		m, err := time.Parse(datetime.DateLayout, key)
		if err != nil || m.Before(first) || (!last.IsZero() && m.After(last)) {
			continue
		}
		out[key] = amount
	}
	return out
}
