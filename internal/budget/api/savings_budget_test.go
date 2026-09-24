package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// get-budget's structure.savings: one row per current savings member, kept
// out of structure.elements.

const (
	savingsUSDID  = savingsAccountID  // S1, USD
	savingsEURID  = savingsAccountID2 // S2, EUR
	augustEURRate = "0.91"            // EUR per USD, the only August rate
)

type savingsElementView struct {
	Id          string `json:"id"`
	Type        int    `json:"type"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	CurrencyId  string `json:"currencyId"`
	OwnerUserId string `json:"ownerUserId"`
	IsArchived  int    `json:"isArchived"`
	Position    int    `json:"position"`
	Budgeted    string `json:"budgeted"`
	Spent       string `json:"spent"`
	Available   string `json:"available"`
}

type savingsBudgetView struct {
	Item struct {
		Balances  json.RawMessage `json:"balances"`
		Structure struct {
			Elements json.RawMessage      `json:"elements"`
			Savings  []savingsElementView `json:"savings"`
		} `json:"structure"`
	} `json:"item"`
}

// newSavingsBudget: a USD budget over the everyday Cash account (USD), S1
// (USD savings) and S2 (EUR savings), with August transfers E->S1 500,
// S1->E 200, E->S2 110 USD / 100 EUR and August plans S1 400, S2 50.
func newSavingsBudget(t *testing.T) (*harness, string, string) {
	t.Helper()
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	eur := h.f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
	h.f.Rate(fixture.Rate{CurrencyID: eur, BaseCurrencyID: usdID, Rate: augustEURRate, PublishedAt: "2026-08-10"})
	h.f.Account(fixture.Account{ID: savingsUSDID, UserID: seedUserID, CurrencyID: usdID, Name: "Rainy day", Type: 3, Icon: "savings"})
	h.f.Account(fixture.Account{ID: savingsEURID, UserID: seedUserID, CurrencyID: eur, Name: "Euro pot", Type: 3, Icon: "euro"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{
		"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01",
		"accountIds": []string{accountID, savingsUSDID, savingsEURID},
	})
	at := func(day int) time.Time { return time.Date(2026, 8, day, 12, 0, 0, 0, time.UTC) }
	for _, tx := range []fixture.Transaction{
		{AccountID: accountID, AccountRecipientID: savingsUSDID, Amount: "500", AmountRecipient: "500", SpentAt: at(3)},
		{AccountID: savingsUSDID, AccountRecipientID: accountID, Amount: "200", AmountRecipient: "200", SpentAt: at(5)},
		{AccountID: accountID, AccountRecipientID: savingsEURID, Amount: "110", AmountRecipient: "100", SpentAt: at(7)},
	} {
		tx.UserID, tx.Type = seedUserID, 2
		h.f.Transaction(tx)
	}
	h.setLimit(t, tok, savingsUSDID, "2026-08-01", "400")
	h.setLimit(t, tok, savingsEURID, "2026-08-01", "50")
	return h, tok, eur
}

func (h *harness) setLimit(t *testing.T, tok, elementID, period string, amount any) {
	t.Helper()
	h.mustDo(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": elementID, "period": period, "amount": amount,
	})
}

func (h *harness) savingsBudget(t *testing.T, tok, date string) (savingsBudgetView, envelope) {
	t.Helper()
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date="+date, tok, nil)
	return mustUnmarshal[savingsBudgetView](t, env.Data), env
}

func savingsByID(rows []savingsElementView) map[string]savingsElementView {
	out := map[string]savingsElementView{}
	for _, r := range rows {
		out[r.Id] = r
	}
	return out
}

func TestGetBudgetSavings_MonthlyRows(t *testing.T) {
	h, tok, eur := newSavingsBudget(t)
	view, _ := h.savingsBudget(t, tok, "2026-08-15")

	rows := view.Item.Structure.Savings
	if len(rows) != 2 {
		t.Fatalf("savings = %+v, want two rows", rows)
	}
	wantOrder := savingsIDs(savingsRows(t, h))
	for i, r := range rows {
		if r.Position != i || r.Id != wantOrder[i] {
			t.Errorf("row %d = %s at position %d, want %s at %d (element sort order)", i, r.Id, r.Position, wantOrder[i], i)
		}
		if r.Type != 5 || r.OwnerUserId != seedUserID || r.IsArchived != 0 {
			t.Errorf("row %s: type=%d owner=%q isArchived=%d, want 5/%s/0", r.Id, r.Type, r.OwnerUserId, r.IsArchived, seedUserID)
		}
	}
	by := savingsByID(rows)

	s1 := by[savingsUSDID]
	if s1.Name != "Rainy day" || s1.Icon != "savings" || s1.CurrencyId != usdID {
		t.Errorf("S1 identity = %+v, want the account's name/icon and USD", s1)
	}
	if s1.Budgeted != "400" || s1.Spent != "300" || s1.Available != "100" {
		t.Errorf("S1 amounts = %s/%s/%s, want budgeted 400, spent 300, available 100", s1.Budgeted, s1.Spent, s1.Available)
	}

	s2 := by[savingsEURID]
	if s2.Name != "Euro pot" || s2.Icon != "euro" || s2.CurrencyId != eur {
		t.Errorf("S2 identity = %+v, want the account's name/icon and EUR", s2)
	}
	if s2.Budgeted != "50" || s2.Spent != "100" || s2.Available != "-50" {
		t.Errorf("S2 amounts = %s/%s/%s, want budgeted 50, spent 100 (EUR), available -50", s2.Budgeted, s2.Spent, s2.Available)
	}
}

func TestGetBudgetSavings_ConvertsToElementCurrency(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/change-element-currency", tok, map[string]any{
		"budgetId": budgetID1, "elementId": savingsEURID, "currencyId": usdID,
	})
	view, _ := h.savingsBudget(t, tok, "2026-08-15")
	s2 := savingsByID(view.Item.Structure.Savings)[savingsEURID]
	want := vo.NewDecimal("100").Div(vo.NewDecimal(augustEURRate)).Round(2)
	if s2.CurrencyId != usdID || s2.Spent != want.String() {
		t.Fatalf("S2 = currency %s spent %s, want USD and %s (100 EUR at %s)", s2.CurrencyId, s2.Spent, want, augustEURRate)
	}
	if s2.Available != vo.NewDecimal("50").Sub(want).String() {
		t.Errorf("S2 available = %s, want 50 - %s", s2.Available, want)
	}
}

func TestGetBudgetSavings_ExpenseSideUnchanged(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	withSavings, _ := h.savingsBudget(t, tok, "2026-08-15")
	for _, id := range []string{savingsUSDID, savingsEURID} {
		if bytes.Contains(withSavings.Item.Structure.Elements, []byte(id)) {
			t.Errorf("structure.elements mentions savings account %s: %s", id, withSavings.Item.Structure.Elements)
		}
	}

	if _, err := h.db.Exec(`UPDATE accounts SET type = 2 WHERE id IN (?, ?)`, savingsUSDID, savingsEURID); err != nil {
		t.Fatal(err)
	}
	regular, _ := h.savingsBudget(t, tok, "2026-08-15")
	if !bytes.Equal(withSavings.Item.Structure.Elements, regular.Item.Structure.Elements) {
		t.Errorf("elements differ:\nsavings: %s\nregular: %s", withSavings.Item.Structure.Elements, regular.Item.Structure.Elements)
	}
	if !bytes.Equal(withSavings.Item.Balances, regular.Item.Balances) {
		t.Errorf("balances differ:\nsavings: %s\nregular: %s", withSavings.Item.Balances, regular.Item.Balances)
	}
	if len(regular.Item.Structure.Savings) != 0 {
		t.Errorf("savings = %+v, want none once no member is a savings account", regular.Item.Structure.Savings)
	}
}

func TestGetBudgetSavings_StaleElementRowNotRendered(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	if _, err := h.db.Exec(`UPDATE accounts SET type = 2 WHERE id = ?`, savingsUSDID); err != nil {
		t.Fatal(err)
	}
	view, _ := h.savingsBudget(t, tok, "2026-08-15")
	rows := view.Item.Structure.Savings
	if len(rows) != 1 || rows[0].Id != savingsEURID || rows[0].Position != 0 {
		t.Fatalf("savings = %+v, want only S2 at position 0 (S1's element row is stale)", rows)
	}
}

func TestGetBudgetSavings_DeletedAccount(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	h.setLimit(t, tok, savingsUSDID, "2026-08-01", nil)
	if _, err := h.db.Exec(`UPDATE accounts SET is_deleted = 1 WHERE id = ?`, savingsUSDID); err != nil {
		t.Fatal(err)
	}

	quiet, _ := h.savingsBudget(t, tok, "2026-07-15")
	if _, ok := savingsByID(quiet.Item.Structure.Savings)[savingsUSDID]; ok {
		t.Errorf("July savings = %+v, want the deleted S1 hidden in a month without activity", quiet.Item.Structure.Savings)
	}

	active, _ := h.savingsBudget(t, tok, "2026-08-15")
	s1, ok := savingsByID(active.Item.Structure.Savings)[savingsUSDID]
	if !ok {
		t.Fatalf("August savings = %+v, want the deleted S1 kept for its activity", active.Item.Structure.Savings)
	}
	if s1.IsArchived != 1 || s1.Budgeted != "0" || s1.Spent != "300" {
		t.Errorf("S1 = %+v, want isArchived 1, budgeted 0, spent 300", s1)
	}
}

func TestGetBudgetSavings_EmptyIsArray(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{
		"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID},
	})
	_, env := h.savingsBudget(t, tok, "2026-08-15")
	if !bytes.Contains(env.raw, []byte(`"savings":[]`)) {
		t.Fatalf("get-budget body lacks \"savings\":[]: %s", env.raw)
	}
}
