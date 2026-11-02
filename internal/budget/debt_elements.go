package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
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
		if period.Before(v.StartMonth) || period.After(v.EndMonth) {
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
// currencies, or a plan dropped on a clash. Months outside t's bounds stay on
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
			dropped, err := s.moveElementData(ctx, from, to, t, now)
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
func (s *Service) moveElementData(ctx context.Context, from, to *model.BudgetElement, t model.ElementDataTransfer, now time.Time) (dropped bool, err error) {
	limits, err := s.limits.ListLimitsByElement(ctx, from.ID)
	if err != nil {
		return false, err
	}
	for _, l := range limits {
		if !inMonths(t, l.Period) {
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
	if t.FirstMonth.IsZero() && t.LastMonth.IsZero() {
		return dropped, s.comments.RepointComments(ctx, from.ID, to.ID)
	}
	first, last := t.FirstMonth, t.LastMonth
	if first.IsZero() {
		first = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if last.IsZero() {
		last = time.Date(9999, 12, 1, 0, 0, 0, 0, time.UTC)
	}
	return dropped, s.comments.RepointCommentsBetween(ctx, from.ID, to.ID, first, last)
}

func inMonths(t model.ElementDataTransfer, period time.Time) bool {
	return (t.FirstMonth.IsZero() || !period.Before(t.FirstMonth)) && (t.LastMonth.IsZero() || !period.After(t.LastMonth))
}
