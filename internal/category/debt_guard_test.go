package category_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	appcategory "github.com/econumo/econumo/internal/category"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// fakeDebtUsage logs the debt calls (with the merge's budget-element step in
// the same log) and, at Release/Reassign time, whether the category still
// exists, so the tests pin the order the delete and the merge take.
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

type loggingMerger struct{ debts *fakeDebtUsage }

func (m loggingMerger) MergeElements(context.Context, vo.Id, vo.Id) error {
	m.debts.calls = append(m.debts.calls, "merge-elements")
	return nil
}

func newGuardCategorySvc(t *testing.T, db *dbtest.DB, debts *fakeDebtUsage) (*appcategory.Service, *fixture.Builder, string) {
	t.Helper()
	f := fixture.New(t, db)
	owner := f.User(fixture.User{Name: "Owner"})
	repo := categoryrepo.NewRepo(db.Engine, db.TX)
	readRepo := categoryrepo.NewReadRepo(db.Engine, db.TX)
	access := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, db.TX))
	svc := appcategory.NewService(repo, db.TX, operationrepo.NewGuard(db.Engine, db.TX), clock.New(), readRepo, access, loggingMerger{debts: debts})
	svc.SetDebtUsage(debts)
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

func TestDeleteCategory_ReleasesDebtRules(t *testing.T) {
	db := dbtest.New(t)
	debts := &fakeDebtUsage{}
	svc, f, owner := newGuardCategorySvc(t, db, debts)
	id := f.Category(fixture.Category{UserID: owner})
	debts.exists = func(ctx context.Context) bool { return categoryExistsIn(t, db, ctx, id) }

	if _, err := svc.DeleteCategory(context.Background(), vo.MustParseId(owner), model.DeleteCategoryRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	// the budgets are locked before anything else, the rules released while
	// the category row still exists (debts_rules has a NO ACTION foreign key)
	if want := []string{"prepare " + id, "release " + id}; !slices.Equal(debts.calls, want) {
		t.Fatalf("debt calls = %v, want %v", debts.calls, want)
	}
	if !slices.Equal(debts.existed, []bool{true}) {
		t.Error("Release ran after the category was deleted")
	}
	if categoryExistsIn(t, db, context.Background(), id) {
		t.Error("category still exists")
	}
}

func TestDeleteCategory_ForeignCategoryTouchesNoDebt(t *testing.T) {
	db := dbtest.New(t)
	debts := &fakeDebtUsage{}
	svc, f, owner := newGuardCategorySvc(t, db, debts)
	stranger := f.User(fixture.User{Name: "Stranger", Email: "stranger@example.test"})
	id := f.Category(fixture.Category{UserID: stranger})

	if _, err := svc.DeleteCategory(context.Background(), vo.MustParseId(owner), model.DeleteCategoryRequest{Id: id}); err == nil {
		t.Fatal("deleting another user's category succeeded")
	}
	if len(debts.calls) != 0 {
		t.Errorf("debt calls = %v, want none before the ownership check passes", debts.calls)
	}
}

func TestDeleteCategory_DebtErrorRollsBack(t *testing.T) {
	db := dbtest.New(t)
	boom := errors.New("boom")
	debts := &fakeDebtUsage{err: boom}
	svc, f, owner := newGuardCategorySvc(t, db, debts)
	id := f.Category(fixture.Category{UserID: owner})
	debts.exists = func(context.Context) bool { return true }

	if _, err := svc.DeleteCategory(context.Background(), vo.MustParseId(owner), model.DeleteCategoryRequest{Id: id}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if !categoryExistsIn(t, db, context.Background(), id) {
		t.Error("category was deleted although releasing its rules failed")
	}
}

func TestMergeCategory_PreparesBothSidesFirst(t *testing.T) {
	db := dbtest.New(t)
	debts := &fakeDebtUsage{}
	svc, f, owner := newGuardCategorySvc(t, db, debts)
	src := f.Category(fixture.Category{UserID: owner})
	dst := f.Category(fixture.Category{UserID: owner})
	debts.exists = func(ctx context.Context) bool { return categoryExistsIn(t, db, ctx, src) }

	if _, err := svc.MergeCategory(context.Background(), vo.MustParseId(owner), model.MergeCategoryRequest{SourceId: src, TargetId: dst}); err != nil {
		t.Fatal(err)
	}
	// the debts' budget rows are locked before the merge touches a budget
	// element row: budget rows, then element rows, the order every debt write
	// takes
	want := []string{"prepare " + src + "," + dst, "merge-elements", "reassign " + src + "," + dst}
	if !slices.Equal(debts.calls, want) {
		t.Fatalf("calls = %v, want %v", debts.calls, want)
	}
	if !slices.Equal(debts.existed, []bool{true}) {
		t.Error("Reassign ran after the source was deleted")
	}
	if categoryExistsIn(t, db, context.Background(), src) {
		t.Error("source survived the merge")
	}
}

func TestMergeCategory_DebtErrorRollsBack(t *testing.T) {
	db := dbtest.New(t)
	boom := errors.New("boom")
	debts := &fakeDebtUsage{err: boom}
	svc, f, owner := newGuardCategorySvc(t, db, debts)
	src := f.Category(fixture.Category{UserID: owner})
	dst := f.Category(fixture.Category{UserID: owner})
	debts.exists = func(context.Context) bool { return true }

	if _, err := svc.MergeCategory(context.Background(), vo.MustParseId(owner), model.MergeCategoryRequest{SourceId: src, TargetId: dst}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if !categoryExistsIn(t, db, context.Background(), src) {
		t.Error("source was deleted although the merge failed")
	}
}
