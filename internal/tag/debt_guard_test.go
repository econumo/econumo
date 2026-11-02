package tag_test

import (
	"context"
	"errors"
	"testing"

	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	apptag "github.com/econumo/econumo/internal/tag"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/wiring"
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

func newGuardTagSvc(t *testing.T, db *dbtest.DB) (*apptag.Service, *fixture.Builder, string) {
	t.Helper()
	f := fixture.New(t, db)
	owner := f.User(fixture.User{Name: "Owner"})
	repo := tagrepo.NewRepo(db.Engine, db.TX)
	readRepo := tagrepo.NewReadRepo(db.Engine, db.TX)
	access := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, db.TX))
	svc := apptag.NewService(repo, db.TX, operationrepo.NewGuard(db.Engine, db.TX), clock.New(), readRepo, access, wiring.BudgetMerger(db.Engine, db.TX, clock.New()))
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

func tagExists(t *testing.T, db *dbtest.DB, id string) bool {
	t.Helper()
	var n int
	if err := db.Raw.QueryRow(db.Rebind("SELECT COUNT(*) FROM tags WHERE id = ?"), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestDeleteTag_RefusedWhileDebtUsesIt(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardTagSvc(t, db)
	id := f.Tag(fixture.Tag{UserID: owner})
	svc.SetDebtUsage(&fakeDebtUsage{used: true})

	_, err := svc.DeleteTag(context.Background(), vo.MustParseId(owner), model.DeleteTagRequest{Id: id})
	var verr *errs.ValidationError
	if !errors.As(err, &verr) || verr.MsgCode != errs.CodeTagUsedByDebt {
		t.Fatalf("want used_by_debt refusal, got %v", err)
	}
	if !tagExists(t, db, id) {
		t.Error("tag was deleted despite the refusal")
	}
}

func TestDeleteTag_AllowedWhenNoDebtUsesIt(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardTagSvc(t, db)
	id := f.Tag(fixture.Tag{UserID: owner})
	svc.SetDebtUsage(&fakeDebtUsage{})

	if _, err := svc.DeleteTag(context.Background(), vo.MustParseId(owner), model.DeleteTagRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	if tagExists(t, db, id) {
		t.Error("tag still exists")
	}
}

func TestMergeTag_DebtFollowsTarget(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardTagSvc(t, db)
	src := f.Tag(fixture.Tag{UserID: owner})
	dst := f.Tag(fixture.Tag{UserID: owner})
	debts := &fakeDebtUsage{sourceExistsAt: func(ctx context.Context) bool { return tagExistsIn(t, db, ctx, src) }}
	svc.SetDebtUsage(debts)

	if _, err := svc.MergeTag(context.Background(), vo.MustParseId(owner), model.MergeTagRequest{SourceId: src, TargetId: dst}); err != nil {
		t.Fatal(err)
	}
	if len(debts.reassigned) != 1 || debts.reassigned[0] != [2]vo.Id{vo.MustParseId(src), vo.MustParseId(dst)} {
		t.Fatalf("Reassign calls = %v", debts.reassigned)
	}
	if !debts.sourceExisted {
		t.Error("Reassign ran after the source was deleted")
	}
	if tagExists(t, db, src) {
		t.Error("source survived the merge")
	}
}

func TestMergeTag_RefusedOnDebtKeyConflict(t *testing.T) {
	db := dbtest.New(t)
	svc, f, owner := newGuardTagSvc(t, db)
	src := f.Tag(fixture.Tag{UserID: owner})
	dst := f.Tag(fixture.Tag{UserID: owner})
	conflict := &errs.ValidationError{Msg: "Another debt could match the same payments.", MsgCode: errs.CodeDebtKeyConflict}
	svc.SetDebtUsage(&fakeDebtUsage{reassignErr: conflict})

	_, err := svc.MergeTag(context.Background(), vo.MustParseId(owner), model.MergeTagRequest{SourceId: src, TargetId: dst})
	if !errors.Is(err, conflict) {
		t.Fatalf("want the key conflict, got %v", err)
	}
	if !tagExists(t, db, src) {
		t.Error("source was deleted although the merge was refused")
	}
}
