package budget

import (
	"context"
	"errors"
	"time"

	dombudget "github.com/econumo/econumo/internal/domain/budget"
	"github.com/econumo/econumo/internal/domain/shared/datetime"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// ExcludeAccount excludes an account the user owns from the budget; returns meta.
func (s *Service) ExcludeAccount(ctx context.Context, userID vo.Id, req ExcludeAccountRequest) (*ExcludeAccountResult, error) {
	meta, err := s.toggleAccount(ctx, userID, req.BudgetId, req.AccountId, true)
	if err != nil {
		return nil, err
	}
	return &ExcludeAccountResult{Item: meta}, nil
}

// IncludeAccount re-includes a previously excluded account; returns meta.
func (s *Service) IncludeAccount(ctx context.Context, userID vo.Id, req IncludeAccountRequest) (*IncludeAccountResult, error) {
	meta, err := s.toggleAccount(ctx, userID, req.BudgetId, req.AccountId, false)
	if err != nil {
		return nil, err
	}
	return &IncludeAccountResult{Item: meta}, nil
}

// toggleAccount excludes (exclude=true) or includes an account; the account must
// be owned by the requester (access denied otherwise).
func (s *Service) toggleAccount(ctx context.Context, userID vo.Id, rawBudget, rawAccount string, exclude bool) (MetaResult, error) {
	budgetID, err := vo.ParseId(rawBudget)
	if err != nil {
		return MetaResult{}, validateBlank(map[string]string{"budgetId": ""})
	}
	accountID, err := vo.ParseId(rawAccount)
	if err != nil {
		return MetaResult{}, validateBlank(map[string]string{"accountId": ""})
	}
	owner, err := s.accounts.AccountOwner(ctx, accountID)
	if err != nil {
		return MetaResult{}, err
	}
	if !owner.Equal(userID) {
		return MetaResult{}, accessDenied()
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if exclude {
			return s.repo.ExcludeAccount(txCtx, budgetID, accountID)
		}
		return s.repo.IncludeAccount(txCtx, budgetID, accountID)
	}); err != nil {
		return MetaResult{}, err
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return MetaResult{}, err
	}
	return s.buildMeta(ctx, b)
}

// ChangeElementCurrency sets a budget element's display currency (canUpdate).
func (s *Service) ChangeElementCurrency(ctx context.Context, userID vo.Id, req ChangeElementCurrencyRequest) (*ChangeElementCurrencyResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	elementID, err := vo.ParseId(req.ElementId)
	if err != nil {
		return nil, validateBlank(map[string]string{"elementId": ""})
	}
	curID, err := vo.ParseId(req.CurrencyId)
	if err != nil {
		return nil, validateBlank(map[string]string{"currencyId": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		// elementId on the wire is the element's EXTERNAL id (category/tag/envelope).
		el, gerr := s.repo.GetElementByExternal(txCtx, budgetID, elementID)
		if gerr != nil {
			return gerr
		}
		el.UpdateCurrency(&curID, now)
		return s.repo.SaveElement(txCtx, el)
	})
	if err != nil {
		return nil, err
	}
	return &ChangeElementCurrencyResult{}, nil
}

// SetLimit sets or clears an element's period limit (canUpdate). amount nil ->
// delete the limit; period must be >= budget.startedAt.
func (s *Service) SetLimit(ctx context.Context, userID vo.Id, req SetLimitRequest) (*SetLimitResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	externalID, err := vo.ParseId(req.ElementId)
	if err != nil {
		return nil, validateBlank(map[string]string{"elementId": ""})
	}
	period, err := time.Parse(datetime.DateLayout, req.Period)
	if err != nil {
		return nil, validateBlank(map[string]string{"period": ""})
	}
	period = firstOfMonth(period)

	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	if period.Before(firstOfMonth(b.budget.StartedAt())) {
		return nil, validateBlank(map[string]string{"period": ""}) // invalid-date guard
	}

	// elementId on the wire is the EXTERNAL id; resolve to the budget element.
	element, err := s.repo.GetElementByExternal(ctx, budgetID, externalID)
	if err != nil {
		return nil, err
	}
	elementID := element.Id()

	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		existing, gerr := s.repo.GetLimit(txCtx, elementID, period)
		hasExisting := gerr == nil
		if gerr != nil {
			var nf *errs.NotFoundError
			if !errors.As(gerr, &nf) {
				return gerr
			}
		}
		if req.Amount == nil {
			if hasExisting {
				return s.repo.DeleteLimit(txCtx, existing.Id())
			}
			return nil
		}
		amount := vo.NewDecimal(req.Amount.String())
		if hasExisting {
			existing.UpdateAmount(amount, now)
			return s.repo.SaveLimit(txCtx, existing)
		}
		limit := dombudget.NewBudgetElementLimit(s.repo.NextIdentity(), elementID, amount, period, now)
		return s.repo.SaveLimit(txCtx, limit)
	})
	if err != nil {
		return nil, err
	}
	return &SetLimitResult{}, nil
}
