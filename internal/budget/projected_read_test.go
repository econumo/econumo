package budget

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	prUser     = "01900000-0000-7000-8000-000000000001"
	prEveryday = "01900000-0000-7000-8000-0000000000a1"
	prSavings  = "01900000-0000-7000-8000-0000000000a2"
	prOutside  = "01900000-0000-7000-8000-0000000000a3"
	prUSD      = "01900000-0000-7000-8000-0000000000c1"
	prEUR      = "01900000-0000-7000-8000-0000000000c2"
	prCat      = "01900000-0000-7000-8000-0000000000d1"
	prTag      = "01900000-0000-7000-8000-0000000000e1"
	prLabel    = "01900000-0000-7000-8000-0000000000f1"
)

func prID(t *testing.T, s string) vo.Id {
	t.Helper()
	id, err := vo.ParseId(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func prIDp(t *testing.T, s string) *vo.Id {
	id := prID(t, s)
	return &id
}

// recurringSourceStub returns every template whose source or recipient is in
// the requested set, like the real glue.
type recurringSourceStub struct{ templates []*model.RecurringTransaction }

func (r *recurringSourceStub) ListByAccountIDs(_ context.Context, ids []vo.Id) ([]*model.RecurringTransaction, error) {
	in := idSet(ids)
	var out []*model.RecurringTransaction
	for _, rt := range r.templates {
		if in[rt.AccountID.String()] || (rt.AccountRecipID != nil && in[rt.AccountRecipID.String()]) {
			out = append(out, rt)
		}
	}
	return out, nil
}

func (r *recurringSourceStub) CanEdit(context.Context, vo.Id, vo.Id) (bool, error) { return true, nil }

type accountMapStub map[string]model.AccountView

func (a accountMapStub) AccountsByIDs(_ context.Context, ids []vo.Id) ([]model.AccountView, error) {
	out := make([]model.AccountView, 0, len(ids))
	for _, id := range ids {
		out = append(out, a[id.String()])
	}
	return out, nil
}

func (a accountMapStub) AccountOwner(_ context.Context, id vo.Id) (vo.Id, error) {
	return vo.ParseId(a[id.String()].OwnerID)
}

func (a accountMapStub) OwnedLiveAccountIDs(context.Context, vo.Id) ([]vo.Id, error) {
	return nil, nil
}
func (a accountMapStub) OwnedAccountIDs(context.Context, vo.Id) ([]vo.Id, error) {
	return nil, nil
}

// rateConvertor multiplies every cross-currency item by a fixed rate.
type rateConvertor struct {
	rate  string
	calls int
}

func (c *rateConvertor) BulkConvert(_ context.Context, _, _ time.Time, items map[string][]model.ConvertItem) (map[string]vo.DecimalNumber, error) {
	c.calls++
	out := map[string]vo.DecimalNumber{}
	for key, list := range items {
		total := vo.NewDecimal("0")
		for _, it := range list {
			amt := it.Amount
			if !it.From.Equal(it.To) {
				amt = amt.Mul(vo.NewDecimal(c.rate))
			}
			total = total.Add(amt)
		}
		out[key] = total
	}
	return out, nil
}

func prAccounts() accountMapStub {
	return accountMapStub{
		prEveryday: {ID: prEveryday, CurrencyID: prUSD, OwnerID: prUser},
		prSavings:  {ID: prSavings, CurrencyID: prEUR, OwnerID: prUser},
		prOutside:  {ID: prOutside, CurrencyID: prUSD, OwnerID: prUser},
	}
}

var (
	prToday = time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	prNov   = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	prDec   = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	prJan   = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
)

func prTemplate(t *testing.T, typ model.TransactionType, account string, amount string, schedule model.RecurringSchedule) *model.RecurringTransaction {
	return &model.RecurringTransaction{
		ID: vo.NewId(), UserID: prID(t, prUser), Type: typ, AccountID: prID(t, account),
		Amount: amount, Description: "rent", Schedule: schedule,
		NextPaymentAt: prToday.Add(10 * time.Hour), ScheduledDay: 2,
	}
}

func prService(src RecurringSource, conv Convertor) *Service {
	s := &Service{
		accounts: prAccounts(), convertor: conv, read: &spendingStub{},
		clock: frozenClock{t: prToday.Add(9 * time.Hour)},
	}
	if src != nil {
		s.SetRecurring(src)
	}
	return s
}

func prRead(t *testing.T, s *Service, members ...string) *projectedRead {
	t.Helper()
	ids := make([]vo.Id, 0, len(members))
	for _, m := range members {
		ids = append(ids, prID(t, m))
	}
	p, err := s.newProjectedRead(context.Background(), filters{includedAccountIDs: ids}, prJan)
	if err != nil {
		t.Fatalf("newProjectedRead: %v", err)
	}
	return p
}

func prIDs(t *testing.T, s ...string) []vo.Id {
	out := make([]vo.Id, 0, len(s))
	for _, v := range s {
		out = append(out, prID(t, v))
	}
	return out
}

func derefOr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestProjectedRead_CountSpendingFilesLikeSQL(t *testing.T) {
	tagged := prTemplate(t, model.TransactionTypeExpense, prEveryday, "30", model.RecurringScheduleMonthly)
	tagged.CategoryID = prIDp(t, prCat)
	tagged.TagID = prIDp(t, prTag)
	bare := prTemplate(t, model.TransactionTypeExpense, prEveryday, "7", model.RecurringScheduleMonthly)
	p := prRead(t, prService(&recurringSourceStub{templates: []*model.RecurringTransaction{tagged, bare}}, nil), prEveryday)

	rows, err := p.CountSpending(context.Background(), prIDs(t, prCat), prIDs(t, prEveryday), prNov, prDec)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[derefOr(r.CategoryID)+"|"+derefOr(r.TagID)+"|"+r.CurrencyID] = vo.NewDecimal(r.Amount).String()
	}
	want := map[string]string{
		prCat + "|" + prTag + "|" + prUSD: "30",
		"<nil>|<nil>|" + prUSD:            "7",
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("row %s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}

	// A category outside the set drops the row; NULL-category rows stay.
	rows, err = p.CountSpending(context.Background(), nil, prIDs(t, prEveryday), prNov, prDec)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].CategoryID != nil {
		t.Errorf("without the category in the set want only the uncategorized row, got %+v", rows)
	}
}

func TestProjectedRead_SpendingByMonthBuckets(t *testing.T) {
	weekly := prTemplate(t, model.TransactionTypeExpense, prEveryday, "10", model.RecurringScheduleWeekly)
	p := prRead(t, prService(&recurringSourceStub{templates: []*model.RecurringTransaction{weekly}}, nil), prEveryday)

	rows, err := p.SpendingByMonth(context.Background(), nil, prIDs(t, prEveryday), prNov, prJan)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.Month] = vo.NewDecimal(r.Amount).String()
	}
	// Mondays: Nov 2, 9, 16, 23, 30 and Dec 7, 14, 21, 28.
	if len(got) != 2 || got["2026-11-01"] != "50" || got["2026-12-01"] != "40" {
		t.Errorf("months = %v, want 2026-11-01=50 and 2026-12-01=40", got)
	}
}

