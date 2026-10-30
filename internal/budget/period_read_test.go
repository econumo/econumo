package budget_test

import (
	"context"
	"fmt"
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
	"github.com/econumo/econumo/internal/model"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type periodRecurring struct{ templates []*model.RecurringTransaction }

func (r periodRecurring) ListByAccountIDs(context.Context, []vo.Id) ([]*model.RecurringTransaction, error) {
	return r.templates, nil
}

func (periodRecurring) CanEdit(context.Context, vo.Id, vo.Id) (bool, error) { return true, nil }

// periodHarness is a budget started on key 2026-10-01 whose periods start on
// startDay, with one everyday account, one savings-capable account and one
// expense category.
type periodHarness struct {
	t        *testing.T
	svc      *appbudget.Service
	f        *fixture.Builder
	tdb      *dbtest.DB
	owner    vo.Id
	budgetID string
	account  string
	savings  string
	category string
	rec      *periodRecurring
}

func newPeriodHarness(t *testing.T, startDay int, now time.Time) *periodHarness {
	t.Helper()
	tdb := dbtest.New(t)
	txm, eng := tdb.TX, tdb.Engine
	f := fixture.New(t, tdb)

	owner := vo.MustParseId(f.User(fixture.User{Name: "Owner", Email: "owner@period.test"}))
	account := f.Account(fixture.Account{UserID: owner.String(), Name: "Cash"})
	f.AccountOption(account, owner.String(), 0)
	savings := f.Account(fixture.Account{UserID: owner.String(), Name: "Savings"})
	f.AccountOption(savings, owner.String(), 1)
	category := f.Category(fixture.Category{UserID: owner.String(), Name: "Food"})

	started := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	budgetID := f.Budget(fixture.Budget{UserID: owner.String(), StartedAt: &started})
	f.BudgetAccount(budgetID, account)
	f.BudgetElement(fixture.BudgetElement{BudgetID: budgetID, ExternalID: category, Type: int(model.ElementCategory)})
	if _, err := tdb.Raw.Exec(tdb.Rebind(`UPDATE budgets SET period_start_day = ? WHERE id = ?`), startDay, budgetID); err != nil {
		t.Fatal(err)
	}

	userRepo := userrepo.NewRepo(eng, txm)
	accountRepo := accountrepo.NewRepo(eng, txm)
	currencyLookup := currencyrepo.New(eng, txm)
	rateProvider := currencyrepo.NewRateProvider(eng, txm, currencyLookup, fixture.USD)
	clk := fixedClock{t: now}
	svc := appbudget.NewService(
		budgetrepo.NewRepo(eng, txm), budgetrepo.NewReadRepo(eng, txm),
		domcurrency.NewConvertor(rateProvider), rateProvider,
		server.NewBudgetUserLookup(userRepo, clk),
		server.NewBudgetAccountLookup(accountRepo),
		server.NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(
			server.NewBudgetCategoryMetadataLookup(categoryrepo.NewRepo(eng, txm)),
			server.NewBudgetTagMetadataLookup(tagrepo.NewRepo(eng, txm)),
			server.NewBudgetPayeeMetadataLookup(payeerepo.NewRepo(eng, txm))),
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(eng, txm)),
		operationrepo.NewGuard(eng, txm),
		txm, clk,
	)
	rec := &periodRecurring{}
	svc.SetRecurring(rec)
	return &periodHarness{t: t, svc: svc, f: f, tdb: tdb, owner: owner, budgetID: budgetID,
		account: account, savings: savings, category: category, rec: rec}
}

func (h *periodHarness) exec(query string, args ...any) {
	h.t.Helper()
	if _, err := h.tdb.Raw.Exec(h.tdb.Rebind(query), args...); err != nil {
		h.t.Fatal(err)
	}
}

func (h *periodHarness) spend(amount, at string) string {
	h.t.Helper()
	return h.f.Transaction(fixture.Transaction{
		UserID: h.owner.String(), AccountID: h.account, CategoryID: h.category, Amount: amount, SpentAt: at,
	})
}

func (h *periodHarness) get(ctx context.Context, userID vo.Id, date string) model.BudgetResult {
	h.t.Helper()
	res, err := h.svc.GetBudget(ctx, userID, model.GetBudgetRequest{Id: h.budgetID, Date: date})
	if err != nil {
		h.t.Fatalf("get-budget %q: %v", date, err)
	}
	return res.Item
}

