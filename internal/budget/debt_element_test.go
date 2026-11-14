package budget_test

import (
	"context"
	"errors"
	"testing"
	"time"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appbudget "github.com/econumo/econumo/internal/budget"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	domcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	appdebt "github.com/econumo/econumo/internal/debt"
	debtrepo "github.com/econumo/econumo/internal/debt/repo"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
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

// debtHarness is a budget from November 2025 over two member accounts (USD
// and EUR) wired to the real debt service, so debts attach to its elements as
// in production. Today is 2026-06-15.
type debtHarness struct {
	ctx      context.Context
	tdb      *dbtest.DB
	svc      *appbudget.Service
	debts    *appdebt.Service
	f        *fixture.Builder
	owner    vo.Id
	budgetID string
	usd, eur string // accounts
	eurCur   string
	loan     string // expense categories
	food     string
	rent     string
	tag      string
}

var debtToday = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

func newDebtHarness(t *testing.T) *debtHarness {
	t.Helper()
	db := dbtest.New(t)
	txm := db.TX
	engine := db.Engine
	f := fixture.New(t, db)
	clk := fixedClock{t: debtToday}

	owner := f.User(fixture.User{Name: "Owner", Email: "owner@debt-element.test"})
	eurCur := f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
	f.Rate(fixture.Rate{CurrencyID: eurCur, BaseCurrencyID: fixture.USD, Rate: "0.50000000", PublishedAt: "2026-06-01"})
	usd := f.Account(fixture.Account{UserID: owner, Name: "Cash"})
	eur := f.Account(fixture.Account{UserID: owner, Name: "Euro", CurrencyID: eurCur})
	budgetID := f.Budget(fixture.Budget{UserID: owner, StartedAt: time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)})
	f.BudgetAccount(budgetID, usd)
	f.BudgetAccount(budgetID, eur)
	loan := f.Category(fixture.Category{UserID: owner, Name: "Mortgage", Position: 1})
	food := f.Category(fixture.Category{UserID: owner, Name: "Food", Position: 2})
	rent := f.Category(fixture.Category{UserID: owner, Name: "Rent", Position: 3})
	tag := f.Tag(fixture.Tag{UserID: owner, Name: "Home"})

	userRepo := userrepo.NewRepo(engine, txm)
	categoryRepo := categoryrepo.NewRepo(engine, txm)
	currencyLookup := currencyrepo.New(engine, txm)
	rateProvider := currencyrepo.NewRateProvider(engine, txm, currencyLookup, fixture.USD)
	convertor := domcurrency.NewConvertor(rateProvider)

	svc := appbudget.NewService(
		budgetrepo.NewRepo(engine, txm), budgetrepo.NewReadRepo(engine, txm), convertor, rateProvider,
		server.NewBudgetUserLookup(userRepo, clk),
		server.NewBudgetAccountLookup(accountrepo.NewRepo(engine, txm)),
		server.NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(server.NewBudgetCategoryMetadataLookup(categoryRepo), server.NewBudgetTagMetadataLookup(tagrepo.NewRepo(engine, txm)), server.NewBudgetPayeeMetadataLookup(payeerepo.NewRepo(engine, txm))),
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(engine, txm)),
		operationrepo.NewGuard(engine, txm),
		txm, clk,
	)
	debts := appdebt.NewService(debtrepo.NewRepo(engine, txm), txm, clk,
		server.NewDebtCategories(categoryRepo), server.NewDebtRates(rateProvider, convertor), server.NewDebtUsers(userRepo))
	svc.SetDebts(debts)
	debts.SetBudgets(server.NewDebtBudgets(svc))
	debts.SetElements(server.NewDebtElements(svc))

	return &debtHarness{
		ctx: context.Background(), tdb: db, svc: svc, debts: debts, f: f,
		owner: vo.MustParseId(owner), budgetID: budgetID, usd: usd, eur: eur, eurCur: eurCur,
		loan: loan, food: food, rent: rent, tag: tag,
	}
}

func (h *debtHarness) spend(account, category, tag, amount string, at time.Time) string {
	return h.f.Transaction(fixture.Transaction{
		UserID: h.owner.String(), AccountID: account, CategoryID: category, TagID: tag,
		Type: 0, Amount: amount, SpentAt: at,
	})
}

