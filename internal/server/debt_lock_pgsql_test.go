//go:build enginecompare

package server_test

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
	appcategory "github.com/econumo/econumo/internal/category"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	domcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	appdebt "github.com/econumo/econumo/internal/debt"
	debtrepo "github.com/econumo/econumo/internal/debt/repo"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/model"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

type debtLockClock struct{}

func (debtLockClock) Now() time.Time { return time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC) }

func fireHook(h *func()) {
	if *h != nil {
		fire := *h
		*h = nil
		fire()
	}
}

// debtLockDebtRepo pauses a debt write inside its open transaction.
type debtLockDebtRepo struct {
	appdebt.Repository
	afterLockCategories func()
}

func (r *debtLockDebtRepo) LockCategories(ctx context.Context, ids []vo.Id) error {
	err := r.Repository.LockCategories(ctx, ids)
	if err == nil {
		fireHook(&r.afterLockCategories)
	}
	return err
}

// debtLockPair is the budget, debt and category services on one connection,
// wired with the production glue.
type debtLockPair struct {
	debtRepo   *debtLockDebtRepo
	budget     *appbudget.Service
	debts      *appdebt.Service
	categories *appcategory.Service
	pid        int
}

func newDebtLockPair(t *testing.T, tdb *dbtest.DB) *debtLockPair {
	t.Helper()
	eng, txm := tdb.Engine, tdb.TX
	clk := debtLockClock{}
	userRepo := userrepo.NewRepo(eng, txm)
	categoryRepo := categoryrepo.NewRepo(eng, txm)
	payeeRepo := payeerepo.NewRepo(eng, txm)
	tagRepo := tagrepo.NewRepo(eng, txm)
	currencyLookup := currencyrepo.New(eng, txm)
	rateProvider := currencyrepo.NewRateProvider(eng, txm, currencyLookup, fixture.USD)
	convertor := domcurrency.NewConvertor(rateProvider)
	accessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(eng, txm))

	p := &debtLockPair{debtRepo: &debtLockDebtRepo{Repository: debtrepo.NewRepo(eng, txm)}}
	p.budget = appbudget.NewService(
		budgetrepo.NewRepo(eng, txm), budgetrepo.NewReadRepo(eng, txm), convertor, rateProvider,
		server.NewBudgetUserLookup(userRepo, clk),
		server.NewBudgetAccountLookup(accountrepo.NewRepo(eng, txm)),
		server.NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(server.NewBudgetCategoryMetadataLookup(categoryRepo), server.NewBudgetTagMetadataLookup(tagRepo), server.NewBudgetPayeeMetadataLookup(payeeRepo)),
		accessResolver, operationrepo.NewGuard(eng, txm), txm, clk,
	)
	p.debts = appdebt.NewService(p.debtRepo, txm, clk,
		server.NewDebtCategories(categoryRepo), server.NewDebtRates(rateProvider, convertor), server.NewDebtUsers(userRepo))
	p.debts.SetBudgets(server.NewDebtBudgets(p.budget))
	p.debts.SetElements(server.NewDebtElements(p.budget))
	p.budget.SetDebts(p.debts)
	p.categories = appcategory.NewService(categoryRepo, txm, operationrepo.NewGuard(eng, txm), clk, categoryrepo.NewReadRepo(eng, txm), accessResolver, nil)
	p.categories.SetDebtUsage(server.NewDebtUsage(p.debts))
	if err := tdb.Raw.QueryRowContext(context.Background(), "SELECT pg_backend_pid()").Scan(&p.pid); err != nil {
		t.Fatalf("backend pid: %v", err)
	}
	return p
}

// debtLockRig is two service pairs on two real connections to one schema,
// plus a third connection that only watches pg_stat_activity. A single pool
// would serialize the transactions and hide the race.
type debtLockRig struct {
	db       *dbtest.DB
	monitor  *sql.DB
	one, two *debtLockPair
	owner    vo.Id
	budgetID string
	loan     string // an expense category
	car      string // another expense category
}

