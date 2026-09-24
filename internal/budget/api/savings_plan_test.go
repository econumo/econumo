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

// get-budget-plan's savings surface: structure.savings (one row per savings
// member, cells per window month), savingsOpeningBalances and savingsFlows.

const savingsPlanWindow = "&from=2026-07-01&months=3" // July, August, September

type planSavingsCellView struct {
	Actual  string `json:"actual"`
	Planned string `json:"planned"`
}

type planSavingsRowView struct {
	Id          string                `json:"id"`
	Type        int                   `json:"type"`
	Name        string                `json:"name"`
	Icon        string                `json:"icon"`
	CurrencyId  string                `json:"currencyId"`
	OwnerUserId string                `json:"ownerUserId"`
	IsArchived  int                   `json:"isArchived"`
	Position    int                   `json:"position"`
	Cells       []planSavingsCellView `json:"cells"`
}

type planSavingsView struct {
	Item struct {
		Months                 []string `json:"months"`
		SavingsOpeningBalances []struct {
			CurrencyId string `json:"currencyId"`
			Amount     string `json:"amount"`
		} `json:"savingsOpeningBalances"`
		SavingsFlows []struct {
			Month      string `json:"month"`
			CurrencyId string `json:"currencyId"`
			Amount     string `json:"amount"`
		} `json:"savingsFlows"`
		Transfers json.RawMessage `json:"transfers"`
		Structure struct {
			Elements json.RawMessage      `json:"elements"`
			Savings  []planSavingsRowView `json:"savings"`
		} `json:"structure"`
	} `json:"item"`
}

func (h *harness) savingsPlan(t *testing.T, tok, budgetID, window string) (planSavingsView, envelope) {
	t.Helper()
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget-plan?id="+budgetID+window, tok, nil)
	return mustUnmarshal[planSavingsView](t, env.Data), env
}

func planSavingsByID(rows []planSavingsRowView) map[string]planSavingsRowView {
	out := map[string]planSavingsRowView{}
	for _, r := range rows {
		out[r.Id] = r
	}
	return out
}

func assertPlanCells(t *testing.T, id string, got []planSavingsCellView, want []planSavingsCellView) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s cells = %+v, want %+v", id, got, want)
	}
	for i := range want {
		if !decEq(got[i].Actual, want[i].Actual) || got[i].Planned != want[i].Planned {
			t.Errorf("%s cell %d = %+v, want %+v", id, i, got[i], want[i])
		}
	}
}

func TestGetBudgetPlanSavings_Rows(t *testing.T) {
	h, tok, eur := newSavingsBudget(t)
	view, _ := h.savingsPlan(t, tok, budgetID1, savingsPlanWindow)

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
	by := planSavingsByID(rows)

	s1 := by[savingsUSDID]
	if s1.Name != "Rainy day" || s1.Icon != "savings" || s1.CurrencyId != usdID {
		t.Errorf("S1 identity = %+v, want the account's name/icon and USD", s1)
	}
	assertPlanCells(t, "S1", s1.Cells, []planSavingsCellView{{"0", ""}, {"300", "400"}, {"0", ""}})

	s2 := by[savingsEURID]
	if s2.Name != "Euro pot" || s2.Icon != "euro" || s2.CurrencyId != eur {
		t.Errorf("S2 identity = %+v, want the account's name/icon and EUR", s2)
	}
	assertPlanCells(t, "S2", s2.Cells, []planSavingsCellView{{"0", ""}, {"100", "50"}, {"0", ""}})
}

func TestGetBudgetPlanSavings_ConvertsToElementCurrency(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/change-element-currency", tok, map[string]any{
		"budgetId": budgetID1, "elementId": savingsEURID, "currencyId": usdID,
	})
	view, _ := h.savingsPlan(t, tok, budgetID1, savingsPlanWindow)
	s2 := planSavingsByID(view.Item.Structure.Savings)[savingsEURID]
	want := vo.NewDecimal("100").Div(vo.NewDecimal(augustEURRate)).Round(2)
	if s2.CurrencyId != usdID {
		t.Fatalf("S2 currency = %s, want USD", s2.CurrencyId)
	}
	assertPlanCells(t, "S2", s2.Cells, []planSavingsCellView{{"0", ""}, {want.String(), "50"}, {"0", ""}})
}

func TestGetBudgetPlanSavings_OpeningBalances(t *testing.T) {
	h, tok, eur := newSavingsBudget(t)
	june := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	// Only savings accounts count: the everyday account's pre-window income
	// stays out of savingsOpeningBalances.
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: savingsUSDID, Type: 1, Amount: "1000", SpentAt: june})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 1, Amount: "5000", SpentAt: june})

	view, _ := h.savingsPlan(t, tok, budgetID1, savingsPlanWindow)
	got := view.Item.SavingsOpeningBalances
	if len(got) != 2 || got[0].CurrencyId != usdID || !decEq(got[0].Amount, "1000") ||
		got[1].CurrencyId != eur || !decEq(got[1].Amount, "0") {
		t.Fatalf("savingsOpeningBalances = %+v, want [{USD 1000} {EUR 0}]", got)
	}
}

