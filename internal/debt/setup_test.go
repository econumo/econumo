package debt_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (e *env) createWithoutTerms(t *testing.T, mod func(*model.CreateDebtRequest)) *model.DebtResult {
	t.Helper()
	return e.create(t, func(r *model.CreateDebtRequest) {
		withoutTerms(r)
		if mod != nil {
			mod(r)
		}
	})
}

func (e *env) viewOf(t *testing.T, id string) model.DebtView {
	t.Helper()
	views, err := e.svc.Views(context.Background(), []vo.Id{e.uid()})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("no view for %s", id)
	return model.DebtView{}
}

func TestUpdateDebt_FinishSetupWritesFirstSnapshot(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)
	e.clock.now = ymd(2026, 4, 10)

	req := e.updateReq(res)
	req.Balance, req.Payment, req.LastPaymentDate = vo.NewFlexString("9000"), vo.NewFlexString("900"), "2027-04-01"
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSetUp != 1 || got.Balance != "9000" || got.Payment != "900" || got.LastPaymentDate != "2027-04-01" ||
		got.CurrentBalance != "9000" || got.PeriodRate == "" || got.NextDueDate != "2026-05-01" {
		t.Fatalf("result = %+v", got)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	if sn := snaps[0]; !sn.AsOf.Equal(ymd(2026, 4, 10)) || sn.Balance.String() != "9000" || sn.Payment.String() != "900" ||
		sn.Schedule != model.RecurringScheduleMonthly || !sn.LastPaymentDate.Equal(ymd(2027, 4, 1)) {
		t.Fatalf("snapshot = %+v", sn)
	}
	if d := e.debtOf(t, res.Id); !d.IsSetUp() || d.Payment.String() != "900" {
		t.Fatalf("stored debt = %+v", d)
	}
}

func TestUpdateDebt_NotSetUpUpdatesPaymentOnly(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)

	req := e.updateReq(res)
	req.Name, req.Payment, req.Schedule, req.StartDate = "Student loan", vo.NewFlexString("250.555"), "weekly", ptr("2026-02-01")
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSetUp != 0 || got.Name != "Student loan" || got.Payment != "250.56" || got.Schedule != "weekly" ||
		got.StartDate != "2026-02-01" || got.Balance != "" || got.CurrentBalance != "" {
		t.Fatalf("result = %+v", got)
	}
	if n := len(e.snapshots(t, res.Id)); n != 0 {
		t.Fatalf("snapshots = %d, want none", n)
	}
}

func TestUpdateDebt_SetUpDebtStillNeedsItsTerms(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	req := e.updateReq(res)
	req.Balance, req.LastPaymentDate = vo.FlexString{}, ""
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "balance", "common.is_blank")
}

// Payments collected before setup carry a provisional zero interest; setup
// splits them by the backward estimate from the first snapshot.
func TestFinishSetup_ResplitsCollectedPayments(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)
	a := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	b := e.expense(t, e.account, ymd(2026, 5, 1), "1000", "")
	e.clock.now = ymd(2026, 6, 10)
	e.sync(t)
	before := e.splits(t, res.Id)
	if len(before) != 2 || before[a] != "0" || before[b] != "0" {
		t.Fatalf("splits before setup = %v, want provisional zeros", before)
	}
	details, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil || len(details.Payments) != 2 || details.Snapshots == nil || len(details.Snapshots) != 0 {
		t.Fatalf("details = %+v, %v", details, err)
	}

	req := e.updateReq(res)
	req.Balance, req.LastPaymentDate = vo.NewFlexString("7500"), "2027-02-01"
	if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatal(err)
	}
	after := e.splits(t, res.Id)
	for _, id := range []string{a, b} {
		if v := vo.NewDecimal(after[id]); !v.IsGreaterThan(vo.NewDecimal("0")) {
			t.Errorf("payment %s interest after setup = %q, want > 0", id, after[id])
		}
	}
	got, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got.Item.PeriodsPaid != 2 {
		t.Errorf("periodsPaid = %d, want 2 (April and May)", got.Item.PeriodsPaid)
	}
}

// A debt converted in September and set up in November keeps September as
// its first plannable month, so plans moved at conversion stay visible.
func TestFinishSetup_KeepsPlanningStart(t *testing.T) {
	e := newEnv(t)
	e.clock.now = time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	res := e.createWithoutTerms(t, nil)

	v := e.viewOf(t, res.Id)
	if v.SetUp || !v.FirstDay.Equal(ymd(2026, 9, 15)) || !v.LastDay.IsZero() || !v.StartMonth.Equal(ymd(2026, 9, 1)) || !v.EndMonth.IsZero() {
		t.Fatalf("view before setup = %+v", v)
	}
	if first, last := v.PlanningBounds(1); !first.Equal(ymd(2026, 9, 1)) || !last.IsZero() {
		t.Fatalf("planning bounds = %v .. %v, want September with no end", first, last)
	}

	e.clock.now = ymd(2026, 11, 5)
	req := e.updateReq(res)
	req.Balance, req.LastPaymentDate = vo.NewFlexString("8000"), "2027-08-01"
	if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatal(err)
	}
	v = e.viewOf(t, res.Id)
	if !v.SetUp || !v.FirstDay.Equal(ymd(2026, 9, 15)) || !v.StartMonth.Equal(ymd(2026, 9, 1)) ||
		!v.LastDay.Equal(ymd(2027, 8, 1)) || !v.EndMonth.Equal(ymd(2027, 8, 1)) {
		t.Fatalf("view after setup = %+v", v)
	}
}

