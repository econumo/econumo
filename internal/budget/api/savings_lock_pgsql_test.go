//go:build enginecompare

package api_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appbudget "github.com/econumo/econumo/internal/budget"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	domcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/model"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// hookedBudgetRepo pauses a use case inside its still-open transaction at a
// chosen repository call, so a second connection can run into its locks.
type hookedBudgetRepo struct {
	appbudget.Repository
	afterHasData    func()
	afterSaveLimit  func()
	afterInsertCmnt func()
}

func fireOnce(h *func()) {
	if *h != nil {
		fire := *h
		*h = nil
		fire()
	}
}

func (r *hookedBudgetRepo) SavingsElementHasData(ctx context.Context, budgetID, accountID vo.Id) (bool, error) {
	has, err := r.Repository.SavingsElementHasData(ctx, budgetID, accountID)
	if err == nil {
		fireOnce(&r.afterHasData)
	}
	return has, err
}

func (r *hookedBudgetRepo) SaveLimit(ctx context.Context, l *model.BudgetElementLimit) error {
	err := r.Repository.SaveLimit(ctx, l)
	if err == nil {
		fireOnce(&r.afterSaveLimit)
	}
	return err
}

func (r *hookedBudgetRepo) InsertComment(ctx context.Context, c *model.BudgetElementComment) error {
	err := r.Repository.InsertComment(ctx, c)
	if err == nil {
		fireOnce(&r.afterInsertCmnt)
	}
	return err
}

func newBudgetServiceOn(tdb *dbtest.DB, repo *hookedBudgetRepo) *appbudget.Service {
	eng, txm := tdb.Engine, tdb.TX
	clk := fixedAugust()
	currencyLookup := currencyrepo.New(eng, txm)
	rateProvider := currencyrepo.NewRateProvider(eng, txm, currencyLookup, usdID)
	repo.Repository = budgetrepo.NewRepo(eng, txm)
	return appbudget.NewService(
		repo, budgetrepo.NewReadRepo(eng, txm), domcurrency.NewConvertor(rateProvider), rateProvider,
		server.NewBudgetUserLookup(userrepo.NewRepo(eng, txm), clk),
		server.NewBudgetAccountLookup(accountrepo.NewRepo(eng, txm)),
		server.NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(
			server.NewBudgetCategoryMetadataLookup(categoryrepo.NewRepo(eng, txm)),
			server.NewBudgetTagMetadataLookup(tagrepo.NewRepo(eng, txm)),
			server.NewBudgetPayeeMetadataLookup(payeerepo.NewRepo(eng, txm))),
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(eng, txm)),
		operationrepo.NewGuard(eng, txm),
		txm, clk,
	)
}

// savingsLockRig is two services on two real connections to one schema, plus
// a third connection that only watches pg_stat_activity. A single pool would
// serialize the transactions and hide the race.
type savingsLockRig struct {
	db1, db2         *dbtest.DB
	monitor          *sql.DB
	repo1, repo2     *hookedBudgetRepo
	svc1, svc2       *appbudget.Service
	pid1, pid2       int
	userID, budgetID vo.Id
}

