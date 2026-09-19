//go:build enginecompare

package user_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// hookedRepo fires hook once, right after the first GetByID returns — i.e.
// inside mutate's transaction, in the window between its read of the aggregate
// and the Save that writes the whole aggregate back.
type hookedRepo struct {
	appuser.Repository
	hook func()
}

func (r *hookedRepo) GetByID(ctx context.Context, id vo.Id) (*model.User, error) {
	u, err := r.Repository.GetByID(ctx, id)
	if err != nil || r.hook == nil {
		return u, err
	}
	h := r.hook
	r.hook = nil
	h()
	return u, nil
}

// The stale-aggregate race is only observable across two connections: every
// other suite pins its test database to one, where mutate's transaction and the
// reset it races run on the same connection and neither the lock nor its
// absence changes the interleaving. So this opens a SECOND pool against the
// same Postgres schema and runs the real ResetPassword on it while mutate holds
// its (pre-lock) read: without the row lock the reset commits inside that
// window and mutate's whole-aggregate Save puts the pre-reset password back,
// handing the evicted session the account again.
func TestUpdateName_ResetBetweenTheReadAndTheSaveDoesNotRestoreTheOldPassword(t *testing.T) {
	db := dbtest.New(t)
	if db.Engine != "postgresql" {
		t.Skipf("the race needs two real connections; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
	}
	ctx := context.Background()

	plain := userrepo.NewRepo(db.Engine, db.TX)
	repo := &hookedRepo{Repository: userrepo.NewRepo(db.Engine, db.TX)}
	svc, _, _ := newUserSvcWithRepo(t, db, repo)

	const email = "mutate-race@econumo.test"
	uid, err := svc.AdminCreateUser(ctx, "Owner", email, "owner-old-password")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	before, err := plain.GetByID(ctx, uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	// The second pool: same schema, its own connection, so its transaction can
	// be in flight while the first one holds its read.
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
	svc2, _, _ := newUserSvcWithRepo(t, db2, userrepo.NewRepo(db2.Engine, db2.TX))

	// The owner's reset code, as the remind flow would have left it.
	const code = "482913"
	pr := model.NewPasswordRequest(vo.NewId(), uid, appuser.HashResetCode(code), time.Now().UTC())
	if err := userrepo.NewPasswordRequestRepo(db2.Engine, db2.TX).Save(ctx, pr); err != nil {
		t.Fatalf("seed password request: %v", err)
	}

	reset := make(chan error, 1)
	repo.hook = func() {
		go func() {
			_, rerr := svc2.ResetPassword(ctx, model.ResetPasswordRequest{
				Username: email, Code: code, Password: "owner-new-password",
			})
			reset <- rerr
		}()
		// Give the reset its chance to commit inside the window. Under the row
		// lock it blocks here instead and finishes after mutate commits; the
		// verdict is the stored password either way, never the timing.
		select {
		case rerr := <-reset:
			reset <- rerr // put it back for the wait below
		case <-time.After(2 * time.Second):
		}
	}

	if _, err := svc.UpdateName(ctx, uid, model.UpdateNameRequest{Name: "Mallory"}); err != nil {
		t.Fatalf("UpdateName: %v", err)
	}
	select {
	case rerr := <-reset:
		if rerr != nil {
			t.Fatalf("ResetPassword on the second connection: %v", rerr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the reset never completed")
	}

	after, err := plain.GetByID(ctx, uid)
	if err != nil {
		t.Fatalf("GetByID after the race: %v", err)
	}
	hasher := auth.NewPasswordHasher()
	if after.Password == before.Password || !hasher.Verify(after.Algorithm, after.Password, "owner-new-password", after.Salt) {
		t.Fatal("a stale aggregate save restored the pre-reset password")
	}
	if after.Name != "Mallory" {
		t.Errorf("name = %q, want the mutate's own write to have landed", after.Name)
	}
}
