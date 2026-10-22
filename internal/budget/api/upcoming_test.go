package api_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// upcomingToday is the 22nd: a template due on the 25th is still ahead.
var upcomingToday = tzClock{t: time.Date(2030, 3, 22, 10, 0, 0, 0, time.UTC)}

type recurringStub struct{ templates []*model.RecurringTransaction }

func (r recurringStub) ListByAccountIDs(_ context.Context, ids []vo.Id) ([]*model.RecurringTransaction, error) {
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

func (recurringStub) CanEdit(context.Context, vo.Id, vo.Id) (bool, error) { return true, nil }

func monthlyExpense(amount string, day int, tag bool) *model.RecurringTransaction {
	cat := vo.MustParseId(catID)
	rt := &model.RecurringTransaction{
		ID: vo.NewId(), UserID: vo.MustParseId(seedUserID), Type: model.TransactionTypeExpense,
		AccountID: vo.MustParseId(accountID), Amount: amount, CategoryID: &cat, Description: "rent",
		Schedule: model.RecurringScheduleMonthly, ScheduledDay: int16(day),
		NextPaymentAt: time.Date(2030, 3, day, 0, 0, 0, 0, time.UTC),
	}
	if tag {
		tg := vo.MustParseId(tagID)
		rt.TagID = &tg
	}
	return rt
}

type upcomingChild struct {
	Id             string `json:"id"`
	Spent          string `json:"spent"`
	Upcoming       string `json:"upcoming"`
	BudgetUpcoming string `json:"budgetUpcoming"`
}

type upcomingElement struct {
	Id             string          `json:"id"`
	Position       int             `json:"position"`
	Budgeted       string          `json:"budgeted"`
	BudgetSpent    string          `json:"budgetSpent"`
	Spent          string          `json:"spent"`
	Available      string          `json:"available"`
	Upcoming       string          `json:"upcoming"`
	BudgetUpcoming string          `json:"budgetUpcoming"`
	UpcomingBefore string          `json:"upcomingBefore"`
	Children       []upcomingChild `json:"children"`
}

type upcomingView struct {
	Item struct {
		Structure struct {
			Elements []upcomingElement `json:"elements"`
		} `json:"structure"`
	} `json:"item"`
}

func upcomingBudget(t *testing.T, h *harness, date string) upcomingView {
	t.Helper()
	st, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date="+date, h.token(t), nil)
	if st != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", st, env.raw)
	}
	return mustUnmarshal[upcomingView](t, env.Data)
}

func (v upcomingView) element(t *testing.T, id string) upcomingElement {
	t.Helper()
	for _, e := range v.Item.Structure.Elements {
		if e.Id == id {
			return e
		}
	}
	t.Fatalf("element %s missing: %+v", id, v.Item.Structure.Elements)
	return upcomingElement{}
}

func newUpcomingHarness(t *testing.T, startDate string, templates ...*model.RecurringTransaction) *harness {
	t.Helper()
	h := newHarnessWithClock(t, upcomingToday)
	h.svc.SetRecurring(recurringStub{templates: templates})
	req := createBudgetReq(budgetID1, "Upcoming")
	req["startDate"] = startDate
	if st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", h.token(t), req); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, env.raw)
	}
	return h
}

func TestBuildBudget_UpcomingCurrentMonth(t *testing.T) {
	h := newUpcomingHarness(t, "2030-03-01", monthlyExpense("50", 25, false))
	cat := upcomingBudget(t, h, "2030-03-01").element(t, catID)
	if cat.Upcoming != "50" || cat.BudgetUpcoming != "50" || cat.UpcomingBefore != "0" {
		t.Errorf("category upcoming = %+v, want upcoming/budgetUpcoming 50, upcomingBefore 0", cat)
	}
	if cat.Spent != "0" || cat.Available != "0" {
		t.Errorf("spent/available must stay real money only, got %+v", cat)
	}
}

func TestBuildBudget_UpcomingFutureMonthCarry(t *testing.T) {
	h := newUpcomingHarness(t, "2030-03-01", monthlyExpense("50", 25, false))
	cat := upcomingBudget(t, h, "2030-04-01").element(t, catID)
	if cat.Upcoming != "50" || cat.UpcomingBefore != "50" {
		t.Errorf("April category = %+v, want upcoming 50 and March's 50 as upcomingBefore", cat)
	}
	if cat.Spent != "0" || cat.Available != "0" {
		t.Errorf("spent/available must stay real money only, got %+v", cat)
	}
}