func (h *periodHarness) categoryRow(res model.BudgetResult) model.ParentElementResult {
	h.t.Helper()
	for _, e := range res.Structure.Elements {
		if e.Id == h.category && e.Type == int(model.ElementCategory) {
			return e
		}
	}
	h.t.Fatalf("category row missing: %+v", res.Structure.Elements)
	return model.ParentElementResult{}
}

func (h *periodHarness) savingsRow(res model.BudgetResult) model.SavingsElementResult {
	h.t.Helper()
	for _, s := range res.Structure.Savings {
		if s.Id == h.savings {
			return s
		}
	}
	h.t.Fatalf("savings row missing: %+v", res.Structure.Savings)
	return model.SavingsElementResult{}
}

func sameAmount(t *testing.T, what, got, want string) {
	t.Helper()
	if vo.NewDecimal(got).String() != vo.NewDecimal(want).String() {
		t.Errorf("%s = %s, want %s", what, got, want)
	}
}

func sameWindow(t *testing.T, res model.BudgetResult, start, end string) {
	t.Helper()
	if start != "" && res.Filters.PeriodStart != start {
		t.Errorf("filters.periodStart = %s, want %s", res.Filters.PeriodStart, start)
	}
	if end != "" && res.Filters.PeriodEnd != end {
		t.Errorf("filters.periodEnd = %s, want %s", res.Filters.PeriodEnd, end)
	}
}

var periodNow = time.Date(2026, 12, 15, 12, 0, 0, 0, time.UTC)

func seedShiftedSpending(h *periodHarness) {
	h.spend("100", "2026-10-27 23:59:59")
	h.spend("50", "2026-10-28 00:00:00")
	h.spend("20", "2026-11-27 10:00:00")
	h.spend("10", "2026-11-28 00:00:00")
}

func TestGetBudgetShiftedWindow(t *testing.T) {
	h := newPeriodHarness(t, 28, periodNow)
	seedShiftedSpending(h)
	ctx := context.Background()

	nov := h.get(ctx, h.owner, "2026-11-01")
	sameWindow(t, nov, "2026-10-28 00:00:00", "2026-11-28 00:00:00")
	row := h.categoryRow(nov)
	sameAmount(t, "november spent", row.Spent, "70")
	// no plans: available = -(spentBefore + spent)
	sameAmount(t, "november available", row.Available, "-170")

	oct := h.get(ctx, h.owner, "2026-10-01")
	sameWindow(t, oct, "2026-09-28 00:00:00", "2026-10-28 00:00:00")
	sameAmount(t, "october spent", h.categoryRow(oct).Spent, "100")
}

