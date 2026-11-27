//go:build enginecompare

package wipetest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// hookedWiper pauses the wipe inside its open transaction: before Erase is
// right after the household check, after Erase is with every lock held and
// everything but the users row gone.
type hookedWiper struct {
	real        *server.UserDataWiper
	beforeErase func()
	afterErase  func()
}

func (w *hookedWiper) LockBudgets(ctx context.Context, userID vo.Id) error {
	return w.real.LockBudgets(ctx, userID)
}

func (w *hookedWiper) Erase(ctx context.Context, userID vo.Id) error {
	if w.beforeErase != nil {
		w.beforeErase()
	}
	if err := w.real.Erase(ctx, userID); err != nil {
		return err
	}
	if w.afterErase != nil {
		w.afterErase()
	}
	return nil
}

// lockRig is the production graph twice, on two real connections to one
// schema (one runs the wipe, the other the competing request), plus a third
// connection that only watches pg_stat_activity.
type lockRig struct {
	db      *dbtest.DB
	monitor *sql.DB
	one     *server.Built
	two     *server.Built
	pid1    int
	pid2    int
}

func newLockRig(t *testing.T) *lockRig {
	t.Helper()
	db := dbtest.New(t)
	if db.Engine != "postgresql" {
		t.Skipf("the interleavings need two real connections; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
	}
	ctx := context.Background()
	var schema string
	if err := db.Raw.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatalf("read current_schema: %v", err)
	}
	open := func() *sql.DB {
		raw, err := pgsql.OpenDB(os.Getenv(dbtest.PgsqlURLEnv))
		if err != nil {
			t.Fatalf("open pool: %v", err)
		}
		t.Cleanup(func() { _ = raw.Close() })
		raw.SetMaxOpenConns(1)
		if _, err := raw.ExecContext(ctx, fmt.Sprintf(`SET search_path TO %q`, schema)); err != nil {
			t.Fatalf("set search_path: %v", err)
		}
		return raw
	}
	raw2 := open()
	r := &lockRig{db: db, monitor: open(), one: Build(t, db),
		two: Build(t, &dbtest.DB{Raw: raw2, TX: backend.NewTxManager(raw2), Engine: db.Engine})}
	if err := db.Raw.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&r.pid1); err != nil {
		t.Fatalf("backend pid: %v", err)
	}
	if err := raw2.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&r.pid2); err != nil {
		t.Fatalf("backend pid: %v", err)
	}
	return r
}

// gate is a pause the test releases; the cleanup releases it too, and runs
// before the schema drop registered earlier, so a failing test never leaves
// the wipe parked on its locks.
func gate(t *testing.T) (wait func(), release func()) {
	ch := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(ch) }) }
	t.Cleanup(release)
	return func() { <-ch }, release
}

// startWipe runs the wipe of userID on the first connection and returns once
// install's hook fired inside its transaction.
func (r *lockRig) startWipe(t *testing.T, userID string, install func(w *hookedWiper, hook func())) (done <-chan wipeResult, release func()) {
	t.Helper()
	reached := make(chan struct{})
	wait, release := gate(t)
	w := &hookedWiper{real: r.one.Wiper}
	install(w, func() {
		close(reached)
		wait()
	})
	r.one.Users.SetDataWiper(w)
	out := make(chan wipeResult, 1)
	go func() {
		o, err := r.one.Users.Wipe(context.Background(), vo.MustParseId(userID), nil)
		out <- wipeResult{o, err}
	}()
	select {
	case <-reached:
	case res := <-out:
		t.Fatalf("the wipe finished before its pause: %q, %v", res.outcome, res.err)
	case <-time.After(10 * time.Second):
		t.Fatal("the wipe never reached its pause")
	}
	return out, release
}

type wipeResult struct {
	outcome appuser.WipeOutcome
	err     error
}

// second runs fn on the second connection with a 10 s bound.
func second(fn func(ctx context.Context) error) <-chan error {
	out := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out <- fn(ctx)
	}()
	return out
}

