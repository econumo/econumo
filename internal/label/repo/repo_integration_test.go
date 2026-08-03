package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	labelrepo "github.com/econumo/econumo/internal/label/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	usdID   = "dffc2a06-6f29-4704-8575-31709adee926"
	userA   = "11111111-1111-1111-1111-111111111111"
	userB   = "22222222-2222-2222-2222-222222222222"
	labelA1 = "1ab00000-0000-0000-0000-0000000000a1"
	labelA2 = "1ab00000-0000-0000-0000-0000000000a2"
	labelB1 = "1ab00000-0000-0000-0000-0000000000b1"
)

var fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

// drivers exercises both engine adapters against the same sqlite-backed test
// database. SQLite's parameter syntax also accepts "$N" placeholders (used by
// the postgresql adapter's generated queries), so NewRepo("postgresql", ...)
// runs correctly here too: this validates the pgsqlQuerier/pgsqlReadQuerier
// whole-struct conversions for real, not just that they compile, without
// requiring a live PostgreSQL for the smoke tier. PostgreSQL-specific SQL
// semantics are covered separately by internal/test/enginecompare.
var drivers = []string{"sqlite", "postgresql"}

func seedUser(t *testing.T, f *fixture.Builder, id string) {
	t.Helper()
	f.User(fixture.User{ID: id, Name: "u"})
}

func newRepo(t *testing.T, driver string) (*labelrepo.Repo, *labelrepo.ReadRepo, *dbtest.DB, *fixture.Builder) {
	t.Helper()
	db := dbtest.New(t)
	return labelrepo.NewRepo(driver, db.TX), labelrepo.NewReadRepo(driver, db.TX), db, fixture.New(t, db)
}

func label(id, userID, name string, pos int16) *model.Label {
	return &model.Label{ID: vo.MustParseId(id), UserID: vo.MustParseId(userID), Name: name, Icon: "kid",
		Position: pos, IsArchived: false, CreatedAt: fixedTime, UpdatedAt: fixedTime}
}

func TestLabelRepo_SaveGetRoundTrip(t *testing.T) {
	for _, driver := range drivers {
		t.Run(driver, func(t *testing.T) {
			repo, _, _, f := newRepo(t, driver)
			ctx := context.Background()
			seedUser(t, f, userA)
			if err := repo.Save(ctx, label(labelA1, userA, "Kid A", 4)); err != nil {
				t.Fatalf("Save: %v", err)
			}
			got, err := repo.GetByID(ctx, vo.MustParseId(labelA1))
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if got.Name != "Kid A" || got.Position != 4 || got.IsArchived {
				t.Errorf("mismatch: name=%q pos=%d archived=%v", got.Name, got.Position, got.IsArchived)
			}
			// A value different from model.DefaultLabelIcon ("label"), so a Save
			// that silently dropped Icon and let the column default fill it in
			// would still be caught here.
			if got.Icon != "kid" {
				t.Errorf("icon mismatch: got %q, want %q (default is %q)", got.Icon, "kid", model.DefaultLabelIcon)
			}
			if !got.UpdatedAt.Equal(fixedTime) {
				t.Errorf("updatedAt mismatch: %v", got.UpdatedAt)
			}
		})
	}
}

func TestLabelRepo_GetByID_NotFound(t *testing.T) {
	for _, driver := range drivers {
		t.Run(driver, func(t *testing.T) {
			repo, _, _, f := newRepo(t, driver)
			seedUser(t, f, userA)
			_, err := repo.GetByID(context.Background(), vo.NewId())
			var nf *errs.NotFoundError
			if !errors.As(err, &nf) {
				t.Fatalf("want NotFoundError, got %v", err)
			}
		})
	}
}

