package debt

import (
	"context"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
)

// Timelines projects each debt of the scope's budget over the requested
// budget periods, with the plans as extra payments. Debts of other budgets
// and debts not set up are left out of the result. It writes nothing: the
// caller has already run Sync, and splits not stored yet are computed in
// memory.
func (s *Service) Timelines(ctx context.Context, req model.DebtTimelineRequest) (map[string]model.DebtTimeline, error) {
	today := req.Today
	if today.IsZero() {
		today = s.today(ctx)
	}
	out := make(map[string]model.DebtTimeline, len(req.DebtIDs))
	if len(req.DebtIDs) == 0 {
		return out, nil
	}
	states, err := s.readStates(ctx, req.Scope)
	if err != nil {
		return nil, err
	}
	for _, raw := range req.DebtIDs {
		for _, st := range states {
			if st.debt.ID.String() == raw && st.debt.IsSetUp() {
				out[raw] = timelineOf(st, today, req.Months, req.Plans[raw], req.PeriodStartDay)
			}
		}
	}
	return out, nil
}

func timelineOf(st *debtState, today time.Time, months []time.Time, plans map[string]string, startDay int) model.DebtTimeline {
	planned := make(map[string]float64, len(plans))
	for month, amount := range plans {
		if f, err := strconv.ParseFloat(amount, 64); err == nil {
			planned[month] = f
		}
	}
	proj := st.projectPeriods(today, months, planned, startDay)
	digits := st.digits
	tl := model.DebtTimeline{
		Months:              make(map[string]model.DebtTimelineMonth, len(proj.Months)),
		PayoffDate:          dateString(proj.PayoffDate),
		BasePayoffDate:      dateString(proj.BasePayoffDate),
		OverpaymentLeft:     amountString(proj.OverpaymentLeft, digits),
		BaseOverpaymentLeft: amountString(proj.BaseOverpaymentLeft, digits),
	}
	for key, m := range proj.Months {
		tl.Months[key] = model.DebtTimelineMonth{ClosingBalance: amountString(m.ClosingBalance, digits), PaymentsLeft: m.PaymentsLeft,
			ScheduledTotal: amountString(m.ScheduledTotal, digits), StillDue: amountString(m.StillDue, digits)}
	}
	return tl
}
