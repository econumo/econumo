package category_test

import (
	"context"
	"errors"
	"testing"

	appcategory "github.com/econumo/econumo/internal/category"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/wiring"
)

// fakeDebtUsage records the calls and, at Reassign time, whether the source
// still exists, so the test pins that the debt moves before the source goes.
type fakeDebtUsage struct {
	used        bool
	reassignErr error
	reassigned  [][2]vo.Id
	// calls is the order of the merge's debt calls: "prepare", "reassign".
	calls          []string
	sourceExistsAt func(ctx context.Context) bool
	sourceExisted  bool
}

func (f *fakeDebtUsage) InUse(context.Context, vo.Id) (bool, error) { return f.used, nil }

func (f *fakeDebtUsage) PrepareMerge(context.Context, vo.Id, vo.Id) error {
	f.calls = append(f.calls, "prepare")
	return nil
}

func (f *fakeDebtUsage) Reassign(ctx context.Context, oldID, newID vo.Id) error {
	f.calls = append(f.calls, "reassign")
	f.reassigned = append(f.reassigned, [2]vo.Id{oldID, newID})
	if f.sourceExistsAt != nil {
		f.sourceExisted = f.sourceExistsAt(ctx)
	}
	return f.reassignErr
}

func newGuardCategorySvc(t *testing.T, db *dbtest.DB) (*appcategory.Service, *fixture.Builder, string) {
	t.Helper()
	f := fixture.New(t, db)
	owner := f.User(fixture.User{Name: "Owner"})
	repo := categoryrepo.NewRepo(db.Engine, db.TX)
	readRepo := categoryrepo.NewReadRepo(db.Engine, db.TX)
	access := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, db.TX))
	svc := appcategory.NewService(repo, db.TX, operationrepo.NewGuard(db.Engine, db.TX), clock.New(), readRepo, access, wiring.BudgetMerger(db.Engine, db.TX, clock.New()))
	return svc, f, owner
}

func categoryExistsIn(t *testing.T, db *dbtest.DB, ctx context.Context, id string) bool {
	t.Helper()
	var n int
	if err := db.TX.Querier(ctx).QueryRowContext(ctx, db.Rebind("SELECT COUNT(*) FROM categories WHERE id = ?"), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func categoryExists(t *testing.T, db *dbtest.DB, id string) bool {
	t.Helper()
	var n int
	if err := db.Raw.QueryRow(db.Rebind("SELECT COUNT(*) FROM categories WHERE id = ?"), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestDeleteCategory_RefusedWhileDebtUsesIt(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardCategorySvc(t, db)
	id := f.Category(fixture.Category{UserID: owner})
	svc.SetDebtUsage(&fakeDebtUsage{used: true})

	_, err := svc.DeleteCategory(context.Background(), vo.MustParseId(owner), model.DeleteCategoryRequest{Id: id})
	var verr *errs.ValidationError
	if !errors.As(err, &verr) || verr.MsgCode != errs.CodeCategoryUsedByDebt {
		t.Fatalf("want used_by_debt refusal, got %v", err)
	}
	if !categoryExists(t, db, id) {
		t.Error("category was deleted despite the refusal")
	}
}

func TestDeleteCategory_AllowedWhenNoDebtUsesIt(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardCategorySvc(t, db)
	id := f.Category(fixture.Category{UserID: owner})
	svc.SetDebtUsage(&fakeDebtUsage{})

	if _, err := svc.DeleteCategory(context.Background(), vo.MustParseId(owner), model.DeleteCategoryRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	if categoryExists(t, db, id) {
		t.Error("category still exists")
	}
}

func TestMergeCategory_DebtFollowsTarget(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardCategorySvc(t, db)
	src := f.Category(fixture.Category{UserID: owner})
	dst := f.Category(fixture.Category{UserID: owner})
	debts := &fakeDebtUsage{sourceExistsAt: func(ctx context.Context) bool { return categoryExistsIn(t, db, ctx, src) }}
	svc.SetDebtUsage(debts)

	if _, err := svc.MergeCategory(context.Background(), vo.MustParseId(owner), model.MergeCategoryRequest{SourceId: src, TargetId: dst}); err != nil {
		t.Fatal(err)
	}
	if len(debts.reassigned) != 1 || debts.reassigned[0] != [2]vo.Id{vo.MustParseId(src), vo.MustParseId(dst)} {
		t.Fatalf("Reassign calls = %v", debts.reassigned)
	}
	// the debt owners are locked before the merge writes any budget row:
	// owner row, then budget rows, the order every debt write takes
	if len(debts.calls) != 2 || debts.calls[0] != "prepare" || debts.calls[1] != "reassign" {
		t.Fatalf("debt calls = %v, want prepare then reassign", debts.calls)
	}
	if !debts.sourceExisted {
		t.Error("Reassign ran after the source was deleted")
	}
	if categoryExists(t, db, src) {
		t.Error("source survived the merge")
	}
}

func TestMergeCategory_RefusedOnDebtKeyConflict(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardCategorySvc(t, db)
	src := f.Category(fixture.Category{UserID: owner})
	dst := f.Category(fixture.Category{UserID: owner})
	conflict := &errs.ValidationError{Msg: "Another debt could match the same payments.", MsgCode: errs.CodeDebtKeyConflict}
	svc.SetDebtUsage(&fakeDebtUsage{reassignErr: conflict})

	_, err := svc.MergeCategory(context.Background(), vo.MustParseId(owner), model.MergeCategoryRequest{SourceId: src, TargetId: dst})
	if !errors.Is(err, conflict) {
		t.Fatalf("want the key conflict, got %v", err)
	}
	if !categoryExists(t, db, src) {
		t.Error("source was deleted although the merge was refused")
	}
}
