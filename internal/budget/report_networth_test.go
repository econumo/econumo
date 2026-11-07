package budget_test

import (
	"context"
	"errors"
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
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

type netWorthClock struct{ t time.Time }

func (c netWorthClock) Now() time.Time { return c.t }

type netWorthHarness struct {
	ctx      context.Context
	svc      *appbudget.Service
	f        *fixture.Builder
	user     string
	acct     string
	accounts *accountrepo.Repo
	tdb      *dbtest.DB
}

func newNetWorthHarness(t *testing.T) *netWorthHarness {
	t.Helper()
	tdb := dbtest.New(t)
	txm, eng := tdb.TX, tdb.Engine
	f := fixture.New(t, tdb)
	user := f.User(fixture.User{Name: "Me", Email: "me@networth.test"})
	f.DefaultOptions(user)
	acct := f.Account(fixture.Account{UserID: user, Name: "Main"})

	userRepo := userrepo.NewRepo(eng, txm)
	accountRepo := accountrepo.NewRepo(eng, txm)
	categoryRepo := categoryrepo.NewRepo(eng, txm)
	tagRepo := tagrepo.NewRepo(eng, txm)
	payeeRepo := payeerepo.NewRepo(eng, txm)
	currencyLookup := currencyrepo.New(eng, txm)
	rateProvider := currencyrepo.NewRateProvider(eng, txm, currencyLookup, fixture.USD)
	clk := netWorthClock{t: time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)}
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
	return &netWorthHarness{ctx: context.Background(), svc: svc, f: f, user: user, acct: acct, accounts: accountRepo, tdb: tdb}
}

func (h *netWorthHarness) tx(typ int, acct, cat, payee, tag, amount, at string) {
	h.f.Transaction(fixture.Transaction{UserID: h.user, AccountID: acct, CategoryID: cat, PayeeID: payee, TagID: tag, Type: typ, Amount: amount, SpentAt: at})
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

func (h *netWorthHarness) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := h.tdb.Raw.ExecContext(h.ctx, h.tdb.Rebind(query), args...); err != nil {
		t.Fatal(err)
	}
}

func (h *netWorthHarness) budget(t *testing.T, startedAt string, accounts ...string) string {
	t.Helper()
	id := h.f.Budget(fixture.Budget{UserID: h.user, StartedAt: startedAt})
	for _, a := range accounts {
		h.f.BudgetAccount(id, a)
	}
	return id
}

func (h *netWorthHarness) netWorth(t *testing.T, req model.GetNetWorthReportRequest) model.NetWorthReportResult {
	t.Helper()
	res, err := h.svc.GetNetWorthReport(h.ctx, vo.MustParseId(h.user), req)
	if err != nil {
		t.Fatal(err)
	}
	return res.Item
}

func accountRow(t *testing.T, r model.NetWorthReportResult, id string) model.NetWorthAccountRow {
	t.Helper()
	for _, a := range r.Accounts {
		if a.Id == id {
			return a
		}
	}
	t.Fatalf("account %s missing in %+v", id, r.Accounts)
	return model.NetWorthAccountRow{}
}

func TestNetWorthReport_Personal_RunningBalance(t *testing.T) {
	h := newNetWorthHarness(t)
	h.tx(1, h.acct, "", "", "", "100", "2026-02-10 10:00:00")
	h.tx(1, h.acct, "", "", "", "50", "2026-03-05 10:00:00")
	h.tx(0, h.acct, "", "", "", "20", "2026-04-05 10:00:00")
	r := h.netWorth(t, model.GetNetWorthReportRequest{From: "2026-03-01", To: "2026-04-01"})
	if r.CurrencyId != fixture.USD || len(r.Months) != 2 || len(r.Accounts) != 1 {
		t.Fatalf("result = %+v", r)
	}
	eqAmounts(t, r.Accounts[0].Balances, "150", "130")
}

