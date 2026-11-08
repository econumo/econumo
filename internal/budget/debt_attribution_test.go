package budget_test

import (
	"context"
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
	labelrepo "github.com/econumo/econumo/internal/label/repo"
	"github.com/econumo/econumo/internal/model"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

type attrRecurring struct{ templates []*model.RecurringTransaction }

func (r *attrRecurring) ListByAccountIDs(_ context.Context, ids []vo.Id) ([]*model.RecurringTransaction, error) {
	in := map[string]bool{}
	for _, id := range ids {
		in[id.String()] = true
	}
	var out []*model.RecurringTransaction
	for _, rt := range r.templates {
		if in[rt.AccountID.String()] {
			out = append(out, rt)
		}
	}
	return out, nil
}

func (r *attrRecurring) CanEdit(context.Context, vo.Id, vo.Id) (bool, error) { return true, nil }

// attrHarness is a budget over two member accounts (USD and EUR) whose owner
// pays a whole-category loan through the real debt service, so stored splits
// drive the attribution exactly as in production. Today is 2026-06-15.
type attrHarness struct {
	ctx       context.Context
	tdb       *dbtest.DB
	svc       *appbudget.Service
	debts     *appdebt.Service
	f         *fixture.Builder
	recurring *attrRecurring
	owner     vo.Id
	budgetID  string
	usd, eur  string // accounts
	eurCur    string
	loan      string // expense category the debt matches
	food      string
	tag       string
}

var attrToday = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

func newAttrHarness(t *testing.T) *attrHarness {
	t.Helper()
	db := dbtest.New(t)
	txm := db.TX
	engine := db.Engine
	f := fixture.New(t, db)
	clk := fixedClock{t: attrToday}

	owner := f.User(fixture.User{Name: "Owner", Email: "owner@attr.test"})
	eurCur := f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
	f.Rate(fixture.Rate{CurrencyID: eurCur, BaseCurrencyID: fixture.USD, Rate: "0.50000000", PublishedAt: "2026-06-01"})
	f.Rate(fixture.Rate{CurrencyID: eurCur, BaseCurrencyID: fixture.USD, Rate: "0.50000000", PublishedAt: "2026-05-01"})
	usd := f.Account(fixture.Account{UserID: owner, Name: "Cash"})
	eur := f.Account(fixture.Account{UserID: owner, Name: "Euro", CurrencyID: eurCur})
	started := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	budgetID := f.Budget(fixture.Budget{UserID: owner, StartedAt: started})
	f.BudgetAccount(budgetID, usd)
	f.BudgetAccount(budgetID, eur)
	loan := f.Category(fixture.Category{UserID: owner, Name: "Mortgage", Position: 1})
	food := f.Category(fixture.Category{UserID: owner, Name: "Food", Position: 2})
	tag := f.Tag(fixture.Tag{UserID: owner, Name: "Home"})

	userRepo := userrepo.NewRepo(engine, txm)
	accountRepo := accountrepo.NewRepo(engine, txm)
	categoryRepo := categoryrepo.NewRepo(engine, txm)
	tagRepo := tagrepo.NewRepo(engine, txm)
	payeeRepo := payeerepo.NewRepo(engine, txm)
	currencyLookup := currencyrepo.New(engine, txm)
	rateProvider := currencyrepo.NewRateProvider(engine, txm, currencyLookup, fixture.USD)
	convertor := domcurrency.NewConvertor(rateProvider)

	svc := appbudget.NewService(
		budgetrepo.NewRepo(engine, txm), budgetrepo.NewReadRepo(engine, txm), convertor, rateProvider,
		server.NewBudgetUserLookup(userRepo, clk),
		server.NewBudgetAccountLookup(accountRepo),
		server.NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(server.NewBudgetCategoryMetadataLookup(categoryRepo), server.NewBudgetTagMetadataLookup(tagRepo), server.NewBudgetPayeeMetadataLookup(payeeRepo)),
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(engine, txm)),
		operationrepo.NewGuard(engine, txm),
		txm, clk,
	)
	debts := appdebt.NewService(debtrepo.NewRepo(engine, txm), txm, clk,
		server.NewDebtClassifications(categoryRepo, payeeRepo, tagRepo, labelrepo.NewRepo(engine, txm)),
		server.NewDebtRates(rateProvider, convertor), currencyLookup, userRepo)
	svc.SetDebts(debts)
	debts.SetBudgetElements(server.NewDebtBudgetElements(svc))
	rec := &attrRecurring{}
	svc.SetRecurring(rec)

	return &attrHarness{
		ctx: context.Background(), tdb: db, svc: svc, debts: debts, f: f, recurring: rec,
		owner: vo.MustParseId(owner), budgetID: budgetID, usd: usd, eur: eur, eurCur: eurCur,
		loan: loan, food: food, tag: tag,
	}
}

