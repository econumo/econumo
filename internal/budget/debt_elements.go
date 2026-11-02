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
// row to the (toID, toType) row in every budget where the source row is in
// currencyID. Each budget's rows are synced first, so a target row that does
// not exist yet is created. A plan for a month the target already plans is
// dropped: the target's wins.
func (s *Service) TransferElementData(ctx context.Context, fromID vo.Id, fromType model.ElementType, toID vo.Id, toType model.ElementType, currencyID vo.Id) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		sources, err := s.elements.ListElementsByExternal(ctx, fromID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, src := range sources {
			if src.Type != fromType {
				continue
			}
			b, err := s.budgets.GetByID(ctx, src.BudgetID)
			if err != nil {
				return err
			}
			currency := b.CurrencyID
			if src.CurrencyID != nil {
				currency = *src.CurrencyID
			}
			if !currency.Equal(currencyID) {
				continue
			}
			if err := s.syncElements(ctx, src.BudgetID, now); err != nil {
				return err
			}
			from, err := s.elementOfType(ctx, src.BudgetID, fromID, fromType)
			if err != nil {
				return err
			}
			to, err := s.elementOfType(ctx, src.BudgetID, toID, toType)
			if err != nil {
				return err
			}
			if from == nil || to == nil {
				continue
			}
			if err := s.moveElementData(ctx, from, to, now); err != nil {
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

func (s *Service) moveElementData(ctx context.Context, from, to *model.BudgetElement, now time.Time) error {
	limits, err := s.limits.ListLimitsByElement(ctx, from.ID)
	if err != nil {
		return err
	}
	for _, l := range limits {
		_, gerr := s.limits.GetLimit(ctx, to.ID, l.Period)
		taken := gerr == nil
		if gerr != nil {
			if _, ok := errs.AsNotFound(gerr); !ok {
				return gerr
			}
		}
		if err := s.limits.DeleteLimit(ctx, l.ID); err != nil {
			return err
		}
		if taken {
			continue
		}
		if err := s.limits.SaveLimit(ctx, &model.BudgetElementLimit{
			ID: s.limits.NextIdentity(), ElementID: to.ID, Amount: l.Amount, Period: l.Period, CreatedAt: l.CreatedAt, UpdatedAt: now,
		}); err != nil {
			return err
		}
	}
	return s.comments.RepointComments(ctx, from.ID, to.ID)
}