func TestBuildBudget_PastMonthNoUpcoming(t *testing.T) {
	h := newUpcomingHarness(t, "2030-01-01", monthlyExpense("50", 25, false))
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, TagID: tagID, Amount: "10", SpentAt: "2030-02-10 12:00:00"})
	v := upcomingBudget(t, h, "2030-02-01")
	if len(v.Item.Structure.Elements) == 0 {
		t.Fatal("no elements")
	}
	for _, e := range v.Item.Structure.Elements {
		if e.Upcoming != "0" || e.BudgetUpcoming != "0" || e.UpcomingBefore != "0" {
			t.Errorf("past month element %s = %+v, want every upcoming field \"0\"", e.Id, e)
		}
		for _, c := range e.Children {
			if c.Upcoming != "0" || c.BudgetUpcoming != "0" {
				t.Errorf("past month child %s = %+v, want \"0\"", c.Id, c)
			}
		}
	}
}

func TestBuildBudget_UpcomingTagChild(t *testing.T) {
	h := newUpcomingHarness(t, "2030-03-01", monthlyExpense("50", 25, true))
	// A real tagged spend puts the tag and its category child on screen.
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, TagID: tagID, Amount: "10", SpentAt: "2030-03-10 12:00:00"})
	tag := upcomingBudget(t, h, "2030-03-01").element(t, tagID)
	if tag.Spent != "10" || tag.Upcoming != "50" || tag.BudgetUpcoming != "50" {
		t.Errorf("tag = %+v, want spent 10, upcoming 50", tag)
	}
	if len(tag.Children) != 1 || tag.Children[0].Id != catID || tag.Children[0].Spent != "10" || tag.Children[0].Upcoming != "50" || tag.Children[0].BudgetUpcoming != "50" {
		t.Errorf("tag children = %+v, want the category with spent 10, upcoming 50", tag.Children)
	}
}

// A tagged rent with no real spend and no limit shows up only through its
// projection, the way a real tagged spend would have shown the tag.
func TestBuildBudget_UpcomingOnlyTagIsAdded(t *testing.T) {
	h := newUpcomingHarness(t, "2030-03-01", monthlyExpense("50", 25, true))
	v := upcomingBudget(t, h, "2030-04-01")
	tag := v.element(t, tagID)
	if tag.Budgeted != "0" || tag.Spent != "0" || tag.BudgetSpent != "0" || tag.Available != "0" {
		t.Errorf("tag real amounts = %+v, want zeros", tag)
	}
	if tag.Upcoming != "50" || tag.BudgetUpcoming != "50" || tag.UpcomingBefore != "50" {
		t.Errorf("tag upcoming = %+v, want 50 / 50 / March's 50", tag)
	}
	if len(tag.Children) != 1 || tag.Children[0].Id != catID || tag.Children[0].Spent != "0" || tag.Children[0].Upcoming != "50" {
		t.Errorf("tag children = %+v, want the category with upcoming 50", tag.Children)
	}
	positions := map[int]string{}
	for _, e := range v.Item.Structure.Elements {
		if other, dup := positions[e.Position]; dup {
			t.Errorf("elements %s and %s share position %d", other, e.Id, e.Position)
		}
		positions[e.Position] = e.Id
	}
}

// A participant's template on their own member account is the owner's
// upcoming money too, filed under the participant's category.
func TestBuildBudget_UpcomingFromParticipant(t *testing.T) {
	const partnerCat = "cccc2222-0000-7000-8000-000000000001"
	h := newUpcomingHarness(t, "2030-03-01")
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: seedSalt})
	h.f.Account(fixture.Account{ID: otherAccountID, UserID: otherUserID, CurrencyID: usdID})
	h.f.Category(fixture.Category{ID: partnerCat, UserID: otherUserID, Name: "Internet", Type: 0, Icon: "wifi"})
	h.f.Connect(seedUserID, otherUserID)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/grant-access", h.token(t), map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": "user"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": budgetID1})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/add-account", otherUserID, map[string]any{"id": budgetID1, "accountId": otherAccountID})

	rt := monthlyExpense("30", 25, false)
	rt.UserID, rt.AccountID = vo.MustParseId(otherUserID), vo.MustParseId(otherAccountID)
	cat := vo.MustParseId(partnerCat)
	rt.CategoryID = &cat
	h.svc.SetRecurring(recurringStub{templates: []*model.RecurringTransaction{rt}})

	got := upcomingBudget(t, h, "2030-03-01").element(t, partnerCat)
	if got.Spent != "0" || got.Upcoming != "30" || got.BudgetUpcoming != "30" || got.UpcomingBefore != "0" {
		t.Errorf("participant's category for the owner = %+v, want spent 0, upcoming 30", got)
	}
}

