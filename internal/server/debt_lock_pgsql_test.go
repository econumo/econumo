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
	labelrepo "github.com/econumo/econumo/internal/label/repo"
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

type debtLockClock struct{}

func (debtLockClock) Now() time.Time { return time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC) }

// The race period: the clock's month, inside every debt's planning life.
const debtLockPeriod = "2026-08-01"

func fireHook(h *func()) {
	if *h != nil {
		fire := *h
		*h = nil
		fire()
	}
}

// debtLockBudgetRepo pauses a budget use case inside its open transaction.
type debtLockBudgetRepo struct {
	appbudget.Repository
	afterSaveLimit, afterLockElements func()
}

func (r *debtLockBudgetRepo) SaveLimit(ctx context.Context, l *model.BudgetElementLimit) error {
	err := r.Repository.SaveLimit(ctx, l)
	if err == nil {
		fireHook(&r.afterSaveLimit)
	}
	return err
}

func (r *debtLockBudgetRepo) LockElementsByExternal(ctx context.Context, externalID vo.Id, typ model.ElementType) error {
	err := r.Repository.LockElementsByExternal(ctx, externalID, typ)
	if err == nil {
		fireHook(&r.afterLockElements)
	}
	return err
}

// debtLockDebtRepo pauses a debt write inside its open transaction.
type debtLockDebtRepo struct {
	appdebt.Repository
	afterLockRuleCategories func()
}

func (r *debtLockDebtRepo) LockRuleCategories(ctx context.Context, ids []vo.Id) error {
	err := r.Repository.LockRuleCategories(ctx, ids)
	if err == nil {
		fireHook(&r.afterLockRuleCategories)
	}
	return err
}

// debtLockPair is the budget, debt and category services on one connection,
// wired with the production glue.
type debtLockPair struct {
	budgetRepo *debtLockBudgetRepo
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

	p := &debtLockPair{
		budgetRepo: &debtLockBudgetRepo{Repository: budgetrepo.NewRepo(eng, txm)},
		debtRepo:   &debtLockDebtRepo{Repository: debtrepo.NewRepo(eng, txm)},
	}
	p.budget = appbudget.NewService(
		p.budgetRepo, budgetrepo.NewReadRepo(eng, txm), convertor, rateProvider,
		server.NewBudgetUserLookup(userRepo, clk),
		server.NewBudgetAccountLookup(accountrepo.NewRepo(eng, txm)),
		server.NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(server.NewBudgetCategoryMetadataLookup(categoryRepo), server.NewBudgetTagMetadataLookup(tagRepo), server.NewBudgetPayeeMetadataLookup(payeeRepo)),
		accessResolver, operationrepo.NewGuard(eng, txm), txm, clk,
	)
	p.debts = appdebt.NewService(p.debtRepo, txm, clk,
		server.NewDebtClassifications(categoryRepo, payeeRepo, tagRepo, labelrepo.NewRepo(eng, txm)),
		server.NewDebtRates(rateProvider, convertor), currencyLookup, server.NewDebtUsers(userRepo))
	p.debts.SetBudgets(server.NewDebtBudgets(p.budget))
	p.debts.SetBudgetElements(server.NewDebtBudgetElements(p.budget))
	p.budget.SetDebts(p.debts)
	p.categories = appcategory.NewService(categoryRepo, txm, operationrepo.NewGuard(eng, txm), clk, categoryrepo.NewReadRepo(eng, txm), accessResolver, nil)
	p.categories.SetDebtUsage(server.NewDebtUsage(p.debts, appdebt.KindCategory))
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
	loan     string // the debt's first rule
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

func (r *debtLockRig) setLimit(svc *appbudget.Service, element string, amount *string) error {
	var a *vo.FlexString
	if amount != nil {
		v := vo.NewFlexString(*amount)
		a = &v
	}
	_, err := svc.SetLimit(context.Background(), r.owner, model.SetLimitRequest{BudgetId: r.budgetID, ElementId: element, Period: debtLockPeriod, Amount: a})
	return err
}

func strp(s string) *string { return &s }

func (r *debtLockRig) createDebt(t *testing.T, rule model.DebtRuleRequest) string {
	t.Helper()
	res, err := r.one.debts.CreateDebt(context.Background(), r.owner, model.CreateDebtRequest{
		BudgetId: r.budgetID, Name: "Loan", CurrencyId: fixture.USD, Rules: []model.DebtRuleRequest{rule},
		Payment: vo.NewFlexString("100"), Schedule: "monthly",
	})
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}
	// The debt's row exists before the race, as it does once the budget has
	// been read: set and clear a plan through the row's self-heal.
	if err := r.setLimit(r.one.budget, res.Id, strp("1")); err != nil {
		t.Fatalf("seed the debt row: %v", err)
	}
	if err := r.setLimit(r.one.budget, res.Id, nil); err != nil {
		t.Fatalf("clear the seed plan: %v", err)
	}
	return res.Id
}

