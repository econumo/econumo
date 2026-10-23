package budget_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appbudget "github.com/econumo/econumo/internal/budget"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	domcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

type flowClock struct{ t time.Time }

func (c flowClock) Now() time.Time { return c.t }

type flowHarness struct {
	ctx      context.Context
	svc      *appbudget.Service
	f        *fixture.Builder
	user     string
	acct     string
	accounts *accountrepo.Repo
	tdb      *dbtest.DB
}

func newFlowHarness(t *testing.T) *flowHarness {
	t.Helper()
	tdb := dbtest.New(t)
	txm, eng := tdb.TX, tdb.Engine
	f := fixture.New(t, tdb)
	user := f.User(fixture.User{Name: "Me", Email: "me@flow.test"})
	f.DefaultOptions(user)
	acct := f.Account(fixture.Account{UserID: user, Name: "Main"})

	userRepo := userrepo.NewRepo(eng, txm)
	accountRepo := accountrepo.NewRepo(eng, txm)
	categoryRepo := categoryrepo.NewRepo(eng, txm)
	tagRepo := tagrepo.NewRepo(eng, txm)
	payeeRepo := payeerepo.NewRepo(eng, txm)
	currencyLookup := currencyrepo.New(eng, txm)
	rateProvider := currencyrepo.NewRateProvider(eng, txm, currencyLookup, fixture.USD)
	clk := flowClock{t: time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)}
	svc := appbudget.NewService(
		budgetrepo.NewRepo(eng, txm), budgetrepo.NewReadRepo(eng, txm),
		domcurrency.NewConvertor(rateProvider), rateProvider,
		server.NewBudgetUserLookup(userRepo, clk),
		server.NewBudgetAccountLookup(accountRepo),
		server.NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(server.NewBudgetCategoryMetadataLookup(categoryRepo), server.NewBudgetTagMetadataLookup(tagRepo), server.NewBudgetPayeeMetadataLookup(payeeRepo)),
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(eng, txm)),
		operationrepo.NewGuard(eng, txm),
		txm, clk,
	)
	return &flowHarness{ctx: context.Background(), svc: svc, f: f, user: user, acct: acct, accounts: accountRepo, tdb: tdb}
}

func (h *flowHarness) report(t *testing.T, req model.GetFlowReportRequest) model.FlowReportResult {
	t.Helper()
	res, err := h.svc.GetFlowReport(h.ctx, vo.MustParseId(h.user), req)
	if err != nil {
		t.Fatal(err)
	}
	return res.Item
}

func (h *flowHarness) tx(typ int, acct, cat, payee, tag, amount, at string) {
	h.f.Transaction(fixture.Transaction{UserID: h.user, AccountID: acct, CategoryID: cat, PayeeID: payee, TagID: tag, Type: typ, Amount: amount, SpentAt: at})
}

func rowByName(t *testing.T, rows []model.ReportRow, name string) model.ReportRow {
	t.Helper()
	for _, r := range rows {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no row %q in %+v", name, rows)
	return model.ReportRow{}
}

func eqAmounts(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("amounts = %v, want %v", got, want)
	}
	for i := range want {
		if !vo.NewDecimal(got[i]).Equals(vo.NewDecimal(want[i])) {
			t.Fatalf("amounts = %v, want %v", got, want)
		}
	}
}

func TestFlowReport_Personal_ByCategory(t *testing.T) {
	h := newFlowHarness(t)
	food := h.f.Category(fixture.Category{UserID: h.user, Name: "Food", Type: 0, Icon: "restaurant"})
	salary := h.f.Category(fixture.Category{UserID: h.user, Name: "Salary", Type: 1})
	h.tx(0, h.acct, food, "", "", "10", "2026-03-02 10:00:00")
	h.tx(0, h.acct, food, "", "", "5", "2026-03-20 10:00:00")
	h.tx(0, h.acct, food, "", "", "7", "2026-04-02 10:00:00")
	h.tx(0, h.acct, "", "", "", "3", "2026-04-03 10:00:00")
	h.tx(1, h.acct, salary, "", "", "100", "2026-04-05 10:00:00")

	r := h.report(t, model.GetFlowReportRequest{From: "2026-03-01", To: "2026-04-30"})
	if r.CurrencyId != fixture.USD {
		t.Fatalf("currency = %s", r.CurrencyId)
	}
	if strings.Join(r.Months, ",") != "2026-03-01,2026-04-01" {
		t.Fatalf("months = %v", r.Months)
	}
	if len(r.Expense) != 2 || r.Expense[0].Name != "Food" {
		t.Fatalf("expense = %+v", r.Expense)
	}
	food0 := r.Expense[0]
	if food0.Kind != "category" || food0.Icon != "restaurant" || food0.Id != food {
		t.Fatalf("food row = %+v", food0)
	}
	eqAmounts(t, food0.Amounts, "15", "7")
	unc := r.Expense[1]
	if unc.Id != model.UncategorizedID || unc.Name != model.UncategorizedName {
		t.Fatalf("uncategorized row = %+v", unc)
	}
	eqAmounts(t, unc.Amounts, "0", "3")
	if len(r.Income) != 1 || r.Income[0].Kind != "income_category" {
		t.Fatalf("income = %+v", r.Income)
	}
	eqAmounts(t, r.Income[0].Amounts, "0", "100")
	if len(food0.Planned) != 0 || len(food0.Children) != 0 {
		t.Fatalf("planned/children must be empty: %+v", food0)
	}
}

