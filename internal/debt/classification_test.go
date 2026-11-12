package debt_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/debt"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (e *env) debtOf(t *testing.T, id string) *model.Debt {
	t.Helper()
	d, err := e.repo.GetByID(context.Background(), vo.MustParseId(id))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// release runs ReleaseClassification in a transaction, as a delete does.
func (e *env) release(t *testing.T, kind, id string) {
	t.Helper()
	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		if err := e.svc.LockForClassification(ctx, kind, vo.MustParseId(id)); err != nil {
			return err
		}
		return e.svc.ReleaseClassification(ctx, kind, vo.MustParseId(id))
	})
	if err != nil {
		t.Fatalf("release %s: %v", kind, err)
	}
}

func (e *env) reassign(t *testing.T, kind, oldID, newID string) {
	t.Helper()
	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		if err := e.svc.LockForClassification(ctx, kind, vo.MustParseId(oldID), vo.MustParseId(newID)); err != nil {
			return err
		}
		return e.svc.ReassignClassification(ctx, kind, vo.MustParseId(oldID), vo.MustParseId(newID))
	})
	if err != nil {
		t.Fatalf("reassign %s: %v", kind, err)
	}
}

// Deleting a category removes every rule naming it; a debt whose only rule
// that was stays, with no rules, and matches nothing. The plans stay put.
func TestReleaseClassification_CategoryRemovesItsRules(t *testing.T) {
	e := newEnv(t)
	b := e.newCategory(e.user, true)
	two := e.create(t, func(r *model.CreateDebtRequest) { r.Rules = []model.DebtRuleRequest{rule(e.category), rule(b)} })
	sole := e.create(t, func(r *model.CreateDebtRequest) { r.Rules = []model.DebtRuleRequest{rule(e.newCategory(e.user, true))} })
	e.clock.now = ymd(2026, 7, 1)
	tx := e.f.Transaction(fixtureExpense(e, sole.Rules[0].CategoryId, ymd(2026, 4, 1), "100"))
	e.sync(t)
	if _, ok := e.splits(t, sole.Id)[tx]; !ok {
		t.Fatal("setup: the payment has no split")
	}
	e.elements.transfers = nil

	e.release(t, debt.KindCategory, e.category)
	if d := e.debtOf(t, two.Id); len(d.Rules) != 1 || d.Rules[0].CategoryID.String() != b {
		t.Errorf("rules = %+v, want b's alone", d.Rules)
	}
	// the transaction itself is re-categorised by the delete: here it keeps
	// its category, the sole rule goes
	e.release(t, debt.KindCategory, sole.Rules[0].CategoryId)
	if d := e.debtOf(t, sole.Id); len(d.Rules) != 0 {
		t.Fatalf("rules = %+v, want none", d.Rules)
	}
	if len(e.elements.transfers) != 0 {
		t.Fatalf("transfers = %+v, want the plans left alone", e.elements.transfers)
	}
	e.sync(t)
	if got := e.splits(t, sole.Id); len(got) != 0 {
		t.Fatalf("a debt without rules kept splits: %v", got)
	}
	got, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: sole.Id})
	if err != nil || got.Item.Icon != model.DefaultIcon || len(got.Item.Rules) != 0 {
		t.Fatalf("debt without rules = %+v, %v", got, err)
	}
}

// Deleting a payee a rule narrows by widens the rule: it now covers the
// whole category.
func TestReleaseClassification_PayeeWidensTheRule(t *testing.T) {
	e := newEnv(t)
	payee, tag, label := e.newPayee(), e.newTag(), e.newLabel()
	res := e.create(t, func(r *model.CreateDebtRequest) {
		r.Rules = []model.DebtRuleRequest{{CategoryId: e.category, PayeeId: &payee, TagId: &tag}}
	})
	other := e.create(t, func(r *model.CreateDebtRequest) {
		r.Rules = []model.DebtRuleRequest{{CategoryId: e.newCategory(e.user, true), LabelId: &label}}
	})
	e.release(t, debt.KindPayee, payee)
	if r := e.debtOf(t, res.Id).Rules; len(r) != 1 || r[0].PayeeID != nil || r[0].TagID == nil || r[0].TagID.String() != tag {
		t.Fatalf("rules = %+v, want the tag alone", r)
	}
	e.release(t, debt.KindTag, tag)
	if r := e.debtOf(t, res.Id).Rules; len(r) != 1 || !r[0].IsWhole() {
		t.Fatalf("rules = %+v, want the whole category", r)
	}
	e.release(t, debt.KindLabel, label)
	if r := e.debtOf(t, other.Id).Rules; len(r) != 1 || r[0].LabelID != nil {
		t.Fatalf("labelled rules = %+v", r)
	}
}

