package budget_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/econumo/econumo/internal/shared/errs"
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

// periodHarness is a budget started on key 2026-09-01 whose periods start on
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

	started := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
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

	oct := h.get(ctx, h.owner, "2026-10-01")
	sameWindow(t, oct, "2026-10-28 00:00:00", "2026-11-28 00:00:00")
	row := h.categoryRow(oct)
	sameAmount(t, "october spent", row.Spent, "70")
	// no plans: available = -(spentBefore + spent)
	sameAmount(t, "october available", row.Available, "-170")

	sep := h.get(ctx, h.owner, "2026-09-01")
	sameWindow(t, sep, "2026-09-28 00:00:00", "2026-10-28 00:00:00")
	sameAmount(t, "september spent", h.categoryRow(sep).Spent, "100")
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

	res := h.get(context.Background(), partner, "2026-10-01")
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
		BudgetId: h.budgetID, PeriodStart: "2026-10-01", CategoryId: &cat,
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
	h.exec(`UPDATE budgets SET ended_at = ? WHERE id = ?`, "2026-10-01 00:00:00", h.budgetID)
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
		sameAmount(t, "key 2026-10 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-10-01")).Upcoming, "0")
		sameAmount(t, "key 2026-11 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-11-01")).Upcoming, "40")
	})
	t.Run("calendar", func(t *testing.T) {
		h := newPeriodHarness(t, 1, now)
		h.monthly("40", time.Date(2026, 11, 29, 0, 0, 0, 0, time.UTC))
		sameAmount(t, "key 2026-11 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-11-01")).Upcoming, "40")
	})
	// Overdue since 2026-10-20: filed today, in the current period (key
	// 2026-10) next to its 2026-11-20 occurrence, and nowhere else.
	t.Run("overdue", func(t *testing.T) {
		h := newPeriodHarness(t, 28, now)
		h.monthly("7", time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC))
		sameAmount(t, "key 2026-09 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-09-01")).Upcoming, "0")
		sameAmount(t, "key 2026-10 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-10-01")).Upcoming, "14")
		sameAmount(t, "key 2026-11 upcoming", h.categoryRow(h.get(ctx, h.owner, "2026-11-01")).Upcoming, "7")
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

	oct := h.savingsRow(h.get(ctx, h.owner, "2026-10-01"))
	sameAmount(t, "october saved", oct.Spent, "30")
	sameAmount(t, "october closing", oct.ClosingBalance, "30")

	sep := h.savingsRow(h.get(ctx, h.owner, "2026-09-01"))
	sameAmount(t, "september saved", sep.Spent, "0")
	sameAmount(t, "september closing", sep.ClosingBalance, "0")

	// The current period (key 2026-11, from 2026-11-28) plans 100 and has
	// saved 30 on 2026-11-29: the projected closing is booked 60 plus the
	// unmet 70.
	element := h.f.BudgetElement(fixture.BudgetElement{BudgetID: h.budgetID, ExternalID: h.savings, Type: int(model.ElementSavings)})
	h.f.BudgetLimit(fixture.BudgetLimit{ElementID: element, Period: "2026-11-01 00:00:00", Amount: "100"})
	h.f.Transaction(fixture.Transaction{
		UserID: h.owner.String(), AccountID: h.account, AccountRecipientID: h.savings, Type: 2,
		Amount: "30", AmountRecipient: "30", SpentAt: "2026-11-29 09:00:00",
	})
	nov := h.savingsRow(h.get(ctx, h.owner, "2026-11-01"))
	sameAmount(t, "november saved", nov.Spent, "30")
	sameAmount(t, "november closing", nov.ClosingBalance, "130")
}

