package debt_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
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
	req.AmountBorrowed, req.StartDate = vo.NewFlexString("9000"), ptr("2026-04-10")
	req.Payment, req.LastPaymentDate = vo.NewFlexString("900"), "2027-04-01"
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSetUp != 1 || got.Balance != "9000" || got.AmountBorrowed != "9000" || got.Payment != "900" || got.LastPaymentDate != "2027-04-01" ||
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

// A payment or schedule sent alone corrects the loan's facts: the origin is
// rewritten, nothing is added.
func TestUpdateDebt_PaymentAloneCorrectsTheOrigin(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	req := e.updateReq(res)
	req.AmountBorrowed, req.LastPaymentDate = vo.FlexString{}, ""
	req.Payment, req.Schedule = vo.NewFlexString("900"), "monthly"
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Payment != "900" || got.AmountBorrowed != "10000" || got.LastPaymentDate != "2027-03-01" {
		t.Fatalf("result = %+v", got)
	}
	if snaps := e.snapshots(t, res.Id); len(snaps) != 1 || snaps[0].Payment.String() != "900" {
		t.Fatalf("snapshots = %+v", snaps)
	}
}

// A rename or filter change from a stale screen sends no terms: the terms
// another device wrote in the meantime stay, and no snapshot is written.
func TestUpdateDebt_WithoutTermsKeepsNewerTerms(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	newer := e.updateReq(res)
	newer.AmountBorrowed, newer.Payment, newer.LastPaymentDate = vo.NewFlexString("8000"), vo.NewFlexString("900"), "2027-02-01"
	if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), newer); err != nil {
		t.Fatal(err)
	}
	payee := e.newPayee()
	for i, mod := range []func(*model.UpdateDebtRequest){
		func(r *model.UpdateDebtRequest) { r.Payment, r.Schedule = vo.FlexString{}, "" },
		// a payment and schedule sent as stored change nothing either
		func(r *model.UpdateDebtRequest) { r.Payment, r.Schedule = vo.NewFlexString("900.00"), "monthly" },
	} {
		stale := e.updateReq(res)
		stale.Name, stale.PayeeId = "Renamed", &payee
		stale.AmountBorrowed, stale.LastPaymentDate = vo.FlexString{}, ""
		mod(&stale)
		got, err := e.svc.UpdateDebt(context.Background(), e.uid(), stale)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if got.Name != "Renamed" || got.PayeeId == nil || *got.PayeeId != payee || got.Balance != "8000" ||
			got.Payment != "900" || got.Schedule != "monthly" || got.LastPaymentDate != "2027-02-01" {
			t.Fatalf("case %d: result = %+v, want the newer terms kept", i, got)
		}
		if n := len(e.snapshots(t, res.Id)); n != 1 {
			t.Fatalf("case %d: snapshots = %d, want the origin alone", i, n)
		}
	}
}

func TestUpdateDebt_NotSetUpKeepsPaymentLeftOut(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)
	req := e.updateReq(res)
	req.Name, req.Payment, req.Schedule = "Renamed", vo.FlexString{}, ""
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Renamed" || got.Payment != res.Payment || got.Schedule != res.Schedule || got.IsSetUp != 0 {
		t.Fatalf("result = %+v, want payment %s and schedule %s kept", got, res.Payment, res.Schedule)
	}
}

// Payments collected before setup carry a provisional zero interest; setup
// splits them forward from the origin.
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
	req.AmountBorrowed, req.StartDate, req.LastPaymentDate = vo.NewFlexString("9500"), ptr("2026-03-10"), "2027-02-01"
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

// A debt converted in September and set up in November with a loan that
// began in July plans from July: the whole category's plans of July and
// August come to it, as at conversion.
func TestFinishSetup_PlanningStartsAtTheLoanStart(t *testing.T) {
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
	e.budget.transfers = nil
	req := e.updateReq(res)
	req.AmountBorrowed, req.StartDate, req.LastPaymentDate = vo.NewFlexString("8000"), ptr("2026-07-01"), "2027-08-01"
	if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatal(err)
	}
	v = e.viewOf(t, res.Id)
	if !v.SetUp || !v.FirstDay.Equal(ymd(2026, 7, 1)) || !v.StartMonth.Equal(ymd(2026, 7, 1)) ||
		!v.LastDay.Equal(ymd(2027, 8, 1)) || !v.EndMonth.Equal(ymd(2027, 8, 1)) {
		t.Fatalf("view after setup = %+v", v)
	}
	head := transferCall{from: e.category, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt,
		currency: res.CurrencyId, sum: true, first: "2026-07-01", before: "2026-09-15"}
	if !slices.Contains(e.budget.transfers, head) {
		t.Fatalf("transfers = %+v, want %+v", e.budget.transfers, head)
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
	_, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id})
	requireCode(t, err, errs.CodeDebtNotSetUp)
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

