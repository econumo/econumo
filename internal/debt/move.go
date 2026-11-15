package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MoveDebt attaches an active debt, with its terms, statements and rate
// changes, to another element of its budget, so a category carrying a debt
// can then move into an envelope (or the debt follow a merge done by hand).
// The target follows create's rules plus reopen's: no debt there that is
// active or created after this one, and a start after the end of the ones
// before. The currency must be the debt's, so no stored number is converted.
// The payments are matched again on the new element: every stored split is
// dropped and the next read splits them again.
func (s *Service) MoveDebt(ctx context.Context, userID vo.Id, req model.MoveDebtRequest) (*model.DebtResult, error) {
	return inOneTx(ctx, s, func(ctx context.Context) (*model.DebtResult, error) { return s.moveDebt(ctx, userID, req) })
}

func (s *Service) moveDebt(ctx context.Context, userID vo.Id, req model.MoveDebtRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	elementID, err := vo.ParseId(req.ElementId)
	if err != nil {
		return nil, err
	}
	var d *model.Debt
	var budget model.DebtBudget
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		cur, b, err := s.lockedDebt(ctx, userID, id)
		if err != nil {
			return err
		}
		d, budget = cur, b
		if cur.IsPaidOff() {
			return paidOff()
		}
		if cur.ElementID.Equal(elementID) {
			return nil
		}
		el, err := s.element(ctx, cur.BudgetID, elementID)
		if err != nil {
			return err
		}
		if !el.Eligible || el.Archived {
			return notEligible()
		}
		if !el.CurrencyID.Equal(cur.CurrencyID) {
			return elementCurrency()
		}
		// a category delete committing meanwhile would leave the debt on
		// nothing: it waits for this write and then deletes the debt
		if el.Type == model.DebtElementCategory {
			if err := s.repo.LockCategories(ctx, []vo.Id{el.ID}); err != nil {
				return err
			}
		}
		budgetDebts, err := s.repo.ListByBudget(ctx, cur.BudgetID)
		if err != nil {
			return err
		}
		others := onElement(budgetDebts, el.ID, cur)
		for _, o := range others {
			if !o.IsPaidOff() || createdBefore(cur, o) {
				return elementHasDebt()
			}
		}
		if err := checkStart(cur.StartDate, previousEnd(others, cur)); err != nil {
			return err
		}
		cur.ElementID, cur.ElementType, cur.UpdatedAt = el.ID, el.Type, s.clock.Now()
		if err := s.repo.Save(ctx, cur); err != nil {
			return err
		}
		return s.dropSplits(ctx, cur.ID)
	})
	if err != nil {
		return nil, err
	}
	return s.resultOf(ctx, budget, d)
}
