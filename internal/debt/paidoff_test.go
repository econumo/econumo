package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (e *env) markPaidOff(t *testing.T, id, date string) *model.DebtResult {
	t.Helper()
	res, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: id, Date: date})
	if err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	return res
}

func (e *env) reopen(id string) (*model.DebtResult, error) {
	return e.svc.ReopenDebt(context.Background(), e.uid(), model.ReopenDebtRequest{Id: id})
}

// A loan of 10000 from 2026-01-01, looked at on 2026-03-10.
func (e *env) loanFromJanuary(t *testing.T) *model.DebtResult {
	t.Helper()
	return e.create(t, func(r *createSpec) { r.StartDate = ptr("2026-01-01") })
}

// Marking a debt paid off as of a date stores a statement of 0 that day and
// ends the debt there: a payment after the end stops matching on the next
// sync, one before it keeps counting.
func TestMarkDebtPaidOff_EndsTheDebt(t *testing.T) {
	e := newEnv(t)
	res := e.loanFromJanuary(t)
	before := e.expense(t, e.account, ymd(2026, 1, 20), "1000", "")
	after := e.expense(t, e.account, ymd(2026, 2, 20), "1000", "")
	e.sync(t)
	if got := e.splits(t, res.Id); len(got) != 2 {
		t.Fatalf("setup: splits = %v, want both payments", got)
	}

	got := e.markPaidOff(t, res.Id, "2026-02-01")
	if got.Status != "paid_off" || got.EndedAt != "2026-02-01" || got.IsPaidOff != 1 || got.CanEdit != 0 {
		t.Fatalf("result = %+v, want paid off on 2026-02-01, read-only", got)
	}
	snaps := e.snapshots(t, res.Id)
	if last := snaps[len(snaps)-1]; !last.AsOf.Equal(ymd(2026, 2, 1)) || !last.Balance.IsZero() {
		t.Fatalf("latest snapshot = %+v, want 0 as of 2026-02-01", last)
	}
	d := e.debtOf(t, res.Id)
	if d.Status != model.DebtPaidOff || d.EndedAt == nil || !d.EndedAt.Equal(ymd(2026, 2, 1)) {
		t.Fatalf("stored = %+v", d)
	}
	e.sync(t)
	splits := e.splits(t, res.Id)
	if _, ok := splits[before]; !ok {
		t.Errorf("the payment before the end lost its split: %v", splits)
	}
	if _, ok := splits[after]; ok {
		t.Errorf("the payment after the end still counts: %v", splits)
	}
}

func TestMarkDebtPaidOff_DateBounds(t *testing.T) {
	e := newEnv(t)
	res := e.loanFromJanuary(t)
	for _, date := range []string{"2026-01-01", "2025-12-31", "2026-03-11"} {
		_, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id, Date: date})
		requireField(t, err, "date", errs.CodeOutOfRange)
	}
	// no date is today
	if got := e.markPaidOff(t, res.Id, ""); got.EndedAt != "2026-03-10" {
		t.Fatalf("endedAt = %q, want today", got.EndedAt)
	}
}

// Reopening takes the debt back to active and removes the statement of 0 it
// stored on its end, so the payments after the end count again.
func TestReopenDebt(t *testing.T) {
	e := newEnv(t)
	res := e.loanFromJanuary(t)
	after := e.expense(t, e.account, ymd(2026, 2, 20), "1000", "")
	e.markPaidOff(t, res.Id, "2026-02-01")

	got, err := e.reopen(res.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "active" || got.EndedAt != "" || got.IsPaidOff != 0 || got.CanEdit != 1 {
		t.Fatalf("result = %+v, want active", got)
	}
	if snaps := e.snapshots(t, res.Id); len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want the origin alone", len(snaps))
	}
	if _, ok := e.splits(t, res.Id)[after]; !ok {
		t.Fatal("the payment after the old end does not count again")
	}
	// reopening an active debt changes nothing
	if again, err := e.reopen(res.Id); err != nil || again.Status != "active" {
		t.Fatalf("reopen active = %+v, %v", again, err)
	}
}

// A debt started on the element after the paid-off one blocks its reopening,
// even once that later debt is paid off too.
func TestReopenDebt_RefusedWhenALaterDebtExists(t *testing.T) {
	e := newEnv(t)
	old := e.loanFromJanuary(t)
	e.markPaidOff(t, old.Id, "2026-02-01")
	e.clock.now = ymd(2026, 3, 11)
	later := e.create(t, func(r *createSpec) { r.StartDate = ptr("2026-02-02") })
	e.markPaidOff(t, later.Id, "2026-03-01")

	_, err := e.reopen(old.Id)
	requireCode(t, err, errs.CodeDebtCannotReopen)
	if _, err := e.reopen(later.Id); err != nil {
		t.Fatalf("the latest debt reopens: %v", err)
	}
}

