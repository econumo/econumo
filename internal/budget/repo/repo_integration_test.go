package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	usdID    = "dffc2a06-6f29-4704-8575-31709adee926"
	userA    = "11111111-1111-1111-1111-111111111111"
	userB    = "22222222-2222-2222-2222-222222222222"
	budgetID = "b0d00000-0000-0000-0000-00000000b001"
	acctA    = "aaaa1111-0000-0000-0000-0000000000a1"
)

var (
	fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)
	startedAt = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	aprPeriod = time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	mayPeriod = time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
)

func seedUser(t *testing.T, db *dbtest.DB, id string) {
	t.Helper()
	fixture.New(t, db).User(fixture.User{ID: id, Name: "u"})
}

func newRepo(t *testing.T) (*budgetrepo.Repo, *dbtest.DB) {
	t.Helper()
	db := dbtest.New(t)
	seedUser(t, db, userA)
	return budgetrepo.NewRepo(db.Engine, db.TX), db
}

// saveBudget persists a base budget so child rows have a valid FK.
func saveBudget(t *testing.T, repo *budgetrepo.Repo, ctx context.Context) {
	t.Helper()
	b := &model.Budget{
		ID: vo.MustParseId(budgetID), UserID: vo.MustParseId(userA), Name: "Household",
		CurrencyID: vo.MustParseId(usdID), StartedAt: startedAt, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	if err := repo.Save(ctx, b); err != nil {
		t.Fatalf("Save budget: %v", err)
	}
}

func TestBudgetRepo_BudgetCRUD(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	saveBudget(t, repo, ctx)

	got, err := repo.GetByID(ctx, vo.MustParseId(budgetID))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Household" || got.CurrencyID.String() != usdID {
		t.Errorf("fields mismatch: name=%q ccy=%s", got.Name, got.CurrencyID)
	}
	if !got.StartedAt.Equal(startedAt) {
		t.Errorf("startedAt mismatch: %v", got.StartedAt)
	}

	list, err := repo.ListForUser(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("want 1 budget, got %d", len(list))
	}

	if err := repo.Delete(ctx, vo.MustParseId(budgetID)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = repo.GetByID(ctx, vo.MustParseId(budgetID))
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound after delete, got %v", err)
	}
}

func TestBudgetRepo_AccessCRUD(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	seedUser(t, db, userB)
	saveBudget(t, repo, ctx)

	a := &model.BudgetAccess{
		ID: vo.MustParseId(budgetID), BudgetID: vo.MustParseId(budgetID), UserID: vo.MustParseId(userB),
		Role: model.BudgetRoleUser, IsAccepted: true, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	if err := repo.SaveAccess(ctx, a); err != nil {
		t.Fatalf("SaveAccess: %v", err)
	}
	got, err := repo.GetAccess(ctx, vo.MustParseId(budgetID), vo.MustParseId(userB))
	if err != nil {
		t.Fatalf("GetAccess: %v", err)
	}
	if got.Role != model.BudgetRoleUser || !got.IsAccepted {
		t.Errorf("access mismatch: role=%d accepted=%v", got.Role, got.IsAccepted)
	}
	list, err := repo.ListAccess(ctx, vo.MustParseId(budgetID))
	if err != nil || len(list) != 1 {
		t.Errorf("ListAccess = %d, %v; want 1", len(list), err)
	}

	if err := repo.DeleteAccess(ctx, vo.MustParseId(budgetID), vo.MustParseId(userB)); err != nil {
		t.Fatalf("DeleteAccess: %v", err)
	}
	_, err = repo.GetAccess(ctx, vo.MustParseId(budgetID), vo.MustParseId(userB))
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound after DeleteAccess, got %v", err)
	}
}

func TestBudgetRepo_ExcludedAccounts(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	saveBudget(t, repo, ctx)
	fixture.New(t, db).Account(fixture.Account{ID: acctA, CurrencyID: usdID, UserID: userA, Name: "A", Icon: "x"})

	if err := repo.ExcludeAccount(ctx, vo.MustParseId(budgetID), vo.MustParseId(acctA)); err != nil {
		t.Fatalf("ExcludeAccount: %v", err)
	}
	ids, err := repo.ExcludedAccountIDs(ctx, vo.MustParseId(budgetID))
	if err != nil || len(ids) != 1 || ids[0].String() != acctA {
		t.Fatalf("ExcludedAccountIDs = %v, %v; want [acctA]", ids, err)
	}
	if err := repo.IncludeAccount(ctx, vo.MustParseId(budgetID), vo.MustParseId(acctA)); err != nil {
		t.Fatalf("IncludeAccount: %v", err)
	}
	ids, _ = repo.ExcludedAccountIDs(ctx, vo.MustParseId(budgetID))
	if len(ids) != 0 {
		t.Errorf("want no excluded after include, got %d", len(ids))
	}
}

