package budget

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// savingsRow is one savings account's in-progress row, shared by the monthly
// and plan builders.
type savingsRow struct {
	account    model.AccountView
	currencyID vo.Id // element currency
	sortKey    sortkey.Key
}

// savingsRows resolves each savings member's element currency (the element
// row's, else the account's own) and sort key. Rows derive from the CURRENT
// savings accounts, not from the element table, because sync is lazy: a stale
// element row whose account is no longer savings must not render.
func savingsRows(f filters, options map[string]elementOption) ([]savingsRow, error) {
	out := make([]savingsRow, 0, len(f.savingsAccounts))
	for _, a := range f.savingsAccounts {
		opt := options[elementKey(a.ID, model.ElementSavings)]
		cur := opt.currencyID
		if cur == nil {
			cid, err := vo.ParseId(a.CurrencyID)
			if err != nil {
				return nil, err
			}
			cur = &cid
		}
		out = append(out, savingsRow{account: a, currencyID: *cur, sortKey: opt.sortKey})
	}
	// Keyless rows (not synced yet) trail in membership order — the stable sort
	// keeps it — because the first sync keys them in that same order, so they
	// don't move once it runs.
	sort.SliceStable(out, func(i, j int) bool {
		ki, kj := out[i].sortKey, out[j].sortKey
		if ki == "" || kj == "" {
			return ki != "" && kj == ""
		}
		if ki != kj {
			return ki < kj
		}
		return out[i].account.ID < out[j].account.ID
	})
	return out, nil
}

func savingsSpentKey(accountID string) string { return "savings-spent_" + accountID }