func TestFlowReport_Personal_ByPayee(t *testing.T) {
	h := newFlowHarness(t)
	shop := h.f.Payee(fixture.Payee{UserID: h.user, Name: "Shop"})
	h.tx(0, h.acct, "", shop, "", "12", "2026-05-02 10:00:00")
	h.tx(0, h.acct, "", "", "", "1", "2026-05-03 10:00:00")
	r := h.report(t, model.GetFlowReportRequest{GroupBy: "payee"})
	if len(r.Months) != 1 || r.Months[0] != "2026-05-01" {
		t.Fatalf("months = %v", r.Months)
	}
	row := rowByName(t, r.Expense, "Shop")
	if row.Kind != "payee" || row.Id != shop {
		t.Fatalf("row = %+v", row)
	}
	eqAmounts(t, row.Amounts, "12")
	if rowByName(t, r.Expense, model.UncategorizedName).Id != model.UncategorizedID {
		t.Fatal("no-payee row missing")
	}
}

func TestFlowReport_Personal_ByTag(t *testing.T) {
	h := newFlowHarness(t)
	trip := h.f.Tag(fixture.Tag{UserID: h.user, Name: "Trip"})
	h.tx(0, h.acct, "", "", trip, "20", "2026-05-02 10:00:00")
	r := h.report(t, model.GetFlowReportRequest{GroupBy: "tag"})
	row := rowByName(t, r.Expense, "Trip")
	if row.Kind != "tag" || row.Id != trip {
		t.Fatalf("row = %+v", row)
	}
	eqAmounts(t, row.Amounts, "20")
}

func TestFlowReport_Personal_TransfersBetweenOwnIgnored(t *testing.T) {
	h := newFlowHarness(t)
	own2 := h.f.Account(fixture.Account{UserID: h.user, Name: "Second"})
	partner := h.f.User(fixture.User{ID: "ffff0000-0000-7000-8000-0000000000aa", Name: "Partner", Email: "partner@flow.test"})
	h.f.DefaultOptions(partner)
	theirs := h.f.Account(fixture.Account{UserID: partner, Name: "Theirs"})
	h.f.Transaction(fixture.Transaction{UserID: h.user, AccountID: h.acct, AccountRecipientID: own2, Type: 2, Amount: "50", AmountRecipient: "50", SpentAt: "2026-05-02 10:00:00"})
	h.f.Transaction(fixture.Transaction{UserID: h.user, AccountID: h.acct, AccountRecipientID: theirs, Type: 2, Amount: "30", AmountRecipient: "30", SpentAt: "2026-05-03 10:00:00"})
	r := h.report(t, model.GetFlowReportRequest{})
	eqAmounts(t, r.TransfersOut, "30")
	eqAmounts(t, r.TransfersIn, "0")
}

func TestFlowReport_Personal_ForeignAccountConverted(t *testing.T) {
	h := newFlowHarness(t)
	eur := h.f.Currency(fixture.Currency{Code: "EUR", Symbol: "E"})
	h.f.Rate(fixture.Rate{CurrencyID: eur, BaseCurrencyID: fixture.USD, Rate: "0.5", PublishedAt: "2026-05-01"})
	eurAcct := h.f.Account(fixture.Account{UserID: h.user, CurrencyID: eur, Name: "Euro"})
	cat := h.f.Category(fixture.Category{UserID: h.user, Name: "Food", Type: 0})
	h.tx(0, eurAcct, cat, "", "", "10", "2026-05-02 10:00:00")
	h.tx(0, h.acct, cat, "", "", "4", "2026-05-03 10:00:00")
	r := h.report(t, model.GetFlowReportRequest{})
	eqAmounts(t, rowByName(t, r.Expense, "Food").Amounts, "24")
}

