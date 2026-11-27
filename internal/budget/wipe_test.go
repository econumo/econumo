package budget_test

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appbudget "github.com/econumo/econumo/internal/budget"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	domcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	appdebt "github.com/econumo/econumo/internal/debt"
	debtrepo "github.com/econumo/econumo/internal/debt/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// The member ids sort P < U < T, so the wiped user's clears sit between the
// owned-budget members' and a sort by insertion order would be caught.
const (
	wipeP = "11111111-1111-7111-8111-111111111111"
	wipeU = "55555555-5555-7555-8555-555555555555"
	wipeT = "99999999-9999-7999-8999-999999999999"

	wipeBP  = "a1111111-1111-7111-8111-111111111111" // P's, U and T members
	wipeBX  = "a2222222-2222-7222-8222-222222222222" // T's, U pending
	wipeBU  = "b1111111-1111-7111-8111-111111111111" // U's, P member
	wipeBU2 = "b2222222-2222-7222-8222-222222222222" // U's, P and T members
)

type wipeEvents struct{ log []string }

func (e *wipeEvents) add(s string) { e.log = append(e.log, s) }

type wipeRecordingRepo struct {
	appbudget.Repository
	events *wipeEvents
}

func (r wipeRecordingRepo) DeleteAccess(ctx context.Context, budgetID, userID vo.Id) error {
	r.events.add("delete:" + budgetID.String() + ":" + userID.String())
	return r.Repository.DeleteAccess(ctx, budgetID, userID)
}

type wipeRecordingDebts struct {
	appbudget.DebtSource
	events *wipeEvents
}

func (r wipeRecordingDebts) LockBudget(ctx context.Context, id vo.Id) error {
	r.events.add("lock:" + id.String())
	return r.DebtSource.LockBudget(ctx, id)
}

type wipeRecordingUsers struct {
	*server.BudgetUserLookup
	events *wipeEvents
}

func (u wipeRecordingUsers) ClearActiveBudget(ctx context.Context, userID, budgetID vo.Id) error {
	u.events.add("clear:" + userID.String() + ":" + budgetID.String())
	return u.BudgetUserLookup.ClearActiveBudget(ctx, userID, budgetID)
}

type wipeHarness struct {
	ctx    context.Context
	tdb    *dbtest.DB
	svc    *appbudget.Service
	events *wipeEvents

	catU, catP, catT string
	accU, accT       string
	elemU, elemT     string
	limitU           string
	envBP            string
}

func newWipeHarness(t *testing.T) *wipeHarness {
	t.Helper()
	tdb := dbtest.New(t)
	txm := tdb.TX
	engine := tdb.Engine
	f := fixture.New(t, tdb)
	clk := clock.New()

	f.User(fixture.User{ID: wipeP, Name: "P", Email: "p@wipe-budget.test"})
	f.User(fixture.User{ID: wipeU, Name: "U", Email: "u@wipe-budget.test"})
	f.User(fixture.User{ID: wipeT, Name: "T", Email: "t@wipe-budget.test"})

	h := &wipeHarness{ctx: context.Background(), tdb: tdb, events: &wipeEvents{}}
	h.catU = f.Category(fixture.Category{UserID: wipeU, Name: "U food"})
	h.catP = f.Category(fixture.Category{UserID: wipeP, Name: "P food"})
	h.catT = f.Category(fixture.Category{UserID: wipeT, Name: "T food"})
	h.accU = f.Account(fixture.Account{UserID: wipeU, Name: "U cash"})
	h.accT = f.Account(fixture.Account{UserID: wipeT, Name: "T cash"})

	f.Budget(fixture.Budget{ID: wipeBU, UserID: wipeU})
	f.BudgetAccess(wipeBU, wipeP, int(model.BudgetRoleUser), true)
	f.BudgetElement(fixture.BudgetElement{BudgetID: wipeBU, ExternalID: h.catP, Type: int(model.ElementCategory)})
	bu := wipeBU
	f.Option(wipeP, model.OptionBudget, &bu)

	f.Budget(fixture.Budget{ID: wipeBU2, UserID: wipeU})
	f.BudgetAccess(wipeBU2, wipeP, int(model.BudgetRoleUser), true)
	f.BudgetAccess(wipeBU2, wipeT, int(model.BudgetRoleUser), true)

	f.Budget(fixture.Budget{ID: wipeBP, UserID: wipeP})
	f.BudgetAccess(wipeBP, wipeU, int(model.BudgetRoleUser), true)
	f.BudgetAccess(wipeBP, wipeT, int(model.BudgetRoleUser), true)
	h.elemU = f.BudgetElement(fixture.BudgetElement{BudgetID: wipeBP, ExternalID: h.catU, Type: int(model.ElementCategory), Position: 1})
	h.elemT = f.BudgetElement(fixture.BudgetElement{BudgetID: wipeBP, ExternalID: h.catT, Type: int(model.ElementCategory), Position: 2})
	h.limitU = f.BudgetLimit(fixture.BudgetLimit{ElementID: h.elemU, Period: "2026-11-01", Amount: "100.00000000"})
	f.BudgetAccount(wipeBP, h.accU)
	f.BudgetAccount(wipeBP, h.accT)
	h.envBP = f.BudgetEnvelope(fixture.BudgetEnvelope{BudgetID: wipeBP, Name: "Shared"})
	f.EnvelopeCategory(h.envBP, h.catU)
	f.EnvelopeCategory(h.envBP, h.catT)

	f.Budget(fixture.Budget{ID: wipeBX, UserID: wipeT})
	f.BudgetAccess(wipeBX, wipeU, int(model.BudgetRoleUser), false)

	userRepo := userrepo.NewRepo(engine, txm)
	categoryRepo := categoryrepo.NewRepo(engine, txm)
	currencyLookup := currencyrepo.New(engine, txm)
	rateProvider := currencyrepo.NewRateProvider(engine, txm, currencyLookup, fixture.USD)
	convertor := domcurrency.NewConvertor(rateProvider)
	svc := appbudget.NewService(
		wipeRecordingRepo{Repository: budgetrepo.NewRepo(engine, txm), events: h.events},
		budgetrepo.NewReadRepo(engine, txm), convertor, rateProvider,
		wipeRecordingUsers{BudgetUserLookup: server.NewBudgetUserLookup(userRepo, clk), events: h.events},
		server.NewBudgetAccountLookup(accountrepo.NewRepo(engine, txm)),
		server.NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(
			server.NewBudgetCategoryMetadataLookup(categoryRepo),
			server.NewBudgetTagMetadataLookup(tagrepo.NewRepo(engine, txm)),
			server.NewBudgetPayeeMetadataLookup(payeerepo.NewRepo(engine, txm))),
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(engine, txm)),
		operationrepo.NewGuard(engine, txm),
		txm, clk,
	)
	debts := appdebt.NewService(debtrepo.NewRepo(engine, txm), txm, clk,
		server.NewDebtCategories(categoryRepo), server.NewDebtRates(rateProvider, convertor), server.NewDebtUsers(userRepo))
	svc.SetDebts(wipeRecordingDebts{DebtSource: debts, events: h.events})
	debts.SetBudgets(server.NewDebtBudgets(svc))
	debts.SetElements(server.NewDebtElements(svc))
	h.svc = svc
	return h
}

