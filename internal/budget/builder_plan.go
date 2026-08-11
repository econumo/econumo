package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// BuildBudgetPlan assembles the plan sheet: window months, opening balances,
// per-month rates, and the row structure. Unlike BuildBudget it never walks
// month-by-month over transactions — the by-month repo queries return the
// whole window in one pass each (the endpoint stays O(1) queries in the
// window size; only the rate lookups are per-month, and those read the small
// rates table).
func (s *Service) BuildBudgetPlan(ctx context.Context, userID vo.Id, b *budgetAggregate, from time.Time, months int) (model.BudgetPlanResult, error) {
	windowEnd := from.AddDate(0, months, 0)

	meta, err := s.buildMeta(ctx, b)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}
	f, err := s.buildFilters(ctx, userID, b, from, windowEnd)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}

	monthsList := make([]time.Time, months)
	monthStrs := make([]string, months)
	for i := range monthsList {
		m := from.AddDate(0, i, 0)
		monthsList[i] = m
		monthStrs[i] = m.Format(datetime.DateLayout)
	}

	opening, err := s.buildOpeningBalances(ctx, b.budget.CurrencyID, f, from)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}

	rates := make([]model.PlanMonthRatesResult, 0, months)
	for i, m := range monthsList {
		monthRates, rerr := s.buildAverageRates(ctx, m, m.AddDate(0, 1, 0))
		if rerr != nil {
			return model.BudgetPlanResult{}, rerr
		}
		rates = append(rates, model.PlanMonthRatesResult{Period: monthStrs[i], Rates: monthRates})
	}

	structure, err := s.buildPlanStructure(ctx, b, f, monthsList)
	if err != nil {
		return model.BudgetPlanResult{}, err
	}

	return model.BudgetPlanResult{
		Meta:            meta,
		Months:          monthStrs,
		OpeningBalances: opening,
		CurrencyRates:   rates,
		Structure:       structure,
	}, nil
}

// buildOpeningBalances sums per-currency balances of the included accounts as
// of the window start, ordered budget currency first then discovery order —
// the same per-currency ordering rule as the budget page's balances block.
func (s *Service) buildOpeningBalances(ctx context.Context, budgetCurrencyID vo.Id, f filters, from time.Time) ([]model.OpeningBalanceResult, error) {
	rows, err := s.read.AccountsBalancesOnDate(ctx, f.includedAccountIDs, from)
	if err != nil {
		return nil, err
	}
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
	out := make([]model.OpeningBalanceResult, 0, len(ordered))
	for _, cid := range ordered {
		out = append(out, model.OpeningBalanceResult{
			CurrencyId: cid.String(),
			Amount:     sumBalances(rows, cid.String()).String(),
		})
	}
	return out, nil
}

// buildPlanStructure emits all folders plus every plan row. Tasks 3-4 fill the
// element walk; the skeleton returns folders only.
func (s *Service) buildPlanStructure(ctx context.Context, b *budgetAggregate, f filters, monthsList []time.Time) (model.PlanStructureResult, error) {
	sorted := append([]*model.BudgetFolder(nil), b.folders...)
	sortBudgetFolders(sorted)
	folders := make([]model.BudgetFolderResult, 0, len(sorted))
	for i, fl := range sorted {
		folders = append(folders, model.BudgetFolderResult{Id: fl.ID.String(), Name: fl.Name, Position: i})
	}
	return model.PlanStructureResult{Folders: folders, Elements: []model.PlanElementResult{}}, nil
}
