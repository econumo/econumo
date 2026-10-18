package txrevision_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/infra/txrevision"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

type env struct {
	db      *dbtest.DB
	bumper  *txrevision.Bumper
	a, b, c string
	cat     string
}

func setup(t *testing.T) env {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := f.User(fixture.User{})
	e := env{
		db:     db,
		bumper: txrevision.New(db.Engine, db.TX),
		a:      f.Account(fixture.Account{UserID: u}),
		b:      f.Account(fixture.Account{UserID: u}),
		c:      f.Account(fixture.Account{UserID: u}),
		cat:    f.Category(fixture.Category{UserID: u}),
	}
	// Transfer a -> b in the category, an expense on a, and an unrelated one on c.
	f.Transaction(fixture.Transaction{UserID: u, AccountID: e.a, AccountRecipientID: e.b, CategoryID: e.cat, Type: 2, Amount: "1", AmountRecipient: "1"})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: e.a, Type: 0, Amount: "1"})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: e.c, Type: 0, Amount: "1"})
	return e
}

func (e env) rev(t *testing.T, id string) int {
	t.Helper()
	var n int
	if err := e.db.Raw.QueryRow(e.db.Rebind("SELECT transactions_revision FROM accounts WHERE id = ?"), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e env) want(t *testing.T, a, b, c int) {
	t.Helper()
	if ga, gb, gc := e.rev(t, e.a), e.rev(t, e.b), e.rev(t, e.c); ga != a || gb != b || gc != c {
		t.Fatalf("revisions a=%d b=%d c=%d, want %d %d %d", ga, gb, gc, a, b, c)
	}
}

func TestBump_OncePerAccountPerTransaction(t *testing.T) {
	e := setup(t)
	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		for i := 0; i < 3; i++ {
			if err := e.bumper.Bump(ctx, e.a); err != nil {
				return err
			}
		}
		if err := e.bumper.Bump(ctx, e.b); err != nil {
			return err
		}
		var n int
		q := e.db.Rebind("SELECT transactions_revision FROM accounts WHERE id = ?")
		if err := e.db.TX.Querier(ctx).QueryRowContext(ctx, q, e.a).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatalf("revision moved before commit: %d", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	e.want(t, 1, 1, 0)
}

func TestBump_NestedSavepointSharesOnePending(t *testing.T) {
	e := setup(t)
	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		if err := e.bumper.Bump(ctx, e.a); err != nil {
			return err
		}
		return e.db.TX.WithTx(ctx, func(ctx context.Context) error { return e.bumper.Bump(ctx, e.a, e.b) })
	})
	if err != nil {
		t.Fatal(err)
	}
	e.want(t, 1, 1, 0)
}

func TestBump_OutsideTransactionAppliesImmediately(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.bumper.Bump(ctx, e.a); err != nil {
		t.Fatal(err)
	}
	if err := e.bumper.Bump(ctx, e.a); err != nil {
		t.Fatal(err)
	}
	e.want(t, 2, 0, 0)
}

func TestBump_RolledBackTransactionChangesNothing(t *testing.T) {
	e := setup(t)
	boom := errors.New("boom")
	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		if err := e.bumper.Bump(ctx, e.a); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	e.want(t, 0, 0, 0)
}

func TestBump_IgnoresEmptyAndDuplicateIDsAnyOrder(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.bumper.Bump(ctx, "", e.a, e.a, e.b, ""); err != nil {
		t.Fatal(err)
	}
	e.want(t, 1, 1, 0)
	err := e.db.TX.WithTx(ctx, func(ctx context.Context) error {
		if err := e.bumper.Bump(ctx, e.b, e.a); err != nil {
			return err
		}
		return e.bumper.Bump(ctx, e.a, e.b, "")
	})
	if err != nil {
		t.Fatal(err)
	}
	e.want(t, 2, 2, 0)
}

func TestBumpWhere_BumpsAccountAndTransferRecipient(t *testing.T) {
	e := setup(t)
	if err := e.bumper.BumpWhere(context.Background(), "category_id = ?", e.cat); err != nil {
		t.Fatal(err)
	}
	e.want(t, 1, 1, 0)
}

func TestBumpWhere_InsideTransactionDefersToCommit(t *testing.T) {
	e := setup(t)
	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		if err := e.bumper.BumpWhere(ctx, "category_id = ?", e.cat); err != nil {
			return err
		}
		return e.bumper.Bump(ctx, e.a)
	})
	if err != nil {
		t.Fatal(err)
	}
	e.want(t, 1, 1, 0)
}