// setUpDebt tracks a debt on the element and sets it up with the loan's
// facts: amount borrowed, start and last payment date, paid monthly.
func (h *debtHarness) setUpDebt(t *testing.T, elementID, start, borrowed, payment, last string) string {
	t.Helper()
	res, err := h.debts.CreateDebt(h.ctx, h.owner, model.CreateDebtRequest{
		BudgetId: h.budgetID, ElementId: elementID, Payment: vo.NewFlexString(payment), Schedule: "monthly",
	})
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}
	if _, err := h.debts.UpdateDebt(h.ctx, h.owner, model.UpdateDebtRequest{
		Id: res.Id, StartDate: &start, AmountBorrowed: vo.NewFlexString(borrowed), LastPaymentDate: last,
	}); err != nil {
		t.Fatalf("UpdateDebt: %v", err)
	}
	return res.Id
}

// looseDebt tracks a debt that is not set up, counting from start (nil: from
// its first period by the debt service's rule).
func (h *debtHarness) looseDebt(t *testing.T, elementID string, start *string) string {
	t.Helper()
	res, err := h.debts.CreateDebt(h.ctx, h.owner, model.CreateDebtRequest{
		BudgetId: h.budgetID, ElementId: elementID, Payment: vo.NewFlexString("500"), Schedule: "monthly", StartDate: start,
	})
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}
	return res.Id
}

func (h *debtHarness) envelope(t *testing.T, name string, categories ...string) string {
	t.Helper()
	id := vo.NewId().String()
	if _, err := h.svc.CreateEnvelope(h.ctx, h.owner, model.CreateEnvelopeRequest{
		BudgetId: h.budgetID, Id: id, Name: name, Icon: "home", CurrencyId: fixture.USD, Categories: categories,
	}); err != nil {
		t.Fatalf("CreateEnvelope: %v", err)
	}
	return id
}

func (h *debtHarness) setLimit(t *testing.T, elementID, period, amount string) {
	t.Helper()
	a := vo.NewFlexString(amount)
	if _, err := h.svc.SetLimit(h.ctx, h.owner, model.SetLimitRequest{
		BudgetId: h.budgetID, ElementId: elementID, Period: period, Amount: &a,
	}); err != nil {
		t.Fatalf("SetLimit %s: %v", period, err)
	}
}

func (h *debtHarness) budgetAt(t *testing.T, date string) *model.GetBudgetResult {
	t.Helper()
	res, err := h.svc.GetBudget(h.ctx, h.owner, model.GetBudgetRequest{Id: h.budgetID, Date: date})
	if err != nil {
		t.Fatalf("GetBudget %s: %v", date, err)
	}
	return res
}

func (h *debtHarness) plan(t *testing.T, from, months string) *model.GetBudgetPlanResult {
	t.Helper()
	res, err := h.svc.GetBudgetPlan(h.ctx, h.owner, model.GetBudgetPlanRequest{Id: h.budgetID, From: from, Months: months})
	if err != nil {
		t.Fatalf("GetBudgetPlan: %v", err)
	}
	return res
}

func (h *debtHarness) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := h.tdb.Raw.QueryRowContext(h.ctx, h.tdb.Rebind(query), args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func findElement(res *model.GetBudgetResult, id string, typ model.ElementType) *model.ParentElementResult {
	for i := range res.Item.Structure.Elements {
		el := &res.Item.Structure.Elements[i]
		if el.Id == id && el.Type == int(typ.Int16()) {
			return el
		}
	}
	return nil
}

func findPlanRow(res *model.GetBudgetPlanResult, id string) *model.PlanElementResult {
	for i := range res.Item.Structure.Elements {
		if res.Item.Structure.Elements[i].Id == id {
			return &res.Item.Structure.Elements[i]
		}
	}
	return nil
}

func debtIDOf(d *model.ElementDebtResult) string {
	if d == nil {
		return ""
	}
	return d.Id
}

func requireFieldCode(t *testing.T, err error, field, code string) {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a validation error %s on %s", err, code, field)
	}
	for _, fe := range ve.Fields {
		if fe.Key == field && fe.Code == code {
			return
		}
	}
	t.Fatalf("err = %+v, want %s on %s", ve.Fields, code, field)
}

