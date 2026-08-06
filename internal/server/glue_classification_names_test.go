package server_test

// Integration test for the shared classification-name namespace: the tag and
// label services, wired to each other exactly as BuildAPI wires them, must
// reject a name the other kind already holds.

import (
	"context"
	"testing"

	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	applabel "github.com/econumo/econumo/internal/label"
	labelrepo "github.com/econumo/econumo/internal/label/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	apptag "github.com/econumo/econumo/internal/tag"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const glueClassificationOwner = "44444444-4444-4444-4444-444444444444"

// newClassificationSvcs builds both services over one db and cross-wires them
// through the real glue adapters, mirroring BuildAPI.
func newClassificationSvcs(t *testing.T, db *dbtest.DB) (*apptag.Service, *applabel.Service) {
	t.Helper()
	opGuard := operationrepo.NewGuard(db.Engine, db.TX)
	access := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, db.TX))

	tagSvc := apptag.NewService(
		tagrepo.NewRepo(db.Engine, db.TX), db.TX, opGuard, clock.New(),
		tagrepo.NewReadRepo(db.Engine, db.TX), access,
	)
	labelSvc := applabel.NewService(
		labelrepo.NewRepo(db.Engine, db.TX), db.TX, opGuard, clock.New(),
		labelrepo.NewReadRepo(db.Engine, db.TX), access,
	)
	server.WireClassificationNames(tagSvc, labelSvc)
	return tagSvc, labelSvc
}

func TestClassificationNamesAreOneNamespace(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: glueClassificationOwner, Name: "Owner"})

	tagSvc, labelSvc := newClassificationSvcs(t, db)
	ctx := context.Background()
	owner := vo.MustParseId(glueClassificationOwner)

	// A tag name blocks a label of the same name (case-insensitively).
	if _, err := tagSvc.CreateTag(ctx, owner, model.CreateTagRequest{
		Id: vo.NewId().String(), Name: "Travel",
	}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	_, err := labelSvc.CreateLabel(ctx, owner, model.CreateLabelRequest{
		Id: vo.NewId().String(), Name: "travel",
	})
	if err == nil {
		t.Fatal("want the label create blocked by the existing tag name, got nil error")
	}
	if ve, ok := errs.AsValidation(err); !ok || ve.MsgCode != errs.CodeLabelAlreadyExists {
		t.Fatalf("want CodeLabelAlreadyExists, got %#v", err)
	}

	// And the reverse: a label name blocks a tag of the same name.
	if _, err := labelSvc.CreateLabel(ctx, owner, model.CreateLabelRequest{
		Id: vo.NewId().String(), Name: "Reimbursable",
	}); err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	_, err = tagSvc.CreateTag(ctx, owner, model.CreateTagRequest{
		Id: vo.NewId().String(), Name: "reimbursable",
	})
	if err == nil {
		t.Fatal("want the tag create blocked by the existing label name, got nil error")
	}
	if ve, ok := errs.AsValidation(err); !ok || ve.MsgCode != errs.CodeTagAlreadyExists {
		t.Fatalf("want CodeTagAlreadyExists, got %#v", err)
	}
}