// plan is the external id's plan in the race period, "" for none; rows is
// how many element rows the budget holds for it.
func (r *debtLockRig) plan(t *testing.T, externalID string) (plan string, rows int) {
	t.Helper()
	if err := r.monitor.QueryRow(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = $1 AND external_id = $2`, r.budgetID, externalID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	err := r.monitor.QueryRow(`SELECT COALESCE((SELECT l.amount::text FROM budgets_elements_limits l
		JOIN budgets_elements e ON e.id = l.element_id
		WHERE e.budget_id = $1 AND e.external_id = $2 AND l.period = $3), '')`, r.budgetID, externalID, debtLockPeriod).Scan(&plan)
	if err != nil {
		t.Fatal(err)
	}
	if plan != "" {
		plan = vo.NewDecimal(plan).String()
	}
	return plan, rows
}

// Review Focus 5: a plan saved on the category row and still open when
// add-debt-rule takes the category: the rule write waits for it, then moves
// the new plan onto the debt row, adding it to what was there.
func TestAddDebtRule_ConcurrentSetLimitIsMovedOrWaits(t *testing.T) {
	r := newDebtLockRig(t)
	ctx := context.Background()
	debtID := r.createDebt(t, model.DebtRuleRequest{CategoryId: r.loan})
	if err := r.setLimit(r.one.budget, debtID, strp("10")); err != nil {
		t.Fatal(err)
	}
	if err := r.setLimit(r.one.budget, r.car, strp("20")); err != nil {
		t.Fatal(err)
	}

	wrote, unblock := pause(t, func(h func()) { r.two.budgetRepo.afterSaveLimit = h },
		func() error { return r.setLimit(r.two.budget, r.car, strp("75")) })
	added := make(chan error, 1)
	go func() {
		_, err := r.one.debts.AddDebtRule(ctx, r.owner, model.AddDebtRuleRequest{Id: debtID, Rule: model.DebtRuleRequest{CategoryId: r.car}})
		added <- err
	}()
	r.waitLockWait(t, r.one.pid, "add-debt-rule", added)

	unblock()
	if err := <-wrote; err != nil {
		t.Fatalf("set-limit: %v", err)
	}
	if err := <-added; err != nil {
		t.Fatalf("add-debt-rule: %v", err)
	}
	if got, _ := r.plan(t, debtID); got != "85" {
		t.Errorf("debt row plan = %q, want 85 (its 10 + the new 75)", got)
	}
	if got, _ := r.plan(t, r.car); got != "" {
		t.Errorf("category row plan = %q, want it moved", got)
	}
}

// set-limit on the debt row is open while delete-debt runs: the delete waits
// for it and then sees the plan, so it asks for confirmation, drops it with
// the row once confirmed, or hands it back to a whole category. When the
// delete holds the row first, set-limit waits and finds the row gone.
func TestDeleteDebt_ConcurrentSetLimitOnTheDebtRow(t *testing.T) {
	for _, c := range []struct {
		name       string
		whole      bool
		confirm    bool
		wantRefuse bool
	}{
		{name: "partial debt, unconfirmed", wantRefuse: true},
		{name: "partial debt, confirmed", confirm: true},
		{name: "whole-category debt", whole: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newDebtLockRig(t)
			ctx := context.Background()
			rule := model.DebtRuleRequest{CategoryId: r.loan}
			if !c.whole {
				bank := fixture.New(t, r.db).Payee(fixture.Payee{UserID: r.owner.String(), Name: "Bank"})
				rule.PayeeId = &bank
			}
			debtID := r.createDebt(t, rule)

			wrote, unblock := pause(t, func(h func()) { r.two.budgetRepo.afterSaveLimit = h },
				func() error { return r.setLimit(r.two.budget, debtID, strp("75")) })
			deleted := make(chan error, 1)
			go func() {
				_, err := r.one.debts.DeleteDebt(ctx, r.owner, model.DeleteDebtRequest{Id: debtID, ConfirmDataRemoval: c.confirm})
				deleted <- err
			}()
			r.waitLockWait(t, r.one.pid, "delete-debt", deleted)

			unblock()
			if err := <-wrote; err != nil {
				t.Fatalf("set-limit: %v", err)
			}
			err := <-deleted
			plan, rows := r.plan(t, debtID)
			switch {
			case c.wantRefuse:
				var ve *errs.ValidationError
				if !errors.As(err, &ve) || len(ve.Fields) != 1 || ve.Fields[0].Key != "confirmDataRemoval" {
					t.Fatalf("delete-debt = %v, want the confirmDataRemoval refusal", err)
				}
				if plan != "75" || rows != 1 {
					t.Errorf("debt row = %d rows, plan %q; want the row and its 75 kept", rows, plan)
				}
			default:
				if err != nil {
					t.Fatalf("delete-debt: %v", err)
				}
				if rows != 0 {
					t.Errorf("%d debt rows left after the delete", rows)
				}
			}
			if c.whole {
				if got, _ := r.plan(t, r.loan); got != "75" {
					t.Errorf("category plan = %q, want the 75 handed back", got)
				}
			}
		})
	}

	t.Run("delete first", func(t *testing.T) {
		r := newDebtLockRig(t)
		ctx := context.Background()
		bank := fixture.New(t, r.db).Payee(fixture.Payee{UserID: r.owner.String(), Name: "Bank"})
		debtID := r.createDebt(t, model.DebtRuleRequest{CategoryId: r.loan, PayeeId: &bank})

		deleted, unblock := pause(t, func(h func()) { r.one.budgetRepo.afterLockElements = h },
			func() error {
				_, err := r.one.debts.DeleteDebt(ctx, r.owner, model.DeleteDebtRequest{Id: debtID})
				return err
			})
		wrote := make(chan error, 1)
		go func() { wrote <- r.setLimit(r.two.budget, debtID, strp("75")) }()
		r.waitLockWait(t, r.two.pid, "set-limit", wrote)

		unblock()
		if err := <-deleted; err != nil {
			t.Fatalf("delete-debt: %v", err)
		}
		if _, nf := errs.AsNotFound(<-wrote); !nf {
			t.Fatal("set-limit after the delete committed, want not found")
		}
		if _, rows := r.plan(t, debtID); rows != 0 {
			t.Errorf("%d debt rows left after the delete", rows)
		}
	})
}

// Ruling 2: add-debt-rule holds a key-share lock on the new rule's category.
// A category delete arriving then waits for it, and releases the rule once it
// commits, instead of failing the rule's foreign key.
func TestAddDebtRule_ConcurrentCategoryDeleteReleasesTheRule(t *testing.T) {
	r := newDebtLockRig(t)
	ctx := context.Background()
	debtID := r.createDebt(t, model.DebtRuleRequest{CategoryId: r.loan})

	added, unblock := pause(t, func(h func()) { r.one.debtRepo.afterLockRuleCategories = h },
		func() error {
			_, err := r.one.debts.AddDebtRule(ctx, r.owner, model.AddDebtRuleRequest{Id: debtID, Rule: model.DebtRuleRequest{CategoryId: r.car}})
			return err
		})
	deleted := make(chan error, 1)
	go func() {
		_, err := r.two.categories.DeleteCategory(ctx, r.owner, model.DeleteCategoryRequest{Id: r.car})
		deleted <- err
	}()
	r.waitLockWait(t, r.two.pid, "delete-category", deleted)

	unblock()
	if err := <-added; err != nil {
		t.Fatalf("add-debt-rule: %v", err)
	}
	if err := <-deleted; err != nil {
		t.Fatalf("delete-category: %v", err)
	}
	var rules, onCar, categories int
	if err := r.monitor.QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE category_id = $2) FROM debts_rules WHERE debt_id = $1`, debtID, r.car).Scan(&rules, &onCar); err != nil {
		t.Fatal(err)
	}
	if rules != 1 || onCar != 0 {
		t.Errorf("debt rules = %d (%d on the deleted category), want the first rule only", rules, onCar)
	}
	if err := r.monitor.QueryRow(`SELECT COUNT(*) FROM categories WHERE id = $1`, r.car).Scan(&categories); err != nil || categories != 0 {
		t.Errorf("category rows = %d, %v; want it deleted", categories, err)
	}
}