// A recurring Cash->S1 150 on the 25th (clock: August 17th) is money the
// closing balance can count on: each month from the current one adds its
// upcoming transfer plus whatever of the plan the booked and upcoming money
// still leave open. S1 books 300 in August against a 400 plan.
func TestSavingsClosing_UpcomingTransfer(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	recip := vo.MustParseId(savingsUSDID)
	h.svc.SetRecurring(recurringStub{templates: []*model.RecurringTransaction{{
		ID: vo.NewId(), UserID: vo.MustParseId(seedUserID), Type: model.TransactionTypeTransfer,
		AccountID: vo.MustParseId(accountID), AccountRecipID: &recip, Amount: "150", Description: "save",
		Schedule: model.RecurringScheduleMonthly, ScheduledDay: 25,
		NextPaymentAt: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
	}}})

	closing := func(date string) string {
		t.Helper()
		return h.savingsClosings(t, tok, date)[savingsUSDID][0]
	}
	// July is past: booked only.
	if got := closing("2026-07-15"); !decEq(got, "0") {
		t.Errorf("July closing = %s, want 0", got)
	}
	// 300 booked + 150 upcoming; the 400 plan is already met.
	if got := closing("2026-08-15"); !decEq(got, "450") {
		t.Errorf("August closing = %s, want 450", got)
	}
	// September without a plan still counts its 150.
	if got := closing("2026-09-15"); !decEq(got, "600") {
		t.Errorf("September closing (no plan) = %s, want 600", got)
	}
	h.setLimit(t, tok, savingsUSDID, "2026-09-01", "100")
	if got := closing("2026-09-15"); !decEq(got, "600") {
		t.Errorf("September closing (plan 100) = %s, want 600", got)
	}
	h.setLimit(t, tok, savingsUSDID, "2026-09-01", "200")
	if got := closing("2026-09-15"); !decEq(got, "650") {
		t.Errorf("September closing (plan 200) = %s, want 650", got)
	}
}

type upcomingPlanCell struct {
	Actual         string `json:"actual"`
	Planned        string `json:"planned"`
	Upcoming       string `json:"upcoming"`
	ClosingBalance string `json:"closingBalance"`
}

type upcomingPlanRow struct {
	Id       string             `json:"id"`
	Type     int                `json:"type"`
	FolderId *string            `json:"folderId"`
	Position int                `json:"position"`
	Cells    []upcomingPlanCell `json:"cells"`
	Children []struct {
		Id    string             `json:"id"`
		Cells []upcomingPlanCell `json:"cells"`
	} `json:"children"`
}

type upcomingAmount struct {
	CurrencyId string `json:"currencyId"`
	Amount     string `json:"amount"`
}

type upcomingFlow struct {
	Month      string `json:"month"`
	CurrencyId string `json:"currencyId"`
	Amount     string `json:"amount"`
}

type upcomingPlanView struct {
	Item struct {
		Structure struct {
			Elements []upcomingPlanRow `json:"elements"`
			Savings  []upcomingPlanRow `json:"savings"`
		} `json:"structure"`
		UpcomingOpeningBalances        []upcomingAmount `json:"upcomingOpeningBalances"`
		UpcomingSavingsOpeningBalances []upcomingAmount `json:"upcomingSavingsOpeningBalances"`
		UpcomingTransfers              []struct {
			Period string `json:"period"`
			Items  []struct {
				CurrencyId string `json:"currencyId"`
				In         string `json:"in"`
				Out        string `json:"out"`
			} `json:"items"`
		} `json:"upcomingTransfers"`
		UpcomingSavingsFlows         []upcomingFlow `json:"upcomingSavingsFlows"`
		UpcomingSavingsIncomeExpense []upcomingFlow `json:"upcomingSavingsIncomeExpense"`
	} `json:"item"`
}

func upcomingPlan(t *testing.T, h *harness, tok, budgetID, window string) (upcomingPlanView, envelope) {
	t.Helper()
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget-plan?id="+budgetID+window, tok, nil)
	return mustUnmarshal[upcomingPlanView](t, env.Data), env
}

