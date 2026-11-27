package user_test

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// orderBudgets records HasAccess in the shared call log and fails the test when
// the check runs outside the use case's transaction: on SQLite (one connection)
// a check on the outer context would wait for the connection the transaction
// holds, forever.
type orderBudgets struct {
	appuser.BudgetAccess
	t     *testing.T
	db    *dbtest.DB
	outer context.Context
	log   *callLog
}

func (b *orderBudgets) HasAccess(ctx context.Context, userID vo.Id, budgetID string) (bool, error) {
	b.log.add("HasAccess")
	if ctx == b.outer {
		b.t.Error("HasAccess received the caller's context, not the transaction's")
	}
	if _, ok := b.db.TX.Querier(ctx).(*sql.Tx); !ok {
		b.t.Error("HasAccess ran outside a transaction")
	}
	return b.BudgetAccess.HasAccess(ctx, userID, budgetID)
}

func newUpdateBudgetEnv(t *testing.T, db *dbtest.DB, outer context.Context, log *callLog) (*appuser.Service, vo.Id) {
	t.Helper()
	repo := &orderRepo{Repository: userrepo.NewRepo(db.Engine, db.TX), log: log}
	budgets := &orderBudgets{BudgetAccess: server.NewUserBudgetAccess(db.Engine, db.TX), t: t, db: db, outer: outer, log: log}
	svc := appuser.NewService(repo, db.TX, auth.NewEncodeService(""), auth.NewPasswordHasher(),
		userrepo.NewAccessTokenRepo(db.Engine, db.TX),
		server.NewUserCurrencyLookup(currencyrepo.New(db.Engine, db.TX)), budgets,
		userrepo.NewPasswordRequestRepo(db.Engine, db.TX), nil,
		userrepo.NewEmailVerificationRepo(db.Engine, db.TX), nil,
		userrepo.NewEmailChangeRequestRepo(db.Engine, db.TX), nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), nil, false, 0, false)
	uid, err := svc.AdminCreateUser(context.Background(), "Budget Picker", "update-budget@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	return svc, uid
}

// The access check runs inside the locked update, so a revoke (or a wipe) that
// clears the pointer under the same row lock cannot be followed by a save of a
// budget id that passed a check made before it.
func TestUpdateBudget_ChecksAccessUnderTheRowLock(t *testing.T) {
	db := dbtest.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	log := &callLog{}
	svc, uid := newUpdateBudgetEnv(t, db, ctx, log)
	budgetID := fixture.New(t, db).Budget(fixture.Budget{UserID: uid.String()})
	log.reset()

	if _, err := svc.UpdateBudget(ctx, uid, model.UpdateActiveBudgetRequest{Value: budgetID}); err != nil {
		t.Fatalf("UpdateBudget: %v", err)
	}
	if want := []string{"LockRow", "HasAccess", "GetByID", "Save"}; !slices.Equal(log.calls, want) {
		t.Fatalf("calls = %v, want %v", log.calls, want)
	}
	u, err := userrepo.NewRepo(db.Engine, db.TX).GetByID(context.Background(), uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if o := u.Option(model.OptionBudget); o == nil || o.Value == nil || *o.Value != budgetID {
		t.Errorf("budget option = %+v, want %s", o, budgetID)
	}
}

func TestUpdateBudget_RefusesABudgetWithoutAccess(t *testing.T) {
	db := dbtest.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	log := &callLog{}
	svc, uid := newUpdateBudgetEnv(t, db, ctx, log)
	f := fixture.New(t, db)
	stranger := f.User(fixture.User{Email: "stranger@econumo.test", Name: "Stranger"})
	foreign := f.Budget(fixture.Budget{UserID: stranger})
	log.reset()

	_, err := svc.UpdateBudget(ctx, uid, model.UpdateActiveBudgetRequest{Value: foreign})
	nf, ok := errs.AsNotFound(err)
	if !ok || nf.Error() != "Plan not found" {
		t.Fatalf("err = %v, want the \"Plan not found\" not-found error", err)
	}
	if slices.Contains(log.calls, "Save") {
		t.Errorf("a refused selection saved the user: calls = %v", log.calls)
	}
}
