package payee_test

import (
	"context"
	"errors"
	"testing"

	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	apppayee "github.com/econumo/econumo/internal/payee"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// fakeDebtUsage records the calls and, at Reassign time, whether the source
// still exists, so the test pins that the debt moves before the source goes.
type fakeDebtUsage struct {
	used           bool
	reassignErr    error
	reassigned     [][2]vo.Id
	sourceExistsAt func(ctx context.Context) bool
	sourceExisted  bool
}

func (f *fakeDebtUsage) InUse(context.Context, vo.Id) (bool, error) { return f.used, nil }

func (f *fakeDebtUsage) Reassign(ctx context.Context, oldID, newID vo.Id) error {
	f.reassigned = append(f.reassigned, [2]vo.Id{oldID, newID})
	if f.sourceExistsAt != nil {
		f.sourceExisted = f.sourceExistsAt(ctx)
	}
	return f.reassignErr
}

func newGuardPayeeSvc(t *testing.T, db *dbtest.DB) (*apppayee.Service, *fixture.Builder, string) {
	t.Helper()
	f := fixture.New(t, db)
	owner := f.User(fixture.User{Name: "Owner"})
	repo := payeerepo.NewRepo(db.Engine, db.TX)
	readRepo := payeerepo.NewReadRepo(db.Engine, db.TX)
	access := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, db.TX))
	svc := apppayee.NewService(repo, db.TX, operationrepo.NewGuard(db.Engine, db.TX), clock.New(), readRepo, access)
	return svc, f, owner
}

func payeeExistsIn(t *testing.T, db *dbtest.DB, ctx context.Context, id string) bool {
	t.Helper()
	var n int
	if err := db.TX.Querier(ctx).QueryRowContext(ctx, db.Rebind("SELECT COUNT(*) FROM payees WHERE id = ?"), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func payeeExists(t *testing.T, db *dbtest.DB, id string) bool {
	t.Helper()
	var n int
	if err := db.Raw.QueryRow(db.Rebind("SELECT COUNT(*) FROM payees WHERE id = ?"), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestDeletePayee_RefusedWhileDebtUsesIt(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardPayeeSvc(t, db)
	id := f.Payee(fixture.Payee{UserID: owner})
	svc.SetDebtUsage(&fakeDebtUsage{used: true})

	_, err := svc.DeletePayee(context.Background(), vo.MustParseId(owner), model.DeletePayeeRequest{Id: id})
	var verr *errs.ValidationError
	if !errors.As(err, &verr) || verr.MsgCode != errs.CodePayeeUsedByDebt {
		t.Fatalf("want used_by_debt refusal, got %v", err)
	}
	if !payeeExists(t, db, id) {
		t.Error("payee was deleted despite the refusal")
	}
}

func TestDeletePayee_AllowedWhenNoDebtUsesIt(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardPayeeSvc(t, db)
	id := f.Payee(fixture.Payee{UserID: owner})
	svc.SetDebtUsage(&fakeDebtUsage{})

	if _, err := svc.DeletePayee(context.Background(), vo.MustParseId(owner), model.DeletePayeeRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	if payeeExists(t, db, id) {
		t.Error("payee still exists")
	}
}

func TestMergePayee_DebtFollowsTarget(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardPayeeSvc(t, db)
	src := f.Payee(fixture.Payee{UserID: owner})
	dst := f.Payee(fixture.Payee{UserID: owner})
	debts := &fakeDebtUsage{sourceExistsAt: func(ctx context.Context) bool { return payeeExistsIn(t, db, ctx, src) }}
	svc.SetDebtUsage(debts)

	if _, err := svc.MergePayee(context.Background(), vo.MustParseId(owner), model.MergePayeeRequest{SourceId: src, TargetId: dst}); err != nil {
		t.Fatal(err)
	}
	if len(debts.reassigned) != 1 || debts.reassigned[0] != [2]vo.Id{vo.MustParseId(src), vo.MustParseId(dst)} {
		t.Fatalf("Reassign calls = %v", debts.reassigned)
	}
	if !debts.sourceExisted {
		t.Error("Reassign ran after the source was deleted")
	}
	if payeeExists(t, db, src) {
		t.Error("source survived the merge")
	}
}

func TestMergePayee_RefusedOnDebtKeyConflict(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardPayeeSvc(t, db)
	src := f.Payee(fixture.Payee{UserID: owner})
	dst := f.Payee(fixture.Payee{UserID: owner})
	conflict := &errs.ValidationError{Msg: "Another debt could match the same payments.", MsgCode: errs.CodeDebtKeyConflict}
	svc.SetDebtUsage(&fakeDebtUsage{reassignErr: conflict})

	_, err := svc.MergePayee(context.Background(), vo.MustParseId(owner), model.MergePayeeRequest{SourceId: src, TargetId: dst})
	if !errors.Is(err, conflict) {
		t.Fatalf("want the key conflict, got %v", err)
	}
	if !payeeExists(t, db, src) {
		t.Error("source was deleted although the merge was refused")
	}
}
