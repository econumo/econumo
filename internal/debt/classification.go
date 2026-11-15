package debt

import (
	"context"
	"errors"
	"slices"

	"github.com/econumo/econumo/internal/shared/vo"
)

// ErrDebtsChangedConcurrently: a debt attached to the category was committed
// after Prepare read them, so its budget is not locked. Taking that lock
// after the category row would invert the order every debt write keeps; the
// delete or merge fails instead (a 5xx, so clients retry it).
var ErrDebtsChangedConcurrently = errors.New("debt: a debt on the category changed meanwhile; try again")

// A category's owner may delete or merge it whatever their access to the
// debts attached to it (a former participant, a guest): the debts follow,
// and nothing is refused (ErrDebtsChangedConcurrently is a race to retry, not
// a refusal). These run inside the delete's or the merge's transaction and
// take no access check.

// budgetsOn are the budgets of the debts attached to any of ids, in id order:
// the order every caller locks them in.
func (s *Service) budgetsOn(ctx context.Context, ids ...vo.Id) ([]vo.Id, error) {
	var out []vo.Id
	for _, id := range ids {
		debts, err := s.repo.ListByElement(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, d := range debts {
			if !slices.ContainsFunc(out, d.BudgetID.Equal) {
				out = append(out, d.BudgetID)
			}
		}
	}
	slices.SortFunc(out, compareIDs)
	return out, nil
}

// LockForCategories locks the budgets of the debts attached to any of the
// categories. A delete or a merge takes them first, so its lock order matches
// every debt write's: the budget rows first, then the category row.
func (s *Service) LockForCategories(ctx context.Context, ids ...vo.Id) error {
	budgets, err := s.budgetsOn(ctx, ids...)
	if err != nil {
		return err
	}
	for _, b := range budgets {
		if err := s.repo.LockBudget(ctx, b); err != nil {
			return err
		}
	}
	return nil
}

// heldOn re-reads, under the category row lock, the budgets of the debts on
// id; each must be one the transaction already locked (LockForCategories).
func (s *Service) heldOn(ctx context.Context, id vo.Id) ([]vo.Id, error) {
	budgets, err := s.budgetsOn(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, b := range budgets {
		if !s.repo.HoldsBudgetLock(ctx, b) {
			return nil, ErrDebtsChangedConcurrently
		}
	}
	return budgets, nil
}

// ReleaseCategory deletes the debts attached to the category, in every
// budget, with their history; called before the category is deleted.
func (s *Service) ReleaseCategory(ctx context.Context, id vo.Id) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		budgets, err := s.heldOn(ctx, id)
		if err != nil {
			return err
		}
		for _, b := range budgets {
			if err := s.deleteOnElement(ctx, b, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// ReassignCategory follows a merge of oldID into newID, after the budget rows
// moved: in each budget the debts move to newID or go (MoveElementDebts).
func (s *Service) ReassignCategory(ctx context.Context, oldID, newID vo.Id) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		budgets, err := s.heldOn(ctx, oldID)
		if err != nil {
			return err
		}
		if _, err := s.heldOn(ctx, newID); err != nil {
			return err
		}
		for _, b := range budgets {
			if err := s.moveOnElement(ctx, b, oldID, newID); err != nil {
				return err
			}
		}
		return nil
	})
}