func TestViews_NotSetUpStartsAtAnEarlierPayment(t *testing.T) {
	e := newEnv(t)
	label := e.newLabel()
	res := e.createWithoutTerms(t, func(r *model.CreateDebtRequest) { r.LabelId = &label })
	e.addLabel(t, e.expense(t, e.account, ymd(2026, 1, 20), "1000", ""), label)

	v := e.viewOf(t, res.Id)
	if !v.FirstDay.Equal(ymd(2026, 1, 20)) || v.LabelID == nil || *v.LabelID != label || !v.CreatedAt.Equal(march10) || v.PaidOff {
		t.Fatalf("view = %+v", v)
	}
}

func TestMarkPaidOff_RefusedUntilSetUp(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)
	if _, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id}); err == nil {
		t.Fatal("a debt without terms was marked paid off")
	}
	if n := len(e.snapshots(t, res.Id)); n != 0 {
		t.Fatalf("snapshots = %d, want none", n)
	}
}

func TestTimelines_NotSetUpDebtIsLeftOut(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)
	got, err := e.svc.Timelines(context.Background(), model.DebtTimelineRequest{DebtIDs: []string{res.Id}, Months: []time.Time{ymd(2026, 3, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[res.Id]; ok {
		t.Fatalf("timelines = %+v, want the debt left out", got)
	}
}

func TestSync_SharedLabelPaymentGoesToOlderDebt(t *testing.T) {
	e := newEnv(t)
	car, student := e.newLabel(), e.newLabel()
	older := e.create(t, func(r *model.CreateDebtRequest) { r.LabelId = &car })
	e.clock.now = e.clock.now.Add(time.Minute)
	newer := e.create(t, func(r *model.CreateDebtRequest) { r.LabelId = &student })
	e.clock.now = ymd(2026, 7, 1)

	both := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.addLabel(t, both, car)
	e.addLabel(t, both, student)
	onlyStudent := e.expense(t, e.account, ymd(2026, 4, 2), "1000", "")
	e.addLabel(t, onlyStudent, student)
	unlabelled := e.expense(t, e.account, ymd(2026, 4, 3), "1000", "")
	e.sync(t)

	o, n := e.splits(t, older.Id), e.splits(t, newer.Id)
	if _, ok := o[both]; !ok || len(o) != 1 {
		t.Errorf("older debt splits = %v, want only the payment carrying both labels", o)
	}
	if _, ok := n[onlyStudent]; !ok || len(n) != 1 {
		t.Errorf("newer debt splits = %v, want only the student payment", n)
	}
	if _, ok := o[unlabelled]; ok {
		t.Error("an unlabelled payment matched a labelled debt")
	}
}

func TestSync_LabelRemovedFromPaymentUnmatches(t *testing.T) {
	e := newEnv(t)
	label := e.newLabel()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.LabelId = &label })
	e.clock.now = ymd(2026, 7, 1)
	tx := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.addLabel(t, tx, label)
	e.sync(t)
	if got := e.splits(t, res.Id); len(got) != 1 {
		t.Fatalf("splits = %v", got)
	}

	if _, err := e.db.Raw.ExecContext(context.Background(), e.db.Rebind("DELETE FROM transactions_labels WHERE transaction_id = ?"), tx); err != nil {
		t.Fatal(err)
	}
	e.sync(t)
	if got := e.splits(t, res.Id); len(got) != 0 {
		t.Fatalf("the unlabelled payment kept its split: %v", got)
	}
}

func TestReassignClassification_LabelFollowsTarget(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	src, dst, other := e.newLabel(), e.newLabel(), e.newLabel()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.LabelId = &src })
	e.create(t, func(r *model.CreateDebtRequest) { r.LabelId = &other })

	used, err := e.svc.InUse(ctx, "label", vo.MustParseId(src))
	if err != nil || !used {
		t.Fatalf("InUse(label) = %v, %v", used, err)
	}
	if err := e.svc.ReassignClassification(ctx, "label", vo.MustParseId(src), vo.MustParseId(dst)); err != nil {
		t.Fatal(err)
	}
	if d := e.debtOf(t, res.Id); d.LabelID == nil || d.LabelID.String() != dst {
		t.Fatalf("label = %v, want %s", d.LabelID, dst)
	}
	// merging into the other debt's label would leave two debts with the same key
	err = e.svc.ReassignClassification(ctx, "label", vo.MustParseId(dst), vo.MustParseId(other))
	requireCode(t, err, "debt.key_conflict")
}