// A debt is information on top of its element: the element's Spent, its
// envelope's and its folder's totals keep every payment.
func TestGetBudget_PaymentsStayOnTheirElement(t *testing.T) {
	h := newDebtHarness(t)
	folderID := vo.NewId().String()
	if _, err := h.svc.CreateFolder(h.ctx, h.owner, model.CreateBudgetFolderRequest{BudgetId: h.budgetID, Id: folderID, Name: "Base", Side: "expense"}); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if _, err := h.svc.MoveElement(h.ctx, h.owner, model.MoveElementRequest{BudgetId: h.budgetID, Id: h.loan, FolderId: &folderID}); err != nil {
		t.Fatalf("MoveElement: %v", err)
	}
	housing := h.envelope(t, "Housing", h.rent)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.eur, h.loan, "", "100", time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.rent, "", "700", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.food, "", "50", time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC))
	h.setUpDebt(t, h.loan, "2026-01-01", "12000", "1000", "2027-01-01")
	h.looseDebt(t, housing, nil)

	res := h.budgetAt(t, "2026-06-01")
	loan := findElement(res, h.loan, model.ElementCategory)
	if loan == nil || loan.Spent != "1000" || loan.BudgetSpent != "1000" || loan.Available != "-1200" {
		t.Fatalf("loan row = %+v, want its 1000 spent and May's 200 carried", loan)
	}
	if loan.FolderId == nil || *loan.FolderId != folderID {
		t.Errorf("loan row folder = %v, want it to stay in its folder", loan.FolderId)
	}
	env := findElement(res, housing, model.ElementEnvelope)
	if env == nil || env.Spent != "700" || len(env.Children) != 1 || env.Children[0].Spent != "700" {
		t.Fatalf("envelope row = %+v, want the rent counted there", env)
	}
	total := vo.NewDecimal("0")
	for _, el := range res.Item.Structure.Elements {
		total = total.Add(vo.NewDecimal(el.BudgetSpent))
	}
	if total.String() != "1750" {
		t.Errorf("rows add up to %s, want every expense once (1750)", total)
	}

	plan := h.plan(t, "2026-05-01", "2")
	if row := findPlanRow(plan, h.loan); row == nil || row.Cells[0].Actual != "200" || row.Cells[1].Actual != "1000" {
		t.Errorf("plan loan row = %+v, want 200 and 1000", row)
	}
	if row := findPlanRow(plan, housing); row == nil || row.Cells[1].Actual != "700" {
		t.Errorf("plan envelope row = %+v, want 700 in June", row)
	}
}

func TestGetBudget_DebtObjectOnTheElement(t *testing.T) {
	h := newDebtHarness(t)
	housing := h.envelope(t, "Housing", h.rent)
	loanDebt := h.setUpDebt(t, h.loan, "2026-06-01", "12000", "1000", "2027-06-01")
	envDebt := h.looseDebt(t, housing, nil)

	res := h.budgetAt(t, "2026-06-01")
	loan := findElement(res, h.loan, model.ElementCategory)
	if loan == nil || loan.Debt == nil {
		t.Fatalf("loan row = %+v, want its debt on it", loan)
	}
	d := loan.Debt
	if d.Id != loanDebt || d.IsSetUp != 1 || d.IsPaidOff != 0 || d.PaymentsLeft != 12 || d.PayoffDate != "2027-06-01" || d.BasePayoffDate != "2027-06-01" || d.ClosingBalance != "12000" {
		t.Errorf("loan debt = %+v, want 12 payments left, paid off 2027-06-01", d)
	}

	env := findElement(res, housing, model.ElementEnvelope)
	if env == nil || env.Debt == nil || env.Debt.Id != envDebt {
		t.Fatalf("envelope row = %+v, want its debt", env)
	}
	if e := env.Debt; e.IsSetUp != 0 || e.ClosingBalance != "" || e.PaymentsLeft != 0 || e.ScheduledTotal != "0" || e.Upcoming != "0" || e.PayoffDate != "" || e.OverpaymentLeft != "" {
		t.Errorf("envelope debt = %+v, want the not-set-up encoding", e)
	}
	if child := env.Children; len(child) != 1 {
		t.Errorf("envelope children = %+v", child)
	}
	if food := findElement(res, h.food, model.ElementCategory); food == nil || food.Debt != nil {
		t.Errorf("food row = %+v, want no debt", food)
	}

	// a planned extra moves the payoff earlier
	h.setLimit(t, h.loan, "2026-07-01", "4000")
	d = findElement(h.budgetAt(t, "2026-06-01"), h.loan, model.ElementCategory).Debt
	if d.PayoffDate >= d.BasePayoffDate || d.BasePayoffDate != "2027-06-01" {
		t.Errorf("with a planned extra: payoff %s, base %s", d.PayoffDate, d.BasePayoffDate)
	}
}

