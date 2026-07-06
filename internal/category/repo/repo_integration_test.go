package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	usdID = "dffc2a06-6f29-4704-8575-31709adee926"
	userA = "11111111-1111-1111-1111-111111111111"
	userB = "22222222-2222-2222-2222-222222222222"
	catA1 = "ca700000-0000-0000-0000-0000000000a1"
	catA2 = "ca700000-0000-0000-0000-0000000000a2"
	catB1 = "ca700000-0000-0000-0000-0000000000b1"
)

var fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func seedUser(t *testing.T, f *fixture.Builder, id string) {
	t.Helper()
	f.User(fixture.User{ID: id, Name: "u"})
}

func newRepo(t *testing.T) (*categoryrepo.Repo, *categoryrepo.ReadRepo, *dbtest.DB, *fixture.Builder) {
	t.Helper()
	db := dbtest.New(t)
	return categoryrepo.NewRepo(db.Engine, db.TX), categoryrepo.NewReadRepo(db.Engine, db.TX), db, fixture.New(t, db)
}

func cat(id, userID, name string, pos int16, typ model.CategoryType) *model.Category {
	return &model.Category{ID: vo.MustParseId(id), UserID: vo.MustParseId(userID), Name: name, Position: pos,
		Type: typ, Icon: "icon", IsArchived: false, CreatedAt: fixedTime, UpdatedAt: fixedTime}
}

func TestCategoryRepo_SaveGetRoundTrip(t *testing.T) {
	repo, _, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)

	if err := repo.Save(ctx, cat(catA1, userA, "Food", 2, model.TypeExpense)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(catA1))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Food" || got.Position != 2 || got.Type != model.TypeExpense {
		t.Errorf("mismatch: name=%q pos=%d type=%d", got.Name, got.Position, got.Type)
	}
	if got.IsArchived {
		t.Error("should not be archived")
	}
	if !got.CreatedAt.Equal(fixedTime) {
		t.Errorf("createdAt mismatch: %v", got.CreatedAt)
	}
}

func TestCategoryRepo_GetByID_NotFound(t *testing.T) {
	repo, _, _, f := newRepo(t)
	seedUser(t, f, userA)
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestCategoryRepo_ListAndCountByOwner(t *testing.T) {
	repo, _, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	seedUser(t, f, userB)
	_ = repo.Save(ctx, cat(catA1, userA, "A1", 1, model.TypeExpense))
	_ = repo.Save(ctx, cat(catA2, userA, "A2", 0, model.TypeIncome))
	_ = repo.Save(ctx, cat(catB1, userB, "B1", 0, model.TypeExpense))

	list, err := repo.ListByOwner(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 own categories, got %d", len(list))
	}
	// Ordered by position: A2 (0) then A1 (1).
	if list[0].ID.String() != catA2 || list[1].ID.String() != catA1 {
		t.Errorf("order wrong: %s, %s", list[0].ID, list[1].ID)
	}
	n, err := repo.CountByOwner(ctx, vo.MustParseId(userA))
	if err != nil || n != 2 {
		t.Errorf("CountByOwner = %d, %v; want 2", n, err)
	}
}

func TestCategoryRepo_Delete(t *testing.T) {
	repo, _, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	_ = repo.Save(ctx, cat(catA1, userA, "A1", 0, model.TypeExpense))
	if err := repo.Delete(ctx, vo.MustParseId(catA1)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := repo.GetByID(ctx, vo.MustParseId(catA1))
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound after delete, got %v", err)
	}
}

func TestCategoryReadRepo_OwnPlusShared(t *testing.T) {
	repo, read, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	seedUser(t, f, userB)
	// userA owns A1; userB owns B1.
	_ = repo.Save(ctx, cat(catA1, userA, "A1", 0, model.TypeExpense))
	_ = repo.Save(ctx, cat(catB1, userB, "B1", 0, model.TypeExpense))

	// Without a grant, A sees only own.
	own, err := read.CategoryListView(ctx, userA)
	if err != nil {
		t.Fatalf("CategoryListView: %v", err)
	}
	if len(own) != 1 || own[0].ID != catA1 {
		t.Fatalf("want only own A1, got %+v", own)
	}
	// Pre-formatted datetime.
	if own[0].CreatedAt != "2024-04-01 12:00:00" {
		t.Errorf("createdAt format wrong: %q", own[0].CreatedAt)
	}

	// userB owns an account and grants access to userA -> A now sees B's categories.
	f.Account(fixture.Account{ID: "acc00000-0000-0000-0000-0000000000b1", UserID: userB, CurrencyID: usdID, Name: "Shared", Type: 2, Icon: "x"})
	f.AccountAccess("acc00000-0000-0000-0000-0000000000b1", userA, 1)

	shared, err := read.CategoryListView(ctx, userA)
	if err != nil {
		t.Fatalf("CategoryListView shared: %v", err)
	}
	if len(shared) != 2 {
		t.Fatalf("want own + shared (2), got %d: %+v", len(shared), shared)
	}
}

func TestCategoryRepo_ReassignTransactions(t *testing.T) {
	repo, _, db, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	_ = repo.Save(ctx, cat(catA1, userA, "Old", 0, model.TypeExpense))
	_ = repo.Save(ctx, cat(catA2, userA, "New", 1, model.TypeExpense))
	f.Account(fixture.Account{ID: "acc00000-0000-0000-0000-0000000000a1", UserID: userA, CurrencyID: usdID, Name: "C", Type: 2, Icon: "x"})
	f.Transaction(fixture.Transaction{ID: "7c000000-0000-0000-0000-000000000001", UserID: userA, AccountID: "acc00000-0000-0000-0000-0000000000a1", CategoryID: catA1, Type: 0, Amount: "10.00", SpentAt: "2024-03-01 00:00:00"})

	if err := repo.ReassignTransactions(ctx, vo.MustParseId(catA1), vo.MustParseId(catA2)); err != nil {
		t.Fatalf("ReassignTransactions: %v", err)
	}
	var catID string
	if err := db.Raw.QueryRowContext(ctx, db.Rebind(`SELECT category_id FROM transactions WHERE id = ?`), "7c000000-0000-0000-0000-000000000001").Scan(&catID); err != nil {
		t.Fatalf("read tx: %v", err)
	}
	if catID != catA2 {
		t.Errorf("want reassigned to %s, got %s", catA2, catID)
	}
}

func TestCategoryRepo_NextIdentity(t *testing.T) {
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

func TestCategoryRepo_OperationGuard_Idempotency(t *testing.T) {
	repo, _, db, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	opID := vo.NewId()

	already, err := repo.Claim(ctx, opID, fixedTime)
	if err != nil {
		t.Fatalf("Claim first: %v", err)
	}
	if already {
		t.Error("first Claim should report already=false")
	}
	already, err = repo.Claim(ctx, opID, fixedTime)
	if err != nil {
		t.Fatalf("Claim second: %v", err)
	}
	if !already {
		t.Error("second Claim should report already=true")
	}

	if err := repo.MarkHandled(ctx, opID, fixedTime); err != nil {
		t.Fatalf("MarkHandled: %v", err)
	}
	var handled bool
	if err := db.Raw.QueryRowContext(ctx, db.Rebind(`SELECT is_handled FROM operation_requests_ids WHERE id = ?`), opID.String()).Scan(&handled); err != nil {
		t.Fatalf("read op row: %v", err)
	}
	if !handled {
		t.Error("is_handled should be true after MarkHandled")
	}
}
