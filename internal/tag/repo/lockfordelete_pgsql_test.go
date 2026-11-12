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
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// A debt write adding a rule key-share-locks the tag row inside its budget
// lock; a delete's LockForDelete must wait for it to commit, so the delete
// then releases the new rule instead of failing the debts_rules foreign key.
// Every other suite pins its database to one connection, so this one opens a
// second pool against the same schema.
func TestTagLockForDelete_WaitsForAKeyShareHolder(t *testing.T) {
	db := dbtest.New(t)
	if db.Engine != "postgresql" {
		t.Skipf("the row lock is only observable on a real pool; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
	}
	ctx := context.Background()
	f := fixture.New(t, db)
	id := f.Tag(fixture.Tag{UserID: f.User(fixture.User{Name: "Owner"})})

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
	repo2 := tagrepo.NewRepo(db.Engine, tx2)

	locked := make(chan struct{})
	release := make(chan struct{})
	committed := make(chan error, 1)
	go func() {
		committed <- db.TX.WithTx(ctx, func(ctx context.Context) error {
			if _, err := db.TX.Querier(ctx).ExecContext(ctx, "SELECT id FROM tags WHERE id = $1 FOR KEY SHARE", id); err != nil {
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
		second <- tx2.WithTx(ctx, func(ctx context.Context) error { return repo2.LockForDelete(ctx, vo.MustParseId(id)) })
	}()

	// Never t.Fatal while tx1 is parked: the cleanup DROP SCHEMA would wait on
	// its lock. Record the verdict, release, assert after.
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
		t.Fatalf("LockForDelete returned (err=%v) while a key-share holder was still open", tooEarly)
	}
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("LockForDelete after the first commit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("LockForDelete did not complete within 2s of the first commit")
	}
}