func TestGetBudgetPlanSavings_Flows(t *testing.T) {
	h, tok, eur := newSavingsBudget(t)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: savingsUSDID, Type: 1, Amount: "3",
		SpentAt: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: savingsEURID, Type: 1, Amount: "7",
		SpentAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)})
	// Outside the window on both sides: never a flow.
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: savingsUSDID, Type: 1, Amount: "1000",
		SpentAt: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: savingsUSDID, Type: 1, Amount: "9",
		SpentAt: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)})

	view, _ := h.savingsPlan(t, tok, budgetID1, savingsPlanWindow)
	type flow struct{ month, currency, amount string }
	want := []flow{
		{"2026-08-01", usdID, "303"},
		{"2026-08-01", eur, "100"},
		{"2026-09-01", eur, "7"},
	}
	got := view.Item.SavingsFlows
	if len(got) != len(want) {
		t.Fatalf("savingsFlows = %+v, want %+v", got, want)
	}
	for i, w := range want {
		if got[i].Month != w.month || got[i].CurrencyId != w.currency || !decEq(got[i].Amount, w.amount) {
			t.Errorf("flow %d = %+v, want %+v", i, got[i], w)
		}
	}
	// The Savings row counts only what was moved in; the interest is a flow only.
	s1 := planSavingsByID(view.Item.Structure.Savings)[savingsUSDID]
	if len(s1.Cells) != 3 || !decEq(s1.Cells[1].Actual, "300") {
		t.Errorf("S1 cells = %+v, want August actual 300 (interest excluded)", s1.Cells)
	}
}

func TestGetBudgetPlanSavings_ElementsUnchanged(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	withSavings, _ := h.savingsPlan(t, tok, budgetID1, savingsPlanWindow)
	for _, id := range []string{savingsUSDID, savingsEURID} {
		if bytes.Contains(withSavings.Item.Structure.Elements, []byte(id)) {
			t.Errorf("structure.elements mentions savings account %s: %s", id, withSavings.Item.Structure.Elements)
		}
	}

	if _, err := h.db.Exec(`UPDATE accounts SET type = 2 WHERE id IN (?, ?)`, savingsUSDID, savingsEURID); err != nil {
		t.Fatal(err)
	}
	regular, env := h.savingsPlan(t, tok, budgetID1, savingsPlanWindow)
	if !bytes.Equal(withSavings.Item.Structure.Elements, regular.Item.Structure.Elements) {
		t.Errorf("elements differ:\nsavings: %s\nregular: %s", withSavings.Item.Structure.Elements, regular.Item.Structure.Elements)
	}
	if !bytes.Equal(withSavings.Item.Transfers, regular.Item.Transfers) {
		t.Errorf("transfers differ:\nsavings: %s\nregular: %s", withSavings.Item.Transfers, regular.Item.Transfers)
	}
	for _, want := range []string{`"savings":[]`, `"savingsOpeningBalances":[]`, `"savingsFlows":[]`} {
		if !bytes.Contains(env.raw, []byte(want)) {
			t.Errorf("body lacks %s once no member is a savings account: %s", want, env.raw)
		}
	}
}

func TestGetBudgetPlanSavings_DeletedAccount(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	h.setLimit(t, tok, savingsUSDID, "2026-07-01", "70")
	h.setLimit(t, tok, savingsUSDID, "2026-08-01", nil)
	if _, err := h.db.Exec(`UPDATE accounts SET is_deleted = 1 WHERE id = ?`, savingsUSDID); err != nil {
		t.Fatal(err)
	}

	quiet, _ := h.savingsPlan(t, tok, budgetID1, "&from=2026-06-01&months=1")
	if _, ok := planSavingsByID(quiet.Item.Structure.Savings)[savingsUSDID]; ok {
		t.Errorf("June savings = %+v, want the deleted S1 hidden in a window without plan or activity", quiet.Item.Structure.Savings)
	}

	planned, _ := h.savingsPlan(t, tok, budgetID1, "&from=2026-07-01&months=1")
	s1, ok := planSavingsByID(planned.Item.Structure.Savings)[savingsUSDID]
	if !ok {
		t.Fatalf("July savings = %+v, want the deleted S1 kept for its plan", planned.Item.Structure.Savings)
	}
	if s1.IsArchived != 1 {
		t.Errorf("S1 isArchived = %d, want 1", s1.IsArchived)
	}
	assertPlanCells(t, "S1", s1.Cells, []planSavingsCellView{{"0", "70"}})

	active, _ := h.savingsPlan(t, tok, budgetID1, "&from=2026-08-01&months=1")
	s1, ok = planSavingsByID(active.Item.Structure.Savings)[savingsUSDID]
	if !ok {
		t.Fatalf("August savings = %+v, want the deleted S1 kept for its activity", active.Item.Structure.Savings)
	}
	assertPlanCells(t, "S1", s1.Cells, []planSavingsCellView{{"300", ""}})
}

func TestGetBudgetPlanSavings_EmptyIsArray(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{
		"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID},
	})
	_, env := h.savingsPlan(t, tok, budgetID1, savingsPlanWindow)
	for _, want := range []string{`"savings":[]`, `"savingsOpeningBalances":[]`, `"savingsFlows":[]`} {
		if !bytes.Contains(env.raw, []byte(want)) {
			t.Errorf("get-budget-plan body lacks %s: %s", want, env.raw)
		}
	}
}