// A payee delete that makes two debts' rules overlap: the older debt keeps
// its rule, the newer one loses its own.
func TestReleaseClassification_OverlapOlderDebtKeeps(t *testing.T) {
	e := newEnv(t)
	p1, p2 := e.newPayee(), e.newPayee()
	b := e.newCategory(e.user, true)
	older := e.create(t, func(r *model.CreateDebtRequest) {
		r.Rules = []model.DebtRuleRequest{{CategoryId: e.category, PayeeId: &p1}}
	})
	e.clock.now = e.clock.now.Add(time.Minute)
	newer := e.create(t, func(r *model.CreateDebtRequest) {
		r.Rules = []model.DebtRuleRequest{{CategoryId: e.category, PayeeId: &p2}, rule(b)}
	})
	e.release(t, debt.KindPayee, p1)
	if r := e.debtOf(t, older.Id).Rules; len(r) != 1 || !r[0].IsWhole() {
		t.Fatalf("older rules = %+v", r)
	}
	if r := e.debtOf(t, newer.Id).Rules; len(r) != 1 || r[0].CategoryID.String() != b {
		t.Fatalf("newer rules = %+v, want b's alone", r)
	}
}

// A partner who left the budget (or is a guest) deletes their own category a
// shared debt names: the release needs no access to the debt.
func TestReleaseClassification_NeedsNoAccessToTheDebt(t *testing.T) {
	e := newEnv(t)
	sam := e.partner(t, "Sam", roleUser)
	hers := e.newCategory(sam, true)
	res := e.create(t, func(r *model.CreateDebtRequest) { r.Rules = []model.DebtRuleRequest{rule(e.category), rule(hers)} })
	e.fakeBudget(e.budget).roles[sam] = roleGuest
	e.release(t, debt.KindCategory, hers)
	if r := e.debtOf(t, res.Id).Rules; len(r) != 1 || r[0].CategoryID.String() != e.category {
		t.Fatalf("rules = %+v", r)
	}
	// the category row can now go: no rule references it
	if _, err := e.db.Raw.ExecContext(context.Background(), e.db.Rebind("DELETE FROM categories WHERE id = ?"), hers); err != nil {
		t.Fatalf("delete category: %v", err)
	}
}

func TestReleaseClassification_NothingToRelease(t *testing.T) {
	e := newEnv(t)
	e.release(t, debt.KindTag, vo.NewId().String())
}

func TestReassignClassification_CategoryFollowsTarget(t *testing.T) {
	e := newEnv(t)
	target := e.newCategory(e.user, true)
	res := e.create(t, nil)

	e.reassign(t, debt.KindCategory, e.category, target)
	if got := e.debtOf(t, res.Id).Rules[0].CategoryID.String(); got != target {
		t.Fatalf("category = %s, want %s", got, target)
	}
	// the merge must not drop the target's plans: a clash adds up
	want := transferCall{budget: e.budget, from: target, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt, currency: res.CurrencyId,
		sum: true, first: "2026-03-10", last: "2027-03-01"}
	if len(e.elements.transfers) != 2 || e.elements.transfers[1] != want {
		t.Fatalf("a whole-category debt must take the target category's budget data: %+v", e.elements.transfers)
	}
}

func TestReassignClassification_PayeeAndTagFollowTarget(t *testing.T) {
	e := newEnv(t)
	srcPayee, dstPayee, srcTag, dstTag := e.newPayee(), e.newPayee(), e.newTag(), e.newTag()
	withPayee := e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].PayeeId = &srcPayee })
	otherCat := e.newCategory(e.user, true)
	withTag := e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].CategoryId = otherCat; r.Rules[0].TagId = &srcTag })

	e.reassign(t, debt.KindPayee, srcPayee, dstPayee)
	e.reassign(t, debt.KindTag, srcTag, dstTag)
	if p := e.debtOf(t, withPayee.Id).Rules[0].PayeeID; p == nil || p.String() != dstPayee {
		t.Errorf("payee = %v, want %s", p, dstPayee)
	}
	if tg := e.debtOf(t, withTag.Id).Rules[0].TagID; tg == nil || tg.String() != dstTag {
		t.Errorf("tag = %v, want %s", tg, dstTag)
	}
}

