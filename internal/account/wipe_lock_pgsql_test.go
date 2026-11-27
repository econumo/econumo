//go:build enginecompare

package account_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// secondWaits runs hold in a transaction on the test's pinned connection and,
// while it is open, second in a transaction on a SECOND pool over the same
// schema. It reports whether second was still blocked after 300 ms, then
// commits the first and requires second to finish cleanly.
func secondWaits(t *testing.T, db *dbtest.DB, hold func(ctx context.Context) error, second func(ctx context.Context, tx *backend.TxManager) error) bool {
	t.Helper()
	if db.Engine != "postgresql" {
		t.Skipf("locks are only observable on a real pool; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
	}
	ctx := context.Background()

	var schema string
	if err := db.Raw.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatalf("read current_schema: %v", err)
	}
	raw2, err := pgsql.OpenDB(os.Getenv(dbtest.PgsqlURLEnv))
	if err != nil {
		t.Fatalf("open second pool: %v", err)
	}
	t.Cleanup(func() { _ = raw2.Close() })
	raw2.SetMaxOpenConns(1)
	if _, err := raw2.ExecContext(ctx, fmt.Sprintf(`SET search_path TO %q`, schema)); err != nil {
		t.Fatalf("set search_path on the second pool: %v", err)
	}
	tx2 := backend.NewTxManager(raw2)

	locked := make(chan struct{})
	release := make(chan struct{})
	committed := make(chan error, 1)
	go func() {
		committed <- db.TX.WithTx(ctx, func(ctx context.Context) error {
			if err := hold(ctx); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-committed:
		t.Fatalf("first transaction ended before taking the lock: %v", err)
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("first transaction never took the lock")
	}

	done := make(chan error, 1)
	go func() {
		done <- tx2.WithTx(ctx, func(ctx context.Context) error { return second(ctx, tx2) })
	}()

	// Never t.Fatal while the first transaction is parked: the cleanup DROP
	// SCHEMA would wait on its lock. Record the verdict, release, assert after.
	waited := true
	var early error
	select {
	case early = <-done:
		waited = false
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-committed; err != nil {
		t.Fatalf("first transaction: %v", err)
	}
	if !waited {
		if early != nil {
			t.Fatalf("second transaction: %v", early)
		}
		return false
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second transaction after the first commit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the second transaction did not complete within 2s of the first commit")
	}
	return true
}

func TestLockOwnedForWipe_PgsqlBlocksTransactionWrites(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	user := f.User(fixture.User{Email: "wipe-lock@example.test"})
	account := f.Account(fixture.Account{UserID: user})
	svc := newAccessSvc(t, db)
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

	waited := secondWaits(t, db,
		func(ctx context.Context) error { return svc.LockOwnedForWipe(ctx, mustParseID(t, user)) },
		func(ctx context.Context, tx *backend.TxManager) error {
			_, err := tx.Querier(ctx).ExecContext(ctx,
				`INSERT INTO transactions (id, user_id, account_id, type, amount, description, spent_at, created_at, updated_at)
				VALUES ($1, $2, $3, 0, '1.00', '', $4, $4, $4)`,
				fixture.NewID(), user, account, now)
			return err
		})
	if !waited {
		t.Fatal("an INSERT into transactions on the user's account returned while LockOwnedForWipe held it")
	}
}