func TestFlowReport_Personal_DeletedAccountKeepsHistory(t *testing.T) {
	h := newFlowHarness(t)
	gone := h.f.Account(fixture.Account{UserID: h.user, Name: "Gone", Deleted: true})
	cat := h.f.Category(fixture.Category{UserID: h.user, Name: "Food", Type: 0})
	h.tx(0, gone, cat, "", "", "9", "2026-05-02 10:00:00")
	r := h.report(t, model.GetFlowReportRequest{})
	eqAmounts(t, rowByName(t, r.Expense, "Food").Amounts, "9")
}

func TestFlowReport_NoTransactions(t *testing.T) {
	h := newFlowHarness(t)
	r := h.report(t, model.GetFlowReportRequest{})
	if len(r.Months) != 1 || r.Months[0] != "2026-05-01" {
		t.Fatalf("months = %v", r.Months)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"expense", "income", "savings"} {
		if string(m[k]) != "[]" {
			t.Fatalf("%s = %s, want []", k, m[k])
		}
	}
	for _, k := range []string{"transfersIn", "transfersOut"} {
		if string(m[k]) != `["0"]` && string(m[k]) != `["0.00000000"]` {
			t.Fatalf("%s = %s, want one zero", k, m[k])
		}
	}
}

func TestFlowReport_InvalidGroupBy(t *testing.T) {
	h := newFlowHarness(t)
	_, err := h.svc.GetFlowReport(h.ctx, vo.MustParseId(h.user), model.GetFlowReportRequest{GroupBy: "bogus"})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestFlowReport_Personal_DeletionCorrectionCounted(t *testing.T) {
	h := newFlowHarness(t)
	gone := h.f.Account(fixture.Account{UserID: h.user, Name: "Gone", Deleted: true})
	h.tx(1, gone, "", "", "", "50", "2026-04-02 10:00:00")
	err := h.accounts.SaveCorrection(h.ctx, model.AccountCorrection{
		ID: vo.NewId(), UserID: vo.MustParseId(h.user), AccountID: vo.MustParseId(gone),
		Description: "Balance adjustment (account deleted)", Type: 0, Amount: "50",
		SpentAt: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := h.report(t, model.GetFlowReportRequest{From: "2026-04-01", To: "2026-05-01"})
	unc := rowByName(t, r.Expense, model.UncategorizedName)
	if unc.Id != model.UncategorizedID {
		t.Fatalf("row = %+v", unc)
	}
	eqAmounts(t, unc.Amounts, "0", "50")
}

func TestFlowReport_Personal_PartnerCategoryOnSharedAccount(t *testing.T) {
	h := newFlowHarness(t)
	partner := h.f.User(fixture.User{ID: "ffff0000-0000-7000-8000-0000000000aa", Name: "Partner", Email: "partner@flow.test"})
	h.f.DefaultOptions(partner)
	theirCat := h.f.Category(fixture.Category{UserID: partner, Name: "PartnerCat", Type: 0, Icon: "pets"})
	theirTag := h.f.Tag(fixture.Tag{UserID: partner, Name: "PartnerTag"})
	h.f.AccountAccess(h.acct, partner, 1)
	h.f.Transaction(fixture.Transaction{UserID: partner, AccountID: h.acct, CategoryID: theirCat, TagID: theirTag, Type: 0, Amount: "8", SpentAt: "2026-05-02 10:00:00"})

	r := h.report(t, model.GetFlowReportRequest{})
	row := rowByName(t, r.Expense, "PartnerCat")
	if row.Id != theirCat || row.Icon != "pets" {
		t.Fatalf("row = %+v", row)
	}
	eqAmounts(t, row.Amounts, "8")
	for _, x := range r.Expense {
		if x.Id == model.UncategorizedID {
			t.Fatalf("partner category folded into Uncategorized: %+v", r.Expense)
		}
	}
	rt := h.report(t, model.GetFlowReportRequest{GroupBy: "tag"})
	if rowByName(t, rt.Expense, "PartnerTag").Id != theirTag {
		t.Fatalf("tag rows = %+v", rt.Expense)
	}
}
