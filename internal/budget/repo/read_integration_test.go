package repo_test

// Integration tests for the budget ReadRepo report queries against a real
// migrated in-memory SQLite. Regression-locks the month-boundary datetime
// binding (a first-of-month transaction/limit must be INCLUDED) and the
// exact scale-8 decimal sums from float SUM rendering.

import (
	"context"
	"testing"
	"time"

	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func seedAccount(t *testing.T, db *dbtest.DB, id, userID string) {
	t.Helper()
	fixture.New(t, db).Account(fixture.Account{ID: id, CurrencyID: usdID, UserID: userID, Name: "A", Icon: "x"})
}

func seedCategory(t *testing.T, db *dbtest.DB, id, userID string) {
	t.Helper()
	fixture.New(t, db).Category(fixture.Category{ID: id, UserID: userID, Name: "C", Icon: "x"})
}

func seedExpense(t *testing.T, db *dbtest.DB, id, account, category, amount, spentAt string) {
	t.Helper()
	fixture.New(t, db).Transaction(fixture.Transaction{ID: id, UserID: userA, AccountID: account, CategoryID: category, Type: 0, Amount: amount, SpentAt: spentAt})
}

func newReadRepo(t *testing.T) (*budgetrepo.ReadRepo, *dbtest.DB) {
	t.Helper()
	db := dbtest.New(t)
	seedUser(t, db, userA)
	seedAccount(t, db, acctA, userA)
	return budgetrepo.NewReadRepo(db.Engine, db.TX), db
}

func TestBudgetReadRepo_AccountsBalances(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	cat := "c0000000-0000-0000-0000-0000000000c1"
	seedCategory(t, db, cat, userA)
	// Two incomes minus one expense; float sum must render clean.
	f := fixture.New(t, db)
	f.Transaction(fixture.Transaction{ID: "70000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, Type: 1, Amount: "100.10", SpentAt: "2024-03-10 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "70000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, Type: 1, Amount: "200.20", SpentAt: "2024-03-11 00:00:00"})
	seedExpense(t, db, "70000000-0000-0000-0000-000000000003", acctA, cat, "0.30", "2024-03-12 00:00:00")

	onDate := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.AccountsBalancesOnDate(ctx, []vo.Id{vo.MustParseId(acctA)}, onDate)
	if err != nil {
		t.Fatalf("AccountsBalancesOnDate: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 balance row, got %d", len(rows))
	}
	// 100.10 + 200.20 - 0.30 = 300.00; the float SUM renders to scale 8.
	if rows[0].Balance != "300.00000000" {
		t.Errorf("balance mismatch: %q", rows[0].Balance)
	}
	if rows[0].CurrencyID != usdID {
		t.Errorf("currency mismatch: %q", rows[0].CurrencyID)
	}

	// Empty id set -> nil.
	none, err := read.AccountsBalancesOnDate(ctx, nil, onDate)
	if err != nil || none != nil {
		t.Errorf("empty ids should be nil,nil; got %v, %v", none, err)
	}
}

func TestBudgetReadRepo_CountSpending_MonthBoundary(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	cat := "c0000000-0000-0000-0000-0000000000c1"
	seedCategory(t, db, cat, userA)
	// Boundary on the first of the month must be included; previous-month excluded.
	seedExpense(t, db, "70000000-0000-0000-0000-000000000010", acctA, cat, "10.00", "2024-04-01 00:00:00")
	seedExpense(t, db, "70000000-0000-0000-0000-000000000011", acctA, cat, "5.50", "2024-04-15 00:00:00")
	seedExpense(t, db, "70000000-0000-0000-0000-000000000012", acctA, cat, "99.00", "2024-03-31 23:59:59")

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.CountSpending(ctx, []vo.Id{vo.MustParseId(cat)}, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("CountSpending: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 spending row, got %d", len(rows))
	}
	// 10.00 + 5.50 = 15.50 (incl. Apr 1 boundary, excl. Mar 31).
	if rows[0].Amount != "15.50000000" {
		t.Errorf("spending amount mismatch: %q", rows[0].Amount)
	}

	// Empty account id set (every account excluded from the budget) -> nil,nil,
	// not an "a.id IN ()" query: that's a no-op on SQLite but a PostgreSQL
	// syntax error, so it must short-circuit like the empty-categoryIDs case.
	none, err := read.CountSpending(ctx, []vo.Id{vo.MustParseId(cat)}, nil, start, end)
	if err != nil || none != nil {
		t.Errorf("empty account ids should be nil,nil; got %v, %v", none, err)
	}
}

func TestBudgetReadRepo_BudgetTransactionsByTag(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	cat := "c0000000-0000-0000-0000-0000000000c1"
	tag := "7a000000-0000-0000-0000-0000000000a1"
	seedCategory(t, db, cat, userA)
	f := fixture.New(t, db)
	f.Tag(fixture.Tag{ID: tag, UserID: userA, Name: "Tag"})
	f.Transaction(fixture.Transaction{ID: "72000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, CategoryID: cat, TagID: tag, Type: 0, Amount: "12.50", SpentAt: "2024-04-05 00:00:00"})
	// A same-period expense tagged differently must not leak into the result.
	otherTag := "7a000000-0000-0000-0000-0000000000a2"
	f.Tag(fixture.Tag{ID: otherTag, UserID: userA, Name: "Other"})
	f.Transaction(fixture.Transaction{ID: "72000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, CategoryID: cat, TagID: otherTag, Type: 0, Amount: "99.00", SpentAt: "2024-04-06 00:00:00"})

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)

	rows, err := read.BudgetTransactionsByTag(ctx, vo.MustParseId(tag), nil, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("BudgetTransactionsByTag: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 tagged transaction, got %d: %+v", len(rows), rows)
	}
	if vo.NewDecimal(rows[0].Amount).String() != vo.NewDecimal("12.5").String() {
		t.Errorf("amount mismatch: %q", rows[0].Amount)
	}
	if rows[0].TagID == nil || *rows[0].TagID != tag {
		t.Errorf("tagId mismatch: %+v", rows[0].TagID)
	}

	// The categoryID filter narrows further; a non-matching category yields none.
	catID := vo.MustParseId(cat)
	withCat, err := read.BudgetTransactionsByTag(ctx, vo.MustParseId(tag), &catID, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("BudgetTransactionsByTag with category: %v", err)
	}
	if len(withCat) != 1 {
		t.Fatalf("want 1 row with matching category filter, got %d", len(withCat))
	}
	otherCatID := vo.NewId()
	none, err := read.BudgetTransactionsByTag(ctx, vo.MustParseId(tag), &otherCatID, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("BudgetTransactionsByTag mismatching category: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("want 0 rows for a non-matching category, got %d", len(none))
	}

	// Empty account id set short-circuits to nil, nil.
	empty, err := read.BudgetTransactionsByTag(ctx, vo.MustParseId(tag), nil, nil, start, end)
	if err != nil || empty != nil {
		t.Errorf("empty account ids should be nil,nil; got %v, %v", empty, err)
	}
}

func TestBudgetReadRepo_SummarizedLimits_MonthBoundary(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	// Seed a budget + element + limits directly so the period range is testable.
	f := fixture.New(t, db)
	f.Budget(fixture.Budget{ID: budgetID, UserID: userA, CurrencyID: usdID, Name: "B", StartedAt: startedAt})
	eid := "e0000000-0000-0000-0000-0000000000e1"
	externalID := "ec000000-0000-0000-0000-0000000000c1"
	f.BudgetElement(fixture.BudgetElement{ID: eid, BudgetID: budgetID, ExternalID: externalID, Type: 1, Position: 0})
	// Two limits: April (in range) + May (out of range for an April-only window).
	f.BudgetLimit(fixture.BudgetLimit{ID: "71000000-0000-0000-0000-000000000001", ElementID: eid, Period: "2024-04-01 00:00:00", Amount: "120.55"})
	f.BudgetLimit(fixture.BudgetLimit{ID: "71000000-0000-0000-0000-000000000002", ElementID: eid, Period: "2024-05-01 00:00:00", Amount: "300.00"})

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.SummarizedLimits(ctx, vo.MustParseId(budgetID), start, end)
	if err != nil {
		t.Fatalf("SummarizedLimits: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 summarized limit (April only), got %d", len(rows))
	}
	if rows[0].Amount != "120.55000000" {
		t.Errorf("summarized limit mismatch: %q", rows[0].Amount)
	}
	if rows[0].ExternalID != externalID {
		t.Errorf("external id mismatch: %q", rows[0].ExternalID)
	}
}

// BudgetTransactionsByCategories backs the drill-down of a standalone category
// row and of an envelope's children. Both display the UNTAGGED bucket, so the
// query deliberately excludes tagged rows ("t.tag_id IS NULL"). A category
// nested under a tag shows the opposite bucket and must not use this query.
func TestBudgetReadRepo_BudgetTransactionsByCategories(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	cat := "c0000000-0000-0000-0000-0000000000c1"
	tag := "7a000000-0000-0000-0000-0000000000b1"
	seedCategory(t, db, cat, userA)
	f := fixture.New(t, db)
	f.Tag(fixture.Tag{ID: tag, UserID: userA, Name: "Tag"})
	// The only row this query should return.
	seedExpense(t, db, "73000000-0000-0000-0000-000000000001", acctA, cat, "10.00", "2024-04-05 00:00:00")
	// Same category but tagged: excluded by design.
	f.Transaction(fixture.Transaction{ID: "73000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, CategoryID: cat, TagID: tag, Type: 0, Amount: "99.00", SpentAt: "2024-04-06 00:00:00"})
	// An income in the same category is not budget spending (type != 0).
	f.Transaction(fixture.Transaction{ID: "73000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, CategoryID: cat, Type: 1, Amount: "50.00", SpentAt: "2024-04-07 00:00:00"})
	// Outside the period.
	seedExpense(t, db, "73000000-0000-0000-0000-000000000004", acctA, cat, "77.00", "2024-05-02 00:00:00")

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	catIDs := []vo.Id{vo.MustParseId(cat)}
	accIDs := []vo.Id{vo.MustParseId(acctA)}

	rows, err := read.BudgetTransactionsByCategories(ctx, catIDs, accIDs, start, end)
	if err != nil {
		t.Fatalf("BudgetTransactionsByCategories: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 untagged in-period expense, got %d: %+v", len(rows), rows)
	}
	if rows[0].ID != "73000000-0000-0000-0000-000000000001" {
		t.Errorf("wrong row returned: %q", rows[0].ID)
	}

	// Empty id sets short-circuit: "IN ()" is a no-op on SQLite but a PostgreSQL
	// syntax error, so both lists must be guarded.
	if none, err := read.BudgetTransactionsByCategories(ctx, nil, accIDs, start, end); err != nil || none != nil {
		t.Errorf("empty category ids should be nil,nil; got %v, %v", none, err)
	}
	if none, err := read.BudgetTransactionsByCategories(ctx, catIDs, nil, start, end); err != nil || none != nil {
		t.Errorf("empty account ids should be nil,nil; got %v, %v", none, err)
	}
}

// A first-of-month transaction is counted in the row total by CountSpending, so
// the drill-down list must contain it too — otherwise the list silently
// contradicts the number above it. SQLite only: the bounds must be bound as
// 'Y-m-d H:i:s' strings, not as time.Time (see sqliteDatetime).
func TestBudgetReadRepo_Drilldown_MonthBoundary(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	cat := "c0000000-0000-0000-0000-0000000000c1"
	tag := "7a000000-0000-0000-0000-0000000000c2"
	seedCategory(t, db, cat, userA)
	f := fixture.New(t, db)
	f.Tag(fixture.Tag{ID: tag, UserID: userA, Name: "Tag"})
	seedExpense(t, db, "74000000-0000-0000-0000-000000000001", acctA, cat, "10.00", "2024-04-01 00:00:00")
	f.Transaction(fixture.Transaction{ID: "74000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, CategoryID: cat, TagID: tag, Type: 0, Amount: "20.00", SpentAt: "2024-04-01 00:00:00"})
	// The previous month must stay excluded (the lower bound is inclusive, the
	// upper bound exclusive).
	seedExpense(t, db, "74000000-0000-0000-0000-000000000003", acctA, cat, "99.00", "2024-03-31 23:59:59")
	seedExpense(t, db, "74000000-0000-0000-0000-000000000004", acctA, cat, "88.00", "2024-05-01 00:00:00")

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	accIDs := []vo.Id{vo.MustParseId(acctA)}

	byCat, err := read.BudgetTransactionsByCategories(ctx, []vo.Id{vo.MustParseId(cat)}, accIDs, start, end)
	if err != nil {
		t.Fatalf("BudgetTransactionsByCategories: %v", err)
	}
	if len(byCat) != 1 {
		t.Fatalf("want the Apr 1 untagged expense in the list, got %d rows: %+v", len(byCat), byCat)
	}
	if byCat[0].ID != "74000000-0000-0000-0000-000000000001" {
		t.Errorf("wrong row: %q", byCat[0].ID)
	}

	byTag, err := read.BudgetTransactionsByTag(ctx, vo.MustParseId(tag), nil, accIDs, start, end)
	if err != nil {
		t.Fatalf("BudgetTransactionsByTag: %v", err)
	}
	if len(byTag) != 1 {
		t.Fatalf("want the Apr 1 tagged expense in the list, got %d rows: %+v", len(byTag), byTag)
	}
	if byTag[0].ID != "74000000-0000-0000-0000-000000000002" {
		t.Errorf("wrong row: %q", byTag[0].ID)
	}
}

// A transfer dated the 1st of the month must be counted in HoldingsReport, and
// the next month's 1st must be excluded. SQLite only: the bounds must be
// bound as 'Y-m-d H:i:s' strings, not as time.Time (see sqliteDatetime).
func TestBudgetReadRepo_HoldingsReport_MonthBoundary(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	acctB := "aaaa1111-0000-0000-0000-0000000000a2"
	seedAccount(t, db, acctB, userA)
	f := fixture.New(t, db)
	// A same-currency transfer OUT of the {acctA} set (recipient acctB is
	// outside): the Apr 1 boundary must be INCLUDED.
	f.Transaction(fixture.Transaction{ID: "75000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, AccountRecipientID: acctB, Type: 2, Amount: "10.00", AmountRecipient: "10.00", SpentAt: "2024-04-01 00:00:00"})
	// The next month's 1st must be EXCLUDED (upper bound is exclusive).
	f.Transaction(fixture.Transaction{ID: "75000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, AccountRecipientID: acctB, Type: 2, Amount: "88.00", AmountRecipient: "88.00", SpentAt: "2024-05-01 00:00:00"})

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.HoldingsReport(ctx, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("HoldingsReport: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 holdings row (Apr 1 transfer only), got %d: %+v", len(rows), rows)
	}
	if vo.NewDecimal(rows[0].ToHoldings).String() != vo.NewDecimal("10.00").String() {
		t.Errorf("to holdings mismatch: %q", rows[0].ToHoldings)
	}
	if rows[0].CurrencyID != usdID {
		t.Errorf("currency mismatch: %q", rows[0].CurrencyID)
	}
}

// Spending with no category must reach the builder so it can be shown as
// "Uncategorized". Tagged rows keep their tag: the tag bucket wins, and the
// uncategorized row only collects what is neither categorized nor tagged.
func TestBudgetReadRepo_CountSpending_NullCategory(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	cat := "c0000000-0000-0000-0000-0000000000c1"
	tag := "7a000000-0000-0000-0000-0000000000d1"
	seedCategory(t, db, cat, userA)
	f := fixture.New(t, db)
	f.Tag(fixture.Tag{ID: tag, UserID: userA, Name: "Tag"})
	// Categorized, untagged.
	seedExpense(t, db, "75000000-0000-0000-0000-000000000001", acctA, cat, "10.00", "2024-04-05 00:00:00")
	// No category, no tag -> the uncategorized bucket.
	f.Transaction(fixture.Transaction{ID: "75000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, Type: 0, Amount: "20.00", SpentAt: "2024-04-06 00:00:00"})
	// No category but tagged -> the tag bucket.
	f.Transaction(fixture.Transaction{ID: "75000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, TagID: tag, Type: 0, Amount: "30.00", SpentAt: "2024-04-07 00:00:00"})

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.CountSpending(ctx, []vo.Id{vo.MustParseId(cat)}, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("CountSpending: %v", err)
	}

	var categorized, uncatUntagged, uncatTagged int
	for _, r := range rows {
		switch {
		case r.CategoryID != nil:
			categorized++
		case r.TagID != nil && *r.TagID != "":
			uncatTagged++
		default:
			uncatUntagged++
		}
	}
	if categorized != 1 || uncatUntagged != 1 || uncatTagged != 1 {
		t.Fatalf("want one row of each kind; got categorized=%d uncategorized-untagged=%d uncategorized-tagged=%d: %+v",
			categorized, uncatUntagged, uncatTagged, rows)
	}

	// An empty category list must still return the uncategorized rows rather
	// than short-circuiting -- and must not emit "IN ()", a PostgreSQL syntax
	// error.
	noCats, err := read.CountSpending(ctx, nil, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("CountSpending with no categories: %v", err)
	}
	if len(noCats) != 2 {
		t.Fatalf("want the 2 uncategorized rows when no categories are given, got %d: %+v", len(noCats), noCats)
	}

	// No accounts still short-circuits.
	if none, err := read.CountSpending(ctx, []vo.Id{vo.MustParseId(cat)}, nil, start, end); err != nil || none != nil {
		t.Errorf("empty account ids should be nil,nil; got %v, %v", none, err)
	}
}
