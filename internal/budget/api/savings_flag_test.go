package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/test/fixture"
)

// The savings role belongs to the budget membership, not to the account: the
// same account can be savings in one budget and everyday in another.

const (
	savingsFlagBudgetB = "bbbb2222-0000-7000-8000-0000000005b0"
	savingsFlagCopyID  = "bbbb2222-0000-7000-8000-0000000005c0"
)

func (h *harness) savingsBudgetOf(t *testing.T, tok, budgetID, date string) savingsBudgetView {
	t.Helper()
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID+"&date="+date, tok, nil)
	return mustUnmarshal[savingsBudgetView](t, env.Data)
}

func TestSavingsFlag_PerBudget(t *testing.T) {
	h, tok, _ := newSavingsBudget(t) // budget A: S1 and S2 flagged
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{
		"id": savingsFlagBudgetB, "name": "Budget B", "currencyId": usdID, "startDate": "2026-06-01",
		"accountIds": []string{accountID, savingsUSDID, savingsEURID},
	})
	flagSavings(t, h.tdb, savingsFlagBudgetB, savingsUSDID, true) // S2 stays everyday in B
	// S2 -> S1: savings-to-savings in A (not counted), everyday-to-savings in B.
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, Type: 2, AccountID: savingsEURID, AccountRecipientID: savingsUSDID,
		Amount: "20", AmountRecipient: "22", SpentAt: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)})

	a := savingsByID(h.savingsBudgetOf(t, tok, budgetID1, "2026-08-15").Item.Structure.Savings)
	if len(a) != 2 {
		t.Fatalf("A savings = %+v, want S1 and S2", a)
	}
	if a[savingsUSDID].Spent != "300" {
		t.Errorf("A S1 spent = %s, want 300 (the S2 transfer is savings-to-savings)", a[savingsUSDID].Spent)
	}

	bRows := h.savingsBudgetOf(t, tok, savingsFlagBudgetB, "2026-08-15").Item.Structure.Savings
	b := savingsByID(bRows)
	if _, ok := b[savingsEURID]; ok || len(bRows) != 1 {
		t.Fatalf("B savings = %+v, want only S1 (S2 is not flagged in B)", bRows)
	}
	if b[savingsUSDID].Spent != "322" {
		t.Errorf("B S1 spent = %s, want 322 (S2 is an everyday account in B)", b[savingsUSDID].Spent)
	}
}

func TestSavingsFlag_CloneCopiesFlag(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	flagSavings(t, h.tdb, budgetID1, savingsEURID, false)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/clone-budget", tok,
		map[string]any{"id": budgetID1, "newId": savingsFlagCopyID, "name": "Copy", "withLimits": true})

	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ?`, savingsFlagCopyID).Scan(&n); err != nil || n != 3 {
		t.Fatalf("copy members = %d (err %v), want all 3", n, err)
	}
	rows := h.savingsBudgetOf(t, tok, savingsFlagCopyID, "2026-08-15").Item.Structure.Savings
	if len(rows) != 1 || rows[0].Id != savingsUSDID || rows[0].Budgeted != "400" {
		t.Fatalf("copy savings = %+v, want only the flagged S1 with its plan", rows)
	}
}