// The creation day is the OWNER's, the same for every viewer: a debt created
// on Oct 31 at 20:00 by an owner in New York (Nov 1 in UTC) plans from
// October, also for a viewer in Sydney; one created on Nov 1 at 08:00 by an
// owner in Sydney (Oct 31 in UTC) only from November, also for a viewer in New
// York. An owner with no stored timezone has the UTC day.
func TestPlanningStart_CreationDayInOwnersTimezone(t *testing.T) {
	for _, c := range []struct {
		name, ownerZone, viewerZone string
		at                          time.Time
		wantFirst                   time.Time
	}{
		{"new york owner", "America/New_York", "Australia/Sydney", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), ymd(2026, 10, 31)},
		{"sydney owner", "Australia/Sydney", "America/New_York", time.Date(2026, 10, 31, 21, 0, 0, 0, time.UTC), ymd(2026, 11, 1)},
		{"no stored timezone", "", "America/New_York", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), ymd(2026, 11, 1)},
	} {
		t.Run(c.name, func(t *testing.T) {
			viewer, err := time.LoadLocation(c.viewerZone)
			if err != nil {
				t.Fatal(err)
			}
			e := newEnv(t)
			e.owners.zones[e.user] = c.ownerZone
			e.clock.now = c.at
			req := e.createReq()
			withoutTerms(&req)
			res, err := e.svc.CreateDebt(reqctx.WithLocation(context.Background(), viewer), e.uid(), req)
			if err != nil {
				t.Fatal(err)
			}
			want := transferCall{from: e.category, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt,
				currency: res.CurrencyId, sum: true, first: c.wantFirst.Format("2006-01-02"), last: ""}
			if len(e.budget.transfers) != 1 || e.budget.transfers[0] != want {
				t.Fatalf("transfers = %+v, want %+v", e.budget.transfers, want)
			}
			stored, err := e.repo.GetByID(context.Background(), vo.MustParseId(res.Id))
			if err != nil {
				t.Fatal(err)
			}
			if !stored.CreatedDay.Equal(c.wantFirst) {
				t.Fatalf("stored creation day = %v, want %v", stored.CreatedDay, c.wantFirst)
			}
			for _, zone := range []string{c.viewerZone, "UTC", "Pacific/Kiritimati"} {
				loc, err := time.LoadLocation(zone)
				if err != nil {
					t.Fatal(err)
				}
				views, err := e.svc.Views(reqctx.WithLocation(context.Background(), loc), []vo.Id{e.uid()})
				if err != nil || len(views) != 1 {
					t.Fatalf("views = %+v, %v", views, err)
				}
				if v := views[0]; !v.FirstDay.Equal(c.wantFirst) || !v.StartMonth.Equal(monthStart(c.wantFirst)) {
					t.Fatalf("view for a viewer in %s = %+v, want first day %v", zone, v, c.wantFirst)
				}
			}
		})
	}
}

// The creation day is stored at creation: an owner who moves to another
// timezone afterwards does not move the debt's first plannable month.
func TestPlanningStart_OwnerTimezoneChangeKeepsTheCreationDay(t *testing.T) {
	e := newEnv(t)
	// 23:30 UTC on Oct 31 is Nov 1 in Berlin
	e.clock.now = time.Date(2026, 10, 31, 23, 30, 0, 0, time.UTC)
	res := e.createWithoutTerms(t, nil)
	e.owners.zones[e.user] = "Europe/Berlin"
	views, err := e.svc.Views(context.Background(), []vo.Id{e.uid()})
	if err != nil || len(views) != 1 {
		t.Fatalf("views = %+v, %v", views, err)
	}
	if v := views[0]; v.ID != res.Id || !v.FirstDay.Equal(ymd(2026, 10, 31)) || !v.StartMonth.Equal(ymd(2026, 10, 1)) {
		t.Fatalf("view = %+v, want first day 2026-10-31", v)
	}
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// A later start date on a whole-category debt hands the periods before it
// back to the category: the debt took them from its creation day on, and its
// row hides every month before its start. An earlier start takes the
// category's plans of the months it adds; a debt with a payee moves nothing.
func TestUpdateDebt_StartMovesTheHeadBetweenCategoryAndDebt(t *testing.T) {
	const back, toDebt = "back", "to debt"
	for _, c := range []struct {
		name  string
		setUp bool
		start string
		payee bool
		want  string
	}{
		{"not set up, later start", false, "2026-06-01", false, back},
		{"set up, later start", true, "2026-06-01", false, back},
		{"earlier start", false, "2026-01-01", false, toDebt},
		{"set up, earlier start", true, "2026-01-01", false, toDebt},
		{"with a payee", false, "2026-06-01", true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			var payee *string
			if c.payee {
				p := e.newPayee()
				payee = &p
			}
			mod := func(r *model.CreateDebtRequest) {
				r.PayeeId, r.LastPaymentDate = payee, "2027-07-01"
				if !c.setUp {
					withoutTerms(r)
				}
			}
			res := e.create(t, mod)
			e.clock.now = ymd(2026, 7, 1)
			e.budget.transfers = nil
			req := e.updateReq(res)
			start := c.start
			req.StartDate = &start
			if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), req); err != nil {
				t.Fatal(err)
			}
			var want []transferCall
			switch c.want {
			case back:
				want = append(want, transferCall{from: res.Id, fromType: model.ElementDebt, to: e.category, toType: model.ElementCategory,
					currency: res.CurrencyId, sum: true, before: c.start})
			case toDebt:
				want = append(want, transferCall{from: e.category, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt,
					currency: res.CurrencyId, sum: true, first: c.start, before: "2026-03-10"})
			}
			if !slices.Equal(e.budget.transfers, want) {
				t.Fatalf("transfers = %+v, want %+v", e.budget.transfers, want)
			}
		})
	}
}

