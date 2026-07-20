package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	transactionrepo "github.com/econumo/econumo/internal/transaction/repo"
)

const (
	usdID = "dffc2a06-6f29-4704-8575-31709adee926"
	userA = "11111111-1111-1111-1111-111111111111"
	userB = "22222222-2222-2222-2222-222222222222"
	acct1 = "aaaa1111-0000-0000-0000-0000000000a1"
	acct2 = "aaaa1111-0000-0000-0000-0000000000a2"
	acctB = "bbbb1111-0000-0000-0000-0000000000b1"
)

var fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func setup(t *testing.T) (*transactionrepo.Repo, *dbtest.DB) {
	t.Helper()
	db := dbtest.New(t)
	seedUser(t, db, userA)
	seedAccount(t, db, acct1, userA)
	seedAccount(t, db, acct2, userA)
	return transactionrepo.NewRepo(db.Engine, db.TX), db
}

func seedUser(t *testing.T, db *dbtest.DB, id string) {
	t.Helper()
	fixture.New(t, db).User(fixture.User{ID: id, Name: "u"})
}

func seedAccount(t *testing.T, db *dbtest.DB, id, userID string) {
	t.Helper()
	fixture.New(t, db).Account(fixture.Account{ID: id, CurrencyID: usdID, UserID: userID, Name: "A", Icon: "x"})
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func expense(id, account, amount string, spentAt time.Time) *model.Transaction {
	return model.FromState(model.NewState{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA), Type: model.TransactionTypeExpense,
		AccountID: vo.MustParseId(account), Amount: amount, Description: "exp",
		SpentAt: spentAt, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	})
}

func TestTransactionRepo_SaveGetRoundTrip_Expense(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	id := "7c000000-0000-0000-0000-000000000001"
	spent := time.Date(2024, 3, 15, 9, 30, 0, 0, time.UTC)
	// SQLite's NUMERIC affinity stores the value exactly; trailing zeros on a
	// fractional literal are normalized off ("123.45000000" -> "123.45"), so use a
	// value with no trailing-zero ambiguity and assert it survives byte-exact.
	if err := repo.Save(ctx, expense(id, acct1, "123.45", spent)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(id))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if vo.NewDecimal(got.Amount).String() != vo.NewDecimal("123.45").String() {
		t.Errorf("amount mismatch: %q", got.Amount)
	}
	if got.Type != model.TransactionTypeExpense || got.AccountID.String() != acct1 {
		t.Errorf("fields mismatch: type=%d account=%s", got.Type, got.AccountID)
	}
	if !got.SpentAt.Equal(spent) {
		t.Errorf("spentAt mismatch: %v", got.SpentAt)
	}
	if got.AccountRecipID != nil {
		t.Error("expense should have no recipient")
	}
}

func TestTransactionRepo_SaveGetRoundTrip_Transfer(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	id := "7c000000-0000-0000-0000-000000000002"
	recip := vo.MustParseId(acct2)
	amtRecip := "90.5"
	tx := model.FromState(model.NewState{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA), Type: model.TransactionTypeTransfer,
		AccountID: vo.MustParseId(acct1), AccountRecipID: &recip,
		Amount: "100.25", AmountRecipient: &amtRecip, Description: "xfer",
		SpentAt: fixedTime, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	})
	if err := repo.Save(ctx, tx); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(id))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if vo.NewDecimal(got.Amount).String() != vo.NewDecimal("100.25").String() {
		t.Errorf("amount mismatch: %q", got.Amount)
	}
	if got.AccountRecipID == nil || got.AccountRecipID.String() != acct2 {
		t.Errorf("recipient mismatch: %v", got.AccountRecipID)
	}
	if got.AmountRecipient == nil || vo.NewDecimal(*got.AmountRecipient).String() != vo.NewDecimal("90.5").String() {
		t.Errorf("amount_recipient mismatch: %v", deref(got.AmountRecipient))
	}
}

func TestTransactionRepo_GetByID_NotFound(t *testing.T) {
	repo, _ := setup(t)
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestTransactionRepo_Delete(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	id := "7c000000-0000-0000-0000-000000000003"
	if err := repo.Save(ctx, expense(id, acct1, "1.00000000", fixedTime)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Delete(ctx, vo.MustParseId(id)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := repo.GetByID(ctx, vo.MustParseId(id))
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound after delete, got %v", err)
	}
}

func TestTransactionRepo_ListByAccount_SourceOrRecipient(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	// One expense on acct1, one transfer acct2 -> acct1 (acct1 as recipient).
	_ = repo.Save(ctx, expense("7c000000-0000-0000-0000-000000000004", acct1, "5.00000000", fixedTime))
	recip := vo.MustParseId(acct1)
	amtR := "7.00000000"
	_ = repo.Save(ctx, model.FromState(model.NewState{
		ID: vo.MustParseId("7c000000-0000-0000-0000-000000000005"), UserID: vo.MustParseId(userA),
		Type: model.TransactionTypeTransfer, AccountID: vo.MustParseId(acct2), AccountRecipID: &recip,
		Amount: "7.00000000", AmountRecipient: &amtR, Description: "x",
		SpentAt: fixedTime, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}))

	list, err := repo.ListByAccount(ctx, vo.MustParseId(acct1))
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("want 2 (source + recipient), got %d", len(list))
	}
}

func TestTransactionRepo_ListByAccountIDs_PeriodBoundary(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	// Three transactions: one on the first of the month (boundary), one mid-month,
	// one the previous month. The period [Mar 1, Apr 1) must INCLUDE the Mar 1
	// boundary row and EXCLUDE the Feb row.
	_ = repo.Save(ctx, expense("7c000000-0000-0000-0000-000000000006", acct1, "1.00000000", time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)))
	_ = repo.Save(ctx, expense("7c000000-0000-0000-0000-000000000007", acct1, "2.00000000", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)))
	_ = repo.Save(ctx, expense("7c000000-0000-0000-0000-000000000008", acct1, "3.00000000", time.Date(2024, 2, 28, 0, 0, 0, 0, time.UTC)))

	start := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	list, err := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(acct1)}, start, end, model.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListByAccountIDs: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 in-period (incl. Mar 1 boundary), got %d", len(list))
	}

	// No period -> all three.
	all, err := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(acct1)}, time.Time{}, time.Time{}, model.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListByAccountIDs no period: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("want 3 without period, got %d", len(all))
	}

	// Empty id set -> nil.
	none, err := repo.ListByAccountIDs(ctx, nil, start, end, model.TransactionFilter{})
	if err != nil || none != nil {
		t.Errorf("empty ids should yield nil,nil; got %v, %v", none, err)
	}
}