func planRow(t *testing.T, rows []upcomingPlanRow, id string, typ int) upcomingPlanRow {
	t.Helper()
	for _, r := range rows {
		if r.Id == id && r.Type == typ {
			return r
		}
	}
	t.Fatalf("plan row %s/%d missing: %+v", id, typ, rows)
	return upcomingPlanRow{}
}

func cellUpcomings(cells []upcomingPlanCell) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = c.Upcoming
	}
	return out
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if !decEq(got[i], want[i]) {
			return false
		}
	}
	return true
}

// The clock is March 22nd: February is past, March holds the rent still due
// on the 25th, and every later window month holds its own. The tagged gym has
// no real spend and no plan, so its tag row shows through the projection.
func TestBuildPlan_UpcomingCells(t *testing.T) {
	h := newUpcomingHarness(t, "2030-01-01", monthlyExpense("50", 25, false), monthlyExpense("20", 25, true))
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, Amount: "10", SpentAt: "2030-03-10 12:00:00"})
	v, _ := upcomingPlan(t, h, h.token(t), budgetID1, "&from=2030-02-01&months=4")
	els := v.Item.Structure.Elements

	cat := planRow(t, els, catID, 1)
	if got := cellUpcomings(cat.Cells); !sameStrings(got, []string{"0", "50", "50", "50"}) {
		t.Errorf("category upcoming = %v, want 0/50/50/50", got)
	}
	if got := []string{cat.Cells[0].Actual, cat.Cells[1].Actual, cat.Cells[2].Actual, cat.Cells[3].Actual}; !sameStrings(got, []string{"0", "10", "0", "0"}) {
		t.Errorf("category actual = %v, want real money only (0/10/0/0)", got)
	}

	tag := planRow(t, els, tagID, 2)
	if got := cellUpcomings(tag.Cells); !sameStrings(got, []string{"0", "20", "20", "20"}) {
		t.Errorf("tag upcoming = %v, want 0/20/20/20", got)
	}
	for i, c := range tag.Cells {
		if c.Actual != "0" || c.Planned != "" {
			t.Errorf("tag cell %d = %+v, want no real money and no plan", i, c)
		}
	}
	if len(tag.Children) != 1 || tag.Children[0].Id != catID || !sameStrings(cellUpcomings(tag.Children[0].Cells), []string{"0", "20", "20", "20"}) {
		t.Errorf("tag children = %+v, want the category with upcoming 0/20/20/20", tag.Children)
	}
	positions := map[[2]any]string{}
	for _, e := range els {
		k := [2]any{e.FolderId == nil, e.Position}
		if e.FolderId != nil {
			k[0] = *e.FolderId
		}
		if other, dup := positions[k]; dup {
			t.Errorf("rows %s and %s share position %d", other, e.Id, e.Position)
		}
		positions[k] = e.Id
	}
}

// A window ending before the current month projects nothing: every upcoming
// field is zero, and the arrays keep their real counterparts' shape.
func TestBuildPlan_WindowInPastNoUpcoming(t *testing.T) {
	h := newUpcomingHarness(t, "2030-01-01", monthlyExpense("50", 25, false), monthlyExpense("20", 25, true))
	v, env := upcomingPlan(t, h, h.token(t), budgetID1, "&from=2030-01-01&months=2")
	for _, e := range v.Item.Structure.Elements {
		if e.Id == tagID {
			t.Errorf("tag with neither real nor projected money must stay hidden: %+v", e)
		}
		if !sameStrings(cellUpcomings(e.Cells), []string{"0", "0"}) {
			t.Errorf("row %s upcoming = %v, want zeros", e.Id, cellUpcomings(e.Cells))
		}
	}
	if len(v.Item.UpcomingOpeningBalances) == 0 {
		t.Error("upcomingOpeningBalances must list the budget's currencies")
	}
	for _, b := range v.Item.UpcomingOpeningBalances {
		if b.Amount != "0" {
			t.Errorf("upcoming opening balance = %+v, want 0", b)
		}
	}
	if tr := v.Item.UpcomingTransfers; len(tr) != 2 || tr[0].Period != "2030-01-01" || len(tr[0].Items) != 0 || len(tr[1].Items) != 0 {
		t.Errorf("upcomingTransfers = %+v, want two empty months", tr)
	}
	for _, key := range []string{`"upcomingSavingsOpeningBalances":[]`, `"upcomingSavingsFlows":[]`, `"upcomingSavingsIncomeExpense":[]`} {
		if !bytes.Contains(env.Data, []byte(key)) {
			t.Errorf("response lacks %s: %s", key, env.Data)
		}
	}
}

