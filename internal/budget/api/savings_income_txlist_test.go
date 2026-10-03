package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func addDec(a, b string) string { return vo.NewDecimal(a).Add(vo.NewDecimal(b)).String() }

// get-transaction-list's accountId and income selectors: the item
// sheets' Transactions lists for a savings row and an income row.

type directedTxView struct {
	Items []struct {
		Id         string `json:"id"`
		Amount     string `json:"amount"`
		CurrencyId string `json:"currencyId"`
		Direction  string `json:"direction"`
		Type       string `json:"type"`
		Category   *struct {
			Name string `json:"name"`
		} `json:"category"`
	} `json:"items"`
}

func (h *harness) directedTxList(t *testing.T, tok, query string) directedTxView {
	t.Helper()
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-transaction-list?budgetId="+budgetID1+"&periodStart=2026-08-01"+query, tok, nil)
	return mustUnmarshal[directedTxView](t, env.Data)
}

// The list holds exactly what the row's Saved counts, each with its sign
// (direction) and kind (type) on the savings account's side.
func TestTxList_SavingsAccount(t *testing.T) {
	h, tok := newSavingsCategoriesBudget(t)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: savingsUSDID, AccountRecipientID: savingsEURID,
		Type: 2, Amount: "55", AmountRecipient: "50", SpentAt: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)})
	// another month: not listed
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: savingsUSDID, Type: 1, Amount: "99",
		SpentAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)})

	got := h.directedTxList(t, tok, "&accountId="+savingsUSDID)
	type row struct{ amount, direction, typ string }
	var rows []row
	signed := "0"
	for _, it := range got.Items {
		if it.CurrencyId != usdID {
			t.Errorf("item %s currency = %s, want the savings account's USD", it.Id, it.CurrencyId)
		}
		rows = append(rows, row{it.Amount, it.Direction, it.Type})
		if it.Direction == "in" {
			signed = addDec(signed, it.Amount)
		} else {
			signed = addDec(signed, "-"+it.Amount)
		}
	}
	// newest first: S1->S2 55, uncategorized 3 in / 1 out, salary 12, fee 5, E->S1 transfers
	want := []row{
		{"55", "out", "transfer"},
		{"3", "in", "income"}, {"1", "out", "expense"},
		{"12", "in", "income"}, {"5", "out", "expense"},
		{"500", "in", "transfer"}, {"200", "out", "transfer"},
	}
	if len(rows) != len(want) {
		t.Fatalf("items = %+v, want %d rows", rows, len(want))
	}
	for _, w := range want {
		found := false
		for _, r := range rows {
			if decEq(r.amount, w.amount) && r.direction == w.direction && r.typ == w.typ {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %+v in %+v", w, rows)
		}
	}
	// 500 - 200 + 12 + 3 - 5 - 1 - 55: the row's Saved
	if !decEq(signed, "254") {
		t.Errorf("signed sum = %s, want 254", signed)
	}

	// the EUR side of the S1->S2 move is listed in EUR on S2's list
	s2 := h.directedTxList(t, tok, "&accountId="+savingsEURID)
	var s2In []string
	for _, it := range s2.Items {
		s2In = append(s2In, it.Amount+" "+it.Direction)
	}
	if len(s2.Items) != 2 {
		t.Fatalf("S2 items = %v, want the 100 transfer in and the 50 move in", s2In)
	}
}

func TestTxList_SavingsAccountMustBeASavingsMember(t *testing.T) {
	h, tok := newSavingsCategoriesBudget(t)
	for _, q := range []string{
		"&accountId=" + accountID,                         // an everyday member
		"&accountId=aaaa9999-0000-7000-8000-000000000001", // no such account
		"&accountId=" + savingsUSDID + "&categoryId=" + catID,
		"&accountId=" + savingsUSDID + "&transfers=1",
	} {
		st, env := h.do(t, http.MethodGet, "/api/v1/budget/get-transaction-list?budgetId="+budgetID1+"&periodStart=2026-08-01"+q, tok, nil)
		if st != http.StatusBadRequest {
			t.Errorf("%s: status %d body %s, want 400", q, st, env.raw)
		}
	}
}

// An income category's list is what its plan cell counts: income in that
// category on the everyday accounts only.
func TestTxList_IncomeCategory(t *testing.T) {
	h, tok := newSavingsCategoriesBudget(t)
	got := h.directedTxList(t, tok, "&income=1&categoryId="+savingsSalaryCatID)
	if len(got.Items) != 1 {
		t.Fatalf("items = %+v, want the everyday 1000 only", got.Items)
	}
	it := got.Items[0]
	if !decEq(it.Amount, "1000") || it.Direction != "in" || it.Type != "income" || it.Category == nil || it.Category.Name != "Salary" {
		t.Errorf("item = %+v, want 1000 in/income in Salary", it)
	}

	for _, q := range []string{
		"&income=1",
		"&income=1&uncategorized=1",
		"&income=1&tagId=" + tagID,
		"&income=1&transfers=1",
	} {
		st, env := h.do(t, http.MethodGet, "/api/v1/budget/get-transaction-list?budgetId="+budgetID1+"&periodStart=2026-08-01"+q, tok, nil)
		if st != http.StatusBadRequest {
			t.Errorf("%s: status %d body %s, want 400", q, st, env.raw)
		}
	}
}