func newSavingsLockRig(t *testing.T) *savingsLockRig {
	t.Helper()
	db := dbtest.New(t)
	if db.Engine != "postgresql" {
		t.Skipf("the race needs two real connections; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
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
	r := &savingsLockRig{
		db1: db, db2: &dbtest.DB{Raw: raw2, TX: backend.NewTxManager(raw2), Engine: db.Engine},
		monitor: open(), repo1: &hookedBudgetRepo{}, repo2: &hookedBudgetRepo{},
		userID: vo.MustParseId(seedUserID), budgetID: vo.MustParseId(budgetID1),
	}
	r.svc1 = newBudgetServiceOn(r.db1, r.repo1)
	r.svc2 = newBudgetServiceOn(r.db2, r.repo2)
	for _, p := range []struct {
		raw *sql.DB
		pid *int
	}{{r.db1.Raw, &r.pid1}, {r.db2.Raw, &r.pid2}} {
		if err := p.raw.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(p.pid); err != nil {
			t.Fatalf("backend pid: %v", err)
		}
	}

	f := fixture.New(t, db).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: seedUserID, Email: seedEmail, Name: seedName, Avatar: seedAvatar, Password: "pw", Salt: seedSalt})
	f.Option(seedUserID, "budget", nil)
	f.Account(fixture.Account{ID: accountID, UserID: seedUserID, CurrencyID: usdID, Name: "Cash"})
	f.Account(fixture.Account{ID: accountID2, UserID: seedUserID, CurrencyID: usdID, Name: "Savings"})
	if _, err := r.svc1.CreateBudget(ctx, r.userID, model.CreateBudgetRequest{
		Id: budgetID1, Name: "Budget", StartDate: "2026-06-01", CurrencyId: usdID,
		AccountIds: []string{accountID, accountID2}, SavingsAccountIds: []string{accountID2},
	}); err != nil {
		t.Fatalf("CreateBudget: %v", err)
	}
	return r
}

// gate is a one-shot release. It is also closed at cleanup, so a failed
// assertion never leaves a paused transaction holding its pool's only
// connection while the schema drop waits on it.
func gate(t *testing.T) (wait <-chan struct{}, unblock func()) {
	ch := make(chan struct{})
	var once sync.Once
	unblock = func() { once.Do(func() { close(ch) }) }
	t.Cleanup(unblock)
	return ch, unblock
}

// waitLockWait polls until the backend pid is waiting on a lock. done reports
// the goroutine under watch finishing first, which means it never blocked.
func (r *savingsLockRig) waitLockWait(t *testing.T, pid int, who string, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("%s finished (err=%v) while the other transaction was still open; it should have waited on the element lock", who, err)
		default:
		}
		var wait sql.NullString
		if err := r.monitor.QueryRow(`SELECT wait_event_type FROM pg_stat_activity WHERE pid = $1`, pid).Scan(&wait); err != nil {
			t.Fatalf("pg_stat_activity: %v", err)
		}
		if wait.String == "Lock" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never waited on a lock within 5s", who)
}

func (r *savingsLockRig) count(t *testing.T, query string) int {
	t.Helper()
	var n int
	if err := r.monitor.QueryRow(query, budgetID1, accountID2).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *savingsLockRig) savingsData(t *testing.T) (elements, limits, comments int) {
	t.Helper()
	elements = r.count(t, `SELECT COUNT(*) FROM budgets_elements WHERE budget_id = $1 AND external_id = $2 AND type = 5`)
	limits = r.count(t, `SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id
		WHERE e.budget_id = $1 AND e.external_id = $2 AND e.type = 5`)
	comments = r.count(t, `SELECT COUNT(*) FROM budgets_elements_comments c JOIN budgets_elements e ON e.id = c.element_id
		WHERE e.budget_id = $1 AND e.external_id = $2 AND e.type = 5`)
	return
}

// savingsWriter is one of the two requests that add data to a savings row, run
// on a given service.
type savingsWriter struct {
	name  string
	run   func(ctx context.Context, svc *appbudget.Service, userID vo.Id) error
	pause func(repo *hookedBudgetRepo, hook func())
}

var savingsWriters = []savingsWriter{
	{
		name: "set-limit",
		run: func(ctx context.Context, svc *appbudget.Service, userID vo.Id) error {
			amount := vo.NewFlexString("75")
			_, err := svc.SetLimit(ctx, userID, model.SetLimitRequest{
				BudgetId: budgetID1, ElementId: accountID2, Period: "2026-08-01", Amount: &amount,
			})
			return err
		},
		pause: func(repo *hookedBudgetRepo, hook func()) { repo.afterSaveLimit = hook },
	},
	{
		name: "create-comment",
		run: func(ctx context.Context, svc *appbudget.Service, userID vo.Id) error {
			_, err := svc.CreateComment(ctx, userID, model.CreateCommentRequest{
				Id: vo.NewId().String(), BudgetId: budgetID1, ElementId: accountID2, Period: "2026-08-01", Comment: "for the trip",
			})
			return err
		},
		pause: func(repo *hookedBudgetRepo, hook func()) { repo.afterInsertCmnt = hook },
	},
}