// Months before the first stored rate convert at the earliest rate, not at face
// value (40,000 RUB is not 40,000 CAD).
func TestNetWorthReport_BeforeFirstRateUsesEarliestRate(t *testing.T) {
	h := newNetWorthHarness(t)
	cad := h.f.Currency(fixture.Currency{Code: "CAD", Symbol: "CA$"})
	rub := h.f.Currency(fixture.Currency{Code: "RUB", Symbol: "R"})
	for _, d := range []string{"2026-03-05", "2026-04-05"} {
		h.f.Rate(fixture.Rate{CurrencyID: cad, BaseCurrencyID: fixture.USD, Rate: "1.4", PublishedAt: d})
		h.f.Rate(fixture.Rate{CurrencyID: rub, BaseCurrencyID: fixture.USD, Rate: "80", PublishedAt: d})
	}
	ra := h.f.Account(fixture.Account{UserID: h.user, CurrencyID: rub, Name: "Rub"})
	h.tx(1, ra, "", "", "", "40000", "2026-01-02 10:00:00")
	b := h.f.Budget(fixture.Budget{UserID: h.user, CurrencyID: cad})
	h.f.BudgetAccount(b, ra)
	r := h.netWorth(t, model.GetNetWorthReportRequest{BudgetId: b, From: "2026-01-01", To: "2026-04-01"})
	eqAmounts(t, accountRow(t, r, ra).Balances, "700", "700", "700", "700")
}

// Rate histories start and stop at different dates. A month with rates for other
// currencies only converts RUB at its latest earlier rate, else at its first.
func TestNetWorthReport_StaggeredRateHistories(t *testing.T) {
	h := newNetWorthHarness(t)
	cad := h.f.Currency(fixture.Currency{Code: "CAD", Symbol: "CA$"})
	rub := h.f.Currency(fixture.Currency{Code: "RUB", Symbol: "R"})
	for _, d := range []string{"2026-01-05", "2026-02-05", "2026-03-05", "2026-04-05"} {
		h.f.Rate(fixture.Rate{CurrencyID: cad, BaseCurrencyID: fixture.USD, Rate: "1.4", PublishedAt: d})
	}
	h.f.Rate(fixture.Rate{CurrencyID: rub, BaseCurrencyID: fixture.USD, Rate: "80", PublishedAt: "2026-02-05"})
	h.f.Rate(fixture.Rate{CurrencyID: rub, BaseCurrencyID: fixture.USD, Rate: "100", PublishedAt: "2026-03-05"})
	ra := h.f.Account(fixture.Account{UserID: h.user, CurrencyID: rub, Name: "Rub"})
	h.tx(1, ra, "", "", "", "40000", "2026-01-02 10:00:00")
	b := h.f.Budget(fixture.Budget{UserID: h.user, CurrencyID: cad})
	h.f.BudgetAccount(b, ra)
	r := h.netWorth(t, model.GetNetWorthReportRequest{BudgetId: b, From: "2026-01-01", To: "2026-04-01"})
	// January before RUB's first rate (80), April after its last (100)
	eqAmounts(t, accountRow(t, r, ra).Balances, "700", "700", "560", "560")
}

// The clock says May 17: a May 25 transaction has not happened in a June window.
func TestNetWorthReport_FutureWindowLeavesOutFutureMoney(t *testing.T) {
	h := newNetWorthHarness(t)
	h.tx(1, h.acct, "", "", "", "100", "2026-05-02 10:00:00")
	h.tx(1, h.acct, "", "", "", "30", "2026-05-25 10:00:00")
	r := h.netWorth(t, model.GetNetWorthReportRequest{From: "2026-06-01", To: "2026-06-01"})
	eqAmounts(t, r.Accounts[0].Balances, "100")
}

func TestNetWorthReport_TransferMovesBetweenAccounts(t *testing.T) {
	h := newNetWorthHarness(t)
	other := h.f.Account(fixture.Account{UserID: h.user, Name: "Other"})
	h.tx(1, h.acct, "", "", "", "100", "2026-03-02 10:00:00")
	h.f.Transaction(fixture.Transaction{UserID: h.user, AccountID: h.acct, AccountRecipientID: other, Type: 2, Amount: "40", AmountRecipient: "40", SpentAt: "2026-04-03 10:00:00"})
	r := h.netWorth(t, model.GetNetWorthReportRequest{From: "2026-03-01", To: "2026-04-01"})
	eqAmounts(t, accountRow(t, r, h.acct).Balances, "100", "60")
	eqAmounts(t, accountRow(t, r, other).Balances, "0", "40")
}

