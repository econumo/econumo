package transactionrepo_test

// Integration tests for the transaction Repo against a real migrated in-memory
// SQLite: CRUD round-trip (incl. exact decimal amount + transfer recipient),
// NotFound, ListByAccount (source or recipient), the dynamic ListByAccountIDs
// with a month-boundary period filter (datetime-binding regression), and the
// own+shared export-accounts list.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtransaction "github.com/econumo/econumo/internal/domain/transaction"
	transactionrepo "github.com/econumo/econumo/internal/infra/repo/transaction"
	"github.com/econumo/econumo/internal/test/dbtest"
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
	db := dbtest.NewSQLite(t)
	seedUser(t, db, userA)
	seedAccount(t, db, acct1, userA)
	seedAccount(t, db, acct2, userA)
	return transactionrepo.NewRepo("sqlite", db.TX), db
}

func seedUser(t *testing.T, db *dbtest.DB, id string) {
	t.Helper()
	db.Exec(t, `INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active) VALUES (?, ?, '', 'u', '', '', '', ?, ?, 1)`,
		id, id, fixedTime, fixedTime)
}

func seedAccount(t *testing.T, db *dbtest.DB, id, userID string) {
	t.Helper()
	db.Exec(t, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, 'A', 2, 'x', 0, ?, ?)`,
		id, usdID, userID, fixedTime, fixedTime)
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func expense(id, account, amount string, spentAt time.Time) *domtransaction.Transaction {
	return domtransaction.FromState(domtransaction.NewState{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA), Type: domtransaction.TypeExpense,
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
	if got.Amount() != "123.45" {
		t.Errorf("amount mismatch: %q", got.Amount())
	}
	if got.Type() != domtransaction.TypeExpense || got.AccountId().String() != acct1 {
		t.Errorf("fields mismatch: type=%d account=%s", got.Type(), got.AccountId())
	}
	if !got.SpentAt().Equal(spent) {
		t.Errorf("spentAt mismatch: %v", got.SpentAt())
	}
	if got.AccountRecipientId() != nil {
		t.Error("expense should have no recipient")
	}
}

func TestTransactionRepo_SaveGetRoundTrip_Transfer(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	id := "7c000000-0000-0000-0000-000000000002"
	recip := vo.MustParseId(acct2)
	amtRecip := "90.5"
	tx := domtransaction.FromState(domtransaction.NewState{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA), Type: domtransaction.TypeTransfer,
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
	if got.Amount() != "100.25" {
		t.Errorf("amount mismatch: %q", got.Amount())
	}
	if got.AccountRecipientId() == nil || got.AccountRecipientId().String() != acct2 {
		t.Errorf("recipient mismatch: %v", got.AccountRecipientId())
	}
	if got.AmountRecipient() == nil || *got.AmountRecipient() != "90.5" {
		t.Errorf("amount_recipient mismatch: %v", deref(got.AmountRecipient()))
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
	_ = repo.Save(ctx, domtransaction.FromState(domtransaction.NewState{
		ID: vo.MustParseId("7c000000-0000-0000-0000-000000000005"), UserID: vo.MustParseId(userA),
		Type: domtransaction.TypeTransfer, AccountID: vo.MustParseId(acct2), AccountRecipID: &recip,
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
	list, err := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(acct1)}, start, end)
	if err != nil {
		t.Fatalf("ListByAccountIDs: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 in-period (incl. Mar 1 boundary), got %d", len(list))
	}

	// No period -> all three.
	all, err := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(acct1)}, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("ListByAccountIDs no period: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("want 3 without period, got %d", len(all))
	}

	// Empty id set -> nil.
	none, err := repo.ListByAccountIDs(ctx, nil, start, end)
	if err != nil || none != nil {
		t.Errorf("empty ids should yield nil,nil; got %v, %v", none, err)
	}
}

func TestTransactionRepo_ListExportAccountsForUser_OwnPlusShared(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	// userB owns acctB and grants access to userA.
	seedUser(t, db, userB)
	seedAccount(t, db, acctB, userB)
	db.Exec(t, `INSERT INTO accounts_access (account_id, user_id, role, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`,
		acctB, userA, fixedTime, fixedTime)

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