func TestTransactionRepo_ListByAccountIDs_ClassificationFilters(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	catA := vo.MustParseId(f.Category(fixture.Category{UserID: userA, Type: 0}))
	catB := vo.MustParseId(f.Category(fixture.Category{UserID: userA, Type: 0}))
	payeeA := vo.MustParseId(f.Payee(fixture.Payee{UserID: userA}))
	tagA := vo.MustParseId(f.Tag(fixture.Tag{UserID: userA}))

	withCat := func(id string, cat *vo.Id, payee *vo.Id, tag *vo.Id) *model.Transaction {
		return model.FromState(model.NewState{
			ID: vo.MustParseId(id), UserID: vo.MustParseId(userA), Type: model.TransactionTypeExpense,
			AccountID: vo.MustParseId(acct1), Amount: "1.00", Description: "x",
			CategoryID: cat, PayeeID: payee, TagID: tag,
			SpentAt: fixedTime, CreatedAt: fixedTime, UpdatedAt: fixedTime,
		})
	}
	mustSave := func(tx *model.Transaction) {
		t.Helper()
		if err := repo.Save(ctx, tx); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	mustSave(withCat("7c000000-0000-0000-0000-000000000010", &catA, &payeeA, &tagA))
	mustSave(withCat("7c000000-0000-0000-0000-000000000011", &catB, nil, nil))
	mustSave(withCat("7c000000-0000-0000-0000-000000000012", nil, nil, nil))

	ids := []vo.Id{vo.MustParseId(acct1)}

	uncategorized, err := repo.ListByAccountIDs(ctx, ids, time.Time{}, time.Time{}, model.TransactionFilter{Uncategorized: true})
	if err != nil {
		t.Fatalf("uncategorized filter: %v", err)
	}
	if len(uncategorized) != 1 || uncategorized[0].ID.String() != "7c000000-0000-0000-0000-000000000012" {
		t.Fatalf("uncategorized filter = %#v, want just tx 12", uncategorized)
	}

	byCategory, err := repo.ListByAccountIDs(ctx, ids, time.Time{}, time.Time{}, model.TransactionFilter{CategoryID: &catA})
	if err != nil {
		t.Fatalf("category filter: %v", err)
	}
	if len(byCategory) != 1 || byCategory[0].ID.String() != "7c000000-0000-0000-0000-000000000010" {
		t.Fatalf("category filter = %#v, want just tx 10", byCategory)
	}

	byPayee, err := repo.ListByAccountIDs(ctx, ids, time.Time{}, time.Time{}, model.TransactionFilter{PayeeID: &payeeA})
	if err != nil {
		t.Fatalf("payee filter: %v", err)
	}
	if len(byPayee) != 1 || byPayee[0].ID.String() != "7c000000-0000-0000-0000-000000000010" {
		t.Fatalf("payee filter = %#v, want just tx 10", byPayee)
	}

	byTag, err := repo.ListByAccountIDs(ctx, ids, time.Time{}, time.Time{}, model.TransactionFilter{TagID: &tagA})
	if err != nil {
		t.Fatalf("tag filter: %v", err)
	}
	if len(byTag) != 1 || byTag[0].ID.String() != "7c000000-0000-0000-0000-000000000010" {
		t.Fatalf("tag filter = %#v, want just tx 10", byTag)
	}

	// Combined with a period window that excludes everything.
	empty, err := repo.ListByAccountIDs(ctx, ids, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2099, 2, 1, 0, 0, 0, 0, time.UTC), model.TransactionFilter{Uncategorized: true})
	if err != nil {
		t.Fatalf("uncategorized+period filter: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("uncategorized+out-of-period filter = %#v, want empty", empty)
	}
}

func TestTransactionRepo_ListExportAccountsForUser_OwnPlusShared(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	// userB owns acctB and grants access to userA.
	seedUser(t, db, userB)
	seedAccount(t, db, acctB, userB)
	fixture.New(t, db).AccountAccess(acctB, userA, 1)

	rows, err := repo.ListExportAccountsForUser(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("ListExportAccountsForUser: %v", err)
	}
	// userA owns acct1 + acct2, plus shared acctB = 3.
	if len(rows) != 3 {
		t.Fatalf("want 3 accessible accounts (2 own + 1 shared), got %d", len(rows))
	}
	for _, r := range rows {
		if r.CurrencyCode != "USD" {
			t.Errorf("currency code mismatch: %q", r.CurrencyCode)
		}
	}
}
