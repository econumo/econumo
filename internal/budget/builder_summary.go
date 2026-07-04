package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// buildFinancialSummary returns per-currency balances (budget currency first) and
// the period average rates. Balance amount fields are nil when the period has not
// started/ended.
func (s *Service) buildFinancialSummary(ctx context.Context, budgetCurrencyID vo.Id, f filters, now time.Time) ([]model.CurrencyBalanceResult, []model.AverageCurrencyRateResult, error) {
	periodStarted := !f.periodStart.After(now)
	periodEnded := !f.periodEnd.After(now)

	var startBalances, endBalances []model.AccountBalanceRow
	var reports []model.AccountReportRow
	var holdings []model.HoldingsRow
	var err error

	if periodStarted {
		if startBalances, err = s.read.AccountsBalancesOnDate(ctx, f.includedAccountIDs, f.periodStart); err != nil {
			return nil, nil, err
		}
	}
	if periodEnded {
		if endBalances, err = s.read.AccountsBalancesBeforeDate(ctx, f.includedAccountIDs, f.periodEnd); err != nil {
			return nil, nil, err
		}
	}
	if periodStarted {
		if reports, err = s.read.AccountsReport(ctx, f.includedAccountIDs, f.periodStart, f.periodEnd); err != nil {
			return nil, nil, err
		}
		if holdings, err = s.read.HoldingsReport(ctx, f.includedAccountIDs, f.periodStart, f.periodEnd); err != nil {
			return nil, nil, err
		}
	}

	holdingsByCurrency := map[string]model.HoldingsRow{}
	for _, h := range holdings {
		holdingsByCurrency[h.CurrencyID] = h
	}

	// Order: budget currency first, then the rest in discovery order.
	ordered := make([]vo.Id, 0, len(f.currencyIDs))
	for _, c := range f.currencyIDs {
		if c.Equal(budgetCurrencyID) {
			ordered = append(ordered, c)
			break
		}
	}
	for _, c := range f.currencyIDs {
		if !c.Equal(budgetCurrencyID) {
			ordered = append(ordered, c)
		}
	}

	balances := make([]model.CurrencyBalanceResult, 0, len(ordered))
	for _, cid := range ordered {
		cidStr := cid.String()
		startBal := sumBalances(startBalances, cidStr)
		endBal := sumBalances(endBalances, cidStr)
		income := sumReport(reports, cidStr, func(r model.AccountReportRow) string { return r.Incomes })
		expenses := sumReport(reports, cidStr, func(r model.AccountReportRow) string { return r.Expenses })
		exchanges := sumReport(reports, cidStr, func(r model.AccountReportRow) string { return r.ExchangeIncomes }).
			Sub(sumReport(reports, cidStr, func(r model.AccountReportRow) string { return r.ExchangeExpenses }))
		hold := vo.NewDecimal("0")
		if h, ok := holdingsByCurrency[cidStr]; ok {
			hold = vo.NewDecimal(h.FromHoldings).Sub(vo.NewDecimal(h.ToHoldings))
		}

		item := model.CurrencyBalanceResult{CurrencyId: cidStr, Holdings: strPtr(hold.String())}
		if periodStarted {
			item.StartBalance = strPtr(startBal.String())
			item.Income = strPtr(income.String())
			item.Expenses = strPtr(expenses.String())
			item.Exchanges = strPtr(exchanges.String())
		}
		if periodEnded {
			item.EndBalance = strPtr(endBal.String())
		}
		balances = append(balances, item)
	}

	rates, err := s.buildAverageRates(ctx, f.periodStart, f.periodEnd)
	if err != nil {
		return nil, nil, err
	}
	return balances, rates, nil
}

// buildAverageRates returns all currency rates (no filter).
func (s *Service) buildAverageRates(ctx context.Context, periodStart, periodEnd time.Time) ([]model.AverageCurrencyRateResult, error) {
	base, err := s.rates.BaseCurrencyID(ctx)
	if err != nil {
		return nil, err
	}
	fullRates, err := s.rates.AverageRates(ctx, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	// The reported period is the SNAPPED one (latest-rate month), not the
	// requested period — the currencyRates block stamps this snapped range.
	rateStart, rateEnd, err := s.rates.SnappedRatePeriod(ctx, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	out := make([]model.AverageCurrencyRateResult, 0, len(fullRates))
	for _, r := range fullRates {
		out = append(out, model.AverageCurrencyRateResult{
			CurrencyId:     r.CurrencyID.String(),
			BaseCurrencyId: base.String(),
			Rate:           r.Rate.String(),
			PeriodStart:    rateStart.Format(datetime.DateLayout),
			PeriodEnd:      rateEnd.Format(datetime.DateLayout),
		})
	}
	return out, nil
}

func sumBalances(rows []model.AccountBalanceRow, currencyID string) vo.DecimalNumber {
	acc := vo.NewDecimal("0")
	for _, r := range rows {
		if r.CurrencyID == currencyID {
			acc = acc.Add(vo.NewDecimal(r.Balance))
		}
	}
	return acc
}

func sumReport(rows []model.AccountReportRow, currencyID string, field func(model.AccountReportRow) string) vo.DecimalNumber {
	acc := vo.NewDecimal("0")
	for _, r := range rows {
		if r.CurrencyID == currencyID {
			acc = acc.Add(vo.NewDecimal(field(r)))
		}
	}
	return acc
}

func strPtr(s string) *string { return &s }
