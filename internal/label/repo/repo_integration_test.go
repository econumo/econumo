package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	labelrepo "github.com/econumo/econumo/internal/label/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/sortkey"
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

func seedUser(t *testing.T, f *fixture.Builder, id string) {
	t.Helper()
	f.User(fixture.User{ID: id, Name: "u"})
}

func newRepo(t *testing.T) (*labelrepo.Repo, *labelrepo.ReadRepo, *dbtest.DB, *fixture.Builder) {
	t.Helper()
	db := dbtest.New(t)
	return labelrepo.NewRepo(db.Engine, db.TX), labelrepo.NewReadRepo(db.Engine, db.TX), db, fixture.New(t, db)
}

func label(id, userID, name string, pos int16) *model.Label {
	return &model.Label{ID: vo.MustParseId(id), UserID: vo.MustParseId(userID), Name: name, Icon: "kid",
		SortKey: keyAt(pos), IsArchived: false, CreatedAt: fixedTime, UpdatedAt: fixedTime}
}

func TestLabelRepo_SaveGetRoundTrip(t *testing.T) {
	repo, _, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	if err := repo.Save(ctx, label(labelA1, userA, "Kid A", 4)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(labelA1))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Kid A" || got.SortKey != keyAt(4) || got.IsArchived {
		t.Errorf("mismatch: name=%q key=%q archived=%v", got.Name, got.SortKey, got.IsArchived)
	}
	// A value different from model.DefaultLabelIcon ("label"), so a Save that
	// silently dropped Icon and let the column default fill it in would still
	// be caught here.
	if got.Icon != "kid" {
		t.Errorf("icon mismatch: got %q, want %q (default is %q)", got.Icon, "kid", model.DefaultLabelIcon)
	}
	if !got.UpdatedAt.Equal(fixedTime) {
		t.Errorf("updatedAt mismatch: %v", got.UpdatedAt)
	}
}

// TestLabelRepo_Save_UpdatesOnConflict exercises the UpsertLabel "ON CONFLICT
// (id) DO UPDATE" path: a second Save with the same id mutates every
// updatable column, so a broken/dropped entry in the query's excluded.* list
// (e.g. icon or is_archived silently not updated) would fail this.
func TestLabelRepo_Save_UpdatesOnConflict(t *testing.T) {
	repo, _, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	seedUser(t, f, userB)

	created := fixedTime
	l := label(labelA1, userA, "Kid A", 0)
	l.CreatedAt, l.UpdatedAt = created, created
	if err := repo.Save(ctx, l); err != nil {
		t.Fatalf("initial Save: %v", err)
	}

	updated := created.Add(24 * time.Hour)
	l2 := &model.Label{
		ID: vo.MustParseId(labelA1), UserID: vo.MustParseId(userB), Name: "Renamed",
		Icon: "pet", SortKey: keyAt(7), IsArchived: true, CreatedAt: updated, UpdatedAt: updated,
	}
	if err := repo.Save(ctx, l2); err != nil {
		t.Fatalf("conflicting Save: %v", err)
	}

	got, err := repo.GetByID(ctx, vo.MustParseId(labelA1))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.UserID.String() != userB {
		t.Errorf("user_id not updated: got %s, want %s", got.UserID, userB)
	}
	if got.Name != "Renamed" {
		t.Errorf("name not updated: got %q", got.Name)
	}
	if got.Icon != "pet" {
		t.Errorf("icon not updated: got %q", got.Icon)
	}
	if got.SortKey != keyAt(7) {
		t.Errorf("sort_key not updated: got %q", got.SortKey)
	}
	if !got.IsArchived {
		t.Error("is_archived not updated: got false")
	}
	if !got.UpdatedAt.Equal(updated) {
		t.Errorf("updated_at not updated: got %v, want %v", got.UpdatedAt, updated)
	}
	// created_at is intentionally NOT in the query's UPDATE SET list -> the
	// original insert value survives a conflicting Save.
	if !got.CreatedAt.Equal(created) {
		t.Errorf("created_at should be unchanged by the conflict update: got %v, want %v", got.CreatedAt, created)
	}
}

