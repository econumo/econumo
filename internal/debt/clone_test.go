package debt_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// clone-budget copies each debt under new ids: its columns, rules (same keys
// and creation times) and snapshots; the creation time and day stay, so the
// tiebreak and the planning start are the same in the copy. No splits.
func TestCloneDebts_CopiesRulesSnapshotsAndTerms(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	payee := e.newPayee()
	b := e.newCategory(e.user, true)
	res := e.create(t, func(r *model.CreateDebtRequest) {
		r.Rules = []model.DebtRuleRequest{rule(e.category), {CategoryId: b, PayeeId: &payee}}
		r.StartDate, r.Balance, r.BalanceDate = ptr("2026-02-01"), vo.NewFlexString("9000"), "2026-03-10"
	})
	e.clock.now = ymd(2026, 7, 1)
	e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.sync(t)
	copyTo := e.newBudget(e.user, "Copy", e.account)

	var ids map[vo.Id]vo.Id
	err := e.db.TX.WithTx(ctx, func(ctx context.Context) error {
		var err error
		ids, err = e.svc.CloneDebts(ctx, vo.MustParseId(e.budget), vo.MustParseId(copyTo))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	src := e.debtOf(t, res.Id)
	newID, ok := ids[src.ID]
	if !ok || newID.Equal(src.ID) || len(ids) != 1 {
		t.Fatalf("ids = %v", ids)
	}
	cp := e.debtOf(t, newID.String())
	if cp.BudgetID.String() != copyTo || cp.Name != src.Name || !cp.CurrencyID.Equal(src.CurrencyID) || !sameDate(cp.StartDate, src.StartDate) ||
		!cp.AmountBorrowed.Equals(src.AmountBorrowed) || !cp.Payment.Equals(src.Payment) || cp.Schedule != src.Schedule ||
		!cp.CreatedAt.Equal(src.CreatedAt) || !cp.CreatedDay.Equal(src.CreatedDay) || cp.Current == nil || !cp.Current.Balance.Equals(src.Current.Balance) {
		t.Fatalf("copy = %+v\nsource = %+v", cp, src)
	}
	if len(cp.Rules) != 2 {
		t.Fatalf("rules = %+v", cp.Rules)
	}
	for i, r := range cp.Rules {
		if r.ID.Equal(src.Rules[i].ID) || !r.SameKey(src.Rules[i]) || !r.CreatedAt.Equal(src.Rules[i].CreatedAt) {
			t.Errorf("rule %d = %+v, source %+v", i, r, src.Rules[i])
		}
	}
	snaps, srcSnaps := e.snapshots(t, newID.String()), e.snapshots(t, res.Id)
	if len(snaps) != 2 || len(srcSnaps) != 2 {
		t.Fatalf("snapshots = %d, source %d", len(snaps), len(srcSnaps))
	}
	for i := range snaps {
		s, o := snaps[i], srcSnaps[i]
		if s.ID.Equal(o.ID) || !s.AsOf.Equal(o.AsOf) || !s.Balance.Equals(o.Balance) || !s.LastPaymentDate.Equal(o.LastPaymentDate) {
			t.Errorf("snapshot %d = %+v, source %+v", i, s, o)
		}
	}
	if got := e.splits(t, newID.String()); len(got) != 0 {
		t.Fatalf("splits copied: %v", got)
	}
	if len(e.splits(t, res.Id)) != 1 {
		t.Fatal("the source lost its split")
	}
}

func sameDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func TestCoveredCategories_PerBudget(t *testing.T) {
	e := newEnv(t)
	b := e.newCategory(e.user, true)
	payee := e.newPayee()
	e.create(t, func(r *model.CreateDebtRequest) {
		r.Rules = []model.DebtRuleRequest{rule(e.category), {CategoryId: b, PayeeId: &payee}}
	})
	other := e.newBudget(e.user, "Other", e.account)
	elsewhere := e.newCategory(e.user, true)
	e.create(t, func(r *model.CreateDebtRequest) {
		r.BudgetId, r.Rules = other, []model.DebtRuleRequest{rule(elsewhere)}
	})

	got, err := e.svc.CoveredCategories(context.Background(), vo.MustParseId(e.budget))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].String() != e.category || got[1].String() != b {
		t.Fatalf("covered = %v, want this budget's two categories", got)
	}
}

func TestTakeCategoryPlans_PerBudget(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.createWithoutTerms(t, nil)
	other := e.newBudget(e.user, "Other", e.account)
	e.elements.transfers = nil

	moved, err := e.svc.TakeCategoryPlans(ctx, vo.MustParseId(other), vo.MustParseId(e.category))
	if err != nil || moved || len(e.elements.transfers) != 0 {
		t.Fatalf("another budget: moved %v, %v, %+v", moved, err, e.elements.transfers)
	}
	moved, err = e.svc.TakeCategoryPlans(ctx, vo.MustParseId(e.budget), vo.MustParseId(e.category))
	if err != nil || !moved {
		t.Fatalf("moved = %v, %v", moved, err)
	}
	if tr := e.elements.transfers; len(tr) != 1 || tr[0].budget != e.budget || tr[0].to != res.Id || tr[0].from != e.category {
		t.Fatalf("transfers = %+v", tr)
	}
}