// Review Focus 2: a past period shows the debt that was in force then.
func TestGetBudget_DebtInForceByPeriod(t *testing.T) {
	h := newDebtHarness(t)
	old := h.setUpDebt(t, h.loan, "2025-12-01", "10000", "1000", "2026-10-01")
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: old, Date: "2026-02-01"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	from := "2026-04-01"
	newer := h.looseDebt(t, h.loan, &from)

	for _, c := range []struct{ month, want string }{
		{"2025-11-01", ""}, {"2026-01-01", old}, {"2026-02-01", old}, {"2026-03-01", ""}, {"2026-04-01", newer}, {"2026-06-01", newer},
	} {
		res := h.budgetAt(t, c.month)
		el := findElement(res, h.loan, model.ElementCategory)
		var got *model.ElementDebtResult
		if el != nil {
			got = el.Debt
		}
		if debtIDOf(got) != c.want {
			t.Errorf("%s debt = %+v, want %q", c.month, got, c.want)
		}
	}
	jan := findElement(h.budgetAt(t, "2026-01-01"), h.loan, model.ElementCategory).Debt
	if jan.IsPaidOff != 1 || jan.IsSetUp != 1 {
		t.Errorf("January's debt = %+v, want the paid-off one", jan)
	}
}

func TestGetBudgetPlan_CellsCarryTheDebtInForce(t *testing.T) {
	h := newDebtHarness(t)
	old := h.setUpDebt(t, h.loan, "2025-12-01", "10000", "1000", "2026-10-01")
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: old, Date: "2026-02-01"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	newer := h.setUpDebt(t, h.loan, "2026-04-01", "6000", "1000", "2026-10-01")

	row := findPlanRow(h.plan(t, "2026-01-01", "6"), h.loan)
	if row == nil {
		t.Fatal("no plan row for the loan")
	}
	want := []string{old, old, "", newer, newer, newer}
	for i, w := range want {
		if got := debtIDOf(row.Cells[i].Debt); got != w {
			t.Errorf("cell %d debt = %q, want %q", i, got, w)
		}
	}
	if row.Debt == nil || row.Debt.Id != newer || row.Debt.PayoffDate == "" {
		t.Errorf("row debt = %+v, want the active one with its payoff", row.Debt)
	}
	if c := row.Cells[5].Debt; c.ScheduledTotal != "1000" || c.ClosingBalance == "" {
		t.Errorf("June cell = %+v, want its schedule and balance", c)
	}
	if food := findPlanRow(h.plan(t, "2026-01-01", "6"), h.food); food == nil || food.Debt != nil || food.Cells[0].Debt != nil {
		t.Errorf("food plan row = %+v, want no debt", food)
	}
}

// Review Focus 4.
func TestMoveIntoEnvelope_DebtRules(t *testing.T) {
	h := newDebtHarness(t)
	housing := h.envelope(t, "Housing", h.rent)
	h.looseDebt(t, h.loan, nil)

	_, err := h.svc.MoveElement(h.ctx, h.owner, model.MoveElementRequest{BudgetId: h.budgetID, Id: h.loan, EnvelopeId: &housing})
	requireFieldCode(t, err, "id", errs.CodeBudgetElementHasDebt)
	_, err = h.svc.CreateEnvelope(h.ctx, h.owner, model.CreateEnvelopeRequest{
		BudgetId: h.budgetID, Id: vo.NewId().String(), Name: "Loans", Icon: "bank", CurrencyId: fixture.USD, Categories: []string{h.loan},
	})
	requireFieldCode(t, err, "categories", errs.CodeBudgetElementHasDebt)
	_, err = h.svc.UpdateEnvelope(h.ctx, h.owner, model.UpdateEnvelopeRequest{
		BudgetId: h.budgetID, Id: housing, Name: "Housing", Icon: "home", CurrencyId: fixture.USD, Categories: []string{h.rent, h.loan},
	})
	requireFieldCode(t, err, "categories", errs.CodeBudgetElementHasDebt)
	if n := h.count(t, "SELECT COUNT(*) FROM budgets_envelopes_categories WHERE category_id = ?", h.loan); n != 0 {
		t.Fatalf("the loan joined %d envelopes", n)
	}

	// only paid-off history: it may move
	paid := h.setUpDebt(t, h.food, "2026-01-01", "5000", "1000", "2026-06-01")
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: paid, Date: "2026-03-01"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	if _, err := h.svc.MoveElement(h.ctx, h.owner, model.MoveElementRequest{BudgetId: h.budgetID, Id: h.food, EnvelopeId: &housing}); err != nil {
		t.Fatalf("moving a category with only paid-off debts: %v", err)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM budgets_envelopes_categories WHERE budget_envelope_id = ? AND category_id = ?", housing, h.food); n != 1 {
		t.Errorf("food in the envelope = %d, want 1", n)
	}
}