func TestProjectedRead_TransfersBoundary(t *testing.T) {
	out := prTemplate(t, model.TransactionTypeTransfer, prEveryday, "25", model.RecurringScheduleMonthly)
	out.AccountRecipID = prIDp(t, prOutside)
	inside := prTemplate(t, model.TransactionTypeTransfer, prEveryday, "40", model.RecurringScheduleMonthly)
	inside.AccountRecipID = prIDp(t, prSavings)
	p := prRead(t, prService(&recurringSourceStub{templates: []*model.RecurringTransaction{out, inside}}, &rateConvertor{rate: "1"}), prEveryday, prSavings)

	rows, err := p.TransfersByMonth(context.Background(), prIDs(t, prEveryday, prSavings), prNov, prDec)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want one boundary row, got %+v", rows)
	}
	r := rows[0]
	if r.Month != "2026-11-01" || r.CurrencyID != prUSD || vo.NewDecimal(r.Out).String() != "25" || vo.NewDecimal(r.In).String() != "0" {
		t.Errorf("row = %+v, want 2026-11-01 USD out 25 in 0", r)
	}

	list, err := p.BudgetTransactionsTransfers(context.Background(), prIDs(t, prEveryday, prSavings), prNov, prDec)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != out.ID.String() || list[0].Direction != "out" || list[0].UserID != prUser {
		t.Errorf("transfer list = %+v, want the outgoing template only", list)
	}
}

