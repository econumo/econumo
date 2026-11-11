package server_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

const debtsMarker = `{"debts":`

// answer replies to the debt detection with debts and to every other request
// with an empty answer.
func (e *onbEnv) answer(debts string) {
	e.ai.bySystem = func(system string) (string, error) {
		if strings.Contains(system, debtsMarker) {
			return debts, nil
		}
		return `{"categories":{},"accounts":{},"savings":[]}`, nil
	}
}

type debtRow struct {
	name, category, currency, payment, schedule string
	setUp                                       bool
}

func (e *onbEnv) debts() []debtRow {
	e.t.Helper()
	rows, err := e.db.Raw.Query(e.db.Rebind(`SELECT name, category_id, currency_id, payment, schedule, amount_borrowed IS NOT NULL FROM debts WHERE user_id = ? ORDER BY name`), e.user)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []debtRow
	for rows.Next() {
		var d debtRow
		if err := rows.Scan(&d.name, &d.category, &d.currency, &d.payment, &d.schedule, &d.setUp); err != nil {
			e.t.Fatal(err)
		}
		d.payment = vo.NewDecimal(d.payment).String()
		out = append(out, d)
	}
	return out
}

func debtReply(entries ...[2]string) string {
	list := make([]map[string]string, 0, len(entries))
	for _, en := range entries {
		list = append(list, map[string]string{"categoryId": en[0], "name": en[1]})
	}
	b, _ := json.Marshal(map[string]any{"debts": list})
	return "Here you go:\n```json\n" + string(b) + "\n```"
}

type debtWorld struct {
	card                                       string
	mortgage, carLoan, food, rent, salary, old string
}

// seedDebtHistory gives the user an account and six months of history before
// stepNow (September 2025 - February 2026).
func seedDebtHistory(e *onbEnv) debtWorld {
	main := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Main"})
	w := debtWorld{card: e.f.Account(fixture.Account{UserID: e.user, Name: "Girokonto", Icon: "credit_card"})}
	e.f.AccountInFolder(main, w.card)
	cat := func(name string, typ int, archived bool) string {
		return e.f.Category(fixture.Category{UserID: e.user, Name: name, Type: typ, Icon: "home", Archived: archived})
	}
	w.mortgage, w.carLoan, w.rent = cat("Hypothek", 0, false), cat("Autokredit", 0, false), cat("Miete", 0, false)
	w.food = e.f.Category(fixture.Category{UserID: e.user, Name: "Lebensmittel", Type: 0, Icon: model.ImportCategoryIcon})
	w.salary, w.old = cat("Gehalt", 1, false), cat("Alter Kredit", 0, true)
	tx := func(category string, typ int, amount, at string) {
		e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: w.card, CategoryID: category, Type: typ, Amount: amount, SpentAt: at + " 10:00:00"})
	}
	// four paid months out of six: the median of 900, 950, 1050 and 2000 is 1000
	tx(w.mortgage, 0, "900", "2025-10-03")
	tx(w.mortgage, 0, "950", "2025-11-03")
	tx(w.mortgage, 0, "1050", "2025-12-03")
	tx(w.mortgage, 0, "1500", "2026-02-03")
	tx(w.mortgage, 0, "500", "2026-02-20")
	tx(w.mortgage, 0, "700", "2025-08-03") // before the window
	tx(w.carLoan, 0, "300", "2026-01-05")  // one paid month only
	for _, at := range []string{"2025-09-10", "2025-10-10", "2025-11-10", "2025-12-10", "2026-01-10", "2026-02-10"} {
		tx(w.food, 0, "120.5", at)
		tx(w.rent, 0, "800", at)
		tx(w.salary, 1, "3000", at)
		tx(w.old, 0, "100", at)
	}
	return w
}

