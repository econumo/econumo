package account_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

type keepBalanceEnv struct {
	db  *dbtest.DB
	f   *fixture.Builder
	svc *appaccount.Service
	ctx context.Context
	// today is the current UTC wall clock, whole seconds.
	today   time.Time
	folders map[string]string
}

func newKeepBalanceEnv(t *testing.T) *keepBalanceEnv {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: accessOwnerID, Name: "Owner"})
	f.User(fixture.User{ID: accessUserBID, Name: "UserB"})
	return &keepBalanceEnv{db: db, f: f, svc: newAccessSvc(t, db), ctx: context.Background(), folders: map[string]string{}, today: time.Now().UTC().Truncate(time.Second)}
}

func (e *keepBalanceEnv) createAccount(t *testing.T, userID, balance string) (string, string) {
	t.Helper()
	res, err := e.svc.CreateAccount(e.ctx, mustParseID(t, userID), model.CreateAccountRequest{
		Id: vo.NewId().String(), Name: "Cash", CurrencyId: fixture.USD, Balance: vo.NewFlexString(balance), Icon: "wallet",
		FolderId: e.folders[userID],
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if res.Item.FolderId != nil {
		e.folders[userID] = *res.Item.FolderId
	}
	corrID := ""
	if res.Transaction != nil {
		corrID = res.Transaction.Id
	}
	return res.Item.Id, corrID
}

func (e *keepBalanceEnv) snapshot(t *testing.T, userID string, ids ...string) *model.BalanceSnapshotResult {
	t.Helper()
	res, err := e.svc.GetBalanceSnapshot(e.ctx, mustParseID(t, userID), ids)
	if err != nil {
		t.Fatalf("GetBalanceSnapshot: %v", err)
	}
	return res
}

// imported inserts a row as an import would: created after the snapshot.
func (e *keepBalanceEnv) imported(t *testing.T, accountID string, typ int, amount string, spentAt time.Time) string {
	t.Helper()
	e.f.At(e.today.Add(time.Hour))
	return e.f.Transaction(fixture.Transaction{UserID: accessOwnerID, AccountID: accountID, Type: typ, Amount: amount, SpentAt: spentAt})
}

func (e *keepBalanceEnv) revision(t *testing.T, accountID string) int64 {
	t.Helper()
	var rev int64
	if err := e.db.Raw.QueryRow(e.db.Rebind(`SELECT transactions_revision FROM accounts WHERE id = ?`), accountID).Scan(&rev); err != nil {
		t.Fatalf("read revision: %v", err)
	}
	return rev
}

type txRow struct {
	typ         int
	amount      string
	spentAt     time.Time
	description string
	categoryID  sql.NullString
}

func (e *keepBalanceEnv) row(t *testing.T, id string) (txRow, bool) {
	t.Helper()
	var r txRow
	err := e.db.Raw.QueryRow(e.db.Rebind(`SELECT type, amount, spent_at, description, category_id FROM transactions WHERE id = ?`), id).
		Scan(&r.typ, &r.amount, &r.spentAt, &r.description, &r.categoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false
	}
	if err != nil {
		t.Fatalf("read transaction %s: %v", id, err)
	}
	r.amount = vo.NewDecimal(r.amount).String()
	r.spentAt = r.spentAt.UTC()
	return r, true
}

func (e *keepBalanceEnv) countRows(t *testing.T, accountID string) int {
	t.Helper()
	var n int
	if err := e.db.Raw.QueryRow(e.db.Rebind(`SELECT COUNT(*) FROM transactions WHERE account_id = ?`), accountID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func itemFor(t *testing.T, res *model.BalanceSnapshotResult, id string) model.BalanceSnapshotItem {
	t.Helper()
	for _, it := range res.Items {
		if it.AccountId == id {
			return it
		}
	}
	t.Fatalf("snapshot has no item for %s", id)
	return model.BalanceSnapshotItem{}
}

func TestBalanceSnapshotFreshStates(t *testing.T) {
	e := newKeepBalanceEnv(t)
	withBalance, corrID := e.createAccount(t, accessOwnerID, "100")
	empty, _ := e.createAccount(t, accessOwnerID, "0")
	used, _ := e.createAccount(t, accessOwnerID, "0")
	cat := e.f.Category(fixture.Category{UserID: accessOwnerID, Name: "Food"})
	e.f.Transaction(fixture.Transaction{UserID: accessOwnerID, AccountID: used, CategoryID: cat, Type: 0, Amount: "5"})
	foreign := e.f.Account(fixture.Account{UserID: accessUserBID})

	res := e.snapshot(t, accessOwnerID, withBalance, empty, used, foreign)
	if _, err := time.Parse(datetime.Layout, res.TakenAt); err != nil {
		t.Fatalf("takenAt %q: %v", res.TakenAt, err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("items = %d, want 3 (foreign omitted): %+v", len(res.Items), res.Items)
	}
	if got := itemFor(t, res, withBalance); !got.Fresh || got.OpeningCorrectionId != corrID || got.Balance != "100" {
		t.Errorf("with balance = %+v, want fresh, correction %s, balance 100", got, corrID)
	}
	if got := itemFor(t, res, empty); !got.Fresh || got.OpeningCorrectionId != "" || got.Balance != "0" {
		t.Errorf("empty = %+v, want fresh, no correction, balance 0", got)
	}
	if got := itemFor(t, res, used); got.Fresh || got.OpeningCorrectionId != "" || got.Balance != "-5" {
		t.Errorf("used = %+v, want not fresh, no correction, balance -5", got)
	}
}

func (e *keepBalanceEnv) restore(t *testing.T, userID string, req model.RestoreBalanceRequest) *model.RestoreBalanceResult {
	t.Helper()
	res, err := e.svc.RestoreBalance(e.ctx, mustParseID(t, userID), req)
	if err != nil {
		t.Fatalf("RestoreBalance: %v", err)
	}
	return res
}

func TestRestoreMovesOpeningCorrection(t *testing.T) {
	e := newKeepBalanceEnv(t)
	acct, corrID := e.createAccount(t, accessOwnerID, "100")
	snap := e.snapshot(t, accessOwnerID, acct)
	tenDaysAgo := e.today.AddDate(0, 0, -10)
	e.imported(t, acct, 0, "30", tenDaysAgo)
	before := e.revision(t, acct)

	res := e.restore(t, accessOwnerID, model.RestoreBalanceRequest{AccountId: acct, Balance: "100", OpeningCorrectionId: corrID, Since: snap.TakenAt})
	if !res.Restored || res.Item.Balance != "100" {
		t.Fatalf("restore = restored %v balance %s, want true 100", res.Restored, res.Item.Balance)
	}
	got, ok := e.row(t, corrID)
	if !ok {
		t.Fatalf("opening correction gone")
	}
	if got.typ != 1 || got.amount != "130" || !got.spentAt.Equal(tenDaysAgo.Add(-time.Second)) {
		t.Errorf("correction = %+v, want income 130 at %s", got, tenDaysAgo.Add(-time.Second))
	}
	if after := e.revision(t, acct); after <= before {
		t.Errorf("revision %d -> %d, want it to move", before, after)
	}
	if res.Item.TransactionsRevision != e.revision(t, acct) {
		t.Errorf("item revision %d, want the stored one", res.Item.TransactionsRevision)
	}
}

func TestRestoreDeletesZeroedCorrection(t *testing.T) {
	e := newKeepBalanceEnv(t)
	acct, corrID := e.createAccount(t, accessOwnerID, "30")
	snap := e.snapshot(t, accessOwnerID, acct)
	e.imported(t, acct, 1, "30", e.today.AddDate(0, 0, -3))

	res := e.restore(t, accessOwnerID, model.RestoreBalanceRequest{AccountId: acct, Balance: "30", OpeningCorrectionId: corrID, Since: snap.TakenAt})
	if !res.Restored || res.Item.Balance != "30" {
		t.Fatalf("restore = restored %v balance %s, want true 30", res.Restored, res.Item.Balance)
	}
	if _, ok := e.row(t, corrID); ok {
		t.Errorf("zeroed opening correction still exists")
	}
	if n := e.countRows(t, acct); n != 1 {
		t.Errorf("rows = %d, want only the imported one", n)
	}
}

func TestRestoreAddsCorrectionWhenNotFresh(t *testing.T) {
	e := newKeepBalanceEnv(t)
	acct, _ := e.createAccount(t, accessOwnerID, "0")
	cat := e.f.Category(fixture.Category{UserID: accessOwnerID, Name: "Food"})
	old := e.f.Transaction(fixture.Transaction{UserID: accessOwnerID, AccountID: acct, CategoryID: cat, Type: 1, Amount: "50", SpentAt: e.today.AddDate(0, 0, -1)})
	oldBefore, _ := e.row(t, old)
	snap := e.snapshot(t, accessOwnerID, acct)
	fiveDaysAgo := e.today.AddDate(0, 0, -5)
	e.imported(t, acct, 0, "20", fiveDaysAgo)
	e.imported(t, acct, 0, "5", e.today.AddDate(0, 0, -2))

	res := e.restore(t, accessOwnerID, model.RestoreBalanceRequest{AccountId: acct, Balance: "50", Since: snap.TakenAt})
	if !res.Restored || res.Item.Balance != "50" {
		t.Fatalf("restore = restored %v balance %s, want true 50", res.Restored, res.Item.Balance)
	}
	var id string
	if err := e.db.Raw.QueryRow(e.db.Rebind(`SELECT id FROM transactions WHERE account_id = ? AND description = ?`), acct, "Balance adjustment").Scan(&id); err != nil {
		t.Fatalf("find new correction: %v", err)
	}
	got, _ := e.row(t, id)
	if got.typ != 1 || got.amount != "25" || !got.spentAt.Equal(fiveDaysAgo.Add(-time.Second)) {
		t.Errorf("new correction = %+v, want income 25 at %s", got, fiveDaysAgo.Add(-time.Second))
	}
	if oldAfter, _ := e.row(t, old); oldAfter != oldBefore {
		t.Errorf("existing row changed: %+v -> %+v", oldBefore, oldAfter)
	}
}

func TestRestoreEditedOpeningCorrectionIsNotMoved(t *testing.T) {
	e := newKeepBalanceEnv(t)
	acct, corrID := e.createAccount(t, accessOwnerID, "100")
	snap := e.snapshot(t, accessOwnerID, acct)
	cat := e.f.Category(fixture.Category{UserID: accessOwnerID, Name: "Salary", Type: 1})
	e.db.Exec(t, e.db.Rebind(`UPDATE transactions SET category_id = ? WHERE id = ?`), cat, corrID)
	corrBefore, _ := e.row(t, corrID)
	tenDaysAgo := e.today.AddDate(0, 0, -10)
	e.imported(t, acct, 0, "30", tenDaysAgo)

	res := e.restore(t, accessOwnerID, model.RestoreBalanceRequest{AccountId: acct, Balance: "100", OpeningCorrectionId: corrID, Since: snap.TakenAt})
	if !res.Restored || res.Item.Balance != "100" {
		t.Fatalf("restore = restored %v balance %s, want true 100", res.Restored, res.Item.Balance)
	}
	if corrAfter, _ := e.row(t, corrID); corrAfter != corrBefore {
		t.Errorf("edited correction changed: %+v -> %+v", corrBefore, corrAfter)
	}
	if n := e.countRows(t, acct); n != 3 {
		t.Errorf("rows = %d, want 3 (opening, imported, new correction)", n)
	}
}

func TestRestoreNoImportedRowsIsNoop(t *testing.T) {
	e := newKeepBalanceEnv(t)
	acct, corrID := e.createAccount(t, accessOwnerID, "100")
	snap := e.snapshot(t, accessOwnerID, acct)
	before := e.revision(t, acct)

	res := e.restore(t, accessOwnerID, model.RestoreBalanceRequest{AccountId: acct, Balance: "150", OpeningCorrectionId: corrID, Since: snap.TakenAt})
	if res.Restored || res.Item.Balance != "100" {
		t.Fatalf("restore = restored %v balance %s, want false 100", res.Restored, res.Item.Balance)
	}
	if after := e.revision(t, acct); after != before {
		t.Errorf("revision %d -> %d, want unchanged", before, after)
	}
	if n := e.countRows(t, acct); n != 1 {
		t.Errorf("rows = %d, want 1", n)
	}
}

func TestRestoreUnchangedBalanceIsNoop(t *testing.T) {
	e := newKeepBalanceEnv(t)
	acct, corrID := e.createAccount(t, accessOwnerID, "100")
	snap := e.snapshot(t, accessOwnerID, acct)
	e.imported(t, acct, 0, "0", e.today.AddDate(0, 0, -1))

	res := e.restore(t, accessOwnerID, model.RestoreBalanceRequest{AccountId: acct, Balance: "100", OpeningCorrectionId: corrID, Since: snap.TakenAt})
	if res.Restored {
		t.Fatalf("restored = true, want false for a zero delta")
	}
}

func TestRestoreRequiresWriteAccess(t *testing.T) {
	e := newKeepBalanceEnv(t)
	acct, corrID := e.createAccount(t, accessOwnerID, "100")
	snap := e.snapshot(t, accessOwnerID, acct)
	e.imported(t, acct, 0, "30", e.today.AddDate(0, 0, -10))

	_, err := e.svc.RestoreBalance(e.ctx, mustParseID(t, accessUserBID), model.RestoreBalanceRequest{AccountId: acct, Balance: "100", OpeningCorrectionId: corrID, Since: snap.TakenAt})
	var denied *errs.AccessDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("err = %v, want access denied", err)
	}
	if got, _ := e.row(t, corrID); got.amount != "100" {
		t.Errorf("correction amount = %s, want untouched 100", got.amount)
	}
	if res := e.snapshot(t, accessUserBID, acct); len(res.Items) != 0 {
		t.Errorf("snapshot for a stranger = %+v, want empty", res.Items)
	}
}
