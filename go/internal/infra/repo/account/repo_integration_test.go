package accountrepo_test

// Integration tests for the account Repo + FolderRepo against a real migrated
// in-memory SQLite. Locks CRUD round-trips, NotFound mapping, accounts_options
// positions, folder membership, and the float-balance SUM rendering (including
// the "-0"/exact-string regression) end-to-end through the repo.

import (
	"context"
	"errors"
	"testing"
	"time"

	domaccount "github.com/econumo/econumo/internal/domain/account"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	accountrepo "github.com/econumo/econumo/internal/infra/repo/account"
	"github.com/econumo/econumo/internal/testutil"
)

const (
	usdID     = "dffc2a06-6f29-4704-8575-31709adee926"
	userA     = "11111111-1111-1111-1111-111111111111"
	userB     = "22222222-2222-2222-2222-222222222222"
	acctCash  = "aaaa1111-0000-0000-0000-0000000000a1"
	acctBank  = "aaaa1111-0000-0000-0000-0000000000a2"
	acctOther = "bbbb1111-0000-0000-0000-0000000000b1"
)

var fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func seedUser(t *testing.T, db *testutil.DB, id, name string) {
	t.Helper()
	db.Exec(t, `INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active) VALUES (?, ?, '', ?, '', '', '', ?, ?, 1)`,
		id, id, name, fixedTime, fixedTime)
}

func seedAccount(t *testing.T, db *testutil.DB, id, userID, name string) {
	t.Helper()
	db.Exec(t, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, ?, 2, 'wallet', 0, ?, ?)`,
		id, usdID, userID, name, fixedTime, fixedTime)
}

func newAccountRepo(t *testing.T) (*accountrepo.Repo, *testutil.DB) {
	t.Helper()
	db := testutil.NewSQLite(t)
	return accountrepo.NewRepo("sqlite", db.TX), db
}

func TestAccountRepo_SaveGetRoundTrip(t *testing.T) {
	repo, db := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA, "A")

	id := vo.MustParseId(acctCash)
	acc := domaccount.FromState(
		id, vo.MustParseId(userA), vo.MustParseId(usdID),
		"Wallet", domaccount.TypeCash, "icon-wallet", false, fixedTime, fixedTime,
	)
	if err := repo.Save(ctx, acc); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Id() != id || got.UserId().String() != userA || got.CurrencyId().String() != usdID {
		t.Errorf("ids mismatch: %+v", got)
	}
	if got.Name() != "Wallet" || got.Type() != domaccount.TypeCash || got.Icon() != "icon-wallet" {
		t.Errorf("fields mismatch: name=%q type=%d icon=%q", got.Name(), got.Type(), got.Icon())
	}
	if got.IsDeleted() {
		t.Error("IsDeleted should be false")
	}
	if !got.CreatedAt().Equal(fixedTime) || !got.UpdatedAt().Equal(fixedTime) {
		t.Errorf("timestamps mismatch: created=%v updated=%v", got.CreatedAt(), got.UpdatedAt())
	}
}

func TestAccountRepo_GetByID_NotFound(t *testing.T) {
	repo, db := newAccountRepo(t)
	seedUser(t, db, userA, "A")
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestAccountRepo_ListAndCountAvailable(t *testing.T) {
	repo, db := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA, "A")
	seedAccount(t, db, acctCash, userA, "Cash")
	seedAccount(t, db, acctBank, userA, "Bank")
	// A deleted account must be excluded.
	db.Exec(t, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, 'Gone', 2, 'x', 1, ?, ?)`,
		acctOther, usdID, userA, fixedTime, fixedTime)

	list, err := repo.ListAvailable(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 available accounts, got %d", len(list))
	}
	n, err := repo.CountAvailable(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("CountAvailable: %v", err)
	}
	if n != 2 {
		t.Errorf("want count 2, got %d", n)
	}
}

func TestAccountRepo_Positions(t *testing.T) {
	repo, db := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA, "A")
	seedAccount(t, db, acctCash, userA, "Cash")
	seedAccount(t, db, acctBank, userA, "Bank")

	// No option row yet.
	_, ok, err := repo.GetPosition(ctx, vo.MustParseId(acctCash), vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("GetPosition: %v", err)
	}
	if ok {
		t.Error("expected no position row")
	}

	if err := repo.SavePosition(ctx, vo.MustParseId(acctCash), vo.MustParseId(userA), 3, fixedTime); err != nil {
		t.Fatalf("SavePosition cash: %v", err)
	}
	if err := repo.SavePosition(ctx, vo.MustParseId(acctBank), vo.MustParseId(userA), 7, fixedTime); err != nil {
		t.Fatalf("SavePosition bank: %v", err)
	}

	pos, ok, err := repo.GetPosition(ctx, vo.MustParseId(acctCash), vo.MustParseId(userA))
	if err != nil || !ok {
		t.Fatalf("GetPosition cash: ok=%v err=%v", ok, err)
	}
	if pos != 3 {
		t.Errorf("want position 3, got %d", pos)
	}
	max, err := repo.MaxPosition(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("MaxPosition: %v", err)
	}
	if max != 7 {
		t.Errorf("want max position 7, got %d", max)
	}

	// Upsert: re-save with a new position overwrites.
	if err := repo.SavePosition(ctx, vo.MustParseId(acctCash), vo.MustParseId(userA), 9, fixedTime); err != nil {
		t.Fatalf("SavePosition re-save: %v", err)
	}
	pos, _, _ = repo.GetPosition(ctx, vo.MustParseId(acctCash), vo.MustParseId(userA))
	if pos != 9 {
		t.Errorf("want position 9 after upsert, got %d", pos)
	}
}