// Review Focus 5.
func TestChangeElementCurrency_MovesOnlyTheActiveDebt(t *testing.T) {
	h := newDebtHarness(t)
	paid := h.setUpDebt(t, h.loan, "2026-01-01", "5000", "1000", "2026-06-01")
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: paid, Date: "2026-03-01"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	from := "2026-04-01"
	active := h.looseDebt(t, h.loan, &from)

	if _, err := h.svc.ChangeElementCurrency(h.ctx, h.owner, model.ChangeElementCurrencyRequest{BudgetId: h.budgetID, ElementId: h.loan, CurrencyId: h.eurCur}); err != nil {
		t.Fatalf("ChangeElementCurrency: %v", err)
	}
	currencyOf := func(id string) string {
		t.Helper()
		var c string
		if err := h.tdb.Raw.QueryRowContext(h.ctx, h.tdb.Rebind("SELECT currency_id FROM debts WHERE id = ?"), id).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if got := currencyOf(active); got != h.eurCur {
		t.Errorf("active debt currency = %s, want EUR", got)
	}
	if got := currencyOf(paid); got != fixture.USD {
		t.Errorf("paid-off debt currency = %s, want it kept in USD", got)
	}
}

func TestDeleteEnvelope_DeletesItsDebts(t *testing.T) {
	h := newDebtHarness(t)
	housing := h.envelope(t, "Housing", h.rent)
	h.looseDebt(t, housing, nil)
	kept := h.looseDebt(t, h.loan, nil)

	if _, err := h.svc.DeleteEnvelope(h.ctx, h.owner, model.DeleteEnvelopeRequest{BudgetId: h.budgetID, Id: housing}); err != nil {
		t.Fatalf("DeleteEnvelope: %v", err)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM debts WHERE element_external_id = ?", housing); n != 0 {
		t.Errorf("%d debts of the deleted envelope left", n)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM debts WHERE id = ?", kept); n != 1 {
		t.Error("the category's debt went too")
	}
}

func TestCloneBudget_CopiesDebtsOnElements(t *testing.T) {
	h := newDebtHarness(t)
	housing := h.envelope(t, "Housing", h.rent)
	h.setUpDebt(t, h.loan, "2026-01-01", "12000", "1000", "2027-01-01")
	h.looseDebt(t, housing, nil)

	newID := vo.NewId().String()
	res, err := h.svc.CloneBudget(h.ctx, h.owner, model.CloneBudgetRequest{Id: h.budgetID, NewId: newID, Name: "Copy", StartDate: "2026-06-01"})
	if err != nil {
		t.Fatalf("CloneBudget: %v", err)
	}
	var copyEnvelope string
	for _, el := range res.Item.Structure.Elements {
		if el.Type == int(model.ElementEnvelope.Int16()) {
			copyEnvelope = el.Id
			if el.Debt == nil || el.Debt.IsSetUp != 0 {
				t.Errorf("the copy's envelope = %+v, want its copied debt", el)
			}
		}
		if el.Id == h.loan && (el.Debt == nil || el.Debt.IsSetUp != 1) {
			t.Errorf("the copy's loan row = %+v, want its copied debt", el)
		}
	}
	if copyEnvelope == "" || copyEnvelope == housing {
		t.Fatalf("the copy's envelope id = %q", copyEnvelope)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM debts WHERE budget_id = ? AND element_external_id = ? AND element_type = 'envelope'", newID, copyEnvelope); n != 1 {
		t.Errorf("debts on the copy's envelope = %d, want 1", n)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM debts WHERE budget_id = ? AND element_external_id = ?", newID, h.loan); n != 1 {
		t.Errorf("debts on the copy's loan = %d, want 1", n)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM debts WHERE budget_id = ?", h.budgetID); n != 2 {
		t.Errorf("source debts = %d, want 2", n)
	}
}

func TestGetTransactionList_ElementListsItsPayments(t *testing.T) {
	h := newDebtHarness(t)
	paid := h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	h.setUpDebt(t, h.loan, "2026-01-01", "12000", "1000", "2027-01-01")
	res, err := h.svc.GetTransactionList(h.ctx, h.owner, model.BudgetTransactionListRequest{BudgetId: h.budgetID, PeriodStart: "2026-06-01", CategoryId: &h.loan})
	if err != nil {
		t.Fatalf("GetTransactionList: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Id != paid {
		t.Errorf("loan list = %+v, want its payment", res.Items)
	}
}

// A budget whose periods start on the 5th: a debt ended on 3 February is in
// force in the period keyed January (5 January to 4 February), and one that
// starts on 3 April in the period keyed March.
func TestGetBudget_DebtInForceByPeriod_ShiftedPeriods(t *testing.T) {
	h := newDebtHarness(t)
	if _, err := h.tdb.Raw.ExecContext(h.ctx, h.tdb.Rebind("UPDATE budgets SET period_start_day = 5 WHERE id = ?"), h.budgetID); err != nil {
		t.Fatal(err)
	}
	old := h.setUpDebt(t, h.loan, "2025-12-10", "10000", "1000", "2026-10-10")
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: old, Date: "2026-02-03"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	from := "2026-04-03"
	newer := h.looseDebt(t, h.loan, &from)

	for _, c := range []struct{ month, want string }{
		{"2025-11-01", ""}, {"2025-12-01", old}, {"2026-01-01", old}, {"2026-02-01", ""}, {"2026-03-01", newer}, {"2026-04-01", newer},
	} {
		el := findElement(h.budgetAt(t, c.month), h.loan, model.ElementCategory)
		var got *model.ElementDebtResult
		if el != nil {
			got = el.Debt
		}
		if debtIDOf(got) != c.want {
			t.Errorf("%s debt = %+v, want %q", c.month, got, c.want)
		}
	}
}

// A paid-off debt whose category has since gone into an envelope cannot be
// reopened: the active debt would sit hidden on an envelope child.
func TestReopenDebt_RefusedOnceTheCategoryIsInAnEnvelope(t *testing.T) {
	h := newDebtHarness(t)
	housing := h.envelope(t, "Housing", h.rent)
	paid := h.setUpDebt(t, h.food, "2026-01-01", "5000", "1000", "2026-06-01")
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: paid, Date: "2026-03-01"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	if _, err := h.svc.MoveElement(h.ctx, h.owner, model.MoveElementRequest{BudgetId: h.budgetID, Id: h.food, EnvelopeId: &housing}); err != nil {
		t.Fatalf("MoveElement: %v", err)
	}
	_, err := h.debts.ReopenDebt(h.ctx, h.owner, model.ReopenDebtRequest{Id: paid})
	requireFieldCode(t, err, "elementId", errs.CodeDebtElementNotEligible)
	if n := h.count(t, "SELECT COUNT(*) FROM debts WHERE id = ? AND status = 'paid_off'", paid); n != 1 {
		t.Error("the refused reopen changed the debt")
	}
}

// update-envelope checks only the categories joining the envelope: one
// already in it never blocks a rename, while a new one with an active debt
// is still refused.
func TestUpdateEnvelope_ChecksOnlyNewCategories(t *testing.T) {
	h := newDebtHarness(t)
	housing := h.envelope(t, "Housing")
	h.looseDebt(t, h.rent, nil)
	h.looseDebt(t, h.loan, nil)
	// a state the API no longer reaches: an active debt on an envelope child
	h.f.EnvelopeCategory(housing, h.rent)

	if _, err := h.svc.UpdateEnvelope(h.ctx, h.owner, model.UpdateEnvelopeRequest{
		BudgetId: h.budgetID, Id: housing, Name: "Home", Icon: "home", CurrencyId: fixture.USD, Categories: []string{h.rent},
	}); err != nil {
		t.Fatalf("renaming the envelope: %v", err)
	}
	if _, err := h.svc.MoveElement(h.ctx, h.owner, model.MoveElementRequest{BudgetId: h.budgetID, Id: h.rent, EnvelopeId: &housing}); err != nil {
		t.Fatalf("moving a child into its own envelope: %v", err)
	}
	_, err := h.svc.UpdateEnvelope(h.ctx, h.owner, model.UpdateEnvelopeRequest{
		BudgetId: h.budgetID, Id: housing, Name: "Home", Icon: "home", CurrencyId: fixture.USD, Categories: []string{h.rent, h.loan},
	})
	requireFieldCode(t, err, "categories", errs.CodeBudgetElementHasDebt)
}

func (h *debtHarness) debtCurrency(t *testing.T, id string) string {
	t.Helper()
	var c string
	if err := h.tdb.Raw.QueryRowContext(h.ctx, h.tdb.Rebind("SELECT currency_id FROM debts WHERE id = ?"), id).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

// A category row with no currency of its own is in the budget's, so its
// active debt follows a budget currency change; a row with its own currency
// keeps its debt where it is.
func TestUpdateBudget_CurrencyChangeMovesDebtsOnRowsWithoutOwnCurrency(t *testing.T) {
	h := newDebtHarness(t)
	loanDebt := h.setUpDebt(t, h.loan, "2026-01-01", "5000", "1000", "2026-06-01")
	housing := h.envelope(t, "Housing", h.rent)
	envDebt := h.looseDebt(t, housing, nil)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC))
	h.budgetAt(t, "2026-02-01")
	if n := h.count(t, "SELECT COUNT(*) FROM debts_payments WHERE debt_id = ?", loanDebt); n != 1 {
		t.Fatalf("splits before = %d, want 1", n)
	}

	if _, err := h.svc.UpdateBudget(h.ctx, h.owner, model.UpdateBudgetRequest{
		Id: h.budgetID, Name: "Budget", CurrencyId: h.eurCur,
	}); err != nil {
		t.Fatalf("UpdateBudget: %v", err)
	}
	if got := h.debtCurrency(t, loanDebt); got != h.eurCur {
		t.Errorf("category debt currency = %s, want EUR", got)
	}
	if got := h.debtCurrency(t, envDebt); got != fixture.USD {
		t.Errorf("envelope debt currency = %s, want its row's USD", got)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM debts_payments WHERE debt_id = ?", loanDebt); n != 0 {
		t.Errorf("splits after the currency change = %d, want them dropped", n)
	}
	h.budgetAt(t, "2026-02-01")
	if n := h.count(t, "SELECT COUNT(*) FROM debts_payments WHERE debt_id = ?", loanDebt); n != 1 {
		t.Errorf("splits after the next sync = %d, want the payment split again", n)
	}
}

func TestReopenDebt_FollowsTheElementCurrency(t *testing.T) {
	h := newDebtHarness(t)
	id := h.setUpDebt(t, h.loan, "2026-01-01", "5000", "1000", "2026-06-01")
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC))
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: id, Date: "2026-03-01"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	if _, err := h.svc.ChangeElementCurrency(h.ctx, h.owner, model.ChangeElementCurrencyRequest{BudgetId: h.budgetID, ElementId: h.loan, CurrencyId: h.eurCur}); err != nil {
		t.Fatalf("ChangeElementCurrency: %v", err)
	}
	if got := h.debtCurrency(t, id); got != fixture.USD {
		t.Fatalf("paid-off debt currency = %s, want USD", got)
	}

	res, err := h.debts.ReopenDebt(h.ctx, h.owner, model.ReopenDebtRequest{Id: id})
	if err != nil {
		t.Fatalf("ReopenDebt: %v", err)
	}
	if got := h.debtCurrency(t, id); got != h.eurCur {
		t.Errorf("reopened debt currency = %s, want the element's EUR", got)
	}
	if res.CurrencyId != h.eurCur {
		t.Errorf("result currency = %s, want EUR", res.CurrencyId)
	}
}

