package budget

import (
	"context"
	"fmt"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// budgetedAmount is one element's current+prior budgeted totals.
type budgetedAmount struct {
	budgeted       vo.DecimalNumber
	budgetedBefore vo.DecimalNumber
	// hasLimit: a limit row exists for the current month. A plan of 0 is a
	// plan; only a missing row means "no plan".
	hasLimit bool
}

// amountSpent is one currency's spent amount within a period.
type amountSpent struct {
	currencyID             vo.Id
	amount                 vo.DecimalNumber
	periodStart, periodEnd time.Time
}

// buildElementsLimits computes per-element limits: budgeted = the element's limit
// for the selected period; budgetedBefore = sum of its limits for prior periods
// (budget.startedAt .. periodKey). Keyed "<externalId>-<typeAlias>".
func (s *Service) buildElementsLimits(ctx context.Context, b *budgetAggregate, f filters) (map[string]budgetedAmount, error) {
	data := map[string]budgetedAmount{}

	// The selected period's limits.
	current, err := s.limits.ListLimitsForPeriod(ctx, b.budget.ID, f.periodKey)
	if err != nil {
		return nil, err
	}
	// Map element id -> (externalId, typeAlias) for the limit rows.
	elemByID := map[string]*model.BudgetElement{}
	for _, e := range b.elements {
		elemByID[e.ID.String()] = e
	}
	for _, l := range current {
		e := elemByID[l.ElementID.String()]
		if e == nil {
			continue
		}
		key := elementKey(e.ExternalID.String(), e.Type)
		amt := data[key]
		if amt.budgeted.String() == "" {
			amt.budgeted = vo.NewDecimal("0")
		}
		if amt.budgetedBefore.String() == "" {
			amt.budgetedBefore = vo.NewDecimal("0")
		}
		amt.budgeted = l.Amount
		amt.hasLimit = true
		data[key] = amt
	}

	// Prior periods' summed limits (budgetedBefore).
	summed, err := s.read.SummarizedLimits(ctx, b.budget.ID, carryOverStart(b, f.startDay), f.periodKey)
	if err != nil {
		return nil, err
	}
	addBefore := func(key, amount string) {
		amt := data[key]
		if amt.budgeted.String() == "" {
			amt.budgeted = vo.NewDecimal("0")
		}
		before := amt.budgetedBefore
		if before.String() == "" {
			before = vo.NewDecimal("0")
		}
		amt.budgetedBefore = before.Add(vo.NewDecimal(amount))
		data[key] = amt
	}
	debtPlans := false
	for _, sl := range summed {
		if _, ok := f.debtByID[sl.ExternalID]; ok && model.ElementType(sl.Type) == model.ElementDebt {
			debtPlans = true
			continue
		}
		addBefore(elementKey(sl.ExternalID, model.ElementType(sl.Type)), sl.Amount)
	}
	if debtPlans {
		if err := s.addDebtPlansBefore(ctx, b, f, addBefore); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// addDebtPlansBefore sums a debt's prior plans over the months of its life
// only: a plan left in a month the debt no longer reaches (an early payoff)
// is ignored, as the row ignores it in its own month.
func (s *Service) addDebtPlansBefore(ctx context.Context, b *budgetAggregate, f filters, add func(key, amount string)) error {
	rows, err := s.read.LimitsByMonth(ctx, b.budget.ID, carryOverStart(b, f.startDay), f.periodKey)
	if err != nil {
		return err
	}
	for _, l := range rows {
		v, ok := f.debtByID[l.ExternalID]
		if !ok || model.ElementType(l.Type) != model.ElementDebt {
			continue
		}
		if m, perr := time.Parse(datetime.DateLayout, l.Month); perr == nil && plannable(v, f.startDay, m) {
			add(elementKey(v.ID, model.ElementDebt), l.Amount)
		}
	}
	return nil
}

// categorySpending holds a category's spending split into current + before.
type categorySpending struct {
	categoryID            string
	currenciesSpent       []amountSpent
	currenciesSpentBefore []amountSpent
}

// elementSpending is per-element (category, tag or debt) spending broken
// down by category.
type elementSpending struct {
	spendingInCategories map[string]*categorySpending // keyed category id
}

// buildElementsSpending computes the selected period's spending + a
// period-by-period "before" walk from budget.startedAt to periodKey.
func (s *Service) buildElementsSpending(ctx context.Context, b *budgetAggregate, f filters) (map[string]*elementSpending, error) {
	data := map[string]*elementSpending{}

	categoryIDs := make([]vo.Id, 0, len(f.categories))
	for idStr := range f.categories {
		id, err := vo.ParseId(idStr)
		if err != nil {
			return nil, err
		}
		categoryIDs = append(categoryIDs, id)
	}

	count := func(periodStart, periodEnd time.Time, current bool) error {
		rows, err := s.read.CountSpending(ctx, categoryIDs, f.everydayAccountIDs, periodStart, periodEnd)
		if err != nil {
			return err
		}
		for _, row := range rows {
			catKey := model.UncategorizedID
			if row.CategoryID != nil {
				catKey = *row.CategoryID
			}
			var key string
			switch d, isDebt := f.debtOf(row.DebtID, row.CategoryID, row.TagID); {
			case isDebt:
				// A matched payment counts on its debt row only, never on the
				// tag or category row it would otherwise land in.
				key = elementKey(d.ID, model.ElementDebt)
			case row.TagID != nil && *row.TagID != "":
				// A tag wins over the category, and over having none: a tagged
				// but uncategorized expense belongs to its tag, surfacing as an
				// Uncategorized child there rather than in the top-level row.
				key = elementKey(*row.TagID, model.ElementTag)
			default:
				key = elementKey(catKey, model.ElementCategory)
			}
			es := data[key]
			if es == nil {
				es = &elementSpending{spendingInCategories: map[string]*categorySpending{}}
				data[key] = es
			}
			cs := es.spendingInCategories[catKey]
			if cs == nil {
				cs = &categorySpending{categoryID: catKey}
				es.spendingInCategories[catKey] = cs
			}
			cid, err := vo.ParseId(row.CurrencyID)
			if err != nil {
				return err
			}
			spent := amountSpent{currencyID: cid, amount: vo.NewDecimal(row.Amount), periodStart: periodStart, periodEnd: periodEnd}
			if current {
				cs.currenciesSpent = append(cs.currenciesSpent, spent)
			} else {
				cs.currenciesSpentBefore = append(cs.currenciesSpentBefore, spent)
			}
		}
		return nil
	}

	if err := count(f.periodStart, f.periodEnd, true); err != nil {
		return nil, err
	}
	// Day 1 keeps the calendar walk from started_at itself, so a legacy
	// mid-month start (rows seeded outside NewBudget) steps exactly as it always
	// has, a 31st start clamped by AddDate included. Any other day walks plain
	// period windows from the first key.
	if f.startDay == model.MinPeriodStartDay {
		for cur := b.budget.StartedAt; cur.Before(f.periodStart); cur = cur.AddDate(0, 1, 0) {
			if err := count(cur, cur.AddDate(0, 1, 0), false); err != nil {
				return nil, err
			}
		}
		return data, nil
	}
	for key := b.budget.FirstPeriod(); key.Before(f.periodKey); key = key.AddDate(0, 1, 0) {
		start, end := model.PeriodWindow(key, f.startDay)
		if err := count(start, end, false); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// buildLabelSpending accumulates per-label spending for the current period
// only. Labels have no carryover, so unlike elements there is no month-by-month
// "before" walk. This is a deliberately separate pass from
// buildElementsSpending: label spend overlaps across labels and must never
// enter the element accumulation it feeds.
// The result is labelID -> categoryID -> amounts; a row with no category is
// filed under the shared UncategorizedID so it renders as one child rather than
// vanishing. A label's own total is the sum over its categories.
func (s *Service) buildLabelSpending(ctx context.Context, f filters) (map[string]map[string][]amountSpent, error) {
	rows, err := s.read.CountSpendingByLabel(ctx, f.everydayAccountIDs, f.periodStart, f.periodEnd)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string][]amountSpent{}
	for _, row := range rows {
		cid, perr := vo.ParseId(row.CurrencyID)
		if perr != nil {
			return nil, perr
		}
		catKey := model.UncategorizedID
		if row.CategoryID != nil {
			catKey = *row.CategoryID
		}
		byCat := out[row.LabelID]
		if byCat == nil {
			byCat = map[string][]amountSpent{}
			out[row.LabelID] = byCat
		}
		byCat[catKey] = append(byCat[catKey], amountSpent{
			currencyID: cid, amount: vo.NewDecimal(row.Amount),
			periodStart: f.periodStart, periodEnd: f.periodEnd,
		})
	}
	return out, nil
}

// elementKey is "<id>-<typeAlias>".
func elementKey(id string, t model.ElementType) string {
	return fmt.Sprintf("%s-%s", id, t.Alias())
}

// convItem builds a model.ConvertItem from an amountSpent into a target currency.
func convItem(a amountSpent, to vo.Id) model.ConvertItem {
	return model.ConvertItem{
		PeriodStart: a.periodStart,
		PeriodEnd:   a.periodEnd,
		From:        a.currencyID,
		To:          to,
		Amount:      a.amount,
	}
}

// carryOverStart is the lower bound of the plans summed into budgetedBefore:
// the stored start on day 1 (unchanged for legacy mid-month starts), the first
// period's key otherwise.
func carryOverStart(b *budgetAggregate, startDay int) time.Time {
	if startDay == model.MinPeriodStartDay {
		return b.budget.StartedAt
	}
	return b.budget.FirstPeriod()
}
