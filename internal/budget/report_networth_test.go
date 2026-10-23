package budget_test

import (
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (h *flowHarness) netWorth(t *testing.T, req model.GetNetWorthReportRequest) model.NetWorthReportResult {
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
	h := newFlowHarness(t)
	h.tx(1, h.acct, "", "", "", "100", "2026-02-10 10:00:00")
	h.tx(1, h.acct, "", "", "", "50", "2026-03-05 10:00:00")
	h.tx(0, h.acct, "", "", "", "20", "2026-04-05 10:00:00")
	r := h.netWorth(t, model.GetNetWorthReportRequest{From: "2026-03-01", To: "2026-04-01"})
	if r.CurrencyId != fixture.USD || len(r.Months) != 2 || len(r.Accounts) != 1 {
		t.Fatalf("result = %+v", r)
	}
	eqAmounts(t, r.Accounts[0].Balances, "150", "130")
}

func TestNetWorthReport_TransferMovesBetweenAccounts(t *testing.T) {
	h := newFlowHarness(t)
	other := h.f.Account(fixture.Account{UserID: h.user, Name: "Other"})
	h.tx(1, h.acct, "", "", "", "100", "2026-03-02 10:00:00")
	h.f.Transaction(fixture.Transaction{UserID: h.user, AccountID: h.acct, AccountRecipientID: other, Type: 2, Amount: "40", AmountRecipient: "40", SpentAt: "2026-04-03 10:00:00"})
	r := h.netWorth(t, model.GetNetWorthReportRequest{From: "2026-03-01", To: "2026-04-01"})
	eqAmounts(t, accountRow(t, r, h.acct).Balances, "100", "60")
	eqAmounts(t, accountRow(t, r, other).Balances, "0", "40")
}

func TestNetWorthReport_ForeignCurrencyConverted(t *testing.T) {
	h := newFlowHarness(t)
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

func TestNetWorthReport_DeletedAccountZeroAfterDeletion(t *testing.T) {
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
	r := h.netWorth(t, model.GetNetWorthReportRequest{From: "2026-04-01", To: "2026-05-01"})
	row := accountRow(t, r, gone)
	if row.IsDeleted != 1 {
		t.Fatalf("row = %+v", row)
	}
	eqAmounts(t, row.Balances, "50", "0")
}

func TestNetWorthReport_ZeroAccountsOmitted(t *testing.T) {
	h := newFlowHarness(t)
	empty := h.f.Account(fixture.Account{UserID: h.user, Name: "Empty"})
	h.tx(1, h.acct, "", "", "", "5", "2026-05-02 10:00:00")
	r := h.netWorth(t, model.GetNetWorthReportRequest{})
	if len(r.Accounts) != 1 || r.Accounts[0].Id == empty {
		t.Fatalf("accounts = %+v", r.Accounts)
	}
}

func TestNetWorthReport_Budget_IncludesPartnerAccounts(t *testing.T) {
	h := newFlowHarness(t)
	partner := h.f.User(fixture.User{ID: "ffff0000-0000-7000-8000-0000000000cc", Name: "Partner", Email: "partner@flow.test"})
	h.f.DefaultOptions(partner)
	pa := h.f.Account(fixture.Account{UserID: partner, Name: "Aardvark"})
	b := h.budget(t, "2026-01-01 00:00:00", h.acct, pa)
	h.f.BudgetAccess(b, partner, 0, true)
	h.tx(1, h.acct, "", "", "", "10", "2026-04-02 10:00:00")
	h.f.Transaction(fixture.Transaction{UserID: partner, AccountID: pa, Type: 1, Amount: "70", SpentAt: "2026-04-03 10:00:00"})
	r := h.netWorth(t, model.GetNetWorthReportRequest{BudgetId: b, From: "2026-04-01", To: "2026-04-01"})
	if len(r.Accounts) != 2 || r.Accounts[0].Id != h.acct || r.Accounts[1].Id != pa {
		t.Fatalf("accounts = %+v", r.Accounts)
	}
	eqAmounts(t, r.Accounts[1].Balances, "70")
	if r.Accounts[1].OwnerUserId != partner {
		t.Fatalf("owner = %s", r.Accounts[1].OwnerUserId)
	}
}

func TestNetWorthReport_Budget_NoAccess(t *testing.T) {
	h := newFlowHarness(t)
	other := h.f.User(fixture.User{ID: "ffff0000-0000-7000-8000-0000000000bb", Name: "Other", Email: "other@flow.test"})
	h.f.DefaultOptions(other)
	b := h.f.Budget(fixture.Budget{UserID: other})
	_, err := h.svc.GetNetWorthReport(h.ctx, vo.MustParseId(h.user), model.GetNetWorthReportRequest{BudgetId: b})
	var ad *errs.AccessDeniedError
	if !errors.As(err, &ad) {
		t.Fatalf("want access denied, got %v", err)
	}
}
