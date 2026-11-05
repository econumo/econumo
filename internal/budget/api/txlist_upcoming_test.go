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
	CurrencyId  string   `json:"currencyId"`
	Direction   string   `json:"direction"`
	Type        string   `json:"type"`
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

	AccountRecipientId   *string `json:"accountRecipientId"`
	AccountRecipientName *string `json:"accountRecipientName"`
	AmountRecipient      *string `json:"amountRecipient"`
	CurrencyRecipientId  *string `json:"currencyRecipientId"`
	AmountSource         *string `json:"amountSource"`
	CurrencySourceId     *string `json:"currencySourceId"`

	Category *struct {
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

// augustTemplate is a monthly template due August 25th 2026, the month the
// savings budget's clock (August 17th) is in.
func augustTemplate(typ model.TransactionType, from, to, amount, category string) *model.RecurringTransaction {
	rt := &model.RecurringTransaction{
		ID: vo.NewId(), UserID: vo.MustParseId(seedUserID), Type: typ,
		AccountID: vo.MustParseId(from), Amount: amount, Description: "auto",
		Schedule: model.RecurringScheduleMonthly, ScheduledDay: 25,
		NextPaymentAt: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
	}
	if to != "" {
		recip := vo.MustParseId(to)
		rt.AccountRecipID = &recip
	}
	if category != "" {
		cat := vo.MustParseId(category)
		rt.CategoryID = &cat
	}
	return rt
}

func augustUpcoming(t *testing.T, h *harness, tok, query string) []upcomingTx {
	t.Helper()
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-transaction-list?budgetId="+budgetID1+"&periodStart=2026-08-01"+query, tok, nil)
	return mustUnmarshal[upcomingTxList](t, env.Data).Upcoming
}

func upcomingByTemplate(t *testing.T, rows []upcomingTx, rt *model.RecurringTransaction) upcomingTx {
	t.Helper()
	for _, r := range rows {
		if r.TemplateId == rt.ID.String() {
			return r
		}
	}
	t.Fatalf("template %s missing from %+v", rt.ID, rows)
	return upcomingTx{}
}

// A transfer from an account outside the budget into a EUR member is listed
// in the recipient's currency, as money in, without the outside account's
// name; one leaving the budget keeps its member account's name.
func TestTxList_UpcomingTransfers(t *testing.T) {
	h, tok, eur := newSavingsBudget(t)
	const privateID = "aaaa1111-0000-7000-8000-0000000000fe"
	h.f.Account(fixture.Account{ID: privateID, UserID: seedUserID, CurrencyID: usdID, Name: "Private"})
	in := augustTemplate(model.TransactionTypeTransfer, privateID, savingsEURID, "100", "")
	out := augustTemplate(model.TransactionTypeTransfer, accountID, privateID, "40", "")
	inside := augustTemplate(model.TransactionTypeTransfer, accountID, savingsUSDID, "150", "")
	h.svc.SetRecurring(recurringStub{templates: []*model.RecurringTransaction{in, out, inside}})

	got := augustUpcoming(t, h, tok, "&transfers=1")
	if len(got) != 2 {
		t.Fatalf("upcoming = %+v, want the two crossing transfers only", got)
	}
	u := upcomingByTemplate(t, got, in)
	if u.CurrencyId != eur || !decEq(u.Amount, "91") || u.Direction != "in" {
		t.Errorf("transfer in = %+v, want 91 EUR in (100 USD at %s)", u, augustEURRate)
	}
	if !decEq(deref(u.AmountSource), "100") || deref(u.CurrencySourceId) != usdID ||
		!decEq(deref(u.AmountRecipient), "91") || deref(u.CurrencyRecipientId) != eur {
		t.Errorf("transfer in sides = %s %s -> %s %s, want 100 USD -> 91 EUR",
			deref(u.AmountSource), deref(u.CurrencySourceId), deref(u.AmountRecipient), deref(u.CurrencyRecipientId))
	}
	if u.AccountId != "" || u.AccountName != "" {
		t.Errorf("transfer in account = %q/%q, want neither id nor name for a non-member", u.AccountId, u.AccountName)
	}
	o := upcomingByTemplate(t, got, out)
	if o.CurrencyId != usdID || !decEq(o.Amount, "40") || o.Direction != "out" || o.AccountName != "Cash" {
		t.Errorf("transfer out = %+v, want 40 USD out of Cash", o)
	}
}

// An income template is listed on its income category's row when it lands on
// an everyday account; one on a savings account belongs to that row instead.
func TestTxList_UpcomingIncome(t *testing.T) {
	h, tok := newSavingsCategoriesBudget(t)
	everyday := augustTemplate(model.TransactionTypeIncome, accountID, "", "2000", savingsSalaryCatID)
	onSavings := augustTemplate(model.TransactionTypeIncome, savingsUSDID, "", "30", savingsSalaryCatID)
	h.svc.SetRecurring(recurringStub{templates: []*model.RecurringTransaction{everyday, onSavings}})

	got := augustUpcoming(t, h, tok, "&income=1&categoryId="+savingsSalaryCatID)
	if len(got) != 1 {
		t.Fatalf("upcoming = %+v, want the everyday income only", got)
	}
	u := got[0]
	if u.TemplateId != everyday.ID.String() || !decEq(u.Amount, "2000") || u.Direction != "in" || u.Type != "income" ||
		u.Category == nil || u.Category.Id != savingsSalaryCatID {
		t.Errorf("income = %+v, want 2000 in/income in Salary", u)
	}
}

// A savings row's list holds every template touching the account, each with
// its direction on the savings side.
func TestTxList_UpcomingSavingsAccount(t *testing.T) {
	h, tok := newSavingsCategoriesBudget(t)
	save := augustTemplate(model.TransactionTypeTransfer, accountID, savingsUSDID, "150", "")
	fee := augustTemplate(model.TransactionTypeExpense, savingsUSDID, "", "5", catID)
	elsewhere := augustTemplate(model.TransactionTypeExpense, accountID, "", "7", catID)
	h.svc.SetRecurring(recurringStub{templates: []*model.RecurringTransaction{save, fee, elsewhere}})

	got := augustUpcoming(t, h, tok, "&accountId="+savingsUSDID)
	if len(got) != 2 {
		t.Fatalf("upcoming = %+v, want the transfer in and the fee", got)
	}
	if s := upcomingByTemplate(t, got, save); !decEq(s.Amount, "150") || s.Direction != "in" || s.Type != "transfer" || s.AccountName != "Cash" {
		t.Errorf("transfer = %+v, want 150 in/transfer from Cash", s)
	}
	if f := upcomingByTemplate(t, got, fee); !decEq(f.Amount, "5") || f.Direction != "out" || f.Type != "expense" || f.AccountName != "Rainy day" {
		t.Errorf("fee = %+v, want 5 out/expense on Rainy day", f)
	}
}

func deref(p *string) string {
	if p == nil {
		return "<absent>"
	}
	return *p
}

// A transfer's occurrence names its recipient: a member's id and name, the
// amount converted into its currency. A sender outside the budget stays
// blank, a recipient outside it would too; a non-transfer carries "" in every
// recipient field.
func TestTxList_UpcomingTransferRecipient(t *testing.T) {
	h, tok, eur := newSavingsBudget(t)
	const privateID = "aaaa1111-0000-7000-8000-0000000000fe"
	h.f.Account(fixture.Account{ID: privateID, UserID: seedUserID, CurrencyID: usdID, Name: "Private"})
	cross := augustTemplate(model.TransactionTypeTransfer, accountID, savingsEURID, "100", "")
	fromPrivate := augustTemplate(model.TransactionTypeTransfer, privateID, savingsUSDID, "60", "")
	fee := augustTemplate(model.TransactionTypeExpense, savingsEURID, "", "5", catID)
	h.svc.SetRecurring(recurringStub{templates: []*model.RecurringTransaction{cross, fromPrivate, fee}})

	eurRows := augustUpcoming(t, h, tok, "&accountId="+savingsEURID)
	c := upcomingByTemplate(t, eurRows, cross)
	if deref(c.AccountRecipientId) != savingsEURID || deref(c.AccountRecipientName) != "Euro pot" ||
		!decEq(deref(c.AmountRecipient), "91") || deref(c.CurrencyRecipientId) != eur {
		t.Errorf("cross-currency recipient = %s/%s %s %s, want Euro pot 91 EUR",
			deref(c.AccountRecipientId), deref(c.AccountRecipientName), deref(c.AmountRecipient), deref(c.CurrencyRecipientId))
	}
	if !decEq(deref(c.AmountSource), "100") || deref(c.CurrencySourceId) != usdID {
		t.Errorf("cross-currency source = %s %s, want 100 USD", deref(c.AmountSource), deref(c.CurrencySourceId))
	}
	if c.AccountId != accountID || c.AccountName != "Cash" {
		t.Errorf("cross-currency sender = %q/%q, want Cash", c.AccountId, c.AccountName)
	}
	f := upcomingByTemplate(t, eurRows, fee)
	for name, v := range map[string]*string{"accountRecipientId": f.AccountRecipientId, "accountRecipientName": f.AccountRecipientName,
		"amountRecipient": f.AmountRecipient, "currencyRecipientId": f.CurrencyRecipientId,
		"amountSource": f.AmountSource, "currencySourceId": f.CurrencySourceId} {
		if deref(v) != "" {
			t.Errorf("expense %s = %q, want \"\"", name, deref(v))
		}
	}

	p := upcomingByTemplate(t, augustUpcoming(t, h, tok, "&accountId="+savingsUSDID), fromPrivate)
	if p.AccountId != "" || p.AccountName != "" {
		t.Errorf("private sender = %q/%q, want blank", p.AccountId, p.AccountName)
	}
	if !decEq(deref(p.AmountSource), "60") || deref(p.CurrencySourceId) != usdID {
		t.Errorf("private sender amount = %s %s, want 60 USD", deref(p.AmountSource), deref(p.CurrencySourceId))
	}
	if deref(p.AccountRecipientId) != savingsUSDID || deref(p.AccountRecipientName) != "Rainy day" ||
		!decEq(deref(p.AmountRecipient), "60") || deref(p.CurrencyRecipientId) != usdID {
		t.Errorf("private->member recipient = %s/%s %s %s, want Rainy day 60 USD",
			deref(p.AccountRecipientId), deref(p.AccountRecipientName), deref(p.AmountRecipient), deref(p.CurrencyRecipientId))
	}

	out := augustTemplate(model.TransactionTypeTransfer, accountID, privateID, "40", "")
	h.svc.SetRecurring(recurringStub{templates: []*model.RecurringTransaction{out}})
	o := upcomingByTemplate(t, augustUpcoming(t, h, tok, "&transfers=1"), out)
	if deref(o.AccountRecipientId) != "" || deref(o.AccountRecipientName) != "" || !decEq(deref(o.AmountRecipient), "40") || deref(o.CurrencyRecipientId) != usdID {
		t.Errorf("member->private recipient = %s/%s %s %s, want blank id/name, 40 USD",
			deref(o.AccountRecipientId), deref(o.AccountRecipientName), deref(o.AmountRecipient), deref(o.CurrencyRecipientId))
	}
}
