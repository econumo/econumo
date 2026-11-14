package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteDebt removes a debt, active or paid off, with its history (snapshots,
// splits, rate changes); its element's plans and transactions are untouched.
func (s *Service) DeleteDebt(ctx context.Context, userID vo.Id, req model.DeleteDebtRequest) (*model.DeleteDebtResult, error) {
	return inOneTx(ctx, s, func(ctx context.Context) (*model.DeleteDebtResult, error) { return s.deleteDebt(ctx, userID, req) })
}

func (s *Service) deleteDebt(ctx context.Context, userID vo.Id, req model.DeleteDebtRequest) (*model.DeleteDebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		d, _, err := s.lockedDebt(ctx, userID, id)
		if err != nil {
			return err
		}
		return s.repo.Delete(ctx, d.ID)
	})
	if err != nil {
		return nil, err
	}
	return &model.DeleteDebtResult{}, nil
}

// DeleteElementDebts deletes every debt attached to the element in the
// budget, with its history: the element is being deleted. It joins the
// caller's transaction and takes no access check.
func (s *Service) DeleteElementDebts(ctx context.Context, budgetID, elementID vo.Id) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockBudget(ctx, budgetID); err != nil {
			return err
		}
		return s.deleteOnElement(ctx, budgetID, elementID)
	})
}

func (s *Service) deleteOnElement(ctx context.Context, budgetID, elementID vo.Id) error {
	debts, err := s.repo.ListByBudget(ctx, budgetID)
	if err != nil {
		return err
	}
	for _, d := range onElement(debts, elementID, nil) {
		if err := s.repo.Delete(ctx, d.ID); err != nil {
			return err
		}
	}
	return nil
}

// MoveElementDebts follows a merge of the element into another: its debts
// move to the target only when the target carries no debt (active or paid
// off), can carry one and its row is in the debts' currency; otherwise they
// are deleted with the source, so one element's debts never overlap. It joins
// the caller's transaction and takes no access check.
func (s *Service) MoveElementDebts(ctx context.Context, budgetID, fromID, toID vo.Id) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockBudget(ctx, budgetID); err != nil {
			return err
		}
		return s.moveOnElement(ctx, budgetID, fromID, toID)
	})
}

func (s *Service) moveOnElement(ctx context.Context, budgetID, fromID, toID vo.Id) error {
	debts, err := s.repo.ListByBudget(ctx, budgetID)
	if err != nil {
		return err
	}
	moving := onElement(debts, fromID, nil)
	if len(moving) == 0 || fromID.Equal(toID) {
		return nil
	}
	if len(onElement(debts, toID, nil)) == 0 {
		target, err := s.element(ctx, budgetID, toID)
		if err != nil {
			return err
		}
		// the newest debt is in the source row's currency (a paid-off one
		// keeps the currency it ended in)
		if target.Eligible && moving[len(moving)-1].CurrencyID.Equal(target.CurrencyID) {
			return s.repo.MoveToElement(ctx, budgetID, fromID, toID, target.Type, s.clock.Now())
		}
	}
	return s.deleteOnElement(ctx, budgetID, fromID)
}
