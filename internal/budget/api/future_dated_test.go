package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// The clock (upcomingToday) is March 22nd 2030, 10:00 UTC: a transaction
// dated from March 23rd 00:00 on has not happened yet.

func TestFutureDated_CurrentMonthIsUpcomingNotSpent(t *testing.T) {
	h := newUpcomingHarness(t, "2030-03-01")
	for _, tx := range []fixture.Transaction{
		{Amount: "10", SpentAt: "2030-03-10 12:00:00"},
		{Amount: "5", SpentAt: "2030-03-22 23:59:59"},
		{Amount: "40", SpentAt: "2030-03-23 00:00:00"},
		{Amount: "2", SpentAt: "2030-03-30 18:00:00"},
	} {
		tx.UserID, tx.AccountID, tx.CategoryID = seedUserID, accountID, catID
		h.f.Transaction(tx)
	}
	cat := upcomingBudget(t, h, "2030-03-01").element(t, catID)
	if !decEq(cat.Spent, "15") || !decEq(cat.BudgetSpent, "15") {
		t.Errorf("category = %+v, want spent 15: today counts, from tomorrow on does not", cat)
	}
	if !decEq(cat.Upcoming, "42") || !decEq(cat.BudgetUpcoming, "42") {
		t.Errorf("category = %+v, want upcoming 42 (the future-dated 40 + 2)", cat)
	}
}

func TestFutureDated_TimezoneDecidesTomorrow(t *testing.T) {
	h := newUpcomingHarness(t, "2030-03-01")
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, Amount: "40", SpentAt: "2030-03-23 08:00:00"})
	// 10:00 UTC is already March 23rd on Kiritimati (UTC+14).
	st, env := h.doH(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2030-03-01", h.token(t), nil,
		map[string]string{"X-Timezone": "Pacific/Kiritimati"})
	if st != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", st, env.raw)
	}
	cat := mustUnmarshal[upcomingView](t, env.Data).element(t, catID)
	if !decEq(cat.Spent, "40") || !decEq(cat.Upcoming, "0") {
		t.Errorf("Kiritimati category = %+v, want spent 40, upcoming 0", cat)
	}
}

func TestFutureDated_FutureMonthAddsToRecurring(t *testing.T) {
	h := newUpcomingHarness(t, "2030-03-01", monthlyExpense("50", 25, false))
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, Amount: "40", SpentAt: "2030-04-05 12:00:00"})
	cat := upcomingBudget(t, h, "2030-04-01").element(t, catID)
	if !decEq(cat.Spent, "0") || !decEq(cat.Available, "0") {
		t.Errorf("April category = %+v, want spent 0, available 0", cat)
	}
	if !decEq(cat.Upcoming, "90") {
		t.Errorf("April category = %+v, want upcoming 90 (rent 50 + the future-dated 40)", cat)
	}
}

// A tag only a future-dated transaction carries shows up through its
// upcoming money, the way an upcoming-only recurring tag does.
func TestFutureDated_UpcomingOnlyTagIsAdded(t *testing.T) {
	h := newUpcomingHarness(t, "2030-03-01")
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, TagID: tagID, Amount: "40", SpentAt: "2030-04-05 12:00:00"})
	tag := upcomingBudget(t, h, "2030-04-01").element(t, tagID)
	if !decEq(tag.Spent, "0") || !decEq(tag.Upcoming, "40") {
		t.Errorf("tag = %+v, want spent 0, upcoming 40", tag)
	}
	if len(tag.Children) != 1 || tag.Children[0].Id != catID || !decEq(tag.Children[0].Upcoming, "40") {
		t.Errorf("tag children = %+v, want the category with upcoming 40", tag.Children)
	}
}

