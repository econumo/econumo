package repo_test

// Integration tests for the per-label spending read model
// (CountSpendingByLabel, LabelsForUsers), which are deliberately independent
// of CountSpending -- see internal/budget/repo/read.go.

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func linkLabel(t *testing.T, db *dbtest.DB, transactionID, labelID string) {
	t.Helper()
	db.Exec(t, db.Rebind(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`), transactionID, labelID)
}

// One expense carrying two labels must produce one row per label, each with
// the FULL amount. The overlap is the feature: "how much went to kid A" and
// "to kid B" are both 50, even though only 50 was spent. A GROUP BY that
// instead split the amount across labels would pass a row-count check alone,
// so this asserts the exact amount per label too.
func TestCountSpendingByLabelCountsFullAmountPerLabel(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	l1 := f.Label(fixture.Label{UserID: userA, Name: "Kid A"})
	l2 := f.Label(fixture.Label{UserID: userA, Name: "Kid B"})
	txID := "78000000-0000-0000-0000-000000000001"
	seedExpense(t, db, txID, acctA, "", "50.00", "2026-08-15 00:00:00")
	linkLabel(t, db, txID, l1)
	linkLabel(t, db, txID, l2)

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.CountSpendingByLabel(ctx, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("CountSpendingByLabel: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want one per label: %+v", len(rows), rows)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.LabelID] {
			t.Fatalf("duplicate row for label %q: %+v", r.LabelID, rows)
		}
		seen[r.LabelID] = true
		if vo.NewDecimal(r.Amount).String() != vo.NewDecimal("50").String() {
			t.Errorf("label %s amount = %s, want the full 50", r.LabelID, r.Amount)
		}
		if r.CurrencyID != usdID {
			t.Errorf("label %s currency = %s, want %s", r.LabelID, r.CurrencyID, usdID)
		}
	}
	if !seen[l1] || !seen[l2] {
		t.Fatalf("want rows for both labels %q and %q, got %+v", l1, l2, rows)
	}
}

// A transaction outside the period, or one carrying no label at all, must not
// contribute a row -- deleting the WHERE/JOIN would let either leak in. The
// first-of-month instant is the classic sqliteDatetime trap (see
// internal/budget/repo/read_integration_test.go's CountSpending counterpart):
// a raw time.Time bound mis-compares against the stored 'Y-m-d H:i:s' TEXT and
// silently drops that row, so it must be seeded here, not just mid-period.
func TestCountSpendingByLabelExcludesOutOfPeriodAndUnlabeled(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	label := f.Label(fixture.Label{UserID: userA, Name: "Groceries"})

	boundary := "78000000-0000-0000-0000-000000000005"
	seedExpense(t, db, boundary, acctA, "", "5.00", "2026-08-01 00:00:00")
	linkLabel(t, db, boundary, label)

	inPeriod := "78000000-0000-0000-0000-000000000002"
	seedExpense(t, db, inPeriod, acctA, "", "10.00", "2026-08-05 00:00:00")
	linkLabel(t, db, inPeriod, label)

	outOfPeriod := "78000000-0000-0000-0000-000000000003"
	seedExpense(t, db, outOfPeriod, acctA, "", "99.00", "2026-07-31 23:59:59")
	linkLabel(t, db, outOfPeriod, label)

	// No label link at all: must not surface as a row for any label.
	seedExpense(t, db, "78000000-0000-0000-0000-000000000004", acctA, "", "77.00", "2026-08-06 00:00:00")

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.CountSpendingByLabel(ctx, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("CountSpendingByLabel: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row (the two in-period labeled expenses only), got %d: %+v", len(rows), rows)
	}
	if rows[0].LabelID != label {
		t.Fatalf("wrong label: %q", rows[0].LabelID)
	}
	// 5.00 (Aug 1 00:00:00 boundary, must be included) + 10.00 (Aug 5) = 15.00.
	if vo.NewDecimal(rows[0].Amount).String() != vo.NewDecimal("15").String() {
		t.Errorf("amount mismatch: %q, want 15 (boundary expense excluded would leave 10)", rows[0].Amount)
	}
}

// A label spanning two categories, plus a transaction with no category at
// all, must come back as three separate rows -- one per category (including
// the nil-category bucket) -- not summed into a single per-label total. This
// is the data the budget folder's second level (spend by category) needs.
func TestCountSpendingByLabelGroupsByCategory(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	label := f.Label(fixture.Label{UserID: userA, Name: "Shared"})

	catA := "c0000000-0000-0000-0000-0000000000a1"
	catB := "c0000000-0000-0000-0000-0000000000b1"
	seedCategory(t, db, catA, userA)
	seedCategory(t, db, catB, userA)

	tx1 := "78000000-0000-0000-0000-000000000011"
	seedExpense(t, db, tx1, acctA, catA, "20.00", "2026-08-10 00:00:00")
	linkLabel(t, db, tx1, label)

	tx2 := "78000000-0000-0000-0000-000000000012"
	seedExpense(t, db, tx2, acctA, catB, "30.00", "2026-08-11 00:00:00")
	linkLabel(t, db, tx2, label)

	tx3 := "78000000-0000-0000-0000-000000000013"
	seedExpense(t, db, tx3, acctA, "", "40.00", "2026-08-12 00:00:00")
	linkLabel(t, db, tx3, label)

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.CountSpendingByLabel(ctx, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("CountSpendingByLabel: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want one per category (incl. nil): %+v", len(rows), rows)
	}

	byCategory := map[string]string{}
	var nilCategoryCount int
	for _, r := range rows {
		if r.LabelID != label {
			t.Fatalf("unexpected label id %q: %+v", r.LabelID, rows)
		}
		if r.CategoryID == nil {
			nilCategoryCount++
			if vo.NewDecimal(r.Amount).String() != vo.NewDecimal("40").String() {
				t.Errorf("nil-category amount = %s, want 40", r.Amount)
			}
			continue
		}
		byCategory[*r.CategoryID] = r.Amount
	}
	if nilCategoryCount != 1 {
		t.Fatalf("want exactly 1 nil-category row, got %d: %+v", nilCategoryCount, rows)
	}
	if vo.NewDecimal(byCategory[catA]).String() != vo.NewDecimal("20").String() {
		t.Errorf("category A amount = %s, want 20", byCategory[catA])
	}
	if vo.NewDecimal(byCategory[catB]).String() != vo.NewDecimal("30").String() {
		t.Errorf("category B amount = %s, want 30", byCategory[catB])
	}
}

func TestCountSpendingByLabelReturnsNilForNoAccounts(t *testing.T) {
	read, _ := newReadRepo(t)
	rows, err := read.CountSpendingByLabel(context.Background(), nil, time.Now(), time.Now().AddDate(0, 1, 0))
	if err != nil || rows != nil {
		t.Fatalf("empty account list must short-circuit; got %v, %v", rows, err)
	}
}

// LabelsForUsers must return every label owned by the given users, keyed by
// id, with the sort key carried through (the labels block renders in list
// order) -- and must not leak a label owned by someone else.
func TestLabelsForUsers(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	seedUser(t, db, userB)

	first := f.Label(fixture.Label{UserID: userA, Name: "First", Icon: "star", Position: 1})
	second := f.Label(fixture.Label{UserID: userA, Name: "Second", Icon: "flag", Position: 0, Archived: true})
	other := f.Label(fixture.Label{UserID: userB, Name: "Other"})

	got, err := read.LabelsForUsers(ctx, []vo.Id{vo.MustParseId(userA)})
	if err != nil {
		t.Fatalf("LabelsForUsers: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 labels for userA, got %d: %+v", len(got), got)
	}
	if _, leaked := got[other]; leaked {
		t.Fatalf("label owned by another user leaked into the result: %+v", got)
	}

	m := got[first]
	if m.Name != "First" || m.Icon != "star" || m.SortKey != labelKeyAt(1) || m.IsArchived || m.OwnerID != userA {
		t.Errorf("first label mismatch: %+v", m)
	}
	m2 := got[second]
	if m2.Name != "Second" || m2.Icon != "flag" || m2.SortKey != labelKeyAt(0) || !m2.IsArchived {
		t.Errorf("second (archived) label mismatch: %+v", m2)
	}

	none, err := read.LabelsForUsers(ctx, nil)
	if err != nil || none != nil {
		t.Fatalf("empty user id set should be nil,nil; got %v, %v", none, err)
	}
}

// labelKeyAt mirrors the fixture builder's Position -> sort-key sugar, so the
// assertions above can still express intended order as a small integer.
func labelKeyAt(pos int) string {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	return "c" + string(alphabet[(pos/3844)%62]) + string(alphabet[(pos/62)%62]) + string(alphabet[pos%62])
}

// The two narrowed label drill-down queries back the folder's per-category
// rows. Each must apply BOTH predicates: a row in the right category but under
// a different label, and a row under the right label in a different category,
// are equally wrong answers -- and the placeholder arithmetic differs per
// engine, so this runs on whichever engine dbtest selects.
func TestBudgetTransactionsByLabelNarrowedByCategory(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	label := f.Label(fixture.Label{UserID: userA, Name: "Work"})
	other := f.Label(fixture.Label{UserID: userA, Name: "Home"})

	catA := "c0000000-0000-0000-0000-0000000000c7"
	catB := "c0000000-0000-0000-0000-0000000000c8"
	seedCategory(t, db, catA, userA)
	seedCategory(t, db, catB, userA)

	want := "78000000-0000-0000-0000-000000000021"
	seedExpense(t, db, want, acctA, catA, "20.00", "2026-08-01 00:00:00")
	linkLabel(t, db, want, label)

	wrongCat := "78000000-0000-0000-0000-000000000022"
	seedExpense(t, db, wrongCat, acctA, catB, "30.00", "2026-08-10 00:00:00")
	linkLabel(t, db, wrongCat, label)

	wrongLabel := "78000000-0000-0000-0000-000000000023"
	seedExpense(t, db, wrongLabel, acctA, catA, "40.00", "2026-08-11 00:00:00")
	linkLabel(t, db, wrongLabel, other)

	wantUncat := "78000000-0000-0000-0000-000000000024"
	seedExpense(t, db, wantUncat, acctA, "", "50.00", "2026-08-12 00:00:00")
	linkLabel(t, db, wantUncat, label)

	otherUncat := "78000000-0000-0000-0000-000000000025"
	seedExpense(t, db, otherUncat, acctA, "", "60.00", "2026-08-13 00:00:00")
	linkLabel(t, db, otherUncat, other)

	// Out of period on the wanted pair: the bounds must still apply.
	outOfPeriod := "78000000-0000-0000-0000-000000000026"
	seedExpense(t, db, outOfPeriod, acctA, catA, "70.00", "2026-09-01 00:00:00")
	linkLabel(t, db, outOfPeriod, label)

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	accIDs := []vo.Id{vo.MustParseId(acctA)}

	rows, err := read.BudgetTransactionsByLabelAndCategory(ctx, vo.MustParseId(label), vo.MustParseId(catA), accIDs, start, end)
	if err != nil {
		t.Fatalf("BudgetTransactionsByLabelAndCategory: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != want {
		t.Fatalf("want only %q (the first-of-month row), got %+v", want, rows)
	}

	uncat, err := read.BudgetTransactionsByLabelUncategorized(ctx, vo.MustParseId(label), accIDs, start, end)
	if err != nil {
		t.Fatalf("BudgetTransactionsByLabelUncategorized: %v", err)
	}
	if len(uncat) != 1 || uncat[0].ID != wantUncat {
		t.Fatalf("want only %q, got %+v", wantUncat, uncat)
	}

	// An empty account set short-circuits, like every sibling query.
	if none, err := read.BudgetTransactionsByLabelAndCategory(ctx, vo.MustParseId(label), vo.MustParseId(catA), nil, start, end); err != nil || none != nil {
		t.Fatalf("no accounts: rows=%v err=%v, want nil/nil", none, err)
	}
	if none, err := read.BudgetTransactionsByLabelUncategorized(ctx, vo.MustParseId(label), nil, start, end); err != nil || none != nil {
		t.Fatalf("no accounts: rows=%v err=%v, want nil/nil", none, err)
	}
}
