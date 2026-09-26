package budget

import (
	"context"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateBudget updates a budget's name/currency/member accounts/savings flags
// and returns its meta. Requires update access.
func (s *Service) UpdateBudget(ctx context.Context, userID vo.Id, req model.UpdateBudgetRequest) (*model.UpdateBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	if err := model.ValidateName("Budget", req.Name); err != nil {
		return nil, err
	}
	curID, err := vo.ParseId(req.CurrencyId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"currencyId": ""})
	}

	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	// Any field change (name, currency, member accounts) requires edit rights;
	// a read-only guest must not alter budget metadata.
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}
	if !curID.Equal(b.budget.CurrencyID) {
		if eerr := s.currency.EnsureUsable(ctx, userID.String(), curID.String()); eerr != nil {
			return nil, eerr
		}
	}

	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		b.budget.UpdateName(req.Name, now)
		b.budget.UpdateCurrency(curID, now)
		if req.EndDate != nil {
			if *req.EndDate == "" {
				if eerr := b.budget.EndAt(nil, now); eerr != nil {
					return eerr
				}
			} else {
				d, perr := time.Parse(datetime.DateLayout, *req.EndDate)
				if perr != nil {
					return model.ValidateBlank(map[string]string{"endDate": ""})
				}
				if eerr := b.budget.EndAt(&d, now); eerr != nil {
					if errors.Is(eerr, model.ErrBudgetEndBeforeStart) {
						return errs.NewValidation("Validation failed", errs.FieldError{
							Key: "endDate", Message: "The end month is before the budget start",
							Code: errs.CodeBudgetEndBeforeStart,
						})
					}
					return eerr
				}
			}
		}
		// Everything is checked before the first write, so a refusal leaves the
		// name and the membership as they were. The membership is re-read here so
		// the guard judges the members as they are now, not as they were loaded.
		members, merr := s.budgets.MemberAccounts(txCtx, budgetID)
		if merr != nil {
			return merr
		}
		b.accounts = members
		change, perr := s.planOwnMembership(txCtx, userID, b, req.AccountIds, req.SavingsAccountIds, now)
		if perr != nil {
			return perr
		}
		if gerr := s.guardSavingsRemoval(txCtx, change, req.ConfirmSavingsRemoval); gerr != nil {
			return gerr
		}
		if serr := s.budgets.Save(txCtx, b.budget); serr != nil {
			return serr
		}
		return s.applyMemberChange(txCtx, change, now)
	})
	if err != nil {
		return nil, err
	}
	b, err = s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	meta, err := s.buildMeta(ctx, b)
	if err != nil {
		return nil, err
	}
	return &model.UpdateBudgetResult{Item: meta}, nil
}

// DeleteBudget deletes a budget (owner|admin). Children cascade via FKs.
func (s *Service) DeleteBudget(ctx context.Context, userID vo.Id, req model.DeleteBudgetRequest) (*model.DeleteBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canDelete(b, userID) {
		return nil, accessDenied()
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if derr := s.budgets.Delete(txCtx, budgetID); derr != nil {
			return derr
		}
		// Every participant whose active-budget option points here would keep
		// requesting a budget that now 404s.
		if cerr := s.users.ClearActiveBudget(txCtx, b.budget.UserID, budgetID); cerr != nil {
			return cerr
		}
		for _, a := range b.access {
			if cerr := s.users.ClearActiveBudget(txCtx, a.UserID, budgetID); cerr != nil {
				return cerr
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &model.DeleteBudgetResult{}, nil
}

// ResetBudget clears all element limits and comments and resets the start month (owner|admin).
func (s *Service) ResetBudget(ctx context.Context, userID vo.Id, req model.ResetBudgetRequest) (*model.ResetBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	startedAt, err := time.Parse(datetime.Layout, req.StartedAt)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"startedAt": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canReset(b, userID) {
		return nil, accessDenied()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if serr := s.limits.DeleteLimitsByBudget(txCtx, budgetID); serr != nil {
			return serr
		}
		// Reset re-anchors the start month, so comments below the new start
		// would be rows no view renders and the list endpoint still returns.
		if cerr := s.comments.DeleteCommentsByBudget(txCtx, budgetID); cerr != nil {
			return cerr
		}
		b.budget.StartFrom(startedAt, now)
		return s.budgets.Save(txCtx, b.budget)
	})
	if err != nil {
		return nil, err
	}
	b, err = s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	meta, err := s.buildMeta(ctx, b)
	if err != nil {
		return nil, err
	}
	return &model.ResetBudgetResult{Item: meta}, nil
}

// canReset = owner|admin (same as canDelete).
func (s *Service) canReset(b *budgetAggregate, userID vo.Id) bool { return s.canDelete(b, userID) }
