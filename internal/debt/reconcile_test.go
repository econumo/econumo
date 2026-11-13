package debt_test

import (
	"context"
	"math"
	"strconv"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (e *env) reconcile(id, balance, date string) (*model.DebtResult, error) {
	return e.svc.ReconcileDebt(context.Background(), e.uid(), model.ReconcileDebtRequest{Id: id, Balance: vo.NewFlexString(balance), BalanceDate: date})
}

func (e *env) details(t *testing.T, id string) *model.DebtDetailsResult {
	t.Helper()
	got, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// The mortgage paid for five years, none of it entered: one statement today
// sets what is owed, and the dues up to it count as paid.
func TestReconcile_StatementSettlesTheDuesBeforeIt(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, func(r *model.CreateDebtRequest) {
		r.AmountBorrowed, r.StartDate, r.Payment, r.LastPaymentDate = vo.NewFlexString("300000"), ptr("2021-03-01"), vo.NewFlexString("2000"), "2051-03-01"
	})
	got, err := e.reconcile(res.Id, "270000", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentBalance != "270000" || got.AmountBorrowed != "300000" || got.PeriodsPaid != 60 || got.PeriodsPaid+got.PaymentsLeft < 300 {
		t.Fatalf("result = %+v", got)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 2 || !snaps[1].AsOf.Equal(ymd(2026, 3, 10)) || snaps[1].Balance.String() != "270000" ||
		snaps[1].Payment.String() != "2000" || !snaps[1].LastPaymentDate.Equal(ymd(2051, 3, 1)) {
		t.Fatalf("snapshots = %+v", snaps)
	}
}

// A statement dated in the past: the payments after it are split again from
// it, the ones before keep their splits; the history shows what Econumo had
// for that date.
func TestReconcile_PastStatementResplitsTheLaterPayments(t *testing.T) {
	e := newEnv(t)
	p1 := e.expense(t, e.account, ymd(2025, 4, 1), "600", "")
	p2 := e.expense(t, e.account, ymd(2025, 6, 1), "600", "")
	res := e.createOrigin(t, nil)
	before := e.splits(t, res.Id)

	if _, err := e.reconcile(res.Id, "9000", "2025-05-15"); err != nil {
		t.Fatal(err)
	}
	after := e.splits(t, res.Id)
	if after[p1] != before[p1] {
		t.Fatalf("split before the statement %s -> %s, want kept", before[p1], after[p1])
	}
	// at the loan's rate, which a statement keeps
	if r := originRate(t); after[p2] != money(9000*r) {
		t.Fatalf("split after the statement = %s, want %s", after[p2], money(9000*r))
	}
	d := e.details(t, res.Id)
	if len(d.Snapshots) != 2 || d.Snapshots[0].IsOrigin != 1 || d.Snapshots[0].ComputedBalance != "" || d.Snapshots[1].IsOrigin != 0 ||
		d.Snapshots[1].Id == "" || d.Snapshots[1].Balance != "9000" {
		t.Fatalf("history = %+v", d.Snapshots)
	}
	rate := originRate(t)
	if want := money(12000 - (600 - round2(12000*rate))); d.Snapshots[1].ComputedBalance != want {
		t.Fatalf("computed then = %s, want %s", d.Snapshots[1].ComputedBalance, want)
	}
}

// A second statement on the same date replaces the first.
func TestReconcile_SameDateReplaces(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	if _, err := e.reconcile(res.Id, "7000", "2025-12-01"); err != nil {
		t.Fatal(err)
	}
	got, err := e.reconcile(res.Id, "6900", "2025-12-01")
	if err != nil {
		t.Fatal(err)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 2 || snaps[1].Balance.String() != "6900" || got.Balance != "6900" {
		t.Fatalf("snapshots = %+v, result %+v", snaps, got)
	}
}

func TestReconcile_DateBetweenTheStartAndToday(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	// the start day's balance is the amount borrowed: corrected in the facts
	for _, date := range []string{"2025-02-28", "2025-03-01", "2026-03-11"} {
		_, err := e.reconcile(res.Id, "100", date)
		requireField(t, err, "balanceDate", errs.CodeOutOfRange)
	}
	for _, date := range []string{"2025-03-02", "2026-03-10"} {
		if _, err := e.reconcile(res.Id, "7000", date); err != nil {
			t.Fatalf("%s: %v", date, err)
		}
	}
	draft := e.createWithoutTerms(t, func(r *model.CreateDebtRequest) { r.Rules[0].CategoryId = e.newCategory(e.user, true) })
	_, err := e.reconcile(draft.Id, "100", "")
	requireCode(t, err, errs.CodeDebtNotSetUp)
	other := e.f.User(fixture.User{Name: "other", Email: "other@example.test"})
	_, err = e.svc.ReconcileDebt(context.Background(), vo.MustParseId(other), model.ReconcileDebtRequest{Id: res.Id, Balance: vo.NewFlexString("1")})
	requireNotFound(t, err)
}

func TestDeleteReconciliation_RemovesItAndResplits(t *testing.T) {
	e := newEnv(t)
	p2 := e.expense(t, e.account, ymd(2025, 6, 1), "600", "")
	res := e.createOrigin(t, nil)
	original := e.splits(t, res.Id)[p2]
	if _, err := e.reconcile(res.Id, "9000", "2025-05-15"); err != nil {
		t.Fatal(err)
	}
	d := e.details(t, res.Id)
	ctx := context.Background()
	_, err := e.svc.DeleteDebtReconciliation(ctx, e.uid(), model.DeleteDebtReconciliationRequest{Id: res.Id, ReconciliationId: d.Snapshots[0].Id})
	requireNotFound(t, err) // the origin is no reconciliation

	got, err := e.svc.DeleteDebtReconciliation(ctx, e.uid(), model.DeleteDebtReconciliationRequest{Id: res.Id, ReconciliationId: d.Snapshots[1].Id})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(e.snapshots(t, res.Id)); n != 1 || got.Balance != "12000" {
		t.Fatalf("snapshots = %d, result %+v", n, got)
	}
	if after := e.splits(t, res.Id)[p2]; after != original {
		t.Fatalf("split %s, want the origin-based %s again", after, original)
	}
}

// A correction that would leave a statement before the start or after the
// last payment says so.
func TestUpdateDebt_CorrectionConflictingWithAStatement(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	if _, err := e.reconcile(res.Id, "7000", "2025-12-01"); err != nil {
		t.Fatal(err)
	}
	req := e.updateReq(res)
	req.StartDate, req.Payment = ptr("2026-01-01"), vo.NewFlexString("1000")
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "startDate", errs.CodeDebtConflictsWithReconciliation)

	req = e.updateReq(res)
	req.LastPaymentDate, req.Payment = "2025-11-01", vo.NewFlexString("2000")
	_, err = e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "lastPaymentDate", errs.CodeDebtConflictsWithReconciliation)
}

// On a set-up debt a start date left out keeps the stored one.
func TestUpdateDebt_StartDateLeftOutIsKept(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	req := e.updateReq(res)
	req.StartDate, req.Name = nil, "Renamed"
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.StartDate != "2025-03-01" || got.Name != "Renamed" {
		t.Fatalf("result = %+v", got)
	}
}

// Debts on one category told apart only by their start dates are no whole
// category: neither takes the category's plans, nor hands its own back.
func TestWholeCategory_NotWhenAnotherDebtSharesTheFilters(t *testing.T) {
	e := newEnv(t)
	first := e.createOrigin(t, nil)
	if len(e.elements.transfers) != 1 {
		t.Fatalf("transfers = %+v, want the first debt to take the category", e.elements.transfers)
	}
	e.elements.transfers = nil
	second := e.create(t, nil) // starts 2026-03-10
	if len(e.elements.transfers) != 0 {
		t.Fatalf("transfers = %+v, want none for a second debt on the same filters", e.elements.transfers)
	}
	req := e.updateReq(second)
	req.StartDate = ptr("2026-03-05")
	if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatal(err)
	}
	if len(e.elements.transfers) != 0 {
		t.Fatalf("transfers = %+v, want none", e.elements.transfers)
	}
	if _, err := e.svc.DeleteDebt(context.Background(), e.uid(), model.DeleteDebtRequest{Id: first.Id}); err != nil {
		t.Fatal(err)
	}
	if len(e.elements.transfers) != 0 {
		t.Fatalf("transfers = %+v, want the first debt's plans not handed back", e.elements.transfers)
	}
}

// update-debt and create-debt take a dated statement too, by the same rules.
func TestUpdateDebt_DatedStatement(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, func(r *model.CreateDebtRequest) { r.Balance, r.BalanceDate = vo.NewFlexString("9000"), "2025-06-01" })
	req := model.UpdateDebtRequest{Id: res.Id, Name: res.Name, Balance: vo.NewFlexString("7000"), BalanceDate: "2025-12-01"}
	if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatal(err)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 3 || !snaps[1].AsOf.Equal(ymd(2025, 6, 1)) || !snaps[2].AsOf.Equal(ymd(2025, 12, 1)) || snaps[2].Balance.String() != "7000" {
		t.Fatalf("snapshots = %+v", snaps)
	}
	req.BalanceDate = "2025-01-01"
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "balanceDate", errs.CodeOutOfRange)
}