func newDebtLockRig(t *testing.T) *debtLockRig {
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
	db2 := &dbtest.DB{Raw: raw2, TX: backend.NewTxManager(raw2), Engine: db.Engine}

	f := fixture.New(t, db)
	owner := f.User(fixture.User{Name: "Owner", Email: "owner@debtlock.test"})
	cash := f.Account(fixture.Account{UserID: owner, Name: "Cash"})
	started := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	budgetID := f.Budget(fixture.Budget{UserID: owner, StartedAt: started})
	f.BudgetAccount(budgetID, cash)
	return &debtLockRig{
		db: db, monitor: open(), one: newDebtLockPair(t, db), two: newDebtLockPair(t, db2),
		owner: vo.MustParseId(owner), budgetID: budgetID,
		loan: f.Category(fixture.Category{UserID: owner, Name: "Loan"}),
		car:  f.Category(fixture.Category{UserID: owner, Name: "Car"}),
	}
}

func debtLockGate(t *testing.T) (wait <-chan struct{}, unblock func()) {
	ch := make(chan struct{})
	var once sync.Once
	unblock = func() { once.Do(func() { close(ch) }) }
	t.Cleanup(unblock)
	return ch, unblock
}

// waitLockWait polls until the backend pid waits on a lock. done reports the
// goroutine under watch finishing first, which means it never blocked.
func (r *debtLockRig) waitLockWait(t *testing.T, pid int, who string, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("%s finished (err=%v) while the other transaction was still open; it should have waited on a lock", who, err)
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

// pause runs fn in the background and returns once hook fired inside its
// transaction; the transaction stays open until unblock.
func pause(t *testing.T, install func(hook func()), fn func() error) (done <-chan error, unblock func()) {
	t.Helper()
	reached := make(chan struct{})
	release, unblock := debtLockGate(t)
	install(func() {
		close(reached)
		<-release
	})
	out := make(chan error, 1)
	go func() { out <- fn() }()
	select {
	case <-reached:
	case err := <-out:
		t.Fatalf("finished before its hook: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("never reached its hook")
	}
	return out, unblock
}

// create-debt holds a key-share lock on its category. A category delete
// arriving then read the category's debts before that debt existed, so it
// holds no lock on the debt's budget; it waits for the category row and, once
// the debt commits, fails with the retryable ErrDebtsChangedConcurrently
// rather than lock that budget after the row (a deadlock risk) or leave the
// debt on a deleted category. A retry deletes the debt with the category.
func TestCreateDebt_ConcurrentCategoryDeleteFailsRetryably(t *testing.T) {
	r := newDebtLockRig(t)
	ctx := context.Background()

	created, unblock := pause(t, func(h func()) { r.one.debtRepo.afterLockCategories = h },
		func() error {
			_, err := r.one.debts.CreateDebt(ctx, r.owner, model.CreateDebtRequest{BudgetId: r.budgetID, ElementId: r.car,
				Payment: vo.NewFlexString("100"), Schedule: "monthly"})
			return err
		})
	deleted := make(chan error, 1)
	go func() {
		_, err := r.two.categories.DeleteCategory(ctx, r.owner, model.DeleteCategoryRequest{Id: r.car})
		deleted <- err
	}()
	r.waitLockWait(t, r.two.pid, "delete-category", deleted)

	unblock()
	if err := <-created; err != nil {
		t.Fatalf("create-debt: %v", err)
	}
	if err := <-deleted; !errors.Is(err, appdebt.ErrDebtsChangedConcurrently) {
		t.Fatalf("delete-category = %v, want ErrDebtsChangedConcurrently", err)
	}
	state := func() (debts, categories int) {
		t.Helper()
		if err := r.monitor.QueryRow(`SELECT COUNT(*) FROM debts WHERE element_external_id = $1`, r.car).Scan(&debts); err != nil {
			t.Fatal(err)
		}
		if err := r.monitor.QueryRow(`SELECT COUNT(*) FROM categories WHERE id = $1`, r.car).Scan(&categories); err != nil {
			t.Fatal(err)
		}
		return debts, categories
	}
	if debts, categories := state(); debts != 1 || categories != 1 {
		t.Errorf("after the refused delete: %d debts, %d category rows; want both kept", debts, categories)
	}

	if _, err := r.two.categories.DeleteCategory(ctx, r.owner, model.DeleteCategoryRequest{Id: r.car}); err != nil {
		t.Fatalf("delete-category retry: %v", err)
	}
	if debts, categories := state(); debts != 0 || categories != 0 {
		t.Errorf("after the retry: %d debts, %d category rows; want both gone", debts, categories)
	}
}
