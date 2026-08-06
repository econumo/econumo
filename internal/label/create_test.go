package label_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	applabel "github.com/econumo/econumo/internal/label"
	labelrepo "github.com/econumo/econumo/internal/label/repo"

	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	labelOwnerID = "11111111-1111-1111-1111-111111111111"
	labelOtherID = "22222222-2222-2222-2222-222222222222"
)

// newLabelSvc wires a real label.Service over sqlite repos for the given db.
func newLabelSvc(t *testing.T, db *dbtest.DB) *applabel.Service {
	t.Helper()
	repo := labelrepo.NewRepo(db.Engine, db.TX)
	readRepo := labelrepo.NewReadRepo(db.Engine, db.TX)
	opGuard := operationrepo.NewGuard(db.Engine, db.TX)
	access := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, db.TX))
	return applabel.NewService(repo, db.TX, opGuard, clock.New(), readRepo, access)
}

// labelFixture seeds an owner and another user; returns the builder.
func labelFixture(t *testing.T, db *dbtest.DB) *fixture.Builder {
	t.Helper()
	f := fixture.New(t, db)
	f.User(fixture.User{ID: labelOwnerID, Name: "Owner"})
	f.User(fixture.User{ID: labelOtherID, Name: "Other"})
	return f
}

func TestCreateLabelIsIdempotentOnOperationID(t *testing.T) {
	db := dbtest.New(t)
	labelFixture(t, db)
	svc := newLabelSvc(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(labelOwnerID)

	opID := vo.NewId().String()

	if _, err := svc.CreateLabel(ctx, owner, model.CreateLabelRequest{Id: opID, Name: "Groceries"}); err != nil {
		t.Fatalf("first CreateLabel: %v", err)
	}

	// Same operation id, different name: must still be rejected as locked, not
	// as a duplicate-name error - the idempotency check runs first.
	_, err := svc.CreateLabel(ctx, owner, model.CreateLabelRequest{Id: opID, Name: "Different Name"})
	if err == nil {
		t.Fatal("want error for a repeated operation id, got nil")
	}
	ve, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("want *ValidationError, got %v (%T)", err, err)
	}
	if ve.Msg != "Operation is locked" || ve.MsgCode != errs.CodeOperationLocked {
		t.Errorf("want %q/%q, got %q/%q", "Operation is locked", errs.CodeOperationLocked, ve.Msg, ve.MsgCode)
	}
}

func TestCreateLabelRejectsDuplicateNamePerOwner(t *testing.T) {
	db := dbtest.New(t)
	labelFixture(t, db)
	svc := newLabelSvc(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(labelOwnerID)

	if _, err := svc.CreateLabel(ctx, owner, model.CreateLabelRequest{Id: vo.NewId().String(), Name: "Groceries"}); err != nil {
		t.Fatalf("first CreateLabel: %v", err)
	}

	_, err := svc.CreateLabel(ctx, owner, model.CreateLabelRequest{Id: vo.NewId().String(), Name: "Groceries"})
	if err == nil {
		t.Fatal("want error for a duplicate name, got nil")
	}
	ve, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("want *ValidationError, got %v (%T)", err, err)
	}
	if ve.Msg != "Label already exists." || ve.MsgCode != errs.CodeLabelAlreadyExists {
		t.Errorf("want %q/%q, got %q/%q", "Label already exists.", errs.CodeLabelAlreadyExists, ve.Msg, ve.MsgCode)
	}

	// A different owner may reuse the same name (per-owner uniqueness only).
	other := vo.MustParseId(labelOtherID)
	if _, err := svc.CreateLabel(ctx, other, model.CreateLabelRequest{Id: vo.NewId().String(), Name: "Groceries"}); err != nil {
		t.Fatalf("other owner's CreateLabel should not collide: %v", err)
	}
}