func TestFutureDated_LabelIsUpcomingNotSpent(t *testing.T) {
	const labelID = "eeee1111-0000-7000-8000-0000000000fd"
	h := newUpcomingHarness(t, "2030-03-01")
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Label(fixture.Label{ID: labelID, UserID: seedUserID, Name: "Trip", Icon: "flight"})
	for _, tx := range []fixture.Transaction{
		{Amount: "10", SpentAt: "2030-03-10 12:00:00"},
		{Amount: "40", SpentAt: "2030-03-28 12:00:00"},
	} {
		tx.UserID, tx.AccountID, tx.CategoryID = seedUserID, accountID, catID
		id := f.Transaction(tx)
		if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`, id, labelID); err != nil {
			t.Fatal(err)
		}
	}
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2030-03-01", h.token(t), nil)
	v := mustUnmarshal[struct {
		Item struct {
			Structure struct {
				Labels []struct {
					Id       string `json:"id"`
					Spent    string `json:"spent"`
					Upcoming string `json:"upcoming"`
				} `json:"labels"`
			} `json:"structure"`
		} `json:"item"`
	}](t, env.Data)
	for _, l := range v.Item.Structure.Labels {
		if l.Id == labelID {
			if !decEq(l.Spent, "10") || !decEq(l.Upcoming, "40") {
				t.Errorf("label = %+v, want spent 10, upcoming 40", l)
			}
			return
		}
	}
	t.Fatalf("label missing: %+v", v.Item.Structure.Labels)
}

// The savings clock is August 17th. A 150 deposit into S1 dated the 20th is
// not saved yet: S1 saved 300 of its 400 plan, so the closing balance is the
// 300 booked plus the 100 still planned, and the 150 is upcoming.
func TestFutureDated_SavingsDepositIsUpcoming(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, AccountRecipientID: savingsUSDID,
		Type: 2, Amount: "150", AmountRecipient: "150", SpentAt: "2026-08-20 12:00:00"})
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-08-01", tok, nil)
	v := mustUnmarshal[struct {
		Item struct {
			Structure struct {
				Savings []struct {
					Id             string `json:"id"`
					Spent          string `json:"spent"`
					Upcoming       string `json:"upcoming"`
					ClosingBalance string `json:"closingBalance"`
				} `json:"savings"`
			} `json:"structure"`
		} `json:"item"`
	}](t, env.Data)
	for _, r := range v.Item.Structure.Savings {
		if r.Id != savingsUSDID {
			continue
		}
		if !decEq(r.Spent, "300") || !decEq(r.Upcoming, "150") || !decEq(r.ClosingBalance, "400") {
			t.Errorf("S1 = %+v, want saved 300, upcoming 150, closing 400", r)
		}
		return
	}
	t.Fatalf("S1 missing: %+v", v.Item.Structure.Savings)
}

// Plan cells: the actual counts what has happened, the upcoming what is dated
// from tomorrow on, for expenses, income and savings alike.
func TestFutureDated_PlanCells(t *testing.T) {
	const incomeCatID = "cccc1111-0000-7000-8000-0000000000fe"
	h := newUpcomingHarness(t, "2030-01-01")
	h.f.Category(fixture.Category{ID: incomeCatID, UserID: seedUserID, Name: "Salary", Type: 1, Icon: "payments"})
	for _, tx := range []fixture.Transaction{
		{CategoryID: catID, Amount: "10", SpentAt: "2030-03-10 12:00:00"},
		{CategoryID: catID, Amount: "40", SpentAt: "2030-03-28 12:00:00"},
		{CategoryID: catID, Amount: "7", SpentAt: "2030-04-05 12:00:00"},
		{CategoryID: incomeCatID, Type: 1, Amount: "100", SpentAt: "2030-03-01 12:00:00"},
		{CategoryID: incomeCatID, Type: 1, Amount: "200", SpentAt: "2030-03-25 12:00:00"},
	} {
		tx.UserID, tx.AccountID = seedUserID, accountID
		h.f.Transaction(tx)
	}
	v, env := upcomingPlan(t, h, h.token(t), budgetID1, "&from=2030-02-01&months=3")
	els := v.Item.Structure.Elements
	cat := planRow(t, els, catID, 1)
	actual := func(cells []upcomingPlanCell) []string {
		out := make([]string, len(cells))
		for i, c := range cells {
			out[i] = c.Actual
		}
		return out
	}
	if got := actual(cat.Cells); !sameStrings(got, []string{"0", "10", "0"}) {
		t.Errorf("category actual = %v, want 0/10/0", got)
	}
	if got := cellUpcomings(cat.Cells); !sameStrings(got, []string{"0", "40", "7"}) {
		t.Errorf("category upcoming = %v, want 0/40/7", got)
	}
	inc := planRow(t, els, incomeCatID, 3)
	if got := actual(inc.Cells); !sameStrings(got, []string{"0", "100", "0"}) {
		t.Errorf("income actual = %v, want 0/100/0", got)
	}
	if got := cellUpcomings(inc.Cells); !sameStrings(got, []string{"0", "200", "0"}) {
		t.Errorf("income upcoming = %v, want 0/200/0", got)
	}
	// The opening balance of a window starting next month leaves the
	// future-dated money out too: 100 received - 10 spent.
	next := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget-plan?id="+budgetID1+"&from=2030-04-01&months=1", h.token(t), nil)
	opening := mustUnmarshal[struct {
		Item struct {
			OpeningBalances []struct {
				Amount string `json:"amount"`
			} `json:"openingBalances"`
		} `json:"item"`
	}](t, next.Data).Item.OpeningBalances
	if len(opening) != 1 || !decEq(opening[0].Amount, "90") {
		t.Errorf("April opening balances = %+v, want 90; plan body %s", opening, env.raw)
	}
}
