//go:build enginecompare

package dbtest

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
)

// AssertLockWaitsForKeyShare holds FOR KEY SHARE on the row id of table (as a
// debt write adding a rule on it does) in one transaction and asserts that
// lock, run in a transaction on a SECOND pool over the same schema, does not
// return until the first commits. Every other suite pins its database to one
// connection, so a second transaction could not even begin there. Skips on
// SQLite, which has no row-level locks.
func AssertLockWaitsForKeyShare(t testing.TB, db *DB, table, id string, lock func(ctx context.Context, tx *backend.TxManager) error) {
	t.Helper()
	if db.Engine != "postgresql" {
		t.Skipf("the row lock is only observable on a real pool; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
	}
	ctx := context.Background()

	var schema string
	if err := db.Raw.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatalf("read current_schema: %v", err)
	}
	raw2, err := pgsql.OpenDB(os.Getenv(PgsqlURLEnv))
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
			if _, err := db.TX.Querier(ctx).ExecContext(ctx, "SELECT id FROM "+table+" WHERE id = $1 FOR KEY SHARE", id); err != nil {
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

	second := make(chan error, 1)
	go func() {
		second <- tx2.WithTx(ctx, func(ctx context.Context) error { return lock(ctx, tx2) })
	}()

	// Never t.Fatal while the first transaction is parked: the cleanup DROP
	// SCHEMA would wait on its lock. Record the verdict, release, assert after.
	var tooEarly error
	var returnedEarly bool
	select {
	case tooEarly = <-second:
		returnedEarly = true
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-committed; err != nil {
		t.Fatalf("first transaction: %v", err)
	}
	if returnedEarly {
		t.Fatalf("the lock returned (err=%v) while a key-share holder was still open", tooEarly)
	}
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("the lock after the first commit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the lock did not complete within 2s of the first commit")
	}
}