func TestGetBudgetShiftedDefaultPeriod(t *testing.T) {
	h := newPeriodHarness(t, 28, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
	sameWindow(t, h.get(context.Background(), h.owner, ""), "2026-10-28 00:00:00", "2026-11-28 00:00:00")
}

func TestGetBudgetShiftedTimezone(t *testing.T) {
	h := newPeriodHarness(t, 28, time.Date(2026, 10, 27, 22, 0, 0, 0, time.UTC))
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	ctx := reqctx.WithLocation(context.Background(), tokyo)
	sameWindow(t, h.get(ctx, h.owner, ""), "2026-10-28 00:00:00", "")
	sameWindow(t, h.get(context.Background(), h.owner, ""), "2026-09-28 00:00:00", "")
}

func TestGetBudgetShiftedPartner(t *testing.T) {
	h := newPeriodHarness(t, 28, periodNow)
	seedShiftedSpending(h)
	partner := vo.MustParseId(h.f.User(fixture.User{Name: "Partner", Email: "partner@period.test"}))
	h.f.BudgetAccess(h.budgetID, partner.String(), int(model.BudgetRoleAdmin), true)

	res := h.get(context.Background(), partner, "2026-11-01")
	sameWindow(t, res, "2026-10-28 00:00:00", "2026-11-28 00:00:00")
	sameAmount(t, "partner spent", h.categoryRow(res).Spent, "70")
}

func TestBudgetTransactionListShifted(t *testing.T) {
	h := newPeriodHarness(t, 28, periodNow)
	h.spend("100", "2026-10-27 23:59:59")
	in1 := h.spend("50", "2026-10-28 00:00:00")
	in2 := h.spend("20", "2026-11-27 10:00:00")
	h.spend("10", "2026-11-28 00:00:00")

	cat := h.category
	res, err := h.svc.GetTransactionList(context.Background(), h.owner, model.BudgetTransactionListRequest{
		BudgetId: h.budgetID, PeriodStart: "2026-11-01", CategoryId: &cat,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, it := range res.Items {
		got[it.Id] = true
	}
	if len(got) != 2 || !got[in1] || !got[in2] {
		t.Fatalf("list = %+v, want exactly %s and %s", res.Items, in1, in2)
	}
}

func TestGetBudgetShiftedEnded(t *testing.T) {
	h := newPeriodHarness(t, 20, time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC))
	h.exec(`UPDATE budgets SET ended_at = ? WHERE id = ?`, "2026-11-01 00:00:00", h.budgetID)
	h.spend("5", "2026-11-25 10:00:00")

	res := h.get(context.Background(), h.owner, "2027-01-01")
	sameWindow(t, res, "2026-10-20 00:00:00", "2026-11-20 00:00:00")
	if len(res.Filters.Accounts) != 1 || !res.Filters.Accounts[0].Removable {
		t.Fatalf("account after the last period must stay removable: %+v", res.Filters.Accounts)
	}
}

func TestRemovableAccountsShifted(t *testing.T) {
	cases := []struct {
		at        string
		removable bool
	}{
		{"2026-10-28 00:00:00", true},
		{"2026-10-27 23:59:59", false},
	}
	for i, c := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			h := newPeriodHarness(t, 28, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
			h.spend("5", c.at)
			res := h.get(context.Background(), h.owner, "")
			if len(res.Filters.Accounts) != 1 || res.Filters.Accounts[0].Removable != c.removable {
				t.Errorf("txn %s: accounts = %+v, want removable %v", c.at, res.Filters.Accounts, c.removable)
			}
		})
	}
}

func (h *periodHarness) monthly(amount string, next time.Time) {
	cat := vo.MustParseId(h.category)
	h.rec.templates = append(h.rec.templates, &model.RecurringTransaction{
		ID: vo.NewId(), UserID: h.owner, Type: model.TransactionTypeExpense,
		AccountID: vo.MustParseId(h.account), Amount: amount, CategoryID: &cat, Description: "rent",
		Schedule: model.RecurringScheduleMonthly, ScheduledDay: int16(next.Day()), NextPaymentAt: next,
	})
}

func TestUpcomingShiftedOccurrences(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	t.Run("shifted", func(t *testing.T) {
		h := newPeriodHarness(t, 28, now)
		h.monthly("40", time.Date(2026, 11, 29, 0, 0, 0, 0, time.UTC))
		sameAmount(t, "key 2026-11 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-11-01")).Upcoming, "0")
		sameAmount(t, "key 2026-12 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-12-01")).Upcoming, "40")
	})
	t.Run("calendar", func(t *testing.T) {
		h := newPeriodHarness(t, 1, now)
		h.monthly("40", time.Date(2026, 11, 29, 0, 0, 0, 0, time.UTC))
		sameAmount(t, "key 2026-11 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-11-01")).Upcoming, "40")
	})
	// Overdue since 2026-10-20: filed today, in the current period (key
	// 2026-11) next to its 2026-11-20 occurrence, and nowhere else.
	t.Run("overdue", func(t *testing.T) {
		h := newPeriodHarness(t, 28, now)
		h.monthly("7", time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC))
		sameAmount(t, "key 2026-10 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-10-01")).Upcoming, "0")
		sameAmount(t, "key 2026-11 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-11-01")).Upcoming, "14")
		sameAmount(t, "key 2026-12 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-12-01")).Upcoming, "7")
	})
}

