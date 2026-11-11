// Reads the onboarding steps run against the user's budgets.
package budget

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DefaultBudget picks the budget onboarding works in: preferred (the user's
// active-budget option) when the user can read it, else the first budget they
// own or accepted. writable reports whether the user may change it (an editing
// role on an unarchived budget). found=false when the user has no such budget;
// a pending invitation does not count.
func (s *Service) DefaultBudget(ctx context.Context, userID vo.Id, preferred string) (id vo.Id, writable, found bool, err error) {
	budgets, err := s.budgets.ListForUser(ctx, userID)
	if err != nil {
		return vo.Id{}, false, false, err
	}
	var first *budgetAggregate
	for _, b := range budgets {
		agg, lerr := s.loadAggregate(ctx, b.ID)
		if lerr != nil {
			return vo.Id{}, false, false, lerr
		}
		if !s.canRead(agg, userID) {
			continue
		}
		if strings.EqualFold(agg.budget.ID.String(), strings.TrimSpace(preferred)) {
			first = agg
			break
		}
		if first == nil {
			first = agg
		}
	}
	if first == nil {
		return vo.Id{}, false, false, nil
	}
	return first.budget.ID, s.canUpdate(first, userID) && !first.budget.IsArchived, true, nil
}

// UnplannedOwnBudget reports the budget's name and currency; ok is set only
// when the caller owns it alone (no participant, invited or accepted), it is
// not archived and no element has a limit in any period. Onboarding follows the
// profile currency only in such a budget: switching it reshapes nobody's plan.
func (s *Service) UnplannedOwnBudget(ctx context.Context, userID, budgetID vo.Id) (name, currencyID string, ok bool, err error) {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return "", "", false, err
	}
	name, currencyID = b.budget.Name, b.budget.CurrencyID.String()
	if !b.budget.UserID.Equal(userID) || len(b.access) > 0 || b.budget.IsArchived {
		return name, currencyID, false, nil
	}
	limits, err := s.limits.ListLimitsFrom(ctx, budgetID, time.Unix(0, 0).UTC())
	if err != nil {
		return "", "", false, err
	}
	return name, currencyID, len(limits) == 0, nil
}

// CurrentPeriod is the first day of the budget's period holding today in the
// caller's timezone, never before the budget's first period. The caller must
// be able to update the budget, and it must not be archived.
func (s *Service) CurrentPeriod(ctx context.Context, userID, budgetID vo.Id) (time.Time, error) {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return time.Time{}, err
	}
	if !s.canUpdate(b, userID) {
		return time.Time{}, accessDenied()
	}
	if err := s.requireNotArchived(b); err != nil {
		return time.Time{}, err
	}
	return openingPeriod(b.budget, currentPeriod(s.clock.Now(), reqctx.Location(ctx), b.budget.PeriodStartDay)), nil
}

// CategoryHistory returns, for every category of the budget's participants,
// its income or spending in each of the `months` periods before the current one
// (the budget's current period in the caller's timezone, never before its first
// period), computed like the plan sheet's category rows but over every account
// the CALLER owns, archived and deleted ones included, rather than the budget's
// members: it predicts what the caller will spend, and onboarding's review may
// take the account holding all the history out of the budget. Transfers are
// excluded and amounts converted into the element's currency with each
// period's average rates. A tagged expense counts for its category. The caller
// must be able to update the budget.
func (s *Service) CategoryHistory(ctx context.Context, userID, budgetID vo.Id, months int) ([]model.BudgetCategoryHistory, error) {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	current := openingPeriod(b.budget, currentPeriod(s.clock.Now(), reqctx.Location(ctx), b.budget.PeriodStartDay))
	f, err := s.buildFilters(ctx, userID, b, current.AddDate(0, -months, 0), months)
	if err != nil {
		return nil, err
	}
	monthIdx := map[string]int{}
	windows := make([][2]time.Time, months)
	for i := range months {
		key := f.periodKey.AddDate(0, i, 0)
		monthIdx[key.Format(datetime.DateLayout)] = i
		windows[i][0], windows[i][1] = f.window(key)
	}

	items := map[string]*model.BudgetCategoryHistory{}
	var order []string
	options := s.elementOptions(b)
	itemFor := func(cat model.CategoryMeta) *model.BudgetCategoryHistory {
		typ := model.ElementCategory
		if cat.IsIncome {
			typ = model.ElementIncomeCategory
		}
		index := elementKey(cat.ID, typ)
		if it, ok := items[index]; ok {
			return it
		}
		currency := b.budget.CurrencyID
		if opt, ok := options[index]; ok && opt.currencyID != nil {
			currency = *opt.currencyID
		}
		it := &model.BudgetCategoryHistory{
			CategoryID: cat.ID, OwnerID: cat.OwnerID, IsIncome: cat.IsIncome, IsArchived: cat.IsArchived,
			CurrencyID: currency.String(), Amounts: make([]string, months),
		}
		items[index] = it
		order = append(order, index)
		return it
	}
	toConvert := map[string][]model.ConvertItem{}
	add := func(cat model.CategoryMeta, month, currencyID, amount string) error {
		i, ok := monthIdx[month]
		if !ok {
			return nil
		}
		from, err := vo.ParseId(currencyID)
		if err != nil {
			return err
		}
		it := itemFor(cat)
		key := fmt.Sprintf("%d_%s_%t", i, it.CategoryID, it.IsIncome)
		toConvert[key] = append(toConvert[key], model.ConvertItem{
			PeriodStart: windows[i][0], PeriodEnd: windows[i][1],
			From: from, To: vo.MustParseId(it.CurrencyID), Amount: vo.NewDecimal(amount),
		})
		return nil
	}

	expenseIDs := make([]vo.Id, 0, len(f.categories))
	for id := range f.categories {
		expenseIDs = append(expenseIDs, vo.MustParseId(id))
	}
	owned, err := s.accounts.OwnedAccountIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(owned) == 0 {
		return []model.BudgetCategoryHistory{}, nil
	}
	spending, err := s.read.SpendingByMonth(ctx, expenseIDs, owned, f.periodStart, f.periodEnd, f.startDay)
	if err != nil {
		return nil, err
	}
	for _, row := range spending {
		if row.CategoryID == nil {
			continue
		}
		if cat, ok := f.categories[*row.CategoryID]; ok {
			if err := add(cat, row.Month, row.CurrencyID, row.Amount); err != nil {
				return nil, err
			}
		}
	}
	income, err := s.read.IncomeByMonth(ctx, owned, f.periodStart, f.periodEnd, f.startDay)
	if err != nil {
		return nil, err
	}
	for _, row := range income {
		if row.CategoryID == nil {
			continue
		}
		if cat, ok := f.incomeCategories[*row.CategoryID]; ok {
			if err := add(cat, row.Month, row.CurrencyID, row.Amount); err != nil {
				return nil, err
			}
		}
	}
	if len(toConvert) == 0 {
		return []model.BudgetCategoryHistory{}, nil
	}
	// The top-level range doubles as month 0's rate range (see buildPlanStructure).
	converted, err := s.convertorFor(f.startDay).BulkConvert(ctx, windows[0][0], windows[0][1], toConvert)
	if err != nil {
		return nil, err
	}
	sort.Strings(order)
	out := make([]model.BudgetCategoryHistory, 0, len(order))
	for _, index := range order {
		it := items[index]
		for i := range months {
			key := fmt.Sprintf("%d_%s_%t", i, it.CategoryID, it.IsIncome)
			if _, ok := toConvert[key]; ok {
				it.Amounts[i] = convertedGetter(converted)(key).String()
			}
		}
		out = append(out, *it)
	}
	return out, nil
}