func TestCreateBudgetShiftedDefaultStart(t *testing.T) {
	h := newPeriodHarness(t, 1, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
	h.spend("40", "2026-10-10 10:00:00") // September's period, before the budget begins
	res, err := h.svc.CreateBudget(context.Background(), h.owner, model.CreateBudgetRequest{
		Id: vo.NewId().String(), Name: "Payday", CurrencyId: fixture.USD, AccountIds: []string{h.account}, PeriodStartDay: 28,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.Meta.StartedAt != "2026-10-01 00:00:00" {
		t.Errorf("startedAt = %s, want the current period's key 2026-10-01", res.Item.Meta.StartedAt)
	}
	sameWindow(t, res.Item, "2026-10-28 00:00:00", "2026-11-28 00:00:00")
	sameAmount(t, "available (no earlier period carried over)", h.categoryRow(res.Item).Available, "0")
}

func TestCommentListShiftedDefaultPeriod(t *testing.T) {
	h := newPeriodHarness(t, 28, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
	ctx := context.Background()
	for _, period := range []string{"2026-09-01", "2026-10-01"} {
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
	if len(res.Items) != 1 || res.Items[0].Period != "2026-10-01" {
		t.Fatalf("comments = %+v, want only the current period's (2026-10-01)", res.Items)
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

	utc := h.get(context.Background(), h.owner, "2026-10-01")
	ub := utc.Balances[0]
	if ub.StartBalance != nil || ub.Income != nil || ub.Expenses != nil || ub.Exchanges != nil {
		t.Errorf("utc: period not started yet has figures: %+v", ub)
	}
}

func (h *periodHarness) setDay(userID vo.Id, day int) (model.MetaResult, error) {
	h.t.Helper()
	res, err := h.svc.UpdateBudget(context.Background(), userID, model.UpdateBudgetRequest{
		Id: h.budgetID, Name: "Budget", CurrencyId: fixture.USD, PeriodStartDay: &day,
	})
	if err != nil {
		return model.MetaResult{}, err
	}
	return res.Item, nil
}

func (h *periodHarness) mustSetDay(day int) string {
	h.t.Helper()
	meta, err := h.setDay(h.owner, day)
	if err != nil {
		h.t.Fatalf("update to day %d: %v", day, err)
	}
	return meta.StartedAt
}

func (h *periodHarness) setLimit(budgetID, period, amount string) {
	h.t.Helper()
	a := vo.NewFlexString(amount)
	if _, err := h.svc.SetLimit(context.Background(), h.owner, model.SetLimitRequest{
		BudgetId: budgetID, ElementId: h.category, Period: period, Amount: &a,
	}); err != nil {
		h.t.Fatalf("set-limit %s on %s: %v", amount, period, err)
	}
}

func (h *periodHarness) comment(budgetID, period, text string) {
	h.t.Helper()
	if _, err := h.svc.CreateComment(context.Background(), h.owner, model.CreateCommentRequest{
		Id: vo.NewId().String(), BudgetId: budgetID, ElementId: h.category, Period: period, Comment: text,
	}); err != nil {
		h.t.Fatalf("comment on %s: %v", period, err)
	}
}

// A brand-new day-1 budget (key October, today 5 October) switched to day 28:
// its first period stays October (28 Oct - 27 Nov) and today falls before it,
// so the budget has not started yet. The default get-budget and plan open on
// October, not on September, and October takes plans and comments.
func TestPeriodStartDayNotStartedYet(t *testing.T) {
	h := newPeriodHarness(t, 1, time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))
	h.exec(`UPDATE budgets SET started_at = ? WHERE id = ?`, "2026-10-01 00:00:00", h.budgetID)
	ctx := context.Background()

	if got := h.mustSetDay(28); got != "2026-10-01 00:00:00" {
		t.Fatalf("startedAt = %s, want the unchanged first key 2026-10-01", got)
	}
	sameWindow(t, h.get(ctx, h.owner, ""), "2026-10-28 00:00:00", "2026-11-28 00:00:00")
	plan, err := h.svc.GetBudgetPlan(ctx, h.owner, model.GetBudgetPlanRequest{Id: h.budgetID, Months: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Item.Months) == 0 || plan.Item.Months[0] != "2026-10-01" {
		t.Fatalf("plan months = %v, want to open on 2026-10-01", plan.Item.Months)
	}
	h.setLimit(h.budgetID, "2026-10-01", "100")
	h.comment(h.budgetID, "2026-10-01", "first")
	sameAmount(t, "october budgeted", h.categoryRow(h.get(ctx, h.owner, "2026-10-01")).Budgeted, "100")
}

// Changing the day and changing it back leaves every figure as it was.
func TestPeriodStartDayRoundTripRestores(t *testing.T) {
	h := newPeriodHarness(t, 1, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	h.exec(`UPDATE budgets SET started_at = ? WHERE id = ?`, "2026-10-01 00:00:00", h.budgetID)
	h.spend("30", "2026-10-05 10:00:00")
	h.spend("7", "2026-09-29 10:00:00")
	h.setLimit(h.budgetID, "2026-10-01", "40")
	ctx := context.Background()
	snapshot := func() string {
		t.Helper()
		out := ""
		for _, date := range []string{"", "2026-10-01", "2026-11-01"} {
			b, err := json.Marshal(h.get(ctx, h.owner, date))
			if err != nil {
				t.Fatal(err)
			}
			out += string(b)
		}
		return out
	}
	before := snapshot()
	h.mustSetDay(28)
	h.mustSetDay(1)
	h.mustSetDay(28)
	h.mustSetDay(1)
	if after := snapshot(); after != before {
		t.Fatalf("day 1 -> 28 -> 1 changed get-budget:\nbefore %s\nafter  %s", before, after)
	}
}

// A budget created on 30 October on day 28 (first key October) switched to
// day 1 keeps October as its first key: its plans still count and no period
// is refused.
func TestPeriodStartDayKeepsTheFirstKey(t *testing.T) {
	h := newPeriodHarness(t, 1, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
	ctx := context.Background()
	res, err := h.svc.CreateBudget(ctx, h.owner, model.CreateBudgetRequest{
		Id: vo.NewId().String(), Name: "Payday", CurrencyId: fixture.USD, AccountIds: []string{h.account}, PeriodStartDay: 28,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.budgetID = res.Item.Meta.Id
	h.setLimit(h.budgetID, "2026-10-01", "100")
	if got := h.mustSetDay(1); got != "2026-10-01 00:00:00" {
		t.Fatalf("startedAt = %s, want 2026-10-01", got)
	}
	sameAmount(t, "october budgeted", h.categoryRow(h.get(ctx, h.owner, "")).Budgeted, "100")
	sameAmount(t, "november available (october's plan carried over)", h.categoryRow(h.get(ctx, h.owner, "2026-11-01")).Available, "100")
	h.setLimit(h.budgetID, "2026-10-01", "120")
	h.comment(h.budgetID, "2026-10-01", "still mine")
}

// An end month set before a day change stays valid after it.
func TestPeriodStartDayKeepsTheEndMonth(t *testing.T) {
	h := newPeriodHarness(t, 1, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	h.exec(`UPDATE budgets SET started_at = ?, ended_at = ? WHERE id = ?`, "2026-10-01 00:00:00", "2026-10-01 00:00:00", h.budgetID)
	ctx := context.Background()
	for _, day := range []int{28, 5, 1} {
		h.mustSetDay(day)
		end := "2026-10-01"
		if _, err := h.svc.UpdateBudget(ctx, h.owner, model.UpdateBudgetRequest{
			Id: h.budgetID, Name: "Budget", CurrencyId: fixture.USD, EndDate: &end,
		}); err != nil {
			t.Fatalf("day %d: end month refused: %v", day, err)
		}
		h.setLimit(h.budgetID, "2026-10-01", "10")
		if res := h.get(ctx, h.owner, ""); res.Meta.EndedAt != "2026-10-01 00:00:00" {
			t.Fatalf("day %d: endedAt = %s", day, res.Meta.EndedAt)
		}
	}
}

// create-budget on 20 October with start 2026-10-20 and day 28: the date names
// the key October; the budget has not started yet and opens on October.
func TestCreateBudgetExplicitStartNamesAKey(t *testing.T) {
	h := newPeriodHarness(t, 1, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	res, err := h.svc.CreateBudget(context.Background(), h.owner, model.CreateBudgetRequest{
		Id: vo.NewId().String(), Name: "Payday", CurrencyId: fixture.USD, AccountIds: []string{h.account},
		StartDate: "2026-10-20", PeriodStartDay: 28,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.Meta.StartedAt != "2026-10-01 00:00:00" {
		t.Errorf("startedAt = %s, want the key 2026-10-01", res.Item.Meta.StartedAt)
	}
	sameWindow(t, res.Item, "2026-10-28 00:00:00", "2026-11-28 00:00:00")
}

// The Complete dialog's sequence against the real service on a day-28 budget
// with key October on 20 October (not started yet): end with October, archive,
// and continue in a copy starting there; the first key's plan and comment are
// copied, as they are by a full copy.
func TestCompleteShiftedBudgetFlow(t *testing.T) {
	h := newPeriodHarness(t, 28, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	h.exec(`UPDATE budgets SET started_at = ? WHERE id = ?`, "2026-10-01 00:00:00", h.budgetID)
	ctx := context.Background()
	h.setLimit(h.budgetID, "2026-10-01", "100")
	h.comment(h.budgetID, "2026-10-01", "rent")

	full, err := h.svc.CloneBudget(ctx, h.owner, model.CloneBudgetRequest{
		Id: h.budgetID, NewId: vo.NewId().String(), Name: "Full", WithLimits: true,
	})
	if err != nil {
		t.Fatalf("full copy: %v", err)
	}

	end := "2026-10-01"
	if _, err := h.svc.UpdateBudget(ctx, h.owner, model.UpdateBudgetRequest{
		Id: h.budgetID, Name: "Budget", CurrencyId: fixture.USD, EndDate: &end,
	}); err != nil {
		t.Fatalf("end with October: %v", err)
	}
	if _, err := h.svc.ArchiveBudget(ctx, h.owner, model.ArchiveBudgetRequest{Id: h.budgetID}); err != nil {
		t.Fatalf("archive: %v", err)
	}
	next, err := h.svc.CloneBudget(ctx, h.owner, model.CloneBudgetRequest{
		Id: h.budgetID, NewId: vo.NewId().String(), Name: "Next", StartDate: "2026-10-01", WithLimits: true,
	})
	if err != nil {
		t.Fatalf("continue in a copy: %v", err)
	}
	if _, err := h.svc.CloneBudget(ctx, h.owner, model.CloneBudgetRequest{
		Id: h.budgetID, NewId: vo.NewId().String(), Name: "Later", StartDate: "2026-11-01",
	}); err == nil {
		t.Fatal("a copy starting after the current period and the first key was accepted")
	}

	for name, copy := range map[string]model.BudgetResult{"full copy": full.Item, "continuation": next.Item} {
		if copy.Meta.StartedAt != "2026-10-01 00:00:00" {
			t.Errorf("%s: startedAt = %s, want 2026-10-01", name, copy.Meta.StartedAt)
		}
		sameWindow(t, copy, "2026-10-28 00:00:00", "2026-11-28 00:00:00")
		sameAmount(t, name+" october budgeted", h.categoryRow(copy).Budgeted, "100")
		comments, err := h.svc.GetCommentList(ctx, h.owner, model.GetCommentListRequest{BudgetId: copy.Meta.Id, From: "2026-10-01", Months: "1"})
		if err != nil {
			t.Fatal(err)
		}
		if len(comments.Items) != 1 || comments.Items[0].Period != "2026-10-01" {
			t.Errorf("%s: comments = %+v, want the October one", name, comments.Items)
		}
	}
}

// A legacy mid-month start on a day-28 budget: the first key is its month
// (15 Sep -> September, 28 Sep - 27 Oct) and earlier windows are plain period
// windows, never shifted again by the start's offset.
func TestCarryOverLegacyMidMonthStartShifted(t *testing.T) {
	h := newPeriodHarness(t, 28, periodNow)
	h.exec(`UPDATE budgets SET started_at = ? WHERE id = ?`, "2026-09-15 00:00:00", h.budgetID)
	h.spend("1", "2026-09-27 10:00:00")   // August's period, before the budget
	h.spend("10", "2026-10-10 10:00:00")  // September
	h.spend("100", "2026-10-29 10:00:00") // October
	row := h.categoryRow(h.get(context.Background(), h.owner, "2026-10-01"))
	sameAmount(t, "october spent", row.Spent, "100")
	sameAmount(t, "october available", row.Available, "-110")
}

// The day reshapes the budget for every participant: owner or admin only; a
// user-role participant may still rename it.
func TestUpdatePeriodStartDayNeedsOwnerOrAdmin(t *testing.T) {
	h := newPeriodHarness(t, 1, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	admin := vo.MustParseId(h.f.User(fixture.User{Name: "Admin", Email: "admin@period.test"}))
	h.f.BudgetAccess(h.budgetID, admin.String(), int(model.BudgetRoleAdmin), true)
	member := vo.MustParseId(h.f.User(fixture.User{Name: "Member", Email: "member@period.test"}))
	h.f.BudgetAccess(h.budgetID, member.String(), int(model.BudgetRoleUser), true)

	_, err := h.setDay(member, 28)
	var denied *errs.AccessDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("user role changing the day: err = %v, want access denied", err)
	}
	if got := h.get(context.Background(), h.owner, "").Meta.PeriodStartDay; got != 1 {
		t.Fatalf("a refused change wrote day %d", got)
	}
	// the stored day, sent back unchanged, is no change
	if _, err := h.setDay(member, 1); err != nil {
		t.Fatalf("user role resending the stored day: %v", err)
	}
	meta, err := h.setDay(admin, 28)
	if err != nil || meta.PeriodStartDay != 28 {
		t.Fatalf("admin changing the day: meta=%+v err=%v", meta, err)
	}
}
