package label_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
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

func labelExistsIn(t *testing.T, db *dbtest.DB, ctx context.Context, id string) bool {
	t.Helper()
	var n int
	if err := db.TX.Querier(ctx).QueryRowContext(ctx, db.Rebind("SELECT COUNT(*) FROM labels WHERE id = ?"), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestDeleteLabel_RefusedWhileDebtUsesIt(t *testing.T) {
	db := dbtest.New(t)
	f := labelFixture(t, db)
	svc := newLabelSvc(t, db)
	id := f.Label(fixture.Label{UserID: labelOwnerID})
	svc.SetDebtUsage(&fakeDebtUsage{used: true})

	_, err := svc.DeleteLabel(context.Background(), vo.MustParseId(labelOwnerID), model.DeleteLabelRequest{Id: id})
	var verr *errs.ValidationError
	if !errors.As(err, &verr) || verr.MsgCode != errs.CodeLabelUsedByDebt {
		t.Fatalf("want used_by_debt refusal, got %v", err)
	}
	if !labelExistsIn(t, db, context.Background(), id) {
		t.Error("label was deleted despite the refusal")
	}
}

func TestDeleteLabel_AllowedWhenNoDebtUsesIt(t *testing.T) {
	db := dbtest.New(t)
	f := labelFixture(t, db)
	svc := newLabelSvc(t, db)
	id := f.Label(fixture.Label{UserID: labelOwnerID})
	svc.SetDebtUsage(&fakeDebtUsage{})

	if _, err := svc.DeleteLabel(context.Background(), vo.MustParseId(labelOwnerID), model.DeleteLabelRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	if labelExistsIn(t, db, context.Background(), id) {
		t.Error("label still exists")
	}
}

func TestMergeLabel_DebtFollowsTarget(t *testing.T) {
	db := dbtest.New(t)
	f := labelFixture(t, db)
	svc := newLabelSvc(t, db)
	src := f.Label(fixture.Label{UserID: labelOwnerID, Name: "Car"})
	dst := f.Label(fixture.Label{UserID: labelOwnerID, Name: "Loan"})
	debts := &fakeDebtUsage{sourceExistsAt: func(ctx context.Context) bool { return labelExistsIn(t, db, ctx, src) }}
	svc.SetDebtUsage(debts)

	if _, err := svc.MergeLabel(context.Background(), vo.MustParseId(labelOwnerID), model.MergeLabelRequest{SourceId: src, TargetId: dst}); err != nil {
		t.Fatal(err)
	}
	if len(debts.reassigned) != 1 || debts.reassigned[0] != [2]vo.Id{vo.MustParseId(src), vo.MustParseId(dst)} {
		t.Fatalf("Reassign calls = %v", debts.reassigned)
	}
	if len(debts.calls) != 2 || debts.calls[0] != "prepare" || debts.calls[1] != "reassign" {
		t.Fatalf("debt calls = %v, want prepare then reassign", debts.calls)
	}
	if !debts.sourceExisted {
		t.Error("Reassign ran after the source was deleted")
	}
	if labelExistsIn(t, db, context.Background(), src) {
		t.Error("source survived the merge")
	}
}

func TestMergeLabel_RefusedOnDebtKeyConflict(t *testing.T) {
	db := dbtest.New(t)
	f := labelFixture(t, db)
	svc := newLabelSvc(t, db)
	src := f.Label(fixture.Label{UserID: labelOwnerID, Name: "Car"})
	dst := f.Label(fixture.Label{UserID: labelOwnerID, Name: "Loan"})
	conflict := &errs.ValidationError{Msg: "Another debt could match the same payments.", MsgCode: errs.CodeDebtKeyConflict}
	svc.SetDebtUsage(&fakeDebtUsage{reassignErr: conflict})

	_, err := svc.MergeLabel(context.Background(), vo.MustParseId(labelOwnerID), model.MergeLabelRequest{SourceId: src, TargetId: dst})
	if !errors.Is(err, conflict) {
		t.Fatalf("want the key conflict, got %v", err)
	}
	if !labelExistsIn(t, db, context.Background(), src) {
		t.Error("source was deleted although the merge was refused")
	}
}