func TestNetWorthReport_ForeignCurrencyConverted(t *testing.T) {
	h := newNetWorthHarness(t)
	eur := h.f.Currency(fixture.Currency{Code: "EUR", Symbol: "E"})
	h.f.Rate(fixture.Rate{CurrencyID: eur, BaseCurrencyID: fixture.USD, Rate: "0.5", PublishedAt: "2026-03-01"})
	h.f.Rate(fixture.Rate{CurrencyID: eur, BaseCurrencyID: fixture.USD, Rate: "0.25", PublishedAt: "2026-04-01"})
	ea := h.f.Account(fixture.Account{UserID: h.user, CurrencyID: eur, Name: "Euro"})
	h.tx(1, ea, "", "", "", "10", "2026-03-02 10:00:00")
	r := h.netWorth(t, model.GetNetWorthReportRequest{From: "2026-03-01", To: "2026-04-01"})
	row := accountRow(t, r, ea)
	if row.CurrencyId != eur {
		t.Fatalf("currency = %s", row.CurrencyId)
	}
	eqAmounts(t, row.Balances, "20", "40")
}

func TestNetWorthReport_DeletedAccountsLeftOut(t *testing.T) {
	h := newNetWorthHarness(t)
	gone := h.f.Account(fixture.Account{UserID: h.user, Name: "Gone", Deleted: true})
	h.tx(1, gone, "", "", "", "50", "2016-04-02 10:00:00")
	err := h.accounts.SaveCorrection(h.ctx, model.AccountCorrection{
		ID: vo.NewId(), UserID: vo.MustParseId(h.user), AccountID: vo.MustParseId(gone),
		Description: "Balance adjustment (account deleted)", Type: 0, Amount: "50",
		SpentAt: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	h.tx(1, h.acct, "", "", "", "5", "2026-05-02 10:00:00")
	r := h.netWorth(t, model.GetNetWorthReportRequest{})
	if len(r.Accounts) != 1 || r.Accounts[0].Id != h.acct {
		t.Fatalf("accounts = %+v", r.Accounts)
	}
	// the all-time window opens before the live account's first transaction, not the deleted one's
	if r.Months[0] != "2026-04-01" {
		t.Fatalf("months = %v", r.Months)
	}
}

func TestNetWorthReport_ZeroAccountsOmitted(t *testing.T) {
	h := newNetWorthHarness(t)
	empty := h.f.Account(fixture.Account{UserID: h.user, Name: "Empty"})
	h.tx(1, h.acct, "", "", "", "5", "2026-05-02 10:00:00")
	r := h.netWorth(t, model.GetNetWorthReportRequest{})
	if len(r.Accounts) != 1 || r.Accounts[0].Id == empty {
		t.Fatalf("accounts = %+v", r.Accounts)
	}
}

// A partner's savings member is named and shared accounts stay as they are, but
// their other everyday accounts reach the caller only as one nameless sum.
func TestNetWorthReport_Budget_PartnerAccounts(t *testing.T) {
	h := newNetWorthHarness(t)
	partner := h.f.User(fixture.User{ID: "ffff0000-0000-7000-8000-0000000000cc", Name: "Partner", Email: "partner@networth.test"})
	h.f.DefaultOptions(partner)
	wallet := h.f.Account(fixture.Account{UserID: partner, Name: "Secret wallet"})
	card := h.f.Account(fixture.Account{UserID: partner, Name: "Secret card"})
	shared := h.f.Account(fixture.Account{UserID: partner, Name: "Shared cash"})
	saving := h.f.Account(fixture.Account{UserID: partner, Name: "Partner savings"})
	h.f.AccountAccess(shared, h.user, 1)
	b := h.budget(t, "2026-01-01 00:00:00", h.acct, wallet, card, shared, saving)
	h.exec(t, "UPDATE budgets_accounts SET is_savings = ? WHERE budget_id = ? AND account_id = ?", true, b, saving)
	h.f.BudgetAccess(b, partner, 0, true)
	h.tx(1, h.acct, "", "", "", "10", "2026-04-02 10:00:00")
	for acct, amount := range map[string]string{wallet: "70", card: "30", shared: "5", saving: "200"} {
		h.f.Transaction(fixture.Transaction{UserID: partner, AccountID: acct, Type: 1, Amount: amount, SpentAt: "2026-04-03 10:00:00"})
	}
	r := h.netWorth(t, model.GetNetWorthReportRequest{BudgetId: b, From: "2026-04-01", To: "2026-04-01"})
	if len(r.Accounts) != 4 || r.Accounts[0].Id != h.acct {
		t.Fatalf("accounts = %+v", r.Accounts)
	}
	eqAmounts(t, accountRow(t, r, shared).Balances, "5")
	if row := accountRow(t, r, saving); row.Name != "Partner savings" || row.IsSavings != 1 {
		t.Fatalf("savings row = %+v", row)
	}
	merged := accountRow(t, r, "everyday-"+partner)
	eqAmounts(t, merged.Balances, "100")
	if merged.Name != "" || merged.Icon != "" || merged.OwnerUserId != partner {
		t.Fatalf("merged row = %+v", merged)
	}
	for _, a := range r.Accounts {
		if a.Id == wallet || a.Id == card {
			t.Fatalf("a partner's own account leaked: %+v", a)
		}
	}
}

func TestNetWorthReport_Budget_NoAccess(t *testing.T) {
	h := newNetWorthHarness(t)
	other := h.f.User(fixture.User{ID: "ffff0000-0000-7000-8000-0000000000bb", Name: "Other", Email: "other@flow.test"})
	h.f.DefaultOptions(other)
	b := h.f.Budget(fixture.Budget{UserID: other})
	_, err := h.svc.GetNetWorthReport(h.ctx, vo.MustParseId(h.user), model.GetNetWorthReportRequest{BudgetId: b})
	var ad *errs.AccessDeniedError
	if !errors.As(err, &ad) {
		t.Fatalf("want access denied, got %v", err)
	}
}

func TestNetWorthReport_CurrentMonthStopsAtEndOfToday(t *testing.T) {
	h := newNetWorthHarness(t)
	h.tx(1, h.acct, "", "", "", "100", "2026-04-10 10:00:00")
	h.tx(1, h.acct, "", "", "", "30", "2026-05-17 23:00:00")
	h.tx(1, h.acct, "", "", "", "7", "2026-05-18 08:00:00")
	h.tx(1, h.acct, "", "", "", "500", "2026-05-25 10:00:00")
	r := h.netWorth(t, model.GetNetWorthReportRequest{From: "2026-04-01"})
	eqAmounts(t, r.Accounts[0].Balances, "100", "130")

	ahead := reqctx.WithLocation(h.ctx, time.FixedZone("UTC+14", 14*3600))
	res, err := h.svc.GetNetWorthReport(ahead, vo.MustParseId(h.user), model.GetNetWorthReportRequest{From: "2026-04-01"})
	if err != nil {
		t.Fatal(err)
	}
	eqAmounts(t, res.Item.Accounts[0].Balances, "100", "137")
}

func TestNetWorthReport_AllTimeOpensBeforeFirstMonth(t *testing.T) {
	h := newNetWorthHarness(t)
	h.tx(1, h.acct, "", "", "", "100", "2026-03-10 10:00:00")
	h.tx(1, h.acct, "", "", "", "20", "2026-04-10 10:00:00")
	r := h.netWorth(t, model.GetNetWorthReportRequest{})
	if len(r.Months) != 4 || r.Months[0] != "2026-02-01" || r.Months[3] != "2026-05-01" {
		t.Fatalf("months = %v, want 2026-02..2026-05", r.Months)
	}
	eqAmounts(t, r.Accounts[0].Balances, "0", "100", "120", "120")
}

func TestNetWorthReport_Budget_FlagsSavingsMembers(t *testing.T) {
	h := newNetWorthHarness(t)
	sav := h.f.Account(fixture.Account{UserID: h.user, Name: "Deposit"})
	b := h.budget(t, "2026-01-01 00:00:00", h.acct, sav)
	h.exec(t, "UPDATE budgets_accounts SET is_savings = ? WHERE budget_id = ? AND account_id = ?", true, b, sav)
	h.tx(1, h.acct, "", "", "", "10", "2026-04-02 10:00:00")
	h.tx(1, sav, "", "", "", "20", "2026-04-02 10:00:00")

	r := h.netWorth(t, model.GetNetWorthReportRequest{BudgetId: b, From: "2026-04-01", To: "2026-04-01"})
	flags := map[string]int{}
	for _, a := range r.Accounts {
		flags[a.Id] = a.IsSavings
	}
	if flags[sav] != 1 || flags[h.acct] != 0 {
		t.Fatalf("savings flags = %v", flags)
	}

	mine := h.netWorth(t, model.GetNetWorthReportRequest{From: "2026-04-01", To: "2026-04-01"})
	for _, a := range mine.Accounts {
		if a.IsSavings != 0 {
			t.Fatalf("my-accounts scope carries no savings flag, got %+v", a)
		}
	}
}