func TestProjectedRead_CrossCurrencyTransfer(t *testing.T) {
	tr := prTemplate(t, model.TransactionTypeTransfer, prEveryday, "100", model.RecurringScheduleMonthly)
	tr.AccountRecipID = prIDp(t, prSavings)
	conv := &rateConvertor{rate: "0.5"}
	p := prRead(t, prService(&recurringSourceStub{templates: []*model.RecurringTransaction{tr}}, conv), prEveryday, prSavings)
	if conv.calls != 1 {
		t.Errorf("BulkConvert calls = %d, want 1", conv.calls)
	}

	rows, err := p.AccountsNetByMonth(context.Background(), prIDs(t, prSavings), prNov, prJan)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.AccountID+"|"+r.Month] = vo.NewDecimal(r.Amount).String()
	}
	if len(got) != 2 || got[prSavings+"|2026-11-01"] != "50" || got[prSavings+"|2026-12-01"] != "50" {
		t.Errorf("savings net = %v, want +50 in Nov and Dec", got)
	}

	rows, err = p.AccountsNetByMonth(context.Background(), prIDs(t, prEveryday), prNov, prDec)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || vo.NewDecimal(rows[0].Amount).String() != "-100" {
		t.Errorf("everyday net = %+v, want -100", rows)
	}

	// Seen from the everyday side alone the transfer crosses into the
	// savings account: In is measured in the recipient's currency.
	trs, err := p.TransfersByMonth(context.Background(), prIDs(t, prSavings), prNov, prDec)
	if err != nil {
		t.Fatal(err)
	}
	if len(trs) != 1 || trs[0].CurrencyID != prEUR || vo.NewDecimal(trs[0].In).String() != "50" {
		t.Errorf("transfers into savings = %+v, want EUR in 50", trs)
	}
}

// A template on a deleted account, or a transfer into one, cannot be seen,
// posted or removed, so it projects nothing.
func TestProjectedRead_DeletedAccountExcluded(t *testing.T) {
	accounts := prAccounts()
	v := accounts[prSavings]
	v.IsDeleted = true
	accounts[prSavings] = v
	onDeleted := prTemplate(t, model.TransactionTypeExpense, prSavings, "12", model.RecurringScheduleMonthly)
	intoDeleted := prTemplate(t, model.TransactionTypeTransfer, prEveryday, "20", model.RecurringScheduleMonthly)
	intoDeleted.AccountRecipID = prIDp(t, prSavings)
	live := prTemplate(t, model.TransactionTypeExpense, prEveryday, "7", model.RecurringScheduleMonthly)
	s := prService(&recurringSourceStub{templates: []*model.RecurringTransaction{onDeleted, intoDeleted, live}}, &rateConvertor{rate: "1"})
	s.accounts = accounts
	p := prRead(t, s, prEveryday, prSavings)

	if len(p.occ) != 2 {
		t.Fatalf("occurrences = %d, want 2 (the live template in Nov and Dec)", len(p.occ))
	}
	for _, o := range p.occ {
		if o.templateID != live.ID.String() {
			t.Errorf("projected template %s, want only the live one", o.templateID)
		}
	}

	only := prService(&recurringSourceStub{templates: []*model.RecurringTransaction{onDeleted}}, nil)
	only.accounts = accounts
	if p := prRead(t, only, prSavings); p != nil {
		t.Errorf("read over a deleted account's template = %+v, want nil", p)
	}
}

