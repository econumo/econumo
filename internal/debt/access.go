package debt

import (
	"context"
	"errors"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// A debt belongs to a budget: reading it needs accepted access to the budget,
// writing it the right to plan there (owner, admin or user) on an unarchived
// budget. No access reads as not found, as for any other user's data.

var errNoBudgets = errors.New("debt: the budget peer is not installed")

func (s *Service) access(ctx context.Context, userID, budgetID vo.Id) (model.DebtBudget, error) {
	if s.budgets == nil {
		return model.DebtBudget{}, errNoBudgets
	}
	return s.budgets.Access(ctx, userID, budgetID)
}

// scope is the budget's member accounts, read with no access check.
func (s *Service) scope(ctx context.Context, budgetID vo.Id) (model.DebtScope, error) {
	if s.budgets == nil {
		return model.DebtScope{}, errNoBudgets
	}
	return s.budgets.Scope(ctx, budgetID)
}

func scopeOf(b model.DebtBudget) model.DebtScope {
	return model.DebtScope{BudgetID: b.ID, AccountIDs: b.AccountIDs}
}

// checkPlan refuses a write by a guest, and any write on an archived budget.
func checkPlan(b model.DebtBudget) error {
	if !b.CanPlan {
		return errs.NewAccessDenied("Access denied")
	}
	if b.IsArchived {
		return &errs.AccessDeniedError{Msg: "This budget is archived", Code: errs.CodeBudgetArchived}
	}
	return nil
}

func canEdit(b model.DebtBudget) bool { return b.CanPlan && !b.IsArchived }

// planBudget resolves a budget the user writes a debt into.
func (s *Service) planBudget(ctx context.Context, userID, budgetID vo.Id) (model.DebtBudget, error) {
	b, err := s.access(ctx, userID, budgetID)
	if err != nil {
		return b, err
	}
	return b, checkPlan(b)
}

// readableDebt masks a debt of a budget the user cannot read as not found.
func (s *Service) readableDebt(ctx context.Context, userID, id vo.Id) (*model.Debt, model.DebtBudget, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, model.DebtBudget{}, err
	}
	b, err := s.access(ctx, userID, d.BudgetID)
	if err != nil {
		if isNotFound(err) {
			return nil, b, errs.NewNotFound("Debt not found")
		}
		return nil, b, err
	}
	return d, b, nil
}

func (s *Service) editableDebt(ctx context.Context, userID, id vo.Id) (*model.Debt, model.DebtBudget, error) {
	d, b, err := s.readableDebt(ctx, userID, id)
	if err != nil {
		return nil, b, err
	}
	return d, b, checkPlan(b)
}

// lockedDebt takes the debt's budget lock, then reads the debt afresh and
// checks the user may still edit it. It runs first in a write's transaction.
func (s *Service) lockedDebt(ctx context.Context, userID, id vo.Id) (*model.Debt, model.DebtBudget, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, model.DebtBudget{}, err
	}
	if err := s.repo.LockBudget(ctx, d.BudgetID); err != nil {
		return nil, model.DebtBudget{}, err
	}
	return s.editableDebt(ctx, userID, id)
}
