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

// ownerEditStub lets only the seeded user act on templates, and counts the
// CanEdit calls so the per-account cache is observable.
type ownerEditStub struct {
	recurringStub
	calls *int
}

func (s ownerEditStub) CanEdit(_ context.Context, userID, _ vo.Id) (bool, error) {
	*s.calls++
	return userID.String() == seedUserID, nil
}

type upcomingTx struct {
	Id          string   `json:"id"`
	Amount      string   `json:"amount"`
	SpentAt     string   `json:"spentAt"`
	LabelIds    []string `json:"labelIds"`
	TemplateId  string   `json:"templateId"`
	Schedule    string   `json:"schedule"`
	AccountId   string   `json:"accountId"`
	AccountName string   `json:"accountName"`
	LabelNames  []string `json:"labelNames"`
	IsNext      bool     `json:"isNext"`
	IsOverdue   bool     `json:"isOverdue"`
	CanEdit     bool     `json:"canEdit"`
	Category    *struct {
		Id string `json:"id"`
	} `json:"category"`
}

type upcomingTxList struct {
	Items    []upcomingTx `json:"items"`
	Upcoming []upcomingTx `json:"upcoming"`
}

func upcomingCategoryList(t *testing.T, h *harness, token, period string) (upcomingTxList, []byte) {
	t.Helper()
	st, env := h.do(t, http.MethodGet, "/api/v1/budget/get-transaction-list?budgetId="+budgetID1+"&periodStart="+period+"&categoryId="+catID, token, nil)
	if st != http.StatusOK {
		t.Fatalf("get-transaction-list=%d body=%s", st, env.raw)
	}
	return mustUnmarshal[upcomingTxList](t, env.Data), env.Data
}

// A weekly template due on the 23rd (clock: March 22nd) lands twice in March;
// only the earlier one is the template's next payment.
func TestTxList_UpcomingCategory(t *testing.T) {
	const (
		labelID      = "eeee1111-0000-7000-8000-000000000001"
		otherLabelID = "eeee1111-0000-7000-8000-000000000002"
	)
	rt := monthlyExpense("50.00", 23, false)
	rt.Schedule = model.RecurringScheduleWeekly
	rt.LabelIDs = []vo.Id{vo.MustParseId(labelID), vo.MustParseId(otherLabelID)}
	h := newUpcomingHarness(t, "2030-03-01", rt)
	h.f.Label(fixture.Label{ID: labelID, UserID: seedUserID, Name: "Home"})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, Amount: "50.00", SpentAt: "2030-03-10 12:00:00"})

	got, _ := upcomingCategoryList(t, h, h.token(t), "2030-03-01")
	if len(got.Items) != 1 || got.Items[0].TemplateId != "" {
		t.Fatalf("items = %+v, want the one real transaction", got.Items)
	}
	if len(got.Upcoming) != 2 {
		t.Fatalf("upcoming = %+v, want two weekly occurrences", got.Upcoming)
	}
	late, early := got.Upcoming[0], got.Upcoming[1]
	if late.SpentAt != "2030-03-30 00:00:00" || early.SpentAt != "2030-03-23 00:00:00" {
		t.Errorf("spentAt = %s, %s; want newest first (30th, 23rd)", late.SpentAt, early.SpentAt)
	}
	if late.IsNext || !early.IsNext {
		t.Errorf("isNext = %v, %v; want only the earliest", late.IsNext, early.IsNext)
	}
	for _, u := range got.Upcoming {
		if u.Id != rt.ID.String() || u.TemplateId != rt.ID.String() {
			t.Errorf("id/templateId = %s/%s, want the template id %s", u.Id, u.TemplateId, rt.ID)
		}
		if u.Amount != got.Items[0].Amount {
			t.Errorf("upcoming amount %q, want the real rows' format %q", u.Amount, got.Items[0].Amount)
		}
		if u.Schedule != "weekly" || u.AccountId != accountID || u.AccountName != "Cash" || u.IsOverdue || !u.CanEdit {
			t.Errorf("occurrence = %+v, want weekly on Cash, not overdue, editable", u)
		}
		if u.Category == nil || u.Category.Id != catID {
			t.Errorf("category = %+v, want %s", u.Category, catID)
		}
		if len(u.LabelIds) != 2 || len(u.LabelNames) != 1 || u.LabelNames[0] != "Home" {
			t.Errorf("labels = %v / %v, want both ids and only the known name", u.LabelIds, u.LabelNames)
		}
	}
}

// A monthly template due February 10th is overdue on March 22nd: it is filed
// in March but keeps its own date.
func TestTxList_UpcomingOverdue(t *testing.T) {
	rt := monthlyExpense("50", 10, false)
	rt.NextPaymentAt = time.Date(2030, 2, 10, 0, 0, 0, 0, time.UTC)
	h := newUpcomingHarness(t, "2030-01-01", rt)

	got, _ := upcomingCategoryList(t, h, h.token(t), "2030-03-01")
	if len(got.Upcoming) != 1 {
		t.Fatalf("upcoming = %+v, want the single overdue occurrence", got.Upcoming)
	}
	u := got.Upcoming[0]
	if u.SpentAt != "2030-02-10 00:00:00" || !u.IsOverdue || !u.IsNext {
		t.Errorf("overdue occurrence = %+v, want Feb 10th, overdue and next", u)
	}
	feb, _ := upcomingCategoryList(t, h, h.token(t), "2030-02-01")
	if len(feb.Upcoming) != 0 {
		t.Errorf("February upcoming = %+v, want none: overdue money files in the current month", feb.Upcoming)
	}
}

// A guest sees the owner's upcoming rows but may not act on them; CanEdit is
// asked once per account.
func TestTxList_UpcomingGuestCannotEdit(t *testing.T) {
	rt := monthlyExpense("50", 23, false)
	rt.Schedule = model.RecurringScheduleWeekly
	h := newUpcomingHarness(t, "2030-03-01")
	calls := 0
	h.svc.SetRecurring(ownerEditStub{recurringStub: recurringStub{templates: []*model.RecurringTransaction{rt}}, calls: &calls})
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: seedSalt})
	h.f.Connect(seedUserID, otherUserID)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/grant-access", h.token(t), map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": "guest"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": budgetID1})

	got, _ := upcomingCategoryList(t, h, otherUserID, "2030-03-01")
	if len(got.Upcoming) != 2 {
		t.Fatalf("guest upcoming = %+v, want both occurrences", got.Upcoming)
	}
	for _, u := range got.Upcoming {
		if u.CanEdit {
			t.Errorf("guest canEdit = true on %s", u.SpentAt)
		}
	}
	if calls != 1 {
		t.Errorf("CanEdit calls = %d, want 1 (cached per account)", calls)
	}
	if owner, _ := upcomingCategoryList(t, h, h.token(t), "2030-03-01"); len(owner.Upcoming) != 2 || !owner.Upcoming[0].CanEdit {
		t.Errorf("owner upcoming = %+v, want editable rows", owner.Upcoming)
	}
}

func TestTxList_UpcomingPastMonthEmpty(t *testing.T) {
	h := newUpcomingHarness(t, "2030-01-01", monthlyExpense("50", 25, false))
	got, raw := upcomingCategoryList(t, h, h.token(t), "2030-02-01")
	if len(got.Upcoming) != 0 || !bytes.Contains(raw, []byte(`"upcoming":[]`)) {
		t.Errorf("past month list = %s, want \"upcoming\":[]", raw)
	}
}