func TestFinishImport_CreatesADebtFromALoanCategory(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	w := seedDebtHistory(e)
	unknown := fixture.NewID()
	e.answer(debtReply([2]string{strings.ToUpper(w.mortgage), " Hypothekendarlehen "}, [2]string{w.carLoan, "Autokredit"},
		[2]string{w.salary, "Gehalt"}, [2]string{unknown, "Phantom"}, [2]string{w.mortgage, "Twice"}))
	status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()})
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	budget := data["budgetId"].(string)
	prompts := e.ai.promptsFor(debtsMarker)
	if len(prompts) != 1 {
		t.Fatalf("debt prompts = %d (all %d)", len(prompts), len(e.ai.prompts))
	}
	var prompt struct {
		Timezone, Language string
		Categories         []struct {
			ID, Name string
			Monthly  []string
		}
	}
	if err := json.Unmarshal([]byte(prompts[0]), &prompt); err != nil {
		t.Fatal(err)
	}
	// the expense categories paid in at least two months: never income, archived
	// or once-paid ones, and never the profile currency
	asked := map[string][]string{}
	for _, c := range prompt.Categories {
		asked[c.ID] = c.Monthly
	}
	if prompt.Language != "de" || len(asked) != 3 || asked[w.mortgage] == nil || asked[w.food] == nil || asked[w.rent] == nil || strings.Contains(prompts[0], "USD") {
		t.Fatalf("prompt = %s", prompts[0])
	}
	if got := strings.Join(asked[w.mortgage], ","); got != "0,900,950,1050,0,2000" {
		t.Fatalf("mortgage history = %s", got)
	}
	got := e.debts()
	if len(got) != 1 {
		t.Fatalf("debts = %+v", got)
	}
	d := got[0]
	if d.name != "Hypothekendarlehen" || d.category != w.mortgage || d.payment != "1000" || d.schedule != string(model.RecurringScheduleMonthly) || d.setUp {
		t.Fatalf("debt = %+v", d)
	}
	if want := e.str(`SELECT currency_id FROM budgets WHERE id = ?`, budget); d.currency != want {
		t.Fatalf("debt currency %s, budget %s", d.currency, want)
	}
	// the debt shows in the budget
	if n := e.count(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND type = ?`, budget, int(model.ElementDebt)); n != 1 {
		t.Fatalf("debt rows in the budget = %d", n)
	}
}

func TestFinishImport_DebtDetectionFailuresCreateNothing(t *testing.T) {
	for name, reply := range map[string]func(string) (string, error){
		"error":       func(string) (string, error) { return "", errors.New("provider down") },
		"malformed":   func(string) (string, error) { return "the mortgage is a loan", nil },
		"wrong shape": func(string) (string, error) { return `{"debts":{"id":"x"}}`, nil },
		"panic":       func(string) (string, error) { panic("boom") },
	} {
		t.Run(name, func(t *testing.T) {
			e := newOnbEnv(t, stepNow)
			w := seedDebtHistory(e)
			e.ai.bySystem = func(system string) (string, error) {
				if strings.Contains(system, debtsMarker) {
					return reply(system)
				}
				return `{"categories":{"` + w.food + `":"shopping_cart"}}`, nil
			}
			if status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()}); status != 200 {
				t.Fatalf("status %d: %v", status, data)
			}
			if got := e.debts(); len(got) != 0 {
				t.Fatalf("debts = %+v", got)
			}
			// the other parts still apply
			if e.icon("categories", w.food) != "shopping_cart" {
				t.Fatal("the category icon part did not run on its own")
			}
		})
	}
}

func TestFinishImport_NoCandidateAsksNothing(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	main := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Main"})
	card := e.f.Account(fixture.Account{UserID: e.user, Name: "Card"})
	e.f.AccountInFolder(main, card)
	food := e.f.Category(fixture.Category{UserID: e.user, Name: "Food", Type: 0})
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: card, CategoryID: food, Type: 0, Amount: "5", SpentAt: "2026-02-01 10:00:00"})
	e.answer(debtReply([2]string{food, "Loan"}))
	if status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()}); status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	if n := len(e.ai.promptsFor(debtsMarker)); n != 0 || len(e.debts()) != 0 {
		t.Fatalf("debt prompts %d, debts %v", n, e.debts())
	}
}

func planBody(budget string, amounts map[string]string) map[string]any {
	list := make([]map[string]any, 0, len(amounts))
	for id, amount := range amounts {
		list = append(list, map[string]any{"categoryId": id, "amount": amount})
	}
	return map[string]any{"id": fixture.NewID(), "budgetId": budget, "amounts": list}
}

func TestSavePlan_CreatesADebtFromAPlannedLoanCategory(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	main := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Main"})
	card := e.f.Account(fixture.Account{UserID: e.user, Name: "Card"})
	e.f.AccountInFolder(main, card)
	budget := e.f.Budget(fixture.Budget{UserID: e.user, StartedAt: "2026-03-01 00:00:00"})
	e.f.BudgetAccount(budget, card)
	loan := e.f.Category(fixture.Category{UserID: e.user, Name: "Kredit", Type: 0})
	// the loan's row is planned in euros: the debt follows its element currency
	e.f.BudgetElement(fixture.BudgetElement{BudgetID: budget, ExternalID: loan, Type: int(model.ElementCategory), CurrencyID: e.eur})
	lease := e.f.Category(fixture.Category{UserID: e.user, Name: "Leasing", Type: 0})
	food := e.f.Category(fixture.Category{UserID: e.user, Name: "Food", Type: 0})
	salary := e.f.Category(fixture.Category{UserID: e.user, Name: "Salary", Type: 1})
	e.answer(debtReply([2]string{loan, "Ratenkredit"}, [2]string{lease, "Leasing"}, [2]string{salary, "Salary"}, [2]string{fixture.NewID(), "Phantom"}))
	status, data := e.post("/api/v1/onboarding/save-plan", planBody(budget, map[string]string{loan: "450.5", lease: "0", food: "300", salary: "2000"}))
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	prompts := e.ai.promptsFor(debtsMarker)
	if len(prompts) != 1 || !strings.Contains(prompts[0], loan) || !strings.Contains(prompts[0], food) ||
		strings.Contains(prompts[0], lease) || strings.Contains(prompts[0], salary) || strings.Contains(prompts[0], "monthly") {
		t.Fatalf("prompts = %v", prompts)
	}
	got := e.debts()
	if len(got) != 1 || got[0].name != "Ratenkredit" || got[0].category != loan || got[0].payment != "450.5" ||
		got[0].schedule != string(model.RecurringScheduleMonthly) || got[0].currency != e.eur || got[0].setUp {
		t.Fatalf("debts = %+v", got)
	}
	// the whole-category debt took the category's plan with it
	debtID := e.str(`SELECT id FROM debts WHERE user_id = ?`, e.user)
	if n := e.count(`SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements el ON el.id = l.element_id WHERE el.budget_id = ? AND el.external_id = ?`, budget, debtID); n != 1 {
		t.Fatalf("plans on the debt row = %d", n)
	}
}

func TestSavePlan_NoPositivePlanStartsNoJob(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	card := e.f.Account(fixture.Account{UserID: e.user, Name: "Card"})
	budget := e.f.Budget(fixture.Budget{UserID: e.user})
	e.f.BudgetAccount(budget, card)
	loan := e.f.Category(fixture.Category{UserID: e.user, Name: "Kredit", Type: 0})
	e.answer(debtReply([2]string{loan, "Kredit"}))
	if status, data := e.post("/api/v1/onboarding/save-plan", planBody(budget, map[string]string{loan: "0"})); status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	if len(e.ai.prompts) != 0 || len(e.debts()) != 0 {
		t.Fatalf("prompts %d, debts %v", len(e.ai.prompts), e.debts())
	}
}

// After an import the plan step covers what the history could not: a planned
// category the import already turned into a debt is skipped, so detection
// running again never duplicates one.
func TestSavePlan_AfterAnImportSkipsCategoriesAlreadyCovered(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	w := seedDebtHistory(e)
	e.answer(debtReply([2]string{w.mortgage, "Hypothek"}, [2]string{w.carLoan, "Autokredit"}))
	status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()})
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	budget := data["budgetId"].(string)
	if got := e.debts(); len(got) != 1 || got[0].category != w.mortgage {
		t.Fatalf("after import: %+v", got)
	}
	status, data = e.post("/api/v1/onboarding/save-plan", planBody(budget, map[string]string{w.mortgage: "1000", w.carLoan: "300"}))
	if status != 200 {
		t.Fatalf("save-plan %d: %v", status, data)
	}
	if n := len(e.ai.promptsFor(debtsMarker)); n != 2 {
		t.Fatalf("debt prompts = %d", n)
	}
	got := e.debts()
	if len(got) != 2 || got[0].category != w.carLoan || got[0].payment != "300" || got[1].category != w.mortgage || got[1].payment != "1000" {
		t.Fatalf("debts = %+v", got)
	}
	// the covered category was never asked about again, and its plan went to
	// the import's debt rather than to a second row
	if strings.Contains(e.ai.promptsFor(debtsMarker)[1], w.mortgage) {
		t.Fatal("a covered category was asked about")
	}
	mortgageDebt := e.str(`SELECT id FROM debts WHERE category_id = ?`, w.mortgage)
	limitsOn := func(externalID string) int {
		return e.count(`SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements el ON el.id = l.element_id WHERE el.budget_id = ? AND el.external_id = ?`, budget, externalID)
	}
	if limitsOn(mortgageDebt) != 1 || limitsOn(w.mortgage) != 0 {
		t.Fatalf("limits: debt %d, category %d", limitsOn(mortgageDebt), limitsOn(w.mortgage))
	}
	status, view := e.get("/api/v1/budget/get-budget?id=" + budget + "&date=2026-03-01")
	if status != 200 {
		t.Fatalf("get-budget %d: %v", status, view)
	}
	structure := view["item"].(map[string]any)["structure"].(map[string]any)
	planned := map[string]string{}
	var walk func(list any)
	walk = func(list any) {
		items, _ := list.([]any)
		for _, it := range items {
			row := it.(map[string]any)
			if row["isBudgeted"] == float64(1) {
				planned[row["id"].(string)] = row["budgeted"].(string)
			}
			walk(row["children"])
		}
	}
	walk(structure["elements"])
	walk(structure["debts"])
	if len(planned) != 2 || planned[mortgageDebt] != "1000" || planned[w.mortgage] != "" {
		t.Fatalf("planned rows = %v", planned)
	}
}