func TestDebtElementIDs_ListsPaidOffDebtsToo(t *testing.T) {
	h := newDebtHarness(t)
	paid := h.setUpDebt(t, h.loan, "2026-01-01", "5000", "1000", "2026-06-01")
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: paid, Date: "2026-03-01"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	h.looseDebt(t, h.food, nil)
	ids, err := h.debts.ElementIDs(h.ctx, vo.MustParseId(h.budgetID))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id.String()] = true
	}
	if len(ids) != 2 || !got[h.loan] || !got[h.food] {
		t.Fatalf("element ids = %v, want the loan and food categories once each", ids)
	}
}

// update-envelope's currency moves the envelope's active debt with its row,
// redoing its splits; a paid-off debt of the envelope keeps its currency.
func TestUpdateEnvelope_CurrencyMovesTheActiveDebt(t *testing.T) {
	h := newDebtHarness(t)
	housing := h.envelope(t, "Housing", h.rent)
	paid := h.setUpDebt(t, housing, "2026-01-01", "5000", "1000", "2026-06-01")
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: paid, Date: "2026-02-15"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	from := "2026-03-01"
	active := h.looseDebt(t, housing, &from)
	h.spend(h.usd, h.rent, "", "500", time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC))
	h.budgetAt(t, "2026-04-01")
	if n := h.count(t, "SELECT COUNT(*) FROM debts_payments WHERE debt_id = ?", active); n != 1 {
		t.Fatalf("splits before = %d, want 1", n)
	}

	if _, err := h.svc.UpdateEnvelope(h.ctx, h.owner, model.UpdateEnvelopeRequest{
		BudgetId: h.budgetID, Id: housing, Name: "Housing", Icon: "home", CurrencyId: h.eurCur, Categories: []string{h.rent},
	}); err != nil {
		t.Fatalf("UpdateEnvelope: %v", err)
	}
	if got := h.debtCurrency(t, active); got != h.eurCur {
		t.Errorf("active debt currency = %s, want EUR", got)
	}
	if got := h.debtCurrency(t, paid); got != fixture.USD {
		t.Errorf("paid-off debt currency = %s, want it kept in USD", got)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM debts_payments WHERE debt_id = ?", active); n != 0 {
		t.Errorf("splits after the currency change = %d, want them dropped", n)
	}
	h.budgetAt(t, "2026-04-01")
	if n := h.count(t, "SELECT COUNT(*) FROM debts_payments WHERE debt_id = ?", active); n != 1 {
		t.Errorf("splits after the next sync = %d, want the payment split again", n)
	}
}