func TestProjectedRead_LabelsAndListsMirrorSQL(t *testing.T) {
	rt := prTemplate(t, model.TransactionTypeExpense, prEveryday, "9", model.RecurringScheduleMonthly)
	rt.LabelIDs = []vo.Id{prID(t, prLabel)}
	rt.CategoryID = prIDp(t, prCat)
	overdue := prTemplate(t, model.TransactionTypeIncome, prEveryday, "100", model.RecurringScheduleMonthly)
	overdue.NextPaymentAt = time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	overdue.ScheduledDay = 25
	overdue.CategoryID = prIDp(t, prCat)
	p := prRead(t, prService(&recurringSourceStub{templates: []*model.RecurringTransaction{rt, overdue}}, nil), prEveryday)
	ctx := context.Background()
	acc := prIDs(t, prEveryday)

	labels, err := p.CountSpendingByLabel(ctx, acc, prNov, prDec)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || labels[0].LabelID != prLabel || derefOr(labels[0].CategoryID) != prCat || vo.NewDecimal(labels[0].Amount).String() != "9" {
		t.Errorf("label spending = %+v", labels)
	}

	byLabel, err := p.BudgetTransactionsByLabelAndCategory(ctx, prID(t, prLabel), prID(t, prCat), acc, prNov, prDec)
	if err != nil {
		t.Fatal(err)
	}
	if len(byLabel) != 1 || byLabel[0].ID != rt.ID.String() || byLabel[0].SpentAt != "2026-11-02 10:00:00" {
		t.Errorf("label list = %+v", byLabel)
	}

	// The overdue Oct 25 income is filed in the current month but keeps its
	// due date; the Oct 25 -> Nov 25 cadence continues after today.
	income, err := p.BudgetTransactionsIncome(ctx, prIDs(t, prCat), acc, prNov, prDec)
	if err != nil {
		t.Fatal(err)
	}
	if len(income) != 2 || income[0].SpentAt != "2026-11-25 00:00:00" || income[1].SpentAt != "2026-10-25 00:00:00" ||
		income[1].Direction != "in" || income[1].Type != "income" {
		t.Errorf("income list = %+v", income)
	}
	inc, err := p.IncomeByMonth(ctx, acc, prNov, prJan)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range inc {
		got[r.Month] = vo.NewDecimal(r.Amount).String()
	}
	if got["2026-11-01"] != "200" || got["2026-12-01"] != "100" || len(got) != 2 {
		t.Errorf("income by month = %v, want overdue + Nov 25 in Nov and Dec 25", got)
	}

	onAccount, err := p.BudgetTransactionsOnAccount(ctx, prID(t, prEveryday), prNov, prDec)
	if err != nil {
		t.Fatal(err)
	}
	if len(onAccount) != 3 || onAccount[0].Type != "income" || onAccount[1].ID != rt.ID.String() || onAccount[1].Direction != "out" || onAccount[1].Type != "expense" {
		t.Errorf("on-account list = %+v, want newest first", onAccount)
	}

	bal, err := p.AccountsBalancesBeforeDate(ctx, acc, prJan)
	if err != nil {
		t.Fatal(err)
	}
	// 3 income x 100 - 2 expense x 9.
	if len(bal) != 1 || vo.NewDecimal(bal[0].Balance).String() != "282" || bal[0].CurrencyID != prUSD {
		t.Errorf("balances = %+v, want 282 USD", bal)
	}
	if lim, _ := p.SummarizedLimits(ctx, vo.NewId(), prNov, prDec); lim != nil {
		t.Errorf("SummarizedLimits = %+v, want nil", lim)
	}
}

