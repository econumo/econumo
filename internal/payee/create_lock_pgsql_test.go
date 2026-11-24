//go:build enginecompare

package payee_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/model"
	apppayee "github.com/econumo/econumo/internal/payee"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// saveHookRepo fires hook once, right before the first Save: inside
// CreatePayee's transaction, after its exact-name check.
type saveHookRepo struct {
	apppayee.Repository
	hook func()
}

func (r *saveHookRepo) Save(ctx context.Context, p *model.Payee) error {
	if h := r.hook; h != nil {
		r.hook = nil
		h()
	}
	return r.Repository.Save(ctx, p)
}

func newPayeeService(db *dbtest.DB, repo apppayee.Repository) *apppayee.Service {
	access := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, db.TX))
	return apppayee.NewService(repo, db.TX, operationrepo.NewGuard(db.Engine, db.TX), clock.New(), payeerepo.NewReadRepo(db.Engine, db.TX), access)
}

// Two creates of one name for one owner (the AI import job and the SPA's Save)
// must leave one payee: payees has no unique constraint, so the check and the
// insert are serialized per owner. Only observable across two connections, so
// a second pool runs the competing create while the first holds its checked,
// not yet inserted, state.
func TestCreatePayee_ConcurrentSameNameLeavesOnePayee(t *testing.T) {
	db := dbtest.New(t)
	if db.Engine != "postgresql" {
		t.Skipf("the race needs two real connections; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
	}
	ctx := context.Background()
	owner := vo.NewId()
	fixture.New(t, db).User(fixture.User{ID: owner.String(), Name: "Owner"})

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
	db2 := &dbtest.DB{Raw: raw2, TX: backend.NewTxManager(raw2), Engine: db.Engine}
	svc2 := newPayeeService(db2, payeerepo.NewRepo(db2.Engine, db2.TX))

	repo := &saveHookRepo{Repository: payeerepo.NewRepo(db.Engine, db.TX)}
	svc := newPayeeService(db, repo)

	second := make(chan error, 1)
	repo.hook = func() {
		go func() {
			_, err := svc2.CreatePayee(ctx, owner, model.CreatePayeeRequest{Id: vo.NewId().String(), Name: "Whole Foods"})
			second <- err
		}()
		// Give the competing create its chance to commit inside the window.
		// Under the lock it waits instead; the verdict is the stored rows.
		select {
		case err := <-second:
			second <- err
		case <-time.After(2 * time.Second):
		}
	}
	if _, err := svc.CreatePayee(ctx, owner, model.CreatePayeeRequest{Id: vo.NewId().String(), Name: "Whole Foods"}); err != nil {
		t.Fatalf("first CreatePayee: %v", err)
	}
	var secondErr error
	select {
	case secondErr = <-second:
	case <-time.After(10 * time.Second):
		t.Fatal("the second create never completed")
	}

	var n int
	if err := db.Raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM payees WHERE user_id = $1 AND name = $2`, owner.String(), "Whole Foods").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("payees named Whole Foods = %d, want 1", n)
	}
	if v, ok := errs.AsValidation(secondErr); !ok || v.MsgCode != errs.CodePayeeAlreadyExists {
		t.Fatalf("second create err = %v, want payee already exists", secondErr)
	}
}
