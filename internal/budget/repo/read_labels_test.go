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
// contribute a row -- deleting the WHERE/JOIN would let either leak in.
func TestCountSpendingByLabelExcludesOutOfPeriodAndUnlabeled(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	label := f.Label(fixture.Label{UserID: userA, Name: "Groceries"})

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
		t.Fatalf("want 1 row (the in-period labeled expense only), got %d: %+v", len(rows), rows)
	}
	if rows[0].LabelID != label {
		t.Fatalf("wrong label: %q", rows[0].LabelID)
	}
	if vo.NewDecimal(rows[0].Amount).String() != vo.NewDecimal("10").String() {
		t.Errorf("amount mismatch: %q", rows[0].Amount)
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
// id, with position carried through (Task 2 renders the block in position
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
	if m.Name != "First" || m.Icon != "star" || m.Position != 1 || m.IsArchived || m.OwnerID != userA {
		t.Errorf("first label mismatch: %+v", m)
	}
	m2 := got[second]
	if m2.Name != "Second" || m2.Icon != "flag" || m2.Position != 0 || !m2.IsArchived {
		t.Errorf("second (archived) label mismatch: %+v", m2)
	}

	none, err := read.LabelsForUsers(ctx, nil)
	if err != nil || none != nil {
		t.Fatalf("empty user id set should be nil,nil; got %v, %v", none, err)
	}
}