func (h *attrHarness) spend(account, category, tag, amount string, at time.Time) string {
	return h.f.Transaction(fixture.Transaction{
		UserID: h.owner.String(), AccountID: account, CategoryID: category, TagID: tag,
		Type: 0, Amount: amount, SpentAt: at,
	})
}

func (h *attrHarness) createDebt(t *testing.T) string {
	t.Helper()
	res, err := h.debts.CreateDebt(h.ctx, h.owner, model.CreateDebtRequest{
		Name: "Mortgage", CurrencyId: fixture.USD, CategoryId: h.loan,
		Balance: vo.NewFlexString("10000"), Payment: vo.NewFlexString("1000"),
		Schedule: "monthly", LastPaymentDate: "2027-06-01",
	})
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}
	return res.Id
}

type debtPaidOnWire struct{ Paid, BudgetPaid, Available string }

func (h *attrHarness) getBudget(t *testing.T) (*model.GetBudgetResult, map[string]debtPaidOnWire) {
	t.Helper()
	res, err := h.svc.GetBudget(h.ctx, h.owner, model.GetBudgetRequest{Id: h.budgetID, Date: "2026-06-01"})
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	paid := map[string]debtPaidOnWire{}
	for _, d := range res.Item.Structure.Debts {
		paid[d.Id] = debtPaidOnWire{Paid: d.Spent, BudgetPaid: d.BudgetSpent, Available: d.Available}
	}
	return res, paid
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

func spentOf(t *testing.T, res *model.GetBudgetResult, id string, typ model.ElementType) string {
	t.Helper()
	el := findElement(res, id, typ)
	if el == nil {
		return "0"
	}
	return el.Spent
}

func TestGetBudget_DebtPaymentLeavesCategoryRow(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.food, "", "50", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))

	before, _ := h.getBudget(t)
	if got := spentOf(t, before, h.loan, model.ElementCategory); got != "1000" {
		t.Fatalf("loan spent without a debt = %s, want 1000", got)
	}

	debtID := h.createDebt(t)
	res, paid := h.getBudget(t)
	if got := spentOf(t, res, h.loan, model.ElementCategory); got != "0" {
		t.Errorf("loan spent with a debt = %s, want 0", got)
	}
	if el := findElement(res, h.loan, model.ElementCategory); el != nil && el.Available != "0" {
		t.Errorf("loan available = %s, want 0 (May's payment left the row too)", el.Available)
	}
	if got := spentOf(t, res, h.food, model.ElementCategory); got != "50" {
		t.Errorf("food spent = %s, want 50", got)
	}
	p := paid[debtID]
	// Nothing planned, 1000 paid in May and 1000 in June.
	if p.Paid != "1000" || p.BudgetPaid != "1000" || p.Available != "-2000" {
		t.Errorf("debt paid = %+v, want 1000 / 1000 / available -2000", p)
	}

	// Entered after the debt: the read syncs its split before summing.
	h.spend(h.usd, h.loan, "", "300", time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC))
	res, paid = h.getBudget(t)
	if got := spentOf(t, res, h.loan, model.ElementCategory); got != "0" {
		t.Errorf("loan spent after a new payment = %s, want 0", got)
	}
	if p := paid[debtID]; p.Paid != "1300" {
		t.Errorf("debt paid after a new payment = %+v, want 1300", p)
	}
}

