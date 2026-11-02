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

// TransferElementData moves the plans and comments of the (fromID, fromType)
// row to the (toID, toType) row in every budget where both rows are in
// currencyID. Each budget's rows are synced first, so a target row that does
// not exist yet is created. A plan for a month the target already plans is
// dropped: the target's wins. leftBehind reports source data that was not
// moved: a budget skipped for its currencies, or a plan dropped on a clash.
func (s *Service) TransferElementData(ctx context.Context, fromID vo.Id, fromType model.ElementType, toID vo.Id, toType model.ElementType, currencyID vo.Id) (leftBehind bool, err error) {
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
			dropped, err := s.moveElementData(ctx, from, to, now)
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
func (s *Service) moveElementData(ctx context.Context, from, to *model.BudgetElement, now time.Time) (dropped bool, err error) {
	limits, err := s.limits.ListLimitsByElement(ctx, from.ID)
	if err != nil {
		return false, err
	}
	for _, l := range limits {
		_, gerr := s.limits.GetLimit(ctx, to.ID, l.Period)
		taken := gerr == nil
		if gerr != nil {
			if _, ok := errs.AsNotFound(gerr); !ok {
				return false, gerr
			}
		}
		if err := s.limits.DeleteLimit(ctx, l.ID); err != nil {
			return false, err
		}
		if taken {
			dropped = true
			continue
		}
		if err := s.limits.SaveLimit(ctx, &model.BudgetElementLimit{
			ID: s.limits.NextIdentity(), ElementID: to.ID, Amount: l.Amount, Period: l.Period, CreatedAt: l.CreatedAt, UpdatedAt: now,
		}); err != nil {
			return false, err
		}
	}
	return dropped, s.comments.RepointComments(ctx, from.ID, to.ID)
}