func (h *wipeHarness) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := h.tdb.Raw.QueryRow(h.tdb.Rebind(query), args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func (h *wipeHarness) strings(t *testing.T, query string, args ...any) []string {
	t.Helper()
	rows, err := h.tdb.Raw.Query(h.tdb.Rebind(query), args...)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	slices.Sort(out)
	return out
}

func TestWipeBudgetIDs_OwnedAndAccessSorted(t *testing.T) {
	h := newWipeHarness(t)
	ids, err := h.svc.WipeBudgetIDs(h.ctx, vo.MustParseId(wipeU))
	if err != nil {
		t.Fatalf("WipeBudgetIDs: %v", err)
	}
	got := make([]string, len(ids))
	for i, id := range ids {
		got[i] = id.String()
	}
	if want := []string{wipeBP, wipeBX, wipeBU, wipeBU2}; !slices.Equal(got, want) {
		t.Errorf("WipeBudgetIDs = %v, want %v", got, want)
	}
}

func TestWipeUnwindUser_RemovesMembershipAndClearsPointers(t *testing.T) {
	h := newWipeHarness(t)
	if err := h.svc.UnwindUser(h.ctx, vo.MustParseId(wipeU)); err != nil {
		t.Fatalf("UnwindUser: %v", err)
	}

	if got := h.strings(t, `SELECT user_id FROM budgets_access WHERE budget_id = ?`, wipeBP); !slices.Equal(got, []string{wipeT}) {
		t.Errorf("BP access = %v, want only T", got)
	}
	if n := h.count(t, `SELECT COUNT(*) FROM budgets_access WHERE user_id = ?`, wipeU); n != 0 {
		t.Errorf("U access rows = %d, want 0 (pending one included)", n)
	}
	if n := h.count(t, `SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND external_id = ?`, wipeBP, h.catU); n != 0 {
		t.Errorf("elements of U's category in BP = %d, want 0", n)
	}
	if n := h.count(t, `SELECT COUNT(*) FROM budgets_elements_limits WHERE id = ?`, h.limitU); n != 0 {
		t.Errorf("U's limit rows = %d, want 0", n)
	}
	if got := h.strings(t, `SELECT account_id FROM budgets_accounts WHERE budget_id = ?`, wipeBP); !slices.Equal(got, []string{h.accT}) {
		t.Errorf("BP member accounts = %v, want only T's", got)
	}
	if got := h.strings(t, `SELECT category_id FROM budgets_envelopes_categories WHERE budget_envelope_id = ?`, h.envBP); !slices.Equal(got, []string{h.catT}) {
		t.Errorf("envelope categories = %v, want only T's", got)
	}
	if n := h.count(t, `SELECT COUNT(*) FROM budgets_elements WHERE id = ?`, h.elemT); n != 1 {
		t.Errorf("T's element rows = %d, want 1", n)
	}

	var pointer sql.NullString
	if err := h.tdb.Raw.QueryRow(h.tdb.Rebind(`SELECT value FROM users_options WHERE user_id = ? AND name = ?`), wipeP, model.OptionBudget).Scan(&pointer); err != nil {
		t.Fatalf("read P's budget option: %v", err)
	}
	if pointer.Valid {
		t.Errorf("P's budget option = %q, want NULL", pointer.String)
	}
}

// Each clear takes a user row lock, which goes after every budget-side change
// and in one global order, or two wipes sharing members can deadlock.
func TestWipeUnwindUser_ClearsAfterRemovalsInAscendingUserOrder(t *testing.T) {
	h := newWipeHarness(t)
	if err := h.svc.UnwindUser(h.ctx, vo.MustParseId(wipeU)); err != nil {
		t.Fatalf("UnwindUser: %v", err)
	}

	firstClear := slices.IndexFunc(h.events.log, func(e string) bool { return strings.HasPrefix(e, "clear:") })
	if firstClear < 0 {
		t.Fatalf("events = %v, want clears", h.events.log)
	}
	for _, e := range h.events.log[firstClear:] {
		if !strings.HasPrefix(e, "clear:") {
			t.Errorf("event %q after the first clear; events = %v", e, h.events.log)
		}
	}
	for _, want := range []string{"lock:" + wipeBP, "delete:" + wipeBP + ":" + wipeU, "lock:" + wipeBX, "delete:" + wipeBX + ":" + wipeU} {
		if !slices.Contains(h.events.log[:firstClear], want) {
			t.Errorf("events before the clears = %v, want %q", h.events.log[:firstClear], want)
		}
	}
	wantClears := []string{
		"clear:" + wipeP + ":" + wipeBU,
		"clear:" + wipeP + ":" + wipeBU2,
		"clear:" + wipeU + ":" + wipeBP,
		"clear:" + wipeU + ":" + wipeBX,
		"clear:" + wipeT + ":" + wipeBU2,
	}
	if got := h.events.log[firstClear:]; !slices.Equal(got, wantClears) {
		t.Errorf("clears = %v, want %v", got, wantClears)
	}
}

func TestWipeDeleteOwned_DeletesOnlyOwnedBudgets(t *testing.T) {
	h := newWipeHarness(t)
	if err := h.svc.UnwindUser(h.ctx, vo.MustParseId(wipeU)); err != nil {
		t.Fatalf("UnwindUser: %v", err)
	}
	if err := h.svc.DeleteOwned(h.ctx, vo.MustParseId(wipeU)); err != nil {
		t.Fatalf("DeleteOwned: %v", err)
	}
	if got := h.strings(t, `SELECT id FROM budgets`); !slices.Equal(got, []string{wipeBP, wipeBX}) {
		t.Errorf("budgets = %v, want BP and BX", got)
	}
}

// A removal must take the budget lock before the access row: a wipe holds the
// member's user row and wants the access row, while a removal holding the
// access row wants that user row. With the budget lock first on both, the
// removal waits instead of closing the cycle.
func TestWipeLockOrder_AccessRemovalsLockBudgetFirst(t *testing.T) {
	cases := []struct {
		name     string
		budgetID string
		userID   string
		run      func(h *wipeHarness) error
	}{
		{"RevokeAccess", wipeBP, wipeT, func(h *wipeHarness) error {
			_, err := h.svc.RevokeAccess(h.ctx, vo.MustParseId(wipeP), model.RevokeAccessRequest{BudgetId: wipeBP, UserId: wipeT})
			return err
		}},
		{"RemoveMember", wipeBP, wipeT, func(h *wipeHarness) error {
			return h.svc.RemoveMember(h.ctx, vo.MustParseId(wipeBP), vo.MustParseId(wipeT))
		}},
		{"RemoveMemberKeepPointer", wipeBP, wipeT, func(h *wipeHarness) error {
			return h.svc.RemoveMemberKeepPointer(h.ctx, vo.MustParseId(wipeBP), vo.MustParseId(wipeT))
		}},
		{"DeclineAccess", wipeBX, wipeU, func(h *wipeHarness) error {
			_, err := h.svc.DeclineAccess(h.ctx, vo.MustParseId(wipeU), model.DeclineAccessRequest{BudgetId: wipeBX})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newWipeHarness(t)
			if err := tc.run(h); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			lock := slices.Index(h.events.log, "lock:"+tc.budgetID)
			del := slices.Index(h.events.log, "delete:"+tc.budgetID+":"+tc.userID)
			if lock < 0 || del < 0 || lock > del {
				t.Errorf("events = %v, want the budget lock before the access delete", h.events.log)
			}
		})
	}
}