func TestReopenDebt_RefusedWhileAnotherIsActive(t *testing.T) {
	e := newEnv(t)
	old := e.loanFromJanuary(t)
	e.markPaidOff(t, old.Id, "2026-02-01")
	// a debt not set up, with no start date, made after the end
	e.create(t, withoutTerms)

	_, err := e.reopen(old.Id)
	requireCode(t, err, errs.CodeDebtCannotReopen)
}

// A paid-off debt is read-only: every write but delete and reopen is refused
// with debt.paid_off, its closing statement included.
func TestPaidOffDebtIsReadOnly(t *testing.T) {
	e := newEnv(t)
	res := e.loanFromJanuary(t)
	e.markPaidOff(t, res.Id, "2026-02-01")
	ctx, uid := context.Background(), e.uid()
	closing := e.snapshots(t, res.Id)[1].ID.String()

	_, err := e.svc.UpdateDebt(ctx, uid, model.UpdateDebtRequest{Id: res.Id, Payment: vo.NewFlexString("900")})
	requireCode(t, err, errs.CodeDebtPaidOff)
	_, err = e.svc.ReconcileDebt(ctx, uid, model.ReconcileDebtRequest{Id: res.Id, Balance: vo.NewFlexString("100"), BalanceDate: "2026-01-15"})
	requireCode(t, err, errs.CodeDebtPaidOff)
	_, err = e.svc.AddDebtRateChange(ctx, uid, model.AddDebtRateChangeRequest{Id: res.Id, Date: "2026-01-15",
		AnnualRate: vo.NewFlexString("5"), Payment: vo.NewFlexString("1000")})
	requireCode(t, err, errs.CodeDebtPaidOff)
	_, err = e.svc.DeleteDebtReconciliation(ctx, uid, model.DeleteDebtReconciliationRequest{Id: res.Id, ReconciliationId: closing})
	requireCode(t, err, errs.CodeDebtPaidOff)
	_, err = e.svc.MarkDebtPaidOff(ctx, uid, model.MarkDebtPaidOffRequest{Id: res.Id})
	requireCode(t, err, errs.CodeDebtPaidOff)

	if _, err := e.svc.DeleteDebt(ctx, uid, model.DeleteDebtRequest{Id: res.Id}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	e.gone(t, res.Id)
}

// Without a start date, a debt that follows a paid-off one on the element
// counts from the day after that end.
func TestSync_NewDebtWithoutStartFollowsThePreviousEnd(t *testing.T) {
	e := newEnv(t)
	old := e.loanFromJanuary(t)
	e.markPaidOff(t, old.Id, "2026-02-01")
	onEnd := e.expense(t, e.account, ymd(2026, 2, 1), "500", "")
	next := e.expense(t, e.account, ymd(2026, 2, 2), "500", "")
	fresh := e.create(t, withoutTerms)
	e.sync(t)

	if got := e.splits(t, fresh.Id); len(got) != 1 || got[next] == "" {
		t.Fatalf("new debt's splits = %v, want only the payment after the end", got)
	}
	if _, ok := e.splits(t, old.Id)[onEnd]; !ok {
		t.Fatal("the payment on the end day belongs to the paid-off debt")
	}
}

// A paid-off date before the latest statement is refused: that statement
// would stay the debt's current terms. On the statement's day the 0 replaces
// it.
func TestMarkDebtPaidOff_NotBeforeTheLatestStatement(t *testing.T) {
	e := newEnv(t)
	res := e.loanFromJanuary(t)
	if _, err := e.svc.ReconcileDebt(context.Background(), e.uid(), model.ReconcileDebtRequest{Id: res.Id,
		Balance: vo.NewFlexString("6000"), BalanceDate: "2026-03-10"}); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id, Date: "2026-03-01"})
	requireField(t, err, "date", errs.CodeOutOfRange)

	got := e.markPaidOff(t, res.Id, "2026-03-10")
	if got.Status != "paid_off" || got.IsPaidOff != 1 || got.CurrentBalance != "0" || got.EndedAt != "2026-03-10" {
		t.Fatalf("result = %+v, want paid off with nothing owed", got)
	}
	if snaps := e.snapshots(t, res.Id); len(snaps) != 2 || !snaps[1].Balance.IsZero() {
		t.Fatalf("snapshots = %+v, want the origin and the 0 replacing the statement", snaps)
	}
}

// A guest is refused before the date is read.
func TestMarkDebtPaidOff_GuestBeforeTheDate(t *testing.T) {
	e := newEnv(t)
	res := e.loanFromJanuary(t)
	guest := e.partner(t, "guest", roleGuest)
	_, err := e.svc.MarkDebtPaidOff(context.Background(), uidOf(guest), model.MarkDebtPaidOffRequest{Id: res.Id, Date: "soon"})
	requireForbidden(t, err)
}
