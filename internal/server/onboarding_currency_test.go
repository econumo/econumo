package server_test

import (
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
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

// The import path end to end: finish-import creates the budget starting this
// month, and the imported history of the three months before it feeds the
// suggestions. They read every account the user owns, so archiving the only
// account with history (which takes it out of the budget) or deleting one
// keeps its history counting.
func TestGetPlanSuggestions_FromHistoryBeforeAnOnboardingBudget(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	card, _ := seedImported(e)
	strata := e.f.Category(fixture.Category{UserID: e.user, Name: "Strata", Type: 0})
	cashback := e.f.Category(fixture.Category{UserID: e.user, Name: "Cashback", Type: 1})
	for _, at := range []string{"2025-12-04", "2026-01-04", "2026-02-04"} {
		e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: card, CategoryID: strata, Type: 0, Amount: "360.13", SpentAt: at + " 00:00:00"})
	}
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: card, CategoryID: cashback, Type: 1, Amount: "0.55", SpentAt: "2026-01-31 10:00:00"})
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: card, CategoryID: cashback, Type: 1, Amount: "3", SpentAt: "2026-02-03 10:00:00"})
	status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()})
	if status != 200 {
		t.Fatalf("finish-import %d: %v", status, data)
	}
	budget := data["budgetId"].(string)
	if started := e.str(`SELECT started_at FROM budgets WHERE id = ?`, budget); started[:10] != "2026-03-01" {
		t.Fatalf("budget starts %s", started)
	}
	status, data = e.get("/api/v1/onboarding/get-plan-suggestions")
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	got := suggestions(t, data)
	if len(got) != 2 || got[strata] != "360.13" || got[cashback] != "1.18" {
		t.Fatalf("suggestions = %v", got)
	}

	if status, data := e.post("/api/v1/onboarding/save-accounts", map[string]any{"id": fixture.NewID(), "mainCurrencyId": fixture.USD, "hide": []string{card}}); status != 200 {
		t.Fatalf("save-accounts %d: %v", status, data)
	}
	if e.member(budget, card) {
		t.Fatal("the archived account is still a budget member")
	}
	_, data = e.get("/api/v1/onboarding/get-plan-suggestions")
	if got := suggestions(t, data); len(got) != 2 || got[strata] != "360.13" || got[cashback] != "1.18" {
		t.Fatalf("after archiving: %v", got)
	}

	// a deleted account's history counts too
	gone := e.f.Account(fixture.Account{UserID: e.user, Name: "Closed card", Deleted: true})
	for _, at := range []string{"2026-01-10", "2026-02-10"} {
		e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: gone, CategoryID: strata, Type: 0, Amount: "30", SpentAt: at + " 00:00:00"})
	}
	_, data = e.get("/api/v1/onboarding/get-plan-suggestions")
	if got := suggestions(t, data); got[strata] != "380.13" {
		t.Fatalf("with a deleted account: %v", got)
	}
}

func TestSaveAccounts_SetsTheBalanceOfImportedAccounts(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	card, cash := seedImported(e)
	other := e.f.User(fixture.User{Name: "Other", Email: "other@onboarding.test"})
	theirs := e.f.Account(fixture.Account{UserID: other, Name: "Theirs"})
	for name, balances := range map[string][]map[string]string{
		"foreign account": {{"accountId": theirs, "balance": "1"}},
		"bad format":      {{"accountId": card, "balance": "1,5"}},
		"blank":           {{"accountId": card, "balance": ""}},
		"duplicate":       {{"accountId": card, "balance": "1"}, {"accountId": card, "balance": "2"}},
	} {
		status, data := e.post("/api/v1/onboarding/save-accounts", map[string]any{"id": fixture.NewID(), "mainCurrencyId": e.eur, "balances": balances})
		errsMap, _ := data["errors"].(map[string]any)
		if status != 400 || errsMap["balances"] == nil {
			t.Fatalf("%s: %d %v", name, status, data)
		}
	}
	op := fixture.NewID()
	body := map[string]any{
		"id": op, "mainCurrencyId": e.eur,
		"currencies": []map[string]string{{"accountId": card, "currencyId": e.eur}},
		"balances":   []map[string]string{{"accountId": card, "balance": "100"}, {"accountId": cash, "balance": "-5.25"}},
	}
	if status, data := e.post("/api/v1/onboarding/save-accounts", body); status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	if e.accountCurrency(card) != e.eur {
		t.Fatal("currency not changed with the balance")
	}
	// card: 120.50 -> 100 is a 20.50 expense correction; cash: 0 -> -5.25 a 5.25 expense
	for account, amount := range map[string]string{card: "20.5", cash: "5.25"} {
		got := e.str(`SELECT amount FROM transactions WHERE account_id = ? AND description = 'Balance adjustment' AND type = 0`, account)
		if vo.NewDecimal(got).String() != amount {
			t.Fatalf("%s correction = %s, want %s", account, got, amount)
		}
	}
	if status, data := e.post("/api/v1/onboarding/save-accounts", body); status != 200 {
		t.Fatalf("replay %d: %v", status, data)
	}
	if n := e.count(`SELECT COUNT(*) FROM transactions WHERE description = 'Balance adjustment'`); n != 2 {
		t.Fatalf("corrections after the replay = %d", n)
	}
}