// The remover takes the savings row first: its guard found nothing, and while
// its transaction is open a limit or comment arrives for the same row. The
// writer must wait on the row, then find it gone and fail cleanly. Without the
// locks it succeeded, and the removal's cascade then deleted what it wrote.
func TestSavingsRemoval_RemoverLockHoldsOffAWriter(t *testing.T) {
	for _, w := range savingsWriters {
		t.Run(w.name, func(t *testing.T) {
			r := newSavingsLockRig(t)
			ctx := context.Background()

			checked := make(chan struct{})
			release, unblock := gate(t)
			r.repo1.afterHasData = func() {
				close(checked)
				<-release
			}
			removed := make(chan error, 1)
			go func() {
				_, err := r.svc1.RemoveAccount(ctx, r.userID, model.RemoveAccountRequest{BudgetId: budgetID1, AccountId: accountID2})
				removed <- err
			}()
			select {
			case <-checked:
			case err := <-removed:
				t.Fatalf("remove-account finished before its guard ran: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("remove-account never reached its guard")
			}

			wrote := make(chan error, 1)
			go func() { wrote <- w.run(ctx, r.svc2, r.userID) }()
			r.waitLockWait(t, r.pid2, w.name, wrote)

			unblock()
			if err := <-removed; err != nil {
				t.Fatalf("remove-account: %v", err)
			}
			err := <-wrote
			var nf *errs.NotFoundError
			if !errors.As(err, &nf) {
				t.Fatalf("%s after the removal committed = %v, want a not-found error", w.name, err)
			}
			if el, lim, cm := r.savingsData(t); el != 0 || lim != 0 || cm != 0 {
				t.Fatalf("savings element/limits/comments = %d/%d/%d, want 0/0/0", el, lim, cm)
			}
		})
	}
}

// The writer takes the savings row first and pauses after its insert. A
// removal arriving now must wait for it and then see the new data, so it asks
// for confirmation instead of cascading the write away.
func TestSavingsRemoval_WriterLockMakesTheGuardSeeTheWrite(t *testing.T) {
	for _, w := range savingsWriters {
		t.Run(w.name, func(t *testing.T) {
			r := newSavingsLockRig(t)
			ctx := context.Background()

			inserted := make(chan struct{})
			release, unblock := gate(t)
			w.pause(r.repo2, func() {
				close(inserted)
				<-release
			})
			wrote := make(chan error, 1)
			go func() { wrote <- w.run(ctx, r.svc2, r.userID) }()
			select {
			case <-inserted:
			case err := <-wrote:
				t.Fatalf("%s finished before its insert hook: %v", w.name, err)
			case <-time.After(5 * time.Second):
				t.Fatalf("%s never reached its insert", w.name)
			}

			removed := make(chan error, 1)
			go func() {
				_, err := r.svc1.RemoveAccount(ctx, r.userID, model.RemoveAccountRequest{BudgetId: budgetID1, AccountId: accountID2})
				removed <- err
			}()
			r.waitLockWait(t, r.pid1, "remove-account", removed)

			unblock()
			if err := <-wrote; err != nil {
				t.Fatalf("%s: %v", w.name, err)
			}
			err := <-removed
			var ve *errs.ValidationError
			if !errors.As(err, &ve) || len(ve.Fields) != 1 || ve.Fields[0].Key != "confirmSavingsRemoval" {
				t.Fatalf("remove-account after the write committed = %v, want the confirmSavingsRemoval refusal", err)
			}
			el, lim, cm := r.savingsData(t)
			if el != 1 || lim+cm != 1 {
				t.Fatalf("savings element/limits/comments = %d/%d/%d, want the row and the write kept", el, lim, cm)
			}
		})
	}
}