// Clock: August 17th. Cash->S1 150 and Cash->an account outside the budget 40,
// both monthly on the 25th: S1's cells and the savings flows carry the 150, the
// boundary transfers line the 40, and S1's closing balance stays booked only.
func TestBuildPlan_UpcomingTransfersAndSavings(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	const outsideID = "aaaa1111-0000-7000-8000-0000000000ff"
	h.f.Account(fixture.Account{ID: outsideID, UserID: seedUserID, CurrencyID: usdID, Name: "Outside"})
	booked, _ := upcomingPlan(t, h, tok, budgetID1, "&from=2026-08-01&months=2")

	transfer := func(to, amount string) *model.RecurringTransaction {
		recip := vo.MustParseId(to)
		return &model.RecurringTransaction{
			ID: vo.NewId(), UserID: vo.MustParseId(seedUserID), Type: model.TransactionTypeTransfer,
			AccountID: vo.MustParseId(accountID), AccountRecipID: &recip, Amount: amount, Description: "move",
			Schedule: model.RecurringScheduleMonthly, ScheduledDay: 25,
			NextPaymentAt: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
		}
	}
	h.svc.SetRecurring(recurringStub{templates: []*model.RecurringTransaction{transfer(savingsUSDID, "150"), transfer(outsideID, "40")}})

	v, _ := upcomingPlan(t, h, tok, budgetID1, "&from=2026-08-01&months=2")
	s1 := planRow(t, v.Item.Structure.Savings, savingsUSDID, 5)
	if got := cellUpcomings(s1.Cells); !sameStrings(got, []string{"150", "150"}) {
		t.Errorf("S1 upcoming = %v, want 150/150", got)
	}
	was := planRow(t, booked.Item.Structure.Savings, savingsUSDID, 5)
	for i := range s1.Cells {
		if s1.Cells[i].ClosingBalance != was.Cells[i].ClosingBalance || s1.Cells[i].Actual != was.Cells[i].Actual {
			t.Errorf("S1 cell %d = %+v, want the booked %+v plus upcoming", i, s1.Cells[i], was.Cells[i])
		}
	}
	if f := v.Item.UpcomingSavingsFlows; len(f) != 2 || f[0].Month != "2026-08-01" || f[1].Month != "2026-09-01" ||
		f[0].CurrencyId != usdID || !decEq(f[0].Amount, "150") || !decEq(f[1].Amount, "150") {
		t.Errorf("upcomingSavingsFlows = %+v, want 150 USD in August and September", f)
	}
	if ie := v.Item.UpcomingSavingsIncomeExpense; len(ie) != 0 {
		t.Errorf("upcomingSavingsIncomeExpense = %+v, want none (transfers only)", ie)
	}
	tr := v.Item.UpcomingTransfers
	if len(tr) != 2 {
		t.Fatalf("upcomingTransfers = %+v, want two months", tr)
	}
	for _, m := range tr {
		if len(m.Items) != 1 || m.Items[0].CurrencyId != usdID || !decEq(m.Items[0].Out, "40") || !decEq(m.Items[0].In, "0") {
			t.Errorf("upcomingTransfers %s = %+v, want 40 USD out", m.Period, m.Items)
		}
	}
	for _, b := range v.Item.UpcomingOpeningBalances {
		if !decEq(b.Amount, "0") {
			t.Errorf("opening balance %+v, want 0 for a window starting this month", b)
		}
	}

	// A September window opens on what August still has due: -40 across the
	// budget, +150 on the savings.
	sep, _ := upcomingPlan(t, h, tok, budgetID1, "&from=2026-09-01&months=1")
	for _, b := range sep.Item.UpcomingOpeningBalances {
		want := "0"
		if b.CurrencyId == usdID {
			want = "-40"
		}
		if !decEq(b.Amount, want) {
			t.Errorf("September upcoming opening %+v, want %s", b, want)
		}
	}
	for _, b := range sep.Item.UpcomingSavingsOpeningBalances {
		want := "0"
		if b.CurrencyId == usdID {
			want = "150"
		}
		if !decEq(b.Amount, want) {
			t.Errorf("September upcoming savings opening %+v, want %s", b, want)
		}
	}
}
