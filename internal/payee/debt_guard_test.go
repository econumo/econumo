package payee_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	apppayee "github.com/econumo/econumo/internal/payee"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// fakeDebtUsage logs the debt calls and, at Release/Reassign time, whether
// the payee still exists, so the tests pin the order the delete and the merge
// take.
type fakeDebtUsage struct {
	calls   []string
	exists  func(ctx context.Context) bool
	existed []bool
	err     error
}

func joinIDs(ids ...vo.Id) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = id.String()
	}
	return strings.Join(s, ",")
}

func (f *fakeDebtUsage) Prepare(_ context.Context, ids ...vo.Id) error {
	f.calls = append(f.calls, "prepare "+joinIDs(ids...))
	return nil
}

func (f *fakeDebtUsage) Release(ctx context.Context, id vo.Id) error {
	f.calls = append(f.calls, "release "+id.String())
	f.existed = append(f.existed, f.exists(ctx))
	return f.err
}

func (f *fakeDebtUsage) Reassign(ctx context.Context, oldID, newID vo.Id) error {
	f.calls = append(f.calls, "reassign "+joinIDs(oldID, newID))
	f.existed = append(f.existed, f.exists(ctx))
	return f.err
}

func newGuardPayeeSvc(t *testing.T, db *dbtest.DB, debts *fakeDebtUsage) (*apppayee.Service, *fixture.Builder, string) {
	t.Helper()
	f := fixture.New(t, db)
	owner := f.User(fixture.User{Name: "Owner"})
	repo := payeerepo.NewRepo(db.Engine, db.TX)
	readRepo := payeerepo.NewReadRepo(db.Engine, db.TX)
	access := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, db.TX))
	svc := apppayee.NewService(repo, db.TX, operationrepo.NewGuard(db.Engine, db.TX), clock.New(), readRepo, access)
	svc.SetDebtUsage(debts)
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

func TestDeletePayee_ReleasesDebtRules(t *testing.T) {
	db := dbtest.New(t)
	debts := &fakeDebtUsage{}
	svc, f, owner := newGuardPayeeSvc(t, db, debts)
	id := f.Payee(fixture.Payee{UserID: owner})
	debts.exists = func(ctx context.Context) bool { return payeeExistsIn(t, db, ctx, id) }

	if _, err := svc.DeletePayee(context.Background(), vo.MustParseId(owner), model.DeletePayeeRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	// the budgets are locked before anything else, the rules released while
	// the payee row still exists (debts_rules has a NO ACTION foreign key)
	if want := []string{"prepare " + id, "release " + id}; !slices.Equal(debts.calls, want) {
		t.Fatalf("debt calls = %v, want %v", debts.calls, want)
	}
	if !slices.Equal(debts.existed, []bool{true}) {
		t.Error("Release ran after the payee was deleted")
	}
	if payeeExistsIn(t, db, context.Background(), id) {
		t.Error("payee still exists")
	}
}

func TestDeletePayee_ForeignPayeeTouchesNoDebt(t *testing.T) {
	db := dbtest.New(t)
	debts := &fakeDebtUsage{}
	svc, f, owner := newGuardPayeeSvc(t, db, debts)
	stranger := f.User(fixture.User{Name: "Stranger", Email: "stranger@example.test"})
	id := f.Payee(fixture.Payee{UserID: stranger})

	if _, err := svc.DeletePayee(context.Background(), vo.MustParseId(owner), model.DeletePayeeRequest{Id: id}); err == nil {
		t.Fatal("deleting another user's payee succeeded")
	}
	if len(debts.calls) != 0 {
		t.Errorf("debt calls = %v, want none before the ownership check passes", debts.calls)
	}
}

func TestDeletePayee_DebtErrorRollsBack(t *testing.T) {
	db := dbtest.New(t)
	boom := errors.New("boom")
	debts := &fakeDebtUsage{err: boom}
	svc, f, owner := newGuardPayeeSvc(t, db, debts)
	id := f.Payee(fixture.Payee{UserID: owner})
	debts.exists = func(context.Context) bool { return true }

	if _, err := svc.DeletePayee(context.Background(), vo.MustParseId(owner), model.DeletePayeeRequest{Id: id}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if !payeeExistsIn(t, db, context.Background(), id) {
		t.Error("payee was deleted although releasing its rules failed")
	}
}

func TestMergePayee_ReassignsDebtRules(t *testing.T) {
	db := dbtest.New(t)
	debts := &fakeDebtUsage{}
	svc, f, owner := newGuardPayeeSvc(t, db, debts)
	src := f.Payee(fixture.Payee{UserID: owner})
	dst := f.Payee(fixture.Payee{UserID: owner})
	debts.exists = func(ctx context.Context) bool { return payeeExistsIn(t, db, ctx, src) }

	if _, err := svc.MergePayee(context.Background(), vo.MustParseId(owner), model.MergePayeeRequest{SourceId: src, TargetId: dst}); err != nil {
		t.Fatal(err)
	}
	want := []string{"prepare " + src + "," + dst, "reassign " + src + "," + dst}
	if !slices.Equal(debts.calls, want) {
		t.Fatalf("calls = %v, want %v", debts.calls, want)
	}
	if !slices.Equal(debts.existed, []bool{true}) {
		t.Error("Reassign ran after the source was deleted")
	}
	if payeeExistsIn(t, db, context.Background(), src) {
		t.Error("source survived the merge")
	}
}

func TestMergePayee_DebtErrorRollsBack(t *testing.T) {
	db := dbtest.New(t)
	boom := errors.New("boom")
	debts := &fakeDebtUsage{err: boom}
	svc, f, owner := newGuardPayeeSvc(t, db, debts)
	src := f.Payee(fixture.Payee{UserID: owner})
	dst := f.Payee(fixture.Payee{UserID: owner})
	debts.exists = func(context.Context) bool { return true }

	if _, err := svc.MergePayee(context.Background(), vo.MustParseId(owner), model.MergePayeeRequest{SourceId: src, TargetId: dst}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if !payeeExistsIn(t, db, context.Background(), src) {
		t.Error("source was deleted although the merge failed")
	}
}