func (h *debtHarness) paymentsOf(t *testing.T, debtID string) map[string]bool {
	t.Helper()
	rows, err := h.tdb.Raw.QueryContext(h.ctx, h.tdb.Rebind("SELECT transaction_id FROM debts_payments WHERE debt_id = ?"), debtID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// A category with a paid-off debt of its own moves into an envelope whose
// debt, older, covers the same days: the category's payments up to its own
// debt's end stay with that debt, the later ones go to the envelope's.
func TestSync_CategorysOwnDebtWinsOverTheEnvelopes(t *testing.T) {
	cases := []struct {
		name          string
		envelopeStart *string
	}{
		{"envelope debt with a start date", func() *string { s := "2025-12-01"; return &s }()},
		{"envelope debt without one", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newDebtHarness(t)
			housing := h.envelope(t, "Housing", h.rent)
			envDebt := h.looseDebt(t, housing, tc.envelopeStart)
			own := h.setUpDebt(t, h.food, "2026-01-01", "5000", "1000", "2026-06-01")
			if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: own, Date: "2026-03-01"}); err != nil {
				t.Fatalf("MarkDebtPaidOff: %v", err)
			}
			if _, err := h.svc.MoveElement(h.ctx, h.owner, model.MoveElementRequest{BudgetId: h.budgetID, Id: h.food, EnvelopeId: &housing}); err != nil {
				t.Fatalf("MoveElement: %v", err)
			}
			inside := h.spend(h.usd, h.food, "", "1000", time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC))
			endDay := h.spend(h.usd, h.food, "", "1000", time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC))
			after := h.spend(h.usd, h.food, "", "700", time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC))
			rent := h.spend(h.usd, h.rent, "", "300", time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC))
			h.budgetAt(t, "2026-04-01")

			ownPays, envPays := h.paymentsOf(t, own), h.paymentsOf(t, envDebt)
			if len(ownPays) != 2 || !ownPays[inside] || !ownPays[endDay] {
				t.Errorf("own debt payments = %v, want the two up to its end", ownPays)
			}
			if len(envPays) != 2 || !envPays[after] || !envPays[rent] {
				t.Errorf("envelope debt payments = %v, want the later food payment and rent", envPays)
			}
		})
	}
}