func TestGetBudget_DebtPaymentLeavesTagRow(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, h.tag, "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.food, h.tag, "40", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))

	before, _ := h.getBudget(t)
	if got := spentOf(t, before, h.tag, model.ElementTag); got != "1040" {
		t.Fatalf("tag spent without a debt = %s, want 1040", got)
	}

	debtID := h.createDebt(t)
	res, paid := h.getBudget(t)
	tagRow := findElement(res, h.tag, model.ElementTag)
	if tagRow == nil || tagRow.Spent != "40" {
		t.Fatalf("tag row = %+v, want spent 40", tagRow)
	}
	for _, ch := range tagRow.Children {
		if ch.Id == h.loan {
			t.Errorf("tag row still has the loan child: %+v", ch)
		}
	}
	if got := spentOf(t, res, h.loan, model.ElementCategory); got != "0" {
		t.Errorf("loan spent = %s, want 0", got)
	}
	if p := paid[debtID]; p.Paid != "1000" {
		t.Errorf("debt paid = %+v, want 1000", p)
	}
}

func TestGetBudget_DebtPaymentLeavesEnvelopeSpent(t *testing.T) {
	h := newAttrHarness(t)
	env := h.f.BudgetEnvelope(fixture.BudgetEnvelope{BudgetID: h.budgetID, Name: "Home", Icon: "home"})
	h.f.BudgetElement(fixture.BudgetElement{BudgetID: h.budgetID, ExternalID: env, Type: int(model.ElementEnvelope)})
	h.f.EnvelopeCategory(env, h.loan)
	h.f.EnvelopeCategory(env, h.food)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.food, "", "70", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))

	before, _ := h.getBudget(t)
	if got := spentOf(t, before, env, model.ElementEnvelope); got != "1070" {
		t.Fatalf("envelope spent without a debt = %s, want 1070", got)
	}

	h.createDebt(t)
	res, _ := h.getBudget(t)
	envRow := findElement(res, env, model.ElementEnvelope)
	if envRow == nil || envRow.Spent != "70" {
		t.Fatalf("envelope row = %+v, want spent 70", envRow)
	}
	for _, ch := range envRow.Children {
		if ch.Id == h.loan && ch.Spent != "0" {
			t.Errorf("loan child spent = %s, want 0", ch.Spent)
		}
	}
}

func TestGetBudget_DebtPaidConverted(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.eur, h.loan, "", "100", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))

	debtID := h.createDebt(t)
	res, paid := h.getBudget(t)
	if got := spentOf(t, res, h.loan, model.ElementCategory); got != "0" {
		t.Errorf("loan spent = %s, want 0", got)
	}
	// 100 EUR at 0.5 EUR per USD.
	if p := paid[debtID]; p.Paid != "200" || p.BudgetPaid != "200" {
		t.Errorf("debt paid = %+v, want 200 USD", p)
	}
}

func TestGetBudgetPlan_DebtActualPerMonth(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.loan, h.tag, "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.food, "", "25", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))

	debtID := h.createDebt(t)
	res, err := h.svc.GetBudgetPlan(h.ctx, h.owner, model.GetBudgetPlanRequest{Id: h.budgetID, From: "2026-04-01", Months: "3"})
	if err != nil {
		t.Fatalf("GetBudgetPlan: %v", err)
	}
	want := []string{"0", "1000", "1000"}
	var got []string
	for _, d := range res.Item.Structure.Debts {
		if d.Id == debtID {
			for _, c := range d.Cells {
				got = append(got, c.Actual)
			}
		}
	}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("debt actual = %v, want %v", got, want)
	}
	for _, el := range res.Item.Structure.Elements {
		if el.Id == h.loan || el.Id == h.tag {
			for i, c := range el.Cells {
				if c.Actual != "0" {
					t.Errorf("%s cell %d actual = %s, want 0", el.Name, i, c.Actual)
				}
			}
		}
		if el.Id == h.food && el.Cells[2].Actual != "25" {
			t.Errorf("food June actual = %s, want 25", el.Cells[2].Actual)
		}
	}
}

