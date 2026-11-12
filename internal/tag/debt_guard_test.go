package tag_test

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
	"github.com/econumo/econumo/internal/shared/vo"
	apptag "github.com/econumo/econumo/internal/tag"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// fakeDebtUsage logs the debt calls (with the merge's budget-element step in
// the same log) and, at Release/Reassign time, whether the tag still
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

func newGuardTagSvc(t *testing.T, db *dbtest.DB, debts *fakeDebtUsage) (*apptag.Service, *fixture.Builder, string) {
	t.Helper()
	f := fixture.New(t, db)
	owner := f.User(fixture.User{Name: "Owner"})
	repo := tagrepo.NewRepo(db.Engine, db.TX)
	readRepo := tagrepo.NewReadRepo(db.Engine, db.TX)
	access := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, db.TX))
	svc := apptag.NewService(repo, db.TX, operationrepo.NewGuard(db.Engine, db.TX), clock.New(), readRepo, access, loggingMerger{debts: debts})
	svc.SetDebtUsage(debts)
	return svc, f, owner
}

func tagExistsIn(t *testing.T, db *dbtest.DB, ctx context.Context, id string) bool {
	t.Helper()
	var n int
	if err := db.TX.Querier(ctx).QueryRowContext(ctx, db.Rebind("SELECT COUNT(*) FROM tags WHERE id = ?"), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestDeleteTag_ReleasesDebtRules(t *testing.T) {
	db := dbtest.New(t)
	debts := &fakeDebtUsage{}
	svc, f, owner := newGuardTagSvc(t, db, debts)
	id := f.Tag(fixture.Tag{UserID: owner})
	debts.exists = func(ctx context.Context) bool { return tagExistsIn(t, db, ctx, id) }

	if _, err := svc.DeleteTag(context.Background(), vo.MustParseId(owner), model.DeleteTagRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	// the budgets are locked before anything else, the rules released while
	// the tag row still exists (debts_rules has a NO ACTION foreign key)
	if want := []string{"prepare " + id, "release " + id}; !slices.Equal(debts.calls, want) {
		t.Fatalf("debt calls = %v, want %v", debts.calls, want)
	}
	if !slices.Equal(debts.existed, []bool{true}) {
		t.Error("Release ran after the tag was deleted")
	}
	if tagExistsIn(t, db, context.Background(), id) {
		t.Error("tag still exists")
	}
}

func TestDeleteTag_ForeignTagTouchesNoDebt(t *testing.T) {
	db := dbtest.New(t)
	debts := &fakeDebtUsage{}
	svc, f, owner := newGuardTagSvc(t, db, debts)
	stranger := f.User(fixture.User{Name: "Stranger", Email: "stranger@example.test"})
	id := f.Tag(fixture.Tag{UserID: stranger})

	if _, err := svc.DeleteTag(context.Background(), vo.MustParseId(owner), model.DeleteTagRequest{Id: id}); err == nil {
		t.Fatal("deleting another user's tag succeeded")
	}
	if len(debts.calls) != 0 {
		t.Errorf("debt calls = %v, want none before the ownership check passes", debts.calls)
	}
}

func TestDeleteTag_DebtErrorRollsBack(t *testing.T) {
	db := dbtest.New(t)
	boom := errors.New("boom")
	debts := &fakeDebtUsage{err: boom}
	svc, f, owner := newGuardTagSvc(t, db, debts)
	id := f.Tag(fixture.Tag{UserID: owner})
	debts.exists = func(context.Context) bool { return true }

	if _, err := svc.DeleteTag(context.Background(), vo.MustParseId(owner), model.DeleteTagRequest{Id: id}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if !tagExistsIn(t, db, context.Background(), id) {
		t.Error("tag was deleted although releasing its rules failed")
	}
}

func TestMergeTag_PreparesBothSidesFirst(t *testing.T) {
	db := dbtest.New(t)
	debts := &fakeDebtUsage{}
	svc, f, owner := newGuardTagSvc(t, db, debts)
	src := f.Tag(fixture.Tag{UserID: owner})
	dst := f.Tag(fixture.Tag{UserID: owner})
	debts.exists = func(ctx context.Context) bool { return tagExistsIn(t, db, ctx, src) }

	if _, err := svc.MergeTag(context.Background(), vo.MustParseId(owner), model.MergeTagRequest{SourceId: src, TargetId: dst}); err != nil {
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
	if tagExistsIn(t, db, context.Background(), src) {
		t.Error("source survived the merge")
	}
}

func TestMergeTag_DebtErrorRollsBack(t *testing.T) {
	db := dbtest.New(t)
	boom := errors.New("boom")
	debts := &fakeDebtUsage{err: boom}
	svc, f, owner := newGuardTagSvc(t, db, debts)
	src := f.Tag(fixture.Tag{UserID: owner})
	dst := f.Tag(fixture.Tag{UserID: owner})
	debts.exists = func(context.Context) bool { return true }

	if _, err := svc.MergeTag(context.Background(), vo.MustParseId(owner), model.MergeTagRequest{SourceId: src, TargetId: dst}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if !tagExistsIn(t, db, context.Background(), src) {
		t.Error("source was deleted although the merge failed")
	}
}
