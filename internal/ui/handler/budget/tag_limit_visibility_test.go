package budget_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// tagBudgetView is the slice of get-budget tests in this file care about.
type tagBudgetView struct {
	Item struct {
		Structure struct {
			Elements []struct {
				Id        string `json:"id"`
				Budgeted  string `json:"budgeted"`
				Available string `json:"available"`
			} `json:"elements"`
		} `json:"structure"`
	} `json:"item"`
}

// findElement returns (budgeted, available, present) for the element with id.
func (v tagBudgetView) findElement(id string) (string, string, bool) {
	for _, e := range v.Item.Structure.Elements {
		if e.Id == id {
			return e.Budgeted, e.Available, true
		}
	}
	return "", "", false
}

// TestTagWithLimitButNoTransactions_StaysVisible is the regression for a
// reported bug: a tag given a budget limit for a period, but with no
// transactions in (or before) that period, disappeared from get-budget. The
// structure builder gated tags on having a spending entry (transactions only)
// before ever considering the limit, so removing a tag's last transaction made
// its budget vanish. A tag must show when it has a limit even with zero spending
// (the rule is transactions OR budget OR non-zero available).
func TestTagWithLimitButNoTransactions_StaysVisible(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Tag Limit Budget"))

	// Set a limit on the tag — and deliberately create NO transaction for it.
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": tagID, "period": "2099-01-01", "amount": "500",
	})
	if st != http.StatusOK {
		t.Fatalf("set-limit=%d body=%s", st, env.raw)
	}

	status, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2099-01-15", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, b.raw)
	}
	type budgetView struct {
		Item struct {
			Structure struct {
				Elements []struct {
					Id        string `json:"id"`
					Budgeted  string `json:"budgeted"`
					Available string `json:"available"`
				} `json:"elements"`
			} `json:"structure"`
		} `json:"item"`
	}
	res := mustUnmarshal[budgetView](t, b.Data)

	var found bool
	var budgeted, available string
	for _, e := range res.Item.Structure.Elements {
		if e.Id == tagID {
			found, budgeted, available = true, e.Budgeted, e.Available
		}
	}
	if !found {
		t.Fatalf("tag with a limit but no transactions must stay visible in get-budget; elements: %s", b.Data)
	}
	if budgeted != "500" {
		t.Errorf("tag budgeted=%q want 500", budgeted)
	}
	// available = budgetedBefore - spentBefore - spent (carryover semantics). The
	// budget starts this month so there is no carry-in, and nothing was spent, so
	// available is "0" — the current month's allocation is reported in `budgeted`,
	// not `available`.
	if available != "0" {
		t.Errorf("tag available=%q want 0 (no carryover, nothing spent)", available)
	}
}

// TestTagLimit_VisibleAfterTaggedTransactionDeleted is the exact reported
// scenario: a tag with a limit and a tagged transaction shows in get-budget;
// after the transaction is deleted the tag must STILL show (its limit keeps it
// visible). Before the fix the tag vanished the moment its last transaction went
// away.
func TestTagLimit_VisibleAfterTaggedTransactionDeleted(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Repro Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	// Limit on the tag.
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok,
		map[string]any{"budgetId": budgetID1, "elementId": tagID, "period": "2024-04-01", "amount": "500"}); st != http.StatusOK {
		t.Fatalf("set-limit=%d body=%s", st, e.raw)
	}

	// A tagged expense in the period.
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	txID := f.Transaction(fixture.Transaction{
		ID: "eeee1111-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, TagID: tagID, Type: 0, Amount: "42.00", SpentAt: "2024-04-10 00:00:00",
	})

	// Sanity: with the transaction, the tag is present.
	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2024-04-15", tok, nil)
	if _, _, ok := mustUnmarshal[tagBudgetView](t, b.Data).findElement(tagID); !ok {
		t.Fatalf("precondition: tag should be present while it has a transaction; body: %s", b.Data)
	}

	// Delete the tagged transaction.
	if _, err := h.db.Exec(`DELETE FROM transactions WHERE id = ?`, txID); err != nil {
		t.Fatalf("delete transaction: %v", err)
	}

	// The tag must remain, carried by its limit alone.
	_, b = h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2024-04-15", tok, nil)
	budgeted, _, ok := mustUnmarshal[tagBudgetView](t, b.Data).findElement(tagID)
	if !ok {
		t.Fatalf("tag must stay visible after its last tagged transaction is deleted; body: %s", b.Data)
	}
	if budgeted != "500" {
		t.Errorf("tag budgeted=%q want 500 after deletion", budgeted)
	}
}

// TestTagWithoutLimitOrTransactions_StaysHidden guards the other side of the
// rule: the fix must NOT flood the budget with every tag. A tag with neither a
// limit nor any transactions stays hidden.
func TestTagWithoutLimitOrTransactions_StaysHidden(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Empty Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	// No limit, no transactions for the seeded tag.
	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2024-04-15", tok, nil)
	if _, _, ok := mustUnmarshal[tagBudgetView](t, b.Data).findElement(tagID); ok {
		t.Fatalf("a tag with no limit and no transactions must stay hidden; body: %s", b.Data)
	}
}
