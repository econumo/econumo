package api_test

import (
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