func savingsAccountIDs(f filters) ([]vo.Id, error) {
	out := make([]vo.Id, 0, len(f.savingsAccounts))
	for _, a := range f.savingsAccounts {
		id, err := vo.ParseId(a.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// savingsConvertKey resolves one SavingsByMonth row to its toConvert bucket
// key and the [start,end) rate period that row's amount was earned in — the
// monthly builder always returns the same key/period (one period, the whole
// call), the plan builder returns a distinct key/period per window month, and
// signals "not this window" with ok=false (a month outside monthIdx).
type savingsConvertKey func(row model.SavingsMonthRow) (key string, start, end time.Time, ok bool)

// addSavings queues each savings row's actual amount(s) into the caller's
// single bulk conversion, account currency -> element currency, over
// [from,to). keyFor is what differs between the monthly and plan builders
// (see savingsConvertKey); everything else — resolving the rows, loading
// SavingsByMonth once, and building the ConvertItem — is shared.
func (s *Service) addSavings(ctx context.Context, f filters, options map[string]elementOption, from, to time.Time,
	keyFor savingsConvertKey, toConvert map[string][]model.ConvertItem) ([]savingsRow, map[string]bool, error) {
	hasActual := map[string]bool{}
	rows, err := savingsRows(f, options)
	if err != nil || len(rows) == 0 {
		return rows, hasActual, err
	}
	ids, err := savingsAccountIDs(f)
	if err != nil {
		return nil, nil, err
	}
	actual, err := s.read.SavingsByMonth(ctx, ids, f.everydayAccountIDs, from, to)
	if err != nil {
		return nil, nil, err
	}
	byID := map[string]savingsRow{}
	for _, r := range rows {
		byID[r.account.ID] = r
	}
	for _, a := range actual {
		r, ok := byID[a.AccountID]
		if !ok {
			continue
		}
		key, start, end, ok := keyFor(a)
		if !ok {
			continue
		}
		accountCur, perr := vo.ParseId(r.account.CurrencyID)
		if perr != nil {
			return nil, nil, perr
		}
		toConvert[key] = append(toConvert[key], model.ConvertItem{
			PeriodStart: start, PeriodEnd: end, From: accountCur, To: r.currencyID, Amount: vo.NewDecimal(a.Amount),
		})
		hasActual[a.AccountID] = true
	}
	return rows, hasActual, nil
}

// addMonthlySavings queues each savings row's actual for the period into the
// structure's single bulk conversion, account currency -> element currency.
func (s *Service) addMonthlySavings(ctx context.Context, f filters, options map[string]elementOption, toConvert map[string][]model.ConvertItem) ([]savingsRow, error) {
	rows, _, err := s.addSavings(ctx, f, options, f.periodStart, f.periodEnd,
		func(a model.SavingsMonthRow) (string, time.Time, time.Time, bool) {
			return savingsSpentKey(a.AccountID), f.periodStart, f.periodEnd, true
		}, toConvert)
	return rows, err
}

func emitMonthlySavings(rows []savingsRow, limits map[string]budgetedAmount, get func(string) vo.DecimalNumber) []model.SavingsElementResult {
	zero := vo.NewDecimal("0")
	out := []model.SavingsElementResult{}
	for _, r := range rows {
		budgeted := orZero(limits[elementKey(r.account.ID, model.ElementSavings)].budgeted, zero)
		spent := get(savingsSpentKey(r.account.ID))
		// A deleted account stays only while it still carries a plan or activity.
		if r.account.IsDeleted && budgeted.IsZero() && spent.IsZero() {
			continue
		}
		out = append(out, model.SavingsElementResult{
			Id: r.account.ID, Type: int(model.ElementSavings.Int16()), Name: r.account.Name, Icon: r.account.Icon,
			CurrencyId: r.currencyID.String(), OwnerUserId: r.account.OwnerID, IsArchived: boolToInt(r.account.IsDeleted),
			Position: len(out), Budgeted: budgeted.String(), Spent: spent.String(), Available: budgeted.Sub(spent).String(),
		})
	}
	return out
}

// addPlanSavings queues each savings row's per-month actual into the plan's
// single bulk conversion, account currency -> element currency, under the
// same planKey scheme as the elements. hasActual marks accounts with any
// activity in the window.
func (s *Service) addPlanSavings(ctx context.Context, f filters, options map[string]elementOption, monthsList []time.Time, monthIdx map[string]int, toConvert map[string][]model.ConvertItem) ([]savingsRow, map[string]bool, error) {
	windowEnd := monthsList[0].AddDate(0, len(monthsList), 0)
	return s.addSavings(ctx, f, options, monthsList[0], windowEnd,
		func(a model.SavingsMonthRow) (string, time.Time, time.Time, bool) {
			i, ok := monthIdx[a.Month]
			if !ok {
				return "", time.Time{}, time.Time{}, false
			}
			return planKey(i, elementKey(a.AccountID, model.ElementSavings)), monthsList[i], monthsList[i].AddDate(0, 1, 0), true
		}, toConvert)
}

// emitPlanSavings renders the plan's savings rows in savingsRows order. A
// deleted account stays only while it carries a plan or activity in the window.
func emitPlanSavings(rows []savingsRow, plannedFor func(string) []string, hasActual map[string]bool, get func(string) vo.DecimalNumber, nMonths int) []model.PlanSavingsElementResult {
	out := []model.PlanSavingsElementResult{}
	for _, r := range rows {
		index := elementKey(r.account.ID, model.ElementSavings)
		planned := plannedFor(index)
		hasPlan := false
		for _, p := range planned {
			if p != "" {
				hasPlan = true
			}
		}
		if r.account.IsDeleted && !hasPlan && !hasActual[r.account.ID] {
			continue
		}
		cells := make([]model.PlanCellResult, nMonths)
		for i := range cells {
			cells[i] = model.PlanCellResult{Actual: get(planKey(i, index)).String(), Planned: planned[i]}
		}
		out = append(out, model.PlanSavingsElementResult{
			Id: r.account.ID, Type: int(model.ElementSavings.Int16()), Name: r.account.Name, Icon: r.account.Icon,
			CurrencyId: r.currencyID.String(), OwnerUserId: r.account.OwnerID, IsArchived: boolToInt(r.account.IsDeleted),
			Position: len(out), Cells: cells,
		})
	}
	return out
}

// buildSavingsOpeningBalances is buildOpeningBalances over the savings
// accounts only (strictly before the window), per savings-account currency,
// budget currency first then discovery order.
func (s *Service) buildSavingsOpeningBalances(ctx context.Context, budgetCurrencyID vo.Id, f filters, from time.Time) ([]model.OpeningBalanceResult, error) {
	out := []model.OpeningBalanceResult{}
	ids, err := savingsAccountIDs(f)
	if err != nil || len(ids) == 0 {
		return out, err
	}
	rows, err := s.read.AccountsBalancesBeforeDate(ctx, ids, from)
	if err != nil {
		return nil, err
	}
	for _, cid := range savingsCurrencies(f, budgetCurrencyID) {
		out = append(out, model.OpeningBalanceResult{CurrencyId: cid, Amount: sumBalances(rows, cid).String()})
	}
	return out, nil
}

// savingsCurrencies lists the savings accounts' currencies once each: the
// budget currency first (only when a savings account holds it), then
// discovery order.
func savingsCurrencies(f filters, budgetCurrencyID vo.Id) []string {
	budgetCur := budgetCurrencyID.String()
	seen := map[string]bool{}
	var rest []string
	hasBudgetCur := false
	for _, a := range f.savingsAccounts {
		if seen[a.CurrencyID] {
			continue
		}
		seen[a.CurrencyID] = true
		if a.CurrencyID == budgetCur {
			hasBudgetCur = true
			continue
		}
		rest = append(rest, a.CurrencyID)
	}
	if hasBudgetCur {
		return append([]string{budgetCur}, rest...)
	}
	return rest
}

// buildSavingsFlows sums AccountsNetByMonth per (month, account currency),
// ordered by month, then budget currency first, then currency id. Months
// without activity have no entry.
func (s *Service) buildSavingsFlows(ctx context.Context, budgetCurrencyID vo.Id, f filters, from, to time.Time) ([]model.PlanSavingsFlowResult, error) {
	out := []model.PlanSavingsFlowResult{}
	ids, err := savingsAccountIDs(f)
	if err != nil || len(ids) == 0 {
		return out, err
	}
	rows, err := s.read.AccountsNetByMonth(ctx, ids, from, to)
	if err != nil {
		return nil, err
	}
	currencyOf := map[string]string{}
	for _, a := range f.savingsAccounts {
		currencyOf[a.ID] = a.CurrencyID
	}
	sums := map[[2]string]vo.DecimalNumber{}
	for _, r := range rows {
		k := [2]string{r.Month, currencyOf[r.AccountID]}
		acc, ok := sums[k]
		if !ok {
			acc = vo.NewDecimal("0")
		}
		sums[k] = acc.Add(vo.NewDecimal(r.Amount))
	}
	budgetCur := budgetCurrencyID.String()
	for k, v := range sums {
		out = append(out, model.PlanSavingsFlowResult{Month: k[0], CurrencyId: k[1], Amount: v.String()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Month != out[j].Month {
			return out[i].Month < out[j].Month
		}
		if (out[i].CurrencyId == budgetCur) != (out[j].CurrencyId == budgetCur) {
			return out[i].CurrencyId == budgetCur
		}
		return out[i].CurrencyId < out[j].CurrencyId
	})
	return out, nil
}
