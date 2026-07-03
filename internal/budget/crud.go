package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateBudget updates a budget's name/currency/excluded-accounts and returns its
// meta. Requires read access; a name change additionally requires update access.
func (s *Service) UpdateBudget(ctx context.Context, userID vo.Id, req UpdateBudgetRequest) (*UpdateBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, validateBlank(map[string]string{"id": ""})
	}
	if err := ValidateName("Budget", req.Name); err != nil {
		return nil, err
	}
	curID, err := vo.ParseId(req.CurrencyId)
	if err != nil {
		return nil, validateBlank(map[string]string{"currencyId": ""})
	}

	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canRead(b, userID) {
		return nil, accessDenied()
	}
	if b.budget.Name() != req.Name && !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}

	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		b.budget.UpdateName(req.Name, now)
		b.budget.UpdateCurrency(curID, now)
		if serr := s.repo.Save(txCtx, b.budget); serr != nil {
			return serr
		}
		// Replace the excluded-account set.
		want := map[string]bool{}
		for _, raw := range req.ExcludedAccounts {
			aid, perr := vo.ParseId(raw)
			if perr != nil {
				return validateBlank(map[string]string{"excludedAccounts": ""})
			}
			want[aid.String()] = true
			if serr := s.repo.ExcludeAccount(txCtx, budgetID, aid); serr != nil {
				return serr
			}
		}
		for _, existing := range b.excludedAccountIDs {
			if !want[existing.String()] {
				if serr := s.repo.IncludeAccount(txCtx, budgetID, existing); serr != nil {
					return serr
				}
			}
		}
		return nil
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
	return &UpdateBudgetResult{Item: meta}, nil
}

// DeleteBudget deletes a budget (owner|admin). Children cascade via FKs.
func (s *Service) DeleteBudget(ctx context.Context, userID vo.Id, req DeleteBudgetRequest) (*DeleteBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, validateBlank(map[string]string{"id": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canDelete(b, userID) {
		return nil, accessDenied()
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		return s.repo.Delete(txCtx, budgetID)
	}); err != nil {
		return nil, err
	}
	return &DeleteBudgetResult{}, nil
}

// ResetBudget clears all element limits and resets the start month (owner|admin).
func (s *Service) ResetBudget(ctx context.Context, userID vo.Id, req ResetBudgetRequest) (*ResetBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, validateBlank(map[string]string{"id": ""})
	}
	startedAt, err := time.Parse(datetime.Layout, req.StartedAt)
	if err != nil {
		return nil, validateBlank(map[string]string{"startedAt": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canReset(b, userID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if serr := s.repo.DeleteLimitsByBudget(txCtx, budgetID); serr != nil {
			return serr
		}
		b.budget.StartFrom(startedAt, now)
		return s.repo.Save(txCtx, b.budget)
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
	return &ResetBudgetResult{Item: meta}, nil
}

// canReset = owner|admin (same as canDelete).
func (s *Service) canReset(b *budgetAggregate, userID vo.Id) bool { return s.canDelete(b, userID) }