func TestUpcoming_MatchingTemplateDropped(t *testing.T) {
	h := newAttrHarness(t)
	next := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	loan, food := vo.MustParseId(h.loan), vo.MustParseId(h.food)
	h.recurring.templates = []*model.RecurringTransaction{
		{ID: vo.NewId(), UserID: h.owner, Type: model.TransactionTypeExpense, AccountID: vo.MustParseId(h.usd),
			Amount: "1000", CategoryID: &loan, Schedule: model.RecurringScheduleMonthly, NextPaymentAt: next, ScheduledDay: 20},
		{ID: vo.NewId(), UserID: h.owner, Type: model.TransactionTypeExpense, AccountID: vo.MustParseId(h.usd),
			Amount: "30", CategoryID: &food, Schedule: model.RecurringScheduleMonthly, NextPaymentAt: next, ScheduledDay: 20},
	}
	// A real payment dated after today is upcoming money too.
	h.spend(h.usd, h.loan, "", "500", time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC))

	before, _ := h.getBudget(t)
	if el := findElement(before, h.loan, model.ElementCategory); el == nil || el.Upcoming != "1500" {
		t.Fatalf("loan upcoming without a debt = %+v, want 1500", el)
	}

	h.createDebt(t)
	res, _ := h.getBudget(t)
	if el := findElement(res, h.loan, model.ElementCategory); el != nil && el.Upcoming != "0" {
		t.Errorf("loan upcoming with a debt = %s, want 0", el.Upcoming)
	}
	if el := findElement(res, h.food, model.ElementCategory); el == nil || el.Upcoming != "30" {
		t.Errorf("food upcoming = %+v, want 30", el)
	}
}