func TestBudgetRepo_FolderCRUD(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	saveBudget(t, repo, ctx)
	fid := vo.NewId()
	f := &model.BudgetFolder{ID: fid, BudgetID: vo.MustParseId(budgetID), Name: "Bills", Position: 3, CreatedAt: fixedTime, UpdatedAt: fixedTime}
	if err := repo.SaveFolder(ctx, f); err != nil {
		t.Fatalf("SaveFolder: %v", err)
	}
	got, err := repo.GetFolder(ctx, fid)
	if err != nil || got.Name != "Bills" || got.Position != 3 {
		t.Fatalf("GetFolder mismatch: %+v err=%v", got, err)
	}
	if l, _ := repo.ListFolders(ctx, vo.MustParseId(budgetID)); len(l) != 1 {
		t.Errorf("want 1 folder, got %d", len(l))
	}
	if err := repo.DeleteFolder(ctx, fid); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	_, err = repo.GetFolder(ctx, fid)
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound after DeleteFolder, got %v", err)
	}
}

func TestBudgetRepo_EnvelopeCRUDAndCategories(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	saveBudget(t, repo, ctx)
	eid := vo.NewId()
	e := &model.BudgetEnvelope{ID: eid, BudgetID: vo.MustParseId(budgetID), Name: "Groceries", Icon: "cart", IsArchived: false, CreatedAt: fixedTime, UpdatedAt: fixedTime}
	if err := repo.SaveEnvelope(ctx, e); err != nil {
		t.Fatalf("SaveEnvelope: %v", err)
	}
	got, err := repo.GetEnvelope(ctx, eid)
	if err != nil || got.Name != "Groceries" || got.Icon != "cart" {
		t.Fatalf("GetEnvelope mismatch: %+v err=%v", got, err)
	}

	// Envelope category membership.
	catID := vo.NewId()
	fixture.New(t, db).Category(fixture.Category{ID: catID.String(), UserID: userA, Name: "Food", Icon: "x"})
	if err := repo.AddEnvelopeCategory(ctx, eid, catID); err != nil {
		t.Fatalf("AddEnvelopeCategory: %v", err)
	}
	cats, err := repo.EnvelopeCategoryIDs(ctx, eid)
	if err != nil || len(cats) != 1 || cats[0].String() != catID.String() {
		t.Fatalf("EnvelopeCategoryIDs = %v, %v", cats, err)
	}
	if err := repo.RemoveEnvelopeCategory(ctx, eid, catID); err != nil {
		t.Fatalf("RemoveEnvelopeCategory: %v", err)
	}
	cats, _ = repo.EnvelopeCategoryIDs(ctx, eid)
	if len(cats) != 0 {
		t.Errorf("want no categories after remove, got %d", len(cats))
	}

	if err := repo.DeleteEnvelope(ctx, eid); err != nil {
		t.Fatalf("DeleteEnvelope: %v", err)
	}
	_, err = repo.GetEnvelope(ctx, eid)
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound after DeleteEnvelope, got %v", err)
	}
}

func TestBudgetRepo_ElementCRUD(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	saveBudget(t, repo, ctx)
	eid := vo.NewId()
	externalID := vo.NewId()
	ccy := vo.MustParseId(usdID)
	el := &model.BudgetElement{
		ID: eid, BudgetID: vo.MustParseId(budgetID), ExternalID: externalID, Type: model.ElementCategory,
		CurrencyID: &ccy, Position: 5, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	if err := repo.SaveElement(ctx, el); err != nil {
		t.Fatalf("SaveElement: %v", err)
	}
	got, err := repo.GetElement(ctx, eid)
	if err != nil {
		t.Fatalf("GetElement: %v", err)
	}
	if got.Type != model.ElementCategory || got.Position != 5 {
		t.Errorf("element mismatch: type=%d pos=%d", got.Type, got.Position)
	}
	if got.CurrencyID == nil || got.CurrencyID.String() != usdID {
		t.Errorf("currency mismatch: %v", got.CurrencyID)
	}
	byExt, err := repo.GetElementByExternal(ctx, vo.MustParseId(budgetID), externalID)
	if err != nil || byExt.ID.String() != eid.String() {
		t.Fatalf("GetElementByExternal mismatch: %+v err=%v", byExt, err)
	}
	if l, _ := repo.ListElements(ctx, vo.MustParseId(budgetID)); len(l) != 1 {
		t.Errorf("want 1 element, got %d", len(l))
	}

	if err := repo.DeleteElement(ctx, eid); err != nil {
		t.Fatalf("DeleteElement: %v", err)
	}
	_, err = repo.GetElement(ctx, eid)
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound after DeleteElement, got %v", err)
	}
}

