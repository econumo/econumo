package label_test

import (
	"context"
	"testing"

	applabel "github.com/econumo/econumo/internal/label"
	labelrepo "github.com/econumo/econumo/internal/label/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func mustCreateLabel(t *testing.T, ctx context.Context, svc *applabel.Service, owner vo.Id, name string) model.LabelResult {
	t.Helper()
	res, err := svc.CreateLabel(ctx, owner, model.CreateLabelRequest{Id: vo.NewId().String(), Name: name})
	if err != nil {
		t.Fatalf("CreateLabel(%q): %v", name, err)
	}
	return res.Item
}

func TestUpdateLabel_RenamesAndEnforcesUniqueness(t *testing.T) {
	db := dbtest.New(t)
	labelFixture(t, db)
	svc := newLabelSvc(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(labelOwnerID)

	a := mustCreateLabel(t, ctx, svc, owner, "Groceries")
	b := mustCreateLabel(t, ctx, svc, owner, "Utilities")

	res, err := svc.UpdateLabel(ctx, owner, model.UpdateLabelRequest{Id: a.Id, Name: "Food"})
	if err != nil {
		t.Fatalf("UpdateLabel: %v", err)
	}
	if res.Item.Name != "Food" {
		t.Errorf("Name = %q, want %q", res.Item.Name, "Food")
	}

	// Renaming b to a's original name is fine (a no longer holds it); renaming
	// b to its own current name is a no-op, not a self-collision.
	if _, err := svc.UpdateLabel(ctx, owner, model.UpdateLabelRequest{Id: b.Id, Name: "Utilities"}); err != nil {
		t.Fatalf("UpdateLabel (unchanged name): %v", err)
	}

	// Renaming b to a's NEW name is a real collision.
	_, err = svc.UpdateLabel(ctx, owner, model.UpdateLabelRequest{Id: b.Id, Name: "Food"})
	if err == nil {
		t.Fatal("want duplicate-name error, got nil")
	}
	if ve, ok := errs.AsValidation(err); !ok || ve.Msg != "Label already exists." {
		t.Errorf("want 'Label already exists.', got %v (%T)", err, err)
	}
}

func TestUpdateLabel_ForeignOwnedIsMaskedAsNotFound(t *testing.T) {
	db := dbtest.New(t)
	labelFixture(t, db)
	svc := newLabelSvc(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(labelOwnerID)
	other := vo.MustParseId(labelOtherID)

	a := mustCreateLabel(t, ctx, svc, owner, "Groceries")

	_, err := svc.UpdateLabel(ctx, other, model.UpdateLabelRequest{Id: a.Id, Name: "Hijacked"})
	if err == nil {
		t.Fatal("want not-found error for a foreign-owned label, got nil")
	}
	nf, ok := errs.AsNotFound(err)
	if !ok {
		t.Fatalf("want *NotFoundError (masked), got %v (%T)", err, err)
	}
	if nf.Msg != "Label not found" {
		t.Errorf("want %q, got %q", "Label not found", nf.Msg)
	}
}

func TestArchiveAndUnarchiveLabel(t *testing.T) {
	db := dbtest.New(t)
	labelFixture(t, db)
	svc := newLabelSvc(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(labelOwnerID)

	a := mustCreateLabel(t, ctx, svc, owner, "Groceries")
	if a.IsArchived != 0 {
		t.Fatalf("newly created label IsArchived = %d, want 0", a.IsArchived)
	}

	if _, err := svc.ArchiveLabel(ctx, owner, model.ArchiveLabelRequest{Id: a.Id}); err != nil {
		t.Fatalf("ArchiveLabel: %v", err)
	}
	list, err := svc.OrderLabelList(ctx, owner, model.OrderLabelListRequest{Changes: []model.PositionChange{{Id: a.Id, Position: 0}}})
	if err != nil {
		t.Fatalf("OrderLabelList: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].IsArchived != 1 {
		t.Fatalf("after archive, want IsArchived=1, got %+v", list.Items)
	}

	if _, err := svc.UnarchiveLabel(ctx, owner, model.UnarchiveLabelRequest{Id: a.Id}); err != nil {
		t.Fatalf("UnarchiveLabel: %v", err)
	}
	list, err = svc.OrderLabelList(ctx, owner, model.OrderLabelListRequest{Changes: []model.PositionChange{{Id: a.Id, Position: 0}}})
	if err != nil {
		t.Fatalf("OrderLabelList: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].IsArchived != 0 {
		t.Fatalf("after unarchive, want IsArchived=0, got %+v", list.Items)
	}
}

func TestArchiveLabel_ForeignOwnedIsMaskedAsNotFound(t *testing.T) {
	db := dbtest.New(t)
	labelFixture(t, db)
	svc := newLabelSvc(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(labelOwnerID)
	other := vo.MustParseId(labelOtherID)

	a := mustCreateLabel(t, ctx, svc, owner, "Groceries")

	_, err := svc.ArchiveLabel(ctx, other, model.ArchiveLabelRequest{Id: a.Id})
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want *NotFoundError (masked), got %v (%T)", err, err)
	}
}

func TestDeleteLabel_RemovesRowAndMasksForeignOwnership(t *testing.T) {
	db := dbtest.New(t)
	labelFixture(t, db)
	svc := newLabelSvc(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(labelOwnerID)
	other := vo.MustParseId(labelOtherID)

	a := mustCreateLabel(t, ctx, svc, owner, "Groceries")

	if _, err := svc.DeleteLabel(ctx, other, model.DeleteLabelRequest{Id: a.Id}); err == nil {
		t.Fatal("want not-found error for a foreign-owned delete, got nil")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want *NotFoundError (masked), got %v (%T)", err, err)
	}

	if _, err := svc.DeleteLabel(ctx, owner, model.DeleteLabelRequest{Id: a.Id}); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}

	readSvc := applabel.NewReadService(labelrepo.NewReadRepo(db.Engine, db.TX))
	res, err := readSvc.GetLabelList(ctx, owner)
	if err != nil {
		t.Fatalf("GetLabelList: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("after delete, want an empty label list, got %+v", res.Items)
	}
}

func TestOrderLabelList_OwnerOnlyWritesButReturnsSharedToo(t *testing.T) {
	db := dbtest.New(t)
	f := labelFixture(t, db)
	svc := newLabelSvc(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(labelOwnerID)
	other := vo.MustParseId(labelOtherID)

	a := mustCreateLabel(t, ctx, svc, owner, "Alpha")
	b := mustCreateLabel(t, ctx, svc, owner, "Beta")
	sharedLabel := mustCreateLabel(t, ctx, svc, other, "Shared")

	// Connect the two users via a shared account so labelOtherID's label is
	// visible in labelOwnerID's available (own + shared) list.
	acctID := f.Account(fixture.Account{UserID: labelOtherID, Name: "Other's account"})
	f.AccountAccess(acctID, labelOwnerID, int(model.RoleUser))

	// Reorder: swap a and b's positions. Also try to move the shared label -
	// that write must be silently ignored (owner-only writes).
	res, err := svc.OrderLabelList(ctx, owner, model.OrderLabelListRequest{Changes: []model.PositionChange{
		{Id: a.Id, Position: 1},
		{Id: b.Id, Position: 0},
		{Id: sharedLabel.Id, Position: 99},
	}})
	if err != nil {
		t.Fatalf("OrderLabelList: %v", err)
	}

	byID := map[string]model.LabelResult{}
	for _, item := range res.Items {
		byID[item.Id] = item
	}
	if len(byID) != 3 {
		t.Fatalf("want 3 available labels (own 2 + shared 1), got %d: %+v", len(byID), res.Items)
	}
	if byID[a.Id].Position != 1 {
		t.Errorf("a.Position = %d, want 1", byID[a.Id].Position)
	}
	if byID[b.Id].Position != 0 {
		t.Errorf("b.Position = %d, want 0", byID[b.Id].Position)
	}
	if byID[sharedLabel.Id].Position == 99 {
		t.Errorf("shared label's position was rewritten by a non-owner order call, want it untouched (not 99)")
	}
}