// Nothing to project yields a nil read, so the callers skip the second
// builder pass.
func TestProjectedRead_NothingProjectedIsNil(t *testing.T) {
	if p := prRead(t, prService(nil, nil), prEveryday); p != nil {
		t.Errorf("no source: read = %+v, want nil", p)
	}
	if p := prRead(t, prService(&recurringSourceStub{}, nil), prEveryday); p != nil {
		t.Errorf("no templates: read = %+v, want nil", p)
	}
	ended := prTemplate(t, model.TransactionTypeExpense, prEveryday, "5", model.RecurringScheduleMonthly)
	ended.NextPaymentAt = prJan.AddDate(0, 1, 0)
	if p := prRead(t, prService(&recurringSourceStub{templates: []*model.RecurringTransaction{ended}}, nil), prEveryday); p != nil {
		t.Errorf("no occurrence in the window: read = %+v, want nil", p)
	}
}

func TestProjectedRead_NoOccurrencesIsEmpty(t *testing.T) {
	p := &projectedRead{ReadModel: &spendingStub{}}
	ctx := context.Background()
	acc := prIDs(t, prEveryday)
	check := func(name string, n int, err error) {
		t.Helper()
		if err != nil || n != 0 {
			t.Errorf("%s: %d rows, err %v; want none", name, n, err)
		}
	}
	r1, err := p.CountSpending(ctx, nil, acc, prNov, prJan)
	check("CountSpending", len(r1), err)
	r2, err := p.CountSpendingByLabel(ctx, acc, prNov, prJan)
	check("CountSpendingByLabel", len(r2), err)
	r3, err := p.SpendingByMonth(ctx, nil, acc, prNov, prJan)
	check("SpendingByMonth", len(r3), err)
	r4, err := p.IncomeByMonth(ctx, acc, prNov, prJan)
	check("IncomeByMonth", len(r4), err)
	r5, err := p.TransfersByMonth(ctx, acc, prNov, prJan)
	check("TransfersByMonth", len(r5), err)
	r6, err := p.AccountsNetByMonth(ctx, acc, prNov, prJan)
	check("AccountsNetByMonth", len(r6), err)
	r7, err := p.AccountsIncomeExpenseByMonth(ctx, acc, prNov, prJan)
	check("AccountsIncomeExpenseByMonth", len(r7), err)
	r8, err := p.BudgetTransactionsUncategorized(ctx, acc, prNov, prJan)
	check("BudgetTransactionsUncategorized", len(r8), err)
	r9, err := p.BudgetTransactionsOnAccount(ctx, acc[0], prNov, prJan)
	check("BudgetTransactionsOnAccount", len(r9), err)
	r10, err := p.AccountsReport(ctx, acc, prNov, prJan)
	check("AccountsReport", len(r10), err)
}

// upcomingRead gates the second builder pass: a current window with no
// templates and no future-dated transactions, like a past one, returns no read.
func TestUpcomingRead_NilWhenNothingUpcoming(t *testing.T) {
	ctx := context.Background()
	now := prToday.Add(9 * time.Hour)
	current := filters{includedAccountIDs: prIDs(t, prEveryday), periodStart: prNov, periodEnd: prDec}
	if p, err := prService(&recurringSourceStub{}, nil).upcomingRead(ctx, current, now); err != nil || p != nil {
		t.Errorf("no templates: read = %+v, err %v; want nil", p, err)
	}
	rt := prTemplate(t, model.TransactionTypeExpense, prEveryday, "9", model.RecurringScheduleMonthly)
	s := prService(&recurringSourceStub{templates: []*model.RecurringTransaction{rt}}, nil)
	if p, err := s.upcomingRead(ctx, current, now); err != nil || p == nil || len(p.(*projectedRead).occ) != 1 {
		t.Errorf("one template: read = %+v, err %v; want its November occurrence", p, err)
	}
	past := filters{includedAccountIDs: current.includedAccountIDs, periodStart: prNov.AddDate(0, -1, 0), periodEnd: prNov}
	if p, err := s.upcomingRead(ctx, past, now); err != nil || p != nil {
		t.Errorf("past window: read = %+v, err %v; want nil", p, err)
	}
}