func TestBudgetRepo_SaveLimit_Decimal(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	saveBudget(t, repo, ctx)
	eid := vo.NewId()
	el := &model.BudgetElement{
		ID: eid, BudgetID: vo.MustParseId(budgetID), ExternalID: vo.NewId(), Type: model.ElementCategory,
		Position: 0, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	if err := repo.SaveElement(ctx, el); err != nil {
		t.Fatalf("SaveElement: %v", err)
	}

	lid := vo.NewId()
	// An exact scale-8 amount must persist byte-identical in the NUMERIC column.
	limit := &model.BudgetElementLimit{ID: lid, ElementID: eid, Amount: vo.NewDecimal("250.12345678"), Period: aprPeriod, CreatedAt: fixedTime, UpdatedAt: fixedTime}
	if err := repo.SaveLimit(ctx, limit); err != nil {
		t.Fatalf("SaveLimit: %v", err)
	}
	var amount string
	if err := db.Raw.QueryRowContext(ctx, db.Rebind(`SELECT amount FROM budgets_elements_limits WHERE id = ?`), lid.String()).Scan(&amount); err != nil {
		t.Fatalf("read amount: %v", err)
	}
	if amount != "250.12345678" {
		t.Errorf("decimal limit drift: %q", amount)
	}

	// Delete via the repo removes the row.
	if err := repo.DeleteLimit(ctx, lid); err != nil {
		t.Fatalf("DeleteLimit: %v", err)
	}
	var n int
	if err := db.Raw.QueryRowContext(ctx, db.Rebind(`SELECT COUNT(*) FROM budgets_elements_limits WHERE id = ?`), lid.String()).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("want limit deleted, still %d", n)
	}
}

// TestBudgetRepo_GetLimit_DatetimeBinding regression-locks the period datetime
// comparison. Periods are stored as bare "Y-m-d H:i:s" TEXT; GetLimit /
// ListLimitsForPeriod normalize via datetime() and bind the period as that same
// string. Seeding the row directly in the canonical form, GetLimit must find it
// and ListLimitsForPeriod must scope to the right month.
func TestBudgetRepo_GetLimit_DatetimeBinding(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	saveBudget(t, repo, ctx)
	eid := "e0000000-0000-0000-0000-0000000000e1"
	f := fixture.New(t, db)
	f.BudgetElement(fixture.BudgetElement{ID: eid, BudgetID: budgetID, ExternalID: vo.NewId().String(), Type: 1, Position: 0})
	f.BudgetLimit(fixture.BudgetLimit{ID: "71000000-0000-0000-0000-000000000001", ElementID: eid, Period: "2024-04-01 00:00:00", Amount: "250.12345678"})
	f.BudgetLimit(fixture.BudgetLimit{ID: "71000000-0000-0000-0000-000000000002", ElementID: eid, Period: "2024-05-01 00:00:00", Amount: "99.99"})

	got, err := repo.GetLimit(ctx, vo.MustParseId(eid), aprPeriod)
	if err != nil {
		t.Fatalf("GetLimit: %v", err)
	}
	if got.Amount.String() != "250.12345678" {
		t.Errorf("decimal limit drift: %q", got.Amount.String())
	}

	apr, err := repo.ListLimitsForPeriod(ctx, vo.MustParseId(budgetID), aprPeriod)
	if err != nil {
		t.Fatalf("ListLimitsForPeriod apr: %v", err)
	}
	if len(apr) != 1 || apr[0].Amount.String() != "250.12345678" {
		t.Fatalf("want only the April limit, got %+v", apr)
	}

	// DeleteLimitsByBudget wipes both.
	if err := repo.DeleteLimitsByBudget(ctx, vo.MustParseId(budgetID)); err != nil {
		t.Fatalf("DeleteLimitsByBudget: %v", err)
	}
	may, _ := repo.ListLimitsForPeriod(ctx, vo.MustParseId(budgetID), mayPeriod)
	if len(may) != 0 {
		t.Errorf("want no limits after DeleteLimitsByBudget, got %d", len(may))
	}
}

func TestBudgetRepo_GetByID_NotFound(t *testing.T) {
	repo, _ := newRepo(t)
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}