// Finishing setup of a whole-category debt in a request that also changes its
// key still hands the months after its end back to the category it took them
// from; a new whole category then moves its own plans to the debt.
func TestFinishSetup_KeyChangeReturnsTheTailToTheFormerCategory(t *testing.T) {
	for _, c := range []struct {
		name string
		mod  func(e *env, r *model.UpdateDebtRequest) (newWholeCategory string)
	}{
		{"payee added", func(e *env, r *model.UpdateDebtRequest) string {
			payee := e.newPayee()
			r.PayeeId = &payee
			return ""
		}},
		{"category changed", func(e *env, r *model.UpdateDebtRequest) string {
			other := e.newCategory(e.user, true)
			r.CategoryId = other
			return other
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			res := e.createWithoutTerms(t, nil)
			if len(e.budget.transfers) != 1 {
				t.Fatalf("transfers at creation = %+v", e.budget.transfers)
			}
			first := e.budget.transfers[0].first
			e.budget.transfers = nil
			req := e.updateReq(res)
			req.AmountBorrowed, req.StartDate, req.LastPaymentDate = vo.NewFlexString("5800"), ptr("2026-03-10"), "2026-09-01"
			newWhole := c.mod(e, &req)
			if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), req); err != nil {
				t.Fatal(err)
			}
			want := []transferCall{{from: res.Id, fromType: model.ElementDebt, to: e.category, toType: model.ElementCategory,
				currency: res.CurrencyId, sum: true, after: "2026-09-01"}}
			if newWhole != "" {
				want = append(want, transferCall{from: newWhole, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt,
					currency: res.CurrencyId, sum: true, first: first, last: "2026-09-01"})
			}
			if !slices.Equal(e.budget.transfers, want) {
				t.Fatalf("transfers = %+v, want %+v", e.budget.transfers, want)
			}
		})
	}
}

// Finishing setup of a whole-category debt hands the months after its last
// payment back to the category; a debt with a payee never took them.
func TestFinishSetup_WholeDebtReturnsTheMonthsAfterItsEnd(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)
	payee := e.newPayee()
	other := e.newCategory(e.user, true)
	keyed := e.createWithoutTerms(t, func(r *model.CreateDebtRequest) { r.CategoryId, r.PayeeId = other, &payee })
	e.budget.transfers, e.budget.calls = nil, nil

	var setUp *model.DebtResult
	for _, r := range []*model.DebtResult{res, keyed} {
		req := e.updateReq(r)
		req.AmountBorrowed, req.StartDate, req.LastPaymentDate = vo.NewFlexString("5800"), ptr("2026-03-10"), "2026-09-01"
		got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
		if err != nil {
			t.Fatal(err)
		}
		if setUp == nil {
			setUp = got
		}
	}
	want := transferCall{from: res.Id, fromType: model.ElementDebt, to: e.category, toType: model.ElementCategory,
		currency: res.CurrencyId, sum: true, after: "2026-09-01"}
	if len(e.budget.transfers) != 1 || e.budget.transfers[0] != want {
		t.Fatalf("transfers = %+v, want only %+v", e.budget.transfers, want)
	}
	if len(e.budget.calls) != 2 || e.budget.calls[0] != "lock" || e.budget.calls[1] != "transfer" {
		t.Fatalf("budget calls = %v, want the rows locked before the transfer", e.budget.calls)
	}

	// already set up: a new end moves nothing
	e.budget.transfers = nil
	req := e.updateReq(setUp)
	req.LastPaymentDate = "2026-10-01"
	if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatal(err)
	}
	if len(e.budget.transfers) != 0 {
		t.Fatalf("transfers = %+v, want none for a debt already set up", e.budget.transfers)
	}
}
