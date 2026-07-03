package budget

import (
	"context"
	"fmt"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// budgetedAmount is one element's current+prior budgeted totals.
type budgetedAmount struct {
	budgeted       vo.DecimalNumber
	budgetedBefore vo.DecimalNumber
}

// amountSpent is one currency's spent amount within a period.
type amountSpent struct {
	currencyID             vo.Id
	amount                 vo.DecimalNumber
	periodStart, periodEnd time.Time
}

// buildElementsLimits computes per-element limits: budgeted = the element's limit
// for the current month; budgetedBefore = sum of its limits for prior months
// (budget.startedAt .. periodStart). Keyed "<externalId>-<typeAlias>".
func (s *Service) buildElementsLimits(ctx context.Context, b *budgetAggregate, f filters) (map[string]budgetedAmount, error) {
	data := map[string]budgetedAmount{}

	// Current-month limits.
	current, err := s.limits.ListLimitsForPeriod(ctx, b.budget.ID, f.periodStart)
	if err != nil {
		return nil, err
	}
	// Map element id -> (externalId, typeAlias) for the limit rows.
	elemByID := map[string]*BudgetElement{}
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
		data[key] = amt
	}

	// Prior-months summed limits (budgetedBefore).
	summed, err := s.read.SummarizedLimits(ctx, b.budget.ID, b.budget.StartedAt, f.periodStart)
	if err != nil {
		return nil, err
	}
	for _, sl := range summed {
		key := elementKey(sl.ExternalID, ElementType(sl.Type))
		amt := data[key]
		if amt.budgeted.String() == "" {
			amt.budgeted = vo.NewDecimal("0")
		}
		before := amt.budgetedBefore
		if before.String() == "" {
			before = vo.NewDecimal("0")
		}
		amt.budgetedBefore = before.Add(vo.NewDecimal(sl.Amount))
		data[key] = amt
	}
	return data, nil
}

// categorySpending holds a category's spending split into current + before.
type categorySpending struct {
	categoryID            string
	currenciesSpent       []amountSpent
	currenciesSpentBefore []amountSpent
}

// elementSpending is per-element (category or tag) spending broken down by
// category.
type elementSpending struct {
	spendingInCategories map[string]*categorySpending // keyed category id
}

// buildElementsSpending computes current-period spending + a month-by-month
// "before" walk from budget.startedAt to periodStart.
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
		rows, err := s.read.CountSpending(ctx, categoryIDs, f.includedAccountIDs, periodStart, periodEnd)
		if err != nil {
			return err
		}
		for _, row := range rows {
			var key string
			if row.TagID != nil && *row.TagID != "" {
				key = elementKey(*row.TagID, ElementTag)
			} else {
				key = elementKey(row.CategoryID, ElementCategory)
			}
			es := data[key]
			if es == nil {
				es = &elementSpending{spendingInCategories: map[string]*categorySpending{}}
				data[key] = es
			}
			cs := es.spendingInCategories[row.CategoryID]
			if cs == nil {
				cs = &categorySpending{categoryID: row.CategoryID}
				es.spendingInCategories[row.CategoryID] = cs
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
	// month-by-month before: [startedAt, periodStart) in 1-month steps.
	for cur := b.budget.StartedAt; cur.Before(f.periodStart); cur = cur.AddDate(0, 1, 0) {
		next := cur.AddDate(0, 1, 0)
		if err := count(cur, next, false); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// elementKey is "<id>-<typeAlias>".
func elementKey(id string, t ElementType) string {
	return fmt.Sprintf("%s-%s", id, t.Alias())
}

// convItem builds a ConvertItem from an amountSpent into a target currency.
func convItem(a amountSpent, to vo.Id) ConvertItem {
	return ConvertItem{
		PeriodStart: a.periodStart,
		PeriodEnd:   a.periodEnd,
		From:        a.currencyID,
		To:          to,
		Amount:      a.amount,
	}
}