func TestSavingsClosingShifted(t *testing.T) {
	h := newPeriodHarness(t, 28, periodNow)
	h.f.BudgetAccount(h.budgetID, h.savings)
	h.exec(`UPDATE budgets_accounts SET is_savings = ? WHERE budget_id = ? AND account_id = ?`, true, h.budgetID, h.savings)
	h.f.Transaction(fixture.Transaction{
		UserID: h.owner.String(), AccountID: h.account, AccountRecipientID: h.savings, Type: 2,
		Amount: "30", AmountRecipient: "30", SpentAt: "2026-10-29 09:00:00",
	})
	ctx := context.Background()

	nov := h.savingsRow(h.get(ctx, h.owner, "2026-11-01"))
	sameAmount(t, "november saved", nov.Spent, "30")
	sameAmount(t, "november closing", nov.ClosingBalance, "30")

	oct := h.savingsRow(h.get(ctx, h.owner, "2026-10-01"))
	sameAmount(t, "october saved", oct.Spent, "0")
	sameAmount(t, "october closing", oct.ClosingBalance, "0")

	// The current period (key 2026-12, from 2026-11-28) plans 100 and has
	// saved 30 on 2026-11-29: the projected closing is booked 60 plus the
	// unmet 70.
	element := h.f.BudgetElement(fixture.BudgetElement{BudgetID: h.budgetID, ExternalID: h.savings, Type: int(model.ElementSavings)})
	h.f.BudgetLimit(fixture.BudgetLimit{ElementID: element, Period: "2026-12-01 00:00:00", Amount: "100"})
	h.f.Transaction(fixture.Transaction{
		UserID: h.owner.String(), AccountID: h.account, AccountRecipientID: h.savings, Type: 2,
		Amount: "30", AmountRecipient: "30", SpentAt: "2026-11-29 09:00:00",
	})
	dec := h.savingsRow(h.get(ctx, h.owner, "2026-12-01"))
	sameAmount(t, "december saved", dec.Spent, "30")
	sameAmount(t, "december closing", dec.ClosingBalance, "130")
}

func TestCreateBudgetShiftedDefaultStart(t *testing.T) {
	h := newPeriodHarness(t, 1, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
	res, err := h.svc.CreateBudget(context.Background(), h.owner, model.CreateBudgetRequest{
		Id: vo.NewId().String(), Name: "Payday", CurrencyId: fixture.USD, AccountIds: []string{h.account}, PeriodStartDay: 28,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.Meta.StartedAt != "2026-11-01 00:00:00" {
		t.Errorf("startedAt = %s, want the current period's key 2026-11-01", res.Item.Meta.StartedAt)
	}
	sameWindow(t, res.Item, "2026-10-28 00:00:00", "2026-11-28 00:00:00")
}

func TestCommentListShiftedDefaultPeriod(t *testing.T) {
	h := newPeriodHarness(t, 28, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
	ctx := context.Background()
	for _, period := range []string{"2026-10-01", "2026-11-01"} {
		if _, err := h.svc.CreateComment(ctx, h.owner, model.CreateCommentRequest{
			Id: vo.NewId().String(), BudgetId: h.budgetID, ElementId: h.category, Period: period, Comment: period,
		}); err != nil {
			t.Fatal(err)
		}
	}
	res, err := h.svc.GetCommentList(ctx, h.owner, model.GetCommentListRequest{BudgetId: h.budgetID, Months: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Period != "2026-11-01" {
		t.Fatalf("comments = %+v, want only the current period's (2026-11-01)", res.Items)
	}
}

func TestGetBudgetSummaryCallerWallClock(t *testing.T) {
	h := newPeriodHarness(t, 28, time.Date(2026, 10, 27, 22, 0, 0, 0, time.UTC))
	h.spend("40", "2026-10-28 00:00:00")
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	res := h.get(reqctx.WithLocation(context.Background(), tokyo), h.owner, "")
	sameWindow(t, res, "2026-10-28 00:00:00", "")
	if len(res.Balances) == 0 {
		t.Fatal("no balances")
	}
	b := res.Balances[0]
	if b.StartBalance == nil || b.Income == nil || b.Expenses == nil || b.Exchanges == nil {
		t.Fatalf("tokyo: started period has null figures: %+v", b)
	}
	if b.EndBalance != nil {
		t.Errorf("tokyo: endBalance = %s, want null", *b.EndBalance)
	}
	sameAmount(t, "tokyo expenses", *b.Expenses, "40")

	utc := h.get(context.Background(), h.owner, "2026-11-01")
	ub := utc.Balances[0]
	if ub.StartBalance != nil || ub.Income != nil || ub.Expenses != nil || ub.Exchanges != nil {
		t.Errorf("utc: period not started yet has figures: %+v", ub)
	}
}
