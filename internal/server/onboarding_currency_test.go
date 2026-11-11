package server_test

import (
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

func (e *onbEnv) accountCurrency(id string) string {
	return e.str(`SELECT currency_id FROM accounts WHERE id = ?`, id)
}

func (e *onbEnv) budgetCurrency(id string) string {
	return e.str(`SELECT currency_id FROM budgets WHERE id = ?`, id)
}

// seedImported is a CSV import's result: accounts in the instance base
// currency (USD) with history, in a visible folder.
func seedImported(e *onbEnv) (card, cash string) {
	folder := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Main"})
	card = e.f.Account(fixture.Account{UserID: e.user, Name: "Card"})
	cash = e.f.Account(fixture.Account{UserID: e.user, Name: "Cash"})
	e.f.AccountInFolder(folder, card)
	e.f.AccountInFolder(folder, cash)
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: card, Type: 1, Amount: "120.50", SpentAt: "2026-03-01 10:00:00"})
	return card, cash
}

func TestSaveAccounts_ChangesTheCurrencyOfImportedAccountsAndTheFreshBudget(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	card, cash := seedImported(e)
	status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()})
	if status != 200 {
		t.Fatalf("finish-import %d: %v", status, data)
	}
	budget := data["budgetId"].(string)
	if e.budgetCurrency(budget) != fixture.USD {
		t.Fatalf("budget currency = %s", e.budgetCurrency(budget))
	}

	op := fixture.NewID()
	body := map[string]any{
		"id": op, "mainCurrencyId": e.eur,
		"currencies": []map[string]string{{"accountId": card, "currencyId": e.eur}, {"accountId": cash, "currencyId": fixture.USD}},
	}
	status, data = e.post("/api/v1/onboarding/save-accounts", body)
	if status != 200 || data["budgetId"] != budget {
		t.Fatalf("status %d: %v", status, data)
	}
	if e.accountCurrency(card) != e.eur || e.accountCurrency(cash) != fixture.USD {
		t.Fatalf("currencies: card %s, cash %s", e.accountCurrency(card), e.accountCurrency(cash))
	}
	if n := e.count(`SELECT COUNT(*) FROM transactions WHERE account_id = ?`, card); n != 1 {
		t.Fatalf("a correction was written: %d transactions", n)
	}
	if e.count(`SELECT COUNT(*) FROM users_hidden_currencies WHERE user_id = ? AND currency_id = ?`, e.user, e.eur) != 0 {
		t.Fatal("the changed currency is hidden")
	}
	if e.budgetCurrency(budget) != e.eur {
		t.Fatalf("the unplanned budget did not follow the main currency: %s", e.budgetCurrency(budget))
	}
	if got := e.onboarding(); got != "import,accounts" {
		t.Fatalf("onboarding = %q", got)
	}

	// A replay of the same id writes nothing.
	body["currencies"] = []map[string]string{{"accountId": card, "currencyId": fixture.USD}}
	body["mainCurrencyId"] = fixture.USD
	if status, data := e.post("/api/v1/onboarding/save-accounts", body); status != 200 {
		t.Fatalf("replay %d: %v", status, data)
	}
	if e.accountCurrency(card) != e.eur || e.budgetCurrency(budget) != e.eur {
		t.Fatal("the replay changed a currency")
	}
}

func TestSaveAccounts_CurrencyChangesAreRefused(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	card, _ := seedImported(e)
	other := e.f.User(fixture.User{Name: "Other", Email: "other@onboarding.test"})
	theirs := e.f.Account(fixture.Account{UserID: other, Name: "Theirs"})
	foreign := e.f.Currency(fixture.Currency{Code: "XFO", Symbol: "X", UserID: other, Rate: "2.00000000"})
	gone := e.f.Currency(fixture.Currency{Code: "XGO", Symbol: "G", Deleted: true})
	deleted := e.f.Account(fixture.Account{UserID: e.user, Name: "Deleted", Deleted: true})
	cases := map[string][]map[string]string{
		"foreign account":         {{"accountId": theirs, "currencyId": e.eur}},
		"deleted account":         {{"accountId": deleted, "currencyId": e.eur}},
		"unknown account":         {{"accountId": fixture.NewID(), "currencyId": e.eur}},
		"foreign custom currency": {{"accountId": card, "currencyId": foreign}},
		"deleted currency":        {{"accountId": card, "currencyId": gone}},
		"unknown currency":        {{"accountId": card, "currencyId": fixture.NewID()}},
		"duplicate account":       {{"accountId": card, "currencyId": e.eur}, {"accountId": card, "currencyId": fixture.USD}},
		"not a uuid":              {{"accountId": "nope", "currencyId": e.eur}},
	}
	for name, currencies := range cases {
		status, data := e.post("/api/v1/onboarding/save-accounts", map[string]any{"id": fixture.NewID(), "mainCurrencyId": e.eur, "currencies": currencies})
		errsMap, _ := data["errors"].(map[string]any)
		if status != 400 || errsMap["currencies"] == nil {
			t.Fatalf("%s: %d %v", name, status, data)
		}
	}
	if e.accountCurrency(card) != fixture.USD || e.onboarding() != "started" {
		t.Fatalf("a refused call wrote: card %s, onboarding %q", e.accountCurrency(card), e.onboarding())
	}
}

func TestSaveAccounts_BudgetKeepsItsCurrencyWhenPlannedOrShared(t *testing.T) {
	for _, tc := range []string{"planned", "shared", "invited"} {
		t.Run(tc, func(t *testing.T) {
			e := newOnbEnv(t, stepNow)
			card, _ := seedImported(e)
			budget := e.f.Budget(fixture.Budget{UserID: e.user})
			e.f.BudgetAccount(budget, card)
			other := e.f.User(fixture.User{Name: "Other", Email: "other@onboarding.test"})
			switch tc {
			case "planned":
				cat := e.f.Category(fixture.Category{UserID: e.user, Name: "Food", Type: 0})
				el := e.f.BudgetElement(fixture.BudgetElement{BudgetID: budget, ExternalID: cat, Type: 0})
				e.f.BudgetLimit(fixture.BudgetLimit{ElementID: el, Period: "2026-03-01", Amount: "10"})
			case "shared":
				e.f.BudgetAccess(budget, other, 1, true)
			case "invited":
				e.f.BudgetAccess(budget, other, 1, false)
			}
			status, data := e.post("/api/v1/onboarding/save-accounts", map[string]any{
				"id": fixture.NewID(), "mainCurrencyId": e.eur,
				"currencies": []map[string]string{{"accountId": card, "currencyId": e.eur}},
			})
			if status != 200 || data["budgetId"] != budget {
				t.Fatalf("status %d: %v", status, data)
			}
			if e.accountCurrency(card) != e.eur {
				t.Fatal("the account currency was not changed")
			}
			if e.budgetCurrency(budget) != fixture.USD {
				t.Fatalf("budget currency switched to %s", e.budgetCurrency(budget))
			}
		})
	}
}