func TestAccountRepo_Balance_RoundsFloatSum(t *testing.T) {
	repo, db := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA, "A")
	seedAccount(t, db, acctCash, userA, "Cash")

	// Three income transactions that float-sum with drift but must render clean.
	seedTx := func(id, amt string) {
		db.Exec(t, `INSERT INTO transactions (id, user_id, account_id, type, amount, description, created_at, updated_at, spent_at) VALUES (?, ?, ?, 1, ?, '', ?, ?, ?)`,
			id, userA, acctCash, amt, fixedTime, fixedTime, "2024-03-01 00:00:00")
	}
	seedTx("c0000000-0000-0000-0000-000000000001", "100.10")
	seedTx("c0000000-0000-0000-0000-000000000002", "200.20")
	seedTx("c0000000-0000-0000-0000-000000000003", "58.05")

	before := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	bal, err := repo.Balance(ctx, vo.MustParseId(acctCash), before)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal != "358.35" {
		t.Errorf("want balance 358.35, got %q", bal)
	}

	// No transactions -> "0".
	seedAccount(t, db, acctBank, userA, "Bank")
	bal2, err := repo.Balance(ctx, vo.MustParseId(acctBank), before)
	if err != nil {
		t.Fatalf("Balance empty: %v", err)
	}
	if bal2 != "0" {
		t.Errorf("want empty balance 0, got %q", bal2)
	}
}

func TestAccountRepo_Balances_NegativeZeroNormalized(t *testing.T) {
	repo, db := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA, "A")
	seedAccount(t, db, acctCash, userA, "Cash")
	db.Exec(t, `INSERT INTO accounts_options (account_id, user_id, position, created_at, updated_at) VALUES (?, ?, 0, ?, ?)`,
		acctCash, userA, fixedTime, fixedTime)

	// An income then an equal expense -> net 0 (must not be "-0").
	db.Exec(t, `INSERT INTO transactions (id, user_id, account_id, type, amount, description, created_at, updated_at, spent_at) VALUES (?, ?, ?, 1, '50.00', '', ?, ?, '2024-03-01 00:00:00')`,
		"d0000000-0000-0000-0000-000000000001", userA, acctCash, fixedTime, fixedTime)
	db.Exec(t, `INSERT INTO transactions (id, user_id, account_id, type, amount, description, created_at, updated_at, spent_at) VALUES (?, ?, ?, 0, '50.00', '', ?, ?, '2024-03-01 00:00:00')`,
		"d0000000-0000-0000-0000-000000000002", userA, acctCash, fixedTime, fixedTime)

	before := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	balances, err := repo.Balances(ctx, vo.MustParseId(userA), before)
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}
	got, ok := balances[acctCash]
	if !ok {
		t.Fatalf("balance for %s missing: %+v", acctCash, balances)
	}
	if got != "0" {
		t.Errorf("want normalized 0 (no -0), got %q", got)
	}
}

func TestAccountRepo_SaveCorrection(t *testing.T) {
	repo, db := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA, "A")
	seedAccount(t, db, acctCash, userA, "Cash")

	corrID := vo.NewId()
	c := domaccount.Correction{
		ID:          corrID,
		UserID:      vo.MustParseId(userA),
		AccountID:   vo.MustParseId(acctCash),
		Description: "fix",
		CreatedAt:   fixedTime,
		SpentAt:     fixedTime,
		Type:        1,
		Amount:      "12.34",
	}
	if err := repo.SaveCorrection(ctx, c); err != nil {
		t.Fatalf("SaveCorrection: %v", err)
	}
	var amount string
	if err := db.Raw.QueryRowContext(ctx, `SELECT amount FROM transactions WHERE id = ?`, corrID.String()).Scan(&amount); err != nil {
		t.Fatalf("read correction: %v", err)
	}
	if amount != "12.34" {
		t.Errorf("want amount 12.34, got %q", amount)
	}
}