func TestLabelRepo_GetByID_NotFound(t *testing.T) {
	repo, _, _, f := newRepo(t)
	seedUser(t, f, userA)
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestLabelRepo_ListByOwner(t *testing.T) {
	repo, _, _, f := newRepo(t)
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
		t.Errorf("order by sort key wrong: %s, %s", list[0].ID, list[1].ID)
	}
}

func TestLabelRepo_Delete(t *testing.T) {
	repo, _, _, f := newRepo(t)
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
}

func TestLabelRepo_NextIdentity(t *testing.T) {
	repo, _, _, _ := newRepo(t)
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

// TestLabelRepo_NewRepo_PostgresqlDispatch exercises the "postgresql" branch
// of NewRepo/NewPgsqlRepo without issuing any SQL: NextIdentity never touches
// the database, so this is dialect-safe (unlike calling the pgsql querier's
// query methods against a non-PostgreSQL backend, which would not be).
func TestLabelRepo_NewRepo_PostgresqlDispatch(t *testing.T) {
	db := dbtest.New(t)
	repo := labelrepo.NewRepo("postgresql", db.TX)
	a := repo.NextIdentity()
	b := repo.NextIdentity()
	if a.String() == "" || a == b {
		t.Fatalf("NextIdentity misbehaved on the postgresql-dispatched Repo: %v, %v", a, b)
	}
}

func TestLabelReadRepo_NewReadRepo_PostgresqlDispatch(t *testing.T) {
	db := dbtest.New(t)
	read := labelrepo.NewReadRepo("postgresql", db.TX)
	if read == nil {
		t.Fatal("NewReadRepo(\"postgresql\", ...) returned nil")
	}
}

func TestLabelReadRepo_OwnPlusShared(t *testing.T) {
	repo, read, _, f := newRepo(t)
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

	f.Account(fixture.Account{ID: "acc00000-0000-0000-0000-0000000000b1", UserID: userB, CurrencyID: usdID, Name: "Shared", Type: 2, Icon: "x"})
	f.AccountAccess("acc00000-0000-0000-0000-0000000000b1", userA, 1)

	shared, err := read.LabelListView(ctx, userA)
	if err != nil {
		t.Fatalf("LabelListView shared: %v", err)
	}
	if len(shared) != 2 {
		t.Fatalf("want own + shared (2), got %d", len(shared))
	}
}

// TestLabelReadRepo_ArchivedStillVisible pins down that the read model does
// NOT filter out archived labels: archived-but-referenced labels must stay
// resolvable for historical transaction display.
func TestLabelReadRepo_ArchivedStillVisible(t *testing.T) {
	repo, read, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	l := label(labelA1, userA, "Retired", 0)
	l.Archive(fixedTime)
	if err := repo.Save(ctx, l); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rows, err := read.LabelListView(ctx, userA)
	if err != nil {
		t.Fatalf("LabelListView: %v", err)
	}
	if len(rows) != 1 || !rows[0].IsArchived {
		t.Fatalf("want the archived label to still be listed, got %+v", rows)
	}
}

// TestLabelReadRepo_OrderedBySortKey pins down GetLabelListView's
// "ORDER BY sort_key, id": TestLabelReadRepo_OwnPlusShared only ever lists one
// own label, so it can't catch an order regression.
func TestLabelReadRepo_OrderedBySortKey(t *testing.T) {
	repo, read, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	_ = repo.Save(ctx, label(labelA1, userA, "A1", 1))
	_ = repo.Save(ctx, label(labelA2, userA, "A2", 0))

	rows, err := read.LabelListView(ctx, userA)
	if err != nil {
		t.Fatalf("LabelListView: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != labelA2 || rows[1].ID != labelA1 {
		t.Fatalf("want sort-key order [A2, A1], got %+v", rows)
	}
}

// keyAt mirrors the 20260803000000 backfill encoding: the 'c' magnitude head
// plus three base-62 digits. Tests keep expressing intended order as a small
// integer while the repo orders by key.
func keyAt[T ~int | ~int16](pos T) sortkey.Key {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	n := int(pos)
	if n < 0 {
		n = 0
	}
	return sortkey.Key("c" + string(alphabet[(n/3844)%62]) + string(alphabet[(n/62)%62]) + string(alphabet[n%62]))
}
