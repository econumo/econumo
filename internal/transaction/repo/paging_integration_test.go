package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	domtransaction "github.com/econumo/econumo/internal/transaction"
	txrepo "github.com/econumo/econumo/internal/transaction/repo"
)

const (
	pUSD   = "dffc2a06-6f29-4704-8575-31709adee926"
	pUser  = "11111111-1111-1111-1111-111111111111"
	pAcctA = "a0000000-0000-0000-0000-0000000000a1"
	pAcctB = "a0000000-0000-0000-0000-0000000000b1"
)

func pagingSetup(t *testing.T) (*txrepo.Repo, *dbtest.DB) {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: pUser, Name: "u"})
	f.Account(fixture.Account{ID: pAcctA, UserID: pUser, CurrencyID: pUSD, Name: "A"})
	f.Account(fixture.Account{ID: pAcctB, UserID: pUser, CurrencyID: pUSD, Name: "B"})
	return txrepo.NewRepo(db.Engine, db.TX), db
}

func seedTx(t *testing.T, db *dbtest.DB, id, account string, recipient *string, spentAt time.Time) {
	t.Helper()
	_, err := db.Raw.Exec(db.Rebind(`INSERT INTO transactions
		(id, user_id, account_id, account_recipient_id, type, amount, amount_recipient, description, spent_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		id, pUser, account, recipient, txType(recipient), "5.00000000", recipAmount(recipient), "t", spentAt, spentAt, spentAt)
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func txType(recipient *string) int {
	if recipient != nil {
		return 2 // transfer
	}
	return 0 // expense
}

func recipAmount(recipient *string) *string {
	if recipient != nil {
		a := "5.00000000"
		return &a
	}
	return nil
}

func day(d int) time.Time { return time.Date(2026, 6, d, 12, 0, 0, 0, time.UTC) }

// ids chosen so the same-timestamp tie-break (id ASC) is observable
func TestListPageByAccount_Keyset(t *testing.T) {
	repo, db := pagingSetup(t)
	ctx := context.Background()
	seedTx(t, db, "d0000000-0000-0000-0000-000000000001", pAcctA, nil, day(3)) // newest, tie
	seedTx(t, db, "d0000000-0000-0000-0000-000000000002", pAcctA, nil, day(3)) // tie, larger id -> second
	seedTx(t, db, "d0000000-0000-0000-0000-000000000003", pAcctA, nil, day(2))
	seedTx(t, db, "d0000000-0000-0000-0000-000000000004", pAcctA, nil, day(1))
	acct := vo.MustParseId(pAcctA)

	first, err := repo.ListPageByAccount(ctx, acct, nil, 3)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	wantIDs(t, first, "…001", "…002", "…003")

	after := &domtransaction.PageCursor{SpentAt: first[1].SpentAt, ID: first[1].ID} // cursor at …002
	second, err := repo.ListPageByAccount(ctx, acct, after, 3)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	wantIDs(t, second, "…003", "…004")
}

func TestListRecentByAccountIDs_PartitionsAndTransfers(t *testing.T) {
	repo, db := pagingSetup(t)
	ctx := context.Background()
	b := pAcctB
	// transfer A->B: must appear in BOTH windows
	seedTx(t, db, "d0000000-0000-0000-0000-000000000011", pAcctA, &b, day(5))
	seedTx(t, db, "d0000000-0000-0000-0000-000000000012", pAcctA, nil, day(4))
	seedTx(t, db, "d0000000-0000-0000-0000-000000000013", pAcctA, nil, day(3))
	seedTx(t, db, "d0000000-0000-0000-0000-000000000014", pAcctB, nil, day(2))

	windows, err := repo.ListRecentByAccountIDs(ctx,
		[]vo.Id{vo.MustParseId(pAcctA), vo.MustParseId(pAcctB)}, 2)
	if err != nil {
		t.Fatalf("ListRecentByAccountIDs: %v", err)
	}
	wantIDs(t, windows[pAcctA], "…011", "…012") // newest 2 of A's 3
	wantIDs(t, windows[pAcctB], "…011", "…014") // transfer counts for B too
}

func TestListRecentByAccountIDs_Empty(t *testing.T) {
	repo, _ := pagingSetup(t)
	windows, err := repo.ListRecentByAccountIDs(context.Background(), nil, 2)
	if err != nil {
		t.Fatalf("empty ids: %v", err)
	}
	if len(windows) != 0 {
		t.Fatalf("windows = %v, want empty", windows)
	}
}

// wantIDs asserts the ordered id suffixes ("…0NN" means suffix "0NN") match.
func wantIDs(t *testing.T, rows []*model.Transaction, suffixes ...string) {
	t.Helper()
	if len(rows) != len(suffixes) {
		t.Fatalf("got %d rows, want %d", len(rows), len(suffixes))
	}
	for i, want := range suffixes {
		got := rows[i].ID.String()
		if got[len(got)-3:] != want[len(want)-3:] {
			t.Errorf("row %d id = %s, want suffix %s", i, got, want)
		}
	}
}
