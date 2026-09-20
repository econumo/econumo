//go:build enginecompare

package repo_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// LockRow is the load-bearing half of the identity-unlink race guard, but every
// other suite pins its test database to a single connection, so a second
// transaction cannot even BEGIN until the first commits and the outcome is
// satisfied by WithTx alone — deleting the lock would not fail those tests.
// This one opens a SECOND pool against the same Postgres schema and asserts the
// lock itself: while tx1 holds the user row, tx2's LockRow must not return.
func TestLockRow_BlocksASecondConnectionUntilCommit(t *testing.T) {
	db := dbtest.New(t)
	if db.Engine != "postgresql" {
		t.Skipf("the row lock is only observable on a real pool; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
	}
	ctx := context.Background()

	repo := userrepo.NewRepo(db.Engine, db.TX)
	u := newTestUser(vo.NewId(), "lock@example.test", "Lock", "face:red", "h", "s", true, fixedTime, fixedTime, nil)
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// dbtest gives each test a private schema on the pinned session; read it back
	// rather than re-deriving the name, and point the second pool at the same one.
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
	repo2 := userrepo.NewRepo(db.Engine, tx2)

	locked := make(chan struct{})
	release := make(chan struct{})
	committed := make(chan error, 1)
	go func() {
		committed <- db.TX.WithTx(ctx, func(ctx context.Context) error {
			if err := repo.LockRow(ctx, u.ID); err != nil {
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
		second <- tx2.WithTx(ctx, func(ctx context.Context) error { return repo2.LockRow(ctx, u.ID) })
	}()

	// Never t.Fatal while tx1 is parked: the cleanup DROP SCHEMA would then wait
	// on the lock it still holds. Record the verdict, release, assert after.
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
		t.Fatalf("the second connection's LockRow returned (err=%v) while the first transaction still held the row: the row lock is not taken", tooEarly)
	}

	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("second LockRow after the first commit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the second connection's LockRow did not complete within 2s of the first commit")
	}
}