func TestCreateLabelForSharedAccountAssignsOwner(t *testing.T) {
	db := dbtest.New(t)
	f := labelFixture(t, db)
	acctID := f.Account(fixture.Account{UserID: labelOwnerID, Name: "Shared"})
	// labelOtherID holds an ACCEPTED admin grant on the owner's account.
	f.AccountAccess(acctID, labelOtherID, int(model.RoleAdmin))

	svc := newLabelSvc(t, db)
	ctx := context.Background()
	caller := vo.MustParseId(labelOtherID)

	res, err := svc.CreateLabel(ctx, caller, model.CreateLabelRequest{
		Id: vo.NewId().String(), Name: "Reimbursable", AccountId: &acctID,
	})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if res.Item.OwnerUserId != labelOwnerID {
		t.Errorf("OwnerUserId = %q, want the account owner %q", res.Item.OwnerUserId, labelOwnerID)
	}

	// A caller with no grant on the account is denied.
	strangerID := "33333333-3333-3333-3333-333333333333"
	f.User(fixture.User{ID: strangerID, Name: "Stranger"})
	stranger := vo.MustParseId(strangerID)
	_, err = svc.CreateLabel(ctx, stranger, model.CreateLabelRequest{
		Id: vo.NewId().String(), Name: "Denied", AccountId: &acctID,
	})
	if err == nil {
		t.Fatal("want AccessDenied for a caller with no grant on the account, got nil")
	}
	if _, ok := errs.AsAccessDenied(err); !ok {
		t.Fatalf("want *AccessDeniedError, got %v (%T)", err, err)
	}
}

func TestCreateLabelNameLengthBoundaries(t *testing.T) {
	db := dbtest.New(t)
	labelFixture(t, db)
	svc := newLabelSvc(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(labelOwnerID)

	cases := []struct {
		name    string
		wantErr bool
	}{
		{strings.Repeat("a", 2), true},   // 2 runes: too short
		{strings.Repeat("a", 3), false},  // 3 runes: ok
		{strings.Repeat("a", 64), false}, // 64 runes: ok
		{strings.Repeat("a", 65), true},  // 65 runes: too long
	}
	for _, c := range cases {
		if got := len([]rune(c.name)); (got < 3 || got > 64) != c.wantErr {
			t.Fatalf("test setup bug: name %q has %d runes, wantErr=%v", c.name, got, c.wantErr)
		}
		_, err := svc.CreateLabel(ctx, owner, model.CreateLabelRequest{Id: vo.NewId().String(), Name: c.name})
		if c.wantErr {
			if err == nil {
				t.Errorf("name %q (%d runes): want error, got nil", c.name, len([]rune(c.name)))
				continue
			}
			ve, ok := errs.AsValidation(err)
			if !ok {
				t.Errorf("name %q: want *ValidationError, got %v (%T)", c.name, err, err)
				continue
			}
			if ve.Msg != "Label name must be 3-64 characters" {
				t.Errorf("name %q: want frozen message, got %q", c.name, ve.Msg)
			}
			if len(ve.Fields) != 1 || ve.Fields[0].Key != "name" || ve.Fields[0].Code != errs.CodeLabelNameLength {
				t.Errorf("name %q: want field 'name' with code %q, got %+v", c.name, errs.CodeLabelNameLength, ve.Fields)
			}
		} else if err != nil {
			t.Errorf("name %q (%d runes): want no error, got %v", c.name, len([]rune(c.name)), err)
		}
	}
}

func TestCreateLabelRejectsCaseVariantName(t *testing.T) {
	db := dbtest.New(t)
	labelFixture(t, db)
	svc := newLabelSvc(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(labelOwnerID)

	_, err := svc.CreateLabel(ctx, owner, model.CreateLabelRequest{
		Id: vo.NewId().String(), Name: "Trip",
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = svc.CreateLabel(ctx, owner, model.CreateLabelRequest{
		Id: vo.NewId().String(), Name: "trip",
	})
	if err == nil {
		t.Fatal("want case-variant name rejected, got nil error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.MsgCode != errs.CodeLabelAlreadyExists {
		t.Fatalf("want CodeLabelAlreadyExists, got %#v", err)
	}
}
