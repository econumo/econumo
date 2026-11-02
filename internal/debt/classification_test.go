package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/debt"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
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

func TestInUse(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	payee, tag, free := e.newPayee(), e.newTag(), e.newPayee()
	e.create(t, func(r *model.CreateDebtRequest) { r.PayeeId = &payee; r.TagId = &tag })

	for _, c := range []struct {
		kind string
		id   string
		want bool
	}{
		{debt.KindCategory, e.category, true},
		{debt.KindPayee, payee, true},
		{debt.KindTag, tag, true},
		{debt.KindPayee, free, false},
	} {
		got, err := e.svc.InUse(ctx, c.kind, vo.MustParseId(c.id))
		if err != nil || got != c.want {
			t.Errorf("InUse(%s) = %v, %v; want %v", c.kind, got, err, c.want)
		}
	}
}

func TestReassignClassification_CategoryFollowsTarget(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	target := e.newCategory(e.user, true)
	res := e.create(t, nil)

	if err := e.svc.ReassignClassification(ctx, debt.KindCategory, vo.MustParseId(e.category), vo.MustParseId(target)); err != nil {
		t.Fatal(err)
	}
	if got := e.debtOf(t, res.Id).CategoryID.String(); got != target {
		t.Fatalf("category = %s, want %s", got, target)
	}
	// the merge must not drop the target's plans: a clash adds up
	want := transferCall{from: target, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt, currency: res.CurrencyId,
		sum: true, first: "2026-03-01", last: "2027-03-01"}
	if len(e.budget.transfers) != 2 || e.budget.transfers[1] != want {
		t.Fatalf("a whole-category debt must take the target category's budget data: %+v", e.budget.transfers)
	}
}

func TestReassignClassification_PayeeAndTagFollowTarget(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	srcPayee, dstPayee, srcTag, dstTag := e.newPayee(), e.newPayee(), e.newTag(), e.newTag()
	withPayee := e.create(t, func(r *model.CreateDebtRequest) { r.PayeeId = &srcPayee })
	otherCat := e.newCategory(e.user, true)
	withTag := e.create(t, func(r *model.CreateDebtRequest) { r.CategoryId = otherCat; r.TagId = &srcTag })

	if err := e.svc.ReassignClassification(ctx, debt.KindPayee, vo.MustParseId(srcPayee), vo.MustParseId(dstPayee)); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ReassignClassification(ctx, debt.KindTag, vo.MustParseId(srcTag), vo.MustParseId(dstTag)); err != nil {
		t.Fatal(err)
	}
	if p := e.debtOf(t, withPayee.Id).PayeeID; p == nil || p.String() != dstPayee {
		t.Errorf("payee = %v, want %s", p, dstPayee)
	}
	if tg := e.debtOf(t, withTag.Id).TagID; tg == nil || tg.String() != dstTag {
		t.Errorf("tag = %v, want %s", tg, dstTag)
	}
}

func TestReassignClassification_PaymentsFollowTheMerge(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	src, dst := e.newPayee(), e.newPayee()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.PayeeId = &src })
	tx := e.expense(t, e.account, ymd(2026, 3, 5), "100", src)
	e.sync(t)
	if _, ok := e.splits(t, res.Id)[tx]; !ok {
		t.Fatal("setup: the payment has no split")
	}

	// What the payee merge does before it calls the debt service.
	if _, err := e.db.Raw.Exec(e.db.Rebind("UPDATE transactions SET payee_id = ? WHERE payee_id = ?"), dst, src); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ReassignClassification(ctx, debt.KindPayee, vo.MustParseId(src), vo.MustParseId(dst)); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.splits(t, res.Id)[tx]; !ok {
		t.Fatalf("the split was lost by the merge: %v", e.splits(t, res.Id))
	}
}

func TestReassignClassification_KeyConflictIsRefused(t *testing.T) {
	cases := []struct {
		name string
		kind string
		make func(t *testing.T, e *env) (src, dst string)
	}{
		{"payee", debt.KindPayee, func(t *testing.T, e *env) (string, string) {
			src, dst := e.newPayee(), e.newPayee()
			e.create(t, func(r *model.CreateDebtRequest) { r.PayeeId = &src })
			e.create(t, func(r *model.CreateDebtRequest) { r.PayeeId = &dst })
			return src, dst
		}},
		{"tag", debt.KindTag, func(t *testing.T, e *env) (string, string) {
			src, dst := e.newTag(), e.newTag()
			e.create(t, func(r *model.CreateDebtRequest) { r.TagId = &src })
			e.create(t, func(r *model.CreateDebtRequest) { r.TagId = &dst })
			return src, dst
		}},
		{"category", debt.KindCategory, func(t *testing.T, e *env) (string, string) {
			dst := e.newCategory(e.user, true)
			e.create(t, nil)
			e.create(t, func(r *model.CreateDebtRequest) { r.CategoryId = dst })
			return e.category, dst
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			src, dst := c.make(t, e)
			// The merge runs in one transaction, so the refusal rolls the move back.
			err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
				return e.svc.ReassignClassification(ctx, c.kind, vo.MustParseId(src), vo.MustParseId(dst))
			})
			requireCode(t, err, errs.CodeDebtKeyConflict)
			if used, _ := e.svc.InUse(context.Background(), c.kind, vo.MustParseId(src)); !used {
				t.Error("the refused merge left the debt moved")
			}
		})
	}
}

func TestReassignClassification_NothingToMove(t *testing.T) {
	e := newEnv(t)
	if err := e.svc.ReassignClassification(context.Background(), debt.KindTag, vo.NewId(), vo.NewId()); err != nil {
		t.Fatal(err)
	}
}

// A category with no debt merged into one a whole-category debt covers: its
// plans, merged onto the target's category row, follow the payments onto the
// debt row.
func TestReassignClassification_MergeIntoWholeCategoryDebtMovesThePlans(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	source := e.newCategory(e.user, true)
	e.budget.transfers = nil

	if err := e.svc.ReassignClassification(ctx, debt.KindCategory, vo.MustParseId(source), vo.MustParseId(e.category)); err != nil {
		t.Fatal(err)
	}
	want := transferCall{from: e.category, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt, currency: res.CurrencyId,
		sum: true, first: "2026-03-01", last: "2027-03-01"}
	if len(e.budget.transfers) != 1 || e.budget.transfers[0] != want {
		t.Fatalf("transfers = %+v, want [%+v]", e.budget.transfers, want)
	}

	// a payee-narrowed debt on the target takes nothing
	payee := e.newPayee()
	narrowCat := e.newCategory(e.user, true)
	e.create(t, func(r *model.CreateDebtRequest) { r.CategoryId = narrowCat; r.PayeeId = &payee })
	e.budget.transfers = nil
	if err := e.svc.ReassignClassification(ctx, debt.KindCategory, vo.MustParseId(e.newCategory(e.user, true)), vo.MustParseId(narrowCat)); err != nil {
		t.Fatal(err)
	}
	if len(e.budget.transfers) != 0 {
		t.Fatalf("a narrowed debt took the category's plans: %+v", e.budget.transfers)
	}
}
