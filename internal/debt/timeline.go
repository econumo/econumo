package debt

import (
	"context"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Timelines projects each debt over the requested months, with the plans as
// extra payments. Unknown debts are left out of the result.
func (s *Service) Timelines(ctx context.Context, req model.DebtTimelineRequest) (map[string]model.DebtTimeline, error) {
	today := req.Today
	if today.IsZero() {
		today = s.today(ctx)
	}
	out := make(map[string]model.DebtTimeline, len(req.DebtIDs))
	synced := map[vo.Id][]*debtState{}
	for _, raw := range req.DebtIDs {
		id, err := vo.ParseId(raw)
		if err != nil {
			return nil, err
		}
		d, err := s.repo.GetByID(ctx, id)
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return nil, err
		}
		states, ok := synced[d.CategoryID]
		if !ok {
			if states, err = s.syncCategory(ctx, d.CategoryID); err != nil {
				return nil, err
			}
			synced[d.CategoryID] = states
		}
		for _, st := range states {
			if st.debt.ID.Equal(id) {
				out[raw] = timelineOf(st, today, req.Months, req.Plans[raw])
			}
		}
	}
	return out, nil
}

func timelineOf(st *debtState, today time.Time, months []time.Time, plans map[string]string) model.DebtTimeline {
	planned := make(map[string]float64, len(plans))
	for month, amount := range plans {
		if f, err := strconv.ParseFloat(amount, 64); err == nil {
			planned[month] = f
		}
	}
	proj := st.project(today, months, planned)
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
