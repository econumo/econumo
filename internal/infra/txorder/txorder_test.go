package txorder_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/txorder"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestPlacerTop(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := f.User(fixture.User{})
	a := f.Account(fixture.Account{UserID: u})
	b := f.Account(fixture.Account{UserID: u})
	p := txorder.New(db.Engine, db.TX)
	ctx := context.Background()
	day := time.Date(2026, 10, 1, 15, 4, 5, 0, time.UTC)

	top := func(account string) string {
		t.Helper()
		k, err := p.Top(ctx, account, day)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	if k := top(a); k != "a0" {
		t.Fatalf("empty day: %q, want a0", k)
	}
	f.Transaction(fixture.Transaction{UserID: u, AccountID: a, Type: 0, SpentAt: "2026-10-01 01:00:00", SortKey: "a1"})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: a, Type: 0, SpentAt: "2026-10-01 23:59:59", SortKey: "a3"})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: b, AccountRecipientID: a, Type: 2, SpentAt: "2026-10-01 00:00:00", SortKey: "a9", RecipientSortKey: "a5"})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: a, Type: 0, SpentAt: "2026-10-02 00:00:00", SortKey: "z0"})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: a, Type: 0, SpentAt: "2026-09-30 23:59:59", SortKey: "z0"})
	if k := top(a); k != "a6" {
		t.Errorf("a's day: %q, want a6 (just above its highest key on either side, a5)", k)
	}
	if k := top(b); k != "aA" {
		t.Errorf("b's day: %q, want aA (just above a9)", k)
	}
}

// Rows written before the keys existed carry ” / NULL. Seeding numbers every
// account's days in the order the list showed (spent_at DESC, id), counting
// the rows it is the source of and the transfers it receives together, and is
// idempotent.
func TestSeedMissing(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := f.User(fixture.User{})
	a := f.Account(fixture.Account{UserID: u})
	b := f.Account(fixture.Account{UserID: u})
	ctx := context.Background()
	raw := func(id, from string, to any, spentAt string) {
		t.Helper()
		typ := 0
		if to != nil {
			typ = 2
		}
		if _, err := db.Raw.ExecContext(ctx, db.Rebind(`INSERT INTO transactions (id, user_id, account_id, account_recipient_id, type, amount, description, spent_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 1, '', ?, ?, ?)`), id, u, from, to, typ, spentAt, spentAt, spentAt); err != nil {
			t.Fatal(err)
		}
	}
	const (
		e1, in, e2, tie1, tie2, other = "01a20000-0000-7000-8000-000000000001", "01a20000-0000-7000-8000-000000000002", "01a20000-0000-7000-8000-000000000003",
			"01a20000-0000-7000-8000-000000000004", "01a20000-0000-7000-8000-000000000005", "01a20000-0000-7000-8000-000000000006"
	)
	raw(e1, a, nil, "2026-10-02 10:00:00")
	raw(in, b, a, "2026-10-02 12:00:00")
	raw(e2, a, nil, "2026-10-02 14:00:00")
	raw(tie1, a, nil, "2026-10-01 00:00:00")
	raw(tie2, a, nil, "2026-10-01 00:00:00")
	raw(other, b, nil, "2026-10-02 23:00:00")
	rev := func(acct string) (n int) {
		t.Helper()
		if err := db.Raw.QueryRowContext(ctx, db.Rebind(`SELECT transactions_revision FROM accounts WHERE id = ?`), acct).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return
	}
	revA, revB := rev(a), rev(b)

	p := txorder.New(db.Engine, db.TX)
	n, err := p.SeedMissing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("seeded %d accounts, want 2", n)
	}
	keys := func(id string) (string, sql.NullString) {
		t.Helper()
		var k string
		var rk sql.NullString
		if err := db.Raw.QueryRowContext(ctx, db.Rebind(`SELECT sort_key, recipient_sort_key FROM transactions WHERE id = ?`), id).Scan(&k, &rk); err != nil {
			t.Fatal(err)
		}
		return k, rk
	}
	k1, _ := keys(e1)
	k2, _ := keys(e2)
	kin, rin := keys(in)
	if !(k2 > rin.String && rin.String > k1) {
		t.Errorf("a's day: e2 %q, incoming %q, e1 %q, want descending (the order by time)", k2, rin.String, k1)
	}
	kOther, _ := keys(other)
	if !(kOther > kin) {
		t.Errorf("b's day: other %q, transfer %q, want other above", kOther, kin)
	}
	t1, _ := keys(tie1)
	t2, _ := keys(tie2)
	if !(t1 > t2) {
		t.Errorf("tied times: %q %q, want the lower id on top", t1, t2)
	}
	if rev(a) != revA+1 || rev(b) != revB+1 {
		t.Errorf("revisions a %d->%d, b %d->%d, want one bump each", revA, rev(a), revB, rev(b))
	}
	if n, err := p.SeedMissing(ctx); err != nil || n != 0 {
		t.Errorf("second run: %d accounts, %v; want none", n, err)
	}
	if again, _ := keys(e1); again != k1 {
		t.Errorf("second run rewrote e1: %q -> %q", k1, again)
	}
}