// waitsOnLock polls until the second connection waits on a lock and returns
// what it waits for. done reports the call finishing first, i.e. never blocking.
func (r *lockRig) waitsOnLock(t *testing.T, who string, done <-chan error) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("%s finished (err=%v) while the wipe was still open; it should have waited on a lock", who, err)
		default:
		}
		var wait sql.NullString
		if err := r.monitor.QueryRow(`SELECT wait_event_type FROM pg_stat_activity WHERE pid = $1`, r.pid2).Scan(&wait); err != nil {
			t.Fatalf("pg_stat_activity: %v", err)
		}
		if wait.String == "Lock" {
			var blockedByWipe bool
			if err := r.monitor.QueryRow(`SELECT $2 = ANY(pg_blocking_pids($1))`, r.pid2, r.pid1).Scan(&blockedByWipe); err != nil {
				t.Fatalf("pg_blocking_pids: %v", err)
			}
			if !blockedByWipe {
				t.Fatalf("%s waits on a lock the wipe does not hold", who)
			}
			return r.describeWait(t)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never waited on a lock within 5s", who)
	return ""
}

func (r *lockRig) describeWait(t *testing.T) string {
	t.Helper()
	rows, err := r.monitor.Query(`SELECT l.locktype, COALESCE(c.relname, ''), l.mode, a.query
		FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid LEFT JOIN pg_class c ON c.oid = l.relation
		WHERE l.pid = $1 AND NOT l.granted`, r.pid2)
	if err != nil {
		t.Fatalf("pg_locks: %v", err)
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var locktype, rel, mode, query string
		if err := rows.Scan(&locktype, &rel, &mode, &query); err != nil {
			t.Fatalf("pg_locks: %v", err)
		}
		parts = append(parts, fmt.Sprintf("%s %s %s during %q", mode, locktype, rel, strings.Join(strings.Fields(query), " ")))
	}
	return strings.Join(parts, "; ")
}

func (r *lockRig) finish(t *testing.T, release func(), wipe <-chan wipeResult, other <-chan error, who string) error {
	t.Helper()
	release()
	select {
	case res := <-wipe:
		if res.err != nil || res.outcome != appuser.WipeDone {
			t.Fatalf("wipe = %q, %v; want %q", res.outcome, res.err, appuser.WipeDone)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the wipe hung after its release")
	}
	select {
	case err := <-other:
		if err != nil && strings.Contains(err.Error(), "deadlock") {
			t.Fatalf("%s ended in a deadlock: %v", who, err)
		}
		return err
	case <-time.After(10 * time.Second):
		t.Fatalf("%s hung after the wipe committed", who)
	}
	return nil
}

type pair struct {
	u, p, uEmail, uCash, pCash string
}

func seedPair(t *testing.T, db *dbtest.DB) pair {
	t.Helper()
	f := fixture.New(t, db)
	var s pair
	s.uEmail = "u@lock.test"
	s.u = f.User(fixture.User{Email: s.uEmail, Name: "U"})
	s.p = f.User(fixture.User{Email: "p@lock.test", Name: "P"})
	f.Connect(s.u, s.p)
	s.uCash = f.Account(fixture.Account{UserID: s.u, Name: "U cash"})
	s.pCash = f.Account(fixture.Account{UserID: s.p, Name: "P cash"})
	f.AccountAccess(s.uCash, s.p, 1)
	f.AccountAccess(s.pCash, s.u, 1)
	f.Transaction(fixture.Transaction{UserID: s.p, AccountID: s.pCash, Type: 1, Amount: "1000", SpentAt: day(8, 1)})
	f.Transaction(fixture.Transaction{UserID: s.u, AccountID: s.uCash, Type: 1, Amount: "500", SpentAt: day(8, 1)})
	f.Transaction(fixture.Transaction{UserID: s.u, AccountID: s.uCash, Type: 2, Amount: "100",
		AccountRecipientID: s.pCash, AmountRecipient: "100", SpentAt: day(8, 2)})
	return s
}

func balanceOf(t *testing.T, db *dbtest.DB, accountID string) vo.DecimalNumber {
	t.Helper()
	b, _ := balances(t, db, []string{accountID}, []string{"all"})
	return b[accountID]["all"]
}

// A partner whose grant the wipe has removed but not committed still sees it:
// their transfer out of U's account must wait for the wipe (the account row
// is FOR UPDATE) and then fail on the missing account, never commit a
// transfer the cascade would delete with no correction.
func TestWipe_PartnerTransferWaits(t *testing.T) {
	r := newLockRig(t)
	s := seedPair(t, r.db)
	ctx := context.Background()
	if err := r.one.Users.AdminMarkDeleted(ctx, s.uEmail); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}
	before := balanceOf(t, r.db, s.pCash)

	wipe, release := r.startWipe(t, s.u, func(w *hookedWiper, hook func()) { w.afterErase = hook })
	other := second(func(ctx context.Context) error {
		recipient := s.pCash
		amount := vo.NewFlexString("40")
		_, err := r.two.Transactions.CreateTransaction(ctx, vo.MustParseId(s.p), model.CreateTransactionRequest{
			Type: "transfer", Amount: amount, AmountRecipient: &amount, AccountId: s.uCash, AccountRecipientId: &recipient,
			Date: "2026-08-10 10:00:00"})
		return err
	})
	t.Logf("partner transfer waits on: %s", r.waitsOnLock(t, "the partner's transfer", other))
	err := r.finish(t, release, wipe, other, "the partner's transfer")
	if err == nil {
		t.Fatal("the partner's transfer from U's account committed after the wipe")
	}
	t.Logf("partner transfer ended with: %v", err)
	if got := balanceOf(t, r.db, s.pCash); !got.Equals(before) {
		t.Fatalf("P's balance = %s, want the pre-wipe %s", got, before)
	}
	if count(t, r.db, `SELECT COUNT(*) FROM transactions WHERE amount = ? AND account_recipient_id = ?`, "40", s.pCash) != 0 {
		t.Fatal("the partner's transfer survived")
	}
}