func TestGetBudget_ExpenseTotalsUnchanged(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.eur, h.loan, h.tag, "100", time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.food, h.tag, "40", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.food, "", "60", time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, "", "", "15", time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC))

	total := func(res *model.GetBudgetResult) vo.DecimalNumber {
		sum := vo.NewDecimal("0")
		for _, el := range res.Item.Structure.Elements {
			sum = sum.Add(vo.NewDecimal(el.BudgetSpent))
		}
		return sum
	}
	before, _ := h.getBudget(t)
	want := total(before)
	if want.String() != "1315" {
		t.Fatalf("expense total without a debt = %s, want 1315", want)
	}

	debtID := h.createDebt(t)
	res, paid := h.getBudget(t)
	got := total(res).Add(vo.NewDecimal(paid[debtID].BudgetPaid))
	if got.String() != want.String() {
		t.Errorf("expenses + debt paid = %s, want %s (rows %s, debt %s)", got, want, total(res), paid[debtID].BudgetPaid)
	}
	if before.Item.Balances[0].CurrencyId != res.Item.Balances[0].CurrencyId {
		t.Fatal("balances reordered")
	}
	for i := range before.Item.Balances {
		b, a := before.Item.Balances[i], res.Item.Balances[i]
		if deref(b.Expenses) != deref(a.Expenses) || deref(b.EndBalance) != deref(a.EndBalance) {
			t.Errorf("balance %d changed: %+v -> %+v", i, b, a)
		}
	}
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func TestDrilldown_LabelledDebtExcludesOnlyItsPayments(t *testing.T) {
	h := newAttrHarness(t)
	car := h.f.Label(fixture.Label{UserID: h.owner.String(), Name: "Car"})
	student := h.f.Label(fixture.Label{UserID: h.owner.String(), Name: "Student"})
	carPaid := h.spend(h.usd, h.loan, "", "300", time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC))
	h.labelTx(t, carPaid, car)
	shared := h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	h.labelTx(t, shared, car, student)
	plain := h.spend(h.usd, h.loan, "", "40", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))
	loan := vo.MustParseId(h.loan)
	carL, studentL := vo.MustParseId(car), vo.MustParseId(student)
	next := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	sharedTemplate, carTemplate, plainTemplate := vo.NewId(), vo.NewId(), vo.NewId()
	template := func(id vo.Id, labels ...vo.Id) *model.RecurringTransaction {
		return &model.RecurringTransaction{ID: id, UserID: h.owner, Type: model.TransactionTypeExpense, AccountID: vo.MustParseId(h.usd),
			Amount: "100", CategoryID: &loan, LabelIDs: labels, Schedule: model.RecurringScheduleMonthly, NextPaymentAt: next, ScheduledDay: 20}
	}
	h.recurring.templates = []*model.RecurringTransaction{
		template(sharedTemplate, carL, studentL), template(carTemplate, carL), template(plainTemplate),
	}

	carDebt := h.createLooseDebt(t, func(r *model.CreateDebtRequest) { r.Name, r.LabelId = "Car", &car })
	studentDebt := h.createLooseDebt(t, func(r *model.CreateDebtRequest) { r.Name, r.LabelId = "Student", &student })
	h.backdateDebt(t, studentDebt, attrToday.AddDate(0, -1, 0))

	loanID := h.loan
	category := h.txList(t, model.BudgetTransactionListRequest{CategoryId: &loanID})
	if got := itemIDs(category); !sameIDs(got, []string{plain}) {
		t.Errorf("category list = %v, want only the unlabelled payment", got)
	}
	if got := upcomingIDs(category); !sameIDs(got, []string{plainTemplate.String()}) {
		t.Errorf("category upcoming = %v, want only the unlabelled template", got)
	}
	studentList := h.txList(t, model.BudgetTransactionListRequest{DebtId: &studentDebt})
	if got := itemIDs(studentList); !sameIDs(got, []string{shared}) {
		t.Errorf("older debt list = %v, want the shared payment", got)
	}
	if got := upcomingIDs(studentList); !sameIDs(got, []string{sharedTemplate.String()}) {
		t.Errorf("older debt upcoming = %v, want the shared template", got)
	}
	carList := h.txList(t, model.BudgetTransactionListRequest{DebtId: &carDebt})
	if got := itemIDs(carList); !sameIDs(got, []string{carPaid}) {
		t.Errorf("newer debt list = %v, want only its own payment", got)
	}
	if got := upcomingIDs(carList); !sameIDs(got, []string{carTemplate.String()}) {
		t.Errorf("newer debt upcoming = %v, want only its own template", got)
	}
}

func TestUpcoming_TemplateWithDebtLabelDropped(t *testing.T) {
	h := newAttrHarness(t)
	car := h.f.Label(fixture.Label{UserID: h.owner.String(), Name: "Car"})
	next := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	loan, carL := vo.MustParseId(h.loan), vo.MustParseId(car)
	h.recurring.templates = []*model.RecurringTransaction{
		{ID: vo.NewId(), UserID: h.owner, Type: model.TransactionTypeExpense, AccountID: vo.MustParseId(h.usd),
			Amount: "1000", CategoryID: &loan, LabelIDs: []vo.Id{carL}, Schedule: model.RecurringScheduleMonthly, NextPaymentAt: next, ScheduledDay: 20},
		{ID: vo.NewId(), UserID: h.owner, Type: model.TransactionTypeExpense, AccountID: vo.MustParseId(h.usd),
			Amount: "30", CategoryID: &loan, Schedule: model.RecurringScheduleMonthly, NextPaymentAt: next, ScheduledDay: 20},
	}

	h.createLooseDebt(t, func(r *model.CreateDebtRequest) { r.LabelId = &car })
	res, _ := h.getBudget(t)
	if el := findElement(res, h.loan, model.ElementCategory); el == nil || el.Upcoming != "30" {
		t.Errorf("loan upcoming = %+v, want 30 (the labelled template went to the debt)", el)
	}
}
