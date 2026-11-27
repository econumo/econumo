//go:build enginecompare

package user_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// A profile update saves every option it read. If a budget revoke cleared the
// member's active-budget pointer between that read and that save, the save
// would put the old pointer back; ClearActiveBudget waits for the member's row
// lock instead, so it lands after the update and the pointer stays cleared.
func TestUpdateName_ClearActiveBudgetBetweenTheReadAndTheSaveStaysCleared(t *testing.T) {
	db := dbtest.New(t)
	if db.Engine != "postgresql" {
		t.Skipf("the race needs two real connections; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
	}
	ctx := context.Background()

	plain := userrepo.NewRepo(db.Engine, db.TX)
	repo := &hookedRepo{Repository: userrepo.NewRepo(db.Engine, db.TX)}
	svc, _, _ := newUserSvcWithRepo(t, db, repo)

	uid, err := svc.AdminCreateUser(ctx, "Member", "clear-budget-race@econumo.test", "member-password")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	budgetID := fixture.New(t, db).Budget(fixture.Budget{UserID: uid.String()})
	if _, err := svc.UpdateBudget(ctx, uid, model.UpdateActiveBudgetRequest{Value: budgetID}); err != nil {
		t.Fatalf("UpdateBudget: %v", err)
	}

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
	lookup2 := server.NewBudgetUserLookup(userrepo.NewRepo(db.Engine, tx2), clock.New())

	cleared := make(chan error, 1)
	repo.hook = func() {
		go func() {
			cleared <- tx2.WithTx(ctx, func(ctx context.Context) error {
				return lookup2.ClearActiveBudget(ctx, uid, vo.MustParseId(budgetID))
			})
		}()
		// Give the clear its chance to commit inside the window. Under the row
		// lock it waits here and finishes after the update commits; the verdict
		// is the stored option either way, never the timing.
		select {
		case cerr := <-cleared:
			cleared <- cerr
		case <-time.After(2 * time.Second):
		}
	}

	if _, err := svc.UpdateName(ctx, uid, model.UpdateNameRequest{Name: "Renamed"}); err != nil {
		t.Fatalf("UpdateName: %v", err)
	}
	select {
	case cerr := <-cleared:
		if cerr != nil {
			t.Fatalf("ClearActiveBudget on the second connection: %v", cerr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the clear never completed")
	}

	after, err := plain.GetByID(ctx, uid)
	if err != nil {
		t.Fatalf("GetByID after the race: %v", err)
	}
	if o := after.Option(model.OptionBudget); o == nil || o.Value != nil {
		t.Fatalf("budget option = %+v, want NULL: a stale save restored the old pointer", o)
	}
	if after.Name != "Renamed" {
		t.Errorf("name = %q, want the update's own write to have landed", after.Name)
	}
}