type recordingDebts struct {
	appbudget.DebtSource
	events *[]string
}

func (r recordingDebts) LockBudget(ctx context.Context, id vo.Id) error {
	*r.events = append(*r.events, "lock:"+id.String())
	return r.DebtSource.LockBudget(ctx, id)
}

func (r recordingDebts) CloneDebts(ctx context.Context, from, to vo.Id, m map[vo.Id]vo.Id) (map[vo.Id]vo.Id, error) {
	*r.events = append(*r.events, "clone")
	return r.DebtSource.CloneDebts(ctx, from, to, m)
}

// A recording fake rather than a PostgreSQL race: the lock-order inversion
// needs two concurrent transactions, this pins the order that prevents it.
func TestCloneBudget_LocksSourceBudgetBeforeCopying(t *testing.T) {
	h := newDebtHarness(t)
	h.envelope(t, "Housing", h.rent)
	var events []string
	h.svc.SetDebts(recordingDebts{DebtSource: h.debts, events: &events})

	newID := vo.NewId().String()
	if _, err := h.svc.CloneBudget(h.ctx, h.owner, model.CloneBudgetRequest{Id: h.budgetID, NewId: newID, Name: "Copy"}); err != nil {
		t.Fatalf("CloneBudget: %v", err)
	}
	if len(events) < 2 || events[0] != "lock:"+h.budgetID || events[1] != "clone" {
		t.Errorf("debt calls = %v, want the source's lock first, then the clone", events)
	}
}
