package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
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

func seedUser(t *testing.T, f *fixture.Builder, id, name string) {
	t.Helper()
	f.User(fixture.User{ID: id, Name: name})
}

func seedAccount(t *testing.T, f *fixture.Builder, id, userID, name string) {
	t.Helper()
	f.Account(fixture.Account{ID: id, UserID: userID, CurrencyID: usdID, Name: name, Type: 2, Icon: "wallet"})
}

func newAccountRepo(t *testing.T) (*accountrepo.Repo, *dbtest.DB, *fixture.Builder) {
	t.Helper()
	db := dbtest.NewSQLite(t)
	return accountrepo.NewRepo("sqlite", db.TX), db, fixture.New(t, db)
}

func TestAccountRepo_SaveGetRoundTrip(t *testing.T) {
	repo, _, f := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA, "A")

	id := vo.MustParseId(acctCash)
	acc := &model.Account{
		ID: id, UserID: vo.MustParseId(userA), CurrencyID: vo.MustParseId(usdID),
		Name: "Wallet", Type: model.TypeCash, Icon: "icon-wallet",
		CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	if err := repo.Save(ctx, acc); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != id || got.UserID.String() != userA || got.CurrencyID.String() != usdID {
		t.Errorf("ids mismatch: %+v", got)
	}
	if got.Name != "Wallet" || got.Type != model.TypeCash || got.Icon != "icon-wallet" {
		t.Errorf("fields mismatch: name=%q type=%d icon=%q", got.Name, got.Type, got.Icon)
	}
	if got.IsDeleted {
		t.Error("IsDeleted should be false")
	}
	if !got.CreatedAt.Equal(fixedTime) || !got.UpdatedAt.Equal(fixedTime) {
		t.Errorf("timestamps mismatch: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}
}

func TestAccountRepo_GetByID_NotFound(t *testing.T) {
	repo, _, f := newAccountRepo(t)
	seedUser(t, f, userA, "A")
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestAccountRepo_ListAndCountAvailable(t *testing.T) {
	repo, _, f := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA, "A")
	seedAccount(t, f, acctCash, userA, "Cash")
	seedAccount(t, f, acctBank, userA, "Bank")
	// A deleted account must be excluded.
	f.Account(fixture.Account{ID: acctOther, UserID: userA, CurrencyID: usdID, Name: "Gone", Type: 2, Icon: "x", Deleted: true})

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
	repo, _, f := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA, "A")
	seedAccount(t, f, acctCash, userA, "Cash")
	seedAccount(t, f, acctBank, userA, "Bank")

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
	repo, _, f := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA, "A")
	seedAccount(t, f, acctCash, userA, "Cash")

	// Three income transactions that float-sum with drift but must render clean.
	seedTx := func(id, amt string) {
		f.Transaction(fixture.Transaction{ID: id, UserID: userA, AccountID: acctCash, Type: 1, Amount: amt, SpentAt: "2024-03-01 00:00:00"})
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
	seedAccount(t, f, acctBank, userA, "Bank")
	bal2, err := repo.Balance(ctx, vo.MustParseId(acctBank), before)
	if err != nil {
		t.Fatalf("Balance empty: %v", err)
	}
	if bal2 != "0" {
		t.Errorf("want empty balance 0, got %q", bal2)
	}
}

func TestAccountRepo_Balances_NegativeZeroNormalized(t *testing.T) {
	repo, _, f := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA, "A")
	seedAccount(t, f, acctCash, userA, "Cash")
	f.AccountOption(acctCash, userA, 0)

	// An income then an equal expense -> net 0 (must not be "-0").
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctCash, Type: 1, Amount: "50.00", SpentAt: "2024-03-01 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctCash, Type: 0, Amount: "50.00", SpentAt: "2024-03-01 00:00:00"})

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
	repo, db, f := newAccountRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA, "A")
	seedAccount(t, f, acctCash, userA, "Cash")

	corrID := vo.NewId()
	c := model.AccountCorrection{
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