func TestReassignClassification_PaymentsFollowTheMerge(t *testing.T) {
	e := newEnv(t)
	src, dst := e.newPayee(), e.newPayee()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].PayeeId = &src; r.StartDate = ptr("2026-03-01") })
	tx := e.expense(t, e.account, ymd(2026, 3, 5), "100", src)
	e.sync(t)
	if _, ok := e.splits(t, res.Id)[tx]; !ok {
		t.Fatal("setup: the payment has no split")
	}

	// What the payee merge does before it calls the debt service.
	if _, err := e.db.Raw.Exec(e.db.Rebind("UPDATE transactions SET payee_id = ? WHERE payee_id = ?"), dst, src); err != nil {
		t.Fatal(err)
	}
	e.reassign(t, debt.KindPayee, src, dst)
	if _, ok := e.splits(t, res.Id)[tx]; !ok {
		t.Fatalf("the split was lost by the merge: %v", e.splits(t, res.Id))
	}
}

// A merge never refuses because of a debt (the merging user may have no
// access to it): a rule that would conflict is resolved, the older debt
// keeping its own.
func TestReassignClassification_NeverRefuses(t *testing.T) {
	cases := []struct {
		name string
		kind string
		make func(t *testing.T, e *env) (src, dst string)
	}{
		{"payee", debt.KindPayee, func(t *testing.T, e *env) (string, string) {
			src, dst := e.newPayee(), e.newPayee()
			e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].PayeeId = &dst })
			e.clock.now = e.clock.now.Add(time.Minute)
			e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].PayeeId = &src })
			return src, dst
		}},
		{"tag", debt.KindTag, func(t *testing.T, e *env) (string, string) {
			src, dst := e.newTag(), e.newTag()
			e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].TagId = &dst })
			e.clock.now = e.clock.now.Add(time.Minute)
			e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].TagId = &src })
			return src, dst
		}},
		{"category", debt.KindCategory, func(t *testing.T, e *env) (string, string) {
			src := e.newCategory(e.user, true)
			e.create(t, nil)
			e.clock.now = e.clock.now.Add(time.Minute)
			payee := e.newPayee()
			e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].CategoryId = src; r.Rules[0].PayeeId = &payee })
			return src, e.category
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			src, dst := c.make(t, e)
			e.reassign(t, c.kind, src, dst)
			debts, err := e.repo.ListByBudget(context.Background(), vo.MustParseId(e.budget))
			if err != nil || len(debts) != 2 {
				t.Fatalf("debts = %v, %v", debts, err)
			}
			if len(debts[0].Rules) != 1 || len(debts[1].Rules) != 0 {
				t.Fatalf("rules = %+v / %+v, want the older debt's kept and the newer one's dropped", debts[0].Rules, debts[1].Rules)
			}
		})
	}
}

// A category with no debt merged into one a whole-category debt covers: its
// plans, merged onto the target's category row, follow the payments onto the
// debt row.
func TestReassignClassification_MergeIntoWholeRuleMovesThePlans(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	source := e.newCategory(e.user, true)
	e.elements.transfers = nil

	e.reassign(t, debt.KindCategory, source, e.category)
	want := transferCall{budget: e.budget, from: e.category, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt, currency: res.CurrencyId,
		sum: true, first: "2026-03-10", last: "2027-03-01"}
	if len(e.elements.transfers) != 1 || e.elements.transfers[0] != want {
		t.Fatalf("transfers = %+v, want [%+v]", e.elements.transfers, want)
	}

	// a payee-narrowed debt on the target takes nothing
	payee := e.newPayee()
	narrowCat := e.newCategory(e.user, true)
	e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].CategoryId = narrowCat; r.Rules[0].PayeeId = &payee })
	e.elements.transfers = nil
	e.reassign(t, debt.KindCategory, e.newCategory(e.user, true), narrowCat)
	if len(e.elements.transfers) != 0 {
		t.Fatalf("a narrowed debt took the category's plans: %+v", e.elements.transfers)
	}
}

func TestLockForClassification_LocksTheBudgetsOfBothSides(t *testing.T) {
	e := newEnv(t)
	e.create(t, nil)
	other := e.newCategory(e.user, true)
	for _, kind := range []string{debt.KindCategory, debt.KindPayee, debt.KindTag, debt.KindLabel} {
		if err := e.svc.LockForClassification(context.Background(), kind, vo.MustParseId(other), vo.MustParseId(e.category)); err != nil {
			t.Fatalf("LockForClassification(%s): %v", kind, err)
		}
	}
	if err := e.svc.LockForClassification(context.Background(), "currency", vo.NewId()); err == nil {
		t.Fatal("an unknown kind must be refused")
	}
}