// A request that authenticated as U before the mark still sees its grant on
// P's account: its insert, authored by U, must wait on U's users row (FOR
// UPDATE blocks the foreign-key check) and then fail, never commit and vanish
// with the cascade.
func TestWipe_InsertAuthoredByWipedUserWaits(t *testing.T) {
	r := newLockRig(t)
	s := seedPair(t, r.db)
	ctx := context.Background()
	if err := r.one.Users.AdminMarkDeleted(ctx, s.uEmail); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}
	before := balanceOf(t, r.db, s.pCash)

	wipe, release := r.startWipe(t, s.u, func(w *hookedWiper, hook func()) { w.afterErase = hook })
	other := second(func(ctx context.Context) error {
		_, err := r.two.Transactions.CreateTransaction(ctx, vo.MustParseId(s.u), model.CreateTransactionRequest{
			Type: "expense", Amount: vo.NewFlexString("25"), AccountId: s.pCash, Date: "2026-08-10 10:00:00"})
		return err
	})
	t.Logf("U's insert waits on: %s", r.waitsOnLock(t, "U's insert", other))
	err := r.finish(t, release, wipe, other, "U's insert")
	if err == nil {
		t.Fatal("an insert authored by the wiped user committed")
	}
	t.Logf("U's insert ended with: %v", err)
	if got := balanceOf(t, r.db, s.pCash); !got.Equals(before) {
		t.Fatalf("P's balance = %s, want the pre-wipe %s", got, before)
	}
	if count(t, r.db, `SELECT COUNT(*) FROM transactions WHERE account_id = ? AND amount = ?`, s.pCash, "25") != 0 {
		t.Fatal("U's insert survived")
	}
}

// For an inactive mark the connected users' rows are locked before the
// household check: P's sign-in, which writes P's row, cannot land between the
// check and the delete.
func TestWipe_InactivePartnerLoginWaits(t *testing.T) {
	r := newLockRig(t)
	ctx := context.Background()
	f := fixture.New(t, r.db).WithCrypto("")
	u := f.User(fixture.User{Email: "idle@lock.test", Name: "U"})
	const pEmail, pPassword = "partner@lock.test", "partner-secret"
	p := f.User(fixture.User{Email: pEmail, Name: "P", Password: pPassword})
	usd := fixture.USD
	f.Option(p, "currency", &usd)
	f.Connect(u, p)
	if n, err := userrepo.NewDeletionRepo(r.db.Engine, r.db.TX).Mark(ctx, vo.MustParseId(u), model.DeletionReasonInactive, Today); err != nil || n != 1 {
		t.Fatalf("Mark inactive = %d, %v", n, err)
	}

	wipe, release := r.startWipe(t, u, func(w *hookedWiper, hook func()) { w.beforeErase = hook })
	other := second(func(ctx context.Context) error {
		_, err := r.two.Users.Login(ctx, model.LoginRequest{Username: pEmail, Password: pPassword}, "test", Today)
		return err
	})
	t.Logf("P's login waits on: %s", r.waitsOnLock(t, "P's login", other))
	if err := r.finish(t, release, wipe, other, "P's login"); err != nil {
		t.Fatalf("P's login after the wipe: %v", err)
	}
	if count(t, r.db, `SELECT COUNT(*) FROM users WHERE id = ?`, u) != 0 {
		t.Fatal("U survived the wipe")
	}
}

