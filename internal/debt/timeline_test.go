package debt_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestTimelines_PlansMoveProjection(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	months := []time.Time{ymd(2026, 4, 1), ymd(2026, 5, 1), ymd(2026, 6, 1)}

	plain, err := e.svc.Timelines(ctx, model.DebtTimelineRequest{DebtIDs: []string{res.Id}, Months: months, Today: ymd(2026, 3, 10)})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := e.svc.Timelines(ctx, model.DebtTimelineRequest{DebtIDs: []string{res.Id}, Months: months, Today: ymd(2026, 3, 10),
		Plans: map[string]map[string]string{res.Id: {"2026-05-01": "3000"}}})
	if err != nil {
		t.Fatal(err)
	}
	p, q := plain[res.Id], planned[res.Id]
	if len(p.Months) != 3 || len(q.Months) != 3 {
		t.Fatalf("months = %v / %v", p.Months, q.Months)
	}
	apr := q.Months["2026-04-01"]
	if apr.ScheduledTotal != "1000" || apr.StillDue != "1000" {
		t.Errorf("April = %+v", apr)
	}
	if p.PayoffDate != "2027-03-01" || p.BasePayoffDate != p.PayoffDate || p.OverpaymentLeft != p.BaseOverpaymentLeft {
		t.Errorf("plain = %+v", p)
	}
	if q.PayoffDate >= q.BasePayoffDate || q.BasePayoffDate != p.PayoffDate {
		t.Errorf("an extra payment should bring the payoff forward: planned %s, base %s", q.PayoffDate, q.BasePayoffDate)
	}
	if !vo.NewDecimal(q.OverpaymentLeft).IsLessThan(vo.NewDecimal(q.BaseOverpaymentLeft)) || q.BaseOverpaymentLeft != p.OverpaymentLeft {
		t.Errorf("overpayment %s should be below base %s", q.OverpaymentLeft, q.BaseOverpaymentLeft)
	}
	if !vo.NewDecimal(q.Months["2026-05-01"].ClosingBalance).IsLessThan(vo.NewDecimal(p.Months["2026-05-01"].ClosingBalance)) {
		t.Errorf("May closing with the plan %s should be below %s", q.Months["2026-05-01"].ClosingBalance, p.Months["2026-05-01"].ClosingBalance)
	}
}

func TestTimelines_UnknownDebtIsLeftOut(t *testing.T) {
	e := newEnv(t)
	got, err := e.svc.Timelines(context.Background(), model.DebtTimelineRequest{DebtIDs: []string{vo.NewId().String()}, Months: []time.Time{ymd(2026, 4, 1)}})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}