// Mark paid off is a statement of 0 as of today: it replaces one entered
// today rather than adding a second snapshot on that day.
func TestMarkPaidOff_ReplacesTodaysStatement(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	if _, err := e.reconcile(res.Id, "6000", ""); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 2 || !snaps[1].Balance.IsZero() || got.IsPaidOff != 1 {
		t.Fatalf("snapshots = %+v, result %+v", snaps, got)
	}
}

// carLoan is the reported loan shifted a year back to the env's today
// (2026-03-10): 16500 borrowed on 2025-05-15, 450 a month to 2029-03-10.
func (e *env) carLoan(t *testing.T) (*model.DebtResult, float64) {
	t.Helper()
	res := e.create(t, func(r *model.CreateDebtRequest) {
		r.AmountBorrowed, r.StartDate, r.Payment, r.LastPaymentDate = vo.NewFlexString("16500"), ptr("2025-05-15"), vo.NewFlexString("450"), "2029-03-10"
	})
	r, err := model.SnapshotRate(model.MathSnapshot{AsOf: ymd(2025, 5, 15), Balance: 16500, Payment: 450,
		Schedule: model.RecurringScheduleMonthly, Last: ymd(2029, 3, 10), Digits: 2, Origin: true})
	if err != nil {
		t.Fatal(err)
	}
	return res, r
}