// RevokeAccess takes the budget lock first, as the wipe does, so a partner
// revoking U mid-wipe waits instead of deadlocking.
func TestWipe_PartnerRevokeAccessWaits(t *testing.T) {
	r := newLockRig(t)
	s := seedPair(t, r.db)
	ctx := context.Background()
	f := fixture.New(t, r.db)
	pBudget := f.Budget(fixture.Budget{UserID: s.p, Name: "P budget", StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	f.BudgetAccess(pBudget, s.u, 1, true)
	f.BudgetAccount(pBudget, s.pCash)
	f.BudgetAccount(pBudget, s.uCash)
	uCat := f.Category(fixture.Category{UserID: s.u, Name: "U food"})
	f.BudgetElement(fixture.BudgetElement{BudgetID: pBudget, ExternalID: uCat, Type: 1})
	f.Option(s.u, "budget", &pBudget)
	if err := r.one.Users.AdminMarkDeleted(ctx, s.uEmail); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}

	wipe, release := r.startWipe(t, s.u, func(w *hookedWiper, hook func()) { w.afterErase = hook })
	other := second(func(ctx context.Context) error {
		_, err := r.two.Budgets.RevokeAccess(ctx, vo.MustParseId(s.p), model.RevokeAccessRequest{BudgetId: pBudget, UserId: s.u})
		return err
	})
	t.Logf("P's revoke waits on: %s", r.waitsOnLock(t, "P's revoke", other))
	err := r.finish(t, release, wipe, other, "P's revoke")
	t.Logf("P's revoke ended with: %v", err)
	if count(t, r.db, `SELECT COUNT(*) FROM users WHERE id = ?`, s.u) != 0 ||
		count(t, r.db, `SELECT COUNT(*) FROM budgets_access WHERE budget_id = ?`, pBudget) != 0 ||
		count(t, r.db, `SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ?`, pBudget) != 0 {
		t.Fatal("the wipe half-applied")
	}
	if count(t, r.db, `SELECT COUNT(*) FROM budgets WHERE id = ? AND user_id = ?`, pBudget, s.p) != 1 {
		t.Fatal("P's budget is gone")
	}
}

// hookedDeletion pauses the inactivity mark on its re-check: by then the mark
// holds the user row lock and has neither marked nor revoked anything.
type hookedDeletion struct {
	appuser.DeletionStore
	onRecheck func()
}

func (d *hookedDeletion) GetState(ctx context.Context, userID vo.Id) (model.DeletionState, error) {
	if d.onRecheck != nil {
		d.onRecheck()
	}
	return d.DeletionStore.GetState(ctx, userID)
}

// A request authenticating while the inactivity mark is open stamps the
// activity before touching its token: the stamp waits on the user row, the
// touch then finds the token revoked, and the request is refused. Touching
// first would commit the touch before the revoke and admit the request.
func TestMarkInactive_AuthenticateWaitsAndIsRefused(t *testing.T) {
	r := newLockRig(t)
	f := fixture.New(t, r.db).WithCrypto("").At(Today.AddDate(-2, 0, 0))
	u := f.User(fixture.User{Email: "idle-auth@lock.test", Name: "U"})
	const raw = "eco_ses_mark_wait"
	exp := Today.Add(24 * time.Hour)
	f.AccessToken(fixture.AccessToken{UserID: u, Kind: model.TokenKindSession, TokenHash: appuser.HashAccessToken(raw), ExpiresAt: &exp})

	reached := make(chan struct{})
	wait, release := gate(t)
	var once sync.Once
	store := &hookedDeletion{DeletionStore: userrepo.NewDeletionRepo(r.db.Engine, r.db.TX), onRecheck: func() {
		once.Do(func() {
			close(reached)
			wait()
		})
	}}
	r.one.Users.SetDeletion(store, userrepo.NewAuditRepo(r.db.Engine, r.db.TX),
		userrepo.NewDeletionRequestRepo(r.db.Engine, r.db.TX), appuser.DeletionConfig{InactiveMonths: 12})
	type markResult struct {
		n   int
		err error
	}
	mark := make(chan markResult, 1)
	go func() {
		n, err := r.one.Users.MarkInactive(context.Background())
		mark <- markResult{n, err}
	}()
	select {
	case <-reached:
	case res := <-mark:
		t.Fatalf("the mark finished before its pause: %d, %v", res.n, res.err)
	case <-time.After(10 * time.Second):
		t.Fatal("the mark never reached its pause")
	}

	other := second(func(ctx context.Context) error {
		_, err := r.two.Users.Authenticate(ctx, raw)
		return err
	})
	t.Logf("Authenticate waits on: %s", r.waitsOnLock(t, "Authenticate", other))
	release()
	select {
	case res := <-mark:
		if res.err != nil || res.n != 1 {
			t.Fatalf("MarkInactive = %d, %v; want 1 marked", res.n, res.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the mark hung after its release")
	}
	select {
	case err := <-other:
		var ue *errs.UnauthorizedError
		if !errors.As(err, &ue) {
			t.Fatalf("Authenticate after the mark = %v, want Unauthorized", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Authenticate hung after the mark committed")
	}
	if count(t, r.db, `SELECT COUNT(*) FROM users WHERE id = ? AND deletion_marked_at IS NOT NULL`, u) != 1 {
		t.Fatal("the user is not marked")
	}
}

// P disconnecting from U while U's wipe holds the budget locks: the disconnect
// drops the shared budget first, so it waits on that lock instead of holding
// the account grants the wipe deletes next.
func TestWipe_PartnerDisconnectDoesNotDeadlock(t *testing.T) {
	r := newLockRig(t)
	s := seedPair(t, r.db)
	ctx := context.Background()
	f := fixture.New(t, r.db)
	pBudget := f.Budget(fixture.Budget{UserID: s.p, Name: "P budget", StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	f.BudgetAccess(pBudget, s.u, 1, true)
	f.BudgetAccount(pBudget, s.pCash)
	f.BudgetAccount(pBudget, s.uCash)
	f.Option(s.u, "budget", &pBudget)
	const raw = "eco_ses_partner_disconnect"
	exp := Today.Add(24 * time.Hour)
	f.AccessToken(fixture.AccessToken{UserID: s.p, Kind: model.TokenKindSession, TokenHash: appuser.HashAccessToken(raw), ExpiresAt: &exp})
	if err := r.one.Users.AdminMarkDeleted(ctx, s.uEmail); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}

	wipe, release := r.startWipe(t, s.u, func(w *hookedWiper, hook func()) { w.beforeErase = hook })
	other := second(func(ctx context.Context) error {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/connection/delete-connection", strings.NewReader(`{"id":"`+s.u+`"}`))
		req.Header.Set("Authorization", "Bearer "+raw)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.two.API.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			return fmt.Errorf("delete-connection = %d: %s", rec.Code, rec.Body.String())
		}
		return nil
	})
	t.Logf("P's disconnect waits on: %s", r.waitsOnLock(t, "P's disconnect", other))
	// The API hides the driver error: a deadlock shows up as a 500 here.
	if err := r.finish(t, release, wipe, other, "P's disconnect"); err != nil {
		t.Fatalf("P's disconnect after the wipe: %v", err)
	}
	if count(t, r.db, `SELECT COUNT(*) FROM users WHERE id = ?`, s.u) != 0 ||
		count(t, r.db, `SELECT COUNT(*) FROM accounts_access WHERE user_id = ? OR account_id = ?`, s.u, s.uCash) != 0 ||
		count(t, r.db, `SELECT COUNT(*) FROM budgets_access WHERE budget_id = ?`, pBudget) != 0 {
		t.Fatal("the wipe half-applied")
	}
	if count(t, r.db, `SELECT COUNT(*) FROM budgets WHERE id = ? AND user_id = ?`, pBudget, s.p) != 1 {
		t.Fatal("P's budget is gone")
	}
}