func TestLabelRepo_ListAndCountByOwner(t *testing.T) {
	for _, driver := range drivers {
		t.Run(driver, func(t *testing.T) {
			repo, _, _, f := newRepo(t, driver)
			ctx := context.Background()
			seedUser(t, f, userA)
			seedUser(t, f, userB)
			_ = repo.Save(ctx, label(labelA1, userA, "A1", 1))
			_ = repo.Save(ctx, label(labelA2, userA, "A2", 0))
			_ = repo.Save(ctx, label(labelB1, userB, "B1", 0))

			list, err := repo.ListByOwner(ctx, vo.MustParseId(userA))
			if err != nil {
				t.Fatalf("ListByOwner: %v", err)
			}
			if len(list) != 2 {
				t.Fatalf("want 2, got %d", len(list))
			}
			if list[0].ID.String() != labelA2 || list[1].ID.String() != labelA1 {
				t.Errorf("order by position wrong: %s, %s", list[0].ID, list[1].ID)
			}
			n, err := repo.CountByOwner(ctx, vo.MustParseId(userA))
			if err != nil || n != 2 {
				t.Errorf("CountByOwner = %d, %v; want 2", n, err)
			}
		})
	}
}

func TestLabelRepo_Delete(t *testing.T) {
	for _, driver := range drivers {
		t.Run(driver, func(t *testing.T) {
			repo, _, _, f := newRepo(t, driver)
			ctx := context.Background()
			seedUser(t, f, userA)
			_ = repo.Save(ctx, label(labelA1, userA, "A1", 0))
			if err := repo.Delete(ctx, vo.MustParseId(labelA1)); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			_, err := repo.GetByID(ctx, vo.MustParseId(labelA1))
			var nf *errs.NotFoundError
			if !errors.As(err, &nf) {
				t.Fatalf("want NotFound after delete, got %v", err)
			}
		})
	}
}

func TestLabelRepo_NextIdentity(t *testing.T) {
	repo, _, _, _ := newRepo(t, "sqlite")
	a := repo.NextIdentity()
	b := repo.NextIdentity()
	if a.String() == "" || b.String() == "" {
		t.Fatal("NextIdentity returned an empty id")
	}
	if a == b {
		t.Error("successive NextIdentity calls should not collide")
	}
}

func TestLabelRepo_UnknownDriver_Panics(t *testing.T) {
	db := dbtest.New(t)
	defer func() {
		if recover() == nil {
			t.Fatal("want panic for an unknown driver")
		}
	}()
	labelrepo.NewRepo("mysql", db.TX)
}

func TestLabelReadRepo_UnknownDriver_Panics(t *testing.T) {
	db := dbtest.New(t)
	defer func() {
		if recover() == nil {
			t.Fatal("want panic for an unknown driver")
		}
	}()
	labelrepo.NewReadRepo("mysql", db.TX)
}

func TestLabelReadRepo_OwnPlusShared(t *testing.T) {
	for _, driver := range drivers {
		t.Run(driver, func(t *testing.T) {
			repo, read, _, f := newRepo(t, driver)
			ctx := context.Background()
			seedUser(t, f, userA)
			seedUser(t, f, userB)
			_ = repo.Save(ctx, label(labelA1, userA, "A1", 0))
			_ = repo.Save(ctx, label(labelB1, userB, "B1", 0))

			own, err := read.LabelListView(ctx, userA)
			if err != nil {
				t.Fatalf("LabelListView: %v", err)
			}
			if len(own) != 1 || own[0].ID != labelA1 {
				t.Fatalf("want only own A1, got %+v", own)
			}
			if own[0].Icon != "kid" {
				t.Errorf("icon mismatch: %q", own[0].Icon)
			}
			if own[0].CreatedAt != "2024-04-01 12:00:00" {
				t.Errorf("datetime format wrong: %q", own[0].CreatedAt)
			}

			accID := "acc10000-0000-0000-0000-0000000000" + driver[:2]
			f.Account(fixture.Account{ID: accID, UserID: userB, CurrencyID: usdID, Name: "Shared", Type: 2, Icon: "x"})
			f.AccountAccess(accID, userA, 1)

			shared, err := read.LabelListView(ctx, userA)
			if err != nil {
				t.Fatalf("LabelListView shared: %v", err)
			}
			if len(shared) != 2 {
				t.Fatalf("want own + shared (2), got %d", len(shared))
			}
		})
	}
}
