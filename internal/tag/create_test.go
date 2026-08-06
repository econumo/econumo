package tag_test

import (
	"context"
	"errors"
	"testing"

	apptag "github.com/econumo/econumo/internal/tag"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"

	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const tagOwnerID = "11111111-1111-1111-1111-111111111111"

// newTagSvc wires a real tag.Service over sqlite repos for the given db.
func newTagSvc(t *testing.T, db *dbtest.DB) *apptag.Service {
	t.Helper()
	repo := tagrepo.NewRepo(db.Engine, db.TX)
	readRepo := tagrepo.NewReadRepo(db.Engine, db.TX)
	opGuard := operationrepo.NewGuard(db.Engine, db.TX)
	access := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, db.TX))
	return apptag.NewService(repo, db.TX, opGuard, clock.New(), readRepo, access)
}

// tagFixture seeds an owner. Returns the builder.
func tagFixture(t *testing.T, db *dbtest.DB) *fixture.Builder {
	t.Helper()
	f := fixture.New(t, db)
	f.User(fixture.User{ID: tagOwnerID, Name: "Owner"})
	return f
}

// stubLabelNames stands in for the label feature's names port.
type stubLabelNames struct{ names []string }

func (s stubLabelNames) NamesByOwner(context.Context, vo.Id) ([]string, error) {
	return s.names, nil
}

func TestCreateTagRejectsNameTakenByLabel(t *testing.T) {
	db := dbtest.New(t)
	tagFixture(t, db)
	svc := newTagSvc(t, db)
	svc.SetLabelNames(stubLabelNames{names: []string{"Trip"}})
	ctx := context.Background()
	owner := vo.MustParseId(tagOwnerID)

	_, err := svc.CreateTag(ctx, owner, model.CreateTagRequest{
		Id: vo.NewId().String(), Name: "trip",
	})
	if err == nil {
		t.Fatal("want name-taken-by-label rejected, got nil error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.MsgCode != errs.CodeTagAlreadyExists {
		t.Fatalf("want CodeTagAlreadyExists, got %#v", err)
	}
}

func TestCreateTagRejectsCaseVariantName(t *testing.T) {
	db := dbtest.New(t)
	tagFixture(t, db)
	svc := newTagSvc(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(tagOwnerID)

	_, err := svc.CreateTag(ctx, owner, model.CreateTagRequest{
		Id: vo.NewId().String(), Name: "Trip",
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = svc.CreateTag(ctx, owner, model.CreateTagRequest{
		Id: vo.NewId().String(), Name: "trip",
	})
	if err == nil {
		t.Fatal("want case-variant name rejected, got nil error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.MsgCode != errs.CodeTagAlreadyExists {
		t.Fatalf("want CodeTagAlreadyExists, got %#v", err)
	}
}