// A statement far below Econumo's balance keeps the loan's rate and moves the
// payoff: fewer payments left, an earlier payoff, interest at the loan's rate.
func TestReconcile_LowerStatementMovesThePayoff(t *testing.T) {
	e := newEnv(t)
	res, loanRate := e.carLoan(t)
	got, err := e.reconcile(res.Id, "10000", "")
	if err != nil {
		t.Fatal(err)
	}
	rate, _ := strconv.ParseFloat(got.PeriodRate, 64)
	if math.Abs(rate-loanRate) > 1e-9 {
		t.Fatalf("periodRate = %s, want the loan's %v", got.PeriodRate, loanRate)
	}
	if got.PaymentsLeft >= 30 || got.PayoffDate >= "2029-03-10" || got.LastPaymentDate != "2029-03-10" {
		t.Fatalf("payments left %d, payoff %s, last payment %s; want well below 36 and before 2029-03-10, the contract kept",
			got.PaymentsLeft, got.PayoffDate, got.LastPaymentDate)
	}
	f := e.forecast(t, res.Id)
	if len(f.Items) != got.PaymentsLeft || f.Items[0].Interest != money(10000*loanRate) {
		t.Fatalf("forecast = %d items, first %+v; want %d, interest %s", len(f.Items), f.Items[0], got.PaymentsLeft, money(10000*loanRate))
	}
	t.Logf("payments left %d, payoff %s, next interest %s", got.PaymentsLeft, got.PayoffDate, f.Items[0].Interest)
}

// A statement above what the contract's dues could repay is accepted at the
// loan's rate; one the payment can never repay is refused.
func TestReconcile_StatementMustBeRepayableAtTheLoansRate(t *testing.T) {
	e := newEnv(t)
	res, loanRate := e.carLoan(t)
	got, err := e.reconcile(res.Id, "20000", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.PayoffDate <= "2029-03-10" {
		t.Fatalf("payoff %s, want after the contract's last payment", got.PayoffDate)
	}
	_, err = e.reconcile(res.Id, money(450/loanRate+1), "")
	requireField(t, err, "payment", errs.CodeDebtPaymentsDontCover)
}
