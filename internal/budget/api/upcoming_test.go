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

// recurringStub returns every template whose source or transfer recipient is
// in the requested set, like the real glue.
type recurringStub struct{ templates []*model.RecurringTransaction }

func (r recurringStub) ListByAccountIDs(_ context.Context, ids []vo.Id) ([]*model.RecurringTransaction, error) {
	in := map[string]bool{}
	for _, id := range ids {
		in[id.String()] = true
	}
	var out []*model.RecurringTransaction
	for _, rt := range r.templates {
		if in[rt.AccountID.String()] || (rt.AccountRecipID != nil && in[rt.AccountRecipID.String()]) {
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
	if cat.Upcoming != "50" || cat.BudgetUpcoming != "50" {
		t.Errorf("category upcoming = %+v, want upcoming/budgetUpcoming 50", cat)
	}
	if cat.Spent != "0" || cat.Available != "0" {
		t.Errorf("spent/available must stay real money only, got %+v", cat)
	}
}

func TestBuildBudget_UpcomingFutureMonth(t *testing.T) {
	h := newUpcomingHarness(t, "2030-03-01", monthlyExpense("50", 25, false))
	cat := upcomingBudget(t, h, "2030-04-01").element(t, catID)
	if cat.Upcoming != "50" {
		t.Errorf("April category = %+v, want upcoming 50", cat)
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
		if e.Upcoming != "0" || e.BudgetUpcoming != "0" {
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
	if tag.Upcoming != "50" || tag.BudgetUpcoming != "50" {
		t.Errorf("tag upcoming = %+v, want 50 / 50", tag)
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
	if got.Spent != "0" || got.Upcoming != "30" || got.BudgetUpcoming != "30" {
		t.Errorf("participant's category for the owner = %+v, want spent 0, upcoming 30", got)
	}
}

// A recurring Cash->S1 150 on the 25th (clock: August 17th) is a planning hint
// only: the closing balance is the booked one plus each month's unmet plan.
// S1 books 300 in August against a 400 plan.
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
	if got := closing("2026-07-15"); !decEq(got, "0") {
		t.Errorf("July closing = %s, want 0", got)
	}
	// 300 booked + the 100 of the 400 plan still open.
	if got := closing("2026-08-15"); !decEq(got, "400") {
		t.Errorf("August closing = %s, want 400", got)
	}
	if got := closing("2026-09-15"); !decEq(got, "400") {
		t.Errorf("September closing (no plan) = %s, want 400", got)
	}
	h.setLimit(t, tok, savingsUSDID, "2026-09-01", "100")
	if got := closing("2026-09-15"); !decEq(got, "500") {
		t.Errorf("September closing (plan 100) = %s, want 500", got)
	}
}

// A monthly 100 transfer into a savings member with no plan shows as that
// row's upcoming and leaves the closing balance at the booked one.
func TestUpcomingNotInSavingsClosing(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	booked := h.savingsClosings(t, tok, "2026-09-15")[savingsEURID][0]
	recip := vo.MustParseId(savingsEURID)
	h.svc.SetRecurring(recurringStub{templates: []*model.RecurringTransaction{{
		ID: vo.NewId(), UserID: vo.MustParseId(seedUserID), Type: model.TransactionTypeTransfer,
		AccountID: vo.MustParseId(accountID), AccountRecipID: &recip, Amount: "100", Description: "save",
		Schedule: model.RecurringScheduleMonthly, ScheduledDay: 25,
		NextPaymentAt: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
	}}})
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-09-01", tok, nil)
	type view struct {
		Item struct {
			Structure struct {
				Savings []struct {
					Id             string `json:"id"`
					Upcoming       string `json:"upcoming"`
					ClosingBalance string `json:"closingBalance"`
				} `json:"savings"`
			} `json:"structure"`
		} `json:"item"`
	}
	v := mustUnmarshal[view](t, env.Data)
	for _, r := range v.Item.Structure.Savings {
		if r.Id != savingsEURID {
			continue
		}
		if decEq(r.Upcoming, "0") {
			t.Errorf("upcoming = %s, want the transfer's money", r.Upcoming)
		}
		if !decEq(r.ClosingBalance, booked) {
			t.Errorf("closingBalance = %s, want the booked %s", r.ClosingBalance, booked)
		}
		return
	}
	t.Fatalf("savings row %s missing", savingsEURID)
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

type upcomingPlanView struct {
	Item struct {
		Structure struct {
			Elements []upcomingPlanRow `json:"elements"`
			Savings  []upcomingPlanRow `json:"savings"`
		} `json:"structure"`
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
// field is zero.
func TestBuildPlan_WindowInPastNoUpcoming(t *testing.T) {
	h := newUpcomingHarness(t, "2030-01-01", monthlyExpense("50", 25, false), monthlyExpense("20", 25, true))
	v, _ := upcomingPlan(t, h, h.token(t), budgetID1, "&from=2030-01-01&months=2")
	for _, e := range v.Item.Structure.Elements {
		if e.Id == tagID {
			t.Errorf("tag with neither real nor projected money must stay hidden: %+v", e)
		}
		if !sameStrings(cellUpcomings(e.Cells), []string{"0", "0"}) {
			t.Errorf("row %s upcoming = %v, want zeros", e.Id, cellUpcomings(e.Cells))
		}
	}
}

// A budget no template touches answers get-budget and get-budget-plan with
// the bytes of one without a recurring source at all.
func TestUpcoming_NoTemplatesSameBytes(t *testing.T) {
	h := newUpcomingHarness(t, "2030-01-01")
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, Amount: "10", SpentAt: "2030-03-10 12:00:00"})
	paths := []string{
		"/api/v1/budget/get-budget?id=" + budgetID1 + "&date=2030-03-01",
		"/api/v1/budget/get-budget?id=" + budgetID1 + "&date=2030-04-01",
		"/api/v1/budget/get-budget-plan?id=" + budgetID1 + "&from=2030-02-01&months=3",
	}
	read := func() [][]byte {
		var out [][]byte
		for _, p := range paths {
			out = append(out, h.mustDo(t, http.MethodGet, p, h.token(t), nil).Data)
		}
		return out
	}
	withSource := read()
	h.svc.SetRecurring(nil)
	for i, want := range read() {
		if !bytes.Equal(withSource[i], want) {
			t.Errorf("%s:\n with source %s\n without     %s", paths[i], withSource[i], want)
		}
	}
}
