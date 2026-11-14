package debt_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// clone-budget copies each debt under new ids: its columns (the element,
// status and end included) and snapshots; the creation time and day stay, so
// the planning start is the same in the copy. No splits.
func TestCloneDebts_KeepsElements(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, func(r *createSpec) {
		r.StartDate, r.Balance, r.BalanceDate = ptr("2026-02-01"), vo.NewFlexString("9000"), "2026-03-10"
	})
	e.clock.now = ymd(2026, 7, 1)
	e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.sync(t)
	copyTo := e.newBudget(e.user, "Copy", e.account)

	var ids map[vo.Id]vo.Id
	err := e.db.TX.WithTx(ctx, func(ctx context.Context) error {
		var err error
		ids, err = e.svc.CloneDebts(ctx, vo.MustParseId(e.budget), vo.MustParseId(copyTo), nil)
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
	if cp.BudgetID.String() != copyTo || !cp.ElementID.Equal(src.ElementID) || cp.ElementType != src.ElementType || cp.Status != src.Status || !cp.CurrencyID.Equal(src.CurrencyID) || !sameDate(cp.StartDate, src.StartDate) ||
		!cp.AmountBorrowed.Equals(src.AmountBorrowed) || !cp.Payment.Equals(src.Payment) || cp.Schedule != src.Schedule ||
		!cp.CreatedAt.Equal(src.CreatedAt) || !cp.CreatedDay.Equal(src.CreatedDay) || cp.Current == nil || !cp.Current.Balance.Equals(src.Current.Balance) {
		t.Fatalf("copy = %+v\nsource = %+v", cp, src)
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

// A debt on an envelope the copy has no envelope for is left out, never
// attached to the source budget's envelope; one mapped follows its copy, and
// a category debt keeps its category.
func TestCloneDebts_SkipsAnUnmappedEnvelope(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	onCategory := e.create(t, nil)
	mappedEnv := e.newEnvelope("Housing", e.newCategory(e.user, true))
	onMapped := e.create(t, func(r *createSpec) { withoutTerms(r); r.ElementId = mappedEnv })
	lostEnv := e.newEnvelope("Car", e.newCategory(e.user, true))
	onLost := e.create(t, func(r *createSpec) { r.ElementId = lostEnv })
	copyTo := e.newBudget(e.user, "Copy", e.account)
	copiedEnv := vo.NewId()

	var ids map[vo.Id]vo.Id
	err := e.db.TX.WithTx(ctx, func(ctx context.Context) error {
		var err error
		ids, err = e.svc.CloneDebts(ctx, vo.MustParseId(e.budget), vo.MustParseId(copyTo),
			map[vo.Id]vo.Id{vo.MustParseId(mappedEnv): copiedEnv})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ids[vo.MustParseId(onLost.Id)]; ok || len(ids) != 2 {
		t.Fatalf("ids = %v, want the unmapped envelope's debt left out", ids)
	}
	if cp := e.debtOf(t, ids[vo.MustParseId(onCategory.Id)].String()); cp.ElementID.String() != onCategory.ElementId {
		t.Fatalf("category debt element = %s", cp.ElementID)
	}
	if cp := e.debtOf(t, ids[vo.MustParseId(onMapped.Id)].String()); !cp.ElementID.Equal(copiedEnv) {
		t.Fatalf("envelope debt element = %s, want %s", cp.ElementID, copiedEnv)
	}
	var n int
	if err := e.db.Raw.QueryRowContext(ctx, e.db.Rebind(`SELECT COUNT(*) FROM debts_snapshots s JOIN debts d ON d.id = s.debt_id WHERE d.budget_id = ?`), copyTo).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("copied snapshots = %d, want the category debt's only", n)
	}
}

func sameDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
