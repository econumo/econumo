package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// checkDebtPeriod refuses a plan or comment on a debt row in a month outside
// the debt's life. A row whose debt is gone names nothing.
func (s *Service) checkDebtPeriod(ctx context.Context, b *budgetAggregate, el *model.BudgetElement, period time.Time) error {
	if el.Type != model.ElementDebt {
		return nil
	}
	views, err := s.debtViews(ctx, b.participantIDs())
	if err != nil {
		return err
	}
	for _, v := range views {
		if v.ID != el.ExternalID.String() {
			continue
		}
		if !plannable(v, b.budget.PeriodStartDay, period) {
			return errs.NewValidation("Validation failed", errs.FieldError{
				Key: "period", Message: "This debt cannot be planned in this month", Code: errs.CodeBudgetDebtPeriodOutOfRange,
			})
		}
		return nil
	}
	return errs.NewNotFound("BudgetElement not found")
}

// TransferElementData moves the source row's plans and comments to the target
// row in every budget where both rows are in t.CurrencyID. Each budget's rows
// are synced first, so a target row that does not exist yet is created.
// leftBehind reports source data that was not moved: a budget skipped for its
// currencies, or a plan dropped on a clash. Months outside t's bounds, and the
// plans of a source category inside an envelope (they count nowhere), stay on
// the source by design and are not reported.
func (s *Service) TransferElementData(ctx context.Context, t model.ElementDataTransfer) (leftBehind bool, err error) {
	fromID, fromType, toID, toType, currencyID := t.FromID, t.FromType, t.ToID, t.ToType, t.CurrencyID
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		sources, err := s.elements.ListElementsByExternal(ctx, fromID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		skip := func(el *model.BudgetElement) error {
			has, err := s.rowHasData(ctx, el)
			leftBehind = leftBehind || has
			return err
		}
		for _, src := range sources {
			if src.Type != fromType {
				continue
			}
			b, err := s.budgets.GetByID(ctx, src.BudgetID)
			if err != nil {
				return err
			}
			if !rowCurrency(src, b.CurrencyID).Equal(currencyID) {
				if err := skip(src); err != nil {
					return err
				}
				continue
			}
			if err := s.syncElements(ctx, src.BudgetID, now); err != nil {
				return err
			}
			from, err := s.elementOfType(ctx, src.BudgetID, fromID, fromType)
			if err != nil {
				return err
			}
			if from == nil {
				continue
			}
			// A category inside an envelope never counts its own plans: moving
			// them would surface them on the target and inflate the totals, so
			// they stay hidden where they are.
			if sourceHidden, err := s.inEnvelope(ctx, src.BudgetID, from); err != nil {
				return err
			} else if sourceHidden {
				continue
			}
			to, err := s.elementOfType(ctx, src.BudgetID, toID, toType)
			if err != nil {
				return err
			}
			if to == nil || !rowCurrency(to, b.CurrencyID).Equal(currencyID) {
				if err := skip(from); err != nil {
					return err
				}
				continue
			}
			hidden, err := s.inEnvelope(ctx, src.BudgetID, to)
			if err != nil {
				return err
			}
			if hidden {
				if err := skip(from); err != nil {
					return err
				}
				continue
			}
			// The owner's user row is already held by the caller, so the
			// owner-before-budget lock order holds.
			if n, err := s.budgets.LockElementPair(ctx, from.ID, to.ID); err != nil {
				return err
			} else if n < 2 {
				// a row was deleted while this transfer waited for it
				continue
			}
			dropped, err := s.moveElementData(ctx, from, to, t, b.PeriodStartDay, now)
			if err != nil {
				return err
			}
			leftBehind = leftBehind || dropped
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return leftBehind, nil
}

// inEnvelope reports whether a category row sits in one of the budget's
// envelopes: such a row never shows its own plans, so data moved onto it would
// vanish from every row and total.
func (s *Service) inEnvelope(ctx context.Context, budgetID vo.Id, el *model.BudgetElement) (bool, error) {
	if el.Type != model.ElementCategory {
		return false, nil
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return false, err
	}
	children, err := s.envelopeCategories(ctx, b)
	if err != nil {
		return false, err
	}
	for _, ids := range children {
		for _, id := range ids {
			if id == el.ExternalID.String() {
				return true, nil
			}
		}
	}
	return false, nil
}

// rowCurrency is the row's own currency, else the budget's.
func rowCurrency(el *model.BudgetElement, budgetCurrency vo.Id) vo.Id {
	if el.CurrencyID != nil {
		return *el.CurrencyID
	}
	return budgetCurrency
}

func (s *Service) rowHasData(ctx context.Context, el *model.BudgetElement) (bool, error) {
	limits, err := s.limits.ListLimitsByElement(ctx, el.ID)
	if err != nil || len(limits) > 0 {
		return len(limits) > 0, err
	}
	comments, err := s.comments.ListCommentsFrom(ctx, el.BudgetID, time.Time{})
	if err != nil {
		return false, err
	}
	for _, c := range comments {
		if c.ElementID.Equal(el.ID) {
			return true, nil
		}
	}
	return false, nil
}

// LockElements write-locks every budget's row of the external id.
func (s *Service) LockElements(ctx context.Context, externalID vo.Id, typ model.ElementType) error {
	return s.budgets.LockElementsByExternal(ctx, externalID, typ)
}

// RemoveElements deletes the external id's rows of this type in every budget;
// their plans and comments go with them.
func (s *Service) RemoveElements(ctx context.Context, externalID vo.Id, typ model.ElementType) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		rows, err := s.elements.ListElementsByExternal(ctx, externalID)
		if err != nil {
			return err
		}
		for _, el := range rows {
			if el.Type != typ {
				continue
			}
			if err := s.elements.DeleteElement(ctx, el.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetElementsCurrency moves the external id's rows of this type in every
// budget to currencyID; their plans keep their numbers, as with a row's own
// currency change.
func (s *Service) SetElementsCurrency(ctx context.Context, externalID vo.Id, typ model.ElementType, currencyID vo.Id) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		rows, err := s.elements.ListElementsByExternal(ctx, externalID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, el := range rows {
			if el.Type != typ || (el.CurrencyID != nil && el.CurrencyID.Equal(currencyID)) {
				continue
			}
			cid := currencyID
			el.UpdateCurrency(&cid, now)
			if err := s.elements.SaveElement(ctx, el); err != nil {
				return err
			}
		}
		return nil
	})
}

// ElementHasData reports whether the row holds a plan or a comment in any
// budget.
func (s *Service) ElementHasData(ctx context.Context, externalID vo.Id, typ model.ElementType) (bool, error) {
	return s.budgets.ElementHasData(ctx, externalID, typ)
}

// elementOfType returns nil when the budget has no such row.
func (s *Service) elementOfType(ctx context.Context, budgetID, externalID vo.Id, typ model.ElementType) (*model.BudgetElement, error) {
	el, err := s.elements.GetElementByExternal(ctx, budgetID, externalID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, nil
		}
		return nil, err
	}
	if el.Type != typ {
		return nil, nil
	}
	return el, nil
}

// moveElementData reports whether a source plan was dropped on a clash.
func (s *Service) moveElementData(ctx context.Context, from, to *model.BudgetElement, t model.ElementDataTransfer, startDay int, now time.Time) (dropped bool, err error) {
	limits, err := s.limits.ListLimitsByElement(ctx, from.ID)
	if err != nil {
		return false, err
	}
	first, last := transferPeriods(t, startDay)
	for _, l := range limits {
		if !inPeriods(first, last, l.Period) {
			continue
		}
		existing, gerr := s.limits.GetLimit(ctx, to.ID, l.Period)
		if gerr != nil {
			if _, ok := errs.AsNotFound(gerr); !ok {
				return false, gerr
			}
			existing = nil
		}
		if err := s.limits.DeleteLimit(ctx, l.ID); err != nil {
			return false, err
		}
		switch {
		case existing == nil:
			err = s.limits.SaveLimit(ctx, &model.BudgetElementLimit{
				ID: s.limits.NextIdentity(), ElementID: to.ID, Amount: l.Amount, Period: l.Period, CreatedAt: l.CreatedAt, UpdatedAt: now,
			})
		case t.SumOnClash:
			existing.Amount = existing.Amount.Add(l.Amount)
			existing.UpdatedAt = now
			err = s.limits.SaveLimit(ctx, existing)
		default:
			dropped = true
		}
		if err != nil {
			return false, err
		}
	}
	if first.IsZero() && last.IsZero() {
		return dropped, s.comments.RepointComments(ctx, from.ID, to.ID)
	}
	if first.IsZero() {
		first = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if last.IsZero() {
		last = time.Date(9999, 12, 1, 0, 0, 0, 0, time.UTC)
	}
	return dropped, s.comments.RepointCommentsBetween(ctx, from.ID, to.ID, first, last)
}

// transferPeriods maps t's day bounds onto the keys of the periods holding
// them in a budget whose periods start on startDay; zero stays open.
func transferPeriods(t model.ElementDataTransfer, startDay int) (first, last time.Time) {
	switch {
	case !t.AfterDay.IsZero():
		first = model.PeriodOf(t.AfterDay, startDay).AddDate(0, 1, 0)
	case !t.FirstDay.IsZero():
		first = model.PeriodOf(t.FirstDay, startDay)
	}
	switch {
	case !t.BeforeDay.IsZero():
		last = model.PeriodOf(t.BeforeDay, startDay).AddDate(0, -1, 0)
	case !t.LastDay.IsZero():
		last = model.PeriodOf(t.LastDay, startDay)
	}
	return first, last
}

func inPeriods(first, last, period time.Time) bool {
	return (first.IsZero() || !period.Before(first)) && (last.IsZero() || !period.After(last))
}

// DebtRowPlans returns the plans the budget holds for the debt's row, by
// period key, with the budget's name and period start day; ok is false when
// the user cannot read the budget or it has no row for the debt.
func (s *Service) DebtRowPlans(ctx context.Context, userID, budgetID, debtID vo.Id) (model.DebtBudgetPlans, bool, error) {
	b, err := s.budgets.GetByID(ctx, budgetID)
	if err != nil {
		if _, nf := errs.AsNotFound(err); nf {
			return model.DebtBudgetPlans{}, false, nil
		}
		return model.DebtBudgetPlans{}, false, err
	}
	access, err := s.access.ListAccess(ctx, budgetID)
	if err != nil {
		return model.DebtBudgetPlans{}, false, err
	}
	if !s.canRead(&budgetAggregate{budget: b, access: access}, userID) {
		return model.DebtBudgetPlans{}, false, nil
	}
	el, err := s.elementOfType(ctx, budgetID, debtID, model.ElementDebt)
	if err != nil || el == nil {
		return model.DebtBudgetPlans{}, false, err
	}
	limits, err := s.limits.ListLimitsByElement(ctx, el.ID)
	if err != nil {
		return model.DebtBudgetPlans{}, false, err
	}
	plans := make(map[string]string, len(limits))
	for _, l := range limits {
		plans[l.Period.Format(datetime.DateLayout)] = l.Amount.String()
	}
	return model.DebtBudgetPlans{Name: b.Name, PeriodStartDay: b.PeriodStartDay, Plans: plans}, true, nil
}
